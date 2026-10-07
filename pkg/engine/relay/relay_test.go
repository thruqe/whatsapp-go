package relay

import "testing"

// TestClassifyFirstByte pins every branch of the first-byte relay packet demux
// against the reference's inline assertions.
func TestClassifyFirstByte(t *testing.T) {
	cases := []struct {
		data []byte
		want RelayPacketKind
	}{
		{[]byte{0x00, 0x01}, RelayPacketStun},
		{[]byte{0x00, 0x03}, RelayPacketStun},
		{[]byte{0x80, 0xc8}, RelayPacketRtcp},
		{[]byte{0x81, 0xc8}, RelayPacketRtcp},
		{[]byte{0x91, 0xce}, RelayPacketRtcp},
		{[]byte{0x90, 0x78}, RelayPacketRtp},
		{[]byte{0x90, 0xe1}, RelayPacketRtp},
		{[]byte{0xff, 0xff}, RelayPacketOther},
		{[]byte{0x00}, RelayPacketOther},
	}
	for _, c := range cases {
		if got := ClassifyRelayPacket(c.data); got != c.want {
			t.Errorf("ClassifyRelayPacket(%x) = %d, want %d", c.data, got, c.want)
		}
	}
}
