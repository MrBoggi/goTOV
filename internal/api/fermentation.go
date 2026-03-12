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

type CompleteFermentationEventRequest struct {
	FermentationID int64 `json:"fermentationId"`
	EventIndex     int   `json:"eventIndex"`
}

type SetFermentationModeRequest struct {
	ID   int64  `json:"id"`
	Mode string `json:"mode"`
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
		"planId": planID,
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
		if errors.Is(err, fermentation.ErrTankBusy) {
			http.Error(w, "tank already has an active fermentation running — stop it first", http.StatusConflict)
			return
		}
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
		"fermentationId": fermentationID,
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

func (s *Server) handleCompleteFermentationEvent(w http.ResponseWriter, r *http.Request) {
	var req CompleteFermentationEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.log.Error().Err(err).Msg("failed to decode complete event request")
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if err := s.fermentationStore.CompleteEvent(req.FermentationID, req.EventIndex); err != nil {
		if errors.Is(err, fermentation.ErrEventNotFound) {
			http.Error(w, "event not found", http.StatusNotFound)
			return
		}
		s.log.Error().Err(err).
			Int64("fermentationId", req.FermentationID).
			Int("eventIndex", req.EventIndex).
			Msg("failed to complete fermentation event")
		http.Error(w, "failed to complete event", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok"})
}

func (s *Server) handleSetFermentationMode(w http.ResponseWriter, r *http.Request) {
	var req SetFermentationModeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.log.Error().Err(err).Msg("failed to decode set fermentation mode request")
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Mode != "Auto" && req.Mode != "Manual" {
		http.Error(w, "mode must be Auto or Manual", http.StatusBadRequest)
		return
	}

	state, err := s.fermentationStore.GetState(req.ID)
	if err != nil {
		s.log.Error().Err(err).Int64("id", req.ID).Msg("failed to get fermentation state for mode change")
		http.Error(w, "fermentation not found", http.StatusNotFound)
		return
	}

	state.Mode = req.Mode
	if err := s.fermentationStore.UpdateState(state); err != nil {
		s.log.Error().Err(err).Int64("id", req.ID).Msg("failed to update fermentation mode")
		http.Error(w, "failed to update mode", http.StatusInternalServerError)
		return
	}

	// Update in engine if active
	if s.engine != nil {
		// Just pull from engine to modify, then we don't need a formal method if it's already there or we can just update the DB and let the engine read it?
		// Engine stores state pointers, so we could theoretically modify the pointer directly, but we don't have direct access in api.
		// Since we updated the DB, next processOne would use old state pointer because engine holds it in memory.
		// Wait, Engine needs a method to update the mode in memory.
		s.engine.UpdateFermentationMode(req.ID, req.Mode)
	}

	s.log.Info().Int64("id", req.ID).Str("mode", req.Mode).Msg("🔄 Fermentation mode updated")

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
				"error":       "Plan is currently in use by active fermentations",
				"activeTanks": tanks,
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

		// Fetch active events for the UI
		events, err := s.fermentationStore.GetActiveEvents(active[i].ID)
		if err == nil {
			active[i].Events = events
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(active)
}

func (s *Server) handleGetApiDocs(w http.ResponseWriter, r *http.Request) {
	docs := map[string]interface{}{
		"title": "goTOV API Documentation",
		"endpoints": []map[string]interface{}{
			// Fermentation
			{"method": "GET", "path": "/api/fermentation/plans", "description": "List all fermentation plans"},
			{"method": "POST", "path": "/api/fermentation/plan", "description": "Save a new fermentation plan"},
			{"method": "POST", "path": "/api/fermentation/start", "description": "Start a fermentation (requires planId, tankId, batchId)"},
			{"method": "POST", "path": "/api/fermentation/stop", "description": "Stop a fermentation (requires id or tankId)"},
			{"method": "POST", "path": "/api/fermentation/event/complete", "description": "Mark a fermentation event as completed (requires fermentationId, eventIndex)"},
			{"method": "GET", "path": "/api/fermentation/status", "description": "Get status of all active fermentations"},
			{"method": "DELETE", "path": "/api/fermentation/plan/{id}", "description": "Delete a plan. Use ?force=true to stop active ones"},

			// Brewing process (New)
			{"method": "POST", "path": "/api/brewing/config/pid", "description": "Save PID configuration for a tank (BK/MLT)"},
			{"method": "GET", "path": "/api/brewing/history", "description": "Get brewing history (BK/MLT temps and padraag)"},
			{"method": "GET", "path": "/api/brewing/status", "description": "Get current brewing session status"},

			// General/Infrastructure
			{"method": "GET", "path": "/api/tags", "description": "Get snapshot of all current PLC tag values"},
			{"method": "POST", "path": "/api/write", "description": "Directly write to a PLC tag (requires tag and value)"},
			{"method": "GET", "path": "/api/tanks", "description": "List available fermentation tanks"},
			{"method": "GET", "path": "/api/glycol/status", "description": "Get current glycol unit load and temperature"},
			{"method": "GET", "path": "/api/stream/tags", "description": "WebSocket endpoint for real-time updates (JSON messages)"},
		},
		"ws_topics": []map[string]string{
			{"topic": "BREWHOUSE_STATE", "description": "Pushed every 1s with complete brewhouse state"},
			{"topic": "any other tag", "description": "Pushed on change for monitored PLC tags"},
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
		s.log.Error().Err(err).Int64("fermentationId", id).Msg("failed to fetch fermentation history")
		http.Error(w, "failed to fetch history", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(history)
}
