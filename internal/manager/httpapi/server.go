package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/liuscraft/orion-x/internal/config"
)

type Server struct {
	httpServer *http.Server
}

func NewServer(cfg config.ManagerServerConfig, healthHandler http.Handler) *Server {
	mux := http.NewServeMux()
	mux.Handle(cfg.HealthPath, healthHandler)

	return &Server{
		httpServer: &http.Server{
			Addr:         cfg.Address,
			Handler:      mux,
			ReadTimeout:  time.Duration(cfg.ReadTimeoutMs) * time.Millisecond,
			WriteTimeout: time.Duration(cfg.WriteTimeoutMs) * time.Millisecond,
		},
	}
}

func (s *Server) Start() error {
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
