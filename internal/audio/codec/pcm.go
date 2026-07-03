package codec

// pcmCodec is a passthrough codec: PCM16LE mono in, PCM16LE mono out. It has
// no framing constraints, so Encode never buffers and Flush is a no-op.
type pcmCodec struct{}

func newPCMCodec() *pcmCodec {
	return &pcmCodec{}
}

func (c *pcmCodec) Encode(pcm []int16) ([][]byte, error) {
	if len(pcm) == 0 {
		return nil, nil
	}
	return [][]byte{int16ToBytesLE(pcm)}, nil
}

func (c *pcmCodec) Decode(data []byte) ([]int16, error) {
	return bytesToInt16LE(data), nil
}

func (c *pcmCodec) Flush() ([][]byte, error) {
	return nil, nil
}

// bytesToInt16LE converts PCM16LE bytes to int16 samples.
func bytesToInt16LE(data []byte) []int16 {
	samples := make([]int16, len(data)/2)
	for i := range samples {
		samples[i] = int16(data[i*2]) | int16(data[i*2+1])<<8
	}
	return samples
}

// int16ToBytesLE converts int16 samples to PCM16LE bytes.
func int16ToBytesLE(samples []int16) []byte {
	data := make([]byte, len(samples)*2)
	for i, s := range samples {
		data[i*2] = byte(s)
		data[i*2+1] = byte(s >> 8)
	}
	return data
}
