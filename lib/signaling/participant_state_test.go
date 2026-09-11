package signaling

import (
	"testing"

	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/types"
)

func TestRaiseHandAndScreenShareRoundTrip(t *testing.T) {
	creator := types.JID{User: "100001", Server: types.HiddenUserServer, Device: 1}
	target := types.NewJID("CID", "call")
	raisedNode := BuildRaiseHand("CID", target, creator, "REQ1", true)
	raisedAction := contentNodes(t, raisedNode)[0]
	raised, err := ParseRaiseHand(&raisedAction)
	if err != nil || !raised {
		t.Fatalf("ParseRaiseHand = %t, %v", raised, err)
	}

	screenShareID := uint32(1)
	shareNode := BuildScreenShare("CID", target, creator, "REQ2", ScreenShareStarted, &screenShareID)
	shareAction := contentNodes(t, shareNode)[0]
	share, err := ParseScreenShare(&shareAction)
	if err != nil {
		t.Fatalf("ParseScreenShare: %v", err)
	}
	if share.State != ScreenShareStarted || share.Version != 2 ||
		!share.HasScreenShareID || share.ScreenShareID != 1 {
		t.Fatalf("screen share = %+v", share)
	}
}

func TestCallControlAckPreservesDeviceRouting(t *testing.T) {
	from := types.JID{User: "100001", Server: types.HiddenUserServer, Device: 1}
	participant := types.JID{User: "100001", Server: types.HiddenUserServer, Device: 2}
	recipient := types.JID{User: "200002", Server: types.HiddenUserServer, Device: 3}
	original := waBinary.Node{
		Tag: "call",
		Attrs: waBinary.Attrs{
			"id": "REQ", "from": from, "participant": participant, "recipient": recipient,
		},
	}
	ack, ok := BuildCallControlAck(&original, "screen_share")
	if !ok {
		t.Fatal("BuildCallControlAck returned false")
	}
	if ack.Attrs["to"] != from || ack.Attrs["participant"] != participant ||
		ack.Attrs["recipient"] != recipient || ack.Attrs["type"] != "screen_share" {
		t.Fatalf("ACK = %+v", ack)
	}
}
