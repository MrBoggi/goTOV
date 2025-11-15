package brewfather

// FermentationStep is how Brewfather represents a single fermentation step.
type FermentationStep struct {
	Step        int     `json:"step"`
	Type        string  `json:"type"`
	Temperature float64 `json:"temperature"`
	Time        float64 `json:"time"`
	TimeUnit    string  `json:"timeUnit"`
	Description string  `json:"description"`
}

// BrewfatherFermentation is the container for all steps.
type BrewfatherFermentation struct {
	Steps []FermentationStep `json:"steps"`
}

// BrewfatherRecipe describes the recipe payload we get from Brewfather.
type BrewfatherRecipe struct {
	ID           string                 `json:"_id"`
	Name         string                 `json:"name"`
	Fermentation BrewfatherFermentation `json:"fermentation"`
}

// BrewfatherBatch represents a Brewfather batch.
// NOTE: Batch fermentation is NOT stored under "fermentation" like recipes,
//
//	but under "batchFermentation".
type BrewfatherBatch struct {
	ID     string           `json:"_id"`
	Name   string           `json:"name"`
	Recipe BrewfatherRecipe `json:"recipe"`

	BatchFermentation struct {
		Steps []struct {
			Step        int     `json:"step"`
			Type        string  `json:"type"`
			Temperature float64 `json:"temperature"`
			Time        float64 `json:"time"`
			TimeUnit    string  `json:"timeUnit"`
			Description string  `json:"description"`
		} `json:"steps"`
	} `json:"batchFermentation"`
}
