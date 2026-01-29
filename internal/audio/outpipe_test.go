package audio

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/liuscraft/orion-x/internal/tts"
)

type mockTTSProvider struct {
	startCalled bool
	stream      *mockTTSStream
}

type mockTTSStream struct {
	closed bool
	reader *io.PipeReader
	writer *io.PipeWriter
}

func newMockTTSStream() *mockTTSStream {
	reader, writer := io.Pipe()
	return &mockTTSStream{
		reader: reader,
		writer: writer,
	}
}

func (m *mockTTSProvider) Start(ctx context.Context, cfg tts.Config) (tts.Stream, error) {
	m.startCalled = true
	if m.stream == nil {
		m.stream = newMockTTSStream()
	}
	return m.stream, nil
}

func (m *mockTTSStream) WriteTextChunk(ctx context.Context, text string) error {
	if m.writer == nil {
		return nil
	}
	_, _ = m.writer.Write([]byte{0x00, 0x00, 0x00, 0x00})
	return nil
}

func (m *mockTTSStream) Close(ctx context.Context) error {
	m.closed = true
	if m.writer != nil {
		_ = m.writer.Close()
	}
	return nil
}

func (m *mockTTSStream) AudioReader() io.ReadCloser {
	if m.reader == nil {
		return io.NopCloser(strings.NewReader(""))
	}
	return m.reader
}

func (m *mockTTSStream) SampleRate() int {
	return 16000
}

func (m *mockTTSStream) Channels() int {
	return 1
}

type mockMixer struct {
	ttsAdded        bool
	ttsReader       io.Reader
	resourceAdded   bool
	ttsStarted      bool
	ttsFinished     bool
	ttsVolume       float64
	resourceVolume  float64
	ttsRemoved      bool
	resourceRemoved bool
	finishedCh      chan struct{}
	finishedOnce    sync.Once
}

func (m *mockMixer) AddTTSStream(audio io.Reader) {
	m.ttsAdded = true
	m.ttsReader = audio
}

func (m *mockMixer) AddResourceStream(audio io.Reader) {
	m.resourceAdded = true
}

func (m *mockMixer) RemoveTTSStream() {
	m.ttsRemoved = true
}

func (m *mockMixer) RemoveResourceStream() {
	m.resourceRemoved = true
}

func (m *mockMixer) SetTTSVolume(volume float64) {
	m.ttsVolume = volume
}

func (m *mockMixer) SetResourceVolume(volume float64) {
	m.resourceVolume = volume
}

func (m *mockMixer) OnTTSStarted() {
	m.ttsStarted = true
}

func (m *mockMixer) OnTTSFinished() {
	m.ttsFinished = true
	m.finishedOnce.Do(func() {
		if m.finishedCh != nil {
			close(m.finishedCh)
		}
	})
}

func (m *mockMixer) Start() {}

func (m *mockMixer) Stop() {}

func TestNewOutPipe(t *testing.T) {
	pipe := NewOutPipe("test-api-key")
	if pipe == nil {
		t.Fatal("NewOutPipe returned nil")
	}
}

func TestNewOutPipeWithConfig(t *testing.T) {
	cfg := DefaultOutPipeConfig()
	cfg.TTS.APIKey = "config-key"
	cfg.VoiceMap = map[string]string{
		"default": "voice-x",
	}

	pipe := NewOutPipeWithConfig(cfg).(*outPipeImpl)
	if pipe.ttsConfig.APIKey != "config-key" {
		t.Fatalf("expected API key to be set from config")
	}
	if pipe.getVoice("unknown") != "voice-x" {
		t.Fatalf("expected custom default voice to be used")
	}
}

func TestOutPipe_GetVoice(t *testing.T) {
	tests := []struct {
		name     string
		emotion  string
		expected string
	}{
		{"happy emotion", "happy", "longanyang"},
		{"sad emotion", "sad", "zhichu"},
		{"angry emotion", "angry", "zhimeng"},
		{"calm emotion", "calm", "longxiaochun"},
		{"unknown emotion", "unknown", "longanyang"},
		{"empty emotion", "", "longanyang"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pipe := NewOutPipe("test-api-key").(*outPipeImpl)
			voice := pipe.getVoice(tt.emotion)
			if voice != tt.expected {
				t.Errorf("getVoice(%q) = %q, want %q", tt.emotion, voice, tt.expected)
			}
		})
	}
}

func TestOutPipe_SetMixer(t *testing.T) {
	pipe := NewOutPipe("test-api-key")
	mixer := &mockMixer{}
	pipe.SetMixer(mixer)
}

func TestOutPipe_StartStop(t *testing.T) {
	pipe := NewOutPipe("test-api-key")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := pipe.Start(ctx)
	if err != nil {
		t.Fatalf("Start error: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	err = pipe.Stop()
	if err != nil {
		t.Fatalf("Stop error: %v", err)
	}
}

func TestOutPipe_PlayTTS(t *testing.T) {
	t.Skip("Skipping test that requires real TTS API connection")
}

func TestOutPipe_PlayTTS_EmptyText(t *testing.T) {
	pipe := NewOutPipe("test-api-key")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := pipe.Start(ctx)
	if err != nil {
		t.Fatalf("Start error: %v", err)
	}
	defer pipe.Stop()

	err = pipe.PlayTTS("", "happy")
	if err != nil {
		t.Errorf("PlayTTS with empty text should return nil, got: %v", err)
	}
}

func TestOutPipe_PlayTTS_CleansUpOnEOF(t *testing.T) {
	pipe := NewOutPipe("test-api-key").(*outPipeImpl)
	provider := &mockTTSProvider{stream: newMockTTSStream()}
	pipe.tts = provider

	mixer := &mockMixer{finishedCh: make(chan struct{})}
	pipe.SetMixer(mixer)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := pipe.Start(ctx); err != nil {
		t.Fatalf("Start error: %v", err)
	}
	defer pipe.Stop()

	if err := pipe.PlayTTS("hello", "happy"); err != nil {
		t.Fatalf("PlayTTS error: %v", err)
	}

	if mixer.ttsReader == nil {
		t.Fatal("expected TTS reader to be set")
	}

	buf := make([]byte, 8)
	for {
		if _, err := mixer.ttsReader.Read(buf); err != nil {
			break
		}
	}

	select {
	case <-mixer.finishedCh:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for TTS cleanup")
	}

	if !mixer.ttsRemoved {
		t.Fatal("expected TTS stream to be removed")
	}
	if len(pipe.ttsStreams) != 0 {
		t.Fatalf("expected ttsStreams to be cleared, got %d", len(pipe.ttsStreams))
	}
}
