//go:build !amd64 && !arm64

package main

func sumAbsPCM16(pcm []byte) uint64 {
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
