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
	PlanID  int64  `json:"planId"`
	TankID  string `json:"tankId"`
	BatchID string `json:"batchId"`
}

type StopFermentationRequest struct {
	ID     int64  `json:"id"`
	TankID string `json:"tankId"`
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

	s.log.Debug().
		Int64("planId", req.PlanID).
		Str("tankId", req.TankID).
		Str("batchId", req.BatchID).
		Msg("🚀 Received start fermentation request")

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

func (s *Server) handleStopFermentation(w http.ResponseWriter, r *http.Request) {
	var req StopFermentationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.log.Error().Err(err).Msg("failed to decode stop fermentation request")
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	var err error
	if req.ID != 0 {
		err = s.engine.StopFermentation(req.ID)
	} else if req.TankID != "" {
		err = s.engine.StopByTank(req.TankID)
	} else {
		http.Error(w, "missing either id or tankID", http.StatusBadRequest)
		return
	}

	if err != nil {
		if errors.Is(err, fermentation.ErrFermentationNotFound) {
			http.Error(w, "no active fermentation found", http.StatusNotFound)
			return
		}
		s.log.Error().Err(err).Msg("failed to stop fermentation")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok"})
}

func (s *Server) handleDeleteFermentationPlan(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid ID format", http.StatusBadRequest)
		return
	}

	// If force=true, stop all active fermentations using this plan first
	if r.URL.Query().Get("force") == "true" {
		s.log.Info().Int64("planID", id).Msg("force-delete: stopping active fermentations for plan")
		active, listErr := s.fermentationStore.ListActiveFermentations()
		if listErr != nil {
			s.log.Error().Err(listErr).Int64("planID", id).Msg("force-delete: failed to list active fermentations")
		} else {
			s.log.Info().Int64("planID", id).Int("activeCount", len(active)).Msg("force-delete: found active fermentations")
			for _, a := range active {
				if a.PlanID == id {
					// Stop in store (DB) first — this is the source of truth for DeletePlan
					if stopErr := s.fermentationStore.StopFermentation(a.ID); stopErr != nil {
						s.log.Error().Err(stopErr).Int64("fermentationID", a.ID).Msg("force-delete: failed to stop fermentation in store")
					} else {
						s.log.Info().Int64("fermentationID", a.ID).Str("tank", a.TankID).Msg("force-delete: stopped fermentation in store")
					}
					// Also remove from engine's in-memory map if engine is running
					if s.engine != nil {
						s.engine.RemoveFermentation(a.ID)
					}
				}
			}
		}
	}

	err = s.fermentationStore.DeletePlan(id)
	if err != nil {
		if errors.Is(err, fermentation.ErrPlanNotFound) {
			http.Error(w, "plan not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, fermentation.ErrPlanInUse) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)

			active, listErr := s.fermentationStore.ListActiveFermentations()
			var tanks []string
			if listErr == nil {
				for _, a := range active {
					if a.PlanID == id {
						tanks = append(tanks, a.TankID)
					}
				}
			}

			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error":        "Plan is currently in use by active fermentations",
				"active_tanks": tanks,
			})
			return
		}
		s.log.Error().Err(err).Int64("id", id).Msg("failed to delete fermentation plan")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetFermentationStatus(w http.ResponseWriter, r *http.Request) {
	var active []fermentation.FermentationState
	var err error

	// Prefer engine states (includes computed fields like transitioning)
	if s.engine != nil {
		active = s.engine.GetActiveStates()
	} else {
		active, err = s.fermentationStore.ListActiveFermentations()
		if err != nil {
			s.log.Error().Err(err).Msg("failed to list active fermentations")
			http.Error(w, "failed to list active fermentations", http.StatusInternalServerError)
			return
		}
	}

	// Enrich with Plan details for the UI
	for i := range active {
		plan, err := s.fermentationStore.GetPlan(active[i].PlanID)
		if err == nil {
			active[i].PlanName = plan.Name
		}
		steps, err := s.fermentationStore.GetSteps(active[i].PlanID)
		if err == nil && active[i].StepIndex < len(steps) {
			active[i].StepDuration = steps[active[i].StepIndex].DurationHours
		}
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
			{"method": "POST", "path": "/api/fermentation/stop", "description": "Stop an active fermentation (requires tankID or id)"},
			{"method": "GET", "path": "/api/fermentation/status", "description": "Get status of all active fermentations"},
			{"method": "GET", "path": "/api/fermentation/docs", "description": "This documentation"},
			{"method": "DELETE", "path": "/api/fermentation/plan/{id}", "description": "Delete a fermentation plan. Use ?force=true to stop active fermentations first"},
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

func (s *Server) handleGetFermentationHistory(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid ID format", http.StatusBadRequest)
		return
	}

	hoursStr := r.URL.Query().Get("hours")
	hours := 24.0 // default to 24 hours
	if hoursStr != "" {
		if h, err := strconv.ParseFloat(hoursStr, 64); err == nil {
			hours = h
		}
	}

	history, err := s.fermentationStore.GetHistory(id, hours)
	if err != nil {
		s.log.Error().Err(err).Int64("planID", id).Msg("failed to fetch fermentation history")
		http.Error(w, "failed to fetch history", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(history)
}
