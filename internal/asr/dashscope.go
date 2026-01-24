package asr

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
)

const defaultDashScopeEndpoint = "wss://dashscope.aliyuncs.com/api-ws/v1/inference"

type DashScopeRecognizer struct {
	cfg       Config
	conn      *websocket.Conn
	onResult  func(Result)
	writeMu   sync.Mutex
	startedCh chan struct{}
	doneCh    chan struct{}
	errCh     chan error
	taskID    string

	startedOnce sync.Once
	doneOnce    sync.Once
}

func NewDashScopeRecognizer(cfg Config) (*DashScopeRecognizer, error) {
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

	return &DashScopeRecognizer{
		cfg:       cfg,
		startedCh: make(chan struct{}),
		doneCh:    make(chan struct{}),
		errCh:     make(chan error, 1),
	}, nil
}

func (r *DashScopeRecognizer) OnResult(handler func(Result)) {
	r.onResult = handler
}

func (r *DashScopeRecognizer) Start(ctx context.Context) error {
	if r.conn != nil {
		return errors.New("recognizer already started")
	}

	conn, err := r.connect(ctx)
	if err != nil {
		return err
	}
	r.conn = conn

	r.taskID = newTaskID()
	if err := r.sendRunTask(ctx); err != nil {
		return err
	}

	r.startReceiver()

	select {
	case <-r.startedCh:
		return nil
	case err := <-r.errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *DashScopeRecognizer) SendAudio(ctx context.Context, data []byte) error {
	if r.conn == nil {
		return errors.New("recognizer not started")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	r.writeMu.Lock()
	err := r.conn.WriteMessage(websocket.BinaryMessage, data)
	r.writeMu.Unlock()
	return err
}

func (r *DashScopeRecognizer) Finish(ctx context.Context) error {
	if r.conn == nil {
		return errors.New("recognizer not started")
	}
	if err := r.sendFinishTask(ctx); err != nil {
		return err
	}
	select {
	case <-r.doneCh:
		return nil
	case err := <-r.errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *DashScopeRecognizer) Close() error {
	if r.conn == nil {
		return nil
	}
	return r.conn.Close()
}

func (r *DashScopeRecognizer) connect(ctx context.Context) (*websocket.Conn, error) {
	header := http.Header{}
	header.Set("Authorization", fmt.Sprintf("Bearer %s", r.cfg.APIKey))
	dialer := websocket.DefaultDialer
	conn, _, err := dialer.DialContext(ctx, r.cfg.Endpoint, header)
	return conn, err
}

func (r *DashScopeRecognizer) sendRunTask(ctx context.Context) error {
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
			TaskID:    r.taskID,
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
	err = r.conn.WriteMessage(websocket.TextMessage, payload)
	r.writeMu.Unlock()
	return err
}

func (r *DashScopeRecognizer) sendFinishTask(ctx context.Context) error {
	msg := finishTaskMessage{
		Header: taskHeader{
			Action:    "finish-task",
			TaskID:    r.taskID,
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
	err = r.conn.WriteMessage(websocket.TextMessage, payload)
	r.writeMu.Unlock()
	return err
}

func (r *DashScopeRecognizer) startReceiver() {
	go func() {
		for {
			_, data, err := r.conn.ReadMessage()
			if err != nil {
				r.setErr(err)
				r.markDone()
				return
			}
			var event eventMessage
			if err := json.Unmarshal(data, &event); err != nil {
				r.setErr(err)
				r.markDone()
				return
			}
			if r.handleEvent(event) {
				r.markDone()
				return
			}
		}
	}()
}

func (r *DashScopeRecognizer) handleEvent(event eventMessage) bool {
	switch event.Header.Event {
	case "task-started":
		r.startedOnce.Do(func() { close(r.startedCh) })
	case "result-generated":
		if event.Payload.Output == nil || event.Payload.Output.Sentence == nil {
			return false
		}
		sentence := event.Payload.Output.Sentence
		if sentence.Heartbeat {
			return false
		}
		if sentence.Text == "" {
			return false
		}
		if r.onResult != nil {
			result := Result{
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
		return true
	case "task-failed":
		if event.Header.ErrorMessage != "" {
			r.setErr(fmt.Errorf("task failed: %s", event.Header.ErrorMessage))
		} else {
			r.setErr(errors.New("task failed"))
		}
		return true
	}
	return false
}

func (r *DashScopeRecognizer) setErr(err error) {
	select {
	case r.errCh <- err:
	default:
	}
}

func (r *DashScopeRecognizer) markDone() {
	r.doneOnce.Do(func() { close(r.doneCh) })
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
