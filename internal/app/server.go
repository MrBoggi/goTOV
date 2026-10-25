package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/MrBoggi/goTOV/internal/ads"
	"github.com/MrBoggi/goTOV/internal/api"
	"github.com/MrBoggi/goTOV/internal/config"
	"github.com/MrBoggi/goTOV/internal/fermentation"
	"github.com/MrBoggi/goTOV/internal/opcua"
	"github.com/MrBoggi/goTOV/internal/version"
	"github.com/gopcua/opcua/ua"
	"github.com/rs/zerolog"
)

// RunServer starter hele goTØV-backend (OPC UA, HTTP/WS, subscription)
// og blokker til prosessen får SIGINT/SIGTERM.
func RunServer(log zerolog.Logger) error {
	log.Info().Str("version", version.Get()).Msg("🚀 Starting goTØV backend")

	// --- Load config ---
	cfg, err := config.Load("")
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to load configuration")
		return err
	}
	log.Info().
		Str("endpoint", cfg.OPCUA.Endpoint).
		Str("user", cfg.OPCUA.Username).
		Str("listen_addr", cfg.Server.ListenAddr).
		Msg("✅ Config loaded")

	// --- Context for subs ---
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	adsFreshness, err := time.ParseDuration(cfg.ADS.FreshnessWindow)
	if err != nil || adsFreshness <= 0 {
		return fmt.Errorf("invalid ADS freshness_window %q", cfg.ADS.FreshnessWindow)
	}
	adsMonitor := ads.NewMonitor(cfg.ADS.Enabled, adsFreshness)

	// --- Fermentation store ---
	fermentationStore, err := fermentation.NewSQLiteStore(cfg.Fermentation.DatabasePath)
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to create fermentation store")
		return err
	}
	defer func() {
		_ = fermentationStore.Close()
		log.Info().Msg("📂 Fermentation store closed")
	}()

	// --- OPC UA client ---
	// The HTTP API remains available when the PLC is offline. This makes health
	// checks represent the API process, while write operations report that OPC UA
	// is unavailable until the PLC can be reached.
	var client *opcua.Client
	var nodes []*ua.NodeID
	candidate, err := opcua.NewClient(cfg.OPCUA.Endpoint, cfg.OPCUA.Username, cfg.OPCUA.Password, log)
	if err != nil {
		log.Warn().Err(err).Msg("⚠️ OPC UA client unavailable; starting API without PLC connection")
	} else if err := candidate.Connect(); err != nil {
		_ = candidate.Close()
		log.Warn().Err(err).Msg("⚠️ OPC UA server unreachable; starting API without PLC connection")
	} else if nodes, err = candidate.ListSymbols(ctx); err != nil {
		_ = candidate.Close()
		log.Warn().Err(err).Msg("⚠️ Failed to list PLC symbols; starting API without PLC connection")
	} else {
		client = candidate
		defer func() {
			_ = client.Close()
			log.Info().Msg("🔌 OPC UA client closed")
		}()
		log.Info().Msg("✅ Connected to Beckhoff PLC via OPC UA")
		log.Info().Msgf("🧭 Found %d symbols manually", len(nodes))
	}

	// --- Waitgroup for graceful shutdown ---
	var wg sync.WaitGroup

	// --- Optional ADS V1 input verification ---
	// This is intentionally independent of OPC UA and never changes API or
	// control behaviour. A missing ADS route is a degraded optional integration.
	if cfg.ADS.Enabled {
		adsCandidate, adsErr := ads.NewClient(cfg.ADS)
		if adsErr != nil {
			adsMonitor.Rejected("connect_failed")
			log.Warn().Err(adsErr).Msg("⚠️ ADS verification unavailable; continuing without ADS")
		} else if adsErr = adsCandidate.Connect(ctx); adsErr != nil {
			adsMonitor.Rejected("connect_failed")
			_ = adsCandidate.Close()
			log.Warn().Err(adsErr).Msg("⚠️ ADS PLC unreachable; continuing without ADS")
		} else if adsErr = adsCandidate.ReadState(ctx); adsErr != nil {
			adsMonitor.Rejected("read_state_failed")
			_ = adsCandidate.Close()
			log.Warn().Err(adsErr).Msg("⚠️ ADS state check failed; continuing without ADS")
		} else if adsErr = adsCandidate.LookupInputs(ctx); adsErr != nil {
			adsMonitor.Rejected("symbol_lookup_failed")
			_ = adsCandidate.Close()
			log.Warn().Err(adsErr).Msg("⚠️ ADS input symbol unavailable; continuing without ADS")
		} else {
			defer func() {
				_ = adsCandidate.Close()
				log.Info().Msg("🔌 ADS client closed")
			}()
			wg.Add(1)
			go func() {
				defer wg.Done()
				runADSInputVerification(ctx, adsCandidate, adsMonitor, log)
			}()
		}
	}

	// --- Start HTTP/WS API server ---
	apiServer := api.NewServer(log, client, fermentationStore)
	apiServer.SetADSMonitor(adsMonitor)
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := apiServer.Start(cfg.Server.ListenAddr); err != nil {
			log.Error().Err(err).Msg("🌐 HTTP/WS server stopped")
			cancel()
		}
	}()

	// --- Start subscription only when OPC UA is connected ---
	if client != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := client.SubscribeAll(ctx, nodes); err != nil {
				log.Error().Err(err).Msg("❌ Subscription failed")
				cancel()
			}
		}()
	}

	// --- Graceful shutdown ---
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	<-stop
	log.Info().Msg("🛑 Shutting down gracefully...")
	cancel()
	wg.Wait() // vent for alle goroutines
	log.Info().Msg("👋 goTØV backend stopped cleanly")

	return nil
}

func runADSInputVerification(ctx context.Context, client *ads.Client, monitor *ads.Monitor, log zerolog.Logger) {
	ticker := time.NewTicker(client.PollInterval())
	defer ticker.Stop()

	for {
		inputs, err := client.ReadInputsSnapshot(ctx)
		if err != nil {
			monitor.Rejected("transport_failed")
			log.Warn().Err(err).Msg("⚠️ ADS input snapshot rejected")
		} else {
			monitor.Accepted(inputs.Sequence, time.Now().UTC())
			log.Debug().Uint32("sequence", inputs.Sequence).Msg("ADS input snapshot verified")
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
