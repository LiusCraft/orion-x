package audio

import (
	"context"
	"io"
	"sync"

	"github.com/liuscraft/orion-x/internal/provider/tts"
)

// mockTTSProvider 模拟 TTS Provider
type mockTTSProvider struct {
	mu         sync.Mutex
	startCount int
	startErr   error
	streams    []*mockTTSStream
	lastConfig tts.Config
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

func (s *mockTTSStream) Finish(ctx context.Context) error {
	return s.Close(ctx)
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
