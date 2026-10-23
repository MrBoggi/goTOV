package brewfather

import (
	"fmt"

	"github.com/MrBoggi/goTOV/internal/fermentation"
)

//
// 1) ENTRYPOINTS
//

// ExtractFermentationPlan – legacy wrapper
func ExtractFermentationPlan(recipe *BrewfatherRecipe) (*fermentation.FermentationPlan, error) {
	return ExtractFermentationPlanFromRecipe(recipe)
}

//
// 2) RECIPE → FERMENTATION PLAN
//

func ExtractFermentationPlanFromRecipe(recipe *BrewfatherRecipe) (*fermentation.FermentationPlan, error) {
	if recipe == nil {
		return nil, fmt.Errorf("nil recipe")
	}

	steps := convertSteps(recipe.Fermentation.Steps)

	plan := &fermentation.FermentationPlan{
		Name:       recipe.Name,
		RecipeID:   recipe.ID,
		TotalSteps: len(steps),
		Steps:      steps,
	}

	return plan, nil
}

//
// 3) BATCH → FERMENTATION PLAN
//

func ExtractFermentationPlanFromBatch(batch *BrewfatherBatch) (*fermentation.FermentationPlan, error) {
	if batch == nil {
		return nil, fmt.Errorf("nil batch")
	}

	// 1) Bruk batchens recipe-snapshot (ALLTID riktig)
	steps := batch.Recipe.Fermentation.Steps

	if len(steps) == 0 {
		return nil, fmt.Errorf("batch %s has no fermentation steps in recipe snapshot", batch.ID)
	}

	fp := &fermentation.FermentationPlan{
		Name:     batch.Name,
		RecipeID: batch.ID,
	}

	out := make([]fermentation.FermentationStep, 0, len(steps))

	for i, s := range steps {
		// Brewfather bruker "stepTemp" og "stepTime" på batch recipe
		var hours float64
		switch s.TimeUnit {
		case "day", "days":
			hours = s.Time * 24
		case "hour", "hours":
			hours = s.Time
		default:
			// fallback (batch recipe har stepTime)
			if s.Time > 0 {
				hours = s.Time
			} else if s.StepTime > 0 {
				hours = float64(s.StepTime)
			}
		}

		temp := s.Temperature
		if temp == 0 && s.StepTemp > 0 {
			temp = s.StepTemp
		}

		out = append(out, fermentation.FermentationStep{
			StepNumber:    i + 1,
			Temperature:   temp,
			DurationHours: hours,
			Type:          s.Type,
			Description:   s.Description,
		})
	}

	fp.Steps = out
	fp.TotalSteps = len(out)
	return fp, nil
}

//
// 4) STEP CONVERTERS
//

// Konverter fra BrewfatherRecipe.Fermentation.Steps til backend-format
func convertSteps(in []FermentationStep) []fermentation.FermentationStep {
	out := make([]fermentation.FermentationStep, 0, len(in))

	for _, s := range in {
		var hours float64
		switch s.TimeUnit {
		case "day", "days":
			hours = s.Time * 24
		case "hour", "hours":
			hours = s.Time
		default:
			hours = s.Time
		}

		out = append(out, fermentation.FermentationStep{
			StepNumber:    s.Step,
			Temperature:   s.Temperature,
			DurationHours: hours,
			Description:   s.Description,
			Type:          s.Type,
		})
	}

	return out
}
