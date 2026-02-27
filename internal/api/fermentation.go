package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/MrBoggi/goTOV/internal/fermentation"
	"github.com/go-chi/chi/v5"
)

type StartFermentationRequest struct {
	PlanID  int64  `json:"planID"`
	TankID  string `json:"tankID"`
	BatchID string `json:"batchID"`
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

	fermentationID, err := s.fermentationStore.StartFermentation(req.PlanID, req.TankID, req.BatchID)
	if err != nil {
		s.log.Error().Err(err).Msg("failed to start fermentation in store")
		http.Error(w, "failed to start fermentation", http.StatusInternalServerError)
		return
	}

	// Load the state we just created
	if s.engine != nil {
		active, err := s.fermentationStore.ListActiveFermentations()
		if err == nil {
			for _, state := range active {
				if state.ID == fermentationID {
					s.engine.AddFermentation(&state)
					break
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":         "ok",
		"fermentationID": fermentationID,
	})
}

func (s *Server) handleDeleteFermentationPlan(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid ID format", http.StatusBadRequest)
		return
	}

	err = s.fermentationStore.DeletePlan(id)
	if err != nil {
		if errors.Is(err, fermentation.ErrPlanNotFound) {
			http.Error(w, "plan not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, fermentation.ErrPlanInUse) {
			http.Error(w, "cannot delete plan while in use by an active fermentation", http.StatusConflict)
			return
		}
		s.log.Error().Err(err).Int64("id", id).Msg("failed to delete fermentation plan")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetFermentationStatus(w http.ResponseWriter, r *http.Request) {
	active, err := s.fermentationStore.ListActiveFermentations()
	if err != nil {
		s.log.Error().Err(err).Msg("failed to list active fermentations")
		http.Error(w, "failed to list active fermentations", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(active)
}

func (s *Server) handleGetApiDocs(w http.ResponseWriter, r *http.Request) {
	docs := map[string]interface{}{
		"title": "goTOV Fermentation API",
		"endpoints": []map[string]string{
			{"method": "GET", "path": "/api/fermentation/plans", "description": "List all fermentation plans"},
			{"method": "POST", "path": "/api/fermentation/plan", "description": "Save a new fermentation plan"},
			{"method": "POST", "path": "/api/fermentation/start", "description": "Start a fermentation process (requires planID, tankID)"},
			{"method": "GET", "path": "/api/fermentation/status", "description": "Get status of all active fermentations"},
			{"method": "GET", "path": "/api/fermentation/docs", "description": "This documentation"},
			{"method": "DELETE", "path": "/api/fermentation/plan/{id}", "description": "Delete a fermentation plan (if not in use)"},
			{"method": "GET", "path": "/api/tanks", "description": "List available tanks"},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(docs)
}

func (s *Server) handleListTanks(w http.ResponseWriter, r *http.Request) {
	// Dynamically list tanks 1 and 2
	tanks := []map[string]string{
		{"id": "1", "name": "Tank 1 (Fermenter 1)"},
		{"id": "2", "name": "Tank 2 (Fermenter 2)"},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(tanks)
}
