package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/MrBoggi/goTOV/internal/brewfather"
)

func (s *Server) handleListBatches(w http.ResponseWriter, r *http.Request) {
	client := brewfather.NewClient(
		s.cfg.Brewfather.UserID,
		s.cfg.Brewfather.APIKey,
	)

	batches, err := client.FetchBatches()
	if err != nil {
		http.Error(w, "failed to fetch brewfather batches: "+err.Error(), 502)
		return
	}

	_ = json.NewEncoder(w).Encode(batches)
}

func (s *Server) handleGetBatch(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	client := brewfather.NewClient(
		s.cfg.Brewfather.UserID,
		s.cfg.Brewfather.APIKey,
	)

	batch, err := client.FetchBatch(id)
	if err != nil {
		http.Error(w, "failed to fetch brewfather batch: "+err.Error(), 502)
		return
	}

	_ = json.NewEncoder(w).Encode(batch)
}
