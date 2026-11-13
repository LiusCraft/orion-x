package realtime

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/liuscraft/orion-x/internal/agent"
	"github.com/liuscraft/orion-x/internal/audio"
	"github.com/liuscraft/orion-x/internal/audio/resampler"
	"github.com/liuscraft/orion-x/internal/channels"
	"github.com/liuscraft/orion-x/internal/config"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/memory"
	"github.com/liuscraft/orion-x/internal/realtime"
	"github.com/liuscraft/orion-x/internal/session"
	"github.com/liuscraft/orion-x/internal/tools"
	"github.com/liuscraft/orion-x/pkg/pipeline"
)

const (
	realtimeInputRate      = 24000
	realtimeOutputRate     = 24000
	maxRealtimeMessageSize = 8 << 20
	maxRealtimeAudioBytes  = 512 << 10
	realtimeWriteTimeout   = 10 * time.Second
	realtimeReadTimeout    = 90 * time.Second
	realtimePingInterval   = 30 * time.Second
)

var errResponseCancelling = errors.New("response cancellation in progress")
var errConnectionClosed = errors.New("connection is closed")

func (c *Channel) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	deviceID := firstNonEmpty(r.Header.Get("X-Device-ID"), r.URL.Query().Get("device_id"))
	if err := c.authenticate(r.Context(), r, deviceID); err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	conn, err := c.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	conn.SetReadLimit(maxRealtimeMessageSize)
	ctx := c.rootCtx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	connState, err := newConnection(ctx, cancel, conn, c, deviceID)
	if err != nil {
		data, _ := realtime.MarshalEvent(realtime.ServerEvent{
			Type:    realtime.ServerError,
			EventID: eventID(),
			Error:   &realtime.Error{Type: "server_error", Code: "session_initialization_failed", Message: err.Error()},
		})
		_ = conn.WriteMessage(websocket.TextMessage, data)
		cancel()
		_ = conn.Close()
		return
	}
	c.track(connState)
	go func() {
		defer c.untrack(connState)
		connState.serve()
	}()
}

func (c *Channel) authenticate(ctx context.Context, r *http.Request, deviceID string) error {
	if c.deps == nil || c.deps.KeyVerifier == nil {
		return nil
	}
	if deviceID == "" {
		return fmt.Errorf("device_id is required")
	}
	token := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(token), "bearer ") {
		token = strings.TrimSpace(token[len("bearer "):])
	}
	if token == "" {
		token = strings.TrimSpace(r.URL.Query().Get("access_token"))
	}
	if token == "" {
		return fmt.Errorf("authorization is required")
	}
	result, err := c.deps.KeyVerifier.Verify(ctx, token, deviceID)
	if err != nil {
		return fmt.Errorf("authorization failed")
	}
	if !result.Allowed {
		return fmt.Errorf("authorization rejected")
	}
	return nil
}

func newConnection(ctx context.Context, cancel context.CancelFunc, ws *websocket.Conn, owner *Channel, deviceID string) (*connection, error) {
	connCfg, err := loadConfig(owner.deps, deviceID)
	if err != nil {
		return nil, err
	}
	if connCfg == nil {
		return nil, fmt.Errorf("device %q is not registered", deviceID)
	}
	sess := session.New(session.SessionMeta{UserID: deviceID, Model: connCfg.Provider.LLM.OpenAI.Model})
	owner.sessions.Add(sess)
	ctx = memory.WithContext(ctx, memory.Context{UserID: deviceID, SessionID: sess.ID, DeviceID: deviceID})

	memSvc, err := memory.NewService(memory.Config{
		MemoryCharLimit: connCfg.Memory.MemoryCharLimit,
		UserCharLimit:   connCfg.Memory.UserCharLimit,
	}, memory.Options{
		ManagerURL:   managerURL(owner.deps),
		DeviceID:     deviceID,
		ReviewConfig: memory.ReviewConfig{Enabled: true},
	})
	if err != nil {
		logging.Warnf("realtime-channel[%s]: memory init: %v", sess.ID, err)
		memSvc = nil
	}

	llmClient, err := owner.providers.GetOrCreateLLM(ctx, connCfg.Provider.LLM.Type, connCfg.Provider.LLM.OpenAI)
	if err != nil {
		if memSvc != nil {
			_ = memSvc.Close()
		}
		owner.sessions.CloseSession(sess.ID)
		return nil, fmt.Errorf("get LLM client: %w", err)
	}
	toolMgr := owner.toolsMgr.Clone()
	defer func() {
		if toolMgr != nil {
			_ = toolMgr.Close()
		}
	}()
	if err := registerMCPServers(ctx, toolMgr, connCfg); err != nil {
		if memSvc != nil {
			_ = memSvc.Close()
		}
		owner.sessions.CloseSession(sess.ID)
		return nil, fmt.Errorf("register MCP servers: %w", err)
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
	connAgent, err := agent.NewWithClient(connAgentCfg, llmClient, toolMgr, memSvc)
	if err != nil {
		if memSvc != nil {
			_ = memSvc.Close()
		}
		owner.sessions.CloseSession(sess.ID)
		return nil, fmt.Errorf("create agent: %w", err)
	}

	recognizer, err := owner.providers.GetOrCreateASR(connCfg.Provider.ASR.Type, connCfg.Provider.ASR.Aliyun)
	if err != nil {
		if memSvc != nil {
			_ = memSvc.Close()
		}
		owner.sessions.CloseSession(sess.ID)
		return nil, fmt.Errorf("create ASR: %w", err)
	}
	inputCfg := connCfg.Audio.InPipe
	asrProc, err := audio.NewASRProcessor(&audio.ASRConfig{
		EnableVAD:       inputCfg.EnableVAD,
		VADThreshold:    inputCfg.VADThreshold,
		VADType:         inputCfg.VADType,
		VADModelPath:    inputCfg.VADModelPath,
		VADMinSilenceMs: inputCfg.VADMinSilenceMs,
		VADSpeechPadMs:  inputCfg.VADSpeechPadMs,
		Recognizer:      recognizer,
	})
	if err != nil {
		if memSvc != nil {
			_ = memSvc.Close()
		}
		owner.sessions.CloseSession(sess.ID)
		return nil, fmt.Errorf("create ASR processor: %w", err)
	}
	if inputCfg.EnableVAD {
		if err := asrProc.SetVADEnabled(true); err != nil {
			if memSvc != nil {
				_ = memSvc.Close()
			}
			owner.sessions.CloseSession(sess.ID)
			return nil, fmt.Errorf("enable ASR VAD: %w", err)
		}
	}

	ttsProvider, err := owner.providers.GetOrCreateTTS(connCfg.Provider.TTS.Type, connCfg.Provider.TTS.Aliyun, realtimeOutputRate)
	if err != nil {
		if memSvc != nil {
			_ = memSvc.Close()
		}
		owner.sessions.CloseSession(sess.ID)
		return nil, fmt.Errorf("create TTS: %w", err)
	}
	ttsCfg := audio.DefaultTTSConfig()
	ttsCfg.Provider = ttsProvider
	if connCfg.Audio.TTSPipeline.TextQueueSize > 0 {
		ttsCfg.QueueSize = connCfg.Audio.TTSPipeline.TextQueueSize
	}
	ttsProc, err := audio.NewTTSProcessor(ttsCfg)
	if err != nil {
		if memSvc != nil {
			_ = memSvc.Close()
		}
		owner.sessions.CloseSession(sess.ID)
		return nil, fmt.Errorf("create TTS processor: %w", err)
	}
	if err := ttsProc.Start(ctx); err != nil {
		if memSvc != nil {
			_ = memSvc.Close()
		}
		owner.sessions.CloseSession(sess.ID)
		return nil, fmt.Errorf("start TTS processor: %w", err)
	}

	state := &connection{
		ctx:               ctx,
		cancel:            cancel,
		ws:                ws,
		owner:             owner,
		session:           sess,
		asr:               asrProc,
		tts:               ttsProc,
		memory:            memSvc,
		tools:             toolMgr,
		deviceID:          deviceID,
		serveDone:         make(chan struct{}),
		config:            sessionConfigForVAD(inputCfg.EnableVAD),
		pendingText:       "",
		turnDetectionType: turnDetectionType(inputCfg.EnableVAD),
		createResponse:    inputCfg.EnableVAD,
		interruptResponse: true,
	}
	state.config.Model = connCfg.Provider.LLM.OpenAI.Model
	if err := state.setupPipeline(connAgent); err != nil {
		_ = ttsProc.Stop()
		if memSvc != nil {
			_ = memSvc.Close()
		}
		owner.sessions.CloseSession(sess.ID)
		return nil, err
	}
	if err := state.writeEvent(realtime.ServerEvent{
		Type:    realtime.ServerSessionCreated,
		EventID: eventID(),
		Session: &state.config,
	}); err != nil {
		_ = ttsProc.Stop()
		if memSvc != nil {
			_ = memSvc.Close()
		}
		owner.sessions.CloseSession(sess.ID)
		return nil, err
	}
	toolMgr = nil
	return state, nil
}

func loadConfig(deps *channels.Dependencies, deviceID string) (*config.AppConfig, error) {
	if deps == nil || deps.DeviceCfgLoader == nil || deviceID == "" {
		return config.DefaultConfig(), nil
	}
	loaded, err := deps.DeviceCfgLoader.LoadConfig(deviceID)
	if err != nil {
		return nil, fmt.Errorf("load device config for %q: %w", deviceID, err)
	}
	return loaded, nil
}

type connection struct {
	ctx    context.Context
	cancel context.CancelFunc
	ws     *websocket.Conn
	owner  *Channel

	session   *session.Session
	asr       audio.ASRProcessor
	tts       audio.TTSProcessor
	memory    *memory.Service
	tools     *tools.Manager
	pipeline  pipeline.Pipeline
	deviceID  string
	serveDone chan struct{}

	writeMu            sync.Mutex
	pipeMu             sync.RWMutex
	stateMu            sync.Mutex
	config             realtime.SessionConfig
	pendingText        string
	responseID         string
	outputItemID       string
	outputText         string
	responseActive     bool
	responseCancelling bool
	turnOpen           bool
	audioCommitPending bool
	audioItemID        string
	audioTranscript    string
	audioTurnStartedAt time.Time
	turnDetectionType  string
	createResponse     bool
	interruptResponse  bool
	closed             atomic.Bool
	stopping           atomic.Bool
	pipelineClosed     bool
}

func (c *connection) setupPipeline(connAgent *agent.Agent) error {
	asrStage := newRealtimeASRStage(c.asr, c.handleASRResult, c.handleSpeechStart)
	pl, err := pipeline.NewDAGBuilder().
		AddStage(asrStage).
		AddStage(agent.NewAgentStage(connAgent, c.session)).
		AddStage(audio.NewTTSStage(c.tts)).
		AddStage(audio.NewOutputStage()).
		Connect("asr", "agent").
		Connect("asr", "session_output").
		Connect("agent", "tts").
		Connect("agent", "session_output").
		Connect("tts", "session_output").
		Build()
	if err != nil {
		return fmt.Errorf("build realtime pipeline: %w", err)
	}
	c.pipeline = pl
	c.session.Pipeline = pl
	if err := pl.Start(c.ctx); err != nil {
		return fmt.Errorf("start realtime pipeline: %w", err)
	}
	go c.consumePipeline()
	return nil
}

func (c *connection) handleSpeechStart() {
	c.stateMu.Lock()
	itemID := c.audioItemID
	c.stateMu.Unlock()
	_ = c.writeEvent(realtime.ServerEvent{
		Type:         realtime.ServerInputAudioSpeechStarted,
		EventID:      eventID(),
		ItemID:       itemID,
		AudioStartMS: 0,
	})
	if c.shouldInterruptResponse() {
		_ = c.cancelResponse()
	}
}

func (c *connection) consumePipeline() {
	for msg := range c.pipeline.Output() {
		if msg.IsError() {
			c.writeError("pipeline_error", msg.Metadata.Error.Error())
			c.finishResponse("failed")
			continue
		}
		switch payload := msg.Payload.(type) {
		case string:
			if source, ok := msg.Metadata.Extra["source"].(string); ok && source == "llm" {
				c.writeOutputText(payload)
			}
		case audio.TTSChunk:
			c.writeAudioChunk(payload)
		}
		if msg.Type == pipeline.MessageTypeInterrupt {
			c.stateMu.Lock()
			c.responseCancelling = false
			shouldCreate := c.createResponse && c.pendingText != ""
			c.stateMu.Unlock()
			if shouldCreate {
				_ = c.startPendingResponse()
			}
		}
	}
}

func (c *connection) serve() {
	defer close(c.serveDone)
	defer c.close()
	defer c.cancel()
	_ = c.ws.SetReadDeadline(time.Now().Add(realtimeReadTimeout))
	c.ws.SetPongHandler(func(string) error {
		return c.ws.SetReadDeadline(time.Now().Add(realtimeReadTimeout))
	})
	pingDone := make(chan struct{})
	go c.pingLoop(pingDone)
	defer close(pingDone)
	for {
		messageType, data, err := c.ws.ReadMessage()
		if err != nil {
			return
		}
		if messageType != websocket.TextMessage {
			c.writeError("invalid_event", "realtime events must be JSON text messages")
			continue
		}
		event, err := realtime.ParseClientEvent(data)
		if err != nil {
			c.writeError("invalid_event", err.Error())
			continue
		}
		if err := c.handleEvent(event); err != nil {
			c.writeError("invalid_event", err.Error())
		}
	}
}

func (c *connection) pingLoop(done <-chan struct{}) {
	ticker := time.NewTicker(realtimePingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			c.writeMu.Lock()
			err := c.ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(realtimeWriteTimeout))
			c.writeMu.Unlock()
			if err != nil {
				return
			}
		}
	}
}

func (c *connection) handleEvent(event realtime.ClientEvent) error {
	if c.closed.Load() || c.stopping.Load() {
		return errConnectionClosed
	}
	switch event.Type {
	case realtime.ClientSessionUpdate:
		return c.handleSessionUpdate(event.Session)
	case realtime.ClientInputAudioAppend:
		return c.handleAudioAppend(event.Audio)
	case realtime.ClientInputAudioCommit:
		return c.handleAudioCommit()
	case realtime.ClientInputAudioClear:
		return c.handleAudioClear()
	case realtime.ClientConversationItemCreate:
		return c.handleItemCreate(event.Item)
	case realtime.ClientResponseCreate:
		return c.handleResponseCreate(event.Response)
	case realtime.ClientResponseCancel:
		return c.cancelResponse()
	default:
		return fmt.Errorf("unsupported event type %q", event.Type)
	}
}

func (c *connection) handleSessionUpdate(update *realtime.SessionConfig) error {
	if update == nil {
		return fmt.Errorf("session is required")
	}
	if update.Audio != nil && update.Audio.Output != nil && update.Audio.Output.Format != nil {
		format := update.Audio.Output.Format
		if format.Type != "" && format.Type != "audio/pcm" {
			return fmt.Errorf("output audio format %q is unsupported, use audio/pcm", format.Type)
		}
		if format.Rate > 0 && format.Rate != realtimeOutputRate {
			return fmt.Errorf("output audio rate %d is unsupported, use %d", format.Rate, realtimeOutputRate)
		}
		if format.SampleRate > 0 && format.SampleRate != realtimeOutputRate {
			return fmt.Errorf("output audio sample rate %d is unsupported, use %d", format.SampleRate, realtimeOutputRate)
		}
	}
	if update.Audio != nil && update.Audio.Input != nil && update.Audio.Input.Format != nil {
		if err := validateInputAudioFormat(update.Audio.Input.Format); err != nil {
			return err
		}
	}
	if update.Audio != nil && update.Audio.Input != nil {
		if err := c.applyTurnDetection(update.Audio.Input); err != nil {
			return err
		}
	}
	c.stateMu.Lock()
	mergeSessionConfig(&c.config, update)
	configCopy := c.config
	c.stateMu.Unlock()
	return c.writeEvent(realtime.ServerEvent{Type: realtime.ServerSessionUpdated, EventID: eventID(), Session: &configCopy})
}

func (c *connection) handleAudioClear() error {
	c.stateMu.Lock()
	turnOpen := c.turnOpen
	commitPending := c.audioCommitPending
	c.stateMu.Unlock()
	if turnOpen || commitPending {
		return fmt.Errorf("input_audio_buffer.clear is unsupported while audio is active")
	}
	c.stateMu.Lock()
	c.audioCommitPending = false
	c.audioTranscript = ""
	c.audioItemID = ""
	c.audioTurnStartedAt = time.Time{}
	c.stateMu.Unlock()
	return c.writeEvent(realtime.ServerEvent{Type: realtime.ServerInputAudioBufferCleared, EventID: eventID()})
}

func (c *connection) applyTurnDetection(input *realtime.AudioInputConfig) error {
	if !input.TurnDetectionSpecified() {
		return nil
	}
	detection := input.TurnDetection
	if detection != nil && detection.Type != "server_vad" {
		return fmt.Errorf("turn detection type %q is unsupported", detection.Type)
	}
	enabled := detection != nil
	if err := c.asr.SetVADEnabled(enabled); err != nil {
		return fmt.Errorf("update turn detection: %w", err)
	}
	createResponse := enabled
	interruptResponse := true
	if detection != nil {
		if detection.CreateResponse != nil {
			createResponse = *detection.CreateResponse
		}
		if detection.InterruptResponse != nil {
			interruptResponse = *detection.InterruptResponse
		}
	}
	c.stateMu.Lock()
	if enabled {
		c.turnDetectionType = detection.Type
	} else {
		c.turnDetectionType = ""
		c.turnOpen = false
	}
	c.createResponse = createResponse
	c.interruptResponse = interruptResponse
	c.stateMu.Unlock()
	return nil
}

func (c *connection) handleAudioAppend(encoded string) error {
	if encoded == "" {
		return fmt.Errorf("audio is required")
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return fmt.Errorf("decode audio: %w", err)
	}
	if len(data) > maxRealtimeAudioBytes {
		return fmt.Errorf("audio chunk exceeds %d bytes", maxRealtimeAudioBytes)
	}
	inputRate, err := c.inputAudioRate()
	if err != nil {
		return err
	}
	data, err = pcm16Resample(data, inputRate, audio.InternalSampleRate)
	if err != nil {
		return err
	}
	c.stateMu.Lock()
	beginTurn := !c.turnOpen
	if beginTurn {
		c.turnOpen = true
		c.audioCommitPending = false
	}
	c.stateMu.Unlock()
	if beginTurn {
		c.stateMu.Lock()
		c.audioItemID = "item_" + uuid.NewString()
		c.audioTranscript = ""
		c.audioTurnStartedAt = time.Now()
		c.stateMu.Unlock()
		if err := c.asr.BeginTurn(c.ctx); err != nil {
			c.stateMu.Lock()
			c.turnOpen = false
			c.audioItemID = ""
			c.audioTurnStartedAt = time.Time{}
			c.stateMu.Unlock()
			return fmt.Errorf("begin audio turn: %w", err)
		}
	}
	return c.asr.Write(data)
}

func (c *connection) inputAudioRate() (int, error) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if c.config.Audio == nil || c.config.Audio.Input == nil {
		return realtimeInputRate, nil
	}
	format := c.config.Audio.Input.Format
	if format == nil {
		return realtimeInputRate, nil
	}
	if format.Type != "" && format.Type != "audio/pcm" {
		return 0, fmt.Errorf("input audio format %q is unsupported, use audio/pcm", format.Type)
	}
	if err := validateInputAudioFormat(format); err != nil {
		return 0, err
	}
	if format.Rate > 0 {
		return format.Rate, nil
	}
	if format.SampleRate > 0 {
		return format.SampleRate, nil
	}
	return realtimeInputRate, nil
}

func (c *connection) handleAudioCommit() error {
	if c.serverVADEnabled() {
		return nil
	}
	c.stateMu.Lock()
	wasOpen := c.turnOpen
	itemID := c.audioItemID
	startedAt := c.audioTurnStartedAt
	c.turnOpen = false
	if wasOpen {
		c.audioCommitPending = true
	}
	c.stateMu.Unlock()
	if !wasOpen {
		return nil
	}
	if err := c.asr.EndTurn(c.ctx); err != nil {
		c.stateMu.Lock()
		c.audioCommitPending = false
		c.turnOpen = true
		c.stateMu.Unlock()
		return fmt.Errorf("commit audio: %w", err)
	}
	c.stateMu.Lock()
	text := strings.TrimSpace(c.audioTranscript)
	c.audioTranscript = ""
	c.audioCommitPending = false
	c.audioItemID = ""
	c.audioTurnStartedAt = time.Time{}
	c.stateMu.Unlock()
	if text == "" {
		return c.writeEvent(realtime.ServerEvent{
			Type:    realtime.ServerInputAudioBufferCommitted,
			EventID: eventID(),
			ItemID:  itemID,
		})
	}
	c.emitAudioTurn(text, itemID, startedAt, false)
	return nil
}

func (c *connection) serverVADEnabled() bool {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	return c.turnDetectionType == "server_vad"
}

func (c *connection) shouldInterruptResponse() bool {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	return c.responseActive && c.interruptResponse
}

func (c *connection) handleItemCreate(item *realtime.ConversationItem) error {
	if item == nil {
		return fmt.Errorf("item is required")
	}
	text := itemText(item)
	if text == "" {
		return fmt.Errorf("only input_text conversation items are supported")
	}
	if item.ID == "" {
		item.ID = "item_" + uuid.NewString()
	}
	if item.Type == "" {
		item.Type = "message"
	}
	if item.Role == "" {
		item.Role = "user"
	}
	c.stateMu.Lock()
	c.pendingText = text
	c.stateMu.Unlock()
	return c.writeEvent(realtime.ServerEvent{Type: realtime.ServerConversationItemCreated, EventID: eventID(), Item: item})
}

func (c *connection) handleResponseCreate(_ *realtime.ResponseCreate) error {
	return c.startPendingResponse()
}

func (c *connection) startPendingResponse() error {
	c.stateMu.Lock()
	if c.responseCancelling {
		c.stateMu.Unlock()
		return errResponseCancelling
	}
	if c.responseActive {
		c.stateMu.Unlock()
		return fmt.Errorf("a response is already in progress")
	}
	text := c.pendingText
	c.pendingText = ""
	c.stateMu.Unlock()
	if text == "" {
		return fmt.Errorf("no pending conversation item")
	}
	if err := c.startResponse(); err != nil {
		c.stateMu.Lock()
		c.pendingText = text
		c.stateMu.Unlock()
		return err
	}
	if err := c.sendPipeline(pipeline.Message{Type: pipeline.MessageTypeData, Payload: text}); err != nil {
		c.finishResponse("failed")
		return err
	}
	return nil
}

func (c *connection) handleASRResult(result audio.ASRResult) {
	if !result.IsFinal || strings.TrimSpace(result.Text) == "" {
		return
	}
	if !c.serverVADEnabled() {
		c.stateMu.Lock()
		if !c.turnOpen && c.audioItemID == "" {
			c.stateMu.Unlock()
			return
		}
		c.audioTranscript += result.Text
		c.stateMu.Unlock()
		return
	}
	c.stateMu.Lock()
	itemID := c.audioItemID
	startedAt := c.audioTurnStartedAt
	commitPending := c.audioCommitPending
	c.audioCommitPending = false
	c.audioItemID = ""
	c.audioTurnStartedAt = time.Time{}
	c.stateMu.Unlock()
	c.emitAudioTurn(result.Text, itemID, startedAt, commitPending)
}

func (c *connection) emitAudioTurn(text, itemID string, startedAt time.Time, commitPending bool) {
	if itemID == "" {
		itemID = "item_" + uuid.NewString()
	}
	audioEndMS := elapsedMilliseconds(startedAt)
	item := &realtime.ConversationItem{
		ID:      itemID,
		Type:    "message",
		Role:    "user",
		Content: []realtime.ContentPart{{Type: "input_audio", Transcript: text}},
	}
	_ = c.writeEvent(realtime.ServerEvent{
		Type:       realtime.ServerInputAudioSpeechStopped,
		EventID:    eventID(),
		ItemID:     itemID,
		AudioEndMS: audioEndMS,
	})
	if !commitPending {
		_ = c.writeEvent(realtime.ServerEvent{Type: realtime.ServerInputAudioBufferCommitted, EventID: eventID(), ItemID: item.ID})
	}
	_ = c.writeEvent(realtime.ServerEvent{Type: realtime.ServerConversationItemCreated, EventID: eventID(), Item: item})
	c.stateMu.Lock()
	c.turnOpen = false
	c.pendingText = text
	createResponse := c.createResponse
	responseCancelling := c.responseCancelling
	c.stateMu.Unlock()
	if !createResponse || !c.serverVADEnabled() || responseCancelling {
		return
	}
	if err := c.startPendingResponse(); err != nil {
		c.writeError("response_error", err.Error())
	}
}

func (c *connection) startResponse() error {
	c.stateMu.Lock()
	if c.responseActive {
		c.stateMu.Unlock()
		return fmt.Errorf("a response is already in progress")
	}
	if c.responseCancelling {
		c.stateMu.Unlock()
		return errResponseCancelling
	}
	c.responseID = "resp_" + uuid.NewString()
	c.outputItemID = "item_" + uuid.NewString()
	c.outputText = ""
	c.responseActive = true
	responseID := c.responseID
	c.stateMu.Unlock()
	if err := c.writeEvent(realtime.ServerEvent{
		Type:       realtime.ServerResponseCreated,
		EventID:    eventID(),
		ResponseID: responseID,
		Response:   map[string]any{"id": responseID, "status": "in_progress"},
	}); err != nil {
		c.stateMu.Lock()
		if c.responseID == responseID {
			c.responseActive = false
		}
		c.stateMu.Unlock()
		return err
	}
	return nil
}

func (c *connection) cancelResponse() error {
	c.stateMu.Lock()
	if !c.responseActive {
		c.stateMu.Unlock()
		return nil
	}
	c.responseCancelling = true
	c.stateMu.Unlock()
	if err := c.sendPipeline(pipeline.Message{Type: pipeline.MessageTypeInterrupt}); err != nil {
		return err
	}
	c.finishResponse("cancelled")
	return nil
}

func (c *connection) writeOutputText(text string) {
	c.stateMu.Lock()
	if !c.responseActive {
		c.stateMu.Unlock()
		return
	}
	responseID := c.responseID
	itemID := c.outputItemID
	c.outputText += text
	c.stateMu.Unlock()
	_ = c.writeEvent(realtime.ServerEvent{Type: realtime.ServerResponseOutputTextDelta, EventID: eventID(), ResponseID: responseID, ItemID: itemID, Delta: text})
	_ = c.writeEvent(realtime.ServerEvent{Type: realtime.ServerResponseAudioTranscriptDelta, EventID: eventID(), ResponseID: responseID, ItemID: itemID, Delta: text})
}

func (c *connection) writeAudioChunk(chunk audio.TTSChunk) {
	if chunk.Final {
		c.finishResponse("completed")
		return
	}
	if len(chunk.Audio) == 0 {
		return
	}
	c.stateMu.Lock()
	if !c.responseActive {
		c.stateMu.Unlock()
		return
	}
	responseID := c.responseID
	itemID := c.outputItemID
	c.stateMu.Unlock()
	_ = c.writeEvent(realtime.ServerEvent{
		Type:         realtime.ServerResponseOutputAudioDelta,
		EventID:      eventID(),
		ResponseID:   responseID,
		ItemID:       itemID,
		OutputIndex:  0,
		ContentIndex: 0,
		Delta:        base64.StdEncoding.EncodeToString(chunk.Audio),
	})
}

func (c *connection) finishResponse(status string) {
	c.stateMu.Lock()
	if !c.responseActive {
		c.stateMu.Unlock()
		return
	}
	responseID := c.responseID
	itemID := c.outputItemID
	outputText := c.outputText
	c.responseActive = false
	if status != "cancelled" {
		c.responseCancelling = false
	}
	c.stateMu.Unlock()
	_ = c.writeEvent(realtime.ServerEvent{Type: realtime.ServerResponseOutputTextDone, EventID: eventID(), ResponseID: responseID, ItemID: itemID, Text: outputText})
	_ = c.writeEvent(realtime.ServerEvent{Type: realtime.ServerResponseAudioTranscriptDone, EventID: eventID(), ResponseID: responseID, ItemID: itemID, Transcript: outputText})
	_ = c.writeEvent(realtime.ServerEvent{Type: realtime.ServerResponseOutputAudioDone, EventID: eventID(), ResponseID: responseID, ItemID: itemID})
	_ = c.writeEvent(realtime.ServerEvent{Type: realtime.ServerResponseDone, EventID: eventID(), ResponseID: responseID, Response: map[string]any{"id": responseID, "status": status}})
}

func (c *connection) writeError(code, message string) {
	_ = c.writeEvent(realtime.ServerEvent{Type: realtime.ServerError, EventID: eventID(), Error: &realtime.Error{Type: "invalid_request_error", Code: code, Message: message}})
}

func (c *connection) sendPipeline(msg pipeline.Message) error {
	c.pipeMu.RLock()
	defer c.pipeMu.RUnlock()
	if c.pipelineClosed || c.pipeline == nil {
		return errConnectionClosed
	}
	select {
	case c.pipeline.Input() <- msg:
		return nil
	case <-c.ctx.Done():
		return c.ctx.Err()
	}
}

func (c *connection) writeEvent(event realtime.ServerEvent) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.writeEventLocked(event)
}

func (c *connection) writeEventLocked(event realtime.ServerEvent) error {
	data, err := realtime.MarshalEvent(event)
	if err != nil {
		return err
	}
	if err := c.ws.SetWriteDeadline(time.Now().Add(realtimeWriteTimeout)); err != nil {
		return err
	}
	defer func() { _ = c.ws.SetWriteDeadline(time.Time{}) }()
	return c.ws.WriteMessage(websocket.TextMessage, data)
}

func (c *connection) close() {
	if !c.closed.CompareAndSwap(false, true) {
		return
	}
	c.cancel()
	c.pipeMu.Lock()
	c.pipelineClosed = true
	pl := c.pipeline
	c.pipeMu.Unlock()
	if pl != nil {
		_ = pl.Stop()
	}
	if c.tts != nil {
		_ = c.tts.Stop()
	}
	if c.tools != nil {
		_ = c.tools.Close()
	}
	if c.memory != nil {
		_ = c.memory.Close()
	}
	if c.session != nil {
		c.owner.sessions.CloseSession(c.session.ID)
	}
	_ = c.ws.Close()
}

func (c *connection) requestClose() {
	c.stopping.Store(true)
	c.pipeMu.Lock()
	c.pipelineClosed = true
	c.pipeMu.Unlock()
	_ = c.ws.Close()
}

func elapsedMilliseconds(startedAt time.Time) int {
	if startedAt.IsZero() {
		return 0
	}
	return max(0, int(time.Since(startedAt).Milliseconds()))
}

func defaultSessionConfig() realtime.SessionConfig {
	return sessionConfigForVAD(true)
}

func sessionConfigForVAD(enabled bool) realtime.SessionConfig {
	createResponse := enabled
	config := realtime.SessionConfig{
		ID:     "sess_" + uuid.NewString(),
		Object: "realtime.session",
		Type:   "realtime",
		Audio: &realtime.AudioConfig{
			Input: &realtime.AudioInputConfig{
				Format: &realtime.AudioFormat{Type: "audio/pcm", Rate: realtimeInputRate},
			},
			Output: &realtime.AudioOutputConfig{
				Format: &realtime.AudioFormat{Type: "audio/pcm", Rate: realtimeOutputRate},
			},
		},
		OutputModalities: []string{"text", "audio"},
	}
	if enabled {
		config.Audio.Input.TurnDetection = &realtime.TurnDetection{
			Type:           "server_vad",
			CreateResponse: &createResponse,
		}
	}
	return config
}

func turnDetectionType(enabled bool) string {
	if enabled {
		return "server_vad"
	}
	return ""
}

func mergeSessionConfig(dst, src *realtime.SessionConfig) {
	if src.Instructions != "" {
		dst.Instructions = src.Instructions
	}
	if src.Audio != nil {
		if dst.Audio == nil {
			dst.Audio = &realtime.AudioConfig{}
		}
		if src.Audio.Input != nil {
			if dst.Audio.Input == nil {
				dst.Audio.Input = &realtime.AudioInputConfig{}
			}
			if src.Audio.Input.Format != nil {
				dst.Audio.Input.Format = src.Audio.Input.Format
			}
			if src.Audio.Input.TurnDetectionSpecified() {
				dst.Audio.Input.TurnDetection = src.Audio.Input.TurnDetection
			}
		}
		if src.Audio.Output != nil {
			if dst.Audio.Output == nil {
				dst.Audio.Output = &realtime.AudioOutputConfig{}
			}
			if src.Audio.Output.Format != nil {
				dst.Audio.Output.Format = src.Audio.Output.Format
			}
			if src.Audio.Output.Voice != "" {
				dst.Audio.Output.Voice = src.Audio.Output.Voice
			}
		}
	}
	if src.OutputModalities != nil {
		dst.OutputModalities = append([]string(nil), src.OutputModalities...)
	}
	if src.Voice != "" {
		dst.Voice = src.Voice
	}
	if src.Tools != nil {
		dst.Tools = append([]realtime.ToolDefinition(nil), src.Tools...)
	}
	if src.ToolChoice != nil {
		dst.ToolChoice = src.ToolChoice
	}
	if src.MaxOutputTokens != nil {
		dst.MaxOutputTokens = src.MaxOutputTokens
	}
}

func itemText(item *realtime.ConversationItem) string {
	for _, part := range item.Content {
		if part.Type == "input_text" || part.Type == "text" {
			if strings.TrimSpace(part.Text) != "" {
				return part.Text
			}
		}
	}
	return ""
}

func pcm16Resample(data []byte, inputRate, outputRate int) ([]byte, error) {
	if len(data)%2 != 0 {
		return nil, fmt.Errorf("pcm16 audio has odd byte length")
	}
	if !isSupportedInputRate(inputRate) {
		return nil, fmt.Errorf("unsupported input audio rate %d", inputRate)
	}
	if inputRate == outputRate {
		return data, nil
	}
	samples := make([]int16, len(data)/2)
	for i := range samples {
		samples[i] = int16(binary.LittleEndian.Uint16(data[i*2:]))
	}
	converted, err := resampler.NewLinearResampler().Resample(samples, inputRate, outputRate, 1)
	if err != nil {
		return nil, fmt.Errorf("resample PCM16 audio: %w", err)
	}
	out := make([]byte, len(converted)*2)
	for i, sample := range converted {
		binary.LittleEndian.PutUint16(out[i*2:], uint16(sample))
	}
	return out, nil
}

func validateInputAudioFormat(format *realtime.AudioFormat) error {
	if format.Type != "" && format.Type != "audio/pcm" {
		return fmt.Errorf("input audio format %q is unsupported, use audio/pcm", format.Type)
	}
	if format.Rate > 0 && format.SampleRate > 0 && format.Rate != format.SampleRate {
		return fmt.Errorf("input audio rate and sample rate must match")
	}
	rate := format.Rate
	if rate == 0 {
		rate = format.SampleRate
	}
	if rate > 0 && !isSupportedInputRate(rate) {
		return fmt.Errorf("input audio rate %d is unsupported, use 8000, 16000, 24000, or 48000", rate)
	}
	return nil
}

func isSupportedInputRate(rate int) bool {
	switch rate {
	case 8000, 16000, 24000, 48000:
		return true
	default:
		return false
	}
}

func managerURL(deps *channels.Dependencies) string {
	if deps == nil || deps.DeviceCfgLoader == nil {
		return ""
	}
	return deps.DeviceCfgLoader.ManagerURL()
}

func registerMCPServers(ctx context.Context, manager *tools.Manager, cfg *config.AppConfig) error {
	if manager == nil || cfg == nil || len(cfg.Tools.MCP) == 0 {
		return nil
	}
	configs := make([]tools.MCPServerConfig, len(cfg.Tools.MCP))
	for i, item := range cfg.Tools.MCP {
		configs[i] = tools.MCPServerConfig{
			ID:           item.ID,
			Transport:    item.Transport,
			Command:      item.Command,
			Args:         item.Args,
			Env:          item.Env,
			CWD:          item.CWD,
			Endpoint:     item.Endpoint,
			Headers:      item.Headers,
			ToolNameList: item.ToolNameList,
			TimeoutMs:    item.TimeoutMs,
		}
	}
	return manager.RegisterMCPServers(ctx, configs)
}

func eventID() string { return "event_" + uuid.NewString() }

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
