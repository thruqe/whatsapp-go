package srtp

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

// hbhFields are the expected hbh_srtp.* outputs from kats.json.
type hbhFields struct {
	UplinkKey30 string `json:"uplinkKey30"`
	MasterKey   string `json:"masterKey"`
	MasterSalt  string `json:"masterSalt"`
	SessionKey  string `json:"sessionKey"`
	SessionSalt string `json:"sessionSalt"`
	AuthKey     string `json:"authKey"`
	RtpIcmNonce string `json:"rtpIcmNonce"`
	CipherOut   string `json:"cipher_out"`
}

func loadHbhFields(t *testing.T) hbhFields {
	t.Helper()
	raw, err := os.ReadFile("testdata/kats.json")
	if err != nil {
		t.Fatalf("read kats.json: %v", err)
	}
	var doc struct {
		HbhSrtp hbhFields `json:"hbh_srtp"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse kats.json: %v", err)
	}
	return doc.HbhSrtp
}

// TestHbhUplinkDerivationMatchesKAT walks the uplink derivation: 30-byte key,
// master key/salt split, and the libsrtp session-key expansion, all vs kats.json.
func TestHbhUplinkDerivationMatchesKAT(t *testing.T) {
	k := loadKat(t)
	hbh := mustHex(t, k.Inputs.HbhKey)
	hf := loadHbhFields(t)

	uplink, err := DeriveHbhSrtpKeyUplink(hbh)
	if err != nil {
		t.Fatalf("uplink derive: %v", err)
	}
	if got := hex.EncodeToString(uplink); got != hf.UplinkKey30 {
		t.Errorf("uplinkKey30 = %s, want %s", got, hf.UplinkKey30)
	}

	keying, err := KeyingFromHbhKeyUplink(hbh)
	if err != nil {
		t.Fatalf("keying: %v", err)
	}
	if got := hex.EncodeToString(keying.MasterKey[:]); got != hf.MasterKey {
		t.Errorf("masterKey = %s, want %s", got, hf.MasterKey)
	}
	if got := hex.EncodeToString(keying.MasterSalt[:]); got != hf.MasterSalt {
		t.Errorf("masterSalt = %s, want %s", got, hf.MasterSalt)
	}

	session, err := ExpandLibsrtpSessionKeys(&keying)
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if got := hex.EncodeToString(session.SessionKey[:]); got != hf.SessionKey {
		t.Errorf("sessionKey = %s, want %s", got, hf.SessionKey)
	}
	if got := hex.EncodeToString(session.SessionSalt[:]); got != hf.SessionSalt {
		t.Errorf("sessionSalt = %s, want %s", got, hf.SessionSalt)
	}
	if got := hex.EncodeToString(session.AuthKey[:]); got != hf.AuthKey {
		t.Errorf("authKey = %s, want %s", got, hf.AuthKey)
	}
}

// TestHbhICMNonceAndCipherMatchKAT checks the AES-ICM nonce and the libsrtp
// AES-ICM cipher output (with round-trip) against kats.json.
func TestHbhICMNonceAndCipherMatchKAT(t *testing.T) {
	k := loadKat(t)
	hbh := mustHex(t, k.Inputs.HbhKey)
	hf := loadHbhFields(t)

	keying, err := KeyingFromHbhKeyUplink(hbh)
	if err != nil {
		t.Fatalf("keying: %v", err)
	}
	session, err := ExpandLibsrtpSessionKeys(&keying)
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	packetIndex := uint64(k.Inputs.Roc)<<16 | uint64(k.Inputs.Seq)

	nonce := BuildRtpICMNonce(k.Inputs.SSRC, packetIndex)
	if got := hex.EncodeToString(nonce[:]); got != hf.RtpIcmNonce {
		t.Errorf("rtpIcmNonce = %s, want %s", got, hf.RtpIcmNonce)
	}

	payload := mustHex(t, k.Inputs.Payload)
	ct, err := CryptRtpPayload(&session, k.Inputs.SSRC, packetIndex, payload)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if got := hex.EncodeToString(ct); got != hf.CipherOut {
		t.Errorf("cipher_out = %s, want %s", got, hf.CipherOut)
	}
	pt, err := CryptRtpPayload(&session, k.Inputs.SSRC, packetIndex, ct)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(pt, payload) {
		t.Errorf("decrypt round-trip = %x, want %x", pt, payload)
	}
}
