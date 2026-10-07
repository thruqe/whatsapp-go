//go:build amd64

package mlow

// dotProdF32 computes the dot product of a and b up to length l using assembly: sum_{i=0..l-1} a[i] * b[i].
//
//go:noescape
func dotProdF32(a, b []float32, l int) float32
