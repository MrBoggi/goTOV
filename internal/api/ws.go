package api

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"

	"github.com/MrBoggi/goTOV/internal/config"
	"github.com/MrBoggi/goTOV/internal/fermentation"
	"github.com/MrBoggi/goTOV/internal/opcua"
)

type Server struct {
	log    zerolog.Logger
	client *opcua.Client
	store  *fermentation.SQLiteStore
	cfg    *config.Config

	// websocket clients
	mu          sync.RWMutex
	subscribers map[*websocket.Conn]bool

	// REST snapshot cache
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

// ------------------------------------------------------
// NewServer
// ------------------------------------------------------
func NewServer(log zerolog.Logger, client *opcua.Client, store *fermentation.SQLiteStore, cfg *config.Config) *Server {
	s := &Server{
		log:         log,
		client:      client,
		store:       store,
		cfg:         cfg,
		subscribers: make(map[*websocket.Conn]bool),
		latest:      make(map[string]WSMessage),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}

	// 👇👇👇 CRITICAL – start WS update pump
	go s.consumeUpdates()

	return s
}

// ------------------------------------------------------
// Router
// ------------------------------------------------------
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()

	// Allow CORS from everywhere (dev mode)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders: []string{"Accept", "Authorization", "Content-Type"},
	}))

	// Health
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("ok"))
	})

	// UA / WS / Snapshot / Write
	r.Get("/api/stream/tags", s.handleWS)
	r.Get("/api/tags", s.handleSnapshot)
	r.Post("/api/write", s.handleWrite)

	// Brewfather
	r.Get("/api/brewfather/batches", s.handleListBatches)
	r.Get("/api/brewfather/batch/{id}", s.handleGetBatch)

	// Fermentation
	r.Post("/api/fermentation/start", s.handleStartFermentation)
	r.Get("/api/fermentation/active", s.handleGetActiveFermentation)

	return r
}

// ------------------------------------------------------
// Start server
// ------------------------------------------------------
func (s *Server) Start(addr string) error {
	s.log.Info().Str("addr", addr).Msg("HTTP/WS server starting")
	return http.ListenAndServe(addr, s.Router())
}

// ------------------------------------------------------
// Websocket + tag updates
// ------------------------------------------------------
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.log.Error().Err(err).Msg("WS upgrade failed")
		return
	}

	s.mu.Lock()
	s.subscribers[conn] = true
	s.mu.Unlock()

	// Send snapshot on connect
	s.latestMu.RLock()
	for _, msg := range s.latest {
		_ = conn.WriteJSON(msg)
	}
	s.latestMu.RUnlock()

	conn.SetReadLimit(1024)
	_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	go s.keepAlive(conn)

	// Listen for disconnect
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}

	s.mu.Lock()
	delete(s.subscribers, conn)
	s.mu.Unlock()
	_ = conn.Close()
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

// Snapshot
func (s *Server) handleSnapshot(w http.ResponseWriter, _ *http.Request) {
	s.latestMu.RLock()
	defer s.latestMu.RUnlock()
	_ = json.NewEncoder(w).Encode(s.latest)
}

// Consume OPC UA update events
func (s *Server) consumeUpdates() {
	for ev := range s.client.Updates {
		msg := WSMessage{
			Tag:         ev.Name,
			Value:       ev.Value,
			ValueType:   ev.Type,
			DisplayName: ev.DisplayName,
			Timestamp:   time.Now().UnixMilli(),
		}

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
			// Remove on failed write
			s.mu.RUnlock()
			s.mu.Lock()
			delete(s.subscribers, conn)
			s.mu.Unlock()
			s.mu.RLock()
			_ = conn.Close()
		}
	}
}
