package srtp

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"testing"
)

type kat struct {
	Inputs struct {
		CallKey string `json:"callKey"`
		HbhKey  string `json:"hbhKey"`
		SelfLid string `json:"selfLid"`
		PeerLid string `json:"peerLid"`
		SSRC    uint32 `json:"ssrc"`
		Seq     uint16 `json:"seq"`
		Roc     uint32 `json:"roc"`
		Payload string `json:"payload"`
	} `json:"inputs"`
	E2eSrtp struct {
		PeerCipherKey string `json:"peer_cipherKey"`
		PeerSalt      string `json:"peer_salt"`
		PeerAuthKey   string `json:"peer_authKey"`
		SelfCipherKey string `json:"self_cipherKey"`
		SelfSalt      string `json:"self_salt"`
		SelfAuthKey   string `json:"self_authKey"`
		RtpIv         string `json:"rtpIv"`
		CipherOut     string `json:"cipher_out"`
	} `json:"e2e_srtp"`
}

func loadKat(t *testing.T) kat {
	t.Helper()
	raw, err := os.ReadFile("testdata/kats.json")
	if err != nil {
		t.Fatalf("read kats.json: %v", err)
	}
	var k kat
	if err := json.Unmarshal(raw, &k); err != nil {
		t.Fatalf("parse kats.json: %v", err)
	}
	return k
}

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("bad hex %q: %v", s, err)
	}
	return b
}

// TestDeriveE2eKeysMatchesKAT derives the peer and self session keys from the call
// key and asserts cipher/salt/auth match the kats.json e2e_srtp expectations.
func TestDeriveE2eKeysMatchesKAT(t *testing.T) {
	k := loadKat(t)
	callKey := mustHex(t, k.Inputs.CallKey)

	peer, err := DeriveE2eKeys(callKey, k.Inputs.PeerLid)
	if err != nil {
		t.Fatalf("peer derive: %v", err)
	}
	if !bytes.Equal(peer.CipherKey[:], mustHex(t, k.E2eSrtp.PeerCipherKey)) {
		t.Errorf("peer cipher_key = %x, want %s", peer.CipherKey, k.E2eSrtp.PeerCipherKey)
	}
	if !bytes.Equal(peer.Salt[:], mustHex(t, k.E2eSrtp.PeerSalt)) {
		t.Errorf("peer salt = %x, want %s", peer.Salt, k.E2eSrtp.PeerSalt)
	}
	if !bytes.Equal(peer.AuthKey[:], mustHex(t, k.E2eSrtp.PeerAuthKey)) {
		t.Errorf("peer auth_key = %x, want %s", peer.AuthKey, k.E2eSrtp.PeerAuthKey)
	}

	self, err := DeriveE2eKeys(callKey, k.Inputs.SelfLid)
	if err != nil {
		t.Fatalf("self derive: %v", err)
	}
	if !bytes.Equal(self.CipherKey[:], mustHex(t, k.E2eSrtp.SelfCipherKey)) {
		t.Errorf("self cipher_key = %x, want %s", self.CipherKey, k.E2eSrtp.SelfCipherKey)
	}
	if !bytes.Equal(self.AuthKey[:], mustHex(t, k.E2eSrtp.SelfAuthKey)) {
		t.Errorf("self auth_key = %x, want %s", self.AuthKey, k.E2eSrtp.SelfAuthKey)
	}
}

// TestRtpIVMatchesKAT builds the per-packet IV from the peer salt and asserts it
// matches the kats.json rtpIv.
func TestRtpIVMatchesKAT(t *testing.T) {
	k := loadKat(t)
	var salt [14]byte
	copy(salt[:], mustHex(t, k.E2eSrtp.PeerSalt))
	iv := BuildE2eRtpIV(salt[:], k.Inputs.SSRC, k.Inputs.Roc, k.Inputs.Seq)
	if got := hex.EncodeToString(iv[:]); got != k.E2eSrtp.RtpIv {
		t.Errorf("rtpIv = %s, want %s", got, k.E2eSrtp.RtpIv)
	}
}

// TestCryptPayloadMatchesKAT encrypts the payload and asserts the ciphertext matches
// cipher_out, then decrypts to confirm the cipher round-trips.
func TestCryptPayloadMatchesKAT(t *testing.T) {
	k := loadKat(t)
	var keys E2eSrtpKeys
	copy(keys.CipherKey[:], mustHex(t, k.E2eSrtp.PeerCipherKey))
	copy(keys.Salt[:], mustHex(t, k.E2eSrtp.PeerSalt))
	payload := mustHex(t, k.Inputs.Payload)

	ct, err := CryptPayload(&keys, k.Inputs.SSRC, k.Inputs.Seq, k.Inputs.Roc, payload)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if got := hex.EncodeToString(ct); got != k.E2eSrtp.CipherOut {
		t.Errorf("cipher_out = %s, want %s", got, k.E2eSrtp.CipherOut)
	}
	pt, err := CryptPayload(&keys, k.Inputs.SSRC, k.Inputs.Seq, k.Inputs.Roc, ct)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(pt, payload) {
		t.Errorf("decrypt round-trip = %x, want %x", pt, payload)
	}
}

func TestSrtcpRoundTripsAndAuthenticates(t *testing.T) {
	callKey := make([]byte, 32)
	for i := range callKey {
		callKey[i] = byte(i)
	}
	keys, err := DeriveE2eSrtcpKeys(callKey, "111111111111111:0@lid")
	if err != nil {
		t.Fatalf("derive srtcp: %v", err)
	}
	rtpKeys, err := DeriveE2eKeys(callKey, "111111111111111:0@lid")
	if err != nil {
		t.Fatalf("derive srtp: %v", err)
	}
	if keys == rtpKeys {
		t.Fatal("SRTCP keys must use distinct KDF labels")
	}

	ssrc := uint32(0x12345678)
	plain := mustHex(t, "80c8000612345678ea11180000000000000006400000000500000258")
	protected, err := ProtectSrtcp(&keys, ssrc, 1, plain)
	if err != nil {
		t.Fatalf("protect srtcp: %v", err)
	}
	if len(protected) != len(plain)+SrtcpTrailerLen {
		t.Fatalf("protected length = %d, want %d", len(protected), len(plain)+SrtcpTrailerLen)
	}
	recovered, index, ok := UnprotectSrtcp(&keys, ssrc, protected)
	if !ok || index != 1 || !bytes.Equal(recovered, plain) {
		t.Fatalf("unprotect = (%x, %d, %v), want (%x, 1, true)", recovered, index, ok, plain)
	}
	protected[len(protected)-1] ^= 1
	if _, _, ok := UnprotectSrtcp(&keys, ssrc, protected); ok {
		t.Fatal("forged SRTCP tag accepted")
	}
}

func TestDeriveE2eSRTCPKeysFromRawMatchesRawRootDerivation(t *testing.T) {
	rawE2E := make([]byte, 32)
	for i := range rawE2E {
		rawE2E[i] = byte(0xa0 + i)
	}
	const participantID = "111111111111111:14@lid"

	got, err := DeriveE2eSRTCPKeysFromRaw(rawE2E, participantID)
	if err != nil {
		t.Fatalf("derive SRTCP keys from raw E2E root: %v", err)
	}
	if want := mustHex(t, "96ec6c38976e1561a01929ef61627fd9"); !bytes.Equal(got.CipherKey[:], want) {
		t.Fatalf("raw-root SRTCP cipher key = %x, want %x", got.CipherKey, want)
	}
	if want := mustHex(t, "b033e3437b17edaa45863a19a6a3969633353bc7"); !bytes.Equal(got.AuthKey[:], want) {
		t.Fatalf("raw-root SRTCP auth key = %x, want %x", got.AuthKey, want)
	}
	if want := mustHex(t, "d66012ec5832edc623a9fd742ff1"); !bytes.Equal(got.Salt[:], want) {
		t.Fatalf("raw-root SRTCP salt = %x, want %x", got.Salt, want)
	}
	want, err := DeriveE2eSrtcpKeys(rawE2E, participantID)
	if err != nil {
		t.Fatalf("derive equivalent SRTCP keys: %v", err)
	}
	if got != want {
		t.Fatalf("raw-root SRTCP keys = %#v, want %#v", got, want)
	}
	if _, err = DeriveE2eSRTCPKeysFromRaw(rawE2E[:31], participantID); !errors.Is(err, errShortKey) {
		t.Fatalf("short raw-root error = %v, want %v", err, errShortKey)
	}
}

// TestRocTrackerWraps exercises both the send-side monotonic tracker and the
// recv-side guess estimator across wraps, reorder dips, and late packets.
func TestRocTrackerWraps(t *testing.T) {
	var tx RocTracker
	if got := tx.Advance(0xFFFE); got != 0 {
		t.Fatalf("seed: roc=%d, want 0", got)
	}
	tx.Advance(0xFFFF)
	if got := tx.Advance(0x0000); got != 1 {
		t.Errorf("0xFFFF->0x0000: roc=%d, want 1", got)
	}
	tx.Advance(0x0001)
	if got := tx.Advance(0x0000); got != 1 {
		t.Errorf("backward dip must not bump: roc=%d, want 1", got)
	}
	tx.Advance(0x0001)
	for _, s := range []uint16{0x7000, 0xE000, 0xFFFF} {
		tx.Advance(s)
	}
	if got := tx.Advance(0x0000); got != 2 {
		t.Errorf("second wrap: roc=%d, want 2", got)
	}

	var rx RecvRocTracker
	if got := rx.GuessRoc(0xFFFE); got != 0 {
		t.Fatalf("recv seed: roc=%d, want 0", got)
	}
	rx.GuessRoc(0xFFFF)
	if got := rx.GuessRoc(0x0000); got != 1 {
		t.Errorf("recv 0xFFFF->0x0000: roc=%d, want 1", got)
	}
	rx.GuessRoc(0x0001)
	if got := rx.GuessRoc(0x0000); got != 1 {
		t.Errorf("reordered dip stays in roc: roc=%d, want 1", got)
	}
	if got := rx.GuessRoc(0x0002); got != 1 {
		t.Errorf("state intact after dip: roc=%d, want 1", got)
	}
	for _, s := range []uint16{0x7000, 0xE000, 0xFFFF} {
		if got := rx.GuessRoc(s); got != 1 {
			t.Errorf("walk forward seq=%#x: roc=%d, want 1", s, got)
		}
	}
	if got := rx.GuessRoc(0x0000); got != 2 {
		t.Errorf("recv second wrap: roc=%d, want 2", got)
	}
	if got := rx.GuessRoc(0xFFF0); got != 1 {
		t.Errorf("late packet returns lower roc: roc=%d, want 1", got)
	}
	if got := rx.GuessRoc(0x0001); got != 2 {
		t.Errorf("state not corrupted by late packet: roc=%d, want 2", got)
	}
}

func TestRecvRocEstimateCommitsOnlyAuthenticatedPacket(t *testing.T) {
	var rx RecvRocTracker
	if got := rx.EstimateRoc(0xFFFF); got != 0 {
		t.Fatalf("initial estimate = %d, want 0", got)
	}
	if got := rx.EstimateRoc(0x0000); got != 0 {
		t.Fatalf("uncommitted estimate mutated state: got %d, want 0", got)
	}

	roc := rx.EstimateRoc(0xFFFF)
	rx.CommitRoc(roc, 0xFFFF)
	if got := rx.EstimateRoc(0x0000); got != 1 {
		t.Fatalf("authenticated wrap estimate = %d, want 1", got)
	}

	if got := rx.EstimateRoc(0x0001); got != 1 {
		t.Fatalf("second uncommitted estimate = %d, want 1", got)
	}
	if got := rx.EstimateRoc(0x0000); got != 1 {
		t.Fatalf("uncommitted estimate poisoned state: got %d, want 1", got)
	}

	rx.CommitRoc(1, 0x0000)
	if got := rx.EstimateRoc(0x0001); got != 1 {
		t.Fatalf("committed wrap state = %d, want 1", got)
	}
}

func TestUnauthenticatedStaircaseCannotAdvanceROCWithoutCommit(t *testing.T) {
	var rx RecvRocTracker
	rx.GuessRoc(0x7FFE)
	if rx.roc != 0 {
		t.Fatalf("seeded ROC = %d, want 0", rx.roc)
	}

	_ = rx.EstimateRoc(0xFFFE)
	_ = rx.EstimateRoc(0x7FFD)
	if rx.roc != 0 {
		t.Fatalf("estimate-only staircase advanced ROC to %d", rx.roc)
	}
	if got := rx.EstimateRoc(0x7FFF); got != 0 {
		t.Fatalf("legitimate in-window estimate = %d, want 0", got)
	}

	roc := rx.EstimateRoc(0xFFFE)
	rx.CommitRoc(roc, 0xFFFE)
	roc = rx.EstimateRoc(0x7FFD)
	rx.CommitRoc(roc, 0x7FFD)
	if rx.roc != 1 {
		t.Fatalf("committed staircase ROC = %d, want 1", rx.roc)
	}
}
