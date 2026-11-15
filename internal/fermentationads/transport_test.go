package fermentationads

import (
	"context"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/siyka-au/go-ads/v3/ams"
)

type writeCall struct {
	group  ams.Group
	handle uint32
	value  []byte
}
type fakeWire struct {
	calls  []writeCall
	failAt int
}

func (f *fakeWire) Read(context.Context, ams.Group, uint32, uint32) ([]byte, error) {
	return nil, errors.New("unexpected read")
}
func (f *fakeWire) Write(_ context.Context, g ams.Group, h uint32, b []byte) error {
	f.calls = append(f.calls, writeCall{g, h, append([]byte(nil), b...)})
	if len(f.calls) == f.failAt {
		return errors.New("injected failure")
	}
	return nil
}
func TestCommandStagesAndCommitStopsOnFailure(t *testing.T) {
	for failAt := 0; failAt <= 3; failAt++ {
		f := &fakeWire{failAt: failAt}
		tr := Transport{client: f, command: 11, commit: 12}
		c := Command{Session: 5, Sequence: 9, Owner: ADS}
		err := tr.WriteCommand(context.Background(), c)
		if (err != nil) != (failAt != 0) {
			t.Fatalf("failAt %d: %v", failAt, err)
		}
		n := 3
		if failAt != 0 {
			n = failAt
		}
		if len(f.calls) != n {
			t.Fatalf("failAt %d: %d writes", failAt, len(f.calls))
		}
		if f.calls[0].handle != 12 || binary.LittleEndian.Uint32(f.calls[0].value) != 0 {
			t.Fatal("first write did not clear commit")
		}
		if n > 1 && (f.calls[1].handle != 11 || len(f.calls[1].value) != CommandSize) {
			t.Fatal("bad command stage")
		}
		if n > 2 && (f.calls[2].handle != 12 || binary.LittleEndian.Uint32(f.calls[2].value) != 9) {
			t.Fatal("bad final commit")
		}
		for _, call := range f.calls {
			if call.group != ams.GroupSymbolValueByHandle {
				t.Fatal("unexpected group")
			}
		}
	}
	f := &fakeWire{}
	tr := Transport{client: f, command: 11, commit: 12}
	if tr.WriteCommand(context.Background(), Command{}) == nil || len(f.calls) != 0 {
		t.Fatal("invalid command wrote to transport")
	}
}
