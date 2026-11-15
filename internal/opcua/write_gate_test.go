package opcua

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gopcua/opcua/ua"
)

const risky = "ns=4;s=MAIN.fbUA.fermenter1Kjoleventil"
const independent = "ns=4;s=MAIN.fbUA.pumpeHLT"

func TestFenceWaitsForInflightAndRejectsAllLaterRiskyWrites(t *testing.T) {
	var g writeGate
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- g.write(risky, func() error { close(started); <-release; return nil }) }()
	<-started
	fenced := make(chan error, 1)
	go func() { _, err := g.begin(context.Background()); fenced <- err }()
	select {
	case err := <-fenced:
		t.Fatalf("fence returned before active write: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := <-fenced; err != nil {
		t.Fatal(err)
	}
	for _, node := range []string{risky, "ns=4;s=MAIN.fbUA.fermenter2Kjoleventil", "ns=4;s=MAIN.fbUA.fermenter1Varmekappe", "ns=4;s=MAIN.fbUA.fermenter2Varmekappe", "ns=4;s=MAIN.fbUA.glykolkjolerPumpe", "ns=4;i=12345", "ns=4;s=some.alias"} {
		called := false
		if g.write(node, func() error { called = true; return nil }) == nil || called {
			t.Fatalf("fenced write ran: %s", node)
		}
	}
	if err := g.write(independent, func() error { return nil }); err != nil {
		t.Fatalf("independent output rejected: %v", err)
	}
}
func TestFenceTimeoutLeavesRiskyWritesBlocked(t *testing.T) {
	var g writeGate
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- g.write(risky, func() error { close(started); <-release; return nil }) }()
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := g.begin(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("expected deadline")
	}
	if err := g.write(risky, func() error { return nil }); err == nil {
		t.Fatal("risky write escaped after timeout")
	}
	if err := g.write(independent, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
func TestUncertainWritePreventsHandover(t *testing.T) {
	var g writeGate
	if err := g.write(risky, func() error { return errors.New("timeout") }); err == nil {
		t.Fatal("expected error")
	}
	if _, err := g.begin(context.Background()); err == nil {
		t.Fatal("fence accepted uncertain UA write")
	}
}
func TestDefaultGateAllowsLegacyWrites(t *testing.T) {
	var g writeGate
	if err := g.write("ns=4;i=123", func() error { return nil }); err != nil {
		t.Fatal(err)
	}
}
func TestIndependentNodeIDsAreCanonical(t *testing.T) {
	for node := range independentBreweryOutputs {
		id, err := ua.ParseNodeID(node)
		if err != nil || id.String() != node {
			t.Fatalf("bad canonical node %s: %v", node, err)
		}
	}
}

func TestAbortAfterTimedOutFenceWaitsForKnownSuccessfulUAWrite(t *testing.T) {
	var g writeGate
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan error, 1)
	go func() { finished <- g.write(risky, func() error { close(started); <-release; return nil }) }()
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	token, err := g.begin(ctx)
	if !errors.Is(err, context.DeadlineExceeded) || token == 0 {
		t.Fatalf("expected reserved timeout: %d %v", token, err)
	}
	rollback := make(chan error, 1)
	go func() { rollback <- g.abort(context.Background(), token) }()
	select {
	case err := <-rollback:
		t.Fatalf("rollback before UA outcome: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	if err := g.write(independent, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	if err := <-rollback; err != nil {
		t.Fatal(err)
	}
	if err := g.write(risky, func() error { return nil }); err != nil {
		t.Fatal("safe pre-ADS rollback did not restore UA")
	}
}
func TestAbortRejectsUnknownOrADSReserved(t *testing.T) {
	var g writeGate
	if err := g.write(risky, func() error { return errors.New("unknown") }); err == nil {
		t.Fatal("missing failure")
	}
	token, _ := g.begin(context.Background())
	if err := g.abort(context.Background(), token); err == nil {
		t.Fatal("aborted despite unknown delivery")
	}
	if err := g.write(risky, func() error { return nil }); err == nil {
		t.Fatal("unknown UA state reopened")
	}
	var h writeGate
	token, err := h.begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err = h.markADSStarted(token); err != nil {
		t.Fatal(err)
	}
	if err = h.abort(context.Background(), token); err == nil {
		t.Fatal("aborted after ADS start reserved")
	}
}
func TestAbortAndADSReservationCannotBothWin(t *testing.T) {
	for i := 0; i < 100; i++ {
		var g writeGate
		token, err := g.begin(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		ready := make(chan struct{})
		results := make(chan error, 2)
		go func() { <-ready; results <- g.abort(context.Background(), token) }()
		go func() { <-ready; results <- g.markADSStarted(token) }()
		close(ready)
		a, b := <-results, <-results
		if (a == nil) == (b == nil) {
			t.Fatalf("both or neither succeeded: %v %v", a, b)
		}
	}
}

func TestOldTransactionCannotAbortNewFence(t *testing.T) {
	var g writeGate
	first, err := g.begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err = g.abort(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	second, err := g.begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if second == first {
		t.Fatal("reused token")
	}
	if err = g.abort(context.Background(), first); err == nil {
		t.Fatal("old token released new fence")
	}
	if err = g.write(risky, func() error { return nil }); err == nil {
		t.Fatal("new fence unexpectedly open")
	}
}
