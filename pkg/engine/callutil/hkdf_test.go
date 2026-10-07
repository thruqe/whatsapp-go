package callutil

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

// TestHKDFSHA256RFC5869 is the KAT against the published RFC 5869 Appendix A
// HKDF-SHA256 vectors (Test Cases 1-3): extract-and-expand must reproduce the
// expected OKM byte-for-byte at the requested length.
func TestHKDFSHA256RFC5869(t *testing.T) {
	raw, err := os.ReadFile("testdata/rfc5869_hkdf_sha256.json")
	if err != nil {
		t.Fatalf("read rfc5869_hkdf_sha256.json: %v", err)
	}
	var recs []struct {
		Name string `json:"name"`
		IKM  string `json:"ikm"`
		Salt string `json:"salt"`
		Info string `json:"info"`
		L    int    `json:"l"`
		OKM  string `json:"okm"`
	}
	if err := json.Unmarshal(raw, &recs); err != nil {
		t.Fatalf("parse rfc5869_hkdf_sha256.json: %v", err)
	}
	if len(recs) == 0 {
		t.Fatal("no hkdf vectors")
	}

	for _, rec := range recs {
		ikm := mustHex(t, rec.IKM)
		salt := mustHex(t, rec.Salt)
		info := mustHex(t, rec.Info)
		want := mustHex(t, rec.OKM)

		got, err := HKDFSHA256(salt, ikm, info, rec.L)
		if err != nil {
			t.Errorf("%s: HKDFSHA256: %v", rec.Name, err)
			continue
		}
		if len(got) != rec.L {
			t.Errorf("%s: length = %d, want %d", rec.Name, len(got), rec.L)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s: okm = %x, want %x", rec.Name, got, want)
		}
	}
}

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("bad hex %q: %v", s, err)
	}
	return b
}
