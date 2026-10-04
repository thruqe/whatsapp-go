package media

// sumAbsPCM16Generic computes the sum of absolute values of 16-bit signed PCM samples in pure Go.
func sumAbsPCM16Generic(pcm []byte) uint64 {
	numSamples := len(pcm) / 2
	var total uint64
	for i := range numSamples {
		val := int16(uint16(pcm[i*2]) | uint16(pcm[i*2+1])<<8)
		if val < 0 {
			total += uint64(-int32(val))
		} else {
			total += uint64(val)
		}
	}
	return total
}
