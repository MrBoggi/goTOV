package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MrBoggi/goTOV/internal/logger"
	"github.com/MrBoggi/goTOV/internal/fermentation"
	"github.com/stretchr/testify/assert"
)

// MockFermentationStore is a mock implementation of fermentation.FermentationStore for testing.
type MockFermentationStore struct {
	SavePlanFunc func(plan fermentation.FermentationPlan) (int64, error)
}

func (m *MockFermentationStore) SavePlan(plan fermentation.FermentationPlan) (int64, error) {
	if m.SavePlanFunc != nil {
		return m.SavePlanFunc(plan)
	}
	return 1, nil // Default return for testing
}

func TestHealthCheckEndpoint(t *testing.T) {
	log := logger.New()
	server := NewServer(log, nil, &MockFermentationStore{}) // OPCUA client is not needed for this test

	req, err := http.NewRequest("GET", "/health", nil)
	assert.NoError(t, err)

	rr := httptest.NewRecorder()
	handler := server.Router()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	expected := `{"status":"ok"}` + "\n"
	assert.Equal(t, expected, rr.Body.String())
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))
}

func TestSaveFermentationPlanEndpoint(t *testing.T) {
	log := logger.New()
	mockStore := &MockFermentationStore{
		SavePlanFunc: func(plan fermentation.FermentationPlan) (int64, error) {
			assert.Equal(t, "Test Plan", plan.Name)
			assert.Equal(t, "RECIPE123", plan.RecipeID)
			assert.Len(t, plan.Steps, 2)
			return 42, nil // Simulate a new plan ID
		},
	}
	server := NewServer(log, nil, mockStore)

	plan := fermentation.FermentationPlan{
		Name:     "Test Plan",
		RecipeID: "RECIPE123",
		Steps: []fermentation.FermentationStep{
			{StepNumber: 1, Temperature: 20.0, DurationHours: 24, Description: "Primary", Type: "Ferment"},
			{StepNumber: 2, Temperature: 2.0, DurationHours: 48, Description: "Cold Crash", Type: "Condition"},
		},
	}
	body, _ := json.Marshal(plan)

	req, err := http.NewRequest("POST", "/api/fermentation/plan", bytes.NewReader(body))
	assert.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler := server.Router()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusCreated, rr.Code)

	var responseMap map[string]interface{}
	err = json.Unmarshal(rr.Body.Bytes(), &responseMap)
	assert.NoError(t, err)
	assert.Equal(t, "ok", responseMap["status"])
	assert.Equal(t, float64(42), responseMap["planID"]) // JSON numbers are often float64
}