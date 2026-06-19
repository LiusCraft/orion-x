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

	"github.com/gorilla/websocket"
	"github.com/liuscraft/orion-x/internal/logging"
	tts "github.com/liuscraft/orion-x/internal/provider/tts"
)

const defaultDashScopeEndpoint = "wss://dashscope.aliyuncs.com/api-ws/v1/inference"

// DashScopeProvider 是有状态的 TTS Provider，基础配置在创建时注入。
type DashScopeProvider struct {
	cfg tts.Config // 已经 normalize 过
}

func init() {
	tts.Register(tts.TypeAliyun, func(cfg tts.Config) (tts.Provider, error) {
		return NewDashScopeProvider(cfg)
	})
}

func NewDashScopeProvider(cfg tts.Config) (*DashScopeProvider, error) {
	normalized, err := normalizeConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &DashScopeProvider{cfg: normalized}, nil
}

// Synthesize 合成一段文本，返回 PCM 音频 reader。
// 调用方在完整读取后需要 Close reader。
func (p *DashScopeProvider) Synthesize(ctx context.Context, text string, opts tts.SynthesisOptions) (io.ReadCloser, error) {
	if strings.TrimSpace(text) == "" {
		return io.NopCloser(strings.NewReader("")), nil
	}

	callCfg := p.cfg
	if opts.Rate > 0 {
		callCfg.Rate = opts.Rate
	}

	stream, err := p.newStream(ctx, callCfg, opts.Emotion)
	if err != nil {
		return nil, err
	}

	if err := stream.writeTextChunk(ctx, text); err != nil {
		_ = stream.closeStream(ctx)
		return nil, err
	}

	if err := stream.closeStream(ctx); err != nil {
		return nil, err
	}

	return stream.audioBuf, nil
}

func (p *DashScopeProvider) newStream(ctx context.Context, cfg tts.Config, emotion string) (*dashScopeStream, error) {
	conn, err := connectDashScope(ctx, cfg)
	if err != nil {
		return nil, err
	}

	audioBuf := newBufferedPipe(1024 * 1024)

	stream := &dashScopeStream{
		cfg:       cfg,
		emotion:   emotion,
		conn:      conn,
		audioBuf:  audioBuf,
		startedCh: make(chan struct{}),
		doneCh:    make(chan struct{}),
		errCh:     make(chan error, 1),
		taskID:    newTaskID(),
	}

	stream.startReceiver()

	if err := stream.sendRunTask(ctx); err != nil {
		_ = conn.Close()
		_ = audioBuf.Close()
		return nil, err
	}

	if err := stream.waitStarted(ctx); err != nil {
		_ = conn.Close()
		_ = audioBuf.Close()
		return nil, err
	}

	return stream, nil
}

type dashScopeStream struct {
	cfg      tts.Config
	emotion  string
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
}

// bufferedPipe is a thread-safe buffered pipe that doesn't block on write.
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

func (s *dashScopeStream) writeTextChunk(ctx context.Context, text string) error {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	if err := s.waitStarted(ctx); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	return s.sendContinueTask(ctx, text)
}

func (s *dashScopeStream) finish(ctx context.Context) error {
	var err error
	s.finishOnce.Do(func() {
		err = s.sendFinishTask(ctx)
	})
	if err != nil {
		s.closeWithError(err)
	}
	return err
}

// closeStream 发送 finish-task 并等待所有音频接收完毕，然后关闭连接。
func (s *dashScopeStream) closeStream(ctx context.Context) error {
	if err := s.finish(ctx); err != nil {
		return err
	}
	select {
	case <-s.doneCh:
		_ = s.conn.Close()
		return s.streamErr()
	case err := <-s.errCh:
		_ = s.conn.Close()
		return err
	case <-ctx.Done():
		_ = s.conn.Close()
		return ctx.Err()
	}
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
	params := map[string]any{
		"text_type":   s.cfg.TextType,
		"voice":       s.cfg.Voice,
		"format":      s.cfg.Format,
		"sample_rate": s.cfg.SampleRate,
		"volume":      s.cfg.Volume,
		"rate":        s.cfg.Rate,
		"pitch":       s.cfg.Pitch,
		"enable_ssml": s.cfg.EnableSSML,
	}
	// emotion 映射：系统内部 emotion → 该 voice 下可用的 emotion 参数
	// 具体映射值由 voice 决定，后续按 voice 完善
	if mapped := mapEmotion(s.cfg.Voice, s.emotion); mapped != "" {
		params["emotion"] = mapped
	}

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
			Model:      s.cfg.Model,
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
			Input: map[string]any{
				"text": text,
			},
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
		Payload: taskPayload{
			Input: map[string]any{},
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
		s.startedOnce.Do(func() { close(s.startedCh) })
	case "task-finished":
		s.markDone()
		return true
	case "task-failed":
		err := mapDashScopeError(event.Header.ErrorCode, event.Header.ErrorMessage)
		s.closeWithError(err)
		return true
	case "result-generated":
		// normal event, no action needed
	}
	return false
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

// mapEmotion 将系统内部 emotion 值映射到指定 voice 支持的 emotion 参数。
// 不同 voice 支持的 emotion 不同，返回空字符串表示不传 emotion 参数。
func mapEmotion(voice, emotion string) string {
	// TODO: 根据实际 voice 的支持情况完善映射表
	_ = voice
	_ = emotion
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
		cfg.Voice = "longanyang"
	}
	if cfg.Format == "" {
		cfg.Format = "mp3"
	}
	if cfg.SampleRate == 0 {
		cfg.SampleRate = 22050
	}
	if cfg.Volume == 0 {
		cfg.Volume = 50
	}
	if cfg.Rate == 0 {
		cfg.Rate = 1
	}
	if cfg.Pitch == 0 {
		cfg.Pitch = 1
	}
	if cfg.TextType == "" {
		cfg.TextType = "PlainText"
	}
	if cfg.EnableDataInspection == nil {
		enabled := true
		cfg.EnableDataInspection = &enabled
	}
	return cfg, nil
}

func connectDashScope(ctx context.Context, cfg tts.Config) (*websocket.Conn, error) {
	header := http.Header{}
	header.Set("Authorization", fmt.Sprintf("bearer %s", cfg.APIKey))
	if cfg.EnableDataInspection != nil && *cfg.EnableDataInspection {
		header.Set("X-DashScope-DataInspection", "enable")
	}
	if strings.TrimSpace(cfg.Workspace) != "" {
		header.Set("X-DashScope-WorkSpace", strings.TrimSpace(cfg.Workspace))
	}
	dialer := websocket.DefaultDialer
	conn, _, err := dialer.DialContext(ctx, cfg.Endpoint, header)
	return conn, err
}

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
}

type eventMessage struct {
	Header taskHeader `json:"header"`
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
