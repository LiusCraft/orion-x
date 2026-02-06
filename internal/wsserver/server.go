package wsserver

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/liuscraft/orion-x/internal/config"
	"github.com/liuscraft/orion-x/internal/logging"
)

type Server struct {
	cfg      *config.AppConfig
	upgrader websocket.Upgrader

	mu       sync.Mutex
	sessions map[string]*Session
	server   *http.Server
}

func NewServer(cfg *config.AppConfig) *Server {
	return &Server{
		cfg: cfg,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		sessions: make(map[string]*Session),
	}
}

func (s *Server) Start() error {
	if s.cfg == nil {
		return errors.New("wsserver: config is nil")
	}

	mux := http.NewServeMux()
	path := s.cfg.Server.Path
	mux.HandleFunc(path, s.handleWebSocket)

	s.server = &http.Server{
		Addr:         s.cfg.Server.Address,
		Handler:      mux,
		ReadTimeout:  time.Duration(s.cfg.Server.ReadTimeoutMs) * time.Millisecond,
		WriteTimeout: time.Duration(s.cfg.Server.WriteTimeoutMs) * time.Millisecond,
	}

	logging.Infof("WebSocket server listening on %s%s", s.cfg.Server.Address, path)
	return s.server.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	for _, session := range s.sessions {
		session.Close()
	}
	s.sessions = make(map[string]*Session)
	s.mu.Unlock()

	if s.server == nil {
		return nil
	}
	return s.server.Shutdown(ctx)
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		logging.Errorf("WebSocket upgrade failed: %v", err)
		return
	}

	deviceID := getDeviceID(r)
	if strings.TrimSpace(deviceID) == "" {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("参数错误!请检查header以及body参数"))
		_ = conn.Close()
		return
	}

	if !s.isAuthorized(r, deviceID) {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("unauthorized"))
		_ = conn.Close()
		return
	}

	clientID := r.URL.Query().Get("client-id")
	sessionID := r.Header.Get("X-Reqid")
	if sessionID == "" {
		sessionID = r.Header.Get("x-reqid")
	}
	if sessionID == "" {
		sessionID = uuid.New().String()
	}

	session := NewSession(s.cfg, conn, deviceID, clientID, sessionID)
	s.registerSession(session)
	go func() {
		defer s.unregisterSession(sessionID)
		session.Run()
	}()
}

func (s *Server) registerSession(session *Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[session.ID()] = session
}

func (s *Server) unregisterSession(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sessionID)
}

func (s *Server) isAuthorized(r *http.Request, deviceID string) bool {
	auth := s.cfg.Server.Auth
	if !auth.Enabled {
		return true
	}

	for _, allowed := range auth.AllowedDevices {
		if strings.TrimSpace(allowed) == deviceID {
			return true
		}
	}

	header := r.Header.Get("Authorization")
	if header == "" {
		return false
	}
	const prefix = "bearer "
	if !strings.HasPrefix(strings.ToLower(header), prefix) {
		return false
	}
	token := strings.TrimSpace(header[len(prefix):])
	return token != "" && token == auth.Token
}

func getDeviceID(r *http.Request) string {
	deviceID := r.Header.Get("device-id")
	if deviceID == "" {
		deviceID = r.Header.Get("Device-Id")
	}
	if deviceID == "" {
		deviceID = r.Header.Get("Device-ID")
	}
	if deviceID == "" {
		deviceID = r.URL.Query().Get("device-id")
	}
	return strings.TrimSpace(deviceID)
}
