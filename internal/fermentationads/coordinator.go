package fermentationads

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"time"
)

// Link is the complete-image ADS transport. A single owner must serialize all
// access; in particular the legacy UA writer must be disabled before Run starts.
type Link interface {
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

// Run explicitly requests ADS ownership and keeps the watchdog alive until
// cancellation or an error. It never requests UA ownership or retries after an
// error. A new invocation with a new session is required for reactivation.
// The caller must fence every UA writer before starting this method and keep
// it fenced until the PLC is independently known to be in a safe handover.
// desired must deliver complete, interlocked output images; the last value is
// retained. The initial command is a disabled ADS handover with all outputs off.
func Run(ctx context.Context, link Link, desired <-chan Outputs, observe chan<- Observation) error {
	if link == nil || desired == nil {
		return fmt.Errorf("link and desired channel are required")
	}
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
		transactionCtx, cancel := context.WithTimeout(ctx, ackTimeout)
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
	output := Outputs{}
	var mismatchSince time.Time
	ticker := time.NewTicker(heartbeat)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case next, ok := <-desired:
			if !ok {
				return fmt.Errorf("desired channel closed")
			}
			if next != output {
				output = next
			}
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
