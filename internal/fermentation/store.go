package fermentation

type FermentationStore interface {
	SavePlan(plan FermentationPlan) (int64, error)
	// Add other methods as needed, e.g., GetPlan, ListPlans, etc.
}
