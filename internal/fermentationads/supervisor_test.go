package fermentationads

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeFence struct {
	started chan struct{}
	release chan struct{}
	err     error
}

func (f *fakeFence) FenceWrites(ctx context.Context) error {
	close(f.started)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-f.release:
		return f.err
	}
}
func TestSupervisorWaitsForUAFenceBeforeAnyADSCommand(t *testing.T) {
	fence := &fakeFence{started: make(chan struct{}), release: make(chan struct{})}
	link := &fakeLink{}
	desired := make(chan Decision, 1)
	desired <- Decision{DecidedAt: time.Now()}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	var s Supervisor
	go func() { done <- s.Run(ctx, fence, link, desired, nil, time.Second) }()
	<-fence.started
	link.mu.Lock()
	n := len(link.commands)
	link.mu.Unlock()
	if n != 0 {
		t.Fatal("ADS wrote before UA fence")
	}
	close(fence.release)
	time.Sleep(80 * time.Millisecond)
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := s.Run(context.Background(), fence, link, desired, nil, time.Second); err == nil {
		t.Fatal("supervisor accepted reuse")
	}
}
func TestSupervisorRejectsUncertainUAWrites(t *testing.T) {
	fence := &fakeFence{started: make(chan struct{}), release: make(chan struct{}), err: errors.New("uncertain")}
	close(fence.release)
	link := &fakeLink{}
	desired := make(chan Decision)
	var s Supervisor
	if err := s.Run(context.Background(), fence, link, desired, nil, time.Second); err == nil {
		t.Fatal("expected fence failure")
	}
	if len(link.commands) != 0 {
		t.Fatal("ADS command despite fence failure")
	}
}

func TestSupervisorDoesNotWriteADSIfUAFenceTimesOut(t *testing.T) {
	fence := &fakeFence{started: make(chan struct{}), release: make(chan struct{})}
	link := &fakeLink{}
	desired := make(chan Decision)
	var s Supervisor
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := s.Run(ctx, fence, link, desired, nil, time.Second); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected timeout: %v", err)
	}
	if len(link.commands) != 0 {
		t.Fatal("ADS command despite unresolved UA write")
	}
}
