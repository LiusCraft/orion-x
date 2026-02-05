package audio

import "context"

// AudioFormat describes the PCM format written to AudioSink.
type AudioFormat struct {
	SampleRate      int
	Channels        int
	FramesPerBuffer int
}

// AudioSink is the audio output target abstraction.
// It receives interleaved PCM16 samples.
type AudioSink interface {
	Start(ctx context.Context, format AudioFormat) error
	WritePCM(samples []int16) error
	Stop() error
}
