package tts

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
)

const defaultDashScopeEndpoint = "wss://dashscope.aliyuncs.com/api-ws/v1/inference"

type DashScopeProvider struct{}

func NewDashScopeProvider() *DashScopeProvider {
	return &DashScopeProvider{}
}

func (p *DashScopeProvider) Start(ctx context.Context, cfg Config) (Stream, error) {
	normalized, err := normalizeConfig(cfg)
	if err != nil {
		return nil, err
	}

	conn, err := connectDashScope(ctx, normalized)
	if err != nil {
		return nil, err
	}

	// Use a buffered channel-based pipe to avoid deadlock
	// The standard io.Pipe blocks on Write if no one is reading,
	// but generateTTS waits for Close() before returning the reader.
	// This creates a deadlock. Using a buffer allows writes to proceed.
	audioBuf := newBufferedPipe(1024 * 1024) // 1MB buffer for audio data

	stream := &dashScopeStream{
		cfg:       normalized,
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
	cfg       Config
	conn      *websocket.Conn
	audioBuf  *bufferedPipe
	writeMu   sync.Mutex
	startedCh chan struct{}
	doneCh    chan struct{}
	errCh     chan error
	taskID    string

	startedOnce sync.Once
	doneOnce    sync.Once
	finishOnce  sync.Once
}

// bufferedPipe is a thread-safe buffered pipe that doesn't block on write
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

	// If buffer is full, wait for some space or just append (may grow)
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

func (s *dashScopeStream) AudioReader() io.ReadCloser {
	return s.audioBuf
}

func (s *dashScopeStream) SampleRate() int {
	// DashScope TTS 根据配置返回采样率
	// 默认为 16000 Hz
	if s.cfg.SampleRate > 0 {
		return s.cfg.SampleRate
	}
	return 16000
}

func (s *dashScopeStream) Channels() int {
	// DashScope TTS 输出单声道 PCM
	return 1
}

func (s *dashScopeStream) WriteTextChunk(ctx context.Context, text string) error {
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

func (s *dashScopeStream) Close(ctx context.Context) error {
	var finishErr error
	s.finishOnce.Do(func() {
		finishErr = s.sendFinishTask(ctx)
	})
	if finishErr != nil {
		s.closeWithError(finishErr)
		return finishErr
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

func (s *dashScopeStream) sendRunTask(ctx context.Context) error {
	payload := runTaskMessage{
		Header: taskHeader{
			Action:    "run-task",
			TaskID:    s.taskID,
			Streaming: "duplex",
		},
		Payload: taskPayload{
			TaskGroup: "audio",
			Task:      "tts",
			Function:  "SpeechSynthesizer",
			Model:     s.cfg.Model,
			Parameters: map[string]any{
				"text_type":   s.cfg.TextType,
				"voice":       s.cfg.Voice,
				"format":      s.cfg.Format,
				"sample_rate": s.cfg.SampleRate,
				"volume":      s.cfg.Volume,
				"rate":        s.cfg.Rate,
				"pitch":       s.cfg.Pitch,
				"enable_ssml": s.cfg.EnableSSML,
			},
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

func (s *dashScopeStream) sendContinueTask(ctx context.Context, text string) error {
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

func (s *dashScopeStream) sendFinishTask(ctx context.Context) error {
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
	// result-generated is expected, ignore it
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

func normalizeConfig(cfg Config) (Config, error) {
	if cfg.APIKey == "" {
		return Config{}, errors.New("DASHSCOPE_API_KEY is required")
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

func connectDashScope(ctx context.Context, cfg Config) (*websocket.Conn, error) {
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
		return fmt.Errorf("%w: %s", ErrAuth, message)
	case strings.Contains(lower, "invalidparameter"), strings.Contains(lower, "bad request"):
		return fmt.Errorf("%w: %s", ErrBadRequest, message)
	case strings.Contains(lower, "timeout"), strings.Contains(lower, "tempor"):
		return fmt.Errorf("%w: %s", ErrTransient, message)
	}
	if message == "" {
		message = "dashscope task failed"
	}
	return errors.New(message)
}

func newTaskID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "fallback-task-id"
	}
	return hex.EncodeToString(bytes[:])
}
