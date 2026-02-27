package fermentation

import (
	"errors"
)

var (
	ErrPlanInUse    = errors.New("cannot delete plan while in use by an active fermentation")
	ErrPlanNotFound = errors.New("fermentation plan not found")
)

type FermentationStore interface {
	SavePlan(plan FermentationPlan) (int64, error)
	GetPlan(id int64) (FermentationPlan, error)
	GetSteps(planID int64) ([]FermentationStep, error)
	ListSteps(planID int64) ([]FermentationStep, error)
	ListPlans() ([]FermentationPlan, error)
	DeletePlan(id int64) error
	StartFermentation(planID int64, tankID string) (int64, error)
	ListActiveFermentations() ([]FermentationState, error)
	UpdateState(state FermentationState) error
	Clear() error
	Close() error
}
