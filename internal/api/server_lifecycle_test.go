package api

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/MrBoggi/goTOV/internal/logger"
	"github.com/stretchr/testify/require"
)

func TestServerShutdownStopsStartedServer(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := listener.Addr().String()
	require.NoError(t, listener.Close())

	server := NewServer(logger.New(), nil, &MockFermentationStore{})
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Start(addr)
	}()

	require.Eventually(t, func() bool {
		resp, err := http.Get("http://" + addr + "/health")
		if err != nil {
			return false
		}
		_ = resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 5*time.Second, 20*time.Millisecond)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, server.Shutdown(shutdownCtx))

	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("HTTP server did not return after shutdown")
	}
}

func TestCORSAllowsMutationMethods(t *testing.T) {
	server := NewServer(logger.New(), nil, &MockFermentationStore{})
	for _, method := range []string{http.MethodPut, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodOptions, "/api/fermentation/plan/1", nil)
			req.Header.Set("Origin", "http://localhost:5173")
			req.Header.Set("Access-Control-Request-Method", method)

			recorder := httptest.NewRecorder()
			server.Router().ServeHTTP(recorder, req)

			require.Equal(t, http.StatusOK, recorder.Code)
			require.Equal(t, method, recorder.Header().Get("Access-Control-Allow-Methods"))
		})
	}
}
