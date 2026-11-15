package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"

	"github.com/rs/zerolog"
)

func TestFermentationControlStatusUnavailableIsExplicit(t *testing.T) {
	s := NewServer(zerolog.New(io.Discard), nil, nil)
	r := httptest.NewRequest(http.MethodGet, "/api/fermentation/control-status", nil)
	w := httptest.NewRecorder()
	s.Router().ServeHTTP(w, r)
	if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("status=%d headers=%v", w.Code, w.Header())
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("testdata/fermentation-control-status-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors struct {
		Contract string `json:"contract"`
		Vectors  []struct {
			Name     string         `json:"name"`
			Response map[string]any `json:"response"`
		} `json:"vectors"`
	}
	if err = json.Unmarshal(raw, &vectors); err != nil {
		t.Fatal(err)
	}
	if vectors.Contract != "fermentation-control-status-v1" || len(vectors.Vectors) != 1 || vectors.Vectors[0].Name != "ua-default-ownership-unverified" {
		t.Fatal("invalid contract fixture")
	}
	want := vectors.Vectors[0].Response
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("status = %#v", got)
	}
}

func TestFermentationControlStatusHasNoMutation(t *testing.T) {
	s := NewServer(zerolog.New(io.Discard), nil, nil)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/fermentation/control-status", nil)
	s.Router().ServeHTTP(w, r)
	if w.Code >= 200 && w.Code < 300 {
		t.Fatalf("unexpected successful mutation: %d", w.Code)
	}
}
