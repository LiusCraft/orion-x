package xiaozhi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/liuscraft/orion-x/internal/agent"
	"github.com/liuscraft/orion-x/internal/audio"
	"github.com/liuscraft/orion-x/internal/audio/codec"
	"github.com/liuscraft/orion-x/internal/channels"
	"github.com/liuscraft/orion-x/internal/channels/xiaozhi/wsproto"
	"github.com/liuscraft/orion-x/internal/config"
	"github.com/liuscraft/orion-x/internal/knowledge"
	"github.com/liuscraft/orion-x/internal/llm"
	llmprovider "github.com/liuscraft/orion-x/internal/llm/provider"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/memory"
	providerpool "github.com/liuscraft/orion-x/internal/provider"
	"github.com/liuscraft/orion-x/internal/session"
	"github.com/liuscraft/orion-x/internal/task"
	"github.com/liuscraft/orion-x/internal/tools"
	"github.com/liuscraft/orion-x/pkg/pipeline"
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
	cfg  *Config
	deps *channels.Dependencies

	toolsMgr  *tools.Manager
	memorySvc *memory.Service
	sessions  *session.Manager
	tasks     *task.Registry
	providers *providerpool.Pool

	upgrader   websocket.Upgrader
	httpServer *http.Server

	rootCtx    context.Context
	rootCancel context.CancelFunc
	connWG     sync.WaitGroup
}

// NewXiaozhiWSChannel creates a new Xiaozhi WS channel.
func NewXiaozhiWSChannel(cfg *Config, deps *channels.Dependencies, toolsMgr *tools.Manager, memorySvc *memory.Service) *XiaozhiWSChannel {
	sessions := dependencySessions(deps)
	return &XiaozhiWSChannel{
		cfg:       cfg,
		deps:      deps,
		toolsMgr:  toolsMgr,
		memorySvc: memorySvc,
		sessions:  sessions,
		tasks:     dependencyTasks(deps, sessions),
		providers: dependencyProviders(deps),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

func dependencySessions(deps *channels.Dependencies) *session.Manager {
	if deps != nil && deps.Sessions != nil {
		return deps.Sessions
	}
	return session.NewManager()
}

func dependencyTasks(deps *channels.Dependencies, sessions *session.Manager) *task.Registry {
	if deps != nil && deps.Tasks != nil {
		return deps.Tasks
	}
	return task.NewRegistry(sessions)
}

func dependencyProviders(deps *channels.Dependencies) *providerpool.Pool {
	if deps != nil && deps.Providers != nil {
		return deps.Providers
	}
	return providerpool.NewPool()
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

	listener, err := net.Listen("tcp", s.cfg.Server.Addr)
	if err != nil {
		return fmt.Errorf("xiaozhi-channel: listen on %s: %w", s.cfg.Server.Addr, err)
	}

	s.httpServer = &http.Server{
		Addr:    s.cfg.Server.Addr,
		Handler: s.wsHandler(),
	}

	go func() {
		logging.Infof("xiaozhi-channel: listening on %s (ws: %s)", s.cfg.Server.Addr, s.cfg.Server.WsPath)
		if err := s.httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			logging.Errorf("xiaozhi-channel: HTTP server error: %v", err)
		}
	}()

	return nil
}

func (s *XiaozhiWSChannel) wsHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(s.cfg.Server.WsPath, s.handleWS)
	return mux
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
		var rejected *sessionRejectedError
		if errors.As(err, &rejected) {
			s.rejectConnection(rawConn, hello, rejected)
			return
		}
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

// sessionRejectedError 表示计费准入拒绝了这次会话。它要走一条特殊路径。
type sessionRejectedError struct {
	SessionID string
	DeviceID  string
	Reason    string
}

func (e *sessionRejectedError) Error() string {
	return fmt.Sprintf("session %s rejected: %s", e.SessionID, e.Reason)
}

// rejectConnection 干净地拒绝一次连接（§15.2）。
//
// wsproto 的 hello 里没有 error 字段，所以只能：先照常回一条 hello（不回的话设备
// 会一直卡在等 hello 的状态），紧接着发一个 Close 帧，code 1008（policy violation），
// reason 用稳定的机器可读串。同时不建 session、不建 pipeline、不写 reservation。
func (s *XiaozhiWSChannel) rejectConnection(rawConn *websocket.Conn, hello *wsproto.HelloMessage, rejected *sessionRejectedError) {
	safeConn := NewSafeConn(rawConn)
	if err := writeHelloResponse(safeConn, rejected.SessionID, hello.Mode, hello); err != nil {
		logging.Warnf("xiaozhi-channel[%s]: send hello before rejection failed: %v", rejected.SessionID, err)
	}
	deadline := time.Now().Add(2 * time.Second)
	_ = rawConn.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.ClosePolicyViolation, rejected.Reason),
		deadline,
	)
	logging.Warnf("xiaozhi-channel[%s]: connection rejected (device_id=%q, reason=%s)",
		rejected.SessionID, rejected.DeviceID, rejected.Reason)
}

// authorizeBilling 做会话准入校验。返回 (nil, nil) 表示这个会话不计费。
func (s *XiaozhiWSChannel) authorizeBilling(sessionID string, hello *wsproto.HelloMessage) (channels.BillingSession, error) {
	if s.deps == nil || s.deps.Billing == nil {
		return nil, nil // 计费关闭
	}
	// Start 之前 rootCtx 还是 nil（单测里直接建连接就是这个情形），给个兵底上下文，
	// 别把 nil ctx 交给 HTTP 客户端。
	ctx := s.rootCtx
	if ctx == nil {
		ctx = context.Background()
	}
	billSess := s.deps.Billing(ctx, channels.BillingSessionMeta{
		DeviceID:  hello.DeviceID,
		SessionID: sessionID,
		Channel:   "xiaozhi",
	})
	if billSess == nil {
		return nil, nil
	}
	resp, err := billSess.Authorize(ctx)
	if err != nil {
		// 数据面不该因为计费请求失败而拒绝服务，client 层已经按 fail open 处理；
		// 这里只是兵底记一笔，继续把会话跑起来。
		logging.Errorf("xiaozhi-channel[%s]: billing authorize error: %v", sessionID, err)
		return billSess, nil
	}
	if !resp.Allowed {
		billSess.Close()
		return nil, &sessionRejectedError{
			SessionID: sessionID,
			DeviceID:  hello.DeviceID,
			Reason:    string(resp.RejectReason),
		}
	}
	return billSess, nil
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
	s.sessions.Add(sess)
	connectionReady := false
	defer func() {
		if !connectionReady {
			s.sessions.CloseSession(sessionID)
		}
	}()

	// 计费准入：会话建立时就校验余额与价格，拒绝就干净地关掉连接，不建 pipeline、
	// 不写 reservation（§14.3 / §15.2）。
	billSess, err := s.authorizeBilling(sessionID, hello)
	if err != nil {
		return nil, err
	}
	billEnv := newConnectionBilling(billSess)

	// Recognizer
	recognizer, err := s.providers.GetOrCreateASR(connCfg.Provider.ASR.Type, connCfg.Provider.ASR.Aliyun)
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
	ttsProvider, err := s.providers.GetOrCreateTTS(connCfg.Provider.TTS.Type, connCfg.Provider.TTS.Aliyun, ttsSampleRate)
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

	// TTS 计量点在 dispatcher 出队处（分句已定稿、合成必然发生）：已发给厂商的
	// 那部分就算数，用户打断的是播放（§13 打断怎么算账）。
	ttsProc.OnSynthesis(func(fact audio.Synthesis) {
		channels.RecordTTSSynthesis(billSess, int64(fact.Runes), nil)
	})

	audioSrc := NewWSAudioSource(inputCodec, clientSampleRate)
	safeConn := NewSafeConn(rawConn)

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
	// 计费的兌底 flush（每 30 秒一次，防止超长会话一直在内存里堆）。
	billEnv.start(ctx)

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
		Provider:        connCfg.Provider.LLM.Type,
		APIKey:          connCfg.Provider.LLM.OpenAI.APIKey,
		BaseURL:         connCfg.Provider.LLM.OpenAI.BaseURL,
		Model:           connCfg.Provider.LLM.OpenAI.Model,
		SoulPrompt:      connCfg.Provider.LLM.OpenAI.SoulPrompt,
		RulesPrompt:     connCfg.Provider.LLM.OpenAI.RulesPrompt,
		ExtraFields:     connCfg.Provider.LLM.OpenAI.ExtraFields,
		ProviderOptions: connCfg.Provider.LLM.OpenAI.Options,
		Thinking:        connCfg.Provider.LLM.OpenAI.Thinking,
		MaxOutputTokens: connCfg.Provider.LLM.OpenAI.MaxOutputTokens,
	}
	logging.Infof(
		"xiaozhi-channel[%s]: agent configured (model=%q, dialect=%s, thinking_enabled=%t, thinking_effort=%s)",
		sessionID,
		connAgentCfg.Model,
		llmprovider.InferDialect(connAgentCfg.Model),
		connAgentCfg.Thinking.Mode == llm.ThinkingModeEnabled,
		connAgentCfg.Thinking.Effort,
	)
	llmClient, err := s.providers.GetOrCreateLLM(ctx, connCfg.Provider.LLM.Type, connCfg.Provider.LLM.OpenAI)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("get LLM client: %w", err)
	}
	taskMemory := memSvc
	if taskMemory == nil {
		taskMemory = s.memorySvc
	}
	for _, spec := range s.tasks.ToolSpecs(sessionID, func(_ context.Context, taskRecord *task.Task) error {
		taskMgr := connMgr.Clone()
		taskAgent, err := agent.NewWithClient(connAgentCfg, llmClient, taskMgr, taskMemory)
		if err != nil {
			return err
		}
		taskSession := session.New(session.SessionMeta{Model: connAgentCfg.Model})
		taskSession.Add(session.Message{Role: session.RoleUser, Content: taskRecord.Title})
		subAgent := agent.NewSubAgent("sub_"+taskRecord.ID, taskRecord.ID, taskAgent)
		if err := subAgent.Start(s.rootCtx, taskSession); err != nil {
			return err
		}
		return s.tasks.AttachSubAgent(taskRecord.ID, subAgent)
	}) {
		connMgr.Registry().Add(spec)
	}
	var connAgent *agent.Agent
	if memSvc != nil {
		connAgent, err = agent.NewWithClient(connAgentCfg, llmClient, connMgr, memSvc)
	} else {
		connAgent, err = agent.NewWithClient(connAgentCfg, llmClient, connMgr, s.memorySvc)
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

	outputGateway := NewWSOutputStage(safeConn, sessionID, outputCodec, ttsSampleRate, frameDurationMs, preBufferFrames)
	pl, err := pipeline.NewDAGBuilder().
		AddStage(audio.NewASRStage(asrProc, audioSrc)).
		AddStage(agent.NewAgentStage(connAgent, sess)).
		AddStage(audio.NewTTSStage(ttsProc)).
		AddStage(audio.NewOutputStage()).
		Connect("asr", "agent").
		Connect("asr", "session_output").
		Connect("agent", "tts").
		Connect("agent", "session_output").
		Connect("tts", "session_output").
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
	sess.Pipeline = pl

	go func() {
		for msg := range pl.Output() {
			outputGateway.Handle(msg)
			if msg.IsError() {
				logging.Warnf("xiaozhi-channel[%s]: pipeline error: %v", sessionID, msg.Metadata.Error)
			}
			if billEnv.observe(msg) {
				// 本地预算撞到 100%：已经发生的用量照常计费，熔断只拦后续 turn
				// （§13 / §15.4）。中断 TTS 播放与 agent，然后走 flush → settle → close。
				logging.Errorf("xiaozhi-channel[%s]: billing budget exhausted, interrupting session", sessionID)
				_ = ttsProc.Interrupt()
				select {
				case pl.Input() <- pipeline.Message{
					Type:     pipeline.MessageTypeInterrupt,
					Metadata: pipeline.Metadata{Timestamp: time.Now()},
				}:
				default:
				}
				cancel()
				return
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

	connectionReady = true
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
		sessions:  s.sessions,
		output:    outputGateway,
		billing:   billEnv,
		ctx:       ctx,
		cancel:    cancel,
	}, nil
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
	safeConn  *SafeConn
	sessionID string
	mode      wsproto.Mode

	asrProc  audio.ASRProcessor
	ttsProc  audio.TTSProcessor
	pl       pipeline.Pipeline
	audioSrc *WSAudioSource

	connMgr   *tools.Registry
	connAgent *agent.Agent
	memSvc    *memory.Service
	iotMgr    *iotManager
	deviceMCP *deviceMCPClient
	sessions  *session.Manager
	output    *WSOutputStage

	// billing 是这次会话的计费接线（nil = 本次会话不计费）。
	billing *connectionBilling

	ctx    context.Context
	cancel context.CancelFunc
}

func (c *wsConnection) sendHelloResponse(hello *wsproto.HelloMessage) error {
	return writeHelloResponse(c.safeConn, c.sessionID, c.mode, hello)
}

// writeHelloResponse 回一条 hello。它被两处用到：正常建连接，以及计费拒绝时的
// “先回 hello 再关连接”（§15.2）。后者手里还没有 wsConnection，所以拆成包级函数。
func writeHelloResponse(conn *SafeConn, sessionID string, mode wsproto.Mode, hello *wsproto.HelloMessage) error {
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
	if mode == "" {
		mode = wsproto.ModeAuto
	}
	resp := wsproto.NewHelloResponse(sessionID, wsproto.AudioParams{
		Format:        format,
		SampleRate:    ttsSampleRate,
		Channels:      ch,
		FrameDuration: frameDurationMs,
		BitsPerSample: supportedBitsPerSample,
	}, mode, "")
	return conn.WriteJSON(resp)
}

func (c *wsConnection) close() {
	logging.Infof("xiaozhi-channel[%s]: cleaning up connection resources", c.sessionID)
	if c.sessions != nil {
		c.sessions.CloseSession(c.sessionID)
	} else if c.pl != nil {
		if err := c.pl.Stop(); err != nil {
			logging.Warnf("xiaozhi-channel[%s]: stop pipeline error: %v", c.sessionID, err)
		}
	}
	if c.output != nil {
		c.output.Close()
	}
	if c.ttsProc != nil {
		if err := c.ttsProc.Stop(); err != nil {
			logging.Warnf("xiaozhi-channel[%s]: stop TTSProcessor error: %v", c.sessionID, err)
		}
	}
	if c.audioSrc != nil {
		_ = c.audioSrc.Close()
	}
	// 交账：先 flush 再 settle。放在最后一步，确保这一轮的事实都已经进缓冲了。
	c.billing.close("client_close")
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
		// 本地估算到 90% 就不再接受新的 listen 窗口，当前这个 turn 让它走完（§15.4）。
		if c.billing.nearLimit() {
			logging.Warnf("xiaozhi-channel[%s]: billing budget near limit, refusing listen start", c.sessionID)
			return
		}
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
		if c.billing.nearLimit() {
			logging.Warnf("xiaozhi-channel[%s]: billing budget near limit, dropping injected text", c.sessionID)
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
