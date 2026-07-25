package fermentation

import (
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestEngineStopFermentationRemovesActiveState(t *testing.T) {
	store, err := NewSQLiteStore(t.TempDir() + "/fermentation.db")
	require.NoError(t, err)
	t.Cleanup(func() {
		if closeErr := store.Close(); closeErr != nil {
			t.Errorf("close store: %v", closeErr)
		}
	})

	planID, err := store.SavePlan(FermentationPlan{
		Name:     "Test plan",
		RecipeID: "recipe-1",
		Steps: []FermentationStep{
			{StepNumber: 1, Temperature: 20, DurationHours: 24},
		},
	})
	require.NoError(t, err)

	fermentationID, err := store.StartFermentation(planID, "1", "batch-1")
	require.NoError(t, err)

	state, err := store.GetState(fermentationID)
	require.NoError(t, err)

	engine := NewEngine(store, nil, zerolog.Nop())
	engine.AddFermentation(&state)

	require.NoError(t, engine.StopFermentation(fermentationID))

	active := engine.GetActiveStates()
	require.Empty(t, active)

	persisted, err := store.GetState(fermentationID)
	require.NoError(t, err)
	require.Equal(t, StatusStopped, persisted.Status)
}
