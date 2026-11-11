// Package realtime implements the local OpenAI Realtime GA WebSocket
// endpoint over the existing ASR → Agent → TTS pipeline.
package realtime

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/liuscraft/orion-x/internal/channels"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/memory"
	providerpool "github.com/liuscraft/orion-x/internal/provider"
	"github.com/liuscraft/orion-x/internal/session"
	"github.com/liuscraft/orion-x/internal/tools"
)

// Channel implements the Realtime GA WebSocket server.
type Channel struct {
	cfg      *Config
	deps     *channels.Dependencies
	toolsMgr *tools.Manager
	memory   *memory.Service

	providers *providerpool.Pool
	sessions  *session.Manager

	upgrader   websocket.Upgrader
	httpServer *http.Server
	rootCtx    context.Context
	rootCancel context.CancelFunc

	connMu sync.Mutex
	conns  map[*connection]struct{}
}

// NewChannel creates a Realtime channel.
func NewChannel(cfg *Config, deps *channels.Dependencies, toolsMgr *tools.Manager, memorySvc *memory.Service) *Channel {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	cfg.normalize()
	if toolsMgr == nil {
		toolsMgr, _ = tools.NewManager(context.Background(), tools.ManagerConfig{})
	}
	providers := providerpool.NewPool()
	sessions := session.NewManager()
	if deps != nil {
		if deps.Providers != nil {
			providers = deps.Providers
		}
		if deps.Sessions != nil {
			sessions = deps.Sessions
		}
	}
	return &Channel{
		cfg:       cfg,
		deps:      deps,
		toolsMgr:  toolsMgr,
		memory:    memorySvc,
		providers: providers,
		sessions:  sessions,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(_ *http.Request) bool { return true },
		},
		conns: make(map[*connection]struct{}),
	}
}

// Name returns the channel identifier.
func (c *Channel) Name() string { return "realtime" }

// Info returns channel metadata.
func (c *Channel) Info() channels.ChannelInfo {
	return channels.NewChannelInfo(
		"realtime",
		"OpenAI Realtime GA",
		channels.ChannelServer,
		[]channels.Capability{channels.CapTextStream, channels.CapAudioStream},
	)
}

// Start starts the Realtime WebSocket server.
func (c *Channel) Start(ctx context.Context) error {
	c.rootCtx, c.rootCancel = context.WithCancel(ctx)
	listener, err := net.Listen("tcp", c.cfg.Addr)
	if err != nil {
		return fmt.Errorf("realtime-channel: listen on %s: %w", c.cfg.Addr, err)
	}
	c.httpServer = &http.Server{Addr: c.cfg.Addr, Handler: c.handler()}
	go func() {
		logging.Infof("realtime-channel: listening on %s (ws: %s)", c.cfg.Addr, c.cfg.Path)
		if err := c.httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			logging.Errorf("realtime-channel: HTTP server error: %v", err)
		}
	}()
	return nil
}

func (c *Channel) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(c.cfg.Path, c.handleWebSocket)
	return mux
}

// Stop gracefully closes the server and active sessions.
func (c *Channel) Stop(ctx context.Context) error {
	if c.rootCancel != nil {
		c.rootCancel()
	}
	if c.httpServer != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_ = c.httpServer.Shutdown(shutdownCtx)
		cancel()
	}
	c.connMu.Lock()
	connections := make([]*connection, 0, len(c.conns))
	for conn := range c.conns {
		connections = append(connections, conn)
	}
	c.connMu.Unlock()
	for _, conn := range connections {
		conn.close()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func (c *Channel) track(conn *connection) {
	c.connMu.Lock()
	c.conns[conn] = struct{}{}
	c.connMu.Unlock()
}

func (c *Channel) untrack(conn *connection) {
	c.connMu.Lock()
	delete(c.conns, conn)
	c.connMu.Unlock()
}
