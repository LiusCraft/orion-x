package wsserver

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/gorilla/websocket"
	"github.com/liuscraft/orion-x/internal/agent"
	"github.com/liuscraft/orion-x/internal/audio"
	"github.com/liuscraft/orion-x/internal/audio/codec"
	audiosink "github.com/liuscraft/orion-x/internal/audio/sink"
	audiosource "github.com/liuscraft/orion-x/internal/audio/source"
	"github.com/liuscraft/orion-x/internal/config"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/memory"
	"github.com/liuscraft/orion-x/internal/metrics"
	_ "github.com/liuscraft/orion-x/internal/provider/llm/register"
	"github.com/liuscraft/orion-x/internal/provider/tts"
	"github.com/liuscraft/orion-x/internal/tools"
	"github.com/liuscraft/orion-x/internal/voicebot"
)

type outboundMessage struct {
	msgType int
	data    []byte
}

type agentRunner interface {
	Process(ctx context.Context, text string) (<-chan agent.AgentEvent, error)
	SummarizeToolResult(ctx context.Context, tool string, args map[string]interface{}, result interface{}) (<-chan agent.AgentEvent, error)
}

const (
	sttStatePartial             = "partial"
	sttStateFinal               = "final"
	interimSTTThrottleInterval  = 200 * time.Millisecond
	interimSTTVADWindow         = 1500 * time.Millisecond
	helloFeatureInterimSTT      = "interim_stt"
	helloFeatureInterimSTTGroup = "stt"
)

type Session struct {
	serverCfg *config.WSServerAppConfig
	voicebot  config.VoicebotSessionConfig
	profileID string
	conn      *websocket.Conn
	deviceID  string
	clientID  string
	sessionID string

	audioParams AudioParams

	writeCh chan outboundMessage
	ctx     context.Context
	cancel  context.CancelFunc
	close   sync.Once

	readTimeout  time.Duration
	writeTimeout time.Duration

	agentRunner  agentRunner
	memorySvc    memory.Service
	toolManager  tools.ToolManager
	toolExecutor tools.ToolExecutor
	orchestrator voicebot.Orchestrator
	audioOutPipe audio.AudioOutPipe
	audioInPipe  audio.AudioInPipe
	wsSource     *audiosource.WebSocketSource
	mixer        audio.AudioMixer
	audioSink    *audiosink.WebSocketSink
	opusDecoder  *codec.OpusDecoder

	wsMetrics       *metrics.WSServerMetrics
	voicebotMetrics *metrics.VoicebotMetrics

	startedAt      time.Time
	lastASRFinalAt time.Time
	lastASRPartial time.Time
	lastASRText    string
	lastVADAt      time.Time
	interimSTT     bool
	asrMu          sync.Mutex

	listening bool
	mu        sync.Mutex
}

func NewSession(serverCfg *config.WSServerAppConfig, voicebot config.VoicebotSessionConfig, profileID string, conn *websocket.Conn, deviceID, clientID, sessionID string, wsMetrics *metrics.WSServerMetrics, voicebotMetrics *metrics.VoicebotMetrics) *Session {
	readTimeout := time.Duration(serverCfg.Server.ReadTimeoutMs) * time.Millisecond
	writeTimeout := time.Duration(serverCfg.Server.WriteTimeoutMs) * time.Millisecond

	return &Session{
		serverCfg:       serverCfg,
		voicebot:        voicebot,
		profileID:       profileID,
		conn:            conn,
		deviceID:        deviceID,
		clientID:        clientID,
		sessionID:       sessionID,
		audioParams:     audioParamsFromConfig(serverCfg),
		writeCh:         make(chan outboundMessage, 256),
		readTimeout:     readTimeout,
		writeTimeout:    writeTimeout,
		wsMetrics:       wsMetrics,
		voicebotMetrics: voicebotMetrics,
		startedAt:       time.Now(),
	}
}

func (s *Session) ID() string {
	return s.sessionID
}

func (s *Session) Run() {
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.ctx = memory.WithContext(s.ctx, memory.Context{
		UserID:    resolveUserID(s.deviceID, s.clientID, s.sessionID),
		SessionID: s.sessionID,
		DeviceID:  s.deviceID,
	})

	if s.readTimeout > 0 {
		_ = s.conn.SetReadDeadline(time.Now().Add(s.readTimeout))
		s.conn.SetPongHandler(func(appData string) error {
			_ = s.conn.SetReadDeadline(time.Now().Add(s.readTimeout))
			return nil
		})
	}

	go s.writeLoop()
	go s.pingLoop()

	if err := s.sendServerHello(); err != nil {
		logging.Errorf("Session %s: send hello failed: %v", s.sessionID, err)
		s.Close()
		return
	}

	if err := s.waitClientHello(); err != nil {
		logging.Errorf("Session %s: handshake failed: %v", s.sessionID, err)
		s.Close()
		return
	}

	if err := s.initPipeline(); err != nil {
		logging.Errorf("Session %s: init pipeline failed: %v", s.sessionID, err)
		_ = s.sendServerStatus("error", "init pipeline failed", map[string]string{"reason": err.Error()})
		s.Close()
		return
	}

	s.readLoop()
	s.Close()
}

func (s *Session) Close() {
	s.close.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
		if s.orchestrator != nil {
			_ = s.orchestrator.Stop()
		}
		s.mu.Lock()
		listening := s.listening
		s.listening = false
		s.mu.Unlock()
		if listening && s.audioInPipe != nil {
			_ = s.audioInPipe.Stop()
		}
		if s.mixer != nil {
			_ = s.mixer.Stop()
		}
		if s.toolManager != nil {
			if err := s.toolManager.Close(); err != nil {
				logging.Warnf("Session %s: close tool manager failed: %v", s.sessionID, err)
			}
		}
		if s.memorySvc != nil {
			if err := s.memorySvc.Close(); err != nil {
				logging.Warnf("Session %s: close memory service failed: %v", s.sessionID, err)
			}
		}
		if s.conn != nil {
			_ = s.conn.Close()
		}
	})
}

func (s *Session) waitClientHello() error {
	for {
		msgType, data, err := s.readMessage()
		if err != nil {
			return err
		}
		if msgType != websocket.TextMessage {
			continue
		}

		var base BaseMessage
		if err := json.Unmarshal(data, &base); err != nil {
			continue
		}
		if base.Type != "hello" {
			continue
		}

		var hello HelloMessage
		if err := json.Unmarshal(data, &hello); err != nil {
			return err
		}
		s.applyClientFeatures(hello.Features)
		s.audioParams = NormalizeAudioParams(hello.AudioParams, s.audioParams)
		if err := s.sendServerHello(); err != nil {
			return err
		}
		return nil
	}
}

func (s *Session) initPipeline() error {
	llmCfg := s.voicebot.Provider.LLM.OpenAI
	ttsCfg := s.voicebot.Provider.TTS.Aliyun

	memCfg := memory.Config{
		Mode:                 memory.Mode(strings.TrimSpace(s.voicebot.Memory.Mode)),
		SessionMaxTurns:      s.voicebot.Memory.SessionMaxTurns,
		SessionSummaryEveryN: s.voicebot.Memory.SessionSummaryEveryN,
		LongTermDBPath:       s.voicebot.Memory.LongTermDBPath,
		LongTermMaxResults:   s.voicebot.Memory.LongTermMaxResults,
		RetentionDays:        s.voicebot.Memory.RetentionDays,
		FTSMinScore:          s.voicebot.Memory.FTSMinScore,
	}
	memorySvc, err := memory.NewService(memCfg, memory.Options{
		SystemPrompt: agent.DefaultSystemPrompt(),
		LLM: memory.LLMConfig{
			Provider: s.voicebot.Provider.LLM.Type,
			APIKey:   llmCfg.APIKey,
			BaseURL:  llmCfg.BaseURL,
			Model:    llmCfg.Model,
		},
	})
	if err != nil {
		return err
	}
	s.memorySvc = memorySvc

	agentCfg := agent.Config{
		Provider:    s.voicebot.Provider.LLM.Type,
		APIKey:      llmCfg.APIKey,
		BaseURL:     llmCfg.BaseURL,
		Model:       llmCfg.Model,
		ExtraFields: llmCfg.ExtraFields,
	}
	toolCfg := tools.ManagerConfig{
		MCPServers: toToolsMCPServers(s.voicebot.Tools.MCP),
	}

	toolManager, err := tools.NewToolManager(s.ctx, toolCfg)
	if err != nil {
		return err
	}
	s.toolManager = toolManager

	agentRuntime, err := agent.NewAgent(s.ctx, agentCfg, toolManager, memorySvc)
	if err != nil {
		return err
	}
	var agentRunner agentRunner = agentRuntime
	if s.voicebotMetrics != nil {
		agentRunner = metrics.NewInstrumentedAgent(agentRuntime, s.voicebotMetrics)
	}
	s.agentRunner = agentRunner

	mixerConfig := &audio.MixerConfig{
		TTSVolume:       s.voicebot.Audio.Mixer.TTSVolume,
		ResourceVolume:  s.voicebot.Audio.Mixer.ResourceVolume,
		SampleRate:      s.audioParams.SampleRate,
		Channels:        s.audioParams.Channels,
		FramesPerBuffer: FrameSize(s.audioParams),
	}
	if mixerConfig.FramesPerBuffer <= 0 {
		return fmt.Errorf("invalid frame size for audio params")
	}
	mixer, err := audio.NewMixer(mixerConfig)
	if err != nil {
		return err
	}
	s.mixer = mixer

	sink := audiosink.NewWebSocketSink(s, audiosink.WebSocketSinkConfig{
		Format:          s.audioParams.Format,
		SampleRate:      s.audioParams.SampleRate,
		Channels:        s.audioParams.Channels,
		FrameDurationMs: s.audioParams.FrameDuration,
	})
	s.audioSink = sink
	mixer.SetSink(sink)
	if err := mixer.Start(); err != nil {
		return err
	}

	outPipeCfg := audio.DefaultOutPipeConfig()
	outPipeCfg.Mixer = mixerConfig
	outPipeCfg.TTSProviderType = s.voicebot.Provider.TTS.Type
	outPipeCfg.TTS = tts.Config{
		APIKey:               ttsCfg.APIKey,
		Endpoint:             ttsCfg.Endpoint,
		Workspace:            ttsCfg.Workspace,
		Model:                ttsCfg.Model,
		Voice:                ttsCfg.Voice,
		Format:               ttsCfg.Format,
		SampleRate:           ttsCfg.SampleRate,
		Volume:               ttsCfg.Volume,
		Rate:                 ttsCfg.Rate,
		Pitch:                ttsCfg.Pitch,
		EnableSSML:           ttsCfg.EnableSSML,
		TextType:             ttsCfg.TextType,
		EnableDataInspection: ttsCfg.EnableDataInspection,
	}
	if len(ttsCfg.VoiceMap) > 0 {
		outPipeCfg.VoiceMap = ttsCfg.VoiceMap
	}

	audioOutPipe := audio.NewOutPipeWithConfig(outPipeCfg)
	audioOutPipe.SetMixer(mixer)
	if s.voicebotMetrics != nil {
		audioOutPipe = metrics.NewInstrumentedAudioOutPipe(audioOutPipe, s.voicebotMetrics)
	}
	audioOutPipe.SetOnTTSItemStarted(func(text string, emotion string) {
		if shouldDisplayOnlySentence(text) {
			_ = s.sendTTSSentence(text)
		}
	})
	s.audioOutPipe = audioOutPipe

	toolExecutor := tools.NewExecutorAdapter(s.ctx, toolManager)
	if s.voicebotMetrics != nil {
		s.toolExecutor = metrics.NewInstrumentedToolExecutor(toolExecutor, s.voicebotMetrics)
	} else {
		s.toolExecutor = toolExecutor
	}

	observer := &sessionObserver{session: s}
	orchestrator := voicebot.NewOrchestratorWithOptions(
		agentRunner,
		audioOutPipe,
		nil,
		s.toolExecutor,
		&voicebot.OrchestratorOptions{
			Observer: observer,
			Memory:   memorySvc,
		},
	)
	s.orchestrator = orchestrator

	if err := s.createAudioInPipe(); err != nil {
		return err
	}

	if s.audioParams.Format == "opus" {
		decoder, err := codec.NewOpusDecoder(codec.OpusConfig{
			SampleRate:      s.audioParams.SampleRate,
			Channels:        s.audioParams.Channels,
			FrameDurationMs: s.audioParams.FrameDuration,
		})
		if err != nil {
			return err
		}
		s.opusDecoder = decoder
	}

	if err := orchestrator.Start(s.ctx); err != nil {
		return err
	}
	return nil
}

func (s *Session) readLoop() {
	consecutiveTimeouts := 0
	const maxConsecutiveTimeouts = 3
	for {
		msgType, data, err := s.readMessage()
		if err != nil {
			if s.wsMetrics != nil {
				s.wsMetrics.IncReadError(wsErrorKind(err))
			}
			if s.ctx != nil && s.ctx.Err() != nil {
				return
			}
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				consecutiveTimeouts++
				if consecutiveTimeouts >= maxConsecutiveTimeouts {
					logging.Warnf("Session %s: read timeout threshold reached, closing", s.sessionID)
					return
				}
				continue
			}
			if wsErrorKind(err) == "closed" {
				logging.Infof("Session %s: websocket closed: %v", s.sessionID, err)
			} else {
				logging.Errorf("Session %s: read error: %v", s.sessionID, err)
			}
			return
		}
		consecutiveTimeouts = 0

		if s.wsMetrics != nil {
			s.wsMetrics.IncMessagesIn(messageTypeLabel(msgType))
		}

		switch msgType {
		case websocket.TextMessage:
			s.handleTextMessage(data)
		case websocket.BinaryMessage:
			s.handleBinaryMessage(data)
		}
	}
}

func (s *Session) readMessage() (int, []byte, error) {
	if s.readTimeout > 0 {
		_ = s.conn.SetReadDeadline(time.Now().Add(s.readTimeout))
	}
	return s.conn.ReadMessage()
}

func (s *Session) handleTextMessage(data []byte) {
	var base BaseMessage
	if err := json.Unmarshal(data, &base); err != nil {
		return
	}

	switch base.Type {
	case "hello":
		// 已经处理过，忽略
	case "listen":
		var msg ListenMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return
		}
		s.handleListen(msg)
	case "abort":
		_ = s.sendTTSStop(true)
		if s.audioOutPipe != nil {
			_ = s.audioOutPipe.Interrupt()
		}
		if s.orchestrator != nil {
			s.orchestrator.OnUserSpeakingDetected()
		}
	case "mcp":
		_ = s.sendServerStatus("error", "mcp not supported", map[string]string{"action": "mcp"})
	case "iot":
		_ = s.sendServerStatus("error", "iot not supported", map[string]string{"action": "iot"})
	case "server":
		_ = s.sendServerStatus("error", "server command not supported", map[string]string{"action": "server"})
	default:
		_ = s.sendServerStatus("error", "unknown message type", map[string]string{"type": base.Type})
	}
}

func (s *Session) handleBinaryMessage(data []byte) {
	if len(data) == 0 {
		s.stopListening()
		return
	}

	if s.wsMetrics != nil {
		s.wsMetrics.AddAudioInBytes(len(data))
	}

	s.mu.Lock()
	listening := s.listening
	wsSource := s.wsSource
	s.mu.Unlock()
	if !listening {
		return
	}
	if wsSource == nil {
		return
	}

	switch s.audioParams.Format {
	case "pcm":
		if err := wsSource.PushPCM(data); err != nil {
			if errors.Is(err, audiosource.ErrWebSocketSourceQueueFull) {
				logging.Warnf("Session %s: websocket source queue full, dropping pcm frame", s.sessionID)
			} else {
				logging.Warnf("Session %s: push pcm to websocket source failed: %v", s.sessionID, err)
			}
		}
	case "opus":
		if s.opusDecoder == nil {
			return
		}
		pcm, err := s.opusDecoder.Decode(data)
		if err != nil {
			logging.Errorf("Session %s: opus decode error: %v", s.sessionID, err)
			return
		}
		payload := int16ToBytes(pcm)
		if err := wsSource.PushPCM(payload); err != nil {
			if errors.Is(err, audiosource.ErrWebSocketSourceQueueFull) {
				logging.Warnf("Session %s: websocket source queue full, dropping opus frame", s.sessionID)
			} else {
				logging.Warnf("Session %s: push decoded opus to websocket source failed: %v", s.sessionID, err)
			}
		}
	}
}

func (s *Session) handleListen(msg ListenMessage) {
	if msg.TextResponse != "" {
		s.playTTSDirect(msg.TextResponse)
		return
	}

	switch msg.State {
	case "detect":
		if strings.TrimSpace(msg.Text) != "" {
			if s.orchestrator != nil {
				// 新输入触发打断，确保切到新的 TTS
				s.orchestrator.OnUserSpeakingDetected()
			}
			s.markASRFinal()
			_ = s.sendSTT(msg.Text, sttStateFinal, 0)
			s.orchestrator.OnASRFinal(msg.Text)
		}
	case "start":
		s.startListening()
	case "stop":
		s.stopListening()
	}
}

func (s *Session) startListening() {
	s.mu.Lock()
	if s.listening {
		s.mu.Unlock()
		return
	}
	s.listening = true
	audioInPipe := s.audioInPipe
	s.mu.Unlock()

	if audioInPipe != nil {
		if err := audioInPipe.Start(s.ctx); err != nil {
			logging.Errorf("Session %s: start audio in pipe failed: %v", s.sessionID, err)
			_ = s.sendServerStatus("error", "asr start failed", map[string]string{"reason": err.Error()})
			s.mu.Lock()
			s.listening = false
			s.mu.Unlock()
		}
		return
	}

	if err := s.createAudioInPipe(); err != nil {
		logging.Errorf("Session %s: create audio in pipe failed: %v", s.sessionID, err)
		_ = s.sendServerStatus("error", "asr init failed", map[string]string{"reason": err.Error()})
		s.mu.Lock()
		s.listening = false
		s.mu.Unlock()
		return
	}

	if err := s.audioInPipe.Start(s.ctx); err != nil {
		logging.Errorf("Session %s: start audio in pipe failed: %v", s.sessionID, err)
		_ = s.sendServerStatus("error", "asr start failed", map[string]string{"reason": err.Error()})
		s.mu.Lock()
		s.listening = false
		s.audioInPipe = nil
		s.wsSource = nil
		s.mu.Unlock()
	}
}

func (s *Session) stopListening() {
	s.mu.Lock()
	if !s.listening {
		s.mu.Unlock()
		return
	}
	s.listening = false
	audioInPipe := s.audioInPipe
	s.audioInPipe = nil
	s.wsSource = nil
	s.mu.Unlock()

	if audioInPipe != nil {
		_ = audioInPipe.Stop()
	}
}

func (s *Session) createAudioInPipe() error {
	wsSource, err := audiosource.NewWebSocketSource(nil)
	if err != nil {
		return err
	}
	asrCfg := s.voicebot.Provider.ASR.Aliyun

	audioInPipe, err := audio.NewInPipeWithAudioSource(asrCfg.APIKey, &audio.InPipeConfig{
		SampleRate:      s.audioParams.SampleRate,
		Channels:        s.audioParams.Channels,
		EnableVAD:       s.voicebot.Audio.InPipe.EnableVAD,
		VADThreshold:    s.voicebot.Audio.InPipe.VADThreshold,
		ASRProviderType: s.voicebot.Provider.ASR.Type,
		ASRModel:        asrCfg.Model,
		ASREndpoint:     asrCfg.Endpoint,
	}, wsSource)
	if err != nil {
		return err
	}

	if s.voicebotMetrics != nil {
		audioInPipe = metrics.NewInstrumentedAudioInPipe(audioInPipe, s.voicebotMetrics)
	}

	audioInPipe.OnASRResult(func(text string, isFinal bool) {
		if text == "" {
			return
		}
		if isFinal {
			s.markASRFinal()
			_ = s.sendSTT(text, sttStateFinal, 0)
			s.orchestrator.OnASRFinal(text)
		} else {
			if s.shouldSendASRPartial(text, time.Now()) {
				_ = s.sendSTT(text, sttStatePartial, 0)
			}
			if s.orchestrator != nil && s.shouldInterruptOnASRPartial() {
				s.orchestrator.OnUserSpeakingDetected()
			}
		}
	})
	audioInPipe.OnUserSpeakingDetected(func() {
		s.markVADDetected()
		s.orchestrator.OnUserSpeakingDetected()
	})

	s.mu.Lock()
	s.audioInPipe = audioInPipe
	s.wsSource = wsSource
	s.mu.Unlock()
	return nil
}

func (s *Session) playTTSDirect(text string) {
	if s.audioOutPipe == nil {
		return
	}
	_ = s.sendTTSStart()
	if err := s.audioOutPipe.PlayTTS(text, ""); err != nil {
		logging.Errorf("Session %s: play tts direct failed: %v", s.sessionID, err)
		_ = s.sendTTSStop(true)
		return
	}
}

func (s *Session) sendServerHello() error {
	msg := ServerHelloMessage{
		Type:        "hello",
		Version:     1,
		Transport:   "websocket",
		SessionID:   s.sessionID,
		AudioParams: s.audioParams,
	}
	return s.sendJSON(msg)
}

func (s *Session) sendLLM(text, emotion string) error {
	msg := LLMMessage{
		Type:      "llm",
		Text:      text,
		Emotion:   emotion,
		Action:    emotion,
		SessionID: s.sessionID,
	}
	return s.sendJSON(msg)
}

func (s *Session) sendSTT(text, state string, code int) error {
	msg := STTMessage{
		Type:      "stt",
		State:     state,
		Text:      text,
		SessionID: s.sessionID,
		ErrorCode: code,
	}
	return s.sendJSON(msg)
}

func (s *Session) sendTTSStart() error {
	s.setSendSilence(true)
	msg := TTSMessage{
		Type:      "tts",
		State:     "start",
		SessionID: s.sessionID,
	}
	return s.sendJSON(msg)
}

func (s *Session) sendTTSSentence(text string) error {
	msg := TTSMessage{
		Type:      "tts",
		State:     "sentence_start",
		Text:      text,
		SessionID: s.sessionID,
	}
	return s.sendJSON(msg)
}

func (s *Session) sendTTSStop(isAborted bool) error {
	s.setSendSilence(false)
	msg := TTSMessage{
		Type:      "tts",
		State:     "stop",
		SessionID: s.sessionID,
		IsAborted: isAborted,
	}
	return s.sendJSON(msg)
}

func (s *Session) setSendSilence(enabled bool) {
	if s.audioSink == nil {
		return
	}
	s.audioSink.SetSendSilence(enabled)
}

func (s *Session) sendServerStatus(status, message string, content map[string]string) error {
	msg := ServerStatusMessage{
		Type:    "server",
		Status:  status,
		Message: message,
		Content: content,
	}
	return s.sendJSON(msg)
}

func (s *Session) sendJSON(message any) error {
	payload, err := json.Marshal(message)
	if err != nil {
		return err
	}
	logging.Infof("WS send text: %s", string(payload))
	return s.enqueue(websocket.TextMessage, payload)
}

func (s *Session) enqueue(msgType int, data []byte) error {
	select {
	case <-s.ctx.Done():
		return s.ctx.Err()
	default:
	}
	select {
	case s.writeCh <- outboundMessage{msgType: msgType, data: data}:
		return nil
	default:
		if s.wsMetrics != nil {
			s.wsMetrics.IncWriteQueueDropped()
		}
		return fmt.Errorf("session %s: write queue full", s.sessionID)
	}
}

func (s *Session) writeLoop() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case msg, ok := <-s.writeCh:
			if !ok {
				return
			}
			if s.writeTimeout > 0 {
				_ = s.conn.SetWriteDeadline(time.Now().Add(s.writeTimeout))
			}
			if err := s.conn.WriteMessage(msg.msgType, msg.data); err != nil {
				logging.Errorf("Session %s: write error: %v", s.sessionID, err)
				if s.wsMetrics != nil {
					s.wsMetrics.IncWriteError(wsErrorKind(err))
				}
				s.Close()
				return
			}
			if s.wsMetrics != nil {
				s.wsMetrics.IncMessagesOut(messageTypeLabel(msg.msgType))
			}
		}
	}
}

func (s *Session) SendBinary(data []byte) error {
	if err := s.enqueue(websocket.BinaryMessage, data); err != nil {
		return err
	}
	if s.wsMetrics != nil {
		s.wsMetrics.AddAudioOutBytes(len(data))
	}
	return nil
}

func (s *Session) pingLoop() {
	if s.readTimeout <= 0 {
		return
	}
	interval := s.readTimeout / 2
	if interval < time.Second {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			_ = s.enqueue(websocket.PingMessage, []byte("ping"))
		}
	}
}

type sessionObserver struct {
	session *Session
}

func (o *sessionObserver) OnLLMTextChunk(text, emotion string) {
	// _ = o.session.sendLLM(text, emotion)
}

func (o *sessionObserver) OnTTSSentence(text, emotion string) {
	// sentence_start 在音频真正开始播放时发送，避免重复。
}

func (o *sessionObserver) OnTTSStart() {
	o.session.observeTurnASRToTTSStart()
	_ = o.session.sendTTSStart()
}

func (o *sessionObserver) OnTTSStop(isAborted bool) {
	_ = o.session.sendTTSStop(isAborted)
}

func shouldDisplayOnlySentence(text string) bool {
	if strings.TrimSpace(text) == "" {
		return false
	}
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return true
		}
	}
	return false
}

func (s *Session) applyClientFeatures(features map[string]any) {
	enabled := clientSupportsInterimSTT(features)
	s.asrMu.Lock()
	s.interimSTT = enabled
	s.lastASRPartial = time.Time{}
	s.lastASRText = ""
	s.asrMu.Unlock()
}

func (s *Session) shouldSendASRPartial(text string, now time.Time) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	s.asrMu.Lock()
	defer s.asrMu.Unlock()
	if !s.interimSTT {
		return false
	}
	if s.voicebot.Audio.InPipe.EnableVAD {
		if s.lastVADAt.IsZero() || now.Sub(s.lastVADAt) > interimSTTVADWindow {
			return false
		}
	}
	if text == s.lastASRText {
		return false
	}
	if !s.lastASRPartial.IsZero() && now.Sub(s.lastASRPartial) < interimSTTThrottleInterval {
		return false
	}
	s.lastASRPartial = now
	s.lastASRText = text
	return true
}

func (s *Session) shouldInterruptOnASRPartial() bool {
	return !s.voicebot.Audio.InPipe.EnableVAD
}

func clientSupportsInterimSTT(features map[string]any) bool {
	if len(features) == 0 {
		return false
	}
	if enabled, ok := featureBool(features, helloFeatureInterimSTT); ok {
		return enabled
	}
	sttRaw, ok := features[helloFeatureInterimSTTGroup]
	if !ok {
		return false
	}
	sttMap, ok := sttRaw.(map[string]any)
	if !ok {
		return false
	}
	enabled, ok := featureBool(sttMap, "interim")
	return ok && enabled
}

func featureBool(features map[string]any, key string) (bool, bool) {
	raw, ok := features[key]
	if !ok {
		return false, false
	}
	switch value := raw.(type) {
	case bool:
		return value, true
	case string:
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "1", "true", "yes", "on":
			return true, true
		case "0", "false", "no", "off":
			return false, true
		}
	}
	return false, false
}

func audioParamsFromConfig(cfg *config.WSServerAppConfig) AudioParams {
	ap := cfg.Server.AudioParams
	return AudioParams{
		Format:               ap.Format,
		SampleRate:           ap.SampleRate,
		Channels:             ap.Channels,
		FrameDuration:        ap.FrameDurationMs,
		BitsPerSample:        ap.BitsPerSample,
		PlayBufferDurationMs: ap.PlayBufferDurationMs,
	}
}

func resolveUserID(deviceID, clientID, sessionID string) string {
	if strings.TrimSpace(deviceID) != "" {
		return deviceID
	}
	if strings.TrimSpace(clientID) != "" {
		return clientID
	}
	return sessionID
}

func int16ToBytes(samples []int16) []byte {
	if len(samples) == 0 {
		return nil
	}
	payload := make([]byte, len(samples)*2)
	for i, v := range samples {
		binary.LittleEndian.PutUint16(payload[i*2:], uint16(v))
	}
	return payload
}

func (s *Session) markASRFinal() {
	s.asrMu.Lock()
	s.lastASRFinalAt = time.Now()
	s.lastASRPartial = time.Time{}
	s.lastASRText = ""
	s.asrMu.Unlock()
}

func (s *Session) markVADDetected() {
	s.asrMu.Lock()
	s.lastVADAt = time.Now()
	s.asrMu.Unlock()
}

func (s *Session) observeTurnASRToTTSStart() {
	if s.voicebotMetrics == nil {
		return
	}
	s.asrMu.Lock()
	last := s.lastASRFinalAt
	if !last.IsZero() {
		s.lastASRFinalAt = time.Time{}
	}
	s.asrMu.Unlock()
	if !last.IsZero() {
		s.voicebotMetrics.ObserveTurnASRToTTSStart(time.Since(last))
	}
}

func messageTypeLabel(msgType int) string {
	switch msgType {
	case websocket.TextMessage:
		return "text"
	case websocket.BinaryMessage:
		return "binary"
	case websocket.PingMessage:
		return "ping"
	case websocket.PongMessage:
		return "pong"
	case websocket.CloseMessage:
		return "close"
	default:
		return "unknown"
	}
}

func wsErrorKind(err error) string {
	if err == nil {
		return "other"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "timeout"
	}
	if errors.Is(err, websocket.ErrCloseSent) || websocket.IsCloseError(err,
		websocket.CloseNormalClosure,
		websocket.CloseGoingAway,
		websocket.CloseAbnormalClosure,
		websocket.CloseNoStatusReceived,
	) {
		return "closed"
	}
	return "other"
}

func toToolsMCPServers(cfgs []config.MCPServerConfig) []tools.MCPServerConfig {
	servers := make([]tools.MCPServerConfig, 0, len(cfgs))
	for _, cfg := range cfgs {
		servers = append(servers, tools.MCPServerConfig{
			ID:           cfg.ID,
			Transport:    cfg.Transport,
			Command:      cfg.Command,
			Args:         cfg.Args,
			Endpoint:     cfg.Endpoint,
			ToolNameList: cfg.ToolNameList,
			TimeoutMs:    cfg.TimeoutMs,
		})
	}
	return servers
}
