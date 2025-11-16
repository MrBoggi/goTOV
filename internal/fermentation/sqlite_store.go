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

CREATE TABLE IF NOT EXISTS fermentation_state (
    batch_id TEXT PRIMARY KEY,
    tank_no INTEGER NOT NULL,
    plan_id INTEGER NOT NULL,
    started_at TEXT NOT NULL,
    step_started_at TEXT NOT NULL,
    status TEXT NOT NULL,
    FOREIGN KEY(plan_id) REFERENCES fermentation_plans(id)
);`
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

func (s *SQLiteStore) SaveFermentationState(state FermentationState) error {
	_, err := s.DB.Exec(`
		INSERT INTO fermentation_state 
		(batch_id, tank_no, plan_id, started_at, step_started_at, status)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(batch_id) DO UPDATE SET
			tank_no = excluded.tank_no,
			plan_id = excluded.plan_id,
			started_at = excluded.started_at,
			step_started_at = excluded.step_started_at,
			status = excluded.status;
	`,
		state.BatchID,
		state.TankNo,
		state.PlanID,
		state.StartedAt.Format(time.RFC3339),
		state.StepStartedAt.Format(time.RFC3339),
		string(state.Status),
	)
	if err != nil {
		return fmt.Errorf("save fermentation state: %w", err)
	}
	return nil
}

//
// ROW OBJECT FOR SQLITE
// (needed to safely scan TEXT → string → time.Time)
//

type fermentationStateRow struct {
	BatchID       string `db:"batch_id"`
	TankNo        int    `db:"tank_no"`
	PlanID        int64  `db:"plan_id"`
	StartedAt     string `db:"started_at"`      // TEXT in SQLite
	StepStartedAt string `db:"step_started_at"` // TEXT in SQLite
	Status        string `db:"status"`
}

//
// MAP ROW → DOMAIN
//

func (r fermentationStateRow) toDomain() (*FermentationState, error) {
	startedAt, err := time.Parse(time.RFC3339, r.StartedAt)
	if err != nil {
		return nil, fmt.Errorf("parse started_at: %w", err)
	}

	stepStartedAt, err := time.Parse(time.RFC3339, r.StepStartedAt)
	if err != nil {
		return nil, fmt.Errorf("parse step_started_at: %w", err)
	}

	return &FermentationState{
		BatchID:       r.BatchID,
		TankNo:        r.TankNo,
		PlanID:        r.PlanID,
		StartedAt:     startedAt,
		StepStartedAt: stepStartedAt,
		Status:        FermentationStatus(r.Status),
	}, nil
}

//
// PUBLIC API
//

func (s *SQLiteStore) GetActiveFermentationState() (*FermentationState, error) {
	var row fermentationStateRow

	err := s.DB.Get(&row, `
		SELECT batch_id, tank_no, plan_id, started_at, step_started_at, status
		FROM fermentation_state
		ORDER BY started_at DESC
		LIMIT 1;
	`)
	if err != nil {
		return nil, fmt.Errorf("get active fermentation state: %w", err)
	}

	return row.toDomain()
}
