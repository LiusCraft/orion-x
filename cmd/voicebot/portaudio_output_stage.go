package main

import (
	"context"
	"encoding/binary"
	"os"

	"github.com/liuscraft/orion-x/internal/audio"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/pipeline"
)

// PortAudioOutputStage consumes audio.TTSChunk messages produced by the TTS
// stage and writes PCM samples to a PortAudioSink. It is a sink stage: it
// never produces further pipeline output.
type PortAudioOutputStage struct {
	*pipeline.BaseStage
	sink       *PortAudioSink
	sampleRate int
}

// NewPortAudioOutputStage creates a PortAudioOutputStage. sink must already
// be started (sink.Start) before the pipeline starts.
func NewPortAudioOutputStage(sink *PortAudioSink, sampleRate int) pipeline.Stage {
	return &PortAudioOutputStage{
		BaseStage:  pipeline.NewBaseStage("portaudio_output"),
		sink:       sink,
		sampleRate: sampleRate,
	}
}

func (s *PortAudioOutputStage) Process(ctx context.Context, input <-chan pipeline.Message) <-chan pipeline.Message {
	output := make(chan pipeline.Message)

	// Debug: VOICEBOT_DUMP_WAV=1 collects all TTS PCM into one buffer and
	// writes a single complete WAV on exit.
	var dumpPCM []byte

	go func() {
		defer close(output)
		defer func() {
			if len(dumpPCM) > 0 {
				writeWAV("/tmp/voicebot_tts_debug.wav", dumpPCM, s.sampleRate)
			}
		}()

		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-input:
				if !ok {
					return
				}
				chunk, ok := msg.Payload.(audio.TTSChunk)
				if !ok || chunk.Final {
					continue
				}
				if os.Getenv("VOICEBOT_DUMP_WAV") == "1" {
					dumpPCM = append(dumpPCM, chunk.Audio...)
				}
				samples := audio.BytesToInt16LE(chunk.Audio)
				if err := s.sink.WritePCM(samples); err != nil {
					logging.Errorf("PortAudioOutputStage: write error: %v", err)
				}
			}
		}
	}()

	return output
}

// writeWAV writes a complete WAV file in one shot: header + all PCM data.
func writeWAV(path string, pcm []byte, sampleRate int) {
	f, err := os.Create(path)
	if err != nil {
		logging.Warnf("PortAudioOutputStage: cannot create wav: %v", err)
		return
	}
	defer func() { _ = f.Close() }()

	var buf [4]byte
	putU32 := func(v uint32) { binary.LittleEndian.PutUint32(buf[:], v); _, _ = f.Write(buf[:]) }
	putU16 := func(v uint16) { binary.LittleEndian.PutUint16(buf[:2], v); _, _ = f.Write(buf[:2]) }

	dataLen := uint32(len(pcm))

	_, _ = f.Write([]byte("RIFF"))
	putU32(36 + dataLen)
	_, _ = f.Write([]byte("WAVE"))
	_, _ = f.Write([]byte("fmt "))
	putU32(16)
	putU16(1)  // PCM
	putU16(1)  // mono
	putU32(uint32(sampleRate))
	putU32(uint32(sampleRate * 2)) // byte rate
	putU16(2)  // block align
	putU16(16) // bits per sample
	_, _ = f.Write([]byte("data"))
	putU32(dataLen)
	_, _ = f.Write(pcm)

	logging.Infof("PortAudioOutputStage: WAV saved → %s (PCM %d bytes, %d Hz)", path, dataLen, sampleRate)
}
