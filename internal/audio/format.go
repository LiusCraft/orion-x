package audio

const (
	InternalSampleRate = 16000
	InternalChannels   = 1
)

// BytesToInt16LE converts PCM16LE bytes to int16 samples.
func BytesToInt16LE(data []byte) []int16 {
	samples := make([]int16, len(data)/2)
	for i := range samples {
		samples[i] = int16(data[i*2]) | int16(data[i*2+1])<<8
	}
	return samples
}
