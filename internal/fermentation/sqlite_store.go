package fermentation

import (
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	DB *sqlx.DB
}

var _ FermentationStore = (*SQLiteStore)(nil)

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

// Close releases the database connection and should be called when the store is no longer needed.
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

CREATE TABLE IF NOT EXISTS fermentation_states (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    plan_id INTEGER NOT NULL,
	batch_id TEXT NOT NULL,
    tank_no INTEGER NOT NULL,
    step_index INTEGER NOT NULL,
    started_at TEXT NOT NULL,
    step_started_at TEXT NOT NULL,
    target_temp REAL NOT NULL,
    status TEXT NOT NULL,
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



func (s *SQLiteStore) StartFermentation(planID int64, tankID string) (int64, error) {
	var plan FermentationPlan
	err := s.DB.Get(&plan, "SELECT id, name, recipe_id, total_steps FROM fermentation_plans WHERE id = ?", planID)
	if err != nil {
		return 0, fmt.Errorf("get plan for starting fermentation: %w", err)
	}

	var steps []FermentationStep
	err = s.DB.Select(&steps, "SELECT step_number, temperature, duration_hours, description, type FROM fermentation_steps WHERE plan_id = ? ORDER BY step_number ASC", planID)
	if err != nil {
		return 0, fmt.Errorf("get steps for starting fermentation: %w", err)
	}
	plan.Steps = steps

	if len(plan.Steps) == 0 {
		return 0, fmt.Errorf("fermentation plan %d has no steps", planID)
	}

	now := time.Now().UTC()
	initialState := FermentationState{
		PlanID:        planID,
		BatchID:       fmt.Sprintf("BATCH-%s-%d", tankID, planID), // Placeholder for BatchID
		TankNo:        1,                                          // Placeholder for TankNo, needs proper parsing from tankID
		StepIndex:     0,
		StartedAt:     now.Format(time.RFC3339),
		StepStartedAt: now.Format(time.RFC3339),
		TargetTemp:    plan.Steps[0].Temperature,
		Status:        "RUNNING",
	}

	res, err := s.DB.Exec(`
INSERT INTO fermentation_states (plan_id, batch_id, tank_no, step_index, started_at, step_started_at, target_temp, status)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		initialState.PlanID, initialState.BatchID, initialState.TankNo, initialState.StepIndex,
		initialState.StartedAt, initialState.StepStartedAt, initialState.TargetTemp, initialState.Status)
	if err != nil {
		return 0, fmt.Errorf("insert fermentation state: %w", err)
	}

	fermentationID, _ := res.LastInsertId()
	return fermentationID, nil
}
