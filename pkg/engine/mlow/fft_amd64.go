//go:build amd64

package mlow

//go:noescape
func fftButterfly2_amd64(out, sub0, sub1, tw []cpx, m int)

func fftButterfly2(out, sub0, sub1, tw []cpx, m int) {
	fftButterfly2_amd64(out, sub0, sub1, tw, m)
}
