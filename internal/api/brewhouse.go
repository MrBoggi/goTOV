package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/MrBoggi/goTOV/internal/brewhouse"
)

func (s *Server) handleGetBrewhouseState(w http.ResponseWriter, r *http.Request) {
	state := s.brewhouseEngine.GetStateSnapshot()
	if state == nil {
		http.Error(w, "Brewhouse state not initialized", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(state); err != nil {
		http.Error(w, "Failed to encode state", http.StatusInternalServerError)
	}
}

func (s *Server) handleUpdateBrewhouseState(w http.ResponseWriter, r *http.Request) {
	var newState brewhouse.BrewhouseState
	if err := json.NewDecoder(r.Body).Decode(&newState); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	if err := s.brewhouseEngine.UpdateState(&newState); err != nil {
		s.log.Error().Err(err).Msg("Failed to update brewhouse state")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(newState); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

func (s *Server) broadcastBrewhouseState() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		state := s.brewhouseEngine.GetStateSnapshot()
		if state == nil {
			continue
		}

		msg := WSMessage{
			Tag:         "BREWHOUSE_STATE",
			DisplayName: "Brewhouse State",
			Value:       state,
			ValueType:   "BrewhouseState",
			Timestamp:   time.Now().UnixMilli(),
		}

		s.latestMu.Lock()
		s.latest[msg.Tag] = msg
		s.latestMu.Unlock()

		s.broadcast(msg)
	}
}
