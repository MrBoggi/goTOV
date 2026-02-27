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
    batch_id TEXT NOT NULL,
    step_index INTEGER NOT NULL,
    started_at TIMESTAMP NOT NULL,
    step_started_at TIMESTAMP NOT NULL,
    target_temp REAL NOT NULL,
    status TEXT NOT NULL,
    FOREIGN KEY(plan_id) REFERENCES fermentation_plans(id)
);

CREATE TABLE IF NOT EXISTS fermentation_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    plan_id INTEGER NOT NULL,
    tank_id TEXT NOT NULL,
    batch_id TEXT NOT NULL,
    timestamp TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    temperature REAL NOT NULL,
    target_temp REAL NOT NULL,
    cooling_valve BOOLEAN NOT NULL,
    heating_jacket BOOLEAN NOT NULL,
    FOREIGN KEY(plan_id) REFERENCES fermentation_plans(id)
);
`
	_, err := s.DB.Exec(schema)
	if err != nil {
		return fmt.Errorf("failed to run schema migration: %w", err)
	}

	// Check for tank_id column in fermentation_states (for backward compatibility)
	var count int
	err = s.DB.Get(&count, "SELECT count(*) FROM pragma_table_info('fermentation_states') WHERE name='tank_id'")
	if err == nil && count == 0 {
		_, err = s.DB.Exec("ALTER TABLE fermentation_states ADD COLUMN tank_id TEXT NOT NULL DEFAULT '1'")
		if err != nil {
			return fmt.Errorf("failed to add tank_id column: %w", err)
		}
	}

	// Check for batch_id column in fermentation_states (for backward compatibility)
	err = s.DB.Get(&count, "SELECT count(*) FROM pragma_table_info('fermentation_states') WHERE name='batch_id'")
	if err == nil && count == 0 {
		_, err = s.DB.Exec("ALTER TABLE fermentation_states ADD COLUMN batch_id TEXT NOT NULL DEFAULT ''")
		if err != nil {
			return fmt.Errorf("failed to add batch_id column: %w", err)
		}
	}

	// Check for legacy tank_no column in fermentation_states (can cause NOT NULL constraint failures)
	err = s.DB.Get(&count, "SELECT count(*) FROM pragma_table_info('fermentation_states') WHERE name='tank_no'")
	if err == nil && count > 0 {
		_, err = s.DB.Exec("ALTER TABLE fermentation_states DROP COLUMN tank_no")
		if err != nil {
			return fmt.Errorf("failed to drop legacy tank_no column: %w", err)
		}
	}

	return nil
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

func (s *SQLiteStore) StartFermentation(planID int64, tankID string, batchID string) (int64, error) {
	steps, err := s.GetSteps(planID)
	if err != nil || len(steps) == 0 {
		return 0, fmt.Errorf("get steps for starting fermentation: %w", err)
	}

	now := time.Now().UTC()
	state := FermentationState{
		PlanID:        planID,
		TankID:        tankID,
		BatchID:       batchID,
		StepIndex:     0,
		StartedAt:     now,
		StepStartedAt: now,
		TargetTemp:    steps[0].Temperature,
		Status:        StatusRunning,
	}

	res, err := s.DB.NamedExec(`
INSERT INTO fermentation_states (plan_id, tank_id, batch_id, step_index, started_at, step_started_at, target_temp, status)
VALUES (:plan_id, :tank_id, :batch_id, :step_index, :started_at, :step_started_at, :target_temp, :status)`,
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

func (s *SQLiteStore) GetState(id int64) (FermentationState, error) {
	var state FermentationState
	err := s.DB.Get(&state, "SELECT * FROM fermentation_states WHERE id = ?", id)
	return state, err
}

func (s *SQLiteStore) GetStateByTank(tankID string) (FermentationState, error) {
	var state FermentationState
	err := s.DB.Get(&state, "SELECT * FROM fermentation_states WHERE tank_id = ? AND status = ? ORDER BY started_at DESC LIMIT 1", tankID, StatusRunning)
	return state, err
}

func (s *SQLiteStore) StopFermentation(id int64) error {
	res, err := s.DB.Exec("UPDATE fermentation_states SET status = ? WHERE id = ?", StatusStopped, id)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrFermentationNotFound
	}
	return nil
}

func (s *SQLiteStore) UpdateState(state FermentationState) error {
	_, err := s.DB.NamedExec(`
UPDATE fermentation_states 
SET step_index = :step_index, step_started_at = :step_started_at, target_temp = :target_temp, status = :status
WHERE id = :id`,
		state)
	return err
}

func (s *SQLiteStore) LogData(planID int64, tankID string, batchID string, temp float32, target float32, valve bool, jacket bool) error {
	_, err := s.DB.Exec(`
INSERT INTO fermentation_history 
(plan_id, tank_id, batch_id, temperature, target_temp, cooling_valve, heating_jacket)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		planID, tankID, batchID, temp, target, valve, jacket)
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
