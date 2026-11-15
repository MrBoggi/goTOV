package fermentationads

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"time"
)

// Link is the complete-image ADS transport. A single owner must serialize all
// access; in particular the legacy UA writer must be disabled before Run starts.
type Link interface {
	Close() error
	WriteCommand(context.Context, Command) error
	ReadStatus(context.Context) (Status, error)
	ReadApplied(context.Context) (Applied, error)
}

// Observation is PLC-reported state; Applied is not physical feedback.
type Observation struct {
	Status    Status
	Applied   Applied
	Converged bool
}

// Decision records when the control producer computed an output image. Producers
// must use the local clock and renew unchanged decisions too. A buffered channel
// must not refresh stale work just because it was received later.
type Decision struct {
	Outputs   Outputs
	DecidedAt time.Time
}

// Run explicitly requests ADS ownership and keeps the watchdog alive until
// cancellation or an error. It never requests UA ownership or retries after an
// error. A new invocation with a new session is required for reactivation.
// The caller must fence every UA writer before starting this method and keep
// it fenced until the PLC is independently known to be in a safe handover.
// desired must supply newly computed decisions before maxDecisionAge expires,
// including repeated unchanged values. The first fresh decision is required
// before the disabled handover; stale, future or out-of-order decisions fail closed.
func Run(ctx context.Context, link Link, desired <-chan Decision, observe chan<- Observation, maxDecisionAge time.Duration) (result error) {
	if link == nil || desired == nil || maxDecisionAge <= 0 {
		return fmt.Errorf("link, desired channel and positive decision age are required")
	}
	defer func() { result = errors.Join(result, link.Close()) }()
	firstWait := time.NewTimer(maxDecisionAge)
	defer firstWait.Stop()
	var first Decision
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-firstWait.C:
		return fmt.Errorf("first control decision expired")
	case d, ok := <-desired:
		if !ok {
			return fmt.Errorf("desired channel closed")
		}
		first = d
	}
	validate := func(d Decision, previous time.Time) error {
		now := time.Now()
		if d.DecidedAt.IsZero() || d.DecidedAt.After(now) || !d.DecidedAt.After(previous) || !now.Before(d.DecidedAt.Add(maxDecisionAge)) {
			return fmt.Errorf("stale, future or out-of-order control decision")
		}
		return nil
	}
	if err := validate(first, time.Time{}); err != nil {
		return err
	}
	decisionDeadline := first.DecidedAt.Add(maxDecisionAge)
	lastDecision := first.DecidedAt
	output := first.Outputs
	var id [4]byte
	if _, err := rand.Read(id[:]); err != nil {
		return fmt.Errorf("session id: %w", err)
	}
	session := binary.LittleEndian.Uint32(id[:])
	if session == 0 {
		return fmt.Errorf("random session id was zero")
	}
	const heartbeat = 200 * time.Millisecond
	const ackTimeout = 400 * time.Millisecond
	// Controller's valve stop and settle stages each require one second; allow
	// bounded additional scan/transport time. This is not physical feedback.
	const convergenceTimeout = 5 * time.Second
	var seq uint32
	send := func(enable bool, output Outputs) (Observation, error) {
		if seq == ^uint32(0) {
			return Observation{}, fmt.Errorf("sequence exhausted; explicit new session required")
		}
		seq++
		cmd := Command{Session: session, Sequence: seq, Owner: ADS, Enable: enable, Outputs: output}
		if !time.Now().Before(decisionDeadline) {
			return Observation{}, fmt.Errorf("control decision expired")
		}
		transactionDeadline := time.Now().Add(ackTimeout)
		if decisionDeadline.Before(transactionDeadline) {
			transactionDeadline = decisionDeadline
		}
		transactionCtx, cancel := context.WithDeadline(ctx, transactionDeadline)
		defer cancel()
		if err := link.WriteCommand(transactionCtx, cmd); err != nil {
			return Observation{}, fmt.Errorf("write command: %w", err)
		}
		poll := time.NewTicker(20 * time.Millisecond)
		defer poll.Stop()
		for {
			st, err := link.ReadStatus(transactionCtx)
			if err != nil {
				return Observation{}, fmt.Errorf("read status: %w", err)
			}
			ap, err := link.ReadApplied(transactionCtx)
			if err != nil {
				return Observation{}, fmt.Errorf("read applied: %w", err)
			}
			if st.Session == session && st.AppliedSequence == seq && ap.Session == session && ap.AppliedSequence == seq {
				if st.Owner != ADS || !st.Valid || !st.WatchdogHealthy || st.Faults != 0 || st.Enabled != enable || ap.Enabled != enable {
					return Observation{}, fmt.Errorf("PLC rejected command or entered unsafe status")
				}
				return Observation{Status: st, Applied: ap, Converged: ap.Outputs == output}, nil
			}
			select {
			case <-transactionCtx.Done():
				return Observation{}, transactionCtx.Err()
			case <-poll.C:
			}
		}
	}
	// A disabled request is a distinct, confirmed transaction before enable.
	handover, err := send(false, Outputs{})
	if err != nil {
		return fmt.Errorf("disabled handover: %w", err)
	}
	if !handover.Converged {
		return fmt.Errorf("disabled handover has nonzero applied outputs")
	}
	if observe != nil {
		select {
		case observe <- handover:
		default:
		}
	}
	var mismatchSince time.Time
	ticker := time.NewTicker(heartbeat)
	defer ticker.Stop()
	expiry := time.NewTimer(time.Until(decisionDeadline))
	defer expiry.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-expiry.C:
			return fmt.Errorf("control decision expired")
		case next, ok := <-desired:
			if err := checkPreviousDecisionDeadline(time.Now(), decisionDeadline); err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("desired channel closed")
			}
			if err := validate(next, lastDecision); err != nil {
				return err
			}
			lastDecision = next.DecidedAt
			decisionDeadline = next.DecidedAt.Add(maxDecisionAge)
			output = next.Outputs
			if !expiry.Stop() {
				select {
				case <-expiry.C:
				default:
				}
			}
			expiry.Reset(time.Until(decisionDeadline))
		case <-ticker.C:
			obs, err := send(true, output)
			if err != nil {
				return err
			}
			if obs.Converged {
				mismatchSince = time.Time{}
			} else if mismatchSince.IsZero() {
				mismatchSince = time.Now()
			} else if time.Since(mismatchSince) > convergenceTimeout {
				return fmt.Errorf("applied outputs failed to converge")
			}
			if observe != nil {
				select {
				case observe <- obs:
				default:
				}
			}
		}
	}
}

// A late renewal cannot revive an expired decision even if both select cases
// are ready; select is allowed to choose the channel rather than the timer.
func checkPreviousDecisionDeadline(now, deadline time.Time) error {
	if !now.Before(deadline) {
		return fmt.Errorf("control decision expired before renewal")
	}
	return nil
}
