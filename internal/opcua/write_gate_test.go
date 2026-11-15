package opcua

import (
	"errors"
	"testing"
	"time"
)

func TestFenceWaitsForInflightAndRejectsAllLaterWrites(t *testing.T) {
	var g writeGate
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- g.write(func() error { close(started); <-release; return nil }) }()
	<-started
	fenced := make(chan error, 1)
	go func() { fenced <- g.fence() }()
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
	called := false
	if g.write(func() error { called = true; return nil }) == nil || called {
		t.Fatal("post-fence write ran")
	}
}
func TestUncertainWritePreventsHandover(t *testing.T) {
	var g writeGate
	if err := g.write(func() error { return errors.New("timeout") }); err == nil {
		t.Fatal("expected error")
	}
	if err := g.fence(); err == nil {
		t.Fatal("fence accepted uncertain UA write")
	}
}
func TestDefaultGateAllowsLegacyWrites(t *testing.T) {
	var g writeGate
	if err := g.write(func() error { return nil }); err != nil {
		t.Fatal(err)
	}
}
