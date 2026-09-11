package srtp

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

// sframeFields are the expected sframe.* outputs from kats.json.
type sframeFields struct {
	ParticipantPeerID string `json:"participantPeerId"`
	InfoLabelPeer     string `json:"infoLabelPeer"`
	PeerKey32         string `json:"peerKey32"`
	CounterToIv5      string `json:"counterToIv_5"`
	Header50          string `json:"header_5_0"`
	EncryptOut        string `json:"encrypt_out"`
}

func loadSframeFields(t *testing.T) sframeFields {
	t.Helper()
	raw, err := os.ReadFile("testdata/kats.json")
	if err != nil {
		t.Fatalf("read kats.json: %v", err)
	}
	var doc struct {
		Sframe sframeFields `json:"sframe"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse kats.json: %v", err)
	}
	return doc.Sframe
}

// TestSframeParticipantKeyAndLabel checks the participant id, info label, and the
// derived 32-byte per-participant key against kats.json.
func TestSframeParticipantKeyAndLabel(t *testing.T) {
	k := loadKat(t)
	sf := loadSframeFields(t)
	callKey := mustHex(t, k.Inputs.CallKey)

	peerID := FormatSframeParticipantID(k.Inputs.PeerLid)
	if peerID != sf.ParticipantPeerID {
		t.Errorf("participantPeerId = %q, want %q", peerID, sf.ParticipantPeerID)
	}
	if got := SframeInfoLabel(peerID); got != sf.InfoLabelPeer {
		t.Errorf("infoLabelPeer = %q, want %q", got, sf.InfoLabelPeer)
	}
	key, err := DeriveE2eSframeKeyForParticipant(callKey, peerID)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if got := hex.EncodeToString(key); got != sf.PeerKey32 {
		t.Errorf("peerKey32 = %s, want %s", got, sf.PeerKey32)
	}
}

// TestDeriveWarpAuthKey checks the WARP auth key derivation against an independently
// computed HKDF-SHA256 vector (empty salt, IKM = callKey, info = "warp auth key",
// L = 32) for kats.json's callKey, and the <32-byte length guard.
func TestDeriveWarpAuthKey(t *testing.T) {
	k := loadKat(t)
	callKey := mustHex(t, k.Inputs.CallKey)

	// Independently computed (RFC 5869 HKDF-SHA256, empty salt) from the captured callKey.
	const wantWarpAuthKey = "6c768f61c7bd16f8c022b58d6a9f01ab832abe95e27538a7d6c584a26b8e0734"
	got, err := DeriveWarpAuthKey(callKey)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if hex.EncodeToString(got) != wantWarpAuthKey {
		t.Errorf("warp auth key = %x, want %s", got, wantWarpAuthKey)
	}

	if _, err := DeriveWarpAuthKey(callKey[:31]); err != errBadCallKeyLen {
		t.Errorf("short callKey error = %v, want errBadCallKeyLen", err)
	}
}

// TestSframeCounterIVAndHeader checks the GCM nonce and the varint header (with a
// parse round-trip) against kats.json.
func TestSframeCounterIVAndHeader(t *testing.T) {
	sf := loadSframeFields(t)
	iv := counterToIV(5)
	if got := hex.EncodeToString(iv[:]); got != sf.CounterToIv5 {
		t.Errorf("counterToIv_5 = %s, want %s", got, sf.CounterToIv5)
	}
	header := buildSframeHeader(5, 0)
	if got := hex.EncodeToString(header); got != sf.Header50 {
		t.Errorf("header_5_0 = %s, want %s", got, sf.Header50)
	}
	counter, keyID, ok := parseSframeHeader(buildSframeHeader(5, 0))
	if !ok || counter != 5 || keyID != 0 {
		t.Errorf("parse round-trip = (%d, %d, %v), want (5, 0, true)", counter, keyID, ok)
	}
}

// TestSframeEncryptMatchesKAT seeds the counter to the captured value and checks
// the sealed frame against kats.json.
func TestSframeEncryptMatchesKAT(t *testing.T) {
	k := loadKat(t)
	sf := loadSframeFields(t)
	callKey := mustHex(t, k.Inputs.CallKey)

	s, err := NewSframeSession(callKey, k.Inputs.SelfLid, k.Inputs.PeerLid)
	if err != nil {
		t.Fatalf("new session: %v", err)
	}
	s.txCounter = 5
	out, err := s.Encrypt(mustHex(t, k.Inputs.Payload))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if got := hex.EncodeToString(out); got != sf.EncryptOut {
		t.Errorf("encrypt_out = %s, want %s", got, sf.EncryptOut)
	}
}

// TestSframeEncryptDecryptRoundTrip confirms a frame the sender seals for the peer
// decrypts back to the original plaintext on the receiver.
func TestSframeEncryptDecryptRoundTrip(t *testing.T) {
	k := loadKat(t)
	callKey := mustHex(t, k.Inputs.CallKey)
	sender, err := NewSframeSession(callKey, k.Inputs.SelfLid, k.Inputs.PeerLid)
	if err != nil {
		t.Fatalf("sender: %v", err)
	}
	receiver, err := NewSframeSession(callKey, k.Inputs.PeerLid, k.Inputs.SelfLid)
	if err != nil {
		t.Fatalf("receiver: %v", err)
	}
	payload := []byte("hello sframe payload")
	frame, err := sender.Encrypt(payload)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	plain, ok := receiver.Decrypt(frame)
	if !ok || !bytes.Equal(plain, payload) {
		t.Errorf("decrypt = (%x, %v), want (%x, true)", plain, ok, payload)
	}
}

// TestSframeWrongKeyDoesNotForge confirms GCM auth rejects a frame opened under the
// wrong key: it must fail closed to plaintext, never emit a forged decryption.
func TestSframeWrongKeyDoesNotForge(t *testing.T) {
	k := loadKat(t)
	callKey := mustHex(t, k.Inputs.CallKey)
	sender, err := NewSframeSession(callKey, k.Inputs.SelfLid, k.Inputs.PeerLid)
	if err != nil {
		t.Fatalf("sender: %v", err)
	}
	payload := bytes.Repeat([]byte{0xaa}, 24)
	frame, err := sender.Encrypt(payload)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	other := append([]byte(nil), callKey...)
	other[0] ^= 0xff
	receiver, err := NewSframeSession(other, k.Inputs.PeerLid, k.Inputs.SelfLid)
	if err != nil {
		t.Fatalf("receiver: %v", err)
	}
	if plain, ok := receiver.Decrypt(frame); ok {
		t.Errorf("wrong key recovered plaintext %x, want fail-closed (ok=false)", plain)
	}
}

// TestSframePlainOpusPassesThrough confirms real plain-Opus frames classify as
// plaintext pass-through (ok=false), so the caller uses the raw bytes unchanged.
func TestSframePlainOpusPassesThrough(t *testing.T) {
	k := loadKat(t)
	callKey := mustHex(t, k.Inputs.CallKey)
	receiver, err := NewSframeSession(callKey, k.Inputs.SelfLid, k.Inputs.PeerLid)
	if err != nil {
		t.Fatalf("receiver: %v", err)
	}
	frames := [][]byte{
		{0x00},
		mustHex(t, "90b81414c4"),
		mustHex(t, "12101a759d3399bbaefb874fd75a004af7c0"),
		mustHex(t, "9036ba6ffa40"),
		mustHex(t, "1236262b4ac920b1206166637b5af2"),
	}
	for _, f := range frames {
		if plain, ok := receiver.Decrypt(f); ok {
			t.Errorf("plain frame %x classified as decrypted (%x); want plaintext pass-through", f, plain)
		}
	}
}
