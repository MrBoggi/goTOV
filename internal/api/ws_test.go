package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MrBoggi/goTOV/internal/logger"
	"github.com/stretchr/testify/assert"
)

func TestHealthCheckEndpoint(t *testing.T) {
	log := logger.New()
	server := NewServer(log, nil) // OPCUA client is not needed for this test

	req, err := http.NewRequest("GET", "/health", nil)
	assert.NoError(t, err)

	rr := httptest.NewRecorder()
	handler := server.Router()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	expected := `{"status":"ok"}` + "\n"
	assert.Equal(t, expected, rr.Body.String())
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type")
}