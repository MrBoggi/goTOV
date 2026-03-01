package brewhouse

import (
	"context"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

type mockStore struct {
	state          *BrewhouseState
	brewingSession string
}

func (m *mockStore) GetState() (*BrewhouseState, error) { return m.state, nil }
func (m *mockStore) SaveState(state *BrewhouseState) error {
	m.state = state
	return nil
}
func (m *mockStore) GetBrewingSession() (string, error) { return m.brewingSession, nil }
func (m *mockStore) SaveBrewingSession(data string) error {
	m.brewingSession = data
	return nil
}
func (m *mockStore) LogBrewingSession(data string) error { return nil }
func (m *mockStore) Close() error                        { return nil }

func TestHeaterInterlock(t *testing.T) {
	store := &mockStore{state: InitialState()}
	engine := NewEngine(store, nil, zerolog.Nop())
	engine.state = InitialState()
	engine.lastEvaluation = time.Now().Add(-1 * time.Second)

	// Set heater to manual and command 50%
	engine.state.Heater.Mode = ModeManual
	engine.state.Heater.Command = 50.0

	// Case 1: Level is safe (25L)
	engine.state.Sensors["bkLevel"] = 25.0
	engine.evaluateAndWrite(context.Background())
	if engine.state.Heater.Command != 50.0 {
		t.Errorf("Expected heater command to remain 50.0, got %f", engine.state.Heater.Command)
	}

	// Case 2: Level is unsafe (15L)
	engine.state.Sensors["bkLevel"] = 15.0
	engine.evaluateAndWrite(context.Background())
	if engine.state.Heater.Command != 0.0 {
		t.Errorf("Expected heater command to be forced to 0.0, got %f", engine.state.Heater.Command)
	}
}

func TestAutoManualTransition(t *testing.T) {
	store := &mockStore{state: InitialState()}
	engine := NewEngine(store, nil, zerolog.Nop())

	// Initial state with a setpoint in Auto
	engine.state = InitialState()
	engine.state.Heater.Mode = ModeAuto
	engine.state.Heater.Setpoint = 65.0

	// User sends update switching to Manual
	newState := InitialState()
	newState.Heater.Mode = ModeManual
	newState.Heater.Setpoint = 0 // User didn't specify a new setpoint

	err := engine.UpdateState(newState)
	if err != nil {
		t.Fatalf("UpdateState failed: %v", err)
	}

	if engine.state.Heater.Setpoint != 65.0 {
		t.Errorf("Expected setpoint 65.0 to be retained, got %f", engine.state.Heater.Setpoint)
	}
}

func TestLevelCalculation(t *testing.T) {
	store := &mockStore{state: InitialState()}
	engine := NewEngine(store, nil, zerolog.Nop())
	engine.state = InitialState()
	engine.state.Sensors["mltLevel"] = 10.0
	engine.state.Sensors["flowMLT"] = 60.0 // 60 L/min = 1 L/sec

	// Simulate 2 seconds passing
	engine.lastEvaluation = time.Now().Add(-2 * time.Second)

	engine.evaluateAndWrite(context.Background())

	// We expect roughly 10 + (60/60 * 2) = 12.0
	// Account for minor timing jitter if necessary, but evaluateAndWrite uses time.Now()
	// In test we can't easily clock it precisely without mocking time, but let's check range
	level := engine.state.Sensors["mltLevel"]
	if level < 11.9 || level > 12.1 {
		t.Errorf("Expected level around 12.0, got %f", level)
	}
}
func TestHeaterPID(t *testing.T) {
	store := &mockStore{state: InitialState()}
	engine := NewEngine(store, nil, zerolog.Nop())
	engine.state = InitialState()
	engine.lastEvaluation = time.Now().Add(-1 * time.Second)

	// Set heater to Auto and PID mode
	engine.state.Heater.Mode = ModeAuto
	engine.state.Heater.IsPID = true
	engine.state.Heater.Setpoint = 50.0
	engine.state.Heater.PID = PIDConfig{P: 1.0, I: 0.1, D: 0.01}
	engine.state.Sensors["bkLevel"] = 25.0 // Safe level
	engine.state.Sensors["bkTemp"] = 40.0  // 10 degrees error

	// First evaluation
	engine.evaluateAndWrite(context.Background())

	// Command should be > 0 (Proportional alone is 10.0)
	if engine.state.Heater.Command <= 0 {
		t.Errorf("Expected heater command to be > 0, got %f", engine.state.Heater.Command)
	}

	cmd1 := engine.state.Heater.Command

	// Wait a bit and evaluate again
	engine.lastEvaluation = time.Now().Add(-1 * time.Second)
	engine.evaluateAndWrite(context.Background())

	// Command should change (Integral should increase)
	cmd2 := engine.state.Heater.Command
	if cmd2 <= cmd1 {
		t.Errorf("Expected heater command to increase due to integral term, got %f (prev %f)", cmd2, cmd1)
	}

	// Test disabling PID
	engine.state.Heater.IsPID = false
	engine.evaluateAndWrite(context.Background())
	// Should fallback to Bang-Bang: 40 < 50 -> 100%
	if engine.state.Heater.Command != 100.0 {
		t.Errorf("Expected bang-bang 100%%, got %f", engine.state.Heater.Command)
	}
}
