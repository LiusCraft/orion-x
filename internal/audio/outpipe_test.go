package audio

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/liuscraft/orion-x/internal/tts"
)

type mockTTSProvider struct {
	startCalled bool
	textChunks  []string
}

type mockTTSStream struct {
	closed bool
}

func (m *mockTTSProvider) Start(ctx context.Context, cfg tts.Config) (tts.Stream, error) {
	m.startCalled = true
	return &mockTTSStream{}, nil
}

func (m *mockTTSStream) WriteTextChunk(ctx context.Context, text string) error {
	return nil
}

func (m *mockTTSStream) Close(ctx context.Context) error {
	m.closed = true
	return nil
}

func (m *mockTTSStream) AudioReader() io.ReadCloser {
	return io.NopCloser(strings.NewReader(""))
}

type mockMixer struct {
	ttsAdded        bool
	resourceAdded   bool
	ttsStarted      bool
	ttsFinished     bool
	ttsVolume       float64
	resourceVolume  float64
	ttsRemoved      bool
	resourceRemoved bool
}

func (m *mockMixer) AddTTSStream(audio io.Reader) {
	m.ttsAdded = true
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
}

func (m *mockMixer) Start() {}

func (m *mockMixer) Stop() {}

func TestNewOutPipe(t *testing.T) {
	pipe := NewOutPipe("test-api-key")
	if pipe == nil {
		t.Fatal("NewOutPipe returned nil")
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
