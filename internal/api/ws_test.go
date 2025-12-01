package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MrBoggi/goTOV/internal/fermentation"
	"github.com/MrBoggi/goTOV/internal/logger"
	"github.com/stretchr/testify/assert"
)

// MockFermentationStore is a mock implementation of fermentation.FermentationStore for testing.
type MockFermentationStore struct {
	SavePlanFunc          func(plan fermentation.FermentationPlan) (int64, error)
	ListPlansFunc         func() ([]fermentation.FermentationPlan, error)
	StartFermentationFunc func(planID int64, tankID string) (int64, error)
}

func (m *MockFermentationStore) SavePlan(plan fermentation.FermentationPlan) (int64, error) {
	if m.SavePlanFunc != nil {
		return m.SavePlanFunc(plan)
	}
	return 1, nil // Default return for testing
}

func (m *MockFermentationStore) ListPlans() ([]fermentation.FermentationPlan, error) {
	if m.ListPlansFunc != nil {
		return m.ListPlansFunc()
	}
	return nil, nil // Default return for testing
}

func (m *MockFermentationStore) StartFermentation(planID int64, tankID string) (int64, error) {
	if m.StartFermentationFunc != nil {
		return m.StartFermentationFunc(planID, tankID)
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

func TestListFermentationPlansEndpoint(t *testing.T) {
	log := logger.New()
	expectedPlans := []fermentation.FermentationPlan{
		{
			ID:         1,
			Name:       "Test Plan 1",
			RecipeID:   "RECIPE001",
			TotalSteps: 2,
			Steps: []fermentation.FermentationStep{
				{StepNumber: 1, Temperature: 20.0, DurationHours: 24, Description: "Primary", Type: "Ferment"},
				{StepNumber: 2, Temperature: 2.0, DurationHours: 48, Description: "Cold Crash", Type: "Condition"},
			},
		},
		{
			ID:         2,
			Name:       "Test Plan 2",
			RecipeID:   "RECIPE002",
			TotalSteps: 1,
			Steps: []fermentation.FermentationStep{
				{StepNumber: 1, Temperature: 18.0, DurationHours: 72, Description: "Primary", Type: "Ferment"},
			},
		},
	}
	mockStore := &MockFermentationStore{
		ListPlansFunc: func() ([]fermentation.FermentationPlan, error) {
			return expectedPlans, nil
		},
	}
	server := NewServer(log, nil, mockStore)

	req, err := http.NewRequest("GET", "/api/fermentation/plans", nil)
	assert.NoError(t, err)

	rr := httptest.NewRecorder()
	handler := server.Router()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var actualPlans []fermentation.FermentationPlan
	err = json.Unmarshal(rr.Body.Bytes(), &actualPlans)
	assert.NoError(t, err)
	assert.Equal(t, expectedPlans, actualPlans)
}

func TestStartFermentationEndpoint(t *testing.T) {
	log := logger.New()
	mockStore := &MockFermentationStore{
		StartFermentationFunc: func(planID int64, tankID string) (int64, error) {
			assert.Equal(t, int64(123), planID)
			assert.Equal(t, "TANK_ALPHA_001", tankID)
			return 789, nil // Simulate a new fermentation ID
		},
	}
	server := NewServer(log, nil, mockStore)

	reqBody := StartFermentationRequest{
		PlanID: 123,
		TankID: "TANK_ALPHA_001",
	}
	body, _ := json.Marshal(reqBody)

	req, err := http.NewRequest("POST", "/api/fermentation/start", bytes.NewReader(body))
	assert.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler := server.Router()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var responseMap map[string]interface{}
	err = json.Unmarshal(rr.Body.Bytes(), &responseMap)
	assert.NoError(t, err)
	assert.Equal(t, "ok", responseMap["status"])
	assert.Equal(t, float64(789), responseMap["fermentationID"])
}

func TestListTanksEndpoint(t *testing.T) {
	log := logger.New()
	server := NewServer(log, nil, &MockFermentationStore{}) // FermentationStore is not directly used by this handler

	req, err := http.NewRequest("GET", "/api/tanks", nil)
	assert.NoError(t, err)

	rr := httptest.NewRecorder()
	handler := server.Router()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	expectedTanks := []string{"TANK_ALPHA_001", "TANK_BETA_002", "TANK_GAMMA_003"}
	var actualTanks []string
	err = json.Unmarshal(rr.Body.Bytes(), &actualTanks)
	assert.NoError(t, err)
	assert.Equal(t, expectedTanks, actualTanks)
}
