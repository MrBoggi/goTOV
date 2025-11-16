package main

import (
	"context"
	"flag"
	"time"

	"github.com/MrBoggi/goTOV/internal/config"
	"github.com/MrBoggi/goTOV/internal/fermentation"
	"github.com/MrBoggi/goTOV/internal/logger"
	"github.com/MrBoggi/goTOV/internal/opcua"

	"github.com/rs/zerolog"
)

func main() {
	// --- Flags ---
	write := flag.Bool("write", false, "Write outputs to PLC")
	interval := flag.Duration("interval", 2*time.Second, "Tick interval")
	flag.Parse()

	// --- Logger ---
	log := logger.New()

	// --- Load config.yaml ---
	cfg, err := config.Load("")
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load config")
	}

	// --- SQLite store (fermentation.db) ---
	store, err := fermentation.NewSQLiteStore(cfg.Fermentation.DatabasePath)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to open sqlite database")
	}

	// --- Fermentation State ---
	fermentState, err := store.GetActiveFermentationState()
	if err != nil {
		log.Fatal().Err(err).Msg("no active fermentation state found in db")
	}

	log.Info().
		Str("batch_id", fermentState.BatchID).
		Int("tank_no", fermentState.TankNo).
		Int64("plan_id", fermentState.PlanID).
		Time("started_at", fermentState.StartedAt).
		Msg("Loaded fermentation state")

	// --- OPC UA Client ---
	ua, err := opcua.NewClient(
		cfg.OPCUA.Endpoint,
		cfg.OPCUA.Username,
		cfg.OPCUA.Password,
		log,
	)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create OPC UA client")
	}

	if err := ua.Connect(); err != nil {
		log.Fatal().Err(err).Msg("OPC UA connection failed")
	}
	defer ua.Close()

	// --- Engine config ---
	engineCfg := fermentation.EngineConfig{
		CoolingHysteresis: cfg.Fermentation.Hysteresis.Cooling,
		HeatingHysteresis: cfg.Fermentation.Hysteresis.Heating,
		PumpDelay:         500 * time.Millisecond,
	}

	engine := fermentation.NewEngineState()

	log.Info().Msg("Starting fermentation PLC test loop...")
	if !*write {
		log.Warn().Msg("Test runs in READ-ONLY mode. Use --write=true to control PLC.")
	}

	// --- TEST LOOP ---
	for {
		now := time.Now()
		ctx := context.Background()

		// Read temps
		t1 := readFloat(ctx, ua, log, "MAIN.fbUA.fermenter1Temp")
		t2 := readFloat(ctx, ua, log, "MAIN.fbUA.fermenter2Temp")

		// Get target temp
		target, err := store.GetTargetTemperature(*fermentState, now)
		if err != nil {
			log.Error().Err(err).Msg("failed to get target temperature")
			target = 0
		}

		// Inputs
		t1In := fermentation.TankInput{Active: true, Temp: t1, TargetTemp: target}
		t2In := fermentation.TankInput{Active: true, Temp: t2, TargetTemp: target}

		// Run engine
		engine = engine.Tick(engineCfg, now, t1In, t2In)

		// Log results
		log.Info().
			Float64("T1", t1).
			Float64("T2", t2).
			Float64("Target", target).
			Bool("T1_Cool", engine.Outputs.Tanks[fermentation.Tank1].Cooling).
			Bool("T1_Heat", engine.Outputs.Tanks[fermentation.Tank1].Heating).
			Bool("T2_Cool", engine.Outputs.Tanks[fermentation.Tank2].Cooling).
			Bool("T2_Heat", engine.Outputs.Tanks[fermentation.Tank2].Heating).
			Bool("Pump", engine.Outputs.Pump).
			Msg("Tick")

		// Write to PLC
		if *write {
			writeBool(ctx, ua, log, "MAIN.fbUA.fermenter1Kjoleventil", engine.Outputs.Tanks[fermentation.Tank1].Cooling)
			writeBool(ctx, ua, log, "MAIN.fbUA.fermenter1Varmekappe", engine.Outputs.Tanks[fermentation.Tank1].Heating)
			writeBool(ctx, ua, log, "MAIN.fbUA.fermenter2Kjoleventil", engine.Outputs.Tanks[fermentation.Tank2].Cooling)
			writeBool(ctx, ua, log, "MAIN.fbUA.fermenter2Varmekappe", engine.Outputs.Tanks[fermentation.Tank2].Heating)

			writeBool(ctx, ua, log, "MAIN.fbUA.glykolkjolerPumpe", engine.Outputs.Pump)
		}

		time.Sleep(*interval)
	}
}

// --- Helper functions ---

func readFloat(ctx context.Context, ua *opcua.Client, log zerolog.Logger, node string) float64 {
	val, err := ua.ReadNodeValue(ctx, node)
	if err != nil {
		log.Error().Err(err).Msgf("Reading %s failed", node)
		return 0
	}

	switch v := val.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int16:
		return float64(v)
	case int32:
		return float64(v)
	default:
		log.Warn().Msgf("Unexpected type for %s: %T", node, val)
		return 0
	}
}

func writeBool(ctx context.Context, ua *opcua.Client, log zerolog.Logger, node string, value bool) {
	if err := ua.WriteNodeValue(ctx, node, value); err != nil {
		log.Error().Err(err).Msgf("Write %s failed", node)
	}
}
