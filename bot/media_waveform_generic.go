//go:build !amd64 && !arm64

package main

func sumAbsPCM16(pcm []byte) uint64 {
	return SumAbsPCM16Generic(pcm)
}
