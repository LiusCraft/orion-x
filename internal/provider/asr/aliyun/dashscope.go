package aliyun

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	asr "github.com/liuscraft/orion-x/internal/provider/asr"
)

const defaultDashScopeEndpoint = "wss://dashscope.aliyuncs.com/api-ws/v1/inference"

func init() {
	asr.Register(asr.TypeAliyun, func(cfg asr.Config) (asr.Recognizer, error) {
		return NewDashScopeRecognizer(cfg)
	}, asr.ProviderMeta{
		Name:           "阿里云 Dashscope",
		DefaultBaseURL: defaultDashScopeEndpoint,
	})
}

type DashScopeRecognizer struct {
	cfg        asr.Config
	conn       *websocket.Conn
	onResult   func(asr.Result)
	writeMu    sync.Mutex
	startedCh  chan struct{}
	doneCh     chan struct{}
	errCh      chan error
	taskID     string
	failed     bool
	taskActive bool

	mu sync.Mutex
}

func NewDashScopeRecognizer(cfg asr.Config) (*DashScopeRecognizer, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("DASHSCOPE_API_KEY is required")
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = defaultDashScopeEndpoint
	}
	if cfg.Model == "" {
		cfg.Model = "fun-asr-realtime"
	}
	if cfg.Format == "" {
		cfg.Format = "pcm"
	}
	if cfg.SampleRate == 0 {
		cfg.SampleRate = 16000
	}

	return &DashScopeRecognizer{cfg: cfg}, nil
}

func (r *DashScopeRecognizer) OnResult(handler func(asr.Result)) {
	r.onResult = handler
}

func (r *DashScopeRecognizer) Start(ctx context.Context) error {
	r.mu.Lock()
	if r.taskActive {
		r.mu.Unlock()
		return errors.New("recognizer task already started")
	}
	if r.failed && r.conn != nil {
		r.mu.Unlock()
		_ = r.Close()
		r.mu.Lock()
	}
	if r.conn == nil {
		conn, err := r.connect(ctx)
		if err != nil {
			r.mu.Unlock()
			return err
		}
		r.conn = conn
		r.failed = false
		r.startReceiver(conn)
	}
	r.taskID = newTaskID()
	r.startedCh = make(chan struct{})
	r.doneCh = make(chan struct{})
	r.errCh = make(chan error, 1)
	r.taskActive = true
	startedCh := r.startedCh
	errCh := r.errCh
	r.mu.Unlock()

	if err := r.sendRunTask(ctx); err != nil {
		_ = r.Close()
		return err
	}

	select {
	case <-startedCh:
		return nil
	case err := <-errCh:
		_ = r.Close()
		return err
	case <-ctx.Done():
		_ = r.Close()
		return ctx.Err()
	}
}

func (r *DashScopeRecognizer) SendAudio(ctx context.Context, data []byte) error {
	r.mu.Lock()
	conn := r.conn
	taskActive := r.taskActive
	r.mu.Unlock()
	if conn == nil || !taskActive {
		return errors.New("recognizer not started")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	result := make(chan error, 1)
	r.writeMu.Lock()
	go func() {
		err := conn.WriteMessage(websocket.BinaryMessage, data)
		r.writeMu.Unlock()
		result <- err
	}()

	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		_ = conn.Close()
		return ctx.Err()
	}
}

func (r *DashScopeRecognizer) Finish(ctx context.Context) error {
	r.mu.Lock()
	doneCh := r.doneCh
	errCh := r.errCh
	taskActive := r.taskActive
	r.mu.Unlock()
	if r.conn == nil || doneCh == nil || !taskActive {
		return errors.New("recognizer not started")
	}
	if err := r.sendFinishTask(ctx); err != nil {
		return err
	}
	select {
	case <-doneCh:
		select {
		case err := <-errCh:
			r.clearCurrentTask()
			return err
		default:
		}
		r.clearCurrentTask()
		return nil
	case err := <-errCh:
		r.clearCurrentTask()
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *DashScopeRecognizer) Close() error {
	r.mu.Lock()
	conn := r.conn
	r.conn = nil
	r.startedCh = nil
	r.doneCh = nil
	r.errCh = nil
	r.taskID = ""
	r.failed = false
	r.taskActive = false
	r.mu.Unlock()
	if conn == nil {
		return nil
	}
	return conn.Close()
}

func (r *DashScopeRecognizer) connect(ctx context.Context) (*websocket.Conn, error) {
	header := http.Header{}
	header.Set("Authorization", fmt.Sprintf("Bearer %s", r.cfg.APIKey))
	dialer := websocket.DefaultDialer
	conn, _, err := dialer.DialContext(ctx, r.cfg.Endpoint, header)
	return conn, err
}

func (r *DashScopeRecognizer) sendRunTask(ctx context.Context) error {
	r.mu.Lock()
	conn := r.conn
	taskID := r.taskID
	r.mu.Unlock()
	if conn == nil {
		return errors.New("recognizer not started")
	}

	params := map[string]any{
		"format":      r.cfg.Format,
		"sample_rate": r.cfg.SampleRate,
	}
	if r.cfg.VocabularyID != "" {
		params["vocabulary_id"] = r.cfg.VocabularyID
	}
	if r.cfg.SemanticPunctuationEnabled != nil {
		params["semantic_punctuation_enabled"] = *r.cfg.SemanticPunctuationEnabled
	}
	if r.cfg.MaxSentenceSilence > 0 {
		params["max_sentence_silence"] = r.cfg.MaxSentenceSilence
	}
	if r.cfg.MultiThresholdModeEnabled != nil {
		params["multi_threshold_mode_enabled"] = *r.cfg.MultiThresholdModeEnabled
	}
	if r.cfg.Heartbeat != nil {
		params["heartbeat"] = *r.cfg.Heartbeat
	}
	if len(r.cfg.LanguageHints) > 0 {
		params["language_hints"] = r.cfg.LanguageHints
	}

	msg := runTaskMessage{
		Header: taskHeader{
			Action:    "run-task",
			TaskID:    taskID,
			Streaming: "duplex",
		},
		Payload: taskPayload{
			TaskGroup:  "audio",
			Task:       "asr",
			Function:   "recognition",
			Model:      r.cfg.Model,
			Parameters: params,
			Input:      map[string]any{},
		},
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	r.writeMu.Lock()
	err = conn.WriteMessage(websocket.TextMessage, payload)
	r.writeMu.Unlock()
	return err
}

func (r *DashScopeRecognizer) sendFinishTask(ctx context.Context) error {
	r.mu.Lock()
	conn := r.conn
	taskID := r.taskID
	r.mu.Unlock()
	if conn == nil {
		return errors.New("recognizer not started")
	}

	msg := finishTaskMessage{
		Header: taskHeader{
			Action:    "finish-task",
			TaskID:    taskID,
			Streaming: "duplex",
		},
		Payload: taskPayload{
			Input: map[string]any{},
		},
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	r.writeMu.Lock()
	err = conn.WriteMessage(websocket.TextMessage, payload)
	r.writeMu.Unlock()
	return err
}

func (r *DashScopeRecognizer) startReceiver(conn *websocket.Conn) {
	go func() {
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				r.markFailed()
				r.setErr(err)
				r.markDone()
				return
			}
			var event eventMessage
			if err := json.Unmarshal(data, &event); err != nil {
				r.markFailed()
				r.setErr(err)
				r.markDone()
				return
			}
			r.handleEvent(event)
		}
	}()
}

func (r *DashScopeRecognizer) handleEvent(event eventMessage) {
	switch event.Header.Event {
	case "task-started":
		r.markStarted()
	case "result-generated":
		if event.Payload.Output == nil || event.Payload.Output.Sentence == nil {
			return
		}
		sentence := event.Payload.Output.Sentence
		if sentence.Heartbeat {
			return
		}
		if sentence.Text == "" {
			return
		}
		if r.onResult != nil {
			result := asr.Result{
				Text:        sentence.Text,
				IsFinal:     sentence.SentenceEnd,
				BeginTimeMs: sentence.BeginTime,
				EndTimeMs:   sentence.EndTime,
			}
			if event.Payload.Usage != nil {
				result.UsageDuration = &event.Payload.Usage.Duration
			}
			r.onResult(result)
		}
	case "task-finished":
		r.markDone()
	case "task-failed":
		r.markFailed()
		if event.Header.ErrorMessage != "" {
			r.setErr(fmt.Errorf("task failed: %s", event.Header.ErrorMessage))
		} else {
			r.setErr(errors.New("task failed"))
		}
		r.markDone()
	}
}

func (r *DashScopeRecognizer) setErr(err error) {
	r.mu.Lock()
	errCh := r.errCh
	r.mu.Unlock()
	if errCh == nil {
		return
	}
	select {
	case errCh <- err:
	default:
	}
}

func (r *DashScopeRecognizer) markDone() {
	r.mu.Lock()
	doneCh := r.doneCh
	if doneCh != nil {
		r.doneCh = nil
	}
	r.mu.Unlock()
	if doneCh != nil {
		close(doneCh)
	}
}

func (r *DashScopeRecognizer) markStarted() {
	r.mu.Lock()
	startedCh := r.startedCh
	if startedCh != nil {
		r.startedCh = nil
	}
	r.mu.Unlock()
	if startedCh != nil {
		close(startedCh)
	}
}

func (r *DashScopeRecognizer) markFailed() {
	r.mu.Lock()
	r.failed = true
	r.mu.Unlock()
}

func (r *DashScopeRecognizer) clearCurrentTask() {
	r.mu.Lock()
	r.startedCh = nil
	r.doneCh = nil
	r.errCh = nil
	r.taskID = ""
	r.taskActive = false
	r.mu.Unlock()
}

type runTaskMessage struct {
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
	Usage      *taskUsage     `json:"usage,omitempty"`
}

type eventMessage struct {
	Header  taskHeader  `json:"header"`
	Payload taskPayload `json:"payload"`
}

type taskOutput struct {
	Sentence *taskSentence `json:"sentence,omitempty"`
}

type taskSentence struct {
	BeginTime   int64  `json:"begin_time"`
	EndTime     *int64 `json:"end_time"`
	Text        string `json:"text"`
	Heartbeat   bool   `json:"heartbeat"`
	SentenceEnd bool   `json:"sentence_end"`
}

type taskUsage struct {
	Duration int `json:"duration"`
}

func newTaskID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "fallback-task-id"
	}
	return hex.EncodeToString(bytes[:])
}
