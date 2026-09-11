package srtp

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/binary"

	"github.com/rs/zerolog"
)

// WARP RTP extension constants and the WARP MESSAGE-INTEGRITY tag (HMAC-SHA1
// appended to protected packets).

const WarpExtProfile uint16 = 0xdebe

const (
	WarpMITagLen             = 4
	WarpPiggybackStartPacket = 2
)

// WarpAudioPiggybackExt is the audio piggyback extension word (big-endian bytes).
var WarpAudioPiggybackExt = [4]byte{0x30, 0x01, 0x00, 0x00}

// AudioPiggybackExtensionFor returns the audio piggyback extension word for
// packetIndex, or nil for the first packets / when disabled.
func AudioPiggybackExtensionFor(packetIndex int, enabled bool, startPacket int, log ...zerolog.Logger) *uint32 {
	// Source of truth: https://github.com/oxidezap/whatsapp-rust/blob/41095d4e6ba4610e054e9ede3af1d5e88a83faee/wacore/src/voip/warp.rs#L15-L24
	lg := pickLog(log)
	if !enabled || packetIndex < startPacket {
		lg.Trace().Int("packet_index", packetIndex).Bool("enabled", enabled).Int("start_packet", startPacket).Msg("warp audio piggyback skipped")
		return nil
	}
	w := binary.BigEndian.Uint32(WarpAudioPiggybackExt[:])
	lg.Trace().Int("packet_index", packetIndex).Msg("warp audio piggyback extension attached")
	return &w
}

// ComputeWarpMITag is the WARP MI tag: the first tagLen bytes of
// HMAC-SHA1(authKey, packetWithoutTag || roc_be32).
func ComputeWarpMITag(authKey, packetWithoutTag []byte, roc uint32, tagLen int, log ...zerolog.Logger) []byte {
	// Source of truth: https://github.com/oxidezap/whatsapp-rust/blob/41095d4e6ba4610e054e9ede3af1d5e88a83faee/wacore/src/voip/warp.rs#L27-L38
	lg := pickLog(log)
	mac := hmac.New(sha1.New, authKey)
	mac.Write(packetWithoutTag)
	var rocBE [4]byte
	binary.BigEndian.PutUint32(rocBE[:], roc)
	mac.Write(rocBE[:])
	lg.Trace().Int("packet_bytes", len(packetWithoutTag)).Uint32("roc", roc).Int("tag_len", tagLen).Msg("computed warp mi tag")
	return mac.Sum(nil)[:tagLen]
}

// AppendWarpMITag appends the WARP MI tag to a protected packet.
func AppendWarpMITag(authKey, packetWithoutTag []byte, roc uint32, tagLen int, log ...zerolog.Logger) []byte {
	// Source of truth: https://github.com/oxidezap/whatsapp-rust/blob/41095d4e6ba4610e054e9ede3af1d5e88a83faee/wacore/src/voip/warp.rs#L41-L52
	lg := pickLog(log)
	tag := ComputeWarpMITag(authKey, packetWithoutTag, roc, tagLen, lg)
	out := make([]byte, 0, len(packetWithoutTag)+len(tag))
	out = append(out, packetWithoutTag...)
	out = append(out, tag...)
	lg.Trace().Int("packet_bytes", len(packetWithoutTag)).Uint32("roc", roc).Int("tag_len", tagLen).Int("total_bytes", len(out)).Msg("appended warp mi tag")
	return out
}

// VerifyWarpMITag authenticates a protected WARP packet and its rollover counter.
func VerifyWarpMITag(authKey, packetWithoutTag []byte, roc uint32, tagLen int, receivedTag []byte, log ...zerolog.Logger) bool {
	// Source of truth: https://github.com/oxidezap/whatsapp-rust/blob/2f001b5a3d6374cc5cf7177792c2a81f87a54080/wacore/src/voip/warp.rs#L45-L58
	lg := pickLog(log)
	if tagLen <= 0 || tagLen > sha1.Size || len(receivedTag) != tagLen {
		lg.Debug().Int("tag_len", tagLen).Int("received_tag_len", len(receivedTag)).Msg("WARP MI tag length rejected")
		return false
	}
	expected := ComputeWarpMITag(authKey, packetWithoutTag, roc, tagLen, lg)
	ok := hmac.Equal(expected, receivedTag)
	lg.Trace().Bool("authenticated", ok).Uint32("roc", roc).Int("packet_bytes", len(packetWithoutTag)).Int("tag_len", tagLen).Msg("verified WARP MI tag")
	return ok
}
