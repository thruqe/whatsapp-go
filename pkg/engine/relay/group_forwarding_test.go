package relay

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func TestUnwrapGroupForwardingPacketCaptureVectors(t *testing.T) {
	// Source of truth: https://github.com/purpshell/wacaller/blob/4db38f0ea0645ac8449a105ffd9aa30c6e269689/diag/analysis/group-call-84987F9DE404B79ED999E6F254B0150A.md#L51-L70
	vectors := []struct {
		name      string
		packetHex string
		innerHex  string
	}{
		{
			name:      "subtype_2_video",
			packetHex: "0902336e0100018890e1001300120c96772e6aa9",
			innerHex:  "90e1001300120c96772e6aa9",
		},
		{
			name:      "subtype_4_video",
			packetHex: "09047eb5410001000000001090e100010013fbf08f1df407",
			innerHex:  "90e100010013fbf08f1df407",
		},
		{
			name:      "subtype_7_audio",
			packetHex: "0907338f2900020a00c801e000000000000090780001000331808cd481d5",
			innerHex:  "90780001000331808cd481d5",
		},
	}
	for _, vector := range vectors {
		t.Run(vector.name, func(t *testing.T) {
			packet, err := hex.DecodeString(vector.packetHex)
			if err != nil {
				t.Fatalf("decode packet: %v", err)
			}
			want, err := hex.DecodeString(vector.innerHex)
			if err != nil {
				t.Fatalf("decode inner packet: %v", err)
			}
			got, wrapped, valid := UnwrapGroupForwardingPacket(packet)
			if !wrapped {
				t.Fatal("capture packet was not recognized as group-forwarded")
			}
			if !valid {
				t.Fatal("capture packet was rejected")
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("inner packet = %x, want %x", got, want)
			}
			if kind := ClassifyRelayPacket(got); kind != RelayPacketRtp {
				t.Fatalf("inner packet kind = %d, want RTP", kind)
			}
		})
	}
}

func TestUnwrapGroupForwardingPacketPassThrough(t *testing.T) {
	// Source of truth: https://github.com/purpshell/wacaller/blob/4db38f0ea0645ac8449a105ffd9aa30c6e269689/diag/analysis/group-call-84987F9DE404B79ED999E6F254B0150A.md#L93-L100
	packet := []byte{0x90, 0xe1, 0, 1, 0, 0, 0, 1, 0, 0, 0, 2}
	got, wrapped, valid := UnwrapGroupForwardingPacket(packet)
	if wrapped {
		t.Fatal("ordinary RTP was reported as group-forwarded")
	}
	if !valid {
		t.Fatal("ordinary RTP was rejected")
	}
	if !bytes.Equal(got, packet) {
		t.Fatalf("ordinary RTP changed: %x", got)
	}
}

func TestUnwrapGroupForwardingPacketRejectsMalformed(t *testing.T) {
	// Source of truth: https://github.com/purpshell/wacaller/blob/4db38f0ea0645ac8449a105ffd9aa30c6e269689/diag/analysis/group-call-84987F9DE404B79ED999E6F254B0150A.md#L93-L100
	cases := [][]byte{
		{0x09},
		{0x09, 0xff},
		{0x09, 0x02, 0, 0, 0, 0, 0, 0},
		{0x09, 0x02, 0, 0, 0, 0, 0, 0, 0x10, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
	}
	for _, packet := range cases {
		got, wrapped, valid := UnwrapGroupForwardingPacket(packet)
		if !wrapped {
			t.Errorf("malformed packet %x was not recognized as group-forwarded", packet)
		}
		if valid {
			t.Errorf("malformed packet %x was accepted", packet)
		}
		if got != nil {
			t.Errorf("malformed packet %x returned payload %x", packet, got)
		}
	}
}
