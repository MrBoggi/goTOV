package brewhouse

type PIDController struct {
	P float64
	I float64
	D float64

	lastError float64
	integral  float64

	minOutput float64
	maxOutput float64
}

func NewPIDController(p, i, d float64, min, max float64) *PIDController {
	return &PIDController{
		P:         p,
		I:         i,
		D:         d,
		minOutput: min,
		maxOutput: max,
	}
}

func (c *PIDController) Calculate(setpoint, processValue, dt float64) float64 {
	if dt <= 0 {
		return 0 // Avoid division by zero or negative time
	}

	error := setpoint - processValue

	// Proportional term
	pTerm := c.P * error

	// Integral term with anti-windup (clamping)
	c.integral += error * dt
	iTerm := c.I * c.integral

	// Derivative term
	derivative := (error - c.lastError) / dt
	dTerm := c.D * derivative

	output := pTerm + iTerm + dTerm

	// Update last error
	c.lastError = error

	// Clamp output and handle anti-windup
	if output > c.maxOutput {
		// If output is capped, we should also cap the integral to prevent windup
		// Simple approach: if output is already at max, don't increase integral further if error is positive
		if error > 0 {
			c.integral -= error * dt
		}
		return c.maxOutput
	}
	if output < c.minOutput {
		if error < 0 {
			c.integral -= error * dt
		}
		return c.minOutput
	}

	return output
}

func (c *PIDController) Reset() {
	c.lastError = 0
	c.integral = 0
}
