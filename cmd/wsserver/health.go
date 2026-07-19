package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"

	"github.com/liuscraft/orion-x/internal/logging"
)

type healthServer struct {
	server *http.Server
}

func newHealthServer(addr string) *healthServer {
	return &healthServer{
		server: &http.Server{
			Addr:    addr,
			Handler: healthHandler(),
		},
	}
}

func healthHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return mux
}

// Start binds synchronously so a successful return means the health endpoint
// is immediately reachable by the container runtime.
func (s *healthServer) Start() error {
	listener, err := net.Listen("tcp", s.server.Addr)
	if err != nil {
		return fmt.Errorf("health server listen on %s: %w", s.server.Addr, err)
	}

	go func() {
		logging.Infof("health server listening on %s", s.server.Addr)
		if err := s.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logging.Errorf("health server error: %v", err)
		}
	}()
	return nil
}

func (s *healthServer) Stop(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}
