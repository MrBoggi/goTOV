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

func TestSQLiteStore_Events(t *testing.T) {
	dbPath := "test_events.db"
	defer os.Remove(dbPath)

	store, err := NewSQLiteStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	// 1. Create a plan with events
	plan := FermentationPlan{
		Name:     "Event Plan",
		RecipeID: "R2",
		Steps: []FermentationStep{
			{StepNumber: 1, Temperature: 20.0, DurationHours: 24},
		},
		Events: []FermentationEvent{
			{OffsetHours: 12, Description: "Dry Hop 1", Type: "Hop"},
			{OffsetHours: 48, Description: "Dry Hop 2", Type: "Hop"},
		},
	}

	planID, err := store.SavePlan(plan)
	require.NoError(t, err)

	// 2. Start fermentation
	activeID, err := store.StartFermentation(planID, "Tank1", "B1")
	require.NoError(t, err)

	// 3. Verify active events were copied
	events, err := store.GetActiveEvents(activeID)
	assert.NoError(t, err)
	assert.Len(t, events, 2)
	assert.Equal(t, "Dry Hop 1", events[0].Description)
	assert.Equal(t, 12.0, events[0].OffsetHours)
	assert.False(t, events[0].Completed)

	// 4. Complete an event
	err = store.CompleteEvent(activeID, 0)
	assert.NoError(t, err)

	// 5. Verify completion
	events, err = store.GetActiveEvents(activeID)
	assert.NoError(t, err)
	assert.True(t, events[0].Completed)
	assert.NotNil(t, events[0].CompletedAt)
	assert.False(t, events[0].CompletedAt.Time.IsZero())
	assert.False(t, events[1].Completed)

	// 6. Test with non-existent event
	err = store.CompleteEvent(activeID, 99)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrEventNotFound)
}

func TestSQLiteStore_PlanUpdateAndActiveStepOverride(t *testing.T) {
	dbPath := "test_plan_update.db"
	defer os.Remove(dbPath)

	store, err := NewSQLiteStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	// 1. Create a plan
	plan := FermentationPlan{
		Name:     "Adjustable Plan",
		RecipeID: "R3",
		Steps: []FermentationStep{
			{StepNumber: 1, Temperature: 18.0, DurationHours: 48},
			{StepNumber: 2, Temperature: 22.0, DurationHours: 72},
		},
	}
	planID, err := store.SavePlan(plan)
	require.NoError(t, err)
	plan.ID = planID

	// 2. Start fermentation (should copy steps)
	activeID, err := store.StartFermentation(planID, "Tank1", "B3")
	require.NoError(t, err)

	// 3. Verify active steps were copied
	activeSteps, err := store.GetActiveSteps(activeID)
	assert.NoError(t, err)
	assert.Len(t, activeSteps, 2)
	assert.Equal(t, 18.0, activeSteps[0].Temperature)

	// 4. Update the original plan
	plan.Steps[0].Temperature = 19.0
	err = store.UpdatePlan(plan)
	assert.NoError(t, err)

	// 5. Verify the active fermentation still has the OLD temperature (isolation)
	activeSteps, err = store.GetActiveSteps(activeID)
	assert.NoError(t, err)
	assert.Equal(t, 18.0, activeSteps[0].Temperature)

	// 6. Override the active step temperature (step_number 1)
	err = store.UpdateActiveStep(activeID, 1, 20.0, 50.0)
	assert.NoError(t, err)

	// 7. Verify the override
	activeSteps, err = store.GetActiveSteps(activeID)
	assert.NoError(t, err)
	assert.Equal(t, 20.0, activeSteps[0].Temperature)
	assert.Equal(t, 50.0, activeSteps[0].DurationHours)

	// 8. Verify the plan remains unchanged by the active step override
	planSteps, err := store.GetSteps(planID)
	assert.NoError(t, err)
	assert.Equal(t, 19.0, planSteps[0].Temperature) // Updated in step 4
}
