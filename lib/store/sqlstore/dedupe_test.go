package sqlstore

import (
	"bytes"
	"testing"

	"go.mau.fi/whatsmeow/store"
)

func TestDedupeMutationMACs(t *testing.T) {
	t.Run("empty mutations", func(t *testing.T) {
		out := dedupeMutationMACs(nil)
		if len(out) != 0 {
			t.Fatalf("expected 0 elements, got %d", len(out))
		}
	})

	t.Run("distinct 32-byte index macs", func(t *testing.T) {
		mac1 := store.AppStateMutationMAC{IndexMAC: bytes.Repeat([]byte{1}, 32), ValueMAC: []byte("val1")}
		mac2 := store.AppStateMutationMAC{IndexMAC: bytes.Repeat([]byte{2}, 32), ValueMAC: []byte("val2")}
		input := []store.AppStateMutationMAC{mac1, mac2}

		out := dedupeMutationMACs(input)
		if len(out) != 2 {
			t.Fatalf("expected 2 elements, got %d", len(out))
		}
		if !bytes.Equal(out[0].IndexMAC, mac1.IndexMAC) || !bytes.Equal(out[0].ValueMAC, mac1.ValueMAC) {
			t.Errorf("element 0 mismatch: got %+v", out[0])
		}
		if !bytes.Equal(out[1].IndexMAC, mac2.IndexMAC) || !bytes.Equal(out[1].ValueMAC, mac2.ValueMAC) {
			t.Errorf("element 1 mismatch: got %+v", out[1])
		}
	})

	t.Run("duplicate index macs keeps latest value", func(t *testing.T) {
		idxA := bytes.Repeat([]byte{0x0A}, 32)
		idxB := bytes.Repeat([]byte{0x0B}, 32)

		input := []store.AppStateMutationMAC{
			{IndexMAC: idxA, ValueMAC: []byte("v1_A")},
			{IndexMAC: idxB, ValueMAC: []byte("v1_B")},
			{IndexMAC: idxA, ValueMAC: []byte("v2_A")},
		}

		out := dedupeMutationMACs(input)
		if len(out) != 2 {
			t.Fatalf("expected 2 elements, got %d", len(out))
		}
		if !bytes.Equal(out[0].IndexMAC, idxA) || string(out[0].ValueMAC) != "v2_A" {
			t.Errorf("expected idxA with v2_A at index 0, got %+v", out[0])
		}
		if !bytes.Equal(out[1].IndexMAC, idxB) || string(out[1].ValueMAC) != "v1_B" {
			t.Errorf("expected idxB with v1_B at index 1, got %+v", out[1])
		}
	})

	t.Run("invalid length index macs are untouched", func(t *testing.T) {
		invalidMAC := store.AppStateMutationMAC{IndexMAC: []byte("short"), ValueMAC: []byte("v_short")}
		validMAC := store.AppStateMutationMAC{IndexMAC: bytes.Repeat([]byte{3}, 32), ValueMAC: []byte("v_valid")}

		input := []store.AppStateMutationMAC{invalidMAC, validMAC, invalidMAC}
		out := dedupeMutationMACs(input)
		if len(out) != 3 {
			t.Fatalf("expected 3 elements (invalid macs not deduped), got %d", len(out))
		}
	})
}
