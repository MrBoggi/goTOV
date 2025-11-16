package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type WriteRequest struct {
	Tag   string      `json:"tag"`
	Value interface{} `json:"value"`
}

func (s *Server) handleWrite(w http.ResponseWriter, r *http.Request) {
	var req WriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Add namespace automatically if missing
	if !strings.HasPrefix(req.Tag, "ns=") {
		req.Tag = fmt.Sprintf("ns=4;s=%s", req.Tag)
	}

	s.log.Info().
		Str("tag", req.Tag).
		Interface("value", req.Value).
		Msg("Write request received")

	if err := s.client.WriteNodeValue(r.Context(), req.Tag, req.Value); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}
