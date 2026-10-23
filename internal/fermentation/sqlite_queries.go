package fermentation

// Henter alle planer
func (s *SQLiteStore) ListPlans() ([]FermentationPlan, error) {
	rows := []FermentationPlan{}
	err := s.DB.Select(&rows, `
		SELECT id, name, recipe_id, total_steps
		FROM fermentation_plans
		ORDER BY id ASC;
	`)
	return rows, err
}

// Henter steps for én plan
func (s *SQLiteStore) ListSteps(planID int) ([]FermentationStep, error) {
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
