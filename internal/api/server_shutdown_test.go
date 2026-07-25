package api

import (
	"context"
	"testing"
	"time"

	"github.com/MrBoggi/goTOV/internal/logger"
	"github.com/stretchr/testify/require"
)

func TestServerShutdownStopsStartedServer(t *testing.T) {
	server := NewServer(logger.New(), nil, &MockFermentationStore{}, nil, nil, nil, nil, nil)
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Start("127.0.0.1:0")
	}()

	require.Eventually(t, func() bool {
		server.httpServerMu.RLock()
		defer server.httpServerMu.RUnlock()
		return server.httpServer != nil
	}, time.Second, 10*time.Millisecond)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, server.Shutdown(shutdownCtx))

	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("HTTP server did not return after shutdown")
	}
}
