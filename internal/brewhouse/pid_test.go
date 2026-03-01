package brewhouse

import (
	"testing"
)

func TestPIDController(t *testing.T) {
	t.Run("Proportional only", func(t *testing.T) {
		pid := NewPIDController(1.0, 0.0, 0.0, 0, 100)
		// Setpoint 50, PV 40, Error 10 -> Command 10
		output := pid.Calculate(50, 40, 1.0)
		if output != 10.0 {
			t.Errorf("Expected 10.0, got %f", output)
		}
	})

	t.Run("Integral component", func(t *testing.T) {
		pid := NewPIDController(0.0, 1.0, 0.0, 0, 100)
		// Setpoint 50, PV 40, Error 10, dt 1.0 -> Integral 10 -> Command 10
		output := pid.Calculate(50, 40, 1.0)
		if output != 10.0 {
			t.Errorf("Expected 10.0, got %f", output)
		}
		// Second call: Error 10, dt 1.0 -> Integral 20 -> Command 20
		output = pid.Calculate(50, 40, 1.0)
		if output != 20.0 {
			t.Errorf("Expected 20.0, got %f", output)
		}
	})

	t.Run("Derivative component", func(t *testing.T) {
		pid := NewPIDController(0.0, 0.0, 1.0, 0, 100)
		// First call: Error 10, dt 1.0 -> Derivative 10 (since lastError is 0) -> Command 10
		output := pid.Calculate(50, 40, 1.0)
		if output != 10.0 {
			t.Errorf("Expected 10.0, got %f", output)
		}
		// Second call: PV still 40, Error 10, dt 1.0 -> Derivative (10-10)/1 = 0 -> Command 0
		output = pid.Calculate(50, 40, 1.0)
		if output != 0.0 {
			t.Errorf("Expected 0.0, got %f", output)
		}
		// Third call: PV 30, Error 20, dt 1.0 -> Derivative (20-10)/1 = 10 -> Command 10
		output = pid.Calculate(50, 30, 1.0)
		if output != 10.0 {
			t.Errorf("Expected 10.0, got %f", output)
		}
	})

	t.Run("Clamping and Anti-Windup", func(t *testing.T) {
		pid := NewPIDController(10.0, 1.0, 0.0, 0, 100)
		// Setpoint 100, PV 0, Error 100
		// P Term = 1000, I Term = 100
		// Total 1100 -> Clamped to 100
		output := pid.Calculate(100, 0, 1.0)
		if output != 100.0 {
			t.Errorf("Expected 100.0, got %f", output)
		}
		// Check that integral didn't keep growing (basic anti-windup check)
		// Actually my logic stops integral growth ONLY if if output > max AND error > 0.
		// In first call: output 1100 > 100, error 100 > 0. Integral was 100, then subtracted back 100 -> 0.
		if pid.integral != 0.0 {
			t.Errorf("Expected integral to be 0 due to anti-windup, got %f", pid.integral)
		}
	})

	t.Run("Reset", func(t *testing.T) {
		pid := NewPIDController(1.0, 1.0, 1.0, 0, 100)
		pid.Calculate(50, 40, 1.0)
		pid.Reset()
		if pid.integral != 0 || pid.lastError != 0 {
			t.Error("Reset failed")
		}
	})
}
