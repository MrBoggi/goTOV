package fermentation

import (
	"errors"
)

var (
	ErrPlanInUse            = errors.New("cannot delete plan while in use by an active fermentation")
	ErrPlanNotFound         = errors.New("fermentation plan not found")
	ErrFermentationNotFound = errors.New("active fermentation not found")
	ErrEventNotFound        = errors.New("event not found")
	ErrTankBusy             = errors.New("tank already has an active fermentation running")
)

type FermentationStore interface {
	SavePlan(plan FermentationPlan) (int64, error)
	GetPlan(id int64) (FermentationPlan, error)
	GetSteps(planID int64) ([]FermentationStep, error)
	ListSteps(planID int64) ([]FermentationStep, error)
	ListPlans() ([]FermentationPlan, error)
	DeletePlan(id int64) error
	GetEvents(planID int64) ([]FermentationEvent, error)
	StartFermentation(planID int64, tankID string, batchID string) (int64, error)
	ListActiveFermentations() ([]FermentationState, error)
	GetState(id int64) (FermentationState, error)
	GetStateByTank(tankID string) (FermentationState, error)
	GetActiveEvents(activeID int64) ([]ActiveFermentationEvent, error)
	UpdateState(state FermentationState) error
	CompleteEvent(fermentationID int64, eventIndex int) error
	StopFermentation(id int64) error
	LogData(fermentationID int64, planID int64, tankID string, batchID string, temp float32, target float32, valve bool, jacket bool) error
	GetHistory(fermentationID int64, hours float64) ([]FermentationHistoryEntry, error)
	LogGlycolData(temp float64, pressure *float64, load float64) error
	GetGlycolHistory(hours float64) ([]GlycolHistoryData, error)
	Clear() error
	Close() error
}
