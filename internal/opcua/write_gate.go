package opcua

import (
	"context"
	"fmt"
	"sync"
)

// Only these exact canonical NodeIDs are known non-fermentation outputs in the
// checked-in UA PLC source. Unknown identities and aliases fail closed when
// ADS ownership is requested. Runtime mapping must be verified separately.
var independentBreweryOutputs = map[string]struct{}{
	"ns=4;s=MAIN.fbUA.pumpeHLT": {}, "ns=4;s=MAIN.fbUA.pumpeWort": {},
	"ns=4;s=MAIN.fbUA.V1_VannInn": {}, "ns=4;s=MAIN.fbUA.V2_HLT_TilMLT": {},
	"ns=4;s=MAIN.fbUA.V3_MLT_TilBK": {}, "ns=4;s=MAIN.fbUA.V4_BK_TilHX": {},
	"ns=4;s=MAIN.fbUA.V5_HX_TilDrain": {}, "ns=4;s=MAIN.fbUA.V6_HX_TilFerment": {},
	"ns=4;s=MAIN.fbUA.V7_SpargeWater": {}, "ns=4;s=MAIN.fbUA.V8_ChillerIn": {},
	"ns=4;s=MAIN.fbUA.V9_ChillerOut": {}, "ns=4;s=MAIN.fbUA.hltHeaterPower": {},
	"ns=4;s=MAIN.fbUA.bkHeaterPower": {}, "ns=4;s=MAIN.fbUA.hxValvePosition": {},
}

// The zero value allows all UA writes. Fence is one-way, including after a
// timeout. A risky write error makes the outcome uncertain and blocks handover.
type writeGate struct {
	mu        sync.Mutex
	fencing   bool
	uncertain bool
	active    int
	drained   chan struct{}
}

func (g *writeGate) write(node string, fn func() error) error {
	_, independent := independentBreweryOutputs[node]
	g.mu.Lock()
	if g.fencing && !independent {
		g.mu.Unlock()
		return fmt.Errorf("OPC UA node is fenced or unclassified")
	}
	if !independent {
		g.active++
	}
	g.mu.Unlock()
	err := fn()
	if !independent {
		g.mu.Lock()
		if err != nil {
			g.uncertain = true
		}
		g.active--
		if g.active == 0 && g.drained != nil {
			close(g.drained)
			g.drained = nil
		}
		g.mu.Unlock()
	}
	return err
}
func (g *writeGate) fence(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	g.mu.Lock()
	g.fencing = true
	if g.active == 0 {
		defer g.mu.Unlock()
		if g.uncertain {
			return fmt.Errorf("cannot fence UA safely: prior write outcome is uncertain")
		}
		return ctx.Err()
	}
	if g.drained == nil {
		g.drained = make(chan struct{})
	}
	drained := g.drained
	g.mu.Unlock()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-drained:
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if g.uncertain {
		return fmt.Errorf("cannot fence UA safely: prior write outcome is uncertain")
	}
	return nil
}

// FenceWrites blocks fermentation and unclassified writes immediately, then
// waits for all possibly relevant in-flight writes within ctx. Cancellation
// never releases the fence, and no error grants permission for ADS to start.
// Other clients, independent output mapping and PLC SAFE remain external gates.
func (c *Client) FenceWrites(ctx context.Context) error {
	if c == nil {
		return fmt.Errorf("nil UA client")
	}
	return c.writeGate.fence(ctx)
}
