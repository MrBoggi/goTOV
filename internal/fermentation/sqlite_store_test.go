package fermentation

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSQLiteStore_HistoryFiltering(t *testing.T) {
	dbPath := "test_fermentation.db"
	defer os.Remove(dbPath)

	store, err := NewSQLiteStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	// 1. Create a plan and start two fermentations
	planID, err := store.SavePlan(FermentationPlan{
		Name:     "Test Plan",
		RecipeID: "R1",
		Steps: []FermentationStep{
			{StepNumber: 1, Temperature: 20.0, DurationHours: 24},
		},
	})
	require.NoError(t, err)

	f1ID, err := store.StartFermentation(planID, "Tank1", "B1")
	require.NoError(t, err)
	f2ID, err := store.StartFermentation(planID, "Tank2", "B2")
	require.NoError(t, err)

	// 2. Log data for each (using different temperatures)
	err = store.LogData(f1ID, planID, "Tank1", "B1", 19.0, 20.0, false, false)
	assert.NoError(t, err)
	err = store.LogData(f2ID, planID, "Tank2", "B2", 21.0, 20.0, true, false)
	assert.NoError(t, err)

	// 3. Verify history for f1 (should only have 19.0)
	h1, err := store.GetHistory(f1ID, 24)
	assert.NoError(t, err)
	assert.Len(t, h1, 1)
	assert.Equal(t, 19.0, h1[0].Temperature)

	// 4. Verify history for f2 (should only have 21.0)
	h2, err := store.GetHistory(f2ID, 24)
	assert.NoError(t, err)
	assert.Len(t, h2, 1)
	assert.Equal(t, 21.0, h2[0].Temperature)
}
