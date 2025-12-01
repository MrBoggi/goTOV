package fermentation

type FermentationStore interface {
	SavePlan(plan FermentationPlan) (int64, error)
	ListPlans() ([]FermentationPlan, error)
	StartFermentation(planID int64, tankID string) (int64, error)
}
