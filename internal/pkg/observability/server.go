// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package observability

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/config"
	"go.uber.org/zap"
)

// Server exposes metrics and health endpoints for probes and scraping.
type Server struct {
	server *http.Server
	ready  atomic.Bool
}

// NewServer creates the HTTP observability endpoint set.
func NewServer(cfg config.ObservabilityConfig, metrics *Metrics) *Server {
	mux := http.NewServeMux()
	srv := &Server{}

	mux.Handle(cfg.MetricsPath, metrics.Handler())
	mux.HandleFunc(cfg.HealthPath, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc(cfg.ReadyPath, func(w http.ResponseWriter, _ *http.Request) {
		if !srv.ready.Load() {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready\n"))
	})

	srv.server = &http.Server{
		Addr:              cfg.ListenAddress,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	return srv
}

// Start serves the observability HTTP endpoints until the context is canceled.
func (s *Server) Start(ctx context.Context, logger *zap.Logger) error {
	listener, err := net.Listen("tcp", s.server.Addr)
	if err != nil {
		return err
	}

	go func() {
		<-ctx.Done()
		s.ready.Store(false)

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := s.server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Warn("shutdown observability server", zap.Error(err))
		}
	}()

	logger.Info("starting observability server", zap.String("address", s.server.Addr))

	go func() {
		if err := s.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("serve observability endpoints", zap.Error(err))
		}
	}()

	return nil
}

// SetReady flips the readiness endpoint state.
func (s *Server) SetReady(ready bool) {
	s.ready.Store(ready)
}
