package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/MrBoggi/goTOV/internal/fermentation"
)

func (s *Server) handleGetGlycolStatus(w http.ResponseWriter, r *http.Request) {
	var status fermentation.GlycolStatus

	// Read current temperature from OPC UA or use 0
	if s.client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		val, err := s.client.ReadNodeValue(ctx, "ns=4;s=MAIN.fbUA.glykolkjolerTemp")
		if err == nil {
			switch v := val.(type) {
			case float32:
				status.Temperature = float64(v)
			case float64:
				status.Temperature = v
			}
		} else {
			s.log.Warn().Err(err).Msg("failed to read glycol temp for API")
		}
	}

	// Set static values for now as per instructions
	status.Pressure = nil

	// Read load percentage from engine (calculated duty cycle over last 1 min)
	status.LoadPercentage = 0.0
	if s.engine != nil {
		status.LoadPercentage = s.engine.GetGlycolLoad()
	}

	// Fetch history from the last 24 hours
	history, err := s.fermentationStore.GetGlycolHistory(24.0)
	if err != nil {
		s.log.Error().Err(err).Msg("failed to fetch glycol history for API")
		// Don't error out completely, just leave trend empty
	} else {
		status.Trend24h = history
	}

	// Ensure array is empty json array not null if there's no history
	if status.Trend24h == nil {
		status.Trend24h = []fermentation.GlycolHistoryData{}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(status)
}
