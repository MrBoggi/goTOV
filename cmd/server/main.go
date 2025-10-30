package main

import (
	"context"
	"fmt"
	"goTOV/internal/config"
	"goTOV/internal/logger"
	"goTOV/internal/opcua"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	// Initialize logger
	log := logger.New()

	// Load configuration
	cfg, err := config.Load("internal/config/config.yaml")
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load config")
	}

	// Create OPC UA client
	client, err := opcua.NewClient(cfg.OPCUA.Endpoint, cfg.OPCUA.Username, cfg.OPCUA.Password, log)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create OPC UA client")
	}
	defer client.Close()

	// Connect to PLC
	if err := client.Connect(); err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to OPC UA server")
	}
	log.Info().Msg("Connected to Beckhoff PLC via OPC UA")

	// Subscribe or read nodes (example)
	nodeID := "ns=4;s=MAIN.Temp_HLT"
	val, err := client.ReadNodeValue(nodeID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read node value")
	} else {
		log.Info().Msgf("Temp_HLT = %v", val)
	}

	// Wait for interrupt (Ctrl+C)
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	log.Info().Msg("Shutting down gracefully...")
	time.Sleep(500 * time.Millisecond)
}
