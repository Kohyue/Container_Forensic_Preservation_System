package api

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"forensic-preservation/internal/config"
	"forensic-preservation/internal/webstatic"
)

// Server serves the web dashboard and REST API.
type Server struct {
	cfg        *config.Config
	logger     *slog.Logger
	startedAt  time.Time
	httpServer *http.Server
}

func NewServer(cfg *config.Config, logger *slog.Logger, startedAt time.Time) *Server {
	s := &Server{cfg: cfg, logger: logger, startedAt: startedAt}

	mux := http.NewServeMux()

	// REST API routes
	mux.HandleFunc("/api/v1/status", s.handleStatus)
	mux.HandleFunc("/api/v1/stats", s.handleStats)
	mux.HandleFunc("/api/v1/alerts", s.handleAlerts)
	mux.HandleFunc("/api/v1/evidence", s.handleListEvidence)
	mux.HandleFunc("/api/v1/evidence/", s.handleEvidence)
	mux.HandleFunc("/api/v1/config", s.handleGetConfig)
	mux.HandleFunc("/api/v1/rules", s.handleGetRules)

	// Static web files — serve from the embedded FS
	webFS, err := fs.Sub(webstatic.FS, "web")
	if err != nil {
		panic("webstatic FS misconfigured: " + err.Error())
	}
	mux.Handle("/", http.FileServer(http.FS(webFS)))

	s.httpServer = &http.Server{
		Addr:         cfg.Web.Addr,
		Handler:      cors(mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
	return s
}

// Start begins listening. Blocks until ctx is cancelled.
func (s *Server) Start(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.httpServer.Shutdown(shutCtx)
	}()
	s.logger.Info("web dashboard starting", "addr", s.cfg.Web.Addr)
	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// cors adds permissive CORS headers for development convenience.
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
