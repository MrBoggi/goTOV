package fermentation

import "time"

// A single step in a fermentation plan stored in SQLite.
type FermentationStep struct {
	StepNumber    int     `db:"step_number" json:"step_number"`
	Temperature   float64 `db:"temperature" json:"temperature"`
	DurationHours float64 `db:"duration_hours" json:"duration_hours"`
	Description   string  `db:"description" json:"description"`
	Type          string  `db:"type" json:"type"`
}

// A complete fermentation plan (one per recipe).
type FermentationPlan struct {
	ID         int64              `db:"id"`
	Name       string             `db:"name"`
	RecipeID   string             `db:"recipe_id"`
	TotalSteps int                `db:"total_steps"`
	Steps      []FermentationStep `db:"-"` // steps loaded separately
}

// Status for a running fermentation.
type FermentationStatus string

const (
	StatusRunning FermentationStatus = "running"
	StatusPaused  FermentationStatus = "paused"
	StatusDone    FermentationStatus = "done"
)

// The ACTIVE fermentation batch (runtime state loaded from SQLite).
type FermentationState struct {
	BatchID       string             `db:"batch_id"`
	TankNo        int                `db:"tank_no"`
	PlanID        int64              `db:"plan_id"`
	StartedAt     time.Time          `db:"-"`
	StepStartedAt time.Time          `db:"-"`
	Status        FermentationStatus `db:"status"`
}
