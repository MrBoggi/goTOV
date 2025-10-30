package main

import (
	// "goTOV/internal/config"
	// "goTOV/internal/logger"
	// "goTOV/internal/opcua"

	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/MrBoggi/goTOV/internal/config"
	"github.com/MrBoggi/goTOV/internal/logger"
	"github.com/MrBoggi/goTOV/internal/opcua"
	"github.com/gopcua/opcua/ua"
)

func main() {
	// Initialize logger
	log := logger.New()
	var (
		nodeID = flag.String("node", "ns=4;s=MAIN.fbUA.iTestSignal", "NodeID to read")
	)
	// Load configuration
	cfg, err := config.Load("internal/config/config.yaml")
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load config")
	}

	// Create OPC UA client
	//client, err := opcua.NewClient(cfg.OPCUA.Endpoint, "", "", log)
	client, err := opcua.NewClient(cfg.OPCUA.Endpoint, cfg.OPCUA.Username, cfg.OPCUA.Password, log)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create OPC UA client")
	}
	defer client.Close()

	// // Connect to PLC
	if err := client.Connect(); err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to OPC UA server")
	}
	log.Info().Msg("Connected to Beckhoff PLC via OPC UA")

	id, err := ua.ParseNodeID(*nodeID)
	if err != nil {
		log.Fatal().Err(err).Msg("invalid node id")
	}
	log.Info().Msg(id.String())
	log.Info().Msgf("Reading node: %s", *nodeID)
	log.Info().Msg(id.Type().String())

	ctx := context.Background()
	resp, err := client.ReadRaw(ctx, "ns=4;s=MAIN.fbUA.iTestSignal")
	if err != nil {
		log.Fatal().Err(err).Msg("Read failed")
	}

	if resp.Results[0].Value == nil {
		log.Warn().Msgf("Value is <nil>, Status=%v", resp.Results[0].Status)
	} else {
		log.Info().Msgf("Response: %v", resp.Results[0].Status)
		val := resp.Results[0].Value.Value()
		log.Info().Msgf("Value=%v (type=%T)", val, val)
	}

	// // // Subscribe or read nodes (example)
	// nodeID := "ns=4;s=MAIN.fbUA.iTestSignal"
	// time.Sleep(500 * time.Millisecond)
	// val, err := client.ReadNodeValue(nodeID)
	// if err != nil {
	// 	log.Error().Err(err).Msg("Failed to read node value")
	// } else if val == nil {
	// 	log.Warn().Msg("Value is <nil>")
	// } else {
	// 	log.Info().Msgf("Temp_HLT = %v (type %T)", val, val)
	// }

	// Wait for interrupt (Ctrl+C)
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	log.Info().Msg("Shutting down gracefully...")
	time.Sleep(500 * time.Millisecond)
}
