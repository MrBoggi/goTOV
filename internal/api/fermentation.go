package api

import (
	"encoding/json"
	"net/http"

	"github.com/MrBoggi/goTOV/internal/fermentation"
)

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
