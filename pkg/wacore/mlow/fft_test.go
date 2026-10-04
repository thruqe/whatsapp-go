package mlow

import (
	"math"
	"testing"
)

func TestDirectAsmButterfly(t *testing.T) {
	for _, m := range []int{1, 2, 3, 4, 5, 8, 16, 32, 64, 128, 256, 288} {
		sub0 := make([]cpx, m)
		sub1 := make([]cpx, m)
		tw := make([]cpx, m)
		for i := range m {
			sub0[i] = cpx{re: float32(i*2 + 1), im: float32(i*3 + 2)}
			sub1[i] = cpx{re: float32(i*4 - 5), im: float32(i*5 + 1)}
			ang := -2.0 * smplPI * float32(i) / float32(2*m)
			tw[i] = cpx{re: float32(math.Cos(float64(ang))), im: float32(math.Sin(float64(ang)))}
		}

		outGen := make([]cpx, 2*m)
		fftButterfly2_generic(outGen, sub0, sub1, tw, m)

		outAsm := make([]cpx, 2*m)
		fftButterfly2(outAsm, sub0, sub1, tw, m)

		for i := 0; i < 2*m; i++ {
			diffRe := math.Abs(float64(outAsm[i].re - outGen[i].re))
			diffIm := math.Abs(float64(outAsm[i].im - outGen[i].im))
			if diffRe > 1e-4 || diffIm > 1e-4 {
				t.Fatalf("m=%d idx=%d: asm=(%v,%v) gen=(%v,%v)",
					m, i, outAsm[i].re, outAsm[i].im, outGen[i].re, outGen[i].im)
			}
		}
	}
}

func BenchmarkRFFTForward576(b *testing.B) {
	timeBuf := make([]float32, 576)
	for i := range timeBuf {
		timeBuf[i] = float32(i % 100)
	}
	f := make([]float32, 576)
	b.ReportAllocs()
	for b.Loop() {
		rfftForwardOrdered(timeBuf, f)
	}
}

func BenchmarkRFFTForward512(b *testing.B) {
	timeBuf := make([]float32, 512)
	for i := range timeBuf {
		timeBuf[i] = float32(i % 100)
	}
	f := make([]float32, 512)
	b.ReportAllocs()
	for b.Loop() {
		rfftForwardOrdered(timeBuf, f)
	}
}

func BenchmarkButterflyGeneric_288(b *testing.B) {
	m := 288
	sub0 := make([]cpx, m)
	sub1 := make([]cpx, m)
	tw := make([]cpx, m)
	out := make([]cpx, 2*m)
	b.ReportAllocs()
	for b.Loop() {
		fftButterfly2_generic(out, sub0, sub1, tw, m)
	}
}

func BenchmarkButterflyAsm_288(b *testing.B) {
	m := 288
	sub0 := make([]cpx, m)
	sub1 := make([]cpx, m)
	tw := make([]cpx, m)
	out := make([]cpx, 2*m)
	b.ReportAllocs()
	for b.Loop() {
		fftButterfly2(out, sub0, sub1, tw, m)
	}
}
