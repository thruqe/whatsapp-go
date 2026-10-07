//go:build !amd64 && !arm64

package mlow

func fftButterfly2(out, sub0, sub1, tw []cpx, m int) {
	fftButterfly2_generic(out, sub0, sub1, tw, m)
}
