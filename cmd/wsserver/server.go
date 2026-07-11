package main

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/liuscraft/orion-x/cmd/wsserver/wsproto"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/memory"
	"github.com/liuscraft/orion-x/internal/tools"
)

// helloTimeout bounds how long a client has to send its hello handshake
// message after the WebSocket upgrade completes.
const helloTimeout = 10 * time.Second

// Server holds process-level resources shared across all WebSocket
// connections: the loaded config and a single Agent instance. agent.Agent
// has no per-connection state (see internal/agent/agent.go) and its Run
// method is driven entirely by the *session.Session passed to it, so
// sharing one instance across concurrent connections is safe; each
// connection still gets its own Session, ASR/TTS processors, and pipeline.
//
// rootCtx/rootCancel/connWG exist so Shutdown can proactively tell every
// active connection to wind down instead of the process just exiting out
// from under them: main() returning kills every goroutine immediately,
// giving handleConnection's cleanup defers no chance to run (Recognizer
// connections, TTSProcessor dispatchers, etc. would leak).
type Server struct {
	toolsMgr  *tools.Manager
	memorySvc *memory.Service
	deviceCfg DeviceConfigLoader
	upgrader  websocket.Upgrader

	rootCtx    context.Context
	rootCancel context.CancelFunc
	connWG     sync.WaitGroup
}

func NewServer(toolsMgr *tools.Manager, memorySvc *memory.Service, deviceCfg DeviceConfigLoader) *Server {
	rootCtx, rootCancel := context.WithCancel(context.Background())
	return &Server{
		toolsMgr:   toolsMgr,
		memorySvc:  memorySvc,
		deviceCfg:  deviceCfg,
		rootCtx:    rootCtx,
		rootCancel: rootCancel,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

// HandleWS upgrades an HTTP request to a WebSocket connection and hands it
// off to a new goroutine for the connection's lifetime.
func (s *Server) HandleWS(w http.ResponseWriter, r *http.Request) {
	// Read auth / identity fields from headers first, then fall back to query params.
	pick := func(header, query string) string {
		if v := r.Header.Get(header); v != "" {
			return v
		}
		return r.URL.Query().Get(query)
	}
	authorization := pick("Authorization", "access_token")
	protocolVersion := pick("Protocol-Version", "protocol-version")
	deviceID := pick("Device-Id", "device-id")
	clientID := pick("Client-Id", "client-id")

	logging.Infof("wsserver: incoming connection — Authorization=%q ProtocolVersion=%q DeviceId=%q ClientId=%q RemoteAddr=%s",
		authorization, protocolVersion, deviceID, clientID, r.RemoteAddr)

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		logging.Warnf("wsserver: upgrade failed: %v", err)
		return
	}
	s.connWG.Add(1)
	go func() {
		defer s.connWG.Done()
		s.handleConnection(conn)
	}()
}

// Shutdown cancels every active connection's context (triggering their
// cleanup: pipeline stop, ASR/TTS processor stop, WebSocket close) and
// blocks until they finish or timeout elapses, whichever comes first.
func (s *Server) Shutdown(timeout time.Duration) {
	s.rootCancel()

	done := make(chan struct{})
	go func() {
		s.connWG.Wait()
		close(done)
	}()

	select {
	case <-done:
		logging.Infof("wsserver: all connections closed cleanly")
	case <-time.After(timeout):
		logging.Warnf("wsserver: shutdown timed out waiting for connections to close")
	}
}

// readHello blocks for the first text frame on conn and parses it as a
// hello handshake message. Any other message type, parse failure, or
// timeout is treated as a failed handshake.
func readHello(conn *websocket.Conn) (*wsproto.HelloMessage, error) {
	_ = conn.SetReadDeadline(time.Now().Add(helloTimeout))
	defer func() { _ = conn.SetReadDeadline(time.Time{}) }()

	msgType, data, err := conn.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("read hello: %w", err)
	}
	if msgType != websocket.TextMessage {
		return nil, fmt.Errorf("expected text frame for hello handshake, got message type %d", msgType)
	}

	msg, err := wsproto.ParseClientMessage(data)
	if err != nil {
		return nil, fmt.Errorf("parse hello: %w", err)
	}
	hello, ok := msg.(*wsproto.HelloMessage)
	if !ok {
		return nil, fmt.Errorf("expected hello as the first message, got %T", msg)
	}
	return hello, nil
}
