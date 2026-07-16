package xiaozhi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/liuscraft/orion-x/internal/agent"
	"github.com/liuscraft/orion-x/internal/audio"
	"github.com/liuscraft/orion-x/internal/audio/codec"
	"github.com/liuscraft/orion-x/internal/channels"
	xstages "github.com/liuscraft/orion-x/internal/channels/xiaozhi/stages"
	"github.com/liuscraft/orion-x/internal/config"
	"github.com/liuscraft/orion-x/internal/knowledge"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/memory"
	"github.com/liuscraft/orion-x/internal/provider/asr"
	"github.com/liuscraft/orion-x/internal/provider/tts"
	"github.com/liuscraft/orion-x/internal/session"
	"github.com/liuscraft/orion-x/internal/tools"
	"github.com/liuscraft/orion-x/pkg/pipeline"
	"github.com/liuscraft/orion-x/internal/channels/xiaozhi/wsproto"
)

// independent of cmd/voicebot's 22050Hz (each entry point constructs its
// own tts.Provider instance, so the two don't need to agree).
const ttsSampleRate = 16000

// defaultAudioFormat is used when a client's hello omits audio_params.format.
const defaultAudioFormat = codec.FormatOpus

// defaultFrameDurationMs is used when a client's hello omits (or sends a
// non-positive) audio_params.frame_duration.
const defaultFrameDurationMs = 60

// supportedBitsPerSample is the only bits_per_sample value this server
// accepts — the entire pipeline (codec, resampler, ASR/TTS providers) is
// hardcoded to PCM16LE.
const supportedBitsPerSample = 16

const (
	defaultPreBufferFrames = 3
	minPreBufferFrames     = 1
	maxPreBufferFrames     = 100
	helloTimeout           = 10 * time.Second
)

// XiaozhiWSChannel implements channels.Channel for the Xiaozhi ESP32
// WebSocket voice protocol. It manages an HTTP server for WebSocket upgrades
// and creates per-connection DAG pipelines (ASR → Agent → TTS → output).
type XiaozhiWSChannel struct {
	cfg   *Config
	deps  *channels.Dependencies

	toolsMgr  *tools.Manager
	memorySvc *memory.Service

	upgrader   websocket.Upgrader
	httpServer *http.Server

	rootCtx    context.Context
	rootCancel context.CancelFunc
	connWG     sync.WaitGroup
}

// NewXiaozhiWSChannel creates a new Xiaozhi WS channel.
func NewXiaozhiWSChannel(cfg *Config, deps *channels.Dependencies, toolsMgr *tools.Manager, memorySvc *memory.Service) *XiaozhiWSChannel {
	return &XiaozhiWSChannel{
		cfg:       cfg,
		deps:      deps,
		toolsMgr:  toolsMgr,
		memorySvc: memorySvc,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

// Name returns the channel identifier.
func (s *XiaozhiWSChannel) Name() string { return "xiaozhi" }

// Info returns channel metadata.
func (s *XiaozhiWSChannel) Info() channels.ChannelInfo {
	return channels.NewChannelInfo(
		"xiaozhi",
		"Xiaozhi WebSocket",
		channels.ChannelServer,
		[]channels.Capability{channels.CapText, channels.CapAudioStream},
	)
}

// Start starts the HTTP server and begins accepting WebSocket connections.
func (s *XiaozhiWSChannel) Start(ctx context.Context) error {
	s.rootCtx, s.rootCancel = context.WithCancel(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc(s.cfg.Server.WsPath, s.handleWS)

	s.httpServer = &http.Server{
		Addr:    s.cfg.Server.Addr,
		Handler: mux,
	}

	go func() {
		logging.Infof("xiaozhi-channel: listening on %s (ws: %s)", s.cfg.Server.Addr, s.cfg.Server.WsPath)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logging.Errorf("xiaozhi-channel: HTTP server error: %v", err)
		}
	}()

	return nil
}

// Stop gracefully shuts down the HTTP server and waits for all connections
// to finish, or until the context is cancelled.
func (s *XiaozhiWSChannel) Stop(ctx context.Context) error {
	if s.rootCancel != nil {
		s.rootCancel()
	}

	if s.httpServer != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = s.httpServer.Shutdown(shutdownCtx)
	}

	done := make(chan struct{})
	go func() {
		s.connWG.Wait()
		close(done)
	}()

	select {
	case <-done:
		logging.Infof("xiaozhi-channel: all connections closed cleanly")
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

// handleWS upgrades an HTTP request to WebSocket and handles it asynchronously.
func (s *XiaozhiWSChannel) handleWS(w http.ResponseWriter, r *http.Request) {
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

	logging.Infof("xiaozhi-channel: incoming connection — Authorization=%q ProtocolVersion=%q DeviceId=%q ClientId=%q RemoteAddr=%s",
		authorization, protocolVersion, deviceID, clientID, r.RemoteAddr)

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		logging.Warnf("xiaozhi-channel: upgrade failed: %v", err)
		return
	}

	s.connWG.Add(1)
	go func() {
		defer s.connWG.Done()
		s.handleConnection(conn)
	}()
}

// handleConnection handles the full lifecycle of a single WebSocket connection.
func (s *XiaozhiWSChannel) handleConnection(rawConn *websocket.Conn) {
	defer func() { _ = rawConn.Close() }()

	hello, err := s.readHello(rawConn)
	if err != nil {
		logging.Warnf("xiaozhi-channel: handshake failed: %v", err)
		return
	}

	c, err := s.newConnection(rawConn, hello)
	if err != nil {
		logging.Errorf("xiaozhi-channel: connection setup failed: %v", err)
		return
	}
	defer c.close()

	if err := c.sendHelloResponse(hello); err != nil {
		logging.Warnf("xiaozhi-channel[%s]: send hello response failed: %v", c.sessionID, err)
		return
	}

	logging.Infof("xiaozhi-channel[%s]: connection established (device_id=%q, mode=%s)", c.sessionID, hello.DeviceID, c.mode)
	c.readLoop()
	logging.Infof("xiaozhi-channel[%s]: connection closed", c.sessionID)
}

// newConnection builds all per-connection resources and the DAG pipeline.
func (s *XiaozhiWSChannel) newConnection(rawConn *websocket.Conn, hello *wsproto.HelloMessage) (*wsConnection, error) {
	if bps := hello.AudioParams.BitsPerSample; bps != 0 && bps != supportedBitsPerSample {
		return nil, fmt.Errorf("unsupported bits_per_sample %d (only %d is supported)", bps, supportedBitsPerSample)
	}

	if hello.DeviceID == "" {
		return nil, fmt.Errorf("device_id is required")
	}

	// Load device config — depends on deps or s.deps
	var connCfg *config.AppConfig
	if s.deps != nil && s.deps.DeviceCfgLoader != nil {
		var err error
		connCfg, err = s.deps.DeviceCfgLoader.LoadConfig(hello.DeviceID)
		if err != nil {
			return nil, fmt.Errorf("load device config for %q: %w", hello.DeviceID, err)
		}
		if connCfg == nil {
			return nil, fmt.Errorf("device %q is not registered", hello.DeviceID)
		}
	} else {
		// Fallback to a default config if no loader available (for standalone testing)
		connCfg = config.DefaultConfig()
	}

	mode := hello.Mode
	if mode == "" {
		mode = wsproto.ModeAuto
	}

	format := hello.AudioParams.Format
	if format == "" {
		format = string(defaultAudioFormat)
	}
	clientSampleRate := hello.AudioParams.SampleRate
	if clientSampleRate <= 0 {
		clientSampleRate = audio.InternalSampleRate
	}
	ch := hello.AudioParams.Channels
	if ch <= 0 {
		ch = audio.InternalChannels
	}
	frameDurationMs := hello.AudioParams.FrameDuration
	if frameDurationMs <= 0 {
		frameDurationMs = defaultFrameDurationMs
	}
	preBufferFrames := computePreBufferFrames(hello.AudioParams.PlayBufferDuration, frameDurationMs)

	inputCodec, err := codec.New(codec.Format(format), clientSampleRate, ch, frameDurationMs)
	if err != nil {
		return nil, err
	}
	outputCodec, err := codec.New(codec.Format(format), ttsSampleRate, ch, frameDurationMs)
	if err != nil {
		return nil, err
	}

	sess := session.New(session.SessionMeta{Model: connCfg.Provider.LLM.OpenAI.Model})
	sessionID := sess.ID

	// Recognizer
	recognizer, err := s.newRecognizer(connCfg)
	if err != nil {
		return nil, err
	}

	inPipeCfg := connCfg.Audio.InPipe
	asrProc, err := audio.NewASRProcessor(&audio.ASRConfig{
		EnableVAD:       mode == wsproto.ModeAuto,
		VADThreshold:    inPipeCfg.VADThreshold,
		VADType:         inPipeCfg.VADType,
		VADModelPath:    inPipeCfg.VADModelPath,
		VADMinSilenceMs: inPipeCfg.VADMinSilenceMs,
		VADSpeechPadMs:  inPipeCfg.VADSpeechPadMs,
		Recognizer:      recognizer,
	})
	if err != nil {
		return nil, err
	}

	// TTS provider + processor
	ttsProvider, err := s.newTTSProvider(connCfg)
	if err != nil {
		return nil, err
	}

	ttsPipeCfg := connCfg.Audio.TTSPipeline
	queueSize := ttsPipeCfg.TextQueueSize
	if queueSize <= 0 {
		queueSize = 100
	}
	ttsProcessorCfg := audio.DefaultTTSConfig()
	ttsProcessorCfg.Provider = ttsProvider
	ttsProcessorCfg.QueueSize = queueSize
	ttsProc, err := audio.NewTTSProcessor(ttsProcessorCfg)
	if err != nil {
		return nil, err
	}

	audioSrc := xstages.NewWSAudioSource(inputCodec, clientSampleRate)
	safeConn := xstages.NewSafeConn(rawConn)

	connMgr := s.toolsMgr.Clone()
	iotMgr := newIoTManager(safeConn, connMgr.Registry())
	var devMCP *deviceMCPClient

	userID := hello.DeviceID
	if userID == "" {
		userID = sessionID
	}

	memCtx := memory.WithContext(s.rootCtx, memory.Context{
		UserID:    userID,
		SessionID: sessionID,
		DeviceID:  hello.DeviceID,
	})
	ctx, cancel := context.WithCancel(memCtx)
	go func() {
		<-ctx.Done()
		_ = rawConn.Close()
	}()

	// Load MCP servers from device config
	mcpCfgs := connCfg.Tools.MCP
	if len(mcpCfgs) > 0 {
		toolCfgs := make([]tools.MCPServerConfig, len(mcpCfgs))
		for i, m := range mcpCfgs {
			toolCfgs[i] = tools.MCPServerConfig{
				ID:           m.ID,
				Transport:    m.Transport,
				Command:      m.Command,
				Args:         m.Args,
				Env:          m.Env,
				CWD:          m.CWD,
				Endpoint:     m.Endpoint,
				Headers:      m.Headers,
				ToolNameList: m.ToolNameList,
				TimeoutMs:    m.TimeoutMs,
			}
		}
		if err := connMgr.RegisterMCPServers(ctx, toolCfgs); err != nil {
			cancel()
			logging.Errorf("xiaozhi-channel[%s]: failed to register MCP servers: %v", sessionID, err)
		}
		defs := connMgr.Registry().Definitions()
		logging.Infof("xiaozhi-channel[%s]: MCP registration complete — total tools: %d", sessionID, len(defs))
	} else {
		logging.Infof("xiaozhi-channel[%s]: no MCP servers in config, total tools: %d", sessionID, len(connMgr.Registry().Definitions()))
	}

	if hello.Features["mcp"] {
		devMCP = newDeviceMCPClient(safeConn, sessionID, connMgr.Registry())
	}

	// Per-connection memory service
	deviceID := hello.DeviceID
	memSvc, err := memory.NewService(memory.Config{
		MemoryCharLimit: connCfg.Memory.MemoryCharLimit,
		UserCharLimit:   connCfg.Memory.UserCharLimit,
	}, memory.Options{
		ManagerURL:   managerURLFromDeps(s.deps),
		DeviceID:     deviceID,
		ReviewConfig: memory.ReviewConfig{Enabled: true},
	})
	if err != nil {
		logging.Warnf("xiaozhi-channel[%s]: memory init: %v", sessionID, err)
		memSvc = nil
	}

	connAgentCfg := agent.Config{
		Provider:    connCfg.Provider.LLM.Type,
		APIKey:      connCfg.Provider.LLM.OpenAI.APIKey,
		BaseURL:     connCfg.Provider.LLM.OpenAI.BaseURL,
		Model:       connCfg.Provider.LLM.OpenAI.Model,
		SoulPrompt:  connCfg.Provider.LLM.OpenAI.SoulPrompt,
		RulesPrompt: connCfg.Provider.LLM.OpenAI.RulesPrompt,
		ExtraFields: connCfg.Provider.LLM.OpenAI.ExtraFields,
	}
	var connAgent *agent.Agent
	if memSvc != nil {
		connAgent, err = agent.New(ctx, connAgentCfg, connMgr, memSvc)
	} else {
		connAgent, err = agent.New(ctx, connAgentCfg, connMgr, s.memorySvc)
	}
	if err != nil {
		cancel()
		return nil, fmt.Errorf("create per-connection agent: %w", err)
	}

	if memSvc != nil && memSvc.CuratedStore() != nil {
		store := memSvc.CuratedStore()
		connAgent.RegisterBuiltinTool(tools.MemoryToolSpec(store))
		connAgent.RegisterBuiltinTool(tools.SessionSearchToolSpec(managerURLFromDeps(s.deps), deviceID))
		logging.Infof("xiaozhi-channel[%s]: registered memory system tools", sessionID)
	}

	knowClient := knowledge.NewSearchClient(managerURLFromDeps(s.deps), deviceID)
	connAgent.RegisterBuiltinTool(tools.KnowledgeSearchToolSpec(knowClient))
	logging.Infof("xiaozhi-channel[%s]: registered knowledge search tool", sessionID)

	if err := ttsProc.Start(ctx); err != nil {
		cancel()
		return nil, err
	}

	pl, err := pipeline.NewDAGBuilder().
		AddStage(audio.NewASRStage(asrProc, audioSrc)).
		AddStage(agent.NewAgentStage(connAgent, sess)).
		AddStage(audio.NewTTSStage(ttsProc)).
		AddStage(xstages.NewWSOutputStage(safeConn, sessionID, outputCodec, ttsSampleRate, frameDurationMs, preBufferFrames)).
		Connect("asr", "agent").
		Connect("asr", "ws_output").
		Connect("agent", "tts").
		Connect("agent", "ws_output").
		Connect("tts", "ws_output").
		SetObserver(pipeline.NewLoggingObserver(false)).
		Build()
	if err != nil {
		_ = ttsProc.Stop()
		cancel()
		return nil, err
	}

	if err := pl.Start(ctx); err != nil {
		_ = ttsProc.Stop()
		cancel()
		return nil, err
	}

	go func() {
		for msg := range pl.Output() {
			if msg.IsError() {
				logging.Warnf("xiaozhi-channel[%s]: pipeline error: %v", sessionID, msg.Metadata.Error)
			}
		}
	}()

	if devMCP != nil {
		go func() {
			if err := devMCP.Initialize(ctx); err != nil {
				logging.Warnf("xiaozhi-channel[%s]: device MCP initialize failed: %v", sessionID, err)
			}
		}()
	}

	return &wsConnection{
		rawConn:   rawConn,
		safeConn:  safeConn,
		sessionID: sessionID,
		mode:      mode,
		asrProc:   asrProc,
		ttsProc:   ttsProc,
		pl:        pl,
		audioSrc:  audioSrc,
		connMgr:   connMgr.Registry(),
		connAgent: connAgent,
		memSvc:    memSvc,
		iotMgr:    iotMgr,
		deviceMCP: devMCP,
		ctx:       ctx,
		cancel:    cancel,
	}, nil
}

func (s *XiaozhiWSChannel) newRecognizer(cfg *config.AppConfig) (asr.Recognizer, error) {
	asrCfg := cfg.Provider.ASR.Aliyun
	return asr.NewRecognizer(asr.ProviderConfig{
		Type: cfg.Provider.ASR.Type,
		Config: asr.Config{
			APIKey:     asrCfg.APIKey,
			Endpoint:   asrCfg.Endpoint,
			Model:      asrCfg.Model,
			Format:     "pcm",
			SampleRate: audio.InternalSampleRate,
		},
	})
}

func (s *XiaozhiWSChannel) newTTSProvider(cfg *config.AppConfig) (tts.Provider, error) {
	ttsCfg := cfg.Provider.TTS.Aliyun
	return tts.NewProvider(tts.ProviderConfig{
		Type: cfg.Provider.TTS.Type,
		Config: tts.Config{
			APIKey:               ttsCfg.APIKey,
			Endpoint:             ttsCfg.Endpoint,
			Workspace:            ttsCfg.Workspace,
			Model:                ttsCfg.Model,
			Voice:                ttsCfg.Voice,
			Format:               "pcm",
			SampleRate:           ttsSampleRate,
			Volume:               ttsCfg.Volume,
			Rate:                 ttsCfg.Rate,
			Pitch:                ttsCfg.Pitch,
			EnableSSML:           ttsCfg.EnableSSML,
			TextType:             ttsCfg.TextType,
			EnableDataInspection: ttsCfg.EnableDataInspection,
		},
	})
}

// readHello blocks for the first text frame and parses it as a hello.
func (s *XiaozhiWSChannel) readHello(conn *websocket.Conn) (*wsproto.HelloMessage, error) {
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

// computePreBufferFrames converts playback buffer duration to a frame count.
func computePreBufferFrames(playBufferDurationMs, frameDurationMs int) int {
	if playBufferDurationMs <= 0 || frameDurationMs <= 0 {
		return defaultPreBufferFrames
	}
	n := playBufferDurationMs / frameDurationMs
	if n < minPreBufferFrames {
		n = minPreBufferFrames
	}
	if n > maxPreBufferFrames {
		n = maxPreBufferFrames
	}
	return n
}

// managerURLFromDeps extracts the manager URL from dependencies.
func managerURLFromDeps(deps *channels.Dependencies) string {
	if deps != nil && deps.DeviceCfgLoader != nil {
		return deps.DeviceCfgLoader.ManagerURL()
	}
	return ""
}

// wsConnection holds all per-connection state and resources.
type wsConnection struct {
	rawConn   *websocket.Conn
	safeConn  *xstages.SafeConn
	sessionID string
	mode      wsproto.Mode

	asrProc  audio.ASRProcessor
	ttsProc  audio.TTSProcessor
	pl       pipeline.Pipeline
	audioSrc *xstages.WSAudioSource

	connMgr   *tools.Registry
	connAgent *agent.Agent
	memSvc    *memory.Service
	iotMgr    *iotManager
	deviceMCP *deviceMCPClient

	ctx    context.Context
	cancel context.CancelFunc
}

func (c *wsConnection) sendHelloResponse(hello *wsproto.HelloMessage) error {
	format := hello.AudioParams.Format
	if format == "" {
		format = string(defaultAudioFormat)
	}
	ch := hello.AudioParams.Channels
	if ch <= 0 {
		ch = audio.InternalChannels
	}
	frameDurationMs := hello.AudioParams.FrameDuration
	if frameDurationMs <= 0 {
		frameDurationMs = defaultFrameDurationMs
	}
	resp := wsproto.NewHelloResponse(c.sessionID, wsproto.AudioParams{
		Format:        format,
		SampleRate:    ttsSampleRate,
		Channels:      ch,
		FrameDuration: frameDurationMs,
		BitsPerSample: supportedBitsPerSample,
	}, c.mode, "")
	return c.safeConn.WriteJSON(resp)
}

func (c *wsConnection) close() {
	logging.Infof("xiaozhi-channel[%s]: cleaning up connection resources", c.sessionID)
	if c.pl != nil {
		if err := c.pl.Stop(); err != nil {
			logging.Warnf("xiaozhi-channel[%s]: stop pipeline error: %v", c.sessionID, err)
		}
	}
	if c.ttsProc != nil {
		if err := c.ttsProc.Stop(); err != nil {
			logging.Warnf("xiaozhi-channel[%s]: stop TTSProcessor error: %v", c.sessionID, err)
		}
	}
	if c.audioSrc != nil {
		_ = c.audioSrc.Close()
	}
	if c.memSvc != nil {
		_ = c.memSvc.Close()
	}
	if c.cancel != nil {
		c.cancel()
	}
}

func (c *wsConnection) readLoop() {
	for {
		msgType, data, err := c.rawConn.ReadMessage()
		if err != nil {
			return
		}

		switch msgType {
		case websocket.BinaryMessage:
			c.audioSrc.PushBinaryFrame(data)
		case websocket.TextMessage:
			c.handleTextMessage(data)
		}
	}
}

func (c *wsConnection) handleTextMessage(data []byte) {
	msg, err := wsproto.ParseClientMessage(data)
	if err != nil {
		logging.Warnf("xiaozhi-channel[%s]: ignoring invalid message: %v", c.sessionID, err)
		return
	}

	switch m := msg.(type) {
	case *wsproto.ListenMessage:
		c.handleListen(m)
	case *wsproto.AbortMessage:
		c.handleAbort()
	case *wsproto.HelloMessage:
		logging.Warnf("xiaozhi-channel[%s]: ignoring duplicate hello after handshake", c.sessionID)
	case *wsproto.IoTMessage:
		if len(m.Descriptors) > 0 {
			c.iotMgr.handleDescriptors(m.Descriptors)
		}
		if len(m.States) > 0 {
			c.iotMgr.handleStates(m.States)
		}
	case *wsproto.MCPMessage:
		if c.deviceMCP == nil {
			logging.Warnf("xiaozhi-channel[%s]: received mcp message but device MCP not enabled", c.sessionID)
			return
		}
		raw, err := json.Marshal(m.Payload)
		if err != nil {
			logging.Warnf("xiaozhi-channel[%s]: device mcp payload marshal error: %v", c.sessionID, err)
			return
		}
		c.deviceMCP.HandleMessage(c.ctx, raw)
	}
}

func (c *wsConnection) handleListen(m *wsproto.ListenMessage) {
	switch m.State {
	case wsproto.ListenStart:
		if c.mode == wsproto.ModeManual {
			if err := c.asrProc.BeginTurn(c.ctx); err != nil {
				logging.Warnf("xiaozhi-channel[%s]: BeginTurn failed: %v", c.sessionID, err)
			}
		}
	case wsproto.ListenStop:
		if c.mode == wsproto.ModeManual {
			if err := c.asrProc.EndTurn(c.ctx); err != nil {
				logging.Warnf("xiaozhi-channel[%s]: EndTurn failed: %v", c.sessionID, err)
			}
		}
	case wsproto.ListenDetect:
		if m.Text == "" {
			return
		}
		select {
		case c.pl.Input() <- pipeline.NewMessage(pipeline.MessageTypeData, m.Text):
		case <-c.ctx.Done():
		}
	}
}

func (c *wsConnection) handleAbort() {
	select {
	case c.pl.Input() <- pipeline.Message{
		Type:     pipeline.MessageTypeInterrupt,
		Metadata: pipeline.Metadata{Timestamp: time.Now()},
	}:
	case <-c.ctx.Done():
	}
}
