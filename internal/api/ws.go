package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/MrBoggi/goTOV/internal/brew"
	"github.com/MrBoggi/goTOV/internal/brewfather"
	"github.com/MrBoggi/goTOV/internal/brewhouse"
	"github.com/MrBoggi/goTOV/internal/fermentation"
	"github.com/MrBoggi/goTOV/internal/opcua"
	"github.com/MrBoggi/goTOV/internal/version"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
)

type Server struct {
	log               zerolog.Logger
	client            *opcua.Client
	fermentationStore fermentation.FermentationStore
	engine            *fermentation.Engine
	brewhouseStore    brewhouse.Store
	brewhouseEngine   *brewhouse.Engine
	brewingEngine     *brew.Engine
	brewfatherClient  *brewfather.Client

	// Connected websocket clients
	mu          sync.RWMutex
	subscribers map[*websocket.Conn]bool

	// Latest known values for REST snapshot
	latestMu sync.RWMutex
	latest   map[string]WSMessage

	upgrader websocket.Upgrader
}

type WSMessage struct {
	Tag         string      `json:"tag"`
	DisplayName string      `json:"display_name"`
	Value       interface{} `json:"value"`
	ValueType   string      `json:"value_type"`
	Timestamp   int64       `json:"ts_ms"`
}

// NewServer initializes the WS/HTTP server and listens for OPC UA updates
func NewServer(log zerolog.Logger, client *opcua.Client, fermentationStore fermentation.FermentationStore, engine *fermentation.Engine, brewhouseStore brewhouse.Store, brewhouseEngine *brewhouse.Engine, brewingEngine *brew.Engine, brewfatherClient *brewfather.Client) *Server {
	s := &Server{
		log:               log,
		client:            client,
		fermentationStore: fermentationStore,
		engine:            engine,
		brewhouseStore:    brewhouseStore,
		brewhouseEngine:   brewhouseEngine,
		brewingEngine:     brewingEngine,
		brewfatherClient:  brewfatherClient,
		subscribers:       make(map[*websocket.Conn]bool),
		latest:            make(map[string]WSMessage),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin:     func(r *http.Request) bool { return true },
		},
	}

	if s.client != nil {
		go s.consumeUpdates()
		go s.seedCache()
	}
	if s.brewhouseEngine != nil {
		go s.broadcastBrewhouseState()
	}
	if s.brewingEngine != nil {
		go s.broadcastBrewingStatus()
	}
	return s
}

func (s *Server) seedCache() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	s.log.Info().Msg("🌱 Seeding WebSocket cache from PLC...")

	nodes, err := s.client.ListSymbols(ctx)
	if err != nil {
		s.log.Error().Err(err).Msg("❌ Failed to list PLC symbols for seeding")
		return
	}

	for _, nodeID := range nodes {
		val, err := s.client.ReadNodeValue(ctx, nodeID.String())
		if err != nil {
			s.log.Warn().Err(err).Msgf("⚠️ Seed failed for %s", nodeID.String())
			continue
		}

		msg := WSMessage{
			Tag:         nodeID.String(),
			DisplayName: s.client.GetDisplayName(nodeID.String()),
			Value:       val,
			ValueType:   fmt.Sprintf("%T", val),
			Timestamp:   time.Now().UnixMilli(),
		}

		s.latestMu.Lock()
		s.latest[msg.Tag] = msg
		s.latestMu.Unlock()

		s.log.Info().Msgf("🔄 Seeded %s = %v", msg.Tag, val)
	}
	s.log.Info().Msg("✅ WebSocket cache seeding complete")
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// Version endpoint
	r.Get("/api/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"version": version.Get(),
		})
	})

	// API endpoints
	r.Get("/api/stream/tags", s.handleWS)
	r.Get("/api/tags", s.handleSnapshot)
	r.Post("/api/write", s.handleWrite)
	r.Post("/api/fermentation/plan", s.handleSaveFermentationPlan)
	r.Get("/api/fermentation/plan", s.handleListFermentationPlans) // Alias for plural
	r.Get("/api/fermentation/plans", s.handleListFermentationPlans)
	r.Post("/api/fermentation/start", s.handleStartFermentation)
	r.Post("/api/fermentation/stop", s.handleStopFermentation)
	r.Get("/api/fermentation/status", s.handleGetFermentationStatus)
	r.Get("/api/fermentation/history/{id}", s.handleGetFermentationHistory)
	r.Get("/api/fermentation/docs", s.handleGetApiDocs)
	r.Get("/api/docs", s.handleGetApiDocs)
	r.Delete("/api/fermentation/plan/{id}", s.handleDeleteFermentationPlan)
	r.Get("/api/tanks", s.handleListTanks)
	r.Get("/api/glycol/status", s.handleGetGlycolStatus)
	r.Get("/api/brewhouse/state", s.handleGetBrewhouseState)
	r.Post("/api/brewhouse/state", s.handleUpdateBrewhouseState)

	// Brewing process
	r.Get("/api/brewfather/batches", s.handleListBrewfatherBatches)
	r.Post("/api/brewing/start", s.handleStartBrewing)
	// Fixed: Let's use handleStopBrewing
	r.Post("/api/brewing/stop", s.handleStopBrewing)
	r.Get("/api/brewing/status", s.handleGetBrewingStatus)
	r.Post("/api/brewing/config/pid", s.handleSavePIDConfig)
	r.Get("/api/brewing/history", s.handleGetBrewingHistory)

	// ----------------------------------------------------
	// STATIC FILES (INGENTING annet fjernes eller endres)
	// ----------------------------------------------------
	fileServer := http.FileServer(http.Dir("/app/cmd/static"))
	r.Handle("/*", fileServer)

	return r
}

// Start the HTTP server (blocking)
func (s *Server) Start(addr string) error {
	s.log.Info().Str("addr", addr).Msg("🌐 HTTP/WS server starting")
	return http.ListenAndServe(addr, s.Router())
}

// --- Internal logic ---

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.log.Error().Err(err).Msg("❌ WS upgrade failed")
		return
	}

	s.mu.Lock()
	s.subscribers[conn] = true
	s.mu.Unlock()
	s.log.Info().Str("remote", conn.RemoteAddr().String()).Msg("💬 WS client connected")

	// Send initial snapshot
	s.latestMu.RLock()
	for _, msg := range s.latest {
		_ = conn.WriteJSON(msg)
	}
	s.latestMu.RUnlock()

	// Setup ping handler
	conn.SetReadLimit(1024)
	_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	go s.keepAlive(conn)

	// Block until client closes
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}

	// Remove client
	s.mu.Lock()
	delete(s.subscribers, conn)
	s.mu.Unlock()
	_ = conn.Close()
	s.log.Info().Msg("🧹 WS client disconnected")
}

func (s *Server) keepAlive(conn *websocket.Conn) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		if err := conn.WriteControl(websocket.PingMessage, []byte("ping"), time.Now().Add(5*time.Second)); err != nil {
			return
		}
	}
}

func (s *Server) handleSnapshot(w http.ResponseWriter, _ *http.Request) {
	s.latestMu.RLock()
	defer s.latestMu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(s.latest); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) consumeUpdates() {
	for ev := range s.client.Updates {
		msg := WSMessage{
			Tag:         ev.Name,
			Value:       ev.Value,
			ValueType:   ev.Type,
			DisplayName: ev.DisplayName, // 👈 legg til dette
			Timestamp:   time.Now().UnixMilli(),
		}

		// oppdater cache
		s.latestMu.Lock()
		s.latest[ev.Name] = msg
		s.latestMu.Unlock()

		s.broadcast(msg)
	}
}

func (s *Server) broadcast(msg WSMessage) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for conn := range s.subscribers {
		_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
		if err := conn.WriteJSON(msg); err != nil {
			s.mu.RUnlock()
			s.mu.Lock()
			delete(s.subscribers, conn)
			s.mu.Unlock()
			s.mu.RLock()
			_ = conn.Close()
			s.log.Warn().Err(err).Msg("❌ WS write failed — client removed")
		}
	}
}
