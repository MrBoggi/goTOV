package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"time"

	"github.com/MrBoggi/goTOV/internal/controlpath"
	"github.com/google/uuid"
)

func (s *Server) controlConnections() controlpath.Connections {
	if s.controlPathConnections != nil {
		return s.controlPathConnections()
	}
	c := controlpath.Connections{UA: "unknown", ADS: "unknown"}
	if s.client != nil {
		if !s.client.IsConnected() {
			c.UA = "disconnected"
		} else {
			// A connected client object alone may be stale. Require a recent local
			// subscription update and reject future timestamps after clock changes.
			now := time.Now()
			s.latestMu.RLock()
			for _, event := range s.latest {
				if recentControlSubscription(now, event.Timestamp) {
					c.UA = "connected"
					break
				}
			}
			s.latestMu.RUnlock()
		}
	}
	// The dedicated fermentation ADS transport is not connected to the app.
	return c
}
func recentControlSubscription(now time.Time, timestamp int64) bool {
	if timestamp <= 0 {
		return false
	}
	age := now.Sub(time.UnixMilli(timestamp))
	return age >= 0 && age <= 10*time.Second
}
func controlError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	body := map[string]any{"contract_version": 1, "error": code}
	if status == 503 {
		body["alert"] = map[string]string{"severity": "high", "code": "persistence_unavailable"}
	}
	_ = json.NewEncoder(w).Encode(body)
}
func (s *Server) getControlPath(w http.ResponseWriter, r *http.Request) {
	if s.controlPath == nil {
		controlError(w, 503, "persistence_unavailable")
		return
	}
	value, err := s.controlPath.Status(r.Context(), s.controlConnections())
	if err != nil {
		controlError(w, 503, "persistence_unavailable")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(value)
}
func parseControlRequest(r *http.Request) (controlpath.Request, error) {
	var result controlpath.Request
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return result, errMedia
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 8193))
	if err != nil || len(body) > 8192 {
		return result, errInvalid
	}
	d := json.NewDecoder(bytes.NewReader(body))
	tok, err := d.Token()
	if err != nil || tok != json.Delim('{') {
		return result, errInvalid
	}
	fields := map[string]json.RawMessage{}
	for d.More() {
		tok, err = d.Token()
		if err != nil {
			return result, errInvalid
		}
		name, ok := tok.(string)
		if !ok {
			return result, errInvalid
		}
		if _, exists := fields[name]; exists {
			return result, errInvalid
		}
		var raw json.RawMessage
		if d.Decode(&raw) != nil {
			return result, errInvalid
		}
		fields[name] = raw
	}
	if _, err = d.Token(); err != nil {
		return result, errInvalid
	}
	var extra any
	if err = d.Decode(&extra); err != io.EOF {
		return result, errInvalid
	}
	if len(fields) != 5 {
		return result, errInvalid
	}
	for _, name := range []string{"contract_version", "request_id", "expected_revision", "target_path", "confirmed"} {
		v, ok := fields[name]
		if !ok || string(v) == "null" {
			return result, errInvalid
		}
	}
	data, err := json.Marshal(fields)
	if err != nil || json.Unmarshal(data, &result) != nil {
		return result, errInvalid
	}
	id, err := uuid.Parse(result.ID)
	if err != nil || id.String() != result.ID || result.Version != 1 || result.Revision == "" || (result.Target != "ua" && result.Target != "ads") || !result.Confirmed {
		return result, errInvalid
	}
	return result, nil
}

var errMedia = errors.New("unsupported media type")
var errInvalid = errors.New("invalid request")

func (s *Server) postControlPath(w http.ResponseWriter, r *http.Request) {
	req, err := parseControlRequest(r)
	if err != nil {
		if errors.Is(err, errMedia) {
			controlError(w, 415, "unsupported_media_type")
		} else {
			controlError(w, 400, "invalid_request")
		}
		return
	}
	if s.controlPath == nil {
		controlError(w, 503, "persistence_unavailable")
		return
	}
	err = s.controlPath.Accept(r.Context(), req, s.controlConnections())
	if err != nil {
		switch {
		case errors.Is(err, controlpath.ErrInvalid):
			controlError(w, 400, "invalid_request")
		case errors.Is(err, controlpath.ErrConflict):
			controlError(w, 409, "idempotency_conflict")
		case errors.Is(err, controlpath.ErrRevision):
			controlError(w, 409, "revision_conflict")
		case errors.Is(err, controlpath.ErrUnavailable):
			controlError(w, 409, "path_unavailable")
		case errors.Is(err, controlpath.ErrInProgress):
			controlError(w, 409, "transition_in_progress")
		default:
			controlError(w, 503, "persistence_unavailable")
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{"operation_id": req.ID, "state": "pending"})
}
