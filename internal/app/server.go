package app

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/MrBoggi/goTOV/internal/api"
	"github.com/MrBoggi/goTOV/internal/brew"
	"github.com/MrBoggi/goTOV/internal/brewfather"
	"github.com/MrBoggi/goTOV/internal/brewhouse"
	"github.com/MrBoggi/goTOV/internal/config"
	"github.com/MrBoggi/goTOV/internal/fermentation"
	"github.com/MrBoggi/goTOV/internal/opcua"
	"github.com/MrBoggi/goTOV/internal/version"
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

	// --- Waitgroup for graceful shutdown ---
	var wg sync.WaitGroup

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

	// --- Fermentation engine ---
	engine := fermentation.NewEngine(fermentationStore, client, log)
	engine.Start()
	defer engine.Stop()

	// --- Brewhouse start ---
	var brewhouseStore brewhouse.Store
	var brewhouseEngine *brewhouse.Engine
	if cfg.Brewhouse.Enabled {
		bs, err := brewhouse.NewSQLiteStore(cfg.Brewhouse.DatabasePath)
		if err != nil {
			log.Error().Err(err).Msg("❌ Failed to create brewhouse store")
			return err
		}
		defer func() {
			_ = bs.Close()
			log.Info().Msg("📂 Brewhouse store closed")
		}()
		brewhouseStore = bs

		brewhouseEngine = brewhouse.NewEngine(brewhouseStore, client, log)
		brewhouseEngine.Start()
		defer brewhouseEngine.Stop()
	}

	// --- Brewing engine ---
	brewingEngine := brew.NewEngine()

	// --- Brewfather client for API proxy ---
	var bfClient *brewfather.Client
	if cfg.Brewfather.UserID != "" && cfg.Brewfather.APIKey != "" {
		bfClient = brewfather.NewClient(cfg.Brewfather.UserID, cfg.Brewfather.APIKey)
		log.Info().Msg("✅ Brewfather API client initialized")
	}

	// --- Start HTTP/WS API server ---
	apiServer := api.NewServer(log, client, fermentationStore, engine, brewhouseStore, brewhouseEngine, brewingEngine, bfClient)
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := apiServer.Start(cfg.Server.ListenAddr); err != nil {
			log.Error().Err(err).Msg("🌐 HTTP/WS server stopped")
			cancel()
		}
	}()

	// --- Start subscription ---
	wg.Add(1)
	go func() {
		defer wg.Done()
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
	wg.Wait() // vent for alle goroutines
	log.Info().Msg("👋 goTØV backend stopped cleanly")

	return nil
}
