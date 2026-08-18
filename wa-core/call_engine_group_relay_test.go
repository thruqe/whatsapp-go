package whatsmeow

import (
	"bytes"
	"errors"
	"testing"

	"go.mau.fi/whatsmeow/rtp"
	"go.mau.fi/whatsmeow/stun"
	"go.mau.fi/whatsmeow/types"
)

func TestGroupRelayDataSelectsCaptureAddressAndCredentials(t *testing.T) {
	update := groupCallUpdate{Relay: &groupCallRelay{
		Key:    bytes.Repeat([]byte{0x21}, 16),
		Tokens: [][]byte{bytes.Repeat([]byte{0x42}, 174)},
		Endpoints: []groupCallRelayEndpoint{{
			RelayID: 7, RelayName: "zrh1c01", TokenID: 0,
			IPv4: "157.240.17.62", Port: 3478,
		}},
	}}
	rd, err := groupRelayData(update, false)
	if err != nil {
		t.Fatalf("groupRelayData: %v", err)
	}
	endpoint := getMediaRelayEndpoint(rd, false)
	if endpoint == nil || endpoint.relayName != "zrh1c01" ||
		endpoint.addresses[0].ipv4 != "157.240.17.62" ||
		endpoint.addresses[0].port != 3478 {
		t.Fatalf("selected endpoint = %+v", endpoint)
	}
	if !bytes.Equal(rd.relayKeyASCII, update.Relay.Key) ||
		!bytes.Equal(rd.relayTokens[0], update.Relay.Tokens[0]) {
		t.Fatal("group relay credentials were not copied")
	}
	rd.relayKeyASCII[0] ^= 0xff
	if bytes.Equal(rd.relayKeyASCII, update.Relay.Key) {
		t.Fatal("relay key aliases the signaling snapshot")
	}
}

func TestGroupRelayAllocateStateCarriesParticipantSubscriptions(t *testing.T) {
	initial := []byte{0x00, 0x01, 0x02}
	key := bytes.Repeat([]byte{0x24}, 16)
	token := bytes.Repeat([]byte{0x42}, 174)
	hbhFEC := [2]uint32{0x11223344, 0x55667788}
	state := newGroupRelayAllocateStateWithHBHFEC(initial, key, hbhFEC)
	endpoint := &relayEndpoint{
		relayName: "zrh1c01",
		addresses: []relayAddress{{ipv4: "157.240.17.62", port: 3478}},
	}
	relayUpdate := &groupCallRelay{
		TransactionID: 4, Key: key, Tokens: [][]byte{token},
		Endpoints: []groupCallRelayEndpoint{{RelayName: "zrh1c01", TokenID: 0}},
	}
	streamSSRCs := [9]uint32{1, 2, 3, 4, 5, 6, 7, 8, 9}
	appDataSSRC := uint32(10)
	pids := []uint32{31, 47}
	transactionID := [12]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}
	var sent []byte
	changed, err := state.ApplyWithSubscriptions(
		endpoint,
		relayUpdate,
		streamSSRCs,
		appDataSSRC,
		pids,
		transactionID,
		func(packet []byte) error {
			sent = bytes.Clone(packet)
			return nil
		},
	)
	if err != nil || !changed {
		t.Fatalf("ApplyWithSubscriptions = (%v, %v), want (true, nil)", changed, err)
	}
	endpointXOR, _ := stun.EncodeXorRelayEndpoint("157.240.17.62", 3478)
	want := stun.BuildWasmStunAllocateRequestWithGroupSubscriptionsAndHBHFEC(
		transactionID, token, endpointXOR, streamSSRCs, appDataSSRC, hbhFEC, pids, key,
	)
	if !bytes.Equal(sent, want) {
		t.Fatalf("group allocate = %x, want %x", sent, want)
	}
}

func TestGroupRelayAllocateSendFailureDoesNotCommitRotation(t *testing.T) {
	initial := []byte{0x00, 0x01, 0x02}
	initialKey := bytes.Repeat([]byte{0x12}, 16)
	state := newGroupRelayAllocateStateWithHBHFEC(initial, initialKey, [2]uint32{})
	endpoint := &relayEndpoint{
		relayName: "zrh1c01",
		addresses: []relayAddress{{ipv4: "157.240.17.62", port: 3478}},
	}
	relayUpdate := &groupCallRelay{
		TransactionID: 1,
		Key:           bytes.Repeat([]byte{0x24}, 16),
		Tokens:        [][]byte{bytes.Repeat([]byte{0x42}, 174)},
		Endpoints:     []groupCallRelayEndpoint{{RelayName: "zrh1c01", TokenID: 0}},
	}
	sendErr := errors.New("relay unavailable")
	changed, err := state.ApplyWithSubscriptions(
		endpoint, relayUpdate, [9]uint32{}, 0, nil, [12]byte{},
		func([]byte) error { return sendErr },
	)
	if !errors.Is(err, sendErr) || changed {
		t.Fatalf("failed ApplyWithSubscriptions = (%v, %v)", changed, err)
	}
	if got := state.Current(); !bytes.Equal(got, initial) {
		t.Fatalf("current allocate after failure = %x, want %x", got, initial)
	}
}

func TestConnectedRemoteParticipantPIDsExcludesSelfAndInactive(t *testing.T) {
	self := types.JID{User: "100", Server: types.HiddenUserServer, Device: 1}
	remote := types.JID{User: "200", Server: types.HiddenUserServer, Device: 2}
	inactive := types.JID{User: "300", Server: types.HiddenUserServer, Device: 3}
	update := groupCallUpdate{Participants: []groupCallParticipant{
		{State: "connected", Devices: []groupCallDevice{{JID: self, PID: 11, HasPID: true}}},
		{State: "connected", Devices: []groupCallDevice{{JID: remote, PID: 22, HasPID: true}}},
		{State: "ringing", Devices: []groupCallDevice{{JID: inactive, PID: 33, HasPID: true}}},
	}}
	got := connectedRemoteParticipantPIDs(update, rtp.FormatE2ESrtpParticipantID(self.String()))
	if len(got) != 1 || got[0] != 22 {
		t.Fatalf("connected remote PIDs = %v, want [22]", got)
	}
}
