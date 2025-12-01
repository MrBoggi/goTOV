package fermentation

import (
	"fmt"
)

// Henter alle planer
func (s *SQLiteStore) ListPlans() ([]FermentationPlan, error) {
	var plans []FermentationPlan
	err := s.DB.Select(&plans, `
		SELECT id, name, recipe_id, total_steps
		FROM fermentation_plans
		ORDER BY id ASC;
	`)
	if err != nil {
		return nil, fmt.Errorf("select plans: %w", err)
	}

	for i := range plans {
		steps, err := s.ListSteps(plans[i].ID)
		if err != nil {
			return nil, fmt.Errorf("select steps for plan %d: %w", plans[i].ID, err)
		}
		plans[i].Steps = steps
	}

	return plans, nil
}

// Henter steps for én plan
func (s *SQLiteStore) ListSteps(planID int64) ([]FermentationStep, error) {
	rows := []FermentationStep{}
	err := s.DB.Select(&rows, `
		SELECT step_number, temperature, duration_hours, description, type
		FROM fermentation_steps
		WHERE plan_id = ?
		ORDER BY step_number ASC;
	`, planID)
	return rows, err
}

// Tømmer begge tabellene
func (s *SQLiteStore) Clear() error {
	_, err := s.DB.Exec(`DELETE FROM fermentation_steps; DELETE FROM fermentation_plans;`)
	return err
}
