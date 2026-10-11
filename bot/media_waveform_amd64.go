//go:build amd64

package media

//go:noescape
func sumAbsPCM16(pcm []byte) uint64
