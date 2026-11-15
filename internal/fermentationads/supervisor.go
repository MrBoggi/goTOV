package fermentationads

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// UAFence must fence the exact UA client used by every in-process write path.
// It must wait for all in-flight writes and fail if their outcomes are unknown.
type UAFence interface {
	BeginFence(context.Context) (uint64, error)
	AbortFence(context.Context, uint64) error
	MarkADSStarted(uint64) error
}

// Supervisor is a one-shot handover guard. It may abort before reserving ADS
// start only after UA writes drain with known success; it cannot establish cross-process or
// physical PLC ownership. It must not be exposed as an activation API until the
// commissioning and restart gates described in README are implemented.
type Supervisor struct {
	mu      sync.Mutex
	started bool
}

func (s *Supervisor) Run(ctx context.Context, ua UAFence, openLink func(context.Context) (Link, error), desired <-chan Decision, observations chan<- Observation, maxDecisionAge time.Duration) error {
	if ua == nil || openLink == nil || desired == nil || maxDecisionAge <= 0 {
		return fmt.Errorf("UA fence, link factory, decisions and positive maximum age are required")
	}
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return fmt.Errorf("supervisor already used")
	}
	s.started = true
	s.mu.Unlock()
	fenceCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	token, err := ua.BeginFence(fenceCtx)
	cancel()
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		if token == 0 {
			return fmt.Errorf("UA fence not acquired: %w", err)
		}
		rollbackCtx, rollbackCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer rollbackCancel()
		if rollbackErr := ua.AbortFence(rollbackCtx, token); rollbackErr != nil {
			return errors.Join(fmt.Errorf("handover abandoned; UA state UNKNOWN: %w", err), rollbackErr)
		}
		return fmt.Errorf("handover abandoned before ADS start; UA fence released: %w", err)
	}
	// The token is made irrevocable before Run can enqueue an ADS frame. Even if
	// Run returns without sending, that conservative state requires reconciliation.
	if err = ua.MarkADSStarted(token); err != nil {
		return fmt.Errorf("reserve ADS start; UA state UNKNOWN: %w", err)
	}
	// The factory must create a fresh session and must not perform work until
	// called here. It cannot safely be called before the irreversible reservation.
	link, err := openLink(ctx)
	if err != nil {
		return fmt.Errorf("open ADS link; UA state UNKNOWN: %w", err)
	}
	if link == nil {
		return fmt.Errorf("open ADS link returned nil; UA state UNKNOWN")
	}
	return run(ctx, link, desired, observations, maxDecisionAge)
}
