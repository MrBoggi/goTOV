package fermentation

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

// Til bruk av prosessmotoren (kommer senere)
type FermentationState struct {
	BatchID       string  `db:"batch_id"`
	TankNo        int     `db:"tank_no"`
	StepIndex     int     `db:"step_index"`
	StartedAt     string  `db:"started_at"`
	StepStartedAt string  `db:"step_started_at"`
	TargetTemp    float64 `db:"target_temp"`
	Status        string  `db:"status"`
}
