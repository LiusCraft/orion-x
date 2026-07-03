package main

import (
	"context"
	"fmt"
	"time"

	"github.com/gorilla/websocket"

	"github.com/liuscraft/orion-x/internal/audio"
	"github.com/liuscraft/orion-x/internal/audio/codec"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/memory"
	"github.com/liuscraft/orion-x/internal/pipeline"
	pstages "github.com/liuscraft/orion-x/internal/pipeline/stages"
	"github.com/liuscraft/orion-x/internal/provider/asr"
	"github.com/liuscraft/orion-x/internal/provider/tts"
	"github.com/liuscraft/orion-x/internal/session"
	"github.com/liuscraft/orion-x/internal/wsproto"
)

// wsserverTTSSampleRate is the sample rate this server uses for TTS output,
// independent of cmd/voicebot's 22050Hz (each entry point constructs its
// own tts.Provider instance, so the two don't need to agree).
const wsserverTTSSampleRate = 16000

// defaultAudioFormat is used when a client's hello omits audio_params.format.
const defaultAudioFormat = codec.FormatOpus

// defaultFrameDurationMs is used when a client's hello omits (or sends a
// non-positive) audio_params.frame_duration.
const defaultFrameDurationMs = 60

// supportedBitsPerSample is the only bits_per_sample value this server
// accepts — the entire pipeline (codec, resampler, ASR/TTS providers) is
// hardcoded to PCM16LE, so any other value fails the handshake rather than
// being silently coerced.
const supportedBitsPerSample = 16

const (
	// defaultPreBufferFrames is used when a client's hello omits (or sends
	// a non-positive) audio_params.play_buffer_duration.
	defaultPreBufferFrames = 3
	// minPreBufferFrames/maxPreBufferFrames bound computePreBufferFrames's
	// result so a malformed play_buffer_duration can't produce a
	// pre-buffer window of zero (defeats the low-latency start) or an
	// unreasonably large one (defeats pacing almost entirely).
	minPreBufferFrames = 1
	maxPreBufferFrames = 100
)

// computePreBufferFrames converts the client's playback buffer size
// (audio_params.play_buffer_duration) into a frame count: the larger the
// client's own buffer, the more frames the server can safely front-load
// before switching to steady-rate pacing without risking a client-side
// overrun. Falls back to defaultPreBufferFrames when either input is
// unusable (not declared, or non-positive).
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

// wsConnection holds all per-connection state and resources: its own
// Session, ASR recognizer/processor, TTS provider/processor, and DAG
// pipeline. None of this is shared across connections — see Server's doc
// comment for what is shared (Agent, ToolManager, MemoryService).
type wsConnection struct {
	server    *Server
	rawConn   *websocket.Conn
	safeConn  *pstages.SafeConn
	sessionID string
	mode      wsproto.Mode

	asrProc  audio.ASRProcessor
	ttsProc  audio.TTSProcessor
	pl       pipeline.Pipeline
	audioSrc *pstages.WSAudioSource

	ctx    context.Context
	cancel context.CancelFunc
}

// handleConnection owns the full lifecycle of one WebSocket connection:
// handshake, resource setup, pipeline wiring, the read loop, and cleanup on
// exit (any exit path — normal close, read error, or setup failure).
func (s *Server) handleConnection(rawConn *websocket.Conn) {
	defer func() { _ = rawConn.Close() }()

	hello, err := readHello(rawConn)
	if err != nil {
		logging.Warnf("wsserver: handshake failed: %v", err)
		return
	}

	c, err := s.newConnection(rawConn, hello)
	if err != nil {
		logging.Errorf("wsserver: connection setup failed: %v", err)
		return
	}
	defer c.close()

	if err := c.sendHelloResponse(hello); err != nil {
		logging.Warnf("wsserver[%s]: send hello response failed: %v", c.sessionID, err)
		return
	}

	logging.Infof("wsserver[%s]: connection established (device_id=%q, mode=%s)", c.sessionID, hello.DeviceID, c.mode)
	c.readLoop()
	logging.Infof("wsserver[%s]: connection closed", c.sessionID)
}

// newConnection negotiates audio format/mode from hello and assembles every
// per-connection resource: Recognizer, ASRProcessor, TTS Provider,
// TTSProcessor, Session, and the 4-node DAG pipeline
// (asr -> agent, asr -> ws_output, agent -> tts, tts -> ws_output).
func (s *Server) newConnection(rawConn *websocket.Conn, hello *wsproto.HelloMessage) (*wsConnection, error) {
	if bps := hello.AudioParams.BitsPerSample; bps != 0 && bps != supportedBitsPerSample {
		return nil, fmt.Errorf("unsupported bits_per_sample %d (only %d is supported)", bps, supportedBitsPerSample)
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
	channels := hello.AudioParams.Channels
	if channels <= 0 {
		channels = audio.InternalChannels
	}
	frameDurationMs := hello.AudioParams.FrameDuration
	if frameDurationMs <= 0 {
		frameDurationMs = defaultFrameDurationMs
	}
	preBufferFrames := computePreBufferFrames(hello.AudioParams.PlayBufferDuration, frameDurationMs)

	inputCodec, err := codec.New(codec.Format(format), clientSampleRate, channels, frameDurationMs)
	if err != nil {
		return nil, err
	}
	outputCodec, err := codec.New(codec.Format(format), wsserverTTSSampleRate, channels, frameDurationMs)
	if err != nil {
		return nil, err
	}

	sess := session.New(session.SessionMeta{Model: s.appConfig.Provider.LLM.OpenAI.Model})
	sessionID := sess.ID

	recognizer, err := s.newRecognizer()
	if err != nil {
		return nil, err
	}

	inPipeCfg := s.appConfig.Audio.InPipe
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

	ttsProvider, err := s.newTTSProvider()
	if err != nil {
		return nil, err
	}

	ttsPipeCfg := s.appConfig.Audio.TTSPipeline
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

	audioSrc := pstages.NewWSAudioSource(inputCodec, clientSampleRate)
	safeConn := pstages.NewSafeConn(rawConn)

	// Tag this connection's context with its own memory.Context so
	// long-term memory (if enabled) is scoped per connection instead of
	// bleeding together — unlike cmd/voicebot, which hardcodes a single
	// "local" UserID/SessionID for its one CLI session.
	userID := hello.DeviceID
	if userID == "" {
		userID = sessionID
	}
	// Derived from s.rootCtx (not context.Background()) so Server.Shutdown
	// cancelling rootCtx propagates here too — see the goroutine below that
	// closes rawConn on ctx.Done(), which is what actually unblocks
	// readLoop's blocking ReadMessage call (context cancellation alone
	// doesn't interrupt a gorilla/websocket read).
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

	if err := ttsProc.Start(ctx); err != nil {
		cancel()
		return nil, err
	}

	pl, err := pipeline.NewDAGBuilder().
		AddStage(pstages.NewASRStage(asrProc, audioSrc)).
		AddStage(pstages.NewAgentStage(s.agentInst, sess)).
		AddStage(pstages.NewTTSStage(ttsProc)).
		AddStage(pstages.NewWSOutputStage(safeConn, sessionID, outputCodec, wsserverTTSSampleRate, frameDurationMs, preBufferFrames)).
		Connect("asr", "agent").
		Connect("asr", "ws_output").
		Connect("agent", "tts").
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

	// ws_output never produces pipeline output (it's a pure sink), so this
	// only logs internal pipeline errors and exits when pl.Output() closes
	// on Stop.
	go func() {
		for msg := range pl.Output() {
			if msg.IsError() {
				logging.Warnf("wsserver[%s]: pipeline error: %v", sessionID, msg.Metadata.Error)
			}
		}
	}()

	return &wsConnection{
		server:    s,
		rawConn:   rawConn,
		safeConn:  safeConn,
		sessionID: sessionID,
		mode:      mode,
		asrProc:   asrProc,
		ttsProc:   ttsProc,
		pl:        pl,
		audioSrc:  audioSrc,
		ctx:       ctx,
		cancel:    cancel,
	}, nil
}

func (s *Server) newRecognizer() (asr.Recognizer, error) {
	asrCfg := s.appConfig.Provider.ASR.Aliyun
	return asr.NewRecognizer(asr.ProviderConfig{
		Type: s.appConfig.Provider.ASR.Type,
		Config: asr.Config{
			APIKey:     asrCfg.APIKey,
			Endpoint:   asrCfg.Endpoint,
			Model:      asrCfg.Model,
			Format:     "pcm",
			SampleRate: audio.InternalSampleRate,
		},
	})
}

func (s *Server) newTTSProvider() (tts.Provider, error) {
	ttsCfg := s.appConfig.Provider.TTS.Aliyun
	return tts.NewProvider(tts.ProviderConfig{
		Type: s.appConfig.Provider.TTS.Type,
		Config: tts.Config{
			APIKey:               ttsCfg.APIKey,
			Endpoint:             ttsCfg.Endpoint,
			Workspace:            ttsCfg.Workspace,
			Model:                ttsCfg.Model,
			Voice:                ttsCfg.Voice,
			Format:               "pcm",
			SampleRate:           wsserverTTSSampleRate,
			Volume:               ttsCfg.Volume,
			Rate:                 ttsCfg.Rate,
			Pitch:                ttsCfg.Pitch,
			EnableSSML:           ttsCfg.EnableSSML,
			TextType:             ttsCfg.TextType,
			EnableDataInspection: ttsCfg.EnableDataInspection,
		},
	})
}

// sendHelloResponse replies with the server-assigned session_id and the
// audio_params the server will actually use for TTS output (downstream).
// Upstream (client -> server) audio keeps using whatever the client
// declared in its hello — the server resamples as needed rather than
// forcing the client to match a server-picked rate. FrameDuration is echoed
// back because it determines the client's Opus decoding/PCM framing for the
// downstream stream; PlayBufferDuration is not echoed back — it's the
// client's own buffer size, not something the server negotiates a value
// for (see computePreBufferFrames, which only consumes it).
func (c *wsConnection) sendHelloResponse(hello *wsproto.HelloMessage) error {
	format := hello.AudioParams.Format
	if format == "" {
		format = string(defaultAudioFormat)
	}
	channels := hello.AudioParams.Channels
	if channels <= 0 {
		channels = audio.InternalChannels
	}
	frameDurationMs := hello.AudioParams.FrameDuration
	if frameDurationMs <= 0 {
		frameDurationMs = defaultFrameDurationMs
	}
	resp := wsproto.NewHelloResponse(c.sessionID, wsproto.AudioParams{
		Format:        format,
		SampleRate:    wsserverTTSSampleRate,
		Channels:      channels,
		FrameDuration: frameDurationMs,
		BitsPerSample: supportedBitsPerSample,
	}, c.mode, "")
	return c.safeConn.WriteJSON(resp)
}

// close releases every per-connection resource. It's safe to call exactly
// once per connection (via defer in handleConnection) and must run on every
// exit path, including setup failures partway through newConnection.
func (c *wsConnection) close() {
	logging.Infof("wsserver[%s]: cleaning up connection resources", c.sessionID)
	if c.pl != nil {
		if err := c.pl.Stop(); err != nil {
			logging.Warnf("wsserver[%s]: stop pipeline error: %v", c.sessionID, err)
		}
	}
	if c.ttsProc != nil {
		if err := c.ttsProc.Stop(); err != nil {
			logging.Warnf("wsserver[%s]: stop TTSProcessor error: %v", c.sessionID, err)
		}
	}
	if c.audioSrc != nil {
		_ = c.audioSrc.Close()
	}
	if c.cancel != nil {
		c.cancel()
	}
}

// readLoop drains WebSocket frames until the connection closes or errors:
// binary frames are audio, text frames are protocol control messages.
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
		logging.Warnf("wsserver[%s]: ignoring invalid message: %v", c.sessionID, err)
		return
	}

	switch m := msg.(type) {
	case *wsproto.ListenMessage:
		c.handleListen(m)
	case *wsproto.AbortMessage:
		c.handleAbort()
	case *wsproto.HelloMessage:
		logging.Warnf("wsserver[%s]: ignoring duplicate hello after handshake", c.sessionID)
	}
}

// handleListen drives manual-mode turn boundaries (BeginTurn/EndTurn are
// no-ops in auto mode, see audio.ASRProcessor) and text injection. In auto
// mode, listen start/stop are informational only: VAD already triggers
// OnSpeechStart -> MessageTypeInterrupt on its own, so a new listen start
// must not also force an interrupt here — mirroring xiaozhi-esp32-server's
// behavior where manual mode never auto-interrupts an in-progress reply.
func (c *wsConnection) handleListen(m *wsproto.ListenMessage) {
	switch m.State {
	case wsproto.ListenStart:
		if c.mode == wsproto.ModeManual {
			if err := c.asrProc.BeginTurn(c.ctx); err != nil {
				logging.Warnf("wsserver[%s]: BeginTurn failed: %v", c.sessionID, err)
			}
		}

	case wsproto.ListenStop:
		if c.mode == wsproto.ModeManual {
			if err := c.asrProc.EndTurn(c.ctx); err != nil {
				logging.Warnf("wsserver[%s]: EndTurn failed: %v", c.sessionID, err)
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
