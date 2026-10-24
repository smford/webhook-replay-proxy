package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/smford/webhook-replay-proxy/internal/config"
)

// WebhookEvent represents an incoming webhook payload and headers.
type WebhookEvent struct {
	ID        string              `json:"id"`
	Timestamp time.Time           `json:"timestamp"`
	Headers   map[string][]string `json:"headers"`
	Body      string              `json:"body"`
	Path      string              `json:"path"`
}

// Server is the HTTP server for the webhook replay proxy.
type Server struct {
	cfg     *config.Config
	version string
	mu      sync.RWMutex
	events  []WebhookEvent
	mux     *http.ServeMux
}

// NewServer initializes a new Server.
func NewServer(cfg *config.Config, version string) *Server {
	s := &Server{
		cfg:     cfg,
		version: version,
		events:  make([]WebhookEvent, 0),
		mux:     http.NewServeMux(),
	}
	s.routes()
	return s
}

// Handler returns the HTTP handler.
func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealthz)
	s.mux.HandleFunc("GET /version", s.handleVersion)
	s.mux.HandleFunc("POST /api/v1/webhooks", s.handleIngestWebhook)
	s.mux.HandleFunc("GET /api/v1/webhooks", s.handleListWebhooks)
	s.mux.HandleFunc("GET /", s.handleRoot)
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"version": s.version,
	})
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"service": "webhook-replay-proxy",
		"version": s.version,
		"status":  "running",
	})
}

func (s *Server) handleIngestWebhook(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	event := WebhookEvent{
		ID:        fmt.Sprintf("evt_%d", time.Now().UnixNano()),
		Timestamp: time.Now().UTC(),
		Headers:   r.Header,
		Path:      r.URL.Path,
	}

	if len(s.events) >= s.cfg.Storage.MaxEvents && len(s.events) > 0 {
		s.events = s.events[1:]
	}
	s.events = append(s.events, event)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":      event.ID,
		"status":  "received",
		"message": "Webhook queued successfully",
	})
}

func (s *Server) handleListWebhooks(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"count":  len(s.events),
		"events": s.events,
	})
}
