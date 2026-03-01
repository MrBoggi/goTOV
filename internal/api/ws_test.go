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
	SavePlanFunc                func(plan fermentation.FermentationPlan) (int64, error)
	GetPlanFunc                 func(id int64) (fermentation.FermentationPlan, error)
	GetStepsFunc                func(planID int64) ([]fermentation.FermentationStep, error)
	ListPlansFunc               func() ([]fermentation.FermentationPlan, error)
	ListStepsFunc               func(planID int64) ([]fermentation.FermentationStep, error)
	ClearFunc                   func() error
	DeletePlanFunc              func(id int64) error
	StartFermentationFunc       func(planID int64, tankID string, batchID string) (int64, error)
	ListActiveFermentationsFunc func() ([]fermentation.FermentationState, error)
	GetStateFunc                func(id int64) (fermentation.FermentationState, error)
	GetStateByTankFunc          func(tankID string) (fermentation.FermentationState, error)
	UpdateStateFunc             func(state fermentation.FermentationState) error
	StopFermentationFunc        func(id int64) error
	LogDataFunc                 func(planID int64, tankID string, batchID string, temp float32, target float32, valve bool, jacket bool) error
	GetHistoryFunc              func(planID int64, hours float64) ([]fermentation.FermentationHistoryEntry, error)
	LogGlycolDataFunc           func(temp float64, pressure *float64, load float64) error
	GetGlycolHistoryFunc        func(hours float64) ([]fermentation.GlycolHistoryData, error)
}

func (m *MockFermentationStore) SavePlan(plan fermentation.FermentationPlan) (int64, error) {
	if m.SavePlanFunc != nil {
		return m.SavePlanFunc(plan)
	}
	return 1, nil
}

func (m *MockFermentationStore) GetPlan(id int64) (fermentation.FermentationPlan, error) {
	if m.GetPlanFunc != nil {
		return m.GetPlanFunc(id)
	}
	return fermentation.FermentationPlan{ID: id}, nil
}

func (m *MockFermentationStore) GetSteps(planID int64) ([]fermentation.FermentationStep, error) {
	return m.ListSteps(planID)
}

func (m *MockFermentationStore) ListSteps(planID int64) ([]fermentation.FermentationStep, error) {
	if m.ListStepsFunc != nil {
		return m.ListStepsFunc(planID)
	}
	return nil, nil
}

func (m *MockFermentationStore) ListPlans() ([]fermentation.FermentationPlan, error) {
	if m.ListPlansFunc != nil {
		return m.ListPlansFunc()
	}
	return nil, nil
}

func (m *MockFermentationStore) StartFermentation(planID int64, tankID string, batchID string) (int64, error) {
	if m.StartFermentationFunc != nil {
		return m.StartFermentationFunc(planID, tankID, batchID)
	}
	return 1, nil
}

func (m *MockFermentationStore) ListActiveFermentations() ([]fermentation.FermentationState, error) {
	if m.ListActiveFermentationsFunc != nil {
		return m.ListActiveFermentationsFunc()
	}
	return nil, nil
}

func (m *MockFermentationStore) GetState(id int64) (fermentation.FermentationState, error) {
	if m.GetStateFunc != nil {
		return m.GetStateFunc(id)
	}
	return fermentation.FermentationState{ID: id}, nil
}

func (m *MockFermentationStore) GetStateByTank(tankID string) (fermentation.FermentationState, error) {
	if m.GetStateByTankFunc != nil {
		return m.GetStateByTankFunc(tankID)
	}
	return fermentation.FermentationState{TankID: tankID}, nil
}

func (m *MockFermentationStore) UpdateState(state fermentation.FermentationState) error {
	if m.UpdateStateFunc != nil {
		return m.UpdateStateFunc(state)
	}
	return nil
}

func (m *MockFermentationStore) StopFermentation(id int64) error {
	if m.StopFermentationFunc != nil {
		return m.StopFermentationFunc(id)
	}
	return nil
}

func (m *MockFermentationStore) LogData(planID int64, tankID string, batchID string, temp float32, target float32, valve bool, jacket bool) error {
	if m.LogDataFunc != nil {
		return m.LogDataFunc(planID, tankID, batchID, temp, target, valve, jacket)
	}
	return nil
}

func (m *MockFermentationStore) GetHistory(planID int64, hours float64) ([]fermentation.FermentationHistoryEntry, error) {
	if m.GetHistoryFunc != nil {
		return m.GetHistoryFunc(planID, hours)
	}
	return nil, nil
}

func (m *MockFermentationStore) LogGlycolData(temp float64, pressure *float64, load float64) error {
	if m.LogGlycolDataFunc != nil {
		return m.LogGlycolDataFunc(temp, pressure, load)
	}
	return nil
}

func (m *MockFermentationStore) GetGlycolHistory(hours float64) ([]fermentation.GlycolHistoryData, error) {
	if m.GetGlycolHistoryFunc != nil {
		return m.GetGlycolHistoryFunc(hours)
	}
	return nil, nil
}

func (m *MockFermentationStore) Clear() error {
	if m.ClearFunc != nil {
		return m.ClearFunc()
	}
	return nil
}

func (m *MockFermentationStore) DeletePlan(id int64) error {
	if m.DeletePlanFunc != nil {
		return m.DeletePlanFunc(id)
	}
	return nil
}

func (m *MockFermentationStore) Close() error { return nil }

func TestHealthCheckEndpoint(t *testing.T) {
	log := logger.New()
	server := NewServer(log, nil, &MockFermentationStore{}, nil, nil, nil)

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
			return 42, nil
		},
	}
	server := NewServer(log, nil, mockStore, nil, nil, nil)

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
	assert.Equal(t, float64(42), responseMap["planId"])
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
	}
	mockStore := &MockFermentationStore{
		ListPlansFunc: func() ([]fermentation.FermentationPlan, error) {
			return expectedPlans, nil
		},
	}
	server := NewServer(log, nil, mockStore, nil, nil, nil)

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
		StartFermentationFunc: func(planID int64, tankID string, batchID string) (int64, error) {
			return 789, nil
		},
		ListActiveFermentationsFunc: func() ([]fermentation.FermentationState, error) {
			return []fermentation.FermentationState{{ID: 789}}, nil
		},
	}
	server := NewServer(log, nil, mockStore, nil, nil, nil)

	reqBody := StartFermentationRequest{
		PlanID: 123,
		TankID: "1",
	}
	body, _ := json.Marshal(reqBody)

	req, err := http.NewRequest("POST", "/api/fermentation/start", bytes.NewReader(body))
	assert.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler := server.Router()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestListTanksEndpoint(t *testing.T) {
	log := logger.New()
	server := NewServer(log, nil, &MockFermentationStore{}, nil, nil, nil)

	req, err := http.NewRequest("GET", "/api/tanks", nil)
	assert.NoError(t, err)

	rr := httptest.NewRecorder()
	handler := server.Router()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var actualTanks []map[string]string
	err = json.Unmarshal(rr.Body.Bytes(), &actualTanks)
	assert.NoError(t, err)
	assert.Len(t, actualTanks, 2)
}

func TestDeleteFermentationPlanEndpoint(t *testing.T) {
	log := logger.New()

	t.Run("Successful Deletion", func(t *testing.T) {
		mockStore := &MockFermentationStore{
			DeletePlanFunc: func(id int64) error {
				assert.Equal(t, int64(123), id)
				return nil
			},
		}
		server := NewServer(log, nil, mockStore, nil, nil, nil)
		req, _ := http.NewRequest("DELETE", "/api/fermentation/plan/123", nil)
		rr := httptest.NewRecorder()
		server.Router().ServeHTTP(rr, req)
		assert.Equal(t, http.StatusNoContent, rr.Code)
	})

	t.Run("Plan In Use Conflict", func(t *testing.T) {
		mockStore := &MockFermentationStore{
			DeletePlanFunc: func(id int64) error {
				return fermentation.ErrPlanInUse
			},
		}
		server := NewServer(log, nil, mockStore, nil, nil, nil)
		req, _ := http.NewRequest("DELETE", "/api/fermentation/plan/123", nil)
		rr := httptest.NewRecorder()
		server.Router().ServeHTTP(rr, req)
		assert.Equal(t, http.StatusConflict, rr.Code)
	})

	t.Run("Plan Not Found", func(t *testing.T) {
		mockStore := &MockFermentationStore{
			DeletePlanFunc: func(id int64) error {
				return fermentation.ErrPlanNotFound
			},
		}
		server := NewServer(log, nil, mockStore, nil, nil, nil)
		req, _ := http.NewRequest("DELETE", "/api/fermentation/plan/999", nil)
		rr := httptest.NewRecorder()
		server.Router().ServeHTTP(rr, req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("Invalid ID Format", func(t *testing.T) {
		server := NewServer(log, nil, &MockFermentationStore{}, nil, nil, nil)
		req, _ := http.NewRequest("DELETE", "/api/fermentation/plan/abc", nil)
		rr := httptest.NewRecorder()
		server.Router().ServeHTTP(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})
}
