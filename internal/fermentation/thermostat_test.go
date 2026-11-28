package fermentation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/MrBoggi/goTOV/internal/fermentationads"
	"github.com/MrBoggi/goTOV/internal/opcua"
)

func TestThermostatDemand(t *testing.T) {
	const target float32 = 20
	for _, tc := range []struct {
		name               string
		current            float32
		cooling, heating   bool
		wantCool, wantHeat bool
	}{
		{"start cooling above band", 21, false, true, true, false},
		{"start heating below band", 19, true, false, false, true},
		{"upper edge does not start", target + temperatureHysteresis, false, false, false, false},
		{"lower edge does not start", target - temperatureHysteresis, false, false, false, false},
		{"keep cooling within band", 20.1, true, false, true, false},
		{"keep heating within band", 19.9, false, true, false, true},
		{"stop at setpoint", target, true, true, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cool, heat := thermostatDemand(tc.current, target, tc.cooling, tc.heating)
			if cool != tc.wantCool || heat != tc.wantHeat {
				t.Fatalf("got cooling=%t heating=%t", cool, heat)
			}
		})
	}
}

// thermostatLink mirrors an acknowledged command in memory.
type thermostatLink struct{ command fermentationads.Command }

func (*thermostatLink) Close() error { return nil }

func (l *thermostatLink) WriteCommand(_ context.Context, command fermentationads.Command) error {
	l.command = command
	return nil
}

func (l *thermostatLink) ReadStatus(context.Context) (fermentationads.Status, error) {
	c := l.command
	return fermentationads.Status{Session: c.Session, AppliedSequence: c.Sequence, Owner: c.Owner, Enabled: c.Enable, Valid: true, WatchdogHealthy: true}, nil
}

func (l *thermostatLink) ReadApplied(context.Context) (fermentationads.Applied, error) {
	c := l.command
	return fermentationads.Applied{Session: c.Session, AppliedSequence: c.Sequence, Enabled: c.Enable, Outputs: c.Outputs}, nil
}

func TestSyntheticThermostatDemandThroughADSSupervisor(t *testing.T) {
	cooling, heating := thermostatDemand(21, 20, false, false)
	decision := fermentationads.Decision{
		DecidedAt: time.Now(),
		Outputs:   fermentationads.Outputs{F1Cooling: cooling, F1Heating: heating},
	}
	desired := make(chan fermentationads.Decision, 1)
	desired <- decision

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	observations := make(chan fermentationads.Observation, 2)
	done := make(chan error, 1)
	var supervisor fermentationads.Supervisor
	go func() {
		// An unconnected UA client supplies only its in-memory write fence.
		done <- supervisor.Run(ctx, &opcua.Client{}, func(context.Context) (fermentationads.Link, error) {
			return &thermostatLink{}, nil
		}, desired, observations, 2*time.Second)
	}()

	for i, expected := range []fermentationads.Outputs{{}, {F1Cooling: true}} {
		select {
		case obs := <-observations:
			if obs.Applied.Outputs != expected || obs.Status.Owner != fermentationads.ADS || obs.Applied.Enabled != (i == 1) || !obs.Converged {
				t.Fatalf("unexpected acknowledgement: %+v", obs)
			}
		case err := <-done:
			t.Fatalf("supervisor stopped before acknowledgement: %v", err)
		case <-ctx.Done():
			t.Fatal("missing acknowledgement")
		}
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
