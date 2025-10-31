package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/MrBoggi/goTOV/internal/config"
	"github.com/MrBoggi/goTOV/internal/logger"
	"github.com/MrBoggi/goTOV/internal/opcua"
	//"github.com/gopcua/opcua/ua" // 👈 nødvendig for StatusCodeString()
)

func main() {
	// --- Init logger ---
	log := logger.New()
	log.Info().Msg("🚀 Starting goTØV backend")

	// --- Load config (default: configs/config.yaml) ---
	cfg, err := config.Load("")
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

	// --- Test read ---
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	nodeID := "ns=4;s=MAIN"
	log.Info().Str("node", nodeID).Msg("📡 Reading node")

	resp, err := client.ReadRaw(ctx, nodeID)
	if err != nil {
		log.Error().Err(err).Msg("❌ Read failed")
	} else if len(resp.Results) == 0 || resp.Results[0].Value == nil {
		log.Warn().
			Str("status", resp.Results[0].Status.Error()).
			Msg("⚠️ Empty or nil value")
	} else {
		val := resp.Results[0].Value.Value()
		statusText := resp.Results[0].Status.Error()

		log.Info().
			Str("status", statusText).
			Interface("value", val).
			Msgf("✅ Read success: %v (%T)", val, val)
	}

	// --- Graceful shutdown ---
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	log.Info().Msg("🛑 Shutting down gracefully...")
	time.Sleep(500 * time.Millisecond)
}
