package fermentationads

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/siyka-au/go-ads/v3/ams"
)

const (
	simCommandHandle = 11
	simCommitHandle  = 12
	simStatusHandle  = 13
	simAppliedHandle = 14
)

var errLostCommitReply = errors.New("simulated lost commit reply")

// simulatedWire accepts a command only at the final commit and publishes
// stable V1 status and applied images. It has no process I/O.
type simulatedWire struct {
	staged      []byte
	committed   [][]byte
	commitClear bool
	dropReplyAt int
	closed      int
	writes      int
}

func (w *simulatedWire) Write(_ context.Context, group ams.Group, handle uint32, value []byte) error {
	w.writes++
	if group != ams.GroupSymbolValueByHandle {
		return fmt.Errorf("unexpected write group")
	}
	switch {
	case handle == simCommitHandle && len(value) == 4:
		sequence := binary.LittleEndian.Uint32(value)
		if sequence == 0 {
			w.commitClear = true
			w.staged = nil
			return nil
		}
		if w.staged == nil || binary.LittleEndian.Uint32(w.staged[8:]) != sequence {
			return fmt.Errorf("commit did not match the staged command")
		}
		w.committed = append(w.committed, w.staged)
		w.staged = nil
		w.commitClear = false
		if len(w.committed) == w.dropReplyAt {
			return errLostCommitReply
		}
		return nil
	case handle == simCommandHandle && w.commitClear && len(value) == CommandSize:
		w.staged = append([]byte(nil), value...)
		return nil
	default:
		return fmt.Errorf("unexpected write stage")
	}
}

func (w *simulatedWire) Read(_ context.Context, group ams.Group, handle uint32, size uint32) ([]byte, error) {
	if group != ams.GroupSymbolValueByHandle || len(w.committed) == 0 {
		return nil, fmt.Errorf("unexpected read")
	}
	command := w.committed[len(w.committed)-1]
	value := make([]byte, size)
	binary.LittleEndian.PutUint16(value, Version)
	binary.LittleEndian.PutUint16(value[2:], uint16(size))
	binary.LittleEndian.PutUint32(value[4:], 2*uint32(len(w.committed)))
	copy(value[8:16], command[4:12])
	switch {
	case handle == simStatusHandle && size == StatusSize:
		value[24], value[25], value[26], value[27] = 1, command[18], 1, byte(ADS)
	case handle == simAppliedHandle && size == AppliedSize:
		copy(value[16:22], command[13:19])
	default:
		return nil, fmt.Errorf("unexpected read handle or size")
	}
	return value, nil
}

func (w *simulatedWire) transport() *Transport {
	return &Transport{
		client: w, command: simCommandHandle, commit: simCommitHandle, status: simStatusHandle, applied: simAppliedHandle,
		closeSession: func() error { w.closed++; return nil },
	}
}

func TestSupervisorAmbiguousCommitRequiresExplicitFreshActivation(t *testing.T) {
	newFence := func() *fakeFence {
		fence := &fakeFence{started: make(chan struct{}), release: make(chan struct{})}
		close(fence.release)
		return fence
	}
	newDecision := func() chan Decision {
		decisions := make(chan Decision, 1)
		decisions <- Decision{DecidedAt: time.Now(), Outputs: Outputs{F1Cooling: true}}
		return decisions
	}
	firstWire := &simulatedWire{dropReplyAt: 2}
	firstTransport := firstWire.transport()
	firstFence := newFence()
	firstFactory := func(context.Context) (Link, error) { return firstTransport, nil }
	var first Supervisor
	err := first.Run(context.Background(), firstFence, firstFactory, newDecision(), nil, time.Second)
	if !errors.Is(err, errLostCommitReply) || !firstTransport.closed || firstWire.closed != 1 || firstFence.aborts != 0 {
		t.Fatalf("lost commit reply must stop ADS and keep UA fenced: err=%v, closes=%d, aborts=%d", err, firstWire.closed, firstFence.aborts)
	}
	if firstWire.writes != 6 || len(firstWire.committed) != 2 || firstWire.committed[1][18] != 1 {
		t.Fatalf("expected disabled and delivered enabled transactions without a retry, writes=%d commits=%d", firstWire.writes, len(firstWire.committed))
	}
	if err := first.Run(context.Background(), firstFence, firstFactory, newDecision(), nil, time.Second); err == nil {
		t.Fatal("used supervisor accepted reactivation")
	}
	if err := firstTransport.WriteCommand(context.Background(), Command{Session: 1, Sequence: 1, Owner: ADS}); err == nil || firstWire.writes != 6 {
		t.Fatal("fenced transport emitted another command")
	}

	// Simulate an independently reconciled environment with a fresh fence and
	// connection. Neither the transport nor coordinator establishes recovery.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	secondWire := &simulatedWire{}
	observations := make(chan Observation, 2)
	done := make(chan error, 1)
	var explicitlyActivated Supervisor
	go func() {
		done <- explicitlyActivated.Run(ctx, newFence(), func(context.Context) (Link, error) {
			return secondWire.transport(), nil
		}, newDecision(), observations, time.Second)
	}()
	for i := 0; i < 2; i++ {
		select {
		case observation := <-observations:
			if observation.Status.Enabled != (i == 1) || !observation.Converged {
				t.Errorf("unexpected observation: %+v", observation)
			}
		case <-ctx.Done():
			t.Fatal("fresh activation failed to acknowledge")
		}
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if secondWire.closed != 1 || len(secondWire.committed) != 2 || secondWire.committed[0][18] != 0 ||
		binary.LittleEndian.Uint32(secondWire.committed[0][8:]) != 1 {
		t.Fatal("fresh activation must explicitly reconfirm a disabled image before enable")
	}
	session := binary.LittleEndian.Uint32(secondWire.committed[0][4:])
	if session == 0 || session == binary.LittleEndian.Uint32(firstWire.committed[0][4:]) {
		t.Fatal("fresh activation did not start a new session")
	}
}
