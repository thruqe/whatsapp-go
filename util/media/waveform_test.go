package media

import (
	"encoding/binary"
	"math/rand"
	"testing"
)

func TestSumAbsPCM16Equivalence(t *testing.T) {
	testCases := []struct {
		name string
		data []int16
	}{
		{name: "empty", data: nil},
		{name: "single_zero", data: []int16{0}},
		{name: "single_positive", data: []int16{1234}},
		{name: "single_negative", data: []int16{-1234}},
		{name: "min_int16", data: []int16{-32768}},
		{name: "max_int16", data: []int16{32767}},
		{name: "min_and_max", data: []int16{-32768, 32767, -1, 1, 0}},
		{name: "7_samples", data: []int16{10, -20, 30, -40, 50, -60, 70}},
		{name: "8_samples", data: []int16{10, -20, 30, -40, 50, -60, 70, -80}},
		{name: "15_samples", data: []int16{-1, -2, -3, -4, -5, -6, -7, -8, -9, -10, -11, -12, -13, -14, -15}},
		{name: "16_samples", data: []int16{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}},
		{name: "17_samples", data: []int16{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			buf := make([]byte, len(tc.data)*2)
			for i, v := range tc.data {
				binary.LittleEndian.PutUint16(buf[i*2:], uint16(v))
			}
			expected := sumAbsPCM16Generic(buf)
			actual := sumAbsPCM16(buf)
			if actual != expected {
				t.Fatalf("mismatch for %s: expected %d, got %d", tc.name, expected, actual)
			}
		})
	}

	// Randomized fuzz tests
	rng := rand.New(rand.NewSource(42))
	for range 500 {
		length := rng.Intn(1000)
		buf := make([]byte, length*2)
		for i := range length {
			val := int16(rng.Intn(65536) - 32768)
			binary.LittleEndian.PutUint16(buf[i*2:], uint16(val))
		}
		expected := sumAbsPCM16Generic(buf)
		actual := sumAbsPCM16(buf)
		if actual != expected {
			t.Fatalf("randomized mismatch at length %d: expected %d, got %d", length, expected, actual)
		}
	}
}

func BenchmarkSumAbsPCM16_Generic(b *testing.B) {
	// Simulate 1 second of 8000Hz PCM16 audio
	data := make([]byte, 8000*2)
	for i := range 8000 {
		binary.LittleEndian.PutUint16(data[i*2:], uint16(i%3000-1500))
	}

	b.ResetTimer()
	b.SetBytes(int64(len(data)))
	for b.Loop() {
		_ = sumAbsPCM16Generic(data)
	}
}

func BenchmarkSumAbsPCM16_Assembly(b *testing.B) {
	data := make([]byte, 8000*2)
	for i := range 8000 {
		binary.LittleEndian.PutUint16(data[i*2:], uint16(i%3000-1500))
	}

	b.ResetTimer()
	b.SetBytes(int64(len(data)))
	for b.Loop() {
		_ = sumAbsPCM16(data)
	}
}

func BenchmarkExtractWaveformAndDuration(b *testing.B) {
	// 5 seconds of 8000Hz 16-bit PCM (40,000 samples = 80,000 bytes)
	pcm := make([]byte, 40000*2)
	for i := range 40000 {
		val := int16(i%4000 - 2000)
		pcm[i*2] = byte(val & 0xFF)
		pcm[i*2+1] = byte((val >> 8) & 0xFF)
	}

	b.ResetTimer()
	b.SetBytes(int64(len(pcm)))
	for b.Loop() {
		_, _ = extractWaveformAndDuration(pcm, 8000)
	}
}
