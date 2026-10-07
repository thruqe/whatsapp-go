package binary_test

import (
	"testing"

	"go.mau.fi/whatsmeow/binary"
)

func FuzzUnmarshal(f *testing.F) {
	for _, seed := range [][]byte{
		{248, 1, 0},
		{248, 1, 255, 128},
		{248, 2, 252, 1, 97, 250, 248, 0, 252, 1, 115},
		{248, 2, 252, 1, 97, 247, 1, 1, 248, 0},
		{248, 2, 252, 1, 97, 246, 248, 0, 0, 0, 252, 4, 109, 115, 103, 114},
		{0},
		{},
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = binary.Unmarshal(data)
	})
}

func FuzzUnpack(f *testing.F) {
	for _, seed := range [][]byte{{}, {0}, {1}, {2, 1, 2, 3}, {3, 4, 5}} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = binary.Unpack(data)
	})
}
