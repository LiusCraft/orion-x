// Package codec provides audio codecs for encoding/decoding PCM16LE mono
// samples to/from wire formats used by WebSocket clients (PCM passthrough or
// Opus).
package codec

import "fmt"

// Codec converts between PCM16LE mono samples and a wire format.
//
// Encode consumes a continuous stream of PCM samples; codecs that require
// fixed frame sizes (e.g. Opus) buffer internally and only emit complete
// frames, so a single call may return zero, one, or multiple frames. Decode
// consumes one already-framed unit of wire data (e.g. one WebSocket binary
// message) and returns the PCM samples it contains.
type Codec interface {
	// Encode buffers pcm internally and returns zero or more complete
	// encoded frames.
	Encode(pcm []int16) ([][]byte, error)
	// Decode converts one encoded frame into PCM samples.
	Decode(data []byte) ([]int16, error)
	// Flush encodes any buffered samples that don't fill a complete frame
	// (zero-padding the remainder), so trailing audio isn't lost when a
	// stream ends. Returns zero or one frame.
	Flush() ([][]byte, error)
}

// Format identifies a wire audio format, as negotiated over the hello
// handshake (audio_params.format).
type Format string

const (
	FormatPCM  Format = "pcm"
	FormatOpus Format = "opus"
)

// New creates a Codec for the given format, sample rate, channel count, and
// frame duration (milliseconds). frameDurationMs is ignored by PCM (which
// has no fixed-frame concept — it's a pure passthrough) and validated
// against Opus's supported set for Opus (see internal/audio/codec/opus.go);
// a non-positive value falls back to a sensible default.
func New(format Format, sampleRate, channels, frameDurationMs int) (Codec, error) {
	switch format {
	case FormatPCM, "":
		return newPCMCodec(), nil
	case FormatOpus:
		return newOpusCodec(sampleRate, channels, frameDurationMs)
	default:
		return nil, fmt.Errorf("codec: unsupported format %q", format)
	}
}
