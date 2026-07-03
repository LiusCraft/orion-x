package codec

import (
	"fmt"

	"gopkg.in/hraban/opus.v2"
)

// defaultOpusFrameDurationMs is used when the caller passes a non-positive
// frameDurationMs. 60ms is a common choice for voice: it keeps packet
// overhead low while staying within Opus's supported frame durations.
const defaultOpusFrameDurationMs = 60

// opusEncodeBufSize is the scratch buffer size passed to the underlying
// encoder. Opus voice bitrates are far below what's needed to fill this at
// 60ms frames; it's sized generously rather than tightly to the bitrate.
const opusEncodeBufSize = 4000

// opusCodec encodes PCM16LE mono samples to Opus, or decodes Opus packets
// back to PCM16LE mono. Opus only supports a fixed set of sample rates
// (8000/12000/16000/24000/48000 Hz); other rates are rejected at
// construction time rather than failing later at Encode/Decode time.
type opusCodec struct {
	channels      int
	frameDuration int // ms, needed by Decode's output buffer sizing
	frameSize     int // interleaved samples per frame = rate * frameDurationMs / 1000 * channels

	enc *opus.Encoder
	dec *opus.Decoder

	// buf holds samples carried over between Encode calls that don't yet
	// fill a complete frame.
	buf []int16
}

// validOpusFrameDurationsMs are the frame durations this codec accepts.
// Opus itself also supports 2.5/5ms, but those are impractical for voice
// (packet header overhead dominates at that size) and don't fit cleanly in
// an integer-millisecond protocol field, so they're intentionally excluded.
var validOpusFrameDurationsMs = map[int]bool{10: true, 20: true, 40: true, 60: true}

func newOpusCodec(sampleRate, channels, frameDurationMs int) (*opusCodec, error) {
	if !isValidOpusSampleRate(sampleRate) {
		return nil, fmt.Errorf("codec: invalid opus sample rate %d (must be one of 8000/12000/16000/24000/48000)", sampleRate)
	}
	if channels != 1 && channels != 2 {
		return nil, fmt.Errorf("codec: invalid opus channels %d (must be 1 or 2)", channels)
	}
	if frameDurationMs <= 0 {
		frameDurationMs = defaultOpusFrameDurationMs
	} else if !validOpusFrameDurationsMs[frameDurationMs] {
		return nil, fmt.Errorf("codec: invalid opus frame duration %dms (must be one of 10/20/40/60)", frameDurationMs)
	}

	enc, err := opus.NewEncoder(sampleRate, channels, opus.AppVoIP)
	if err != nil {
		return nil, fmt.Errorf("codec: create opus encoder: %w", err)
	}
	dec, err := opus.NewDecoder(sampleRate, channels)
	if err != nil {
		return nil, fmt.Errorf("codec: create opus decoder: %w", err)
	}

	return &opusCodec{
		channels:      channels,
		frameDuration: frameDurationMs,
		frameSize:     sampleRate * frameDurationMs / 1000 * channels,
		enc:           enc,
		dec:           dec,
	}, nil
}

func isValidOpusSampleRate(rate int) bool {
	switch rate {
	case 8000, 12000, 16000, 24000, 48000:
		return true
	}
	return false
}

func (c *opusCodec) Encode(pcm []int16) ([][]byte, error) {
	c.buf = append(c.buf, pcm...)

	var frames [][]byte
	for len(c.buf) >= c.frameSize {
		frame, err := c.encodeFrame(c.buf[:c.frameSize])
		if err != nil {
			return frames, err
		}
		frames = append(frames, frame)
		c.buf = c.buf[c.frameSize:]
	}
	c.compactBuf()
	return frames, nil
}

func (c *opusCodec) Flush() ([][]byte, error) {
	if len(c.buf) == 0 {
		return nil, nil
	}
	padded := make([]int16, c.frameSize)
	copy(padded, c.buf)
	c.buf = nil

	frame, err := c.encodeFrame(padded)
	if err != nil {
		return nil, err
	}
	return [][]byte{frame}, nil
}

// compactBuf copies the remaining carry-over samples into a freshly sized
// slice so repeated Encode calls don't keep growing the backing array of a
// slice that's mostly re-sliced away.
func (c *opusCodec) compactBuf() {
	if len(c.buf) == 0 {
		c.buf = nil
		return
	}
	remaining := make([]int16, len(c.buf))
	copy(remaining, c.buf)
	c.buf = remaining
}

func (c *opusCodec) encodeFrame(pcm []int16) ([]byte, error) {
	out := make([]byte, opusEncodeBufSize)
	n, err := c.enc.Encode(pcm, out)
	if err != nil {
		return nil, fmt.Errorf("codec: opus encode: %w", err)
	}
	return out[:n], nil
}

func (c *opusCodec) Decode(data []byte) ([]int16, error) {
	// The caller's packet may use a different frame duration than this
	// codec's own c.frameDuration (a peer is free to encode with any valid
	// Opus frame size); size the output buffer generously rather than
	// tying it to frameSize.
	const maxFrameDurationMs = 120
	pcm := make([]int16, maxFrameDurationMs*c.frameSize/c.frameDuration)
	n, err := c.dec.Decode(data, pcm)
	if err != nil {
		return nil, fmt.Errorf("codec: opus decode: %w", err)
	}
	return pcm[:n*c.channels], nil
}
