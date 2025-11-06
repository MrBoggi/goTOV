package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/MrBoggi/goTOV/internal/api"
	"github.com/MrBoggi/goTOV/internal/config"
	"github.com/MrBoggi/goTOV/internal/logger"
	"github.com/MrBoggi/goTOV/internal/opcua"
)

func main() {
	// --- Init logger ---
	log := logger.New()
	log.Info().Msg("🚀 Starting goTØV backend")

	// --- Load config ---
	cfg, err := config.Load("configs/config.yaml")
	if err != nil {
		log.Fatal().Err(err).Msg("❌ Failed to load configuration")
	}
	log.Info().
		Str("endpoint", cfg.OPCUA.Endpoint).
		Str("user", cfg.OPCUA.Username).
		Msg("✅ Config loaded")

	// --- Initialize OPC UA client ---
	client, err := opcua.NewClient(cfg.OPCUA.Endpoint, cfg.OPCUA.Username, cfg.OPCUA.Password, log)
	if err != nil {
		log.Fatal().Err(err).Msg("❌ Failed to create OPC UA client")
	}
	defer func() {
		client.Close()
		log.Info().Msg("🔌 OPC UA client closed")
	}()

	// --- Connect ---
	if err := client.Connect(); err != nil {
		log.Fatal().Err(err).Msg("❌ Failed to connect to OPC UA server")
	}
	log.Info().Msg("✅ Connected to Beckhoff PLC via OPC UA")

	// --- Create cancellable context ---
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// --- List symbols ---
	nodes, err := client.ListSymbols(ctx)
	if err != nil {
		log.Fatal().Err(err).Msg("❌ Failed to list PLC symbols")
	}
	log.Info().Msgf("🧭 Found %d symbols manually", len(nodes))

	// --- Start HTTP/WebSocket API server ---
	apiServer := api.NewServer(log, client)
	go func() {
		if err := apiServer.Start(":8080"); err != nil {
			log.Error().Err(err).Msg("🌐 HTTP/WS server stopped")
			cancel()
		}
	}()

	// --- Start subscription (OPC UA feed) ---
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
	cancel() // stop everything

	// Vent litt slik at alt rekker å avsluttes pent
	time.Sleep(500 * time.Millisecond)
	log.Info().Msg("👋 goTØV backend stopped cleanly")
}
