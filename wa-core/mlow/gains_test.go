package mlow

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

// TestDecodeSmplGains is the gains KAT against gains_vectors.json: force-run on each
// active frame's post-pulse decoder state (LSF(0)->pulses(0)->gains), the decode is
// deterministic and must reproduce gain_q[] and nrg_res[]. Mirrors gains_match_go.
func TestDecodeSmplGains(t *testing.T) {
	raw, err := os.ReadFile("testdata/gains_vectors.json")
	if err != nil {
		t.Fatalf("read gains_vectors.json: %v", err)
	}
	var recs []struct {
		Frame  string  `json:"frame"`
		GainQ  []int32 `json:"gain_q"`
		NrgRes []int32 `json:"nrg_res"`
	}
	if err := json.Unmarshal(raw, &recs); err != nil {
		t.Fatalf("parse gains_vectors.json: %v", err)
	}
	if len(recs) == 0 {
		t.Fatal("no gains vectors")
	}

	tbl := LoadSmplTables()
	mem := LoadSmplMem()
	for i, rec := range recs {
		frame, err := hex.DecodeString(rec.Frame)
		if err != nil {
			t.Fatalf("rec %d: bad hex: %v", i, err)
		}
		var st SmplLsfState
		dec := NewRangeDecoder(frame[1:])
		lsf := DecodeSmplLsf(dec, tbl, &st, 0, 0)
		pulses := DecodeSmplPulses(dec, mem, 320, 4, 1, 0, lsf.Stage1)
		g := DecodeSmplGains(dec, mem, 4, pulses.Subfr)

		if !reflect.DeepEqual(g.GainQ[:], rec.GainQ) {
			t.Errorf("rec %d: gain_q got %v want %v", i, g.GainQ, rec.GainQ)
		}
		if !reflect.DeepEqual(g.NrgRes[:], rec.NrgRes) {
			t.Errorf("rec %d: nrg_res got %v want %v", i, g.NrgRes, rec.NrgRes)
		}
	}
}

func TestDecodeSmplGainsTwoSubframes(t *testing.T) {
	gain2 := ccDcmf([]byte{
		139, 194, 196, 200, 209, 225, 239, 243, 246, 246, 253, 255,
		253, 249, 244, 239, 233, 231, 230, 228, 219, 216, 214, 214,
		213, 211, 214, 215, 220, 223, 230, 234, 237, 240, 242, 244,
		245, 245, 246, 242, 237, 230, 220, 210, 190, 170, 148, 125,
		104, 83, 64, 49, 36, 22, 14, 8, 8, 8, 8, 8, 8, 8, 8, 8,
		8, 8, 8, 8, 8, 8, 8,
	})
	shape2 := ccDcmf([]byte{
		47, 52, 137, 58, 61, 21, 73, 240, 55, 95, 255,
		68, 83, 108, 48, 50, 49, 45, 35, 155, 31, 41,
	})

	enc := NewRangeEncoder(16)
	enc.EncodeCDF(30, gain2)
	enc.EncodeCDF(10, shape2)
	enc.Done()
	dec := NewRangeDecoder(enc.Bytes())
	got := DecodeSmplGains(dec, LoadSmplMem(), 2, [4]int32{})

	if got.GainMain != 30 || got.GainDelta != 10 {
		t.Fatalf("gain symbols got main=%d shape=%d, want main=30 shape=10", got.GainMain, got.GainDelta)
	}
	want := [4]int32{-800294, -794406}
	if got.GainQ != want {
		t.Fatalf("gain reconstruction got %v, want %v", got.GainQ, want)
	}
}
