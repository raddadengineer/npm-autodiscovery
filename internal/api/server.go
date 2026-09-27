package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/sampson/npm-autodiscovery/internal/config"
	"github.com/sampson/npm-autodiscovery/internal/metrics"
	"github.com/sampson/npm-autodiscovery/internal/syncer"
)

// Server handles HTTP API requests and serves the frontend Single Page Application.
type Server struct {
	cfg       *config.Config
	syncer    *syncer.Syncer
	webFS     fs.FS
	httpSrv   *http.Server
}

// NewServer initializes an HTTP server.
func NewServer(cfg *config.Config, s *syncer.Syncer, webFS fs.FS) *Server {
	return &Server{
		cfg:    cfg,
		syncer: s,
		webFS:  webFS,
	}
}

// Start runs the HTTP listener on the configured port.
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// API Routes
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/proxies", s.handleProxies)
	mux.HandleFunc("/api/streams", s.handleStreams)
	mux.HandleFunc("/api/containers", s.handleContainers)
	mux.HandleFunc("/api/sync", s.handleSync)
	mux.HandleFunc("/api/events/stream", s.handleEventsStream)
	mux.HandleFunc("/api/events", s.handleEventsHistory)
	mux.HandleFunc("/api/config", s.handleConfig)

	// Prometheus Metrics Endpoint (Phase 1)
	mux.HandleFunc("/metrics", s.handleMetrics)

	// Static Web Dashboard files
	if s.webFS != nil {
		fileServer := http.FileServer(http.FS(s.webFS))
		mux.Handle("/", fileServer)
	} else {
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("NPM Auto-Discovery Backend API is running."))
		})
	}

	addr := fmt.Sprintf(":%d", s.cfg.Port)
	s.httpSrv = &http.Server{
		Addr:         addr,
		Handler:      s.withMiddleware(mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 0, // 0 for SSE streaming connections
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("[info] [server] Web Dashboard & API listening on http://0.0.0.0:%d", s.cfg.Port)
	return s.httpSrv.ListenAndServe()
}

// Stop gracefully shuts down the server.
func (s *Server) Stop(ctx context.Context) error {
	if s.httpSrv != nil {
		return s.httpSrv.Shutdown(ctx)
	}
	return nil
}

func (s *Server) withMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// handleStatus returns overall status, connection health, and metrics.
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	overview := s.syncer.GetStatusOverview(r.Context())
	s.writeJSON(w, http.StatusOK, overview)
}

// handleProxies returns active auto-discovered proxy hosts.
func (s *Server) handleProxies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	proxies := s.syncer.GetTrackedProxies()
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"proxies": proxies,
		"count":   len(proxies),
	})
}

// handleStreams returns active auto-discovered Layer 4 streams.
func (s *Server) handleStreams(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	streams := s.syncer.GetTrackedStreams()
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"streams": streams,
		"count":   len(streams),
	})
}

// handleContainers returns running containers with discovery evaluation.
func (s *Server) handleContainers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	containers, err := s.syncer.GetContainersView(r.Context())
	if err != nil {
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"containers": containers,
		"count":      len(containers),
	})
}

// handleSync triggers an immediate full scan and reconciliation.
func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.syncer.TriggerSync()
	s.writeJSON(w, http.StatusOK, map[string]string{
		"status":  "sync_triggered",
		"message": "Full discovery scan initiated immediately",
	})
}

// handleEventsHistory returns recent log events.
func (s *Server) handleEventsHistory(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if parsed, err := strconv.Atoi(lStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	events := s.syncer.GetLogEvents(limit)
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"events": events,
		"count":  len(events),
	})
}

// handleEventsStream streams live logs via Server-Sent Events (SSE).
func (s *Server) handleEventsStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Send initial ping comment
	fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	// Send recent logs first so client sees immediate context
	history := s.syncer.GetLogEvents(25)
	for _, ev := range history {
		if data, err := json.Marshal(ev); err == nil {
			fmt.Fprintf(w, "event: log\ndata: %s\n\n", string(data))
		}
	}
	flusher.Flush()

	eventCh, unsubscribe := s.syncer.SubscribeLogs()
	defer unsubscribe()

	keepAliveTicker := time.NewTicker(15 * time.Second)
	defer keepAliveTicker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return

		case ev, ok := <-eventCh:
			if !ok {
				return
			}
			data, err := json.Marshal(ev)
			if err == nil {
				fmt.Fprintf(w, "event: log\ndata: %s\n\n", string(data))
				flusher.Flush()
			}

		case <-keepAliveTicker.C:
			fmt.Fprintf(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

// handleConfig returns public configuration details.
func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, s.cfg)
}

// handleMetrics serves standard Prometheus metrics.
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	metrics.Handler().ServeHTTP(w, r)
}

func (s *Server) writeJSON(w http.ResponseWriter, code int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(data)
}
