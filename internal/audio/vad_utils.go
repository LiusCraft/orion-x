package audio

// Int16PCMToFloat32 converts int16 PCM audio to float32
// Input: []byte of int16 PCM (little-endian)
// Output: []float32 normalized to [-1.0, 1.0]
func Int16PCMToFloat32(pcm []byte) []float32 {
	count := len(pcm) / 2
	floats := make([]float32, count)
	for i := 0; i < count; i++ {
		lo := pcm[i*2]
		hi := pcm[i*2+1]
		sample := int16(lo) | int16(hi)<<8
		floats[i] = float32(sample) / 32768.0
	}
	return floats
}
