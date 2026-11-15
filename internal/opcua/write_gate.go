package opcua

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// writeGate fences every UA write, including arbitrary API nodes and callbacks
// scheduled before fencing. A network error leaves delivery uncertain; a fence
// cannot be established after any uncertain write without separate recovery.
// The zero value retains the existing UA behavior. There is deliberately no
// automatic release or recovery method.
type writeGate struct {
	mu        sync.RWMutex
	fenced    bool
	uncertain atomic.Bool
}

func (g *writeGate) write(fn func() error) error {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.fenced {
		return fmt.Errorf("OPC UA writes are fenced")
	}
	err := fn()
	if err != nil {
		g.uncertain.Store(true)
	}
	return err
}
func (g *writeGate) fence() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	// This lock waits for in-flight calls; failure cannot prove an old frame was
	// not applied after the call returned.
	if g.uncertain.Load() {
		return fmt.Errorf("cannot fence UA safely: prior write outcome is uncertain")
	}
	g.fenced = true
	return nil
}

// FenceWrites waits for in-flight writes then rejects all subsequent UA writes.
// It is one-way: a failed ADS session must not automatically re-enable UA.
// This process-local gate alone cannot fence other UA clients or cancel frames
// already accepted by the PLC; commissioning must establish exclusive control.
func (c *Client) FenceWrites() error {
	if c == nil {
		return fmt.Errorf("nil UA client")
	}
	return c.writeGate.fence()
}
