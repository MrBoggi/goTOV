package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"

	"github.com/MrBoggi/goTOV/internal/api"
	"github.com/MrBoggi/goTOV/internal/config"
	"github.com/MrBoggi/goTOV/internal/fermentation"
	"github.com/MrBoggi/goTOV/internal/opcua"
)

func RunServer(log zerolog.Logger) error {
	log.Info().Msg("🚀 Starting goTØV backend")

	// 1. Load config
	cfg, err := config.Load("")
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// 2. Fermentation DB
	store, err := fermentation.NewSQLiteStore(cfg.Fermentation.DatabasePath)
	if err != nil {
		return fmt.Errorf("sqlite init: %w", err)
	}

	// 3. OPC UA client
	client, err := opcua.NewClient(
		cfg.OPCUA.Endpoint,
		cfg.OPCUA.Username,
		cfg.OPCUA.Password,
		log,
	)
	if err != nil {
		return fmt.Errorf("opcua: %w", err)
	}

	if err := client.Connect(); err != nil {
		return fmt.Errorf("opcua connect: %w", err)
	}

	// 4. Context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 5. List symbols
	nodes, err := client.ListSymbols(ctx)
	if err != nil {
		return fmt.Errorf("list symbols: %w", err)
	}
	log.Info().Msgf("🧭 Found %d OPC UA nodes", len(nodes))

	// 6. Start subscription
	go func() {
		if err := client.SubscribeAll(ctx, nodes); err != nil {
			log.Error().Err(err).Msg("❌ Subscription failed")
			cancel()
		}
	}()

	// 7. API server
	apiServer := api.NewServer(log, client, store, cfg)
	go func() {
		if err := apiServer.Start(":8080"); err != nil {
			log.Error().Err(err).Msg("HTTP server crashed")
			cancel()
		}
	}()

	// 8. Graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	log.Info().Msg("🛑 Shutting down backend...")
	cancel()
	time.Sleep(250 * time.Millisecond)
	return nil
}
