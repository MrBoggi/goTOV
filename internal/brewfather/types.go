package brewfather

// FermentationStep is how Brewfather represents a single fermentation step.
// We support both the "old" style (time/temperature/timeUnit) and the
// recipe-snapshot style (stepTemp/stepTime).
type FermentationStep struct {
	// Generisk form (noen endepunkt bruker dette)
	Step        int     `json:"step"` // optional
	Type        string  `json:"type"`
	Temperature float64 `json:"temperature"` // optional
	Time        float64 `json:"time"`        // optional
	TimeUnit    string  `json:"timeUnit"`    // "day"/"hour", optional
	Description string  `json:"description"`

	// Recipe/batch snapshot-form (som i Postman-jsonen din)
	StepTemp float64 `json:"stepTemp"` // °C
	StepTime float64 `json:"stepTime"` // typisk i dager
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
