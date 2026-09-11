package signaling

import (
	"os"
	"testing"
)

// TestParseVoipSettings is the KAT for the voip_settings parser, wired to the
// captured sample blob (use_mlow_codec_v1="true", frame_ms="60",
// target_bitrate="24000"). Skipped until ParseVoipSettings lands.
func TestParseVoipSettings(t *testing.T) {
	raw, err := os.ReadFile("testdata/voip_settings_sample.json")
	if err != nil {
		t.Fatal(err)
	}
	vs, err := ParseVoipSettings(raw)
	if err != nil {
		t.Fatalf("ParseVoipSettings: %v", err)
	}
	if !vs.Present {
		t.Error("Present = false, want true")
	}
	if !vs.UseMlowCodecV1 {
		t.Error("UseMlowCodecV1 = false, want true (sample sets it true)")
	}
	if vs.FrameMs != 60 {
		t.Errorf("FrameMs = %d, want 60", vs.FrameMs)
	}
	if vs.TargetBitrate != 24000 {
		t.Errorf("TargetBitrate = %d, want 24000", vs.TargetBitrate)
	}
}

// TestParseVoipSettingsOpus pins the codec lever: use_mlow_codec_v1="false" must
// parse to UseMlowCodecV1=false. Skipped until ParseVoipSettings lands.
func TestParseVoipSettingsOpus(t *testing.T) {
	vs, err := ParseVoipSettings([]byte(`{"encode":{"use_mlow_codec_v1":"false","frame_ms":"60"}}`))
	if err != nil {
		t.Fatalf("ParseVoipSettings: %v", err)
	}
	if vs.UseMlowCodecV1 {
		t.Error("UseMlowCodecV1 = true, want false")
	}
}
