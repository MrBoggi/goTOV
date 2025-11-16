package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/MrBoggi/goTOV/internal/fermentation"
)

type startFermentationRequest struct {
	BatchID   string                          `json:"batch_id"`
	TankNo    int                             `json:"tank_no"`
	StartStep int                             `json:"start_step"`
	Steps     []fermentation.FermentationStep `json:"steps"`
}

func (s *Server) handleStartFermentation(w http.ResponseWriter, r *http.Request) {
	var req startFermentationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}

	// ---- 1) Save plan ----
	plan := fermentation.FermentationPlan{
		Name:     "Imported Brewfather Plan",
		RecipeID: req.BatchID,
		Steps:    req.Steps,
	}
	planID, err := s.store.SavePlan(plan)
	if err != nil {
		http.Error(w, "failed to save plan: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// ---- 2) Save fermentation state ----
	now := time.Now()

	state := fermentation.FermentationState{
		BatchID:       req.BatchID,
		TankNo:        req.TankNo,
		PlanID:        planID,
		StartedAt:     now,
		StepStartedAt: now,
		Status:        fermentation.StatusRunning,
	}

	if err := s.store.SaveFermentationState(state); err != nil {
		http.Error(w, "failed to save fermentation state: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write([]byte("ok"))
}

// GET /api/fermentation/active
func (s *Server) handleGetActiveFermentation(w http.ResponseWriter, r *http.Request) {
	state, err := s.store.GetActiveFermentationState()
	if err != nil {
		http.Error(w, "no active fermentation: "+err.Error(), 404)
		return
	}

	_ = json.NewEncoder(w).Encode(state)
}
