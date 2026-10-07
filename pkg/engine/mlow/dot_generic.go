//go:build !amd64 && !arm64

package mlow

// dotProdF32 computes the dot product of a and b up to length l: sum_{i=0..l-1} a[i] * b[i].
func dotProdF32(a, b []float32, l int) float32 {
	var s float32
	for i := 0; i < l; i++ {
		s += a[i] * b[i]
	}
	return s
}
