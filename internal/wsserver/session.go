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
	"github.com/liuscraft/orion-x/internal/config"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/metrics"
	"github.com/liuscraft/orion-x/internal/tools"
	"github.com/liuscraft/orion-x/internal/tts"
	"github.com/liuscraft/orion-x/internal/voicebot"
)

type outboundMessage struct {
	msgType int
	data    []byte
}

type Session struct {
	cfg       *config.AppConfig
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

	voiceAgent   agent.VoiceAgent
	toolExecutor tools.ToolExecutor
	orchestrator voicebot.Orchestrator
	audioOutPipe audio.AudioOutPipe
	audioInPipe  audio.AudioInPipe
	mixer        audio.AudioMixer
	audioSink    *audiosink.WebSocketSink
	opusDecoder  *codec.OpusDecoder

	wsMetrics       *metrics.WSServerMetrics
	voicebotMetrics *metrics.VoicebotMetrics

	startedAt      time.Time
	lastASRFinalAt time.Time
	asrMu          sync.Mutex

	listening bool
	mu        sync.Mutex
}

func NewSession(cfg *config.AppConfig, conn *websocket.Conn, deviceID, clientID, sessionID string, wsMetrics *metrics.WSServerMetrics, voicebotMetrics *metrics.VoicebotMetrics) *Session {
	readTimeout := time.Duration(cfg.Server.ReadTimeoutMs) * time.Millisecond
	writeTimeout := time.Duration(cfg.Server.WriteTimeoutMs) * time.Millisecond

	return &Session{
		cfg:             cfg,
		conn:            conn,
		deviceID:        deviceID,
		clientID:        clientID,
		sessionID:       sessionID,
		audioParams:     audioParamsFromConfig(cfg),
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
		s.audioParams = NormalizeAudioParams(hello.AudioParams, s.audioParams)
		if err := s.sendServerHello(); err != nil {
			return err
		}
		return nil
	}
}

func (s *Session) initPipeline() error {
	toolTypes, err := agent.ParseToolTypes(s.cfg.Tools.Types)
	if err != nil {
		return err
	}

	voiceAgent, err := agent.NewVoiceAgentWithConfig(s.ctx, agent.Config{
		APIKey:          s.cfg.LLM.APIKey,
		BaseURL:         s.cfg.LLM.BaseURL,
		Model:           s.cfg.LLM.Model,
		ToolTypes:       toolTypes,
		ActionResponses: s.cfg.Tools.ActionResponses,
	})
	if err != nil {
		return err
	}
	if s.voicebotMetrics != nil {
		voiceAgent = metrics.NewInstrumentedVoiceAgent(voiceAgent, s.voicebotMetrics)
	}
	s.voiceAgent = voiceAgent

	mixerConfig := &audio.MixerConfig{
		TTSVolume:       s.cfg.Audio.Mixer.TTSVolume,
		ResourceVolume:  s.cfg.Audio.Mixer.ResourceVolume,
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
	outPipeCfg.TTS = tts.Config{
		APIKey:               s.cfg.TTS.APIKey,
		Endpoint:             s.cfg.TTS.Endpoint,
		Workspace:            s.cfg.TTS.Workspace,
		Model:                s.cfg.TTS.Model,
		Voice:                s.cfg.TTS.Voice,
		Format:               s.cfg.TTS.Format,
		SampleRate:           s.cfg.TTS.SampleRate,
		Volume:               s.cfg.TTS.Volume,
		Rate:                 s.cfg.TTS.Rate,
		Pitch:                s.cfg.TTS.Pitch,
		EnableSSML:           s.cfg.TTS.EnableSSML,
		TextType:             s.cfg.TTS.TextType,
		EnableDataInspection: s.cfg.TTS.EnableDataInspection,
	}
	if len(s.cfg.TTS.VoiceMap) > 0 {
		outPipeCfg.VoiceMap = s.cfg.TTS.VoiceMap
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

	toolExecutor := tools.NewToolExecutor()
	toolExecutor.RegisterTool("getTime", tools.GetTimeTool)
	toolExecutor.RegisterTool("getWeather", tools.GetWeatherTool)
	if s.voicebotMetrics != nil {
		toolExecutor = metrics.NewInstrumentedToolExecutor(toolExecutor, s.voicebotMetrics)
	}
	s.toolExecutor = toolExecutor

	observer := &sessionObserver{session: s}
	orchestrator := voicebot.NewOrchestratorWithOptions(
		voiceAgent,
		audioOutPipe,
		nil,
		toolExecutor,
		&voicebot.OrchestratorOptions{
			Observer: observer,
			TTSScheduler: voicebot.TTSSchedulerConfig{
				MaxInFlightSentences: s.cfg.Audio.TTSScheduler.MaxInFlightSentences,
				MaxCacheSentences:    s.cfg.Audio.TTSScheduler.MaxCacheSentences,
			},
		},
	)
	s.orchestrator = orchestrator

	audioInPipe, err := audio.NewInPipe(s.cfg.ASR.APIKey, &audio.InPipeConfig{
		SampleRate:   s.audioParams.SampleRate,
		Channels:     s.audioParams.Channels,
		EnableVAD:    s.cfg.Audio.InPipe.EnableVAD,
		VADThreshold: s.cfg.Audio.InPipe.VADThreshold,
		ASRModel:     s.cfg.ASR.Model,
		ASREndpoint:  s.cfg.ASR.Endpoint,
	})
	if err != nil {
		return err
	}
	if s.voicebotMetrics != nil {
		audioInPipe = metrics.NewInstrumentedAudioInPipe(audioInPipe, s.voicebotMetrics)
	}
	s.audioInPipe = audioInPipe

	audioInPipe.OnASRResult(func(text string, isFinal bool) {
		if text == "" {
			return
		}
		if isFinal {
			s.markASRFinal()
			_ = s.sendSTT(text, 0)
			s.orchestrator.OnASRFinal(text)
		} else {
			s.orchestrator.OnUserSpeakingDetected()
		}
	})
	audioInPipe.OnUserSpeakingDetected(func() {
		s.orchestrator.OnUserSpeakingDetected()
	})

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
			if !errors.Is(err, websocket.ErrCloseSent) && !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
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
	s.mu.Unlock()
	if !listening {
		return
	}
	if s.audioInPipe == nil {
		return
	}

	switch s.audioParams.Format {
	case "pcm":
		_ = s.audioInPipe.SendAudio(data)
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
		_ = s.audioInPipe.SendAudio(payload)
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
			_ = s.sendSTT(msg.Text, 0)
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
	s.mu.Unlock()

	if s.audioInPipe != nil {
		if err := s.audioInPipe.Start(s.ctx); err != nil {
			logging.Errorf("Session %s: start audio in pipe failed: %v", s.sessionID, err)
			_ = s.sendServerStatus("error", "asr start failed", map[string]string{"reason": err.Error()})
			s.mu.Lock()
			s.listening = false
			s.mu.Unlock()
		}
	}
}

func (s *Session) stopListening() {
	s.mu.Lock()
	if !s.listening {
		s.mu.Unlock()
		return
	}
	s.listening = false
	s.mu.Unlock()

	if s.audioInPipe != nil {
		_ = s.audioInPipe.Stop()
	}
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

func (s *Session) sendSTT(text string, code int) error {
	msg := STTMessage{
		Type:      "stt",
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

func audioParamsFromConfig(cfg *config.AppConfig) AudioParams {
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
