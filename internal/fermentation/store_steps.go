package fermentation

import (
	"fmt"
	"time"
)

// Returns the active fermentation step for a given time.
func (s *SQLiteStore) GetActiveStep(state FermentationState, now time.Time) (*FermentationStep, int, error) {

	// Load all steps for this plan
	var steps []FermentationStep
	err := s.DB.Select(&steps, `
        SELECT step_number, temperature, duration_hours, description, type
        FROM fermentation_steps
        WHERE plan_id = ?
        ORDER BY step_number ASC
    `, state.PlanID)
	if err != nil {
		return nil, 0, fmt.Errorf("load steps: %w", err)
	}

	if len(steps) == 0 {
		return nil, 0, fmt.Errorf("no fermentation steps for plan %d", state.PlanID)
	}

	// time since start in hours
	elapsed := now.Sub(state.StartedAt).Hours()

	accum := 0.0
	for i, step := range steps {
		accum += step.DurationHours
		if elapsed <= accum {
			return &step, i, nil
		}
	}

	// If beyond all steps → return last
	return &steps[len(steps)-1], len(steps) - 1, nil
}

func (s *SQLiteStore) GetTargetTemperature(state FermentationState, now time.Time) (float64, error) {
	step, _, err := s.GetActiveStep(state, now)
	if err != nil {
		return 0, err
	}
	return step.Temperature, nil
}
