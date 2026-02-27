package fermentation

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	DB *sqlx.DB
}

var _ FermentationStore = (*SQLiteStore)(nil)

func NewSQLiteStore(path string) (*SQLiteStore, error) {
	// Ensure the directory for the database file exists.
	dir := filepath.Dir(path)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("create directory for sqlite db: %w", err)
		}
	}

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
    tank_id TEXT NOT NULL,
    step_index INTEGER NOT NULL,
    started_at TIMESTAMP NOT NULL,
    step_started_at TIMESTAMP NOT NULL,
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

func (s *SQLiteStore) GetPlan(id int64) (FermentationPlan, error) {
	var plan FermentationPlan
	err := s.DB.Get(&plan, "SELECT * FROM fermentation_plans WHERE id = ?", id)
	return plan, err
}

func (s *SQLiteStore) GetSteps(planID int64) ([]FermentationStep, error) {
	return s.ListSteps(planID)
}

func (s *SQLiteStore) ListSteps(planID int64) ([]FermentationStep, error) {
	var steps []FermentationStep
	err := s.DB.Select(&steps, "SELECT step_number, temperature, duration_hours, description, type FROM fermentation_steps WHERE plan_id = ? ORDER BY step_number ASC", planID)
	return steps, err
}

func (s *SQLiteStore) ListPlans() ([]FermentationPlan, error) {
	var plans []FermentationPlan
	err := s.DB.Select(&plans, "SELECT * FROM fermentation_plans")
	if err != nil {
		return nil, err
	}
	// Fetch steps for each plan
	for i := range plans {
		steps, err := s.GetSteps(plans[i].ID)
		if err == nil {
			plans[i].Steps = steps
		}
	}
	return plans, nil
}

func (s *SQLiteStore) StartFermentation(planID int64, tankID string) (int64, error) {
	steps, err := s.GetSteps(planID)
	if err != nil || len(steps) == 0 {
		return 0, fmt.Errorf("get steps for starting fermentation: %w", err)
	}

	now := time.Now().UTC()
	state := FermentationState{
		PlanID:        planID,
		TankID:        tankID,
		StepIndex:     0,
		StartedAt:     now,
		StepStartedAt: now,
		TargetTemp:    steps[0].Temperature,
		Status:        StatusRunning,
	}

	res, err := s.DB.NamedExec(`
INSERT INTO fermentation_states (plan_id, tank_id, step_index, started_at, step_started_at, target_temp, status)
VALUES (:plan_id, :tank_id, :step_index, :started_at, :step_started_at, :target_temp, :status)`,
		state)
	if err != nil {
		return 0, fmt.Errorf("insert fermentation state: %w", err)
	}

	id, _ := res.LastInsertId()
	return id, nil
}

func (s *SQLiteStore) ListActiveFermentations() ([]FermentationState, error) {
	var active []FermentationState
	err := s.DB.Select(&active, "SELECT * FROM fermentation_states WHERE status = ?", StatusRunning)
	return active, err
}

func (s *SQLiteStore) UpdateState(state FermentationState) error {
	_, err := s.DB.NamedExec(`
UPDATE fermentation_states 
SET step_index = :step_index, step_started_at = :step_started_at, target_temp = :target_temp, status = :status
WHERE id = :id`,
		state)
	return err
}

func (s *SQLiteStore) DeletePlan(id int64) error {
	// 1. Check if plan exists
	var exists bool
	err := s.DB.Get(&exists, "SELECT EXISTS(SELECT 1 FROM fermentation_plans WHERE id = ?)", id)
	if err != nil {
		return fmt.Errorf("check plan existence: %w", err)
	}
	if !exists {
		return ErrPlanNotFound
	}

	// 2. Check if plan is in use
	var inUse bool
	err = s.DB.Get(&inUse, "SELECT EXISTS(SELECT 1 FROM fermentation_states WHERE plan_id = ? AND status = ?)", id, StatusRunning)
	if err != nil {
		return fmt.Errorf("check if plan in use: %w", err)
	}
	if inUse {
		return ErrPlanInUse
	}

	// 3. Delete plan and steps in transition
	tx, err := s.DB.Beginx()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	// Delete steps first
	_, err = tx.Exec("DELETE FROM fermentation_steps WHERE plan_id = ?", id)
	if err != nil {
		return fmt.Errorf("delete steps: %w", err)
	}

	// Delete plan
	_, err = tx.Exec("DELETE FROM fermentation_plans WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete plan: %w", err)
	}

	return tx.Commit()
}

func (s *SQLiteStore) Clear() error {
	_, err := s.DB.Exec("DELETE FROM fermentation_steps; DELETE FROM fermentation_states; DELETE FROM fermentation_plans;")
	return err
}
