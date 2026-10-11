//go:build !amd64 && !arm64

package media

func sumAbsPCM16(pcm []byte) uint64 {
	return sumAbsPCM16Generic(pcm)
}
