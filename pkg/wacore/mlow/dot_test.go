package mlow

import (
	"math"
	"testing"
)

func dotProdGoRef(a, b []float32, l int) float32 {
	var s float32
	for i := range l {
		s += a[i] * b[i]
	}
	return s
}

func TestDotProdF32_Correctness(t *testing.T) {
	lengths := []int{0, 1, 2, 3, 4, 5, 7, 8, 9, 15, 16, 17, 31, 32, 40, 63, 64, 80, 127, 128, 160, 320, 512}

	for _, l := range lengths {
		a := make([]float32, l)
		b := make([]float32, l)
		for i := range l {
			a[i] = float32(i)*0.25 - 5.0
			b[i] = float32(l-i)*0.125 + 1.5
		}

		expected := dotProdGoRef(a, b, l)
		actual := dotProdF32(a, b, l)

		diff := math.Abs(float64(expected - actual))
		denom := math.Max(math.Abs(float64(expected)), 1.0)
		relDiff := diff / denom
		if relDiff > 1e-4 {
			t.Errorf("mismatch at length %d: expected %f, got %f (relDiff: %e)", l, expected, actual, relDiff)
		}
	}
}

func TestDotProdF32_EdgeCases(t *testing.T) {
	// Length <= 0
	a := []float32{1.0, 2.0}
	b := []float32{3.0, 4.0}
	if got := dotProdF32(a, b, 0); got != 0 {
		t.Errorf("expected 0 for len 0, got %f", got)
	}
	if got := dotProdF32(a, b, -5); got != 0 {
		t.Errorf("expected 0 for negative len, got %f", got)
	}
}

var (
	benchVecA = make([]float32, 320)
	benchVecB = make([]float32, 320)
	benchSink float32
)

func init() {
	for i := range benchVecA {
		benchVecA[i] = float32(i) * 0.1
		benchVecB[i] = float32(i) * 0.05
	}
}

func BenchmarkDotProd_Go_40(b *testing.B) {
	var s float32
	for b.Loop() {
		s += dotProdGoRef(benchVecA, benchVecB, 40)
	}
	benchSink = s
}

func BenchmarkDotProd_Asm_40(b *testing.B) {
	var s float32
	for b.Loop() {
		s += dotProdF32(benchVecA, benchVecB, 40)
	}
	benchSink = s
}

func BenchmarkDotProd_Go_320(b *testing.B) {
	var s float32
	for b.Loop() {
		s += dotProdGoRef(benchVecA, benchVecB, 320)
	}
	benchSink = s
}

func BenchmarkDotProd_Asm_320(b *testing.B) {
	var s float32
	for b.Loop() {
		s += dotProdF32(benchVecA, benchVecB, 320)
	}
	benchSink = s
}
