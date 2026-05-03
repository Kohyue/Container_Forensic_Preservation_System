package detector

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// AlertHandler is called when a qualifying Falco alert arrives.
type AlertHandler func(ctx context.Context, alert *FalcoAlert)

// Server listens for Falco webhook alerts over HTTP.
type Server struct {
	addr        string
	minPriority string
	handler     AlertHandler
	logger      *slog.Logger
	httpServer  *http.Server
}

func NewServer(addr, minPriority string, handler AlertHandler, logger *slog.Logger) *Server {
	s := &Server{
		addr:        addr,
		minPriority: minPriority,
		handler:     handler,
		logger:      logger,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/alert", s.handleAlert)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	return s
}

// Start begins listening. It blocks until the context is cancelled.
func (s *Server) Start(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.httpServer.Shutdown(shutCtx)
	}()
	s.logger.Info("detector webhook server starting", "addr", s.addr)
	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Server) handleAlert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1 MB max
	if err != nil {
		s.logger.Warn("failed to read alert body", "err", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	var alert FalcoAlert
	if err := json.Unmarshal(body, &alert); err != nil {
		s.logger.Warn("failed to parse alert JSON", "err", err, "body", string(body))
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	s.logger.Info("alert received",
		"rule", alert.Rule,
		"priority", alert.Priority,
		"container_id", alert.ContainerID(),
	)
	if !alert.MeetsMinPriority(s.minPriority) {
		s.logger.Debug("alert below min priority, skipping",
			"priority", alert.Priority,
			"min_priority", s.minPriority,
		)
		w.WriteHeader(http.StatusOK)
		return
	}
	if alert.ContainerID() == "" {
		s.logger.Debug("alert has no container ID, skipping", "rule", alert.Rule)
		w.WriteHeader(http.StatusOK)
		return
	}
	go s.handler(r.Context(), &alert)
	w.WriteHeader(http.StatusAccepted)
}
