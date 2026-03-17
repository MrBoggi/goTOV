package fermentation

import (
	"database/sql/driver"
	"fmt"
	"time"
)

// SQLiteTime wraps time.Time to handle scanning from SQLite string columns.
// modernc.org/sqlite stores timestamps as strings; database/sql cannot scan
// them into *time.Time directly.
type SQLiteTime struct {
	time.Time
}

// sqliteTimeFormats lists formats we may encounter in the database.
var sqliteTimeFormats = []string{
	"2006-01-02 15:04:05.999999999 +0000 UTC", // Go time.Time.String() format
	"2006-01-02 15:04:05.999999999 -0700 MST", // Go time.Time.String() with named timezone
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02 15:04:05.999999999-07:00",
	"2006-01-02T15:04:05.999999999-07:00",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02T15:04:05.999999999",
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05Z",
	"2006-01-02T15:04:05Z",
}

func (st *SQLiteTime) Scan(value interface{}) error {
	if value == nil {
		st.Time = time.Time{}
		return nil
	}
	switch v := value.(type) {
	case time.Time:
		st.Time = v
		return nil
	case string:
		for _, format := range sqliteTimeFormats {
			if t, err := time.Parse(format, v); err == nil {
				st.Time = t
				return nil
			}
		}
		return fmt.Errorf("cannot parse SQLite time string: %q", v)
	case []byte:
		s := string(v)
		for _, format := range sqliteTimeFormats {
			if t, err := time.Parse(format, s); err == nil {
				st.Time = t
				return nil
			}
		}
		return fmt.Errorf("cannot parse SQLite time bytes: %q", s)
	default:
		return fmt.Errorf("unsupported SQLiteTime scan type: %T", value)
	}
}

func (st SQLiteTime) Value() (driver.Value, error) {
	return st.Time.UTC().Format(time.RFC3339Nano), nil
}

// Et enkelt steg i en plan vi LAGRER i SQLite.
type FermentationStep struct {
	StepNumber    int     `db:"step_number" json:"stepNumber"`
	Temperature   float64 `db:"temperature" json:"temperature"`
	DurationHours float64 `db:"duration_hours" json:"durationHours"`
	Description   string  `db:"description" json:"description"`
	Type          string  `db:"type" json:"type"`
}

// Hendelser (events) som skjer i løpet av en gjæring (f.eks. tørrhumling).
type FermentationEvent struct {
	OffsetHours float64 `db:"offset_hours" json:"offsetHours"`
	Description string  `db:"description" json:"description"`
	Type        string  `db:"type" json:"type"`
}

// Selve planen – én plan per recipe.
type FermentationPlan struct {
	ID         int64               `db:"id" json:"id"`
	Name       string              `db:"name" json:"name"`
	RecipeID   string              `db:"recipe_id" json:"recipeId"`
	TotalSteps int                 `db:"total_steps" json:"totalSteps"`
	Steps      []FermentationStep  `db:"-" json:"steps,omitempty"` // hentes separat
	Events     []FermentationEvent `db:"-" json:"events,omitempty"`
}

const (
	StatusRunning   = "RUNNING"
	StatusCompleted = "COMPLETED"
	StatusPaused    = "PAUSED"
	StatusStopped   = "STOPPED"
	StatusError     = "ERROR"
)

// Til bruk av prosessmotoren (kommer senere)
type FermentationState struct {
	ID            int64                     `db:"id" json:"id"`
	PlanID        int64                     `db:"plan_id" json:"planId"`
	PlanName      string                    `db:"-" json:"planName,omitempty"`
	TankID        string                    `db:"tank_id" json:"tankId"`
	BatchID       string                    `db:"batch_id" json:"batchId"`
	StepIndex     int                       `db:"step_index" json:"stepIndex"`
	StartedAt     SQLiteTime                `db:"started_at" json:"startedAt"`
	StepStartedAt SQLiteTime                `db:"step_started_at" json:"stepStartedAt"`
	StepDuration  float64                   `db:"-" json:"stepDuration,omitempty"`
	TargetTemp    float64                   `db:"target_temp" json:"targetTemp"`
	Status        string                    `db:"status" json:"status"`
	Mode          string                    `db:"mode" json:"mode"`
	Transitioning bool                      `db:"-" json:"transitioning"`
	StepActive    bool                      `db:"step_active" json:"stepActive"`
	Events        []ActiveFermentationEvent `db:"-" json:"events,omitempty"`
}

type ActiveFermentationEvent struct {
	FermentationEvent
	Completed   bool        `db:"completed" json:"completed"`
	CompletedAt *SQLiteTime `db:"completed_at" json:"completedAt"`
}

type FermentationHistoryEntry struct {
	Timestamp     SQLiteTime `db:"timestamp" json:"timestamp"`
	Temperature   float64    `db:"temperature" json:"temperature"`
	TargetTemp    float64    `db:"target_temp" json:"targetTemp"`
	CoolingValve  bool       `db:"cooling_valve" json:"coolingValve"`
	HeatingJacket bool       `db:"heating_jacket" json:"heatingJacket"`
}

// GlycolStatus representerer nåværende status for glykol-kjøleren
type GlycolStatus struct {
	Temperature    float64             `json:"temperature"`
	Pressure       *float64            `json:"pressure"`
	LoadPercentage float64             `json:"loadPercentage"`
	Trend24h       []GlycolHistoryData `json:"trend24h"`
	Mode           string              `json:"mode"`           // "Auto" or "Manual"
	ManualPumpOn   bool                `json:"manualPumpOn"`   // Current manual desired state
}

type GlycolModeRequest struct {
	Mode string `json:"mode"`
}

type GlycolControlRequest struct {
	On bool `json:"on"`
}

// GlycolHistoryData er et enkelt datapunkt for glykol trendgrafer
type GlycolHistoryData struct {
	Timestamp SQLiteTime `db:"timestamp" json:"timestamp"`
	Value     float64    `db:"temperature" json:"value"` // Exposed as "value" in API trend for graphs
	Pressure  *float64   `db:"pressure" json:"-"`
	LoadPct   float64    `db:"load_percentage" json:"-"`
}

// ManualControlRequest representerer en forespørsel om å sette manuell styring av kjøleren.
type ManualControlRequest struct {
	ID      int64 `json:"id"`
	Cooling bool  `json:"cooling"`
}
