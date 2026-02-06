package wsserver

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
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
	server := &Server{
		cfg:      cfg,
		sessions: make(map[string]*Session),
	}
	server.upgrader = websocket.Upgrader{
		CheckOrigin: server.isOriginAllowed,
	}
	return server
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
	if !s.registerSession(session) {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("session id already exists"))
		_ = conn.Close()
		return
	}
	go func() {
		defer s.unregisterSession(sessionID)
		session.Run()
	}()
}

func (s *Server) registerSession(session *Session) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.sessions[session.ID()]; exists {
		return false
	}
	s.sessions[session.ID()] = session
	return true
}

func (s *Server) unregisterSession(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sessionID)
}

func (s *Server) isOriginAllowed(r *http.Request) bool {
	originConfig := s.cfg.Server.OriginCheck
	if !originConfig.Enabled {
		return true
	}

	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return false
	}
	parsedOrigin, ok := parseOrigin(origin)
	if !ok {
		return false
	}
	if isSameOrigin(r, parsedOrigin) {
		return true
	}
	if len(originConfig.AllowedOrigins) == 0 {
		return false
	}

	for _, allowed := range originConfig.AllowedOrigins {
		parsedAllowed, ok := parseOrigin(allowed)
		if !ok {
			continue
		}
		if originMatches(parsedOrigin, parsedAllowed) {
			return true
		}
	}
	return false
}

func requestScheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

func defaultPort(scheme string) string {
	switch strings.ToLower(strings.TrimSpace(scheme)) {
	case "https":
		return "443"
	case "http":
		return "80"
	default:
		return ""
	}
}

func splitHostPort(hostport string) (string, string) {
	hostport = strings.TrimSpace(hostport)
	if hostport == "" {
		return "", ""
	}
	host, port, err := net.SplitHostPort(hostport)
	if err == nil {
		return host, port
	}
	return hostport, ""
}

type originParts struct {
	scheme string
	host   string
	port   string
}

func parseOrigin(origin string) (originParts, bool) {
	origin = strings.TrimSpace(origin)
	if origin == "" {
		return originParts{}, false
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return originParts{}, false
	}
	host, port := splitHostPort(parsed.Host)
	if port == "" {
		port = defaultPort(parsed.Scheme)
	}
	if host == "" || port == "" {
		return originParts{}, false
	}
	return originParts{
		scheme: strings.ToLower(parsed.Scheme),
		host:   strings.ToLower(host),
		port:   port,
	}, true
}

func originMatches(a, b originParts) bool {
	return a.scheme == b.scheme && a.host == b.host && a.port == b.port
}

func isSameOrigin(r *http.Request, origin originParts) bool {
	reqHost := strings.TrimSpace(r.Host)
	if reqHost == "" {
		return false
	}
	reqHostOnly, reqPort := splitHostPort(reqHost)
	if reqPort == "" {
		reqPort = defaultPort(requestScheme(r))
	}
	if reqHostOnly == "" || reqPort == "" {
		return false
	}
	reqScheme := strings.ToLower(requestScheme(r))
	return origin.scheme == reqScheme &&
		strings.EqualFold(origin.host, reqHostOnly) &&
		origin.port == reqPort
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
