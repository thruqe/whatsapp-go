package mlow

import (
	"math"
	"sync"
)

// cpx is a single-precision complex value.
//
// Source of truth: https://github.com/oxidezap/whatsapp-rust/blob/674e85164b35ca19115dfebcf605708d15951ee7/wacore/src/voip/mlow/smpl_perc.rs#L318-L343
type cpx struct {
	re, im float32
}

func (a cpx) add(b cpx) cpx {
	return cpx{re: a.re + b.re, im: a.im + b.im}
}

func (a cpx) mul(b cpx) cpx {
	return cpx{
		re: a.re*b.re - a.im*b.im,
		im: a.re*b.im + a.im*b.re,
	}
}

// smallestFactor returns the smallest prime factor of n (>= 2).
func smallestFactor(n int) int {
	// Source of truth: https://github.com/oxidezap/whatsapp-rust/blob/674e85164b35ca19115dfebcf605708d15951ee7/wacore/src/voip/mlow/smpl_perc.rs#L346-L358
	if n%2 == 0 {
		return 2
	}
	p := 3
	for p*p <= n {
		if n%p == 0 {
			return p
		}
		p += 2
	}
	return n
}

func fftButterfly2_generic(out, sub0, sub1, tw []cpx, m int) {
	for k := range m {
		tRe := sub1[k].re*tw[k].re - sub1[k].im*tw[k].im
		tIm := sub1[k].re*tw[k].im + sub1[k].im*tw[k].re
		out[k] = cpx{re: sub0[k].re + tRe, im: sub0[k].im + tIm}
		out[m+k] = cpx{re: sub0[k].re - tRe, im: sub0[k].im - tIm}
	}
}

var (
	twiddleTableFwd [577][]cpx
	twiddleTableBwd [577][]cpx
	tw9TableFwd     [9][3]cpx
	tw9TableBwd     [9][3]cpx

	w3_1_fwd = cpx{re: float32(math.Cos(float64(-1.0 * 2.0 * smplPI * 1.0 / 3.0))), im: float32(math.Sin(float64(-1.0 * 2.0 * smplPI * 1.0 / 3.0)))}
	w3_2_fwd = cpx{re: float32(math.Cos(float64(-1.0 * 2.0 * smplPI * 2.0 / 3.0))), im: float32(math.Sin(float64(-1.0 * 2.0 * smplPI * 2.0 / 3.0)))}
	w3_1_bwd = cpx{re: float32(math.Cos(float64(1.0 * 2.0 * smplPI * 1.0 / 3.0))), im: float32(math.Sin(float64(1.0 * 2.0 * smplPI * 1.0 / 3.0)))}
	w3_2_bwd = cpx{re: float32(math.Cos(float64(1.0 * 2.0 * smplPI * 2.0 / 3.0))), im: float32(math.Sin(float64(1.0 * 2.0 * smplPI * 2.0 / 3.0)))}
)

func init() {
	for _, n := range []int{2, 4, 8, 16, 18, 32, 36, 64, 72, 128, 144, 256, 288, 512, 576} {
		m := n / 2
		twFwd := make([]cpx, m)
		twBwd := make([]cpx, m)
		for k := range m {
			angFwd := -1.0 * 2.0 * smplPI * float32(k) / float32(n)
			twFwd[k] = cpx{re: float32(math.Cos(float64(angFwd))), im: float32(math.Sin(float64(angFwd)))}
			angBwd := 1.0 * 2.0 * smplPI * float32(k) / float32(n)
			twBwd[k] = cpx{re: float32(math.Cos(float64(angBwd))), im: float32(math.Sin(float64(angBwd)))}
		}
		twiddleTableFwd[n] = twFwd
		twiddleTableBwd[n] = twBwd
	}

	for k := range 9 {
		for q := range 3 {
			angFwd := -1.0 * 2.0 * smplPI * float32(k) * float32(q) / 9.0
			tw9TableFwd[k][q] = cpx{re: float32(math.Cos(float64(angFwd))), im: float32(math.Sin(float64(angFwd)))}
			angBwd := 1.0 * 2.0 * smplPI * float32(k) * float32(q) / 9.0
			tw9TableBwd[k][q] = cpx{re: float32(math.Cos(float64(angBwd))), im: float32(math.Sin(float64(angBwd)))}
		}
	}
}

func getTwiddles(n int, sign float32) []cpx {
	if n < len(twiddleTableFwd) {
		if sign < 0 {
			if tw := twiddleTableFwd[n]; tw != nil {
				return tw
			}
		} else {
			if tw := twiddleTableBwd[n]; tw != nil {
				return tw
			}
		}
	}
	m := n / 2
	tw := make([]cpx, m)
	for k := range m {
		ang := sign * 2.0 * smplPI * float32(k) / float32(n)
		tw[k] = cpx{re: float32(math.Cos(float64(ang))), im: float32(math.Sin(float64(ang)))}
	}
	return tw
}

var fftScratchPool = sync.Pool{
	New: func() any {
		b := make([]cpx, 2048)
		return &b
	},
}

func fftRecFast(x []cpx, stride, n int, sign float32, out []cpx, scratch []cpx) {
	if n == 1 {
		out[0] = x[0]
		return
	}
	if n == 2 {
		x0 := x[0]
		x1 := x[stride]
		out[0] = cpx{re: x0.re + x1.re, im: x0.im + x1.im}
		out[1] = cpx{re: x0.re - x1.re, im: x0.im - x1.im}
		return
	}
	if n == 3 {
		x0 := x[0]
		x1 := x[stride]
		x2 := x[2*stride]
		w1, w2 := w3_1_fwd, w3_2_fwd
		if sign > 0 {
			w1, w2 = w3_1_bwd, w3_2_bwd
		}
		t1 := x1.mul(w1)
		t2 := x2.mul(w2)
		out[0] = cpx{re: x0.re + x1.re + x2.re, im: x0.im + x1.im + x2.im}
		out[1] = cpx{re: x0.re + t1.re + t2.re, im: x0.im + t1.im + t2.im}
		t1b := x1.mul(w2)
		t2b := x2.mul(w1)
		out[2] = cpx{re: x0.re + t1b.re + t2b.re, im: x0.im + t1b.im + t2b.im}
		return
	}
	p := smallestFactor(n)
	if p == 2 {
		m := n / 2
		sub0 := scratch[0:m]
		sub1 := scratch[m:n]
		nextScratch := scratch[n:]

		fftRecFast(x[0:], stride*2, m, sign, sub0, nextScratch)
		fftRecFast(x[stride:], stride*2, m, sign, sub1, nextScratch)

		tw := getTwiddles(n, sign)
		fftButterfly2(out, sub0, sub1, tw, m)
		return
	}
	if n == 9 {
		m := 3
		sub := scratch[0:9]
		nextScratch := scratch[9:]
		fftRecFast(x[0:], stride*3, m, sign, sub[0:3], nextScratch)
		fftRecFast(x[stride:], stride*3, m, sign, sub[3:6], nextScratch)
		fftRecFast(x[2*stride:], stride*3, m, sign, sub[6:9], nextScratch)

		twTable := &tw9TableFwd
		if sign > 0 {
			twTable = &tw9TableBwd
		}
		for k := range 9 {
			kmod := k % 3
			s0 := sub[kmod]
			s1 := sub[3+kmod].mul(twTable[k][1])
			s2 := sub[6+kmod].mul(twTable[k][2])
			out[k] = cpx{re: s0.re + s1.re + s2.re, im: s0.im + s1.im + s2.im}
		}
		return
	}
	if p == n {
		for k := range n {
			var acc cpx
			angK := sign * 2.0 * smplPI * float32(k) / float32(n)
			for j := range n {
				ang := angK * float32(j)
				w := cpx{re: float32(math.Cos(float64(ang))), im: float32(math.Sin(float64(ang)))}
				acc = acc.add(x[j*stride].mul(w))
			}
			out[k] = acc
		}
		return
	}
	m := n / p
	sub := scratch[0:n]
	nextScratch := scratch[n:]
	for q := range p {
		fftRecFast(x[q*stride:], stride*p, m, sign, sub[q*m:(q+1)*m], nextScratch)
	}
	for k := range n {
		kmod := k % m
		var acc cpx
		for q := range p {
			ang := sign * 2.0 * smplPI * float32(k) * float32(q) / float32(n)
			tw := cpx{re: float32(math.Cos(float64(ang))), im: float32(math.Sin(float64(ang)))}
			acc = acc.add(sub[q*m+kmod].mul(tw))
		}
		out[k] = acc
	}
}

// cfft computes the complex FFT of a mixed-radix length into out. sign=-1 forward,
// +1 inverse.
func cfft(input, out []cpx, sign float32) {
	n := len(input)
	if n <= 1 {
		if n == 1 {
			out[0] = input[0]
		}
		return
	}
	scratchPtr := fftScratchPool.Get().(*[]cpx)
	scratch := *scratchPtr
	if len(scratch) < 2*n {
		scratch = make([]cpx, 2*n)
	}
	fftRecFast(input, 1, n, sign, out, scratch)
	fftScratchPool.Put(scratchPtr)
}

type rfftPair struct {
	cin  []cpx
	spec []cpx
}

var rfftPairPool = sync.Pool{
	New: func() any {
		return &rfftPair{
			cin:  make([]cpx, 1024),
			spec: make([]cpx, 1024),
		}
	},
}

// rfftForwardOrdered is the forward real FFT of n real samples, re-packed into the
// ordered REAL layout: f[0]=DC.re, f[1]=Nyquist.re, then [re,im] pairs for bins
// 1..n/2-1. Output length is n.
func rfftForwardOrdered(time, f []float32) {
	n := len(time)
	pair := rfftPairPool.Get().(*rfftPair)
	if len(pair.cin) < n {
		pair.cin = make([]cpx, n)
		pair.spec = make([]cpx, n)
	}
	cin := pair.cin[:n]
	spec := pair.spec[:n]

	for i := range n {
		cin[i] = cpx{re: time[i], im: 0}
	}
	cfft(cin, spec, -1.0)
	f[0] = spec[0].re
	f[1] = spec[n/2].re
	for i := 1; i < n/2; i++ {
		f[2*i] = spec[i].re
		f[2*i+1] = spec[i].im
	}
	rfftPairPool.Put(pair)
}
