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

// The zero value allows all UA writes. A risky write error makes the outcome
// uncertain and blocks both ADS handover and automatic fence rollback.
type writeGate struct {
	mu         sync.Mutex
	fencing    bool
	uncertain  bool
	active     int
	drained    chan struct{}
	generation uint64
	token      uint64
	adsStarted bool
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

// begin claims a unique handover transaction before waiting. Even if waiting
// times out the returned token owns the fence, until a safe abort or ADS start.
func (g *writeGate) begin(ctx context.Context) (uint64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	g.mu.Lock()
	if g.fencing {
		g.mu.Unlock()
		return 0, fmt.Errorf("UA handover already reserved")
	}
	if g.generation == ^uint64(0) {
		g.mu.Unlock()
		return 0, fmt.Errorf("UA handover generation exhausted")
	}
	g.generation++
	token := g.generation
	g.token = token
	g.fencing = true
	g.adsStarted = false
	g.mu.Unlock()
	return token, g.waitDrained(ctx, token)
}
func (g *writeGate) waitDrained(ctx context.Context, token uint64) error {
	g.mu.Lock()
	if !g.fencing || token == 0 || token != g.token {
		g.mu.Unlock()
		return fmt.Errorf("invalid UA handover token")
	}
	if g.active == 0 {
		defer g.mu.Unlock()
		if err := ctx.Err(); err != nil {
			return err
		}
		if g.uncertain {
			return fmt.Errorf("UA write outcome unknown")
		}
		return nil
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
	if !g.fencing || token != g.token {
		return fmt.Errorf("UA handover no longer reserved")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if g.uncertain {
		return fmt.Errorf("UA write outcome unknown")
	}
	return nil
}

// abort only succeeds before ADS start is reserved and after all relevant UA
// writes returned known success. An error leaves the fence closed.
func (g *writeGate) abort(ctx context.Context, token uint64) error {
	if err := g.waitDrained(ctx, token); err != nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if !g.fencing || token != g.token || g.adsStarted || g.active != 0 || g.uncertain {
		return fmt.Errorf("UA handover cannot be safely aborted")
	}
	g.fencing = false
	g.token = 0
	return nil
}

// markADSStarted irreversibly gives up the right to auto-abort before any ADS
// call. A racing abort and mark cannot both succeed.
func (g *writeGate) markADSStarted(token uint64) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.fencing || token == 0 || token != g.token || g.adsStarted || g.active != 0 || g.uncertain {
		return fmt.Errorf("UA handover is not ready for ADS")
	}
	g.adsStarted = true
	return nil
}
func (c *Client) BeginFence(ctx context.Context) (uint64, error) {
	if c == nil {
		return 0, fmt.Errorf("nil UA client")
	}
	return c.writeGate.begin(ctx)
}
func (c *Client) AbortFence(ctx context.Context, token uint64) error {
	if c == nil {
		return fmt.Errorf("nil UA client")
	}
	return c.writeGate.abort(ctx, token)
}
func (c *Client) MarkADSStarted(token uint64) error {
	if c == nil {
		return fmt.Errorf("nil UA client")
	}
	return c.writeGate.markADSStarted(token)
}
