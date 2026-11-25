package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MrBoggi/goTOV/internal/controlpath"
	"github.com/rs/zerolog"
	_ "modernc.org/sqlite"
)

func TestControlPathIntentAndReplay(t *testing.T) {
	db, e := sql.Open("sqlite", filepath.Join(t.TempDir(), "path.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	store, e := controlpath.New(context.Background(), db)
	if e != nil {
		t.Fatal(e)
	}
	server := NewServer(zerolog.New(io.Discard), nil, nil)
	server.SetControlPathStore(store)
	server.controlPathConnections = func(context.Context) controlpath.Connections {
		return controlpath.Connections{UA: "connected", ADS: "connected"}
	}
	router := server.Router()
	get := httptest.NewRecorder()
	router.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/fermentation/control-path", nil))
	var snap controlpath.Snapshot
	if e = json.Unmarshal(get.Body.Bytes(), &snap); e != nil {
		t.Fatal(e)
	}
	if get.Code != 200 || snap.Requested != "ua" || snap.Owner != nil {
		t.Fatalf("initial: %s", get.Body.String())
	}
	body := fmt.Sprintf(`{"contract_version":1,"request_id":"b037789b-ff3e-48d3-87ea-bbe3c19f8069","expected_revision":%q,"target_path":"ads","confirmed":true}`, snap.Revision)
	post := func(raw string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/fermentation/control-path", strings.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, r)
		return w
	}
	if w := post(body); w.Code != 202 {
		t.Fatalf("post %d %s", w.Code, w.Body.String())
	}
	server.controlPathConnections = func(context.Context) controlpath.Connections {
		t.Fatal("idempotent replay probed a transport")
		return controlpath.Connections{}
	}
	if w := post(body); w.Code != 202 {
		t.Fatalf("replay %d %s", w.Code, w.Body.String())
	}
	server.controlPathConnections = func(context.Context) controlpath.Connections {
		return controlpath.Connections{UA: "connected", ADS: "connected"}
	}
	get = httptest.NewRecorder()
	router.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/fermentation/control-path", nil))
	if e = json.Unmarshal(get.Body.Bytes(), &snap); e != nil {
		t.Fatal(e)
	}
	if snap.Requested != "ads" || snap.Owner != nil || snap.Confirmed != nil || snap.Transition == nil || snap.Transition.State != "unknown" || snap.Alert == nil || snap.Alert.Severity != "high" {
		t.Fatalf("after %s", get.Body.String())
	}
	if w := post(strings.Replace(body, `"target_path":"ads"`, `"target_path":"ua"`, 1)); w.Code != 409 || !bytes.Contains(w.Body.Bytes(), []byte("idempotency_conflict")) {
		t.Fatalf("conflict %d %s", w.Code, w.Body.String())
	}
	if w := post(strings.Replace(body, `"confirmed":true`, `"confirmed":true,"confirmed":true`, 1)); w.Code != 400 {
		t.Fatalf("duplicate accepted: %s", w.Body.String())
	}
}
func TestControlPathUnavailablePersistenceIsHighSeverity(t *testing.T) {
	s := NewServer(zerolog.New(io.Discard), nil, nil)
	w := httptest.NewRecorder()
	s.Router().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/fermentation/control-path", nil))
	if w.Code != 503 || !strings.Contains(w.Body.String(), `"severity":"high"`) {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestControlPathRejectsInvalidRequestsAndUnavailableTarget(t *testing.T) {
	db, e := sql.Open("sqlite", filepath.Join(t.TempDir(), "path.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	store, e := controlpath.New(context.Background(), db)
	if e != nil {
		t.Fatal(e)
	}
	s := NewServer(zerolog.New(io.Discard), nil, nil)
	s.SetControlPathStore(store)
	s.controlPathConnections = func(context.Context) controlpath.Connections {
		return controlpath.Connections{UA: "connected", ADS: "unknown"}
	}
	router := s.Router()
	snap, e := store.Status(context.Background(), s.controlConnections(context.Background()))
	if e != nil {
		t.Fatal(e)
	}
	valid := fmt.Sprintf(`{"contract_version":1,"request_id":"b037789b-ff3e-48d3-87ea-bbe3c19f8069","expected_revision":%q,"target_path":"ads","confirmed":true}`, snap.Revision)
	for _, tc := range []struct {
		body, content string
		status        int
		code          string
	}{
		{valid, "application/json", 409, "path_unavailable"},
		{strings.Replace(valid, `"confirmed":true`, `"confirmed":false`, 1), "application/json", 400, "invalid_request"},
		{strings.Replace(valid, `"contract_version":1`, `"contract_version":2`, 1), "application/json", 400, "invalid_request"},
		{strings.Replace(valid, `"confirmed":true`, `"confirmed":true,"other":1`, 1), "application/json", 400, "invalid_request"},
		{valid, "text/plain", 415, "unsupported_media_type"},
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/fermentation/control-path", strings.NewReader(tc.body))
		r.Header.Set("Content-Type", tc.content)
		router.ServeHTTP(w, r)
		if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.code) {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
	}
	after, e := store.Status(context.Background(), s.controlConnections(context.Background()))
	if e != nil || after.Revision != snap.Revision || after.Transition != nil {
		t.Fatalf("rejection mutated intent: %+v %v", after, e)
	}
}
