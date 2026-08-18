package callutil

import (
	"strings"

	"github.com/rs/zerolog"
)

// FormatParticipantID is the device-qualified participant id used as the HKDF
// info for E2E-SRTP and SFrame: strip the resource, give a bare @lid an implicit
// :0 device suffix, and pass everything else through unchanged.
func FormatParticipantID(jid string, log ...zerolog.Logger) string {
	// Source of truth: https://github.com/oxidezap/whatsapp-rust/blob/41095d4e6ba4610e054e9ede3af1d5e88a83faee/wacore/src/voip/mod.rs#L44-L58
	lg := pickLog(log)
	bare, _, _ := strings.Cut(jid, "/")
	bare = strings.TrimSpace(bare)
	at := strings.LastIndexByte(bare, '@')
	if at <= 0 {
		lg.Trace().Bool("has_domain", false).Msg("formatted participant id (no domain, passthrough)")
		return bare
	}
	user := bare[:at]
	domain := bare[at+1:]
	if domain == "lid" && !strings.Contains(user, ":") {
		lg.Trace().Str("domain", domain).Bool("implicit_device", true).Msg("formatted participant id (added :0 device suffix)")
		return user + ":0@" + domain
	}
	lg.Trace().Str("domain", domain).Bool("implicit_device", false).Msg("formatted participant id (passthrough)")
	return bare
}
