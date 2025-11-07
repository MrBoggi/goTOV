package api

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/MrBoggi/goTOV/internal/opcua"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
)

type Server struct {
	log    zerolog.Logger
	client *opcua.Client

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
func NewServer(log zerolog.Logger, client *opcua.Client) *Server {
	s := &Server{
		log:         log,
		client:      client,
		subscribers: make(map[*websocket.Conn]bool),
		latest:      make(map[string]WSMessage),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin:     func(r *http.Request) bool { return true },
		},
	}

	go s.consumeUpdates()
	return s
}

// Router exposes HTTP endpoints
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()

	// ✅ Tillat CORS fra localhost og filsystem (for testing)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"}, // bruk "*" for testing, begrens senere
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	// Endpoints
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	r.Get("/api/stream/tags", s.handleWS)
	r.Get("/api/tags", s.handleSnapshot)
	r.Post("/api/write", s.handleWrite)

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
	s.log.Info().Msg("💬 WS client connected")

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
