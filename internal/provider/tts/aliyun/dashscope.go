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
	cfg tts.Config // 已经 normalize 过

	// warming 防止并发预热（同时只允许一个 Warm 在运行）
	warming atomic.Bool
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

// Warm 同步建立 WebSocket 连接并完成 task-started 握手，返回就绪的 stream。
// ctx 取消时返回 nil。同时只允许一个 Warm 运行，重复调用直接返回 nil。
// 实现 tts.WarmableProvider 接口，调用方应在 goroutine 里调用。
func (p *DashScopeProvider) Warm(ctx context.Context, opts tts.SynthesisOptions) tts.SynthesisStream {
	if !p.warming.CompareAndSwap(false, true) {
		return nil
	}
	defer p.warming.Store(false)

	callCfg := p.cfg
	if opts.Rate > 0 {
		callCfg.Rate = opts.Rate
	}
	stream, err := p.newStream(ctx, callCfg, opts.Emotion)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			logging.Warnf("AliyunTTS: warm failed: %v", err)
		}
		return nil
	}
	logging.Infof("AliyunTTS: warm stream ready")
	return stream
}

// Synthesize 合成一段文本，返回 PCM 音频 reader。
// 调用方在完整读取后需要 Close reader。
func (p *DashScopeProvider) Synthesize(ctx context.Context, text string, opts tts.SynthesisOptions) (io.ReadCloser, error) {
	if strings.TrimSpace(text) == "" {
		return io.NopCloser(strings.NewReader("")), nil
	}

	totalStart := time.Now()
	callCfg := p.cfg
	if opts.Rate > 0 {
		callCfg.Rate = opts.Rate
	}
	logging.Infof("AliyunTTS: synthesize start (text_len=%d, model=%s, voice=%s)",
		len([]rune(text)), callCfg.Model, callCfg.Voice)

	stream, err := p.newStream(ctx, callCfg, opts.Emotion)
	if err != nil {
		logging.Errorf("AliyunTTS: create stream failed after %v: %v", time.Since(totalStart), err)
		return nil, err
	}

	if err := stream.WriteTextChunk(ctx, text); err != nil {
		_ = stream.closeStream(ctx)
		logging.Errorf("AliyunTTS: write text failed after %v: %v", time.Since(totalStart), err)
		return nil, err
	}

	if err := stream.closeStream(ctx); err != nil {
		logging.Errorf("AliyunTTS: close stream failed after %v: %v", time.Since(totalStart), err)
		return nil, err
	}

	logging.Infof("AliyunTTS: synthesize done (text_len=%d, total=%v)", len([]rune(text)), time.Since(totalStart))
	return stream.audioBuf, nil
}

// StartSynthesis 建立 WebSocket 连接并等待 task-started，返回可立即写文本的 stream。
// 实现 tts.StreamingProvider 接口，供 TTSProcessor 走流式播放路径。
func (p *DashScopeProvider) StartSynthesis(ctx context.Context, opts tts.SynthesisOptions) (tts.SynthesisStream, error) {
	callCfg := p.cfg
	if opts.Rate > 0 {
		callCfg.Rate = opts.Rate
	}
	return p.newStream(ctx, callCfg, opts.Emotion)
}

func (p *DashScopeProvider) newStream(ctx context.Context, cfg tts.Config, emotion string) (*dashScopeStream, error) {
	streamStart := time.Now()
	connectStart := time.Now()
	conn, err := connectDashScope(ctx, cfg)
	if err != nil {
		logging.Errorf("AliyunTTS: websocket connect failed after %v: %v", time.Since(connectStart), err)
		return nil, err
	}
	logging.Infof("AliyunTTS: websocket connected in %v", time.Since(connectStart))

	audioBuf := newBufferedPipe(1024 * 1024)

	stream := &dashScopeStream{
		createdAt: streamStart,
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

	runTaskStart := time.Now()
	if err := stream.sendRunTask(ctx); err != nil {
		_ = conn.Close()
		_ = audioBuf.Close()
		logging.Errorf("AliyunTTS: send run-task failed after %v: %v", time.Since(runTaskStart), err)
		return nil, err
	}
	logging.Infof("AliyunTTS: run-task sent in %v", time.Since(runTaskStart))

	waitStartedStart := time.Now()
	if err := stream.waitStarted(ctx); err != nil {
		_ = conn.Close()
		_ = audioBuf.Close()
		logging.Errorf("AliyunTTS: wait task-started failed after %v: %v", time.Since(waitStartedStart), err)
		return nil, err
	}
	logging.Infof("AliyunTTS: task-started wait completed in %v (stream_ready=%v)",
		time.Since(waitStartedStart), time.Since(streamStart))

	return stream, nil
}

type dashScopeStream struct {
	createdAt time.Time

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

	metricsMu      sync.Mutex
	taskStartedAt  time.Time
	taskFinishedAt time.Time
	firstAudioAt   time.Time
	audioBytes     int
	audioFrames    int
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

// closeStream 发送 finish-task 并等待所有音频接收完毕。conn 由 markDone 统一关闭。
func (s *dashScopeStream) closeStream(ctx context.Context) error {
	finishStart := time.Now()
	if err := s.Finish(ctx); err != nil {
		logging.Errorf("AliyunTTS: finish-task failed after %v: %v", time.Since(finishStart), err)
		return err
	}
	logging.Infof("AliyunTTS: finish-task sent in %v", time.Since(finishStart))

	waitDoneStart := time.Now()
	select {
	case <-s.doneCh:
		err := s.streamErr()
		audioBytes, audioFrames, firstAudioAt, taskStartedAt, taskFinishedAt := s.metricsSnapshot()
		if err != nil {
			logging.Errorf("AliyunTTS: task done with error after %v: %v", time.Since(waitDoneStart), err)
			return err
		}
		logging.Infof("AliyunTTS: task done wait completed in %v (audio_bytes=%d, audio_frames=%d, first_audio_latency=%s, synthesis_window=%s, total_since_stream=%v)",
			time.Since(waitDoneStart),
			audioBytes,
			audioFrames,
			formatDuration(s.createdAt, firstAudioAt),
			formatDuration(taskStartedAt, taskFinishedAt),
			time.Since(s.createdAt),
		)
		return nil
	case err := <-s.errCh:
		logging.Errorf("AliyunTTS: wait task done failed after %v: %v", time.Since(waitDoneStart), err)
		return err
	case <-ctx.Done():
		s.closeWithError(ctx.Err())
		logging.Errorf("AliyunTTS: wait task done canceled after %v: %v", time.Since(waitDoneStart), ctx.Err())
		return ctx.Err()
	}
}

// WriteTextChunk 实现 tts.SynthesisStream，委托内部小写方法。
func (s *dashScopeStream) WriteTextChunk(ctx context.Context, text string) error {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	waitStart := time.Now()
	if err := s.waitStarted(ctx); err != nil {
		return err
	}
	waitDuration := time.Since(waitStart)
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	sendStart := time.Now()
	if err := s.sendContinueTask(ctx, text); err != nil {
		logging.Errorf("AliyunTTS: send continue-task failed after %v: %v", time.Since(sendStart), err)
		return err
	}
	logging.Infof("AliyunTTS: continue-task sent in %v (wait_started=%v, text_len=%d)",
		time.Since(sendStart), waitDuration, len([]rune(text)))
	return nil
}

// Finish 发送 finish-task，立即返回，不等 task-finished。
// receiver goroutine 在 task-finished 后通过 markDone 关闭 audioBuf 和 conn。
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

// AudioReader 返回流式音频 reader，可在 Finish 前开始读；task-finished 后 EOF。
func (s *dashScopeStream) AudioReader() io.ReadCloser {
	return s.audioBuf
}

// Abort 立即中止 stream，用于打断场景。
func (s *dashScopeStream) Abort() {
	s.setErr(context.Canceled)
	s.markDone()
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
		params["instruction"] = fmt.Sprintf("请用情绪%s说话", mapped)
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
		// normal event, no action needed
	}
	return false
}

func (s *dashScopeStream) recordTaskStarted() {
	now := time.Now()
	s.metricsMu.Lock()
	s.taskStartedAt = now
	s.metricsMu.Unlock()
	logging.Infof("AliyunTTS: task-started event in %v", now.Sub(s.createdAt))
}

func (s *dashScopeStream) recordTaskFinished() {
	now := time.Now()
	s.metricsMu.Lock()
	s.taskFinishedAt = now
	audioBytes := s.audioBytes
	audioFrames := s.audioFrames
	firstAudioAt := s.firstAudioAt
	taskStartedAt := s.taskStartedAt
	s.metricsMu.Unlock()
	logging.Infof("AliyunTTS: task-finished event in %v (audio_bytes=%d, audio_frames=%d, first_audio_latency=%s, synthesis_window=%s)",
		now.Sub(s.createdAt),
		audioBytes,
		audioFrames,
		formatDuration(s.createdAt, firstAudioAt),
		formatDuration(taskStartedAt, now),
	)
}

func (s *dashScopeStream) recordAudioFrame(n int) {
	now := time.Now()
	firstFrame := false

	s.metricsMu.Lock()
	if s.firstAudioAt.IsZero() {
		s.firstAudioAt = now
		firstFrame = true
	}
	s.audioBytes += n
	s.audioFrames++
	totalBytes := s.audioBytes
	totalFrames := s.audioFrames
	s.metricsMu.Unlock()

	if firstFrame {
		logging.Infof("AliyunTTS: first audio frame received in %v (frame_bytes=%d)", now.Sub(s.createdAt), n)
		return
	}
	logging.Debugf("AliyunTTS: audio frame received (frame_bytes=%d, total_bytes=%d, total_frames=%d)", n, totalBytes, totalFrames)
}

func (s *dashScopeStream) metricsSnapshot() (audioBytes int, audioFrames int, firstAudioAt time.Time, taskStartedAt time.Time, taskFinishedAt time.Time) {
	s.metricsMu.Lock()
	defer s.metricsMu.Unlock()
	return s.audioBytes, s.audioFrames, s.firstAudioAt, s.taskStartedAt, s.taskFinishedAt
}

func formatDuration(from, to time.Time) string {
	if from.IsZero() || to.IsZero() {
		return "n/a"
	}
	return to.Sub(from).String()
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

// mapEmotion 将系统 emotion 值（emoji 或标签名）映射到指定 voice 支持的 emotion 参数。
// 返回空字符串表示不传 emotion 参数。
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
	if cfg.Format == "" {
		cfg.Format = "pcm"
	}
	if cfg.SampleRate == 0 {
		cfg.SampleRate = 16000
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
