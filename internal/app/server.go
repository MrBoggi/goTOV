package app

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/MrBoggi/goTOV/internal/api"
	"github.com/MrBoggi/goTOV/internal/config"
	"github.com/MrBoggi/goTOV/internal/opcua"
	"github.com/rs/zerolog"
)

// RunServer starter hele goTØV-backend (OPC UA, HTTP/WS, subscription)
// og blokker til prosessen får SIGINT/SIGTERM.
func RunServer(log zerolog.Logger) error {
	log.Info().Msg("🚀 Starting goTØV backend")

	// --- Load config ---
	cfg, err := config.Load("")
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to load configuration")
		return err
	}
	log.Info().
		Str("endpoint", cfg.OPCUA.Endpoint).
		Str("user", cfg.OPCUA.Username).
		Msg("✅ Config loaded")

	// --- OPC UA client ---
	client, err := opcua.NewClient(cfg.OPCUA.Endpoint, cfg.OPCUA.Username, cfg.OPCUA.Password, log)
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to create OPC UA client")
		return err
	}
	defer func() {
		_ = client.Close()
		log.Info().Msg("🔌 OPC UA client closed")
	}()

	if err := client.Connect(); err != nil {
		log.Error().Err(err).Msg("❌ Failed to connect to OPC UA server")
		return err
	}
	log.Info().Msg("✅ Connected to Beckhoff PLC via OPC UA")

	// --- Context for subs ---
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// --- List symbols / nodes ---
	nodes, err := client.ListSymbols(ctx)
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to list PLC symbols")
		return err
	}
	log.Info().Msgf("🧭 Found %d symbols manually", len(nodes))

	// --- Start HTTP/WS API server ---
	apiServer := api.NewServer(log, client)
	go func() {
		if err := apiServer.Start(":8080"); err != nil {
			log.Error().Err(err).Msg("🌐 HTTP/WS server stopped")
			cancel()
		}
	}()

	// --- Start subscription ---
	go func() {
		if err := client.SubscribeAll(ctx, nodes); err != nil {
			log.Error().Err(err).Msg("❌ Subscription failed")
			cancel()
		}
	}()

	// --- Graceful shutdown ---
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	<-stop
	log.Info().Msg("🛑 Shutting down gracefully...")
	cancel()
	time.Sleep(500 * time.Millisecond)
	log.Info().Msg("👋 goTØV backend stopped cleanly")

	return nil
}
