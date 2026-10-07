package srtp

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha1"
	"errors"

	"github.com/rs/zerolog"
	"go.mau.fi/whatsmeow/callutil"
)

// errShortKey is returned when the supplied key material is shorter than the
// 32-byte minimum the E2E derivation requires.
var errShortKey = errors.New("srtp: key material shorter than 32 bytes")

// E2eSrtpKeys holds the per-participant session keys for the end-to-end 1:1 SRTP
// cipher: the AES-128 cipher key, the 14-byte master salt, and the auth key.
type E2eSrtpKeys struct {
	CipherKey [16]byte
	Salt      [14]byte
	AuthKey   [20]byte
}

const (
	SrtcpAuthTagLen = 10
	SrtcpTrailerLen = 4 + SrtcpAuthTagLen
	rtcpHeaderLen   = 8
)

// aesCmKdf is the AES-CM PRF (libsrtp KDF): IV = master salt with label XORed into
// byte 7, zero-padded to 16, then AES-128-CTR keystream over len zero bytes.
func aesCmKdf(masterKey, masterSalt []byte, label byte, length int) ([]byte, error) {
	// Source of truth: https://github.com/oxidezap/whatsapp-rust/blob/41095d4e6ba4610e054e9ede3af1d5e88a83faee/wacore/src/voip/e2e_srtp.rs#L24-L32
	var iv [16]byte
	copy(iv[:14], masterSalt[:14])
	iv[7] ^= label
	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return nil, err
	}
	out := make([]byte, length)
	cipher.NewCTR(block, iv[:]).XORKeyStream(out, out)
	return out, nil
}

// deriveSessionKeysFromMaster splits the 46-byte master into key (16) + salt (14)
// and runs the AES-CM PRF three times (labels 0x00/0x01/0x02) for cipher/auth/salt.
func deriveSessionKeysFromMasterLabels(master []byte, cipherLabel, authLabel, saltLabel byte) (E2eSrtpKeys, error) {
	// Source of truth: https://github.com/oxidezap/whatsapp-rust/blob/41095d4e6ba4610e054e9ede3af1d5e88a83faee/wacore/src/voip/e2e_srtp.rs#L34-L49
	masterKey := master[0:16]
	masterSalt := master[16:30]
	var keys E2eSrtpKeys
	cipherKey, err := aesCmKdf(masterKey, masterSalt, cipherLabel, 16)
	if err != nil {
		return E2eSrtpKeys{}, err
	}
	copy(keys.CipherKey[:], cipherKey)
	authKey, err := aesCmKdf(masterKey, masterSalt, authLabel, 20)
	if err != nil {
		return E2eSrtpKeys{}, err
	}
	copy(keys.AuthKey[:], authKey)
	salt, err := aesCmKdf(masterKey, masterSalt, saltLabel, 14)
	if err != nil {
		return E2eSrtpKeys{}, err
	}
	copy(keys.Salt[:], salt)
	return keys, nil
}

func deriveSessionKeysFromMaster(master []byte) (E2eSrtpKeys, error) {
	return deriveSessionKeysFromMasterLabels(master, 0x00, 0x01, 0x02)
}

// DeriveE2eKeys derives the E2E 1:1 keys from callKey (>=32B) using participantLid
// as the HKDF info. It errors when callKey is shorter than 32 bytes.
func DeriveE2eKeys(callKey []byte, participantLid string, log ...zerolog.Logger) (E2eSrtpKeys, error) {
	// Source of truth: https://github.com/oxidezap/whatsapp-rust/blob/41095d4e6ba4610e054e9ede3af1d5e88a83faee/wacore/src/voip/e2e_srtp.rs#L55-L61
	lg := pickLog(log)
	if len(callKey) < 32 {
		lg.Debug().Err(errShortKey).Int("call_key_bytes", len(callKey)).Str("participant_lid", participantLid).Msg("e2e key derivation rejected: short call key")
		return E2eSrtpKeys{}, errShortKey
	}
	lg.Debug().Int("call_key_bytes", len(callKey)).Str("participant_lid", participantLid).Msg("deriving e2e srtp keys from call key")
	master, err := callutil.HKDFSHA256(make([]byte, 32), callKey[:32], []byte(participantLid), 46)
	if err != nil {
		lg.Debug().Err(err).Str("participant_lid", participantLid).Msg("e2e master hkdf failed")
		return E2eSrtpKeys{}, err
	}
	return deriveSessionKeysFromMaster(master)
}

// DeriveE2eSrtcpKeys derives the per-participant SRTCP keys using RFC 3711 labels.
func DeriveE2eSrtcpKeys(callKey []byte, participantLid string, log ...zerolog.Logger) (E2eSrtpKeys, error) {
	lg := pickLog(log)
	if len(callKey) < 32 {
		return E2eSrtpKeys{}, errShortKey
	}
	master, err := callutil.HKDFSHA256(make([]byte, 32), callKey[:32], []byte(participantLid), 46)
	if err != nil {
		return E2eSrtpKeys{}, err
	}
	keys, err := deriveSessionKeysFromMasterLabels(master, 0x03, 0x04, 0x05)
	if err != nil {
		lg.Debug().Err(err).Str("participant_lid", participantLid).Msg("e2e srtcp key derivation failed")
	}
	return keys, err
}

// DeriveE2eKeysFromRaw derives the E2E 1:1 keys from a keygen-v2 <raw_e2e> blob
// (>=32B) in place of callKey. It errors when rawE2e is shorter than 32 bytes.
func DeriveE2eKeysFromRaw(rawE2e []byte, participantLid string, log ...zerolog.Logger) (E2eSrtpKeys, error) {
	// Source of truth: https://github.com/oxidezap/whatsapp-rust/blob/41095d4e6ba4610e054e9ede3af1d5e88a83faee/wacore/src/voip/e2e_srtp.rs#L64-L70
	lg := pickLog(log)
	if len(rawE2e) < 32 {
		lg.Debug().Err(errShortKey).Int("raw_e2e_bytes", len(rawE2e)).Str("participant_lid", participantLid).Msg("e2e key derivation rejected: short raw blob")
		return E2eSrtpKeys{}, errShortKey
	}
	lg.Debug().Int("raw_e2e_bytes", len(rawE2e)).Str("participant_lid", participantLid).Msg("deriving e2e srtp keys from raw blob")
	master, err := callutil.HKDFSHA256(make([]byte, 32), rawE2e[:32], []byte(participantLid), 46)
	if err != nil {
		lg.Debug().Err(err).Str("participant_lid", participantLid).Msg("e2e master hkdf failed")
		return E2eSrtpKeys{}, err
	}
	return deriveSessionKeysFromMaster(master)
}

// DeriveE2eSRTCPKeysFromRaw derives per-participant SRTCP keys from a keygen-v2
// raw E2E root using the RFC 3711 SRTCP labels.
func DeriveE2eSRTCPKeysFromRaw(rawE2E []byte, participantLID string, log ...zerolog.Logger) (E2eSrtpKeys, error) {
	// Source of truth: https://github.com/oxidezap/whatsapp-rust/blob/41095d4e6ba4610e054e9ede3af1d5e88a83faee/wacore/src/voip/e2e_srtp.rs#L34-L70
	lg := pickLog(log)
	if len(rawE2E) < 32 {
		lg.Debug().Err(errShortKey).Int("raw_e2e_bytes", len(rawE2E)).Str("participant_lid", participantLID).Msg("e2e srtcp key derivation rejected: short raw blob")
		return E2eSrtpKeys{}, errShortKey
	}
	master, err := callutil.HKDFSHA256(make([]byte, 32), rawE2E[:32], []byte(participantLID), 46)
	if err != nil {
		lg.Debug().Err(err).Str("participant_lid", participantLID).Msg("e2e srtcp master hkdf failed")
		return E2eSrtpKeys{}, err
	}
	keys, err := deriveSessionKeysFromMasterLabels(master, 0x03, 0x04, 0x05)
	if err != nil {
		lg.Debug().Err(err).Str("participant_lid", participantLID).Msg("e2e srtcp key derivation failed")
	}
	return keys, err
}

// BuildE2eRtpIV builds the E2E RTP IV: salt right-aligned into 16 bytes, SSRC XORed
// at bytes 4-7, and the 48-bit packet index (ROC<<16 | seq) XORed at bytes 8-13.
func BuildE2eRtpIV(salt []byte, ssrc uint32, roc uint32, seq uint16) [16]byte {
	// Source of truth: https://github.com/oxidezap/whatsapp-rust/blob/41095d4e6ba4610e054e9ede3af1d5e88a83faee/wacore/src/voip/e2e_srtp.rs#L74-L92
	var iv [16]byte
	off := 14 - len(salt)
	copy(iv[off:off+len(salt)], salt)
	iv[4] ^= byte(ssrc >> 24)
	iv[5] ^= byte(ssrc >> 16)
	iv[6] ^= byte(ssrc >> 8)
	iv[7] ^= byte(ssrc)
	packetIndex := uint64(roc)*0x1_0000 + uint64(seq)
	hi16 := uint16((packetIndex >> 32) & 0xffff)
	lo32 := uint32(packetIndex & 0xffff_ffff)
	iv[8] ^= byte(hi16 >> 8)
	iv[9] ^= byte(hi16)
	iv[10] ^= byte(lo32 >> 24)
	iv[11] ^= byte(lo32 >> 16)
	iv[12] ^= byte(lo32 >> 8)
	iv[13] ^= byte(lo32)
	return iv
}

// CryptPayload AES-128-CTR encrypts/decrypts an RTP payload (the cipher is symmetric).
func CryptPayload(keys *E2eSrtpKeys, ssrc uint32, seq uint16, roc uint32, payload []byte, log ...zerolog.Logger) ([]byte, error) {
	// Source of truth: https://github.com/oxidezap/whatsapp-rust/blob/41095d4e6ba4610e054e9ede3af1d5e88a83faee/wacore/src/voip/e2e_srtp.rs#L95-L101
	lg := pickLog(log)
	iv := BuildE2eRtpIV(keys.Salt[:], ssrc, roc, seq)
	block, err := aes.NewCipher(keys.CipherKey[:])
	if err != nil {
		lg.Debug().Err(err).Uint32("ssrc", ssrc).Msg("e2e cipher init failed")
		return nil, err
	}
	out := append([]byte(nil), payload...)
	cipher.NewCTR(block, iv[:]).XORKeyStream(out, out)
	lg.Trace().Uint32("ssrc", ssrc).Uint16("seq", seq).Uint32("roc", roc).Int("payload_bytes", len(payload)).Msg("e2e crypt payload")
	return out, nil
}

// ProtectSrtcp encrypts and authenticates one RTCP packet.
func ProtectSrtcp(keys *E2eSrtpKeys, senderSsrc, index uint32, rtcp []byte) ([]byte, error) {
	split := min(len(rtcp), rtcpHeaderLen)
	out := append([]byte(nil), rtcp[:split]...)
	body, err := CryptPayload(keys, senderSsrc, uint16(index), index>>16, rtcp[split:])
	if err != nil {
		return nil, err
	}
	out = append(out, body...)
	var indexWord [4]byte
	indexWord[0] = byte((0x80000000 | index) >> 24)
	indexWord[1] = byte((0x80000000 | index) >> 16)
	indexWord[2] = byte((0x80000000 | index) >> 8)
	indexWord[3] = byte(0x80000000 | index)
	out = append(out, indexWord[:]...)
	mac := hmac.New(sha1.New, keys.AuthKey[:])
	_, _ = mac.Write(out)
	out = append(out, mac.Sum(nil)[:SrtcpAuthTagLen]...)
	return out, nil
}

// UnprotectSrtcp authenticates and decrypts one SRTCP packet.
func UnprotectSrtcp(keys *E2eSrtpKeys, senderSsrc uint32, packet []byte) ([]byte, uint32, bool) {
	if len(packet) < rtcpHeaderLen+SrtcpTrailerLen {
		return nil, 0, false
	}
	tagStart := len(packet) - SrtcpAuthTagLen
	mac := hmac.New(sha1.New, keys.AuthKey[:])
	_, _ = mac.Write(packet[:tagStart])
	if !hmac.Equal(packet[tagStart:], mac.Sum(nil)[:SrtcpAuthTagLen]) {
		return nil, 0, false
	}
	indexStart := tagStart - 4
	index := uint32(packet[indexStart])<<24 | uint32(packet[indexStart+1])<<16 |
		uint32(packet[indexStart+2])<<8 | uint32(packet[indexStart+3])
	index &= 0x7fffffff
	body, err := CryptPayload(keys, senderSsrc, uint16(index), index>>16, packet[rtcpHeaderLen:indexStart])
	if err != nil {
		return nil, 0, false
	}
	out := append([]byte(nil), packet[:rtcpHeaderLen]...)
	out = append(out, body...)
	return out, index, true
}

// RocTracker is the send-side ROC tracker for monotonic 16-bit sequence numbers.
type RocTracker struct {
	roc         uint32
	lastSeq     uint16
	initialized bool
}

// Advance folds seq into the tracker and returns the current ROC, bumping it on the
// 0xFFFF->0x0000 wrap (a signed 16-bit gap below -32768).
func (t *RocTracker) Advance(seq uint16, log ...zerolog.Logger) uint32 {
	// Source of truth: https://github.com/oxidezap/whatsapp-rust/blob/41095d4e6ba4610e054e9ede3af1d5e88a83faee/wacore/src/voip/e2e_srtp.rs#L112-L124
	lg := pickLog(log)
	if !t.initialized {
		t.lastSeq = seq
		t.initialized = true
		lg.Debug().Uint16("seq", seq).Uint32("roc", t.roc).Msg("send roc tracker seeded")
		return t.roc
	}
	if int32(seq)-int32(t.lastSeq) < -32768 {
		t.roc++
		lg.Debug().Uint16("seq", seq).Uint16("last_seq", t.lastSeq).Uint32("roc", t.roc).Msg("send roc wrapped")
	}
	t.lastSeq = seq
	return t.roc
}

// RecvRocTracker is the recv-side ROC estimator (RFC 3711 guess-index): it tolerates
// reorder/loss by guessing each packet's ROC from the highest seq seen.
type RecvRocTracker struct {
	roc         uint32
	sL          uint16
	initialized bool
}

// EstimateRoc estimates the ROC for seq without mutating receive state.
func (t *RecvRocTracker) EstimateRoc(seq uint16, log ...zerolog.Logger) uint32 {
	// Source of truth: https://github.com/oxidezap/whatsapp-rust/blob/2f001b5a3d6374cc5cf7177792c2a81f87a54080/wacore/src/voip/e2e_srtp.rs#L148-L168
	lg := pickLog(log)
	if !t.initialized {
		return t.roc
	}
	if t.sL < 0x8000 {
		if int32(seq)-int32(t.sL) > 0x8000 {
			return t.roc - 1
		}
		return t.roc
	}
	if int32(t.sL)-int32(seq) > 0x8000 {
		return t.roc + 1
	}
	lg.Trace().Uint16("seq", seq).Uint32("estimated_roc", t.roc).Msg("recv roc estimated")
	return t.roc
}

// CommitRoc folds an authenticated packet's estimated ROC and sequence into state.
func (t *RecvRocTracker) CommitRoc(v uint32, seq uint16, log ...zerolog.Logger) {
	// Source of truth: https://github.com/oxidezap/whatsapp-rust/blob/2f001b5a3d6374cc5cf7177792c2a81f87a54080/wacore/src/voip/e2e_srtp.rs#L170-L188
	lg := pickLog(log)
	if !t.initialized {
		t.sL = seq
		t.initialized = true
		lg.Debug().Uint16("seq", seq).Uint32("roc", t.roc).Msg("recv roc tracker seeded")
		return
	}
	switch v {
	case t.roc:
		if seq > t.sL {
			t.sL = seq
		}
	case t.roc + 1:
		t.roc = v
		t.sL = seq
		lg.Debug().Uint16("seq", seq).Uint32("roc", t.roc).Msg("recv roc advanced")
	}
	lg.Trace().Uint16("seq", seq).Uint32("committed_roc", v).Uint32("roc", t.roc).Msg("recv roc committed")
}

// GuessRoc estimates and commits seq in one step.
func (t *RecvRocTracker) GuessRoc(seq uint16, log ...zerolog.Logger) uint32 {
	// Source of truth: https://github.com/oxidezap/whatsapp-rust/blob/2f001b5a3d6374cc5cf7177792c2a81f87a54080/wacore/src/voip/e2e_srtp.rs#L190-L198
	v := t.EstimateRoc(seq, log...)
	t.CommitRoc(v, seq, log...)
	return v
}
