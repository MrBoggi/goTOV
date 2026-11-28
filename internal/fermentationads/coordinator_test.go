package fermentationads

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeLink struct {
	mu            sync.Mutex
	commands      []Command
	failAt        int
	forceMismatch bool
	statusReads   int
	status        func(Command, Status) Status
	applied       func(Command, Applied) Applied
}

func (f *fakeLink) WriteCommand(_ context.Context, c Command) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.commands = append(f.commands, c)
	if len(f.commands) == f.failAt {
		return errors.New("write failed")
	}
	return nil
}
func (f *fakeLink) Close() error { return nil }
func (f *fakeLink) current() Command {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.commands[len(f.commands)-1]
}
func (f *fakeLink) ReadStatus(context.Context) (Status, error) {
	c := f.current()
	f.statusReads++
	st := Status{Session: c.Session, AppliedSequence: c.Sequence, WatchdogHealthy: true, Enabled: c.Enable, Valid: true, Owner: ADS}
	if f.status != nil {
		st = f.status(c, st)
	}
	return st, nil
}
func (f *fakeLink) ReadApplied(context.Context) (Applied, error) {
	c := f.current()
	out := c.Outputs
	if f.forceMismatch && c.Enable {
		out = Outputs{F1Cooling: true}
	}
	ap := Applied{Session: c.Session, AppliedSequence: c.Sequence, Outputs: out, Enabled: c.Enable}
	if f.applied != nil {
		ap = f.applied(c, ap)
	}
	return ap, nil
}

func TestRunRechecksOwnershipBeforeEnable(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Status)
	}{
		{"PLC fault", func(st *Status) { st.Faults = 1 }},
		{"owner loss", func(st *Status) { st.Owner = Safe }},
		{"watchdog loss", func(st *Status) { st.WatchdogHealthy = false }},
		{"PLC restart", func(st *Status) { *st = Status{Owner: Safe} }},
		{"new session", func(st *Status) { st.Session ^= 1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeLink{}
			f.status = func(_ Command, st Status) Status {
				if f.statusReads > 1 {
					tc.change(&st)
				}
				return st
			}
			desired := make(chan Decision, 1)
			desired <- Decision{DecidedAt: time.Now()}
			err := run(context.Background(), f, desired, nil, time.Second)
			if err == nil || !strings.Contains(err.Error(), "verify existing ADS ownership") {
				t.Fatalf("expected preflight rejection, got %v", err)
			}
			if len(f.commands) != 1 || f.commands[0].Enable || f.commands[0].Owner != ADS {
				t.Fatalf("unexpected commands %+v", f.commands)
			}
		})
	}
}

func TestRunRejectsUnsafeStatusBeforeMatchingAppliedAcknowledgement(t *testing.T) {
	f := &fakeLink{
		status: func(c Command, st Status) Status {
			if c.Enable {
				st.Faults = 1
			}
			return st
		},
		applied: func(c Command, ap Applied) Applied {
			if c.Enable {
				ap.AppliedSequence--
				ap.Enabled = false
			}
			return ap
		},
	}
	desired := make(chan Decision, 1)
	desired <- Decision{DecidedAt: time.Now()}
	err := run(context.Background(), f, desired, nil, time.Second)
	if err == nil || !strings.Contains(err.Error(), "unsafe status") {
		t.Fatalf("expected immediate fault, got %v", err)
	}
	if f.statusReads != 3 || len(f.commands) != 2 {
		t.Fatalf("expected stop at first fault, got %d status reads and %d commands", f.statusReads, len(f.commands))
	}
}

func TestRunWaitsForIndependentAppliedAcknowledgement(t *testing.T) {
	f := &fakeLink{}
	f.applied = func(c Command, ap Applied) Applied {
		if c.Enable && f.statusReads == 3 {
			ap.AppliedSequence--
			ap.Enabled = false
			ap.Outputs = Outputs{}
		}
		return ap
	}
	desired := make(chan Decision, 1)
	desired <- Decision{DecidedAt: time.Now(), Outputs: Outputs{F1Cooling: true}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	observations := make(chan Observation, 2)
	done := make(chan error, 1)
	go func() { done <- run(ctx, f, desired, observations, time.Second) }()
	for i := 0; i < 2; i++ {
		select {
		case obs := <-observations:
			if obs.Applied.AppliedSequence != uint32(i+1) || !obs.Converged {
				t.Errorf("unexpected acknowledgement: %+v", obs)
			}
		case <-ctx.Done():
			t.Fatal("expected acknowledgement")
		}
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if f.statusReads != 4 || len(f.commands) != 2 {
		t.Fatalf("expected a second acknowledgement poll without a new write, got %d reads and %d commands", f.statusReads, len(f.commands))
	}
}

func TestRunRejectsHazardousAppliedImageWhileAcknowledgementIsPending(t *testing.T) {
	for name, outputs := range map[string]Outputs{
		"F1 heating and cooling": {F1Heating: true, F1Cooling: true},
		"F2 heating and cooling": {F2Heating: true, F2Cooling: true},
		"pump without valve":     {Pump: true},
	} {
		t.Run(name, func(t *testing.T) {
			f := &fakeLink{applied: func(c Command, ap Applied) Applied {
				if c.Enable {
					ap.AppliedSequence--
					ap.Enabled = false
					ap.Outputs = outputs
				}
				return ap
			}}
			desired := make(chan Decision, 1)
			desired <- Decision{DecidedAt: time.Now()}
			err := run(context.Background(), f, desired, nil, time.Second)
			if err == nil || !strings.Contains(err.Error(), "violates output interlocks") {
				t.Fatalf("expected immediate interlock rejection, got %v", err)
			}
			if f.statusReads != 3 || len(f.commands) != 2 {
				t.Fatalf("expected stop at first hazardous image, got %d status reads and %d commands", f.statusReads, len(f.commands))
			}
		})
	}
}
func TestRunDisabledHandoverThenHeartbeats(t *testing.T) {
	f := &fakeLink{}
	desired := make(chan Decision, 1)
	desired <- Decision{Outputs: Outputs{F1Cooling: true}, DecidedAt: time.Now()}
	ctx, cancel := context.WithCancel(context.Background())
	observations := make(chan Observation, 5)
	done := make(chan error, 1)
	go func() { done <- run(ctx, f, desired, observations, 10*time.Second) }()
	for i := 0; i < 3; i++ {
		select {
		case <-observations:
		case <-time.After(2 * time.Second):
			t.Fatal("observation timeout")
		}
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.commands) != 3 || f.commands[0].Enable || !f.commands[1].Enable || !f.commands[1].Outputs.F1Cooling {
		t.Fatalf("commands %+v", f.commands)
	}
	if f.commands[0].Session == 0 || f.commands[0].Session != f.commands[2].Session || f.commands[0].Sequence != 1 || f.commands[2].Sequence != 3 {
		t.Fatalf("session/sequence %+v", f.commands)
	}
}
func TestRunStopsWritingOnError(t *testing.T) {
	f := &fakeLink{failAt: 2}
	desired := make(chan Decision, 1)
	desired <- Decision{DecidedAt: time.Now()}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := run(ctx, f, desired, nil, 10*time.Second); err == nil {
		t.Fatal("expected error")
	}
	if len(f.commands) != 2 {
		t.Fatalf("expected no retry: %d", len(f.commands))
	}
}
func TestRunAllowsTransientAppliedMismatchWhileSendingHeartbeats(t *testing.T) {
	f := &fakeLink{forceMismatch: true}
	desired := make(chan Decision, 1)
	desired <- Decision{DecidedAt: time.Now()}
	ctx, cancel := context.WithCancel(context.Background())
	observations := make(chan Observation, 5)
	done := make(chan error, 1)
	go func() { done <- run(ctx, f, desired, observations, 10*time.Second) }()
	for i := 0; i < 3; i++ {
		select {
		case o := <-observations:
			if i > 0 && o.Converged {
				t.Error("expected mismatch")
			}
		case <-time.After(2 * time.Second):
			t.Fatal("observation timeout")
		}
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if len(f.commands) != 3 {
		t.Fatalf("heartbeat stopped: %d", len(f.commands))
	}
}

func TestRunStopsAfterPersistentAppliedMismatch(t *testing.T) {
	f := &fakeLink{forceMismatch: true}
	desired := make(chan Decision, 1)
	desired <- Decision{DecidedAt: time.Now()}
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
	defer cancel()
	err := run(ctx, f, desired, nil, 10*time.Second)
	if err == nil || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected convergence failure, got %v", err)
	}
	f.mu.Lock()
	count := len(f.commands)
	f.mu.Unlock()
	if count < 10 {
		t.Fatalf("expected heartbeats during convergence window, got %d", count)
	}
}

func TestRunStopsWhenChangingDemandNeverConverges(t *testing.T) {
	f := &fakeLink{forceMismatch: true}
	desired := make(chan Decision, 1)
	desired <- Decision{DecidedAt: time.Now()}
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
	defer cancel()
	go func() {
		ticker := time.NewTicker(75 * time.Millisecond)
		defer ticker.Stop()
		flip := false
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				flip = !flip
				next := Outputs{}
				if flip {
					next.F2Cooling = true
				}
				select {
				case desired <- Decision{Outputs: next, DecidedAt: time.Now()}:
				default:
				}
			}
		}
	}()
	err := run(ctx, f, desired, nil, 10*time.Second)
	if err == nil || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected convergence failure despite changing demand, got %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	seenOff, seenF2 := false, false
	for _, command := range f.commands {
		if command.Enable {
			if command.Outputs.F2Cooling {
				seenF2 = true
			} else {
				seenOff = true
			}
		}
	}
	if !seenOff || !seenF2 {
		t.Fatal("expected both output demands to be transmitted")
	}
}

func TestNoInitialDecisionNeverWrites(t *testing.T) {
	f := &fakeLink{}
	desired := make(chan Decision)
	err := run(context.Background(), f, desired, nil, 100*time.Millisecond)
	if err == nil || len(f.commands) != 0 {
		t.Fatalf("expected timeout without writes: %v, %d", err, len(f.commands))
	}
}
func TestStoppedProducerExpiresAndStopsHeartbeats(t *testing.T) {
	f := &fakeLink{}
	desired := make(chan Decision, 1)
	desired <- Decision{Outputs: Outputs{F1Cooling: true}, DecidedAt: time.Now()}
	err := run(context.Background(), f, desired, nil, 550*time.Millisecond)
	if err == nil {
		t.Fatal("expected expired decision")
	}
	f.mu.Lock()
	n := len(f.commands)
	enabled := false
	for _, c := range f.commands {
		enabled = enabled || c.Enable
	}
	f.mu.Unlock()
	if !enabled {
		t.Fatal("test never reached enabled state")
	}
	time.Sleep(300 * time.Millisecond)
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.commands) != n {
		t.Fatal("heartbeat continued after decision expired")
	}
}
func TestStaleQueuedDecisionCannotRenew(t *testing.T) {
	f := &fakeLink{}
	desired := make(chan Decision, 2)
	old := time.Now().Add(-time.Second)
	desired <- Decision{DecidedAt: old}
	err := run(context.Background(), f, desired, nil, 300*time.Millisecond)
	if err == nil || len(f.commands) != 0 {
		t.Fatalf("stale decision admitted: %v", err)
	}
}
func TestUnchangedFreshDecisionsRenew(t *testing.T) {
	f := &fakeLink{}
	desired := make(chan Decision, 1)
	desired <- Decision{DecidedAt: time.Now()}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- run(ctx, f, desired, nil, 500*time.Millisecond) }()
	for i := 0; i < 4; i++ {
		time.Sleep(150 * time.Millisecond)
		desired <- Decision{DecidedAt: time.Now()}
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("fresh producer failed: %v", err)
	}
}

func TestLateRenewalCannotReviveExpiredDecision(t *testing.T) {
	now := time.Now()
	if checkPreviousDecisionDeadline(now, now) == nil {
		t.Fatal("deadline at now must fail")
	}
	if checkPreviousDecisionDeadline(now, now.Add(-time.Nanosecond)) == nil {
		t.Fatal("expired deadline must fail")
	}
	if err := checkPreviousDecisionDeadline(now, now.Add(time.Nanosecond)); err != nil {
		t.Fatal(err)
	}
}
