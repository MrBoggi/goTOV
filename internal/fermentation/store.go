package fermentation

import (
	"errors"
)

var (
	ErrPlanInUse            = errors.New("cannot delete plan while in use by an active fermentation")
	ErrPlanNotFound         = errors.New("fermentation plan not found")
	ErrFermentationNotFound = errors.New("active fermentation not found")
)

type FermentationStore interface {
	SavePlan(plan FermentationPlan) (int64, error)
	GetPlan(id int64) (FermentationPlan, error)
	GetSteps(planID int64) ([]FermentationStep, error)
	ListSteps(planID int64) ([]FermentationStep, error)
	ListPlans() ([]FermentationPlan, error)
	DeletePlan(id int64) error
	StartFermentation(planID int64, tankID string, batchID string) (int64, error)
	ListActiveFermentations() ([]FermentationState, error)
	GetState(id int64) (FermentationState, error)
	GetStateByTank(tankID string) (FermentationState, error)
	UpdateState(state FermentationState) error
	StopFermentation(id int64) error
	LogData(planID int64, tankID string, batchID string, temp float32, target float32, valve bool, jacket bool) error
	GetHistory(planID int64, hours float64) ([]FermentationHistoryEntry, error)
	Clear() error
	Close() error
}
