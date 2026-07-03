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

// Int16ToBytesLE converts int16 samples to PCM16LE bytes.
func Int16ToBytesLE(samples []int16) []byte {
	data := make([]byte, len(samples)*2)
	for i, s := range samples {
		data[i*2] = byte(s)
		data[i*2+1] = byte(s >> 8)
	}
	return data
}
