package fermentation

import "time"

// Et enkelt steg i en plan vi LAGRER i SQLite.
type FermentationStep struct {
	StepNumber    int     `db:"step_number" json:"step_number"`
	Temperature   float64 `db:"temperature" json:"temperature"`
	DurationHours float64 `db:"duration_hours" json:"duration_hours"`
	Description   string  `db:"description" json:"description"`
	Type          string  `db:"type" json:"type"`
}

// Selve planen – én plan per recipe.
type FermentationPlan struct {
	ID         int64              `db:"id"`
	Name       string             `db:"name"`
	RecipeID   string             `db:"recipe_id"`
	TotalSteps int                `db:"total_steps"`
	Steps      []FermentationStep `db:"-"` // hentes separat
}

const (
	StatusRunning   = "RUNNING"
	StatusCompleted = "COMPLETED"
	StatusPaused    = "PAUSED"
	StatusError     = "ERROR"
)

// Til bruk av prosessmotoren (kommer senere)
type FermentationState struct {
	ID            int64     `db:"id" json:"id"`
	PlanID        int64     `db:"plan_id" json:"planID"`
	TankID        string    `db:"tank_id" json:"tankID"`
	BatchID       string    `db:"batch_id" json:"batchID"`
	StepIndex     int       `db:"step_index" json:"stepIndex"`
	StartedAt     time.Time `db:"started_at" json:"startedAt"`
	StepStartedAt time.Time `db:"step_started_at" json:"stepStartedAt"`
	TargetTemp    float64   `db:"target_temp" json:"targetTemp"`
	Status        string    `db:"status" json:"status"`
}
