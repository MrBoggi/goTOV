package fermentationads

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"

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
	mu                                       sync.Mutex
	closed                                   bool
	closeErr                                 error
	closeSession                             func() error
	client                                   wireClient
	inputs, command, commit, status, applied uint32
}

func NewTransport(ctx context.Context, session *adsclient.Session) (*Transport, error) {
	if session == nil {
		return nil, fmt.Errorf("nil ADS session")
	}
	t := &Transport{client: session.Client(), closeSession: session.Close}
	success := false
	defer func() {
		if !success {
			_ = session.Close()
		}
	}()
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
	success = true
	return t, nil
}

// Close permanently fences this transport and closes its session. It is safe to
// call more than once. A previously transmitted frame cannot be recalled.
func (t *Transport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.closeLocked()
}
func (t *Transport) closeLocked() error {
	if !t.closed {
		t.closed = true
		if t.closeSession != nil {
			t.closeErr = t.closeSession()
		}
	}
	return t.closeErr
}
func (t *Transport) enter(ctx context.Context) error {
	if t.closed {
		return fmt.Errorf("ADS transport fenced")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}
func (t *Transport) snapshot(ctx context.Context, handle uint32, size uint32) ([][]byte, error) {
	images := make([][]byte, 3)
	for i := range images {
		b, err := t.client.Read(ctx, ams.GroupSymbolValueByHandle, handle, size)
		if err != nil {
			return nil, err
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		images[i] = b
	}
	return images, nil
}
func (t *Transport) ReadInputs(ctx context.Context) (value Inputs, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	defer func() {
		if err != nil {
			err = errors.Join(err, t.closeLocked())
		}
	}()
	if err = t.enter(ctx); err != nil {
		return
	}
	var b [][]byte
	b, err = t.snapshot(ctx, t.inputs, InputSize)
	if err != nil {
		return
	}
	value, err = DecodeInputs(b[0], b[1], b[2])
	return
}
func (t *Transport) ReadStatus(ctx context.Context) (value Status, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	defer func() {
		if err != nil {
			err = errors.Join(err, t.closeLocked())
		}
	}()
	if err = t.enter(ctx); err != nil {
		return
	}
	var b [][]byte
	b, err = t.snapshot(ctx, t.status, StatusSize)
	if err != nil {
		return
	}
	value, err = DecodeStatus(b[0], b[1], b[2])
	return
}
func (t *Transport) ReadApplied(ctx context.Context) (value Applied, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	defer func() {
		if err != nil {
			err = errors.Join(err, t.closeLocked())
		}
	}()
	if err = t.enter(ctx); err != nil {
		return
	}
	var b [][]byte
	b, err = t.snapshot(ctx, t.applied, AppliedSize)
	if err != nil {
		return
	}
	value, err = DecodeApplied(b[0], b[1], b[2])
	return
}

// WriteCommand stages the complete command before its commit. Any error leaves
// ownership and outcome uncertain: the coordinator must latch a fault and stop.
func (t *Transport) WriteCommand(ctx context.Context, c Command) (err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	defer func() {
		if err != nil {
			err = errors.Join(err, t.closeLocked())
		}
	}()
	if err = t.enter(ctx); err != nil {
		return err
	}
	image, err := c.MarshalBinary()
	if err != nil {
		return err
	}
	commit := make([]byte, 4)
	if err = t.client.Write(ctx, ams.GroupSymbolValueByHandle, t.commit, commit); err != nil {
		return fmt.Errorf("clear commit: %w", err)
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = t.client.Write(ctx, ams.GroupSymbolValueByHandle, t.command, image); err != nil {
		return fmt.Errorf("stage command: %w", err)
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	binary.LittleEndian.PutUint32(commit, c.Sequence)
	if err = t.client.Write(ctx, ams.GroupSymbolValueByHandle, t.commit, commit); err != nil {
		return fmt.Errorf("commit command: %w", err)
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	return nil
}
