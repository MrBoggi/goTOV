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
type UAFence interface{ FenceWrites(context.Context) error }

// Supervisor is a one-shot handover guard. It does not release the UA fence
// on success, failure or cancellation, and cannot establish cross-process or
// physical PLC ownership. It must not be exposed as an activation API until the
// commissioning and restart gates described in README are implemented.
type Supervisor struct {
	mu      sync.Mutex
	started bool
}

func (s *Supervisor) Run(ctx context.Context, ua UAFence, link Link, desired <-chan Decision, observations chan<- Observation, maxDecisionAge time.Duration) error {
	if ua == nil || link == nil || desired == nil || maxDecisionAge <= 0 {
		return fmt.Errorf("UA fence, link, decisions and positive maximum age are required")
	}
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return fmt.Errorf("supervisor already used")
	}
	s.started = true
	s.mu.Unlock()
	fenceCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := ua.FenceWrites(fenceCtx); err != nil {
		return errors.Join(fmt.Errorf("fence UA writes: %w", err), link.Close())
	}
	return Run(ctx, link, desired, observations, maxDecisionAge)
}
