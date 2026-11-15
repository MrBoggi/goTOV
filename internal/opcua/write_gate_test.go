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
	go func() { fenced <- g.fence(context.Background()) }()
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
	if !errors.Is(g.fence(ctx), context.DeadlineExceeded) {
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
	if err := g.fence(context.Background()); err == nil {
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
