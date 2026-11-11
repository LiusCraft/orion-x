package realtime

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/liuscraft/orion-x/internal/agent"
	"github.com/liuscraft/orion-x/internal/audio"
	"github.com/liuscraft/orion-x/internal/audio/resampler"
	"github.com/liuscraft/orion-x/internal/channels"
	"github.com/liuscraft/orion-x/internal/config"
	"github.com/liuscraft/orion-x/internal/realtime"
	"github.com/liuscraft/orion-x/internal/session"
	"github.com/liuscraft/orion-x/pkg/pipeline"
)

const (
	realtimeInputRate  = 24000
	realtimeOutputRate = 24000
)

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
		defer connState.close()
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

	llmClient, err := owner.providers.GetOrCreateLLM(ctx, connCfg.Provider.LLM.Type, connCfg.Provider.LLM.OpenAI)
	if err != nil {
		owner.sessions.CloseSession(sess.ID)
		return nil, fmt.Errorf("get LLM client: %w", err)
	}
	toolMgr := owner.toolsMgr.Clone()
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
	connAgent, err := agent.NewWithClient(connAgentCfg, llmClient, toolMgr, owner.memory)
	if err != nil {
		owner.sessions.CloseSession(sess.ID)
		return nil, fmt.Errorf("create agent: %w", err)
	}

	recognizer, err := owner.providers.GetOrCreateASR(connCfg.Provider.ASR.Type, connCfg.Provider.ASR.Aliyun)
	if err != nil {
		owner.sessions.CloseSession(sess.ID)
		return nil, fmt.Errorf("create ASR: %w", err)
	}
	inputCfg := connCfg.Audio.InPipe
	asrProc, err := audio.NewASRProcessor(&audio.ASRConfig{
		EnableVAD:       true,
		VADThreshold:    inputCfg.VADThreshold,
		VADType:         inputCfg.VADType,
		VADModelPath:    inputCfg.VADModelPath,
		VADMinSilenceMs: inputCfg.VADMinSilenceMs,
		VADSpeechPadMs:  inputCfg.VADSpeechPadMs,
		Recognizer:      recognizer,
	})
	if err != nil {
		owner.sessions.CloseSession(sess.ID)
		return nil, fmt.Errorf("create ASR processor: %w", err)
	}

	ttsProvider, err := owner.providers.GetOrCreateTTS(connCfg.Provider.TTS.Type, connCfg.Provider.TTS.Aliyun, realtimeOutputRate)
	if err != nil {
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
		owner.sessions.CloseSession(sess.ID)
		return nil, fmt.Errorf("create TTS processor: %w", err)
	}
	if err := ttsProc.Start(ctx); err != nil {
		owner.sessions.CloseSession(sess.ID)
		return nil, fmt.Errorf("start TTS processor: %w", err)
	}

	state := &connection{
		ctx:         ctx,
		cancel:      cancel,
		ws:          ws,
		owner:       owner,
		session:     sess,
		asr:         asrProc,
		tts:         ttsProc,
		deviceID:    deviceID,
		config:      defaultSessionConfig(),
		pendingText: "",
	}
	state.config.Model = connCfg.Provider.LLM.OpenAI.Model
	if err := state.setupPipeline(connAgent); err != nil {
		_ = ttsProc.Stop()
		owner.sessions.CloseSession(sess.ID)
		return nil, err
	}
	if err := state.writeEvent(realtime.ServerEvent{
		Type:    realtime.ServerSessionCreated,
		EventID: eventID(),
		Session: &state.config,
	}); err != nil {
		_ = ttsProc.Stop()
		owner.sessions.CloseSession(sess.ID)
		return nil, err
	}
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

	session  *session.Session
	asr      audio.ASRProcessor
	tts      audio.TTSProcessor
	pipeline pipeline.Pipeline
	deviceID string

	writeMu        sync.Mutex
	stateMu        sync.Mutex
	config         realtime.SessionConfig
	pendingText    string
	responseID     string
	outputItemID   string
	outputText     string
	responseActive bool
	turnOpen       bool
	closed         atomic.Bool
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
	_ = c.writeEvent(realtime.ServerEvent{Type: realtime.ServerInputAudioSpeechStarted, EventID: eventID()})
	c.finishResponse("cancelled")
}

func (c *connection) consumePipeline() {
	for msg := range c.pipeline.Output() {
		switch payload := msg.Payload.(type) {
		case string:
			if source, ok := msg.Metadata.Extra["source"].(string); ok && source == "llm" {
				c.writeOutputText(payload)
			}
		case audio.TTSChunk:
			c.writeAudioChunk(payload)
		}
		if msg.Type == pipeline.MessageTypeInterrupt {
			c.finishResponse("cancelled")
		}
		if msg.Type == pipeline.MessageTypeError && msg.Metadata.Error != nil {
			c.writeError("pipeline_error", msg.Metadata.Error.Error())
			c.finishResponse("failed")
		}
	}
}

func (c *connection) serve() {
	defer c.cancel()
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

func (c *connection) handleEvent(event realtime.ClientEvent) error {
	switch event.Type {
	case realtime.ClientSessionUpdate:
		return c.handleSessionUpdate(event.Session)
	case realtime.ClientInputAudioAppend:
		return c.handleAudioAppend(event.Audio)
	case realtime.ClientInputAudioCommit:
		return c.handleAudioCommit()
	case realtime.ClientInputAudioClear:
		return c.writeEvent(realtime.ServerEvent{Type: realtime.ServerInputAudioBufferCleared, EventID: eventID()})
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
	c.stateMu.Lock()
	mergeSessionConfig(&c.config, update)
	configCopy := c.config
	c.stateMu.Unlock()
	if update.Audio != nil && update.Audio.Input != nil && update.Audio.Input.TurnDetection != nil {
		enabled := strings.EqualFold(update.Audio.Input.TurnDetection.Type, "server_vad")
		if err := c.asr.SetVADEnabled(enabled); err != nil {
			return fmt.Errorf("update turn detection: %w", err)
		}
		if !enabled {
			c.stateMu.Lock()
			c.turnOpen = false
			c.stateMu.Unlock()
		}
	}
	return c.writeEvent(realtime.ServerEvent{Type: realtime.ServerSessionUpdated, EventID: eventID(), Session: &configCopy})
}

func (c *connection) handleAudioAppend(encoded string) error {
	if encoded == "" {
		return fmt.Errorf("audio is required")
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return fmt.Errorf("decode audio: %w", err)
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
	}
	c.stateMu.Unlock()
	if beginTurn {
		if err := c.asr.BeginTurn(c.ctx); err != nil {
			c.stateMu.Lock()
			c.turnOpen = false
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
	if format.Rate > 0 {
		return format.Rate, nil
	}
	if format.SampleRate > 0 {
		return format.SampleRate, nil
	}
	return realtimeInputRate, nil
}

func (c *connection) handleAudioCommit() error {
	if err := c.asr.EndTurn(c.ctx); err != nil {
		return fmt.Errorf("commit audio: %w", err)
	}
	c.stateMu.Lock()
	c.turnOpen = false
	c.stateMu.Unlock()
	return c.writeEvent(realtime.ServerEvent{Type: realtime.ServerInputAudioBufferCommitted, EventID: eventID()})
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
	c.stateMu.Lock()
	text := c.pendingText
	c.pendingText = ""
	c.stateMu.Unlock()
	if text == "" {
		return fmt.Errorf("no pending conversation item")
	}
	if err := c.startResponse(); err != nil {
		return err
	}
	select {
	case c.pipeline.Input() <- pipeline.Message{Type: pipeline.MessageTypeData, Payload: text}:
		return nil
	case <-c.ctx.Done():
		return c.ctx.Err()
	}
}

func (c *connection) handleASRResult(result audio.ASRResult) {
	if !result.IsFinal || strings.TrimSpace(result.Text) == "" {
		return
	}
	item := &realtime.ConversationItem{
		ID:      "item_" + uuid.NewString(),
		Type:    "message",
		Role:    "user",
		Content: []realtime.ContentPart{{Type: "input_audio", Transcript: result.Text}},
	}
	_ = c.writeEvent(realtime.ServerEvent{Type: realtime.ServerInputAudioSpeechStopped, EventID: eventID()})
	_ = c.writeEvent(realtime.ServerEvent{Type: realtime.ServerInputAudioBufferCommitted, EventID: eventID(), ItemID: item.ID})
	_ = c.writeEvent(realtime.ServerEvent{Type: realtime.ServerConversationItemCreated, EventID: eventID(), Item: item})
	c.stateMu.Lock()
	c.pendingText = ""
	c.turnOpen = false
	c.stateMu.Unlock()
	if err := c.startResponse(); err != nil {
		c.writeError("response_error", err.Error())
		return
	}
}

func (c *connection) startResponse() error {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if c.responseActive {
		return fmt.Errorf("a response is already in progress")
	}
	c.responseID = "resp_" + uuid.NewString()
	c.outputItemID = "item_" + uuid.NewString()
	c.outputText = ""
	c.responseActive = true
	return c.writeEventLocked(realtime.ServerEvent{
		Type:       realtime.ServerResponseCreated,
		EventID:    eventID(),
		ResponseID: c.responseID,
		Response:   map[string]any{"id": c.responseID, "status": "in_progress"},
	})
}

func (c *connection) cancelResponse() error {
	select {
	case c.pipeline.Input() <- pipeline.Message{Type: pipeline.MessageTypeInterrupt}:
	case <-c.ctx.Done():
		return c.ctx.Err()
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
	_ = c.writeEvent(realtime.ServerEvent{Type: realtime.ServerResponseOutputAudioDelta, EventID: eventID(), ResponseID: responseID, ItemID: itemID, Audio: base64.StdEncoding.EncodeToString(chunk.Audio)})
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
	c.stateMu.Unlock()
	_ = c.writeEvent(realtime.ServerEvent{Type: realtime.ServerResponseOutputTextDone, EventID: eventID(), ResponseID: responseID, ItemID: itemID, Text: outputText})
	_ = c.writeEvent(realtime.ServerEvent{Type: realtime.ServerResponseAudioTranscriptDone, EventID: eventID(), ResponseID: responseID, ItemID: itemID, Transcript: outputText})
	_ = c.writeEvent(realtime.ServerEvent{Type: realtime.ServerResponseOutputAudioDone, EventID: eventID(), ResponseID: responseID, ItemID: itemID})
	_ = c.writeEvent(realtime.ServerEvent{Type: realtime.ServerResponseDone, EventID: eventID(), ResponseID: responseID, Response: map[string]any{"id": responseID, "status": status}})
}

func (c *connection) writeError(code, message string) {
	_ = c.writeEvent(realtime.ServerEvent{Type: realtime.ServerError, EventID: eventID(), Error: &realtime.Error{Type: "invalid_request_error", Code: code, Message: message}})
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
	return c.ws.WriteMessage(websocket.TextMessage, data)
}

func (c *connection) close() {
	if !c.closed.CompareAndSwap(false, true) {
		return
	}
	c.cancel()
	if c.pipeline != nil {
		_ = c.pipeline.Stop()
	}
	if c.tts != nil {
		_ = c.tts.Stop()
	}
	if c.asr != nil {
		_ = c.asr.Stop()
	}
	if c.session != nil {
		c.owner.sessions.CloseSession(c.session.ID)
	}
	_ = c.ws.Close()
}

func defaultSessionConfig() realtime.SessionConfig {
	createResponse := true
	return realtime.SessionConfig{
		ID:     "sess_" + uuid.NewString(),
		Object: "realtime.session",
		Type:   "realtime",
		Audio: &realtime.AudioConfig{
			Input: &realtime.AudioInputConfig{
				Format:        &realtime.AudioFormat{Type: "audio/pcm", Rate: realtimeInputRate},
				TurnDetection: &realtime.TurnDetection{Type: "server_vad", CreateResponse: &createResponse},
			},
			Output: &realtime.AudioOutputConfig{
				Format: &realtime.AudioFormat{Type: "audio/pcm", Rate: realtimeOutputRate},
			},
		},
		OutputModalities: []string{"text", "audio"},
	}
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
			if src.Audio.Input.TurnDetection != nil {
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

func eventID() string { return "event_" + uuid.NewString() }

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
