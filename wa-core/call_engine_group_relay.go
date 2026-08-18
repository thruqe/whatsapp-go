package whatsmeow

import (
	"bytes"
	"fmt"
	"sync"

	"go.mau.fi/whatsmeow/rtp"
	"go.mau.fi/whatsmeow/stun"
)

type groupRelayAllocateState struct {
	mu            sync.RWMutex
	packet        []byte
	key           []byte
	transactionID uint32
	hasGroup      bool
	hbhFECSSRCs   [2]uint32
}

func newGroupRelayAllocateStateWithHBHFEC(
	initial, initialKey []byte,
	hbhFECSSRCs [2]uint32,
) *groupRelayAllocateState {
	// Source of truth: https://github.com/purpshell/wacaller/blob/89ebce510859a415ef19e9b5d74b0006b60ba634/datasheets/group-video-reactions.md#L47-L53
	return &groupRelayAllocateState{
		packet:      bytes.Clone(initial),
		key:         bytes.Clone(initialKey),
		hbhFECSSRCs: hbhFECSSRCs,
	}
}

func (s *groupRelayAllocateState) Current() []byte {
	// Source of truth: https://github.com/purpshell/wacaller/blob/a9e4195fb846a730f30ce98c26a7d1c03993fdb2/datasheets/group-media-relay-refresh.md#L60
	s.mu.RLock()
	defer s.mu.RUnlock()
	return bytes.Clone(s.packet)
}

func (s *groupRelayAllocateState) SendCurrent(send func([]byte) error) error {
	// Source of truth: https://github.com/purpshell/wacaller/blob/bcfb7f0c076b131422c22f024dfff080448e70f4/datasheets/group-media-relay-refresh.md#L59-L67
	if send == nil {
		return fmt.Errorf("wacaller: relay keepalive send is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return send(s.packet)
}

func (s *groupRelayAllocateState) ApplyWithSubscriptions(
	endpoint *relayEndpoint,
	relayUpdate *groupCallRelay,
	streamSSRCs [9]uint32,
	appDataSSRC uint32,
	participantPIDs []uint32,
	transactionID [12]byte,
	send func([]byte) error,
) (bool, error) {
	// Source of truth: https://github.com/purpshell/wacaller/blob/99134bb900df3ee83a69d9a38112e623817597ae/datasheets/group-video-reactions.md#L36-L50
	if relayUpdate == nil {
		return false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.hasGroup && relayUpdate.TransactionID <= s.transactionID {
		return false, nil
	}
	if endpoint == nil || endpoint.relayName == "" || len(endpoint.addresses) == 0 ||
		endpoint.addresses[0].ipv4 == "" || endpoint.addresses[0].port == 0 {
		return false, fmt.Errorf("wacaller: active relay endpoint is incomplete")
	}
	if len(relayUpdate.Key) == 0 {
		return false, fmt.Errorf("wacaller: group relay has no key")
	}
	if send == nil {
		return false, fmt.Errorf("wacaller: group relay send is nil")
	}

	var matched *groupCallRelayEndpoint
	for i := range relayUpdate.Endpoints {
		candidate := &relayUpdate.Endpoints[i]
		if candidate.RelayName == endpoint.relayName && candidate.IsFNA == endpoint.isFNA {
			matched = candidate
			break
		}
	}
	if matched == nil {
		return false, fmt.Errorf("wacaller: active relay %s missing from group allocation", endpoint.relayName)
	}
	if int(matched.TokenID) >= len(relayUpdate.Tokens) || len(relayUpdate.Tokens[matched.TokenID]) == 0 {
		return false, fmt.Errorf("wacaller: group relay token %d is missing", matched.TokenID)
	}
	endpointXOR, ok := stun.EncodeXorRelayEndpoint(endpoint.addresses[0].ipv4, endpoint.addresses[0].port)
	if !ok {
		return false, fmt.Errorf("wacaller: active relay IPv4 is malformed")
	}
	packet := stun.BuildWasmStunAllocateRequestWithGroupSubscriptionsAndHBHFEC(
		transactionID,
		relayUpdate.Tokens[matched.TokenID],
		endpointXOR,
		streamSSRCs,
		appDataSSRC,
		s.hbhFECSSRCs,
		participantPIDs,
		relayUpdate.Key,
	)
	if err := send(packet); err != nil {
		return false, err
	}
	s.packet = append(s.packet[:0], packet...)
	s.key = append(s.key[:0], relayUpdate.Key...)
	s.transactionID = relayUpdate.TransactionID
	s.hasGroup = true
	return true, nil
}

func buildRelayBindingSuccess(request, integrityKey []byte) ([]byte, bool) {
	// Source of truth: https://github.com/purpshell/wacaller/blob/a9e4195fb846a730f30ce98c26a7d1c03993fdb2/datasheets/group-media-relay-refresh.md#L72-L92
	messageType, ok := stun.StunMessageType(request)
	if !ok || messageType != stun.MsgBindingRequest || len(integrityKey) == 0 {
		return nil, false
	}
	transactionID, ok := stun.StunTransactionID(request)
	if !ok || len(transactionID) != 12 {
		return nil, false
	}
	var transaction [12]byte
	copy(transaction[:], transactionID)
	return stun.EncodeStunRequest(stun.MsgBindingSuccess, transaction, nil, integrityKey, true), true
}

func (s *groupRelayAllocateState) SendBindingSuccess(
	request []byte,
	send func([]byte) error,
) ([]byte, bool, error) {
	// Source of truth: https://github.com/purpshell/wacaller/blob/bcfb7f0c076b131422c22f024dfff080448e70f4/datasheets/group-media-relay-refresh.md#L77-L111
	if send == nil {
		return nil, false, fmt.Errorf("wacaller: relay binding-success send is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	response, ok := buildRelayBindingSuccess(request, s.key)
	if !ok {
		return nil, false, nil
	}
	if err := send(response); err != nil {
		return response, true, err
	}
	return response, true, nil
}

func groupRelayData(update groupCallUpdate, inbound bool) (*relayData, error) {
	// Source of truth: https://github.com/purpshell/wacaller/blob/a9e4195fb846a730f30ce98c26a7d1c03993fdb2/datasheets/group-media-relay-refresh.md#L64-L92
	if update.Relay == nil {
		return nil, fmt.Errorf("wacaller: group call has no authoritative relay")
	}
	groupRelay := update.Relay
	rd := &relayData{
		relayKeyASCII: bytes.Clone(groupRelay.Key),
		relayTokens:   cloneByteSlices(groupRelay.Tokens),
	}
	for _, endpoint := range groupRelay.Endpoints {
		if endpoint.IPv4 == "" || endpoint.Port == 0 {
			continue
		}
		rd.endpoints = append(rd.endpoints, relayEndpoint{
			relayID: endpoint.RelayID, relayName: endpoint.RelayName,
			tokenID: endpoint.TokenID, authTokenID: endpoint.AuthTokenID,
			isFNA:     endpoint.IsFNA,
			addresses: []relayAddress{{ipv4: endpoint.IPv4, port: endpoint.Port}},
		})
	}
	if getMediaRelayEndpoint(rd, inbound) == nil {
		return nil, fmt.Errorf("wacaller: group relay has no usable endpoint")
	}
	return rd, nil
}

func connectedRemoteParticipantPIDs(update groupCallUpdate, selfID string) []uint32 {
	// Source of truth: https://github.com/purpshell/wacaller/blob/99134bb900df3ee83a69d9a38112e623817597ae/datasheets/group-video-reactions.md#L40-L50
	var pids []uint32
	for _, participant := range update.Participants {
		if participant.State != "connected" {
			continue
		}
		for _, device := range participant.Devices {
			if !device.HasPID || rtp.FormatE2ESrtpParticipantID(device.JID.String()) == selfID {
				continue
			}
			pids = append(pids, device.PID)
		}
	}
	return pids
}
