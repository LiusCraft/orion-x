package audio

import (
	"context"
	"io"
	"sync"

	"github.com/liuscraft/orion-x/internal/tts"
)

// mockTTSProvider 模拟 TTS Provider
type mockTTSProvider struct {
	mu           sync.Mutex
	startCount   int
	startDelay   int // milliseconds
	startErr     error
	streams      []*mockTTSStream
	lastConfig   tts.Config
	onStartCalls []string
}

func newMockTTSProvider() *mockTTSProvider {
	return &mockTTSProvider{}
}

func (p *mockTTSProvider) Start(ctx context.Context, cfg tts.Config) (tts.Stream, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.startCount++
	p.lastConfig = cfg

	if p.startErr != nil {
		return nil, p.startErr
	}

	stream := newMockTTSStream()
	p.streams = append(p.streams, stream)
	return stream, nil
}

func (p *mockTTSProvider) getStartCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.startCount
}

func (p *mockTTSProvider) getLastConfig() tts.Config {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastConfig
}

// mockTTSStream 模拟 TTS Stream
type mockTTSStream struct {
	mu          sync.Mutex
	text        string
	closed      bool
	audioData   []byte
	reader      *mockAudioReader
	sampleRate  int
	channels    int
	writeErr    error
	closeErr    error
	writeCalled int
	closeCalled int
}

func newMockTTSStream() *mockTTSStream {
	s := &mockTTSStream{
		sampleRate: 16000,
		channels:   1,
	}
	s.reader = newMockAudioReader()
	return s
}

func (s *mockTTSStream) WriteTextChunk(ctx context.Context, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.writeCalled++
	s.text = text

	if s.writeErr != nil {
		return s.writeErr
	}

	// 生成模拟音频数据（每个字符生成 100 字节）
	s.audioData = make([]byte, len(text)*100)
	for i := range s.audioData {
		s.audioData[i] = byte(i % 256)
	}
	s.reader.setData(s.audioData)

	return nil
}

func (s *mockTTSStream) Close(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closeCalled++

	if s.closeErr != nil {
		return s.closeErr
	}

	s.closed = true
	s.reader.close()
	return nil
}

func (s *mockTTSStream) AudioReader() io.ReadCloser {
	return s.reader
}

func (s *mockTTSStream) SampleRate() int {
	return s.sampleRate
}

func (s *mockTTSStream) Channels() int {
	return s.channels
}

func (s *mockTTSStream) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// mockAudioReader 模拟音频读取器
type mockAudioReader struct {
	mu       sync.Mutex
	data     []byte
	pos      int
	closed   bool
	readCond *sync.Cond
}

func newMockAudioReader() *mockAudioReader {
	r := &mockAudioReader{}
	r.readCond = sync.NewCond(&r.mu)
	return r
}

func (r *mockAudioReader) setData(data []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data = data
	r.readCond.Broadcast()
}

func (r *mockAudioReader) Read(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 等待数据或关闭
	for r.pos >= len(r.data) && !r.closed {
		r.readCond.Wait()
	}

	if r.closed && r.pos >= len(r.data) {
		return 0, io.EOF
	}

	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

func (r *mockAudioReader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	r.readCond.Broadcast()
	return nil
}

func (r *mockAudioReader) close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	r.readCond.Broadcast()
}

// mockMixer 模拟 Mixer
type mockMixer struct {
	mu                   sync.Mutex
	ttsStream            io.Reader
	resourceStream       io.Reader
	ttsStartedCount      int
	ttsFinishedCount     int
	addTTSStreamCount    int
	removeTTSStreamCount int
	ttsVolume            float64
	resourceVolume       float64
	finishedCh           chan struct{}
	finishedOnce         sync.Once
}

func newMockMixer() *mockMixer {
	return &mockMixer{
		finishedCh: make(chan struct{}),
	}
}

func (m *mockMixer) AddTTSStream(audio io.Reader) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ttsStream = audio
	m.addTTSStreamCount++
}

func (m *mockMixer) AddResourceStream(audio io.Reader) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resourceStream = audio
}

func (m *mockMixer) RemoveTTSStream() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ttsStream = nil
	m.removeTTSStreamCount++
}

func (m *mockMixer) RemoveResourceStream() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resourceStream = nil
}

func (m *mockMixer) SetTTSVolume(volume float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ttsVolume = volume
}

func (m *mockMixer) SetResourceVolume(volume float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resourceVolume = volume
}

func (m *mockMixer) OnTTSStarted() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ttsStartedCount++
}

func (m *mockMixer) OnTTSFinished() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ttsFinishedCount++
	m.finishedOnce.Do(func() {
		if m.finishedCh != nil {
			close(m.finishedCh)
		}
	})
}

func (m *mockMixer) Start() {}

func (m *mockMixer) Stop() {}

func (m *mockMixer) getTTSStartedCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ttsStartedCount
}

func (m *mockMixer) getTTSFinishedCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ttsFinishedCount
}

func (m *mockMixer) getAddTTSStreamCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.addTTSStreamCount
}

func (m *mockMixer) getRemoveTTSStreamCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.removeTTSStreamCount
}

func (m *mockMixer) getTTSStream() io.Reader {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ttsStream
}

// mockReferenceSink 模拟 ReferenceSink
type mockReferenceSink struct {
	mu   sync.Mutex
	data []byte
}

func newMockReferenceSink() *mockReferenceSink {
	return &mockReferenceSink{}
}

func (s *mockReferenceSink) WriteReference(data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = append(s.data, data...)
}

func (s *mockReferenceSink) getData() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]byte, len(s.data))
	copy(result, s.data)
	return result
}
