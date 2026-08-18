package whatsmeow

import (
	"testing"

	"go.mau.fi/whatsmeow/signaling"
)

// TestSelectAudioCodec pins the codec selection: only an explicit, present
// use_mlow_codec_v1=false picks Opus; everything else (nil, absent, true) stays
// on MLow. Skipped until selectAudioCodec lands.
func TestSelectAudioCodec(t *testing.T) {
	cases := []struct {
		name string
		vs   *signaling.VoipSettings
		want AudioCodec
	}{
		{"nil keeps mlow", nil, AudioCodecMlow},
		{"absent keeps mlow", &signaling.VoipSettings{Present: false, UseMlowCodecV1: false}, AudioCodecMlow},
		{"mlow true", &signaling.VoipSettings{Present: true, UseMlowCodecV1: true}, AudioCodecMlow},
		{"mlow false -> opus", &signaling.VoipSettings{Present: true, UseMlowCodecV1: false}, AudioCodecOpus},
	}
	for _, c := range cases {
		if got := selectAudioCodec(c.vs); got != c.want {
			t.Errorf("%s: selectAudioCodec = %v, want %v", c.name, got, c.want)
		}
	}
}
