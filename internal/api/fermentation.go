package api

import (
	"encoding/json"
	"net/http"

	"github.com/MrBoggi/goTOV/internal/fermentation"
)

type StartFermentationRequest struct {
	PlanID int64  `json:"planID"`
	TankID string `json:"tankID"`
}

func (s *Server) handleSaveFermentationPlan(w http.ResponseWriter, r *http.Request) {
	var plan fermentation.FermentationPlan
	if err := json.NewDecoder(r.Body).Decode(&plan); err != nil {
		s.log.Error().Err(err).Msg("failed to decode fermentation plan")
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	planID, err := s.fermentationStore.SavePlan(plan)
	if err != nil {
		s.log.Error().Err(err).Msg("failed to save fermentation plan")
		http.Error(w, "failed to save fermentation plan", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"planID": planID,
	})
}

func (s *Server) handleListFermentationPlans(w http.ResponseWriter, r *http.Request) {
	plans, err := s.fermentationStore.ListPlans()
	if err != nil {
		s.log.Error().Err(err).Msg("failed to list fermentation plans")
		http.Error(w, "failed to list fermentation plans", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(plans)
}

func (s *Server) handleStartFermentation(w http.ResponseWriter, r *http.Request) {
	var req StartFermentationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.log.Error().Err(err).Msg("failed to decode start fermentation request")
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	fermentationID, err := s.fermentationStore.StartFermentation(req.PlanID, req.TankID)
	if err != nil {
		s.log.Error().Err(err).Msg("failed to start fermentation")
		http.Error(w, "failed to start fermentation", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":         "ok",
		"fermentationID": fermentationID,
	})
}

func (s *Server) handleListTanks(w http.ResponseWriter, r *http.Request) {
	tanks := []string{"TANK_ALPHA_001", "TANK_BETA_002", "TANK_GAMMA_003"} // Hardcoded list of tank IDs

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(tanks)
}
