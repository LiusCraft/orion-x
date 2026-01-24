package audio

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"
)

type mockReader struct {
	data []byte
	pos  int
}

func newMockReader(data []byte) *mockReader {
	return &mockReader{data: data}
}

func (m *mockReader) Read(p []byte) (n int, err error) {
	if m.pos >= len(m.data) {
		return 0, io.EOF
	}
	n = copy(p, m.data[m.pos:])
	m.pos += n
	return n, nil
}

func TestNewMixer(t *testing.T) {
	config := DefaultMixerConfig()
	mixer, err := NewMixer(config)
	if err != nil {
		t.Fatalf("NewMixer failed: %v", err)
	}
	if mixer == nil {
		t.Fatal("NewMixer returned nil mixer")
	}
	mixer.Stop()
}

func TestMixerVolumeControl(t *testing.T) {
	mixer, err := NewMixer(DefaultMixerConfig())
	if err != nil {
		t.Fatalf("NewMixer failed: %v", err)
	}
	defer mixer.Stop()

	testCases := []struct {
		name   string
		volume float64
	}{
		{"zero volume", 0.0},
		{"half volume", 0.5},
		{"full volume", 1.0},
		{"double volume", 2.0},
	}

	for _, tc := range testCases {
		t.Run(tc.name+" TTS", func(t *testing.T) {
			mixer.SetTTSVolume(tc.volume)
			mixer.SetTTSVolume(1.0)
		})
	}

	for _, tc := range testCases {
		t.Run(tc.name+" Resource", func(t *testing.T) {
			mixer.SetResourceVolume(tc.volume)
			mixer.SetResourceVolume(1.0)
		})
	}
}

func TestMixerTTSStartedFinished(t *testing.T) {
	mixer, err := NewMixer(DefaultMixerConfig())
	if err != nil {
		t.Fatalf("NewMixer failed: %v", err)
	}
	defer mixer.Stop()

	mixer.OnTTSStarted()
	mixer.OnTTSFinished()
}

func TestMixerStreamManagement(t *testing.T) {
	mixer, err := NewMixer(DefaultMixerConfig())
	if err != nil {
		t.Fatalf("NewMixer failed: %v", err)
	}
	defer mixer.Stop()

	audioData := make([]byte, 1000)
	ttsReader := newMockReader(audioData)
	resourceReader := newMockReader(audioData)

	t.Run("AddTTSStream", func(t *testing.T) {
		mixer.AddTTSStream(ttsReader)
		mixer.RemoveTTSStream()
	})

	t.Run("AddResourceStream", func(t *testing.T) {
		mixer.AddResourceStream(resourceReader)
		mixer.RemoveResourceStream()
	})

	t.Run("BothStreams", func(t *testing.T) {
		mixer.AddTTSStream(ttsReader)
		mixer.AddResourceStream(resourceReader)
		mixer.RemoveTTSStream()
		mixer.RemoveResourceStream()
	})
}

func TestMixerMixing(t *testing.T) {
	mixer, err := NewMixer(DefaultMixerConfig())
	if err != nil {
		t.Fatalf("NewMixer failed: %v", err)
	}
	defer mixer.Stop()

	audioData := make([]byte, 1024)
	ttsReader := newMockReader(audioData)
	resourceReader := newMockReader(audioData)

	mixer.AddTTSStream(ttsReader)
	mixer.AddResourceStream(resourceReader)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	go func() {
		mixer.Start()
	}()

	<-ctx.Done()
	mixer.Stop()
}

func TestMixerStartStop(t *testing.T) {
	mixer, err := NewMixer(DefaultMixerConfig())
	if err != nil {
		t.Fatalf("NewMixer failed: %v", err)
	}

	mixer.Start()
	time.Sleep(10 * time.Millisecond)
	mixer.Stop()
}

func TestMixFromStream(t *testing.T) {
	buf := make([][]float32, 2)
	buf[0] = make([]float32, 512)
	buf[1] = make([]float32, 512)

	audioData := make([]byte, 2048)
	for i := range audioData {
		audioData[i] = byte(i % 256)
	}
	reader := newMockReader(audioData)

	mixFromStream(reader, buf, 1.0)

	hasNonZero := false
	for _, v := range buf[0] {
		if v != 0 {
			hasNonZero = true
			break
		}
	}
	if !hasNonZero {
		t.Error("Expected non-zero values after mixing")
	}
}

func TestMixFromStreamWithVolume(t *testing.T) {
	buf := make([][]float32, 2)
	buf[0] = make([]float32, 512)
	buf[1] = make([]float32, 512)

	audioData := make([]byte, 2048)
	reader := newMockReader(audioData)

	mixFromStream(reader, buf, 0.5)
	maxValue := float32(0)
	for _, v := range buf[0] {
		if v > maxValue {
			maxValue = v
		}
	}
	if maxValue > 0.6 {
		t.Errorf("Expected max value <= 0.6 with 0.5 volume, got %f", maxValue)
	}
}

func TestMixFromStreamNilReader(t *testing.T) {
	buf := make([][]float32, 2)
	buf[0] = make([]float32, 512)
	buf[1] = make([]float32, 512)

	mixFromStream(nil, buf, 1.0)

	hasNonZero := false
	for _, v := range buf[0] {
		if v != 0 {
			hasNonZero = true
			break
		}
	}
	if hasNonZero {
		t.Error("Expected all zeros with nil reader")
	}
}

func TestMixerWithMultipleVolumes(t *testing.T) {
	mixer, err := NewMixer(DefaultMixerConfig())
	if err != nil {
		t.Fatalf("NewMixer failed: %v", err)
	}
	defer mixer.Stop()

	volumes := []float64{0.0, 0.25, 0.5, 0.75, 1.0, 1.5}
	for _, vol := range volumes {
		mixer.SetTTSVolume(vol)
		mixer.SetResourceVolume(vol)
	}
}

func TestMixerStreamReplacement(t *testing.T) {
	mixer, err := NewMixer(DefaultMixerConfig())
	if err != nil {
		t.Fatalf("NewMixer failed: %v", err)
	}
	defer mixer.Stop()

	audioData1 := make([]byte, 1000)
	audioData2 := make([]byte, 1000)

	reader1 := newMockReader(audioData1)
	reader2 := newMockReader(audioData2)

	mixer.AddTTSStream(reader1)
	mixer.AddTTSStream(reader2)
	mixer.RemoveTTSStream()

	mixer.AddResourceStream(reader1)
	mixer.AddResourceStream(reader2)
	mixer.RemoveResourceStream()
}

func TestMixerEOFHandling(t *testing.T) {
	mixer, err := NewMixer(DefaultMixerConfig())
	if err != nil {
		t.Fatalf("NewMixer failed: %v", err)
	}
	defer mixer.Stop()

	emptyReader := bytes.NewReader([]byte{})

	mixer.AddTTSStream(emptyReader)
	mixer.AddResourceStream(emptyReader)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	go func() {
		mixer.Start()
	}()

	<-ctx.Done()
	mixer.Stop()
}
