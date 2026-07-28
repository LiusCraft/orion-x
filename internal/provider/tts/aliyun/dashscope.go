package aliyun

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/liuscraft/orion-x/internal/logging"
	tts "github.com/liuscraft/orion-x/internal/provider/tts"
)

const defaultDashScopeEndpoint = "wss://dashscope.aliyuncs.com/api-ws/v1/inference"

// DashScopeProvider 是有状态的 TTS Provider，基础配置在创建时注入。
type DashScopeProvider struct {
	cfg tts.Config

	warming atomic.Bool
}

func NewDashScopeProvider(cfg tts.Config) (*DashScopeProvider, error) {
	normalized, err := normalizeConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &DashScopeProvider{cfg: normalized}, nil
}

// ── Synthesizer 接口 ──

func (p *DashScopeProvider) Synthesize(ctx context.Context, req tts.SynthesizeRequest) (*tts.SynthesizeResult, error) {
	if strings.TrimSpace(req.Input.Text) == "" {
		return &tts.SynthesizeResult{
			Audio:      io.NopCloser(strings.NewReader("")),
			Format:     tts.FormatPCM,
			SampleRate: p.cfg.SampleRate,
		}, nil
	}

	totalStart := time.Now()
	logging.Infof("AliyunTTS: synthesize start (text_len=%d, model=%s, voice=%s)",
		len([]rune(req.Input.Text)), p.cfg.Model, p.cfg.Voice)

	stream, err := p.newStream(ctx, req)
	if err != nil {
		logging.Errorf("AliyunTTS: create stream failed after %v: %v", time.Since(totalStart), err)
		return nil, err
	}

	if err := stream.WriteTextChunk(ctx, req.Input.Text); err != nil {
		_ = stream.closeStream(ctx)
		logging.Errorf("AliyunTTS: write text failed after %v: %v", time.Since(totalStart), err)
		return nil, err
	}

	if err := stream.closeStream(ctx); err != nil {
		logging.Errorf("AliyunTTS: close stream failed after %v: %v", time.Since(totalStart), err)
		return nil, err
	}

	logging.Infof("AliyunTTS: synthesize done (text_len=%d, total=%v)", len([]rune(req.Input.Text)), time.Since(totalStart))
	return &tts.SynthesizeResult{
		Audio:      stream.audioBuf,
		Format:     formatToSDK(req.Audio.Format),
		SampleRate: p.cfg.SampleRate,
	}, nil
}

// ── StreamingSynthesizer 接口 ──

func (p *DashScopeProvider) StartSynthesis(ctx context.Context, req tts.SynthesizeRequest) (tts.SynthesisStream, error) {
	return p.newStream(ctx, req)
}

// ── WarmableProvider 接口 ──

func (p *DashScopeProvider) Warm(ctx context.Context, req tts.SynthesizeRequest) tts.SynthesisStream {
	if !p.warming.CompareAndSwap(false, true) {
		return nil
	}
	defer p.warming.Store(false)

	stream, err := p.newStream(ctx, req)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			logging.Warnf("AliyunTTS: warm failed: %v", err)
		}
		return nil
	}
	logging.Infof("AliyunTTS: warm stream ready")
	return stream
}

func (p *DashScopeProvider) newStream(ctx context.Context, req tts.SynthesizeRequest) (*dashScopeStream, error) {
	streamStart := time.Now()
	conn, err := connectDashScope(ctx, p.cfg)
	if err != nil {
		logging.Errorf("AliyunTTS: websocket connect failed after %v: %v", time.Since(streamStart), err)
		return nil, err
	}
	logging.Infof("AliyunTTS: websocket connected in %v", time.Since(streamStart))

	audioBuf := newBufferedPipe(1024 * 1024)

	stream := &dashScopeStream{
		createdAt:          streamStart,
		cfg:                p.cfg,
		req:                req,
		conn:               conn,
		audioBuf:           audioBuf,
		startedCh:          make(chan struct{}),
		doneCh:             make(chan struct{}),
		errCh:              make(chan error, 1),
		taskID:             newTaskID(),
		sentenceBoundaryCh: make(chan tts.SentenceBoundary, 16),
	}

	stream.startReceiver()

	if err := stream.sendRunTask(ctx); err != nil {
		_ = conn.Close()
		_ = audioBuf.Close()
		logging.Errorf("AliyunTTS: send run-task failed: %v", err)
		return nil, err
	}

	if err := stream.waitStarted(ctx); err != nil {
		_ = conn.Close()
		_ = audioBuf.Close()
		logging.Errorf("AliyunTTS: wait task-started failed: %v", err)
		return nil, err
	}

	return stream, nil
}

type dashScopeStream struct {
	createdAt time.Time

	cfg      tts.Config
	req      tts.SynthesizeRequest
	conn     *websocket.Conn
	audioBuf *bufferedPipe
	writeMu  sync.Mutex

	startedCh chan struct{}
	doneCh    chan struct{}
	errCh     chan error
	taskID    string

	startedOnce sync.Once
	doneOnce    sync.Once
	finishOnce  sync.Once

	metricsMu      sync.Mutex
	taskStartedAt  time.Time
	taskFinishedAt time.Time
	firstAudioAt   time.Time
	audioBytes     int
	audioFrames    int

	sentenceBoundaryCh chan tts.SentenceBoundary
}

type bufferedPipe struct {
	buf    []byte
	mu     sync.Mutex
	cond   *sync.Cond
	closed bool
	maxLen int
}

func newBufferedPipe(maxLen int) *bufferedPipe {
	bp := &bufferedPipe{
		buf:    make([]byte, 0, maxLen),
		maxLen: maxLen,
	}
	bp.cond = sync.NewCond(&bp.mu)
	return bp
}

func (bp *bufferedPipe) Write(p []byte) (int, error) {
	bp.mu.Lock()
	defer bp.mu.Unlock()
	if bp.closed {
		return 0, io.ErrClosedPipe
	}
	bp.buf = append(bp.buf, p...)
	bp.cond.Signal()
	return len(p), nil
}

func (bp *bufferedPipe) Read(p []byte) (int, error) {
	bp.mu.Lock()
	defer bp.mu.Unlock()
	for len(bp.buf) == 0 && !bp.closed {
		bp.cond.Wait()
	}
	if len(bp.buf) == 0 && bp.closed {
		return 0, io.EOF
	}
	n := copy(p, bp.buf)
	bp.buf = bp.buf[n:]
	return n, nil
}

func (bp *bufferedPipe) Close() error {
	bp.mu.Lock()
	defer bp.mu.Unlock()
	bp.closed = true
	bp.cond.Broadcast()
	return nil
}

func (s *dashScopeStream) closeStream(ctx context.Context) error {
	if err := s.Finish(ctx); err != nil {
		return err
	}

	select {
	case <-s.doneCh:
		err := s.streamErr()
		if err != nil {
			return err
		}
		return nil
	case err := <-s.errCh:
		return err
	case <-ctx.Done():
		s.closeWithError(ctx.Err())
		return ctx.Err()
	}
}

func (s *dashScopeStream) WriteTextChunk(ctx context.Context, text string) error {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	if err := s.waitStarted(ctx); err != nil {
		return err
	}
	return s.sendContinueTask(ctx, text)
}

func (s *dashScopeStream) Finish(ctx context.Context) error {
	var err error
	s.finishOnce.Do(func() {
		err = s.sendFinishTask(ctx)
	})
	if err != nil {
		s.closeWithError(err)
	}
	return err
}

func (s *dashScopeStream) AudioReader() io.ReadCloser { return s.audioBuf }

func (s *dashScopeStream) Abort() {
	s.setErr(context.Canceled)
	s.markDone()
}

func (s *dashScopeStream) SentenceBoundaries() <-chan tts.SentenceBoundary {
	return s.sentenceBoundaryCh
}

func (s *dashScopeStream) waitStarted(ctx context.Context) error {
	select {
	case <-s.startedCh:
		return nil
	case err := <-s.errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *dashScopeStream) sendRunTask(_ context.Context) error {
	params := buildDashScopeParams(s.cfg, s.req)

	payload := runTaskMessage{
		Header: taskHeader{
			Action:    "run-task",
			TaskID:    s.taskID,
			Streaming: "duplex",
		},
		Payload: taskPayload{
			TaskGroup:  "audio",
			Task:       "tts",
			Function:   "SpeechSynthesizer",
			Model:      pickModel(s.cfg, s.req),
			Parameters: params,
			Input:      map[string]any{},
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	s.writeMu.Lock()
	err = s.conn.WriteMessage(websocket.TextMessage, data)
	s.writeMu.Unlock()
	return err
}

func (s *dashScopeStream) sendContinueTask(_ context.Context, text string) error {
	payload := continueTaskMessage{
		Header: taskHeader{
			Action:    "continue-task",
			TaskID:    s.taskID,
			Streaming: "duplex",
		},
		Payload: taskPayload{
			Input: map[string]any{"text": text},
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	s.writeMu.Lock()
	err = s.conn.WriteMessage(websocket.TextMessage, data)
	s.writeMu.Unlock()
	return err
}

func (s *dashScopeStream) sendFinishTask(_ context.Context) error {
	payload := finishTaskMessage{
		Header: taskHeader{
			Action:    "finish-task",
			TaskID:    s.taskID,
			Streaming: "duplex",
		},
		Payload: taskPayload{Input: map[string]any{}},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	s.writeMu.Lock()
	err = s.conn.WriteMessage(websocket.TextMessage, data)
	s.writeMu.Unlock()
	return err
}

func (s *dashScopeStream) startReceiver() {
	go func() {
		for {
			messageType, data, err := s.conn.ReadMessage()
			if err != nil {
				s.closeWithError(err)
				return
			}
			if messageType == websocket.BinaryMessage {
				if _, err := s.audioBuf.Write(data); err != nil {
					s.closeWithError(err)
					return
				}
				s.recordAudioFrame(len(data))
				continue
			}
			if messageType != websocket.TextMessage {
				continue
			}
			var event eventMessage
			if err := json.Unmarshal(data, &event); err != nil {
				s.closeWithError(err)
				return
			}
			if s.handleEvent(event) {
				return
			}
		}
	}()
}

func (s *dashScopeStream) handleEvent(event eventMessage) bool {
	switch event.Header.Event {
	case "task-started":
		s.recordTaskStarted()
		s.startedOnce.Do(func() { close(s.startedCh) })
	case "task-finished":
		s.recordTaskFinished()
		s.markDone()
		return true
	case "task-failed":
		err := mapDashScopeError(event.Header.ErrorCode, event.Header.ErrorMessage)
		s.closeWithError(err)
		return true
	case "result-generated":
		s.handleResultGenerated(event.Payload)
	}
	return false
}

func (s *dashScopeStream) handleResultGenerated(payload taskPayload) {
	if payload.Output == nil {
		return
	}
	text := payload.Output.OriginalText
	if text == "" {
		return
	}
	switch payload.Output.Type {
	case "sentence-begin":
		select {
		case s.sentenceBoundaryCh <- tts.SentenceBoundary{Offset: -1, Text: text, IsBegin: true}:
		default:
		}
	case "sentence-end":
		s.metricsMu.Lock()
		offset := s.audioBytes
		s.metricsMu.Unlock()
		select {
		case s.sentenceBoundaryCh <- tts.SentenceBoundary{Offset: offset, Text: text}:
		default:
		}
	}
}

func (s *dashScopeStream) recordTaskStarted() {
	now := time.Now()
	s.metricsMu.Lock()
	s.taskStartedAt = now
	s.metricsMu.Unlock()
}

func (s *dashScopeStream) recordTaskFinished() {
	s.metricsMu.Lock()
	s.taskFinishedAt = time.Now()
	s.metricsMu.Unlock()
}

func (s *dashScopeStream) recordAudioFrame(n int) {
	s.metricsMu.Lock()
	if s.firstAudioAt.IsZero() {
		s.firstAudioAt = time.Now()
	}
	s.audioBytes += n
	s.audioFrames++
	s.metricsMu.Unlock()
}

func (s *dashScopeStream) closeWithError(err error) {
	s.setErr(err)
	s.markDone()
}

func (s *dashScopeStream) setErr(err error) {
	if err == nil {
		return
	}
	select {
	case s.errCh <- err:
	default:
	}
}

func (s *dashScopeStream) markDone() {
	s.doneOnce.Do(func() {
		_ = s.audioBuf.Close()
		_ = s.conn.Close()
		close(s.doneCh)
	})
}

func (s *dashScopeStream) streamErr() error {
	select {
	case err := <-s.errCh:
		return err
	default:
		return nil
	}
}

// ── 参数映射 ──

func buildDashScopeParams(cfg tts.Config, req tts.SynthesizeRequest) map[string]any {
	voice := pickVoice(cfg, req)
	sampleRate := cfg.SampleRate
	if req.Audio.SampleRate > 0 {
		sampleRate = req.Audio.SampleRate
	}

	rate := tts.SpeedToRate(req.Speech.Speed)
	if rate == 0 {
		rate = 1.0
	}
	pitch := tts.PitchToRatio(req.Speech.Pitch)
	if pitch == 0 {
		pitch = 1.0
	}
	volume := tts.VolumeToPercent(req.Speech.Volume)
	if volume == 0 {
		volume = 50
	}

	textType := "PlainText"
	if req.Input.TextType == tts.TextTypeSSML {
		textType = "SSML"
	}

	params := map[string]any{
		"text_type":              textType,
		"voice":                  voice,
		"format":                 formatFromSDK(req.Audio.Format),
		"sample_rate":            sampleRate,
		"volume":                 volume,
		"rate":                   rate,
		"pitch":                  pitch,
		"enable_ssml":            req.Input.TextType == tts.TextTypeSSML,
		"word_timestamp_enabled": true,
	}

	if emotion := req.Speech.Emotion; emotion != "" {
		if mapped := mapEmotion(voice, emotion); mapped != "" {
			params["instruction"] = fmt.Sprintf("你说话的情感是%s。", mapped)
		}
	}
	return params
}

func pickVoice(cfg tts.Config, req tts.SynthesizeRequest) string {
	if req.Voice.VoiceID != "" {
		return req.Voice.VoiceID
	}
	return cfg.Voice
}

func pickModel(cfg tts.Config, req tts.SynthesizeRequest) string {
	if req.Voice.Model != "" {
		return req.Voice.Model
	}
	if cfg.Model != "" {
		return cfg.Model
	}
	return "cosyvoice-v3-flash"
}

func formatFromSDK(f tts.AudioFormat) string {
	switch f {
	case tts.FormatPCM:
		return "pcm"
	case tts.FormatMP3:
		return "mp3"
	case tts.FormatWAV:
		return "wav"
	case tts.FormatOpus:
		return "opus"
	default:
		return "pcm"
	}
}

func formatToSDK(f tts.AudioFormat) tts.AudioFormat {
	if f == "" {
		return tts.FormatPCM
	}
	return f
}

func mapEmotion(voice, emotion string) string {
	_ = voice
	emojiMap := map[string]string{
		"😊": "happy", "😄": "happy", "😃": "happy", "😁": "happy",
		"😢": "sad", "😭": "sad", "😥": "sad",
		"😡": "angry", "🤬": "angry", "😠": "angry",
		"😌": "calm",
		"🎉": "excited", "🥳": "excited",
	}
	if v, ok := emojiMap[emotion]; ok {
		return v
	}
	switch emotion {
	case "happy", "sad", "angry", "calm", "excited":
		return emotion
	}
	return ""
}

func normalizeConfig(cfg tts.Config) (tts.Config, error) {
	if cfg.APIKey == "" {
		return tts.Config{}, errors.New("DASHSCOPE_API_KEY is required")
	}
	if strings.TrimSpace(cfg.Endpoint) == "" {
		cfg.Endpoint = defaultDashScopeEndpoint
	}
	if cfg.Model == "" {
		cfg.Model = "cosyvoice-v3-flash"
	}
	if cfg.Voice == "" {
		cfg.Voice = "longanhuan_v3"
	}
	if cfg.SampleRate == 0 {
		cfg.SampleRate = 16000
	}
	return cfg, nil
}

func connectDashScope(ctx context.Context, cfg tts.Config) (*websocket.Conn, error) {
	header := http.Header{}
	header.Set("Authorization", fmt.Sprintf("bearer %s", cfg.APIKey))
	if cfg.Extra != nil {
		if v, ok := cfg.Extra["data_inspection"].(bool); ok && v {
			header.Set("X-DashScope-DataInspection", "enable")
		}
		if v, ok := cfg.Extra["workspace"].(string); ok && strings.TrimSpace(v) != "" {
			header.Set("X-DashScope-WorkSpace", strings.TrimSpace(v))
		}
	}
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, cfg.Endpoint, header)
	return conn, err
}

// ── 协议类型 ──

type runTaskMessage struct {
	Header  taskHeader  `json:"header"`
	Payload taskPayload `json:"payload"`
}
type continueTaskMessage struct {
	Header  taskHeader  `json:"header"`
	Payload taskPayload `json:"payload"`
}
type finishTaskMessage struct {
	Header  taskHeader  `json:"header"`
	Payload taskPayload `json:"payload"`
}
type taskHeader struct {
	Action       string `json:"action,omitempty"`
	TaskID       string `json:"task_id,omitempty"`
	Streaming    string `json:"streaming,omitempty"`
	Event        string `json:"event,omitempty"`
	ErrorCode    string `json:"error_code,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
}
type taskPayload struct {
	TaskGroup  string         `json:"task_group,omitempty"`
	Task       string         `json:"task,omitempty"`
	Function   string         `json:"function,omitempty"`
	Model      string         `json:"model,omitempty"`
	Parameters map[string]any `json:"parameters,omitempty"`
	Input      map[string]any `json:"input"`
	Output     *taskOutput    `json:"output,omitempty"`
}
type taskOutput struct {
	Type         string        `json:"type,omitempty"`
	OriginalText string        `json:"original_text,omitempty"`
	Sentence     *taskSentence `json:"sentence,omitempty"`
}
type taskSentence struct {
	Index int    `json:"index"`
	Type  string `json:"type,omitempty"`
}
type eventMessage struct {
	Header  taskHeader  `json:"header"`
	Payload taskPayload `json:"payload"`
}

func mapDashScopeError(code, message string) error {
	logging.Errorf("TTS error: code=%s, message=%s", code, message)
	lower := strings.ToLower(code + " " + message)
	switch {
	case strings.Contains(lower, "unauthorized"), strings.Contains(lower, "authentication"):
		return fmt.Errorf("%w: %s", tts.ErrAuth, message)
	case strings.Contains(lower, "invalidparameter"), strings.Contains(lower, "bad request"):
		return fmt.Errorf("%w: %s", tts.ErrBadRequest, message)
	case strings.Contains(lower, "timeout"), strings.Contains(lower, "tempor"):
		return fmt.Errorf("%w: %s", tts.ErrTransient, message)
	}
	if message == "" {
		message = "dashscope task failed"
	}
	return errors.New(message)
}

func newTaskID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "fallback-task-id"
	}
	return hex.EncodeToString(b[:])
}
