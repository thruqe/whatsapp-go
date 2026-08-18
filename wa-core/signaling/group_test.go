package signaling

import (
	"bytes"
	"testing"

	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/types"
)

func TestInitialAndInviteGroupOfferShapes(t *testing.T) {
	self := types.JID{User: "100001", Server: types.HiddenUserServer, Device: 1}
	first := types.NewJID("200002", types.HiddenUserServer)
	second := types.NewJID("300003", types.HiddenUserServer)
	participants := []GroupCallParticipant{
		{JID: self.ToNonAD(), Devices: []GroupCallDevice{{JID: self, CapabilityVersion: 1, Capability: CapabilityOffer}}},
		{JID: first, Devices: []GroupCallDevice{{JID: types.JID{User: first.User, Server: first.Server, Device: 2}, CapabilityVersion: 1, Capability: CapabilityOffer}}},
		{JID: second, Devices: []GroupCallDevice{{JID: types.JID{User: second.User, Server: second.Server, Device: 3}, CapabilityVersion: 1, Capability: CapabilityOffer}}},
	}

	initial, err := BuildInitialGroupOffer(InitialGroupOfferParams{
		CallID: "CID", CallCreator: self, Participants: participants, Video: true,
	})
	if err != nil {
		t.Fatalf("BuildInitialGroupOffer: %v", err)
	}
	if got := childTags(t, initial); !eqTags(got, []string{"audio", "audio", "video", "net", "group_info"}) {
		t.Fatalf("initial children = %v", got)
	}
	if to, _ := initial.Attrs["to"].(types.JID); to != types.NewJID("CID", "call") {
		t.Fatalf("initial to = %s", to)
	}
	groupInfo, ok := getChild(t, contentNodes(t, initial)[0], "group_info")
	if !ok || len(contentNodes(t, groupInfo)) != 3 {
		t.Fatalf("initial group_info = %+v", groupInfo)
	}
	selfCapability, _ := getChild(t, contentNodes(t, contentNodes(t, groupInfo)[0])[0], "capability")
	if !bytes.Equal(nodeBytes(&selfCapability), CapabilityVideoOffer) {
		t.Fatalf("creator video capability = %x", nodeBytes(&selfCapability))
	}

	targetDevice := types.JID{User: "400004", Server: types.HiddenUserServer, Device: 4}
	invite, err := BuildGroupInviteOffer(GroupInviteOfferParams{
		CallID: "CID", To: targetDevice.ToNonAD(), CallCreator: self,
		TargetDevices: []types.JID{targetDevice}, Participants: participants[:2],
	})
	if err != nil {
		t.Fatalf("BuildGroupInviteOffer: %v", err)
	}
	if got := childTags(t, invite); !eqTags(got, []string{"audio", "net", "destination", "group_info"}) {
		t.Fatalf("invite children = %v", got)
	}
}

func TestActiveGroupInviteResponsesUseCallScope(t *testing.T) {
	creator := types.JID{User: "100001", Server: types.HiddenUserServer, Device: 1}
	preaccept, err := BuildActiveGroupPreaccept("CID", creator, "PRE", true)
	if err != nil {
		t.Fatalf("BuildActiveGroupPreaccept: %v", err)
	}
	if to, _ := preaccept.Attrs["to"].(types.JID); to != types.NewJID("CID", "call") {
		t.Fatalf("preaccept to = %s", to)
	}
	if got := childTags(t, preaccept); !eqTags(got, []string{"audio", "video", "encopt", "capability"}) {
		t.Fatalf("preaccept children = %v", got)
	}

	accept, err := BuildActiveGroupAccept("CID", creator, "ACCEPT")
	if err != nil {
		t.Fatalf("BuildActiveGroupAccept: %v", err)
	}
	if to, _ := accept.Attrs["to"].(types.JID); to != types.NewJID("CID", "call") {
		t.Fatalf("accept to = %s", to)
	}
	if got := childTags(t, accept); !eqTags(got, []string{"audio", "net", "encopt"}) {
		t.Fatalf("accept children = %v", got)
	}
	if _, ok := getChild(t, contentNodes(t, accept)[0], "group_info"); ok {
		t.Fatal("active group accept must not repeat group_info")
	}
}

func TestParseActiveGroupInviteSnapshot(t *testing.T) {
	creator := types.JID{User: "100001", Server: types.HiddenUserServer, Device: 1}
	user := types.NewJID("200002", types.HiddenUserServer)
	offer := waBinary.Node{
		Tag: "offer",
		Attrs: waBinary.Attrs{
			"call-id": "CID", "call-creator": creator, "joinable": "1",
		},
		Content: []waBinary.Node{{
			Tag: "group_info",
			Attrs: waBinary.Attrs{
				"call-id": "CID", "call-creator": creator, "transaction-id": "7",
				"media": "audio", "connected-limit": "8",
			},
			Content: []waBinary.Node{{
				Tag: "user", Attrs: waBinary.Attrs{"jid": user, "state": "connected"},
				Content: []waBinary.Node{{
					Tag: "device", Attrs: waBinary.Attrs{
						"jid": types.JID{User: user.User, Server: user.Server, Device: 2},
					},
				}},
			}},
		}},
	}
	update, isGroup, err := ParseGroupInviteSnapshot(&offer)
	if err != nil {
		t.Fatalf("ParseGroupInviteSnapshot: %v", err)
	}
	if !isGroup || update == nil || update.CallID != "CID" || update.TransactionID != 7 ||
		!update.Joinable || len(update.Participants) != 1 {
		t.Fatalf("group invite = present %t, update %+v", isGroup, update)
	}
}

func TestParseGroupUpdatePreservesRosterRelayAndRekeyDirective(t *testing.T) {
	creator := types.JID{User: "100001", Server: types.HiddenUserServer, Device: 1}
	user := types.NewJID("200002", types.HiddenUserServer)
	device := types.JID{User: user.User, Server: user.Server, Device: 2}
	action := waBinary.Node{
		Tag: "group_update",
		Attrs: waBinary.Attrs{
			"call-id": "CID", "call-creator": creator,
		},
		Content: []waBinary.Node{
			{
				Tag: "group_info",
				Attrs: waBinary.Attrs{
					"transaction-id": "7", "media": "video", "connected-limit": "8",
					"joinable": "1", "rekey": "1",
				},
				Content: []waBinary.Node{{
					Tag: "user", Attrs: waBinary.Attrs{"jid": user, "state": "connected"},
					Content: []waBinary.Node{{
						Tag: "device", Attrs: waBinary.Attrs{"jid": device, "pid": "22", "platform": "web"},
					}},
				}},
			},
			{
				Tag: "relay",
				Attrs: waBinary.Attrs{
					"transaction-id": "7", "self_pid": "11", "uuid": "relay-uuid",
					"participant_uuid": "participant-uuid", "warp_mi_tag_len": "8",
				},
				Content: []waBinary.Node{
					{Tag: "key", Content: []byte("relay-key")},
					{Tag: "hbh_key", Content: []byte("hbh-key")},
					{Tag: "token", Attrs: waBinary.Attrs{"id": "0"}, Content: []byte("token")},
					{Tag: "auth_token", Attrs: waBinary.Attrs{"id": "0"}, Content: []byte("auth")},
					{Tag: "te2", Attrs: waBinary.Attrs{
						"relay_id": "3", "token_id": "0", "auth_token_id": "0",
						"relay_name": "zrh1c01", "c2r_rtt": "24", "is_fna": "0",
					}, Content: []byte{1, 2, 3, 4, 0x1f, 0x90}},
				},
			},
		},
	}
	update, err := ParseGroupUpdate(&action)
	if err != nil {
		t.Fatalf("ParseGroupUpdate: %v", err)
	}
	if update.TransactionID != 7 || update.Media != "video" || !update.Joinable || !update.RekeyRequested {
		t.Fatalf("group state = %+v", update)
	}
	if len(update.Participants) != 1 || len(update.Participants[0].Devices) != 1 ||
		update.Participants[0].Devices[0].PID != 22 {
		t.Fatalf("participants = %+v", update.Participants)
	}
	if update.Relay == nil || update.Relay.SelfPID != 11 || len(update.Relay.Endpoints) != 1 ||
		update.Relay.Endpoints[0].RelayID != 3 || update.Relay.Endpoints[0].IPv4 != "1.2.3.4" ||
		update.Relay.Endpoints[0].Port != 8080 {
		t.Fatalf("relay = %+v", update.Relay)
	}
	if !bytes.Equal(update.Relay.Key, []byte("relay-key")) ||
		!bytes.Equal(update.Relay.Tokens[0], []byte("token")) {
		t.Fatal("relay secrets were not preserved")
	}
}

func TestGroupRekeyBuildAndParse(t *testing.T) {
	creator := types.JID{User: "100001", Server: types.HiddenUserServer, Device: 1}
	target := types.JID{User: "200002", Server: types.HiddenUserServer, Device: 2}
	call, err := BuildGroupEncRekey(GroupEncRekeyParams{
		CallID: "CID", To: target, CallCreator: creator, TransactionID: 9, RequestID: "REQ",
		DeviceKey: OfferDeviceKey{DeviceJid: target, EncType: "msg", Ciphertext: []byte{1, 2, 3}},
	})
	if err != nil {
		t.Fatalf("BuildGroupEncRekey: %v", err)
	}
	action := contentNodes(t, call)[0]
	rekey, err := ParseGroupCallEncRekey(&action)
	if err != nil {
		t.Fatalf("ParseGroupCallEncRekey: %v", err)
	}
	if rekey.CallID != "CID" || rekey.TransactionID != 9 || rekey.KeyGeneration != 2 ||
		rekey.EncryptionType != "msg" || !bytes.Equal(rekey.Ciphertext, []byte{1, 2, 3}) {
		t.Fatalf("rekey = %+v", rekey)
	}
}

func TestParseCallControlEnvelopePreservesCompanionRouting(t *testing.T) {
	from := types.JID{User: "100001", Server: types.HiddenUserServer, Device: 1}
	participant := types.JID{User: "100001", Server: types.HiddenUserServer, Device: 2}
	recipient := types.JID{User: "200002", Server: types.HiddenUserServer, Device: 3}
	creator := types.JID{User: "300003", Server: types.HiddenUserServer, Device: 4}
	node := waBinary.Node{
		Tag: "call",
		Attrs: waBinary.Attrs{
			"from": from, "participant": participant, "recipient": recipient, "t": "1700000000",
		},
		Content: []waBinary.Node{{
			Tag: "group_update", Attrs: waBinary.Attrs{"call-id": "CID", "call-creator": creator},
		}},
	}
	envelope, err := ParseCallControlEnvelope(&node)
	if err != nil {
		t.Fatalf("ParseCallControlEnvelope: %v", err)
	}
	if envelope.From != from || envelope.Participant != participant || envelope.Recipient != recipient ||
		envelope.CallID != "CID" || envelope.CallCreator != creator || envelope.Action.Tag != "group_update" {
		t.Fatalf("envelope = %+v", envelope)
	}
}

func TestParseCallControlEnvelopeAllowsRekeyWithoutTimestamp(t *testing.T) {
	from := types.JID{User: "100001", Server: types.HiddenUserServer}
	creator := types.JID{User: "300003", Server: types.HiddenUserServer, Device: 4}
	node := waBinary.Node{
		Tag:   "call",
		Attrs: waBinary.Attrs{"from": from, "id": "REKEY"},
		Content: []waBinary.Node{{
			Tag: "enc_rekey",
			Attrs: waBinary.Attrs{
				"call-id": "CID", "call-creator": creator, "transaction-id": "4",
			},
		}},
	}

	envelope, err := ParseCallControlEnvelope(&node)
	if err != nil {
		t.Fatalf("ParseCallControlEnvelope: %v", err)
	}
	if envelope.From != from || envelope.CallID != "CID" ||
		envelope.CallCreator != creator || !envelope.Timestamp.IsZero() {
		t.Fatalf("envelope = %+v", envelope)
	}
}
