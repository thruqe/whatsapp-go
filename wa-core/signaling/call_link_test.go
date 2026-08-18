package signaling

import (
	"testing"

	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/types"
)

func TestCallLinkBuildersMatchCapturedRoutes(t *testing.T) {
	create, err := BuildCallLinkCreate(CallLinkMediaVideo, "REQ1")
	if err != nil {
		t.Fatalf("BuildCallLinkCreate: %v", err)
	}
	if to, _ := create.Attrs["to"].(types.JID); to != types.NewJID("", "call") {
		t.Fatalf("create to = %s", to)
	}
	action := contentNodes(t, create)[0]
	if action.Tag != "link_create" || action.Attrs["media"] != "video" {
		t.Fatalf("create action = %+v", action)
	}

	join, err := BuildCallLinkJoin("TOKEN", CallLinkMediaVideo, "REQ2")
	if err != nil {
		t.Fatalf("BuildCallLinkJoin: %v", err)
	}
	if got := childTags(t, join); !eqTags(got, []string{"audio", "video", "net", "capability"}) {
		t.Fatalf("join children = %v", got)
	}

	creator := types.JID{User: "100001", Server: types.HiddenUserServer, Device: 1}
	user := types.NewJID("200002", types.HiddenUserServer)
	for name, node := range map[string]waBinary.Node{
		"toggle": BuildWaitingRoomToggle("CID", creator, true, "REQ3"),
		"admit":  BuildWaitingRoomAdmit("CID", creator, user, "REQ4"),
		"deny":   BuildWaitingRoomDeny("CID", creator, user, "REQ5"),
	} {
		t.Run(name, func(t *testing.T) {
			if to, _ := node.Attrs["to"].(types.JID); to != types.NewJID("CID", "call") {
				t.Fatalf("to = %s", to)
			}
		})
	}
}

func TestCallLinkParsersDistinguishWaitingAndAdmitted(t *testing.T) {
	creator := types.JID{User: "100001", Server: types.HiddenUserServer, Device: 1}
	waiting := waBinary.Node{
		Tag: "ack", Attrs: waBinary.Attrs{"class": "call", "type": "link_join"},
		Content: []waBinary.Node{{
			Tag: "waiting_room",
			Attrs: waBinary.Attrs{
				"call-id": "CID", "call-creator": creator, "link-token": "TOKEN",
				"media": "video", "enabled": "1", "is_admin": "0", "transaction-id": "4",
			},
		}},
	}
	result, err := ParseCallLinkJoinAck(&waiting)
	if err != nil {
		t.Fatalf("ParseCallLinkJoinAck(waiting): %v", err)
	}
	if !result.InWaitingRoom || result.Group != nil || result.CallID != "CID" {
		t.Fatalf("waiting result = %+v", result)
	}

	admitted := waBinary.Node{
		Tag: "ack", Attrs: waBinary.Attrs{"class": "call", "type": "link_join"},
		Content: []waBinary.Node{{
			Tag: "group_info",
			Attrs: waBinary.Attrs{
				"call-id": "CID", "call-creator": creator, "transaction-id": "5",
				"media": "video", "connected-limit": "8",
			},
		}},
	}
	result, err = ParseCallLinkJoinAck(&admitted)
	if err != nil {
		t.Fatalf("ParseCallLinkJoinAck(admitted): %v", err)
	}
	if result.InWaitingRoom || result.Group == nil || result.Group.TransactionID != 5 {
		t.Fatalf("admitted result = %+v", result)
	}
}

func TestWaitingRoomUpdateAndAck(t *testing.T) {
	creator := types.JID{User: "100001", Server: types.HiddenUserServer, Device: 1}
	user := types.NewJID("200002", types.HiddenUserServer)
	action := waBinary.Node{
		Tag:   "waiting_room_update",
		Attrs: waBinary.Attrs{"call-id": "CID", "call-creator": creator},
		Content: []waBinary.Node{{
			Tag: "waiting_room",
			Attrs: waBinary.Attrs{
				"call-id": "CID", "call-creator": creator, "link-token": "TOKEN",
				"media": "audio", "enabled": "1", "is_admin": "1", "transaction-id": "7",
			},
			Content: []waBinary.Node{{Tag: "user", Attrs: waBinary.Attrs{
				"jid": user, "state": "pending",
			}}},
		}},
	}
	room, err := ParseWaitingRoomUpdate(&action)
	if err != nil {
		t.Fatalf("ParseWaitingRoomUpdate: %v", err)
	}
	if !room.Enabled || !room.IsAdmin || room.TransactionID != 7 || len(room.Users) != 1 {
		t.Fatalf("waiting room = %+v", room)
	}
	envelope := waBinary.Node{
		Tag: "call", Attrs: waBinary.Attrs{"id": "REQ", "from": creator},
		Content: []waBinary.Node{action},
	}
	ack, err := BuildWaitingRoomUpdateAck(&envelope)
	if err != nil {
		t.Fatalf("BuildWaitingRoomUpdateAck: %v", err)
	}
	if ack.Attrs["class"] != "call" || ack.Attrs["type"] != "waiting_room_update" {
		t.Fatalf("ACK = %+v", ack)
	}
}
