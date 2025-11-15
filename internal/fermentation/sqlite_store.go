package fermentation

import (
	"fmt"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	DB *sqlx.DB
}

func NewSQLiteStore(path string) (*SQLiteStore, error) {
	db, err := sqlx.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	s := &SQLiteStore{DB: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *SQLiteStore) Close() error {
	return s.DB.Close()
}

func (s *SQLiteStore) migrate() error {
	schema := `
CREATE TABLE IF NOT EXISTS fermentation_plans (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    recipe_id TEXT NOT NULL,
    total_steps INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS fermentation_steps (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    plan_id INTEGER NOT NULL,
    step_number INTEGER NOT NULL,
    temperature REAL NOT NULL,
    duration_hours REAL NOT NULL,
    description TEXT,
    type TEXT,
    FOREIGN KEY(plan_id) REFERENCES fermentation_plans(id)
);
`
	_, err := s.DB.Exec(schema)
	return err
}

func (s *SQLiteStore) SavePlan(plan FermentationPlan) (int64, error) {
	res, err := s.DB.Exec(`
INSERT INTO fermentation_plans (name, recipe_id, total_steps)
VALUES (?, ?, ?)`,
		plan.Name, plan.RecipeID, len(plan.Steps))
	if err != nil {
		return 0, fmt.Errorf("insert plan: %w", err)
	}

	planID, _ := res.LastInsertId()

	for _, step := range plan.Steps {
		_, err := s.DB.Exec(`
INSERT INTO fermentation_steps 
(plan_id, step_number, temperature, duration_hours, description, type)
VALUES (?, ?, ?, ?, ?, ?)`,
			planID, step.StepNumber, step.Temperature,
			step.DurationHours, step.Description, step.Type)
		if err != nil {
			return 0, fmt.Errorf("insert step: %w", err)
		}
	}
	return planID, nil
}
