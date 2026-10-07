package srtp

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

// TestWarpMITagMatchesKAT checks the 4-byte WARP MI tag over the sample packet.
func TestWarpMITagMatchesKAT(t *testing.T) {
	raw, err := os.ReadFile("testdata/kats.json")
	if err != nil {
		t.Fatalf("read kats.json: %v", err)
	}
	var doc struct {
		Inputs struct {
			SamplePacket string `json:"samplePacket"`
			Roc          uint32 `json:"roc"`
		} `json:"inputs"`
		E2eSrtp struct {
			PeerAuthKey string `json:"peer_authKey"`
			WarpMITag4  string `json:"warp_mi_tag4"`
		} `json:"e2e_srtp"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse kats.json: %v", err)
	}
	authKey := mustHex(t, doc.E2eSrtp.PeerAuthKey)
	packet := mustHex(t, doc.Inputs.SamplePacket)

	tag := ComputeWarpMITag(authKey, packet, doc.Inputs.Roc, WarpMITagLen)
	if got := hex.EncodeToString(tag); got != doc.E2eSrtp.WarpMITag4 {
		t.Errorf("warp_mi_tag4 = %s, want %s", got, doc.E2eSrtp.WarpMITag4)
	}

	// AppendWarpMITag yields packet || tag.
	appended := AppendWarpMITag(authKey, packet, doc.Inputs.Roc, WarpMITagLen)
	if got, want := hex.EncodeToString(appended), doc.Inputs.SamplePacket+doc.E2eSrtp.WarpMITag4; got != want {
		t.Errorf("appended = %s, want %s", got, want)
	}
	if !VerifyWarpMITag(authKey, packet, doc.Inputs.Roc, len(tag), tag) {
		t.Error("valid WARP MI tag was rejected")
	}

	tamperedTag := append([]byte(nil), tag...)
	tamperedTag[0] ^= 0x80
	if VerifyWarpMITag(authKey, packet, doc.Inputs.Roc, len(tamperedTag), tamperedTag) {
		t.Error("tampered WARP MI tag was accepted")
	}

	wrongKey := append([]byte(nil), authKey...)
	wrongKey[0] ^= 0x80
	if VerifyWarpMITag(wrongKey, packet, doc.Inputs.Roc, len(tag), tag) {
		t.Error("WARP MI tag authenticated under the wrong participant key")
	}

	if VerifyWarpMITag(authKey, packet, doc.Inputs.Roc+1, len(tag), tag) {
		t.Error("WARP MI tag authenticated under the wrong ROC")
	}
	if VerifyWarpMITag(authKey, packet, doc.Inputs.Roc, 0, nil) {
		t.Error("empty WARP MI tag was accepted")
	}
	if VerifyWarpMITag(authKey, packet, doc.Inputs.Roc, len(tag), tag[:len(tag)-1]) {
		t.Error("truncated WARP MI tag was accepted")
	}
	if VerifyWarpMITag(authKey, packet, doc.Inputs.Roc, 21, make([]byte, 21)) {
		t.Error("oversized WARP MI tag was accepted")
	}
}

// TestAudioPiggybackExtension checks the piggyback gating and the extension word.
func TestAudioPiggybackExtension(t *testing.T) {
	if AudioPiggybackExtensionFor(0, true, WarpPiggybackStartPacket) != nil {
		t.Error("packet 0 should have no piggyback extension")
	}
	if AudioPiggybackExtensionFor(2, false, WarpPiggybackStartPacket) != nil {
		t.Error("disabled piggyback should be nil")
	}
	w := AudioPiggybackExtensionFor(2, true, WarpPiggybackStartPacket)
	if w == nil || *w != 0x30010000 {
		t.Errorf("packet 2 extension = %v, want 0x30010000", w)
	}
}
