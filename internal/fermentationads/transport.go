package fermentationads

import (
	"context"
	"encoding/binary"
	"fmt"

	adsclient "github.com/siyka-au/go-ads/v3"
	"github.com/siyka-au/go-ads/v3/ams"
)

type wireClient interface {
	Read(context.Context, ams.Group, uint32, uint32) ([]byte, error)
	Write(context.Context, ams.Group, uint32, []byte) error
}

// Transport uses a connected SDK session. Construction does not connect; callers
// must supply an explicitly connected session with route registration disabled.
// Symbol handles and sizes are resolved before any process writes are possible.
type Transport struct {
	client                                   wireClient
	inputs, command, commit, status, applied uint32
}

func NewTransport(ctx context.Context, session *adsclient.Session) (*Transport, error) {
	if session == nil {
		return nil, fmt.Errorf("nil ADS session")
	}
	t := &Transport{client: session.Client()}
	symbols := []struct {
		name   string
		size   uint32
		target *uint32
	}{
		{"GVL_FermentationADS.stInputs", InputSize, &t.inputs}, {"GVL_FermentationADS.stCommand", CommandSize, &t.command}, {"GVL_FermentationADS.udCommandCommit", 4, &t.commit}, {"GVL_FermentationADS.stStatus", StatusSize, &t.status}, {"GVL_FermentationADS.stAppliedOutputs", AppliedSize, &t.applied},
	}
	seen := map[uint32]bool{}
	for _, s := range symbols {
		sym, err := session.Symbol(ctx, s.name)
		if err != nil {
			return nil, fmt.Errorf("resolve %s: %w", s.name, err)
		}
		if sym.Length != s.size || sym.Handle == 0 || seen[sym.Handle] {
			return nil, fmt.Errorf("invalid metadata for %s", s.name)
		}
		seen[sym.Handle] = true
		*s.target = sym.Handle
	}
	return t, nil
}
func (t *Transport) snapshot(ctx context.Context, handle uint32, size uint32) ([][]byte, error) {
	images := make([][]byte, 3)
	for i := range images {
		b, err := t.client.Read(ctx, ams.GroupSymbolValueByHandle, handle, size)
		if err != nil {
			return nil, err
		}
		images[i] = b
	}
	return images, nil
}
func (t *Transport) ReadInputs(ctx context.Context) (Inputs, error) {
	b, e := t.snapshot(ctx, t.inputs, InputSize)
	if e != nil {
		return Inputs{}, e
	}
	return DecodeInputs(b[0], b[1], b[2])
}
func (t *Transport) ReadStatus(ctx context.Context) (Status, error) {
	b, e := t.snapshot(ctx, t.status, StatusSize)
	if e != nil {
		return Status{}, e
	}
	return DecodeStatus(b[0], b[1], b[2])
}
func (t *Transport) ReadApplied(ctx context.Context) (Applied, error) {
	b, e := t.snapshot(ctx, t.applied, AppliedSize)
	if e != nil {
		return Applied{}, e
	}
	return DecodeApplied(b[0], b[1], b[2])
}

// WriteCommand stages the complete command before its commit. Any error leaves
// ownership and outcome uncertain: the coordinator must latch a fault and stop.
func (t *Transport) WriteCommand(ctx context.Context, c Command) error {
	image, err := c.MarshalBinary()
	if err != nil {
		return err
	}
	commit := make([]byte, 4)
	if err = t.client.Write(ctx, ams.GroupSymbolValueByHandle, t.commit, commit); err != nil {
		return fmt.Errorf("clear commit: %w", err)
	}
	if err = t.client.Write(ctx, ams.GroupSymbolValueByHandle, t.command, image); err != nil {
		return fmt.Errorf("stage command: %w", err)
	}
	binary.LittleEndian.PutUint32(commit, c.Sequence)
	if err = t.client.Write(ctx, ams.GroupSymbolValueByHandle, t.commit, commit); err != nil {
		return fmt.Errorf("commit command: %w", err)
	}
	return nil
}
