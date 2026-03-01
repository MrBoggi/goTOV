package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/MrBoggi/goTOV/internal/brew"
)

type StartBrewingRequest struct {
	Type    string `json:"type"` // "manual" or "brewfather"
	BatchID string `json:"batchId,omitempty"`
}

func (s *Server) handleListBrewfatherBatches(w http.ResponseWriter, r *http.Request) {
	if s.brewfatherClient == nil {
		http.Error(w, "Brewfather client not configured", http.StatusServiceUnavailable)
		return
	}

	batches, err := s.brewfatherClient.FetchBatches()
	if err != nil {
		s.log.Error().Err(err).Msg("Failed to fetch batches from Brewfather")
		http.Error(w, "Failed to fetch batches", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(batches); err != nil {
		s.log.Error().Err(err).Msg("Failed to encode batches")
	}
}

func (s *Server) handleStartBrewing(w http.ResponseWriter, r *http.Request) {
	var req StartBrewingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	session := &brew.BrewingSession{
		ID:        "session-" + time.Now().Format("20060102150405"),
		StartTime: time.Now(),
		Status:    req.Type,
		BatchID:   req.BatchID,
	}

	if req.Type == "manual" {
		session.RecipeName = "Manual Brew"
	} else if req.Type == "brewfather" {
		// Fetch batch details if needed
		session.RecipeName = "Brewfather Batch " + req.BatchID
	}

	s.brewingEngine.StartSession(session)

	// Persist to DB
	data, _ := json.Marshal(session)
	_ = s.brewhouseStore.SaveBrewingSession(string(data))

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(session)
}

func (s *Server) handleStopBrewing(w http.ResponseWriter, r *http.Request) {
	session := s.brewingEngine.GetSession()
	if session == nil {
		http.Error(w, "No active session", http.StatusNotFound)
		return
	}

	// Log to DB
	data, _ := json.Marshal(session)
	_ = s.brewhouseStore.LogBrewingSession(string(data))

	// Clear active session in DB
	_ = s.brewhouseStore.SaveBrewingSession("")

	s.brewingEngine.StopSession()
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleGetBrewingStatus(w http.ResponseWriter, r *http.Request) {
	session := s.brewingEngine.GetSession()
	if session == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(session)
}

func (s *Server) broadcastBrewingStatus() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		session := s.brewingEngine.GetSession()
		if session == nil {
			continue
		}

		msg := WSMessage{
			Tag:         "BREWING_STATUS",
			DisplayName: "Brewing Status",
			Value:       session,
			ValueType:   "BrewingSession",
			Timestamp:   time.Now().UnixMilli(),
		}

		s.latestMu.Lock()
		s.latest[msg.Tag] = msg
		s.latestMu.Unlock()

		s.broadcast(msg)
	}
}
