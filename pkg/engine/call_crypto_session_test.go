package whatsmeow

import (
	"bytes"
	"testing"

	"go.mau.fi/whatsmeow/types"

	"go.mau.fi/whatsmeow/rtp"
	"go.mau.fi/whatsmeow/srtp"
)

func peerJID() types.JID { return types.JID{User: "222222222222222", Server: types.HiddenUserServer} }
func creatorJID() types.JID {
	return types.JID{User: "111111111111111", Server: types.HiddenUserServer, Device: 1}
}

func iota32() []byte {
	b := make([]byte, 32)
	for i := range b {
		b[i] = byte(i)
	}
	return b
}

// TestOutgoingLifecycle pins the outgoing transition table.
func TestOutgoingLifecycle(t *testing.T) {
	s := NewOutgoingSession("CID", peerJID(), creatorJID())
	if s.Phase() != CallPhaseIdle {
		t.Fatalf("phase = %d, want Idle", s.Phase())
	}
	for _, ph := range []CallPhase{CallPhaseCalling, CallPhaseRinging, CallPhaseConnecting, CallPhaseActive} {
		if !s.TransitionTo(ph) {
			t.Fatalf("transition to %d rejected", ph)
		}
	}
	if !s.IsActive() {
		t.Error("not active after reaching Active")
	}
	if s.TransitionTo(CallPhaseCalling) {
		t.Error("illegal Active→Calling accepted")
	}
	if !s.TransitionTo(CallPhaseEnded) {
		t.Error("→Ended rejected")
	}
	if !s.IsEnded() {
		t.Error("not ended")
	}
	if s.TransitionTo(CallPhaseActive) {
		t.Error("Ended is not a sink")
	}
}

// TestIncomingStartsRinging pins the incoming start phase and the no-Calling rule.
func TestIncomingStartsRinging(t *testing.T) {
	s := NewIncomingSession("CID", peerJID(), creatorJID())
	if s.Phase() != CallPhaseRinging {
		t.Fatalf("phase = %d, want Ringing", s.Phase())
	}
	if s.TransitionTo(CallPhaseCalling) {
		t.Error("incoming must not go to Calling")
	}
	if !s.TransitionTo(CallPhaseConnecting) || !s.TransitionTo(CallPhaseActive) {
		t.Error("Ringing→Connecting→Active rejected")
	}
}

// TestMediaPipelineRoundTrips checks the protect→unprotect composition loopback.
func TestMediaPipelineRoundTrips(t *testing.T) {
	callKey := iota32()
	lid := "222222222222222:0@lid"
	tx, err := NewMediaPipeline(callKey, lid, lid, 0x12345678, 960)
	if err != nil {
		t.Fatalf("tx: %v", err)
	}
	rx, err := NewMediaPipeline(callKey, lid, lid, 0x12345678, 960)
	if err != nil {
		t.Fatalf("rx: %v", err)
	}
	opus := []byte{0x48, 0x11, 0x22, 0x33, 0x44, 0x55}
	packet, err := tx.ProtectAudio(opus)
	if err != nil {
		t.Fatalf("protect: %v", err)
	}
	header, payload, ok := rx.UnprotectAudio(packet)
	if !ok {
		t.Fatal("unprotect failed")
	}
	if header.SequenceNumber != 1 || header.Ssrc != 0x12345678 || header.PayloadType != 120 {
		t.Errorf("header = seq %d ssrc %#x pt %d", header.SequenceNumber, header.Ssrc, header.PayloadType)
	}
	if !bytes.Equal(payload, opus) {
		t.Errorf("payload = %x, want %x", payload, opus)
	}
}

func TestMediaPipelineRekeysReceivePathForAnsweringDevice(t *testing.T) {
	callKey := iota32()
	self := "111111111111111:0@lid"
	initialPeer := "222222222222222:0@lid"
	answeringPeer := "222222222222222:7@lid"
	const ssrc = 0x55667788
	receiver, err := NewMediaPipeline(callKey, self, initialPeer, ssrc, FrameSamples)
	if err != nil {
		t.Fatalf("receiver: %v", err)
	}
	answeringDevice, err := NewMediaPipeline(callKey, answeringPeer, self, ssrc, FrameSamples)
	if err != nil {
		t.Fatalf("answering device: %v", err)
	}
	payload := []byte{1, 2, 3, 4, 5}
	packet, err := answeringDevice.ProtectAudio(payload)
	if err != nil {
		t.Fatalf("protect: %v", err)
	}

	if err = receiver.RekeyRecv(callKey, answeringPeer); err != nil {
		t.Fatalf("RekeyRecv: %v", err)
	}
	_, got, ok := receiver.UnprotectAudio(packet)
	if !ok {
		t.Fatal("rekeyed receiver rejected answering-device packet")
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("rekeyed payload = %x, want %x", got, payload)
	}
}

func TestMediaPipelineRekeysReceivePathFromRawParticipantKey(t *testing.T) {
	callKey := iota32()
	rawKey := bytes.Repeat([]byte{0xa5}, 32)
	self := "111111111111111:0@lid"
	peer := "222222222222222:7@lid"
	const ssrc = 0x55667788
	receiver, err := NewMediaPipeline(callKey, self, peer, ssrc, FrameSamples)
	if err != nil {
		t.Fatalf("receiver: %v", err)
	}
	payload := []byte{1, 2, 3, 4, 5}
	header := rtp.RtpHeader{
		PayloadType:    rtp.RtpPayloadTypeOpus,
		SequenceNumber: 1,
		Timestamp:      FrameSamples,
		Ssrc:           ssrc,
	}
	keys, err := srtp.DeriveE2eKeysFromRaw(rawKey, rtp.FormatE2ESrtpParticipantID(peer))
	if err != nil {
		t.Fatalf("derive raw keys: %v", err)
	}
	packet := rtp.EncodeRtpHeader(&header)
	encrypted, err := srtp.CryptPayload(&keys, ssrc, header.SequenceNumber, 0, payload)
	if err != nil {
		t.Fatalf("encrypt raw-key packet: %v", err)
	}
	packet = append(packet, encrypted...)
	packet = srtp.AppendWarpMITag(keys.AuthKey[:], packet, 0, srtp.WarpMITagLen)

	if _, _, ok := receiver.UnprotectAudio(packet); ok {
		t.Fatal("call-key receiver unexpectedly authenticated raw-key packet before rekey")
	}
	if err = receiver.RekeyRecvFromRaw(rawKey, peer); err != nil {
		t.Fatalf("RekeyRecvFromRaw: %v", err)
	}
	_, got, ok := receiver.UnprotectAudio(packet)
	if !ok {
		t.Fatal("raw-key receiver rejected participant packet")
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("raw-key payload = %x, want %x", got, payload)
	}

	peerReceiver, err := NewMediaPipeline(callKey, peer, self, ssrc, FrameSamples)
	if err != nil {
		t.Fatalf("peer receiver: %v", err)
	}
	outboundPayload := []byte{6, 7, 8}
	outbound, err := receiver.ProtectAudio(outboundPayload)
	if err != nil {
		t.Fatalf("protect after receive rekey: %v", err)
	}
	_, got, ok = peerReceiver.UnprotectAudio(outbound)
	if !ok || !bytes.Equal(got, outboundPayload) {
		t.Fatal("raw receive rekey modified the send keys")
	}

	callKeyReceiver, err := NewMediaPipeline(callKey, self, peer, ssrc, FrameSamples)
	if err != nil {
		t.Fatalf("call-key receiver: %v", err)
	}
	callKeySender, err := NewMediaPipeline(callKey, peer, self, ssrc, FrameSamples)
	if err != nil {
		t.Fatalf("call-key sender: %v", err)
	}
	callKeyPacket, err := callKeySender.ProtectAudio(payload)
	if err != nil {
		t.Fatalf("protect call-key packet: %v", err)
	}
	if err = callKeyReceiver.RekeyRecvFromRaw(rawKey[:31], peer); err == nil {
		t.Fatal("short raw receive key was accepted")
	}
	_, got, ok = callKeyReceiver.UnprotectAudio(callKeyPacket)
	if !ok || !bytes.Equal(got, payload) {
		t.Fatal("rejected raw receive rekey modified the working receive state")
	}
}

func TestMediaPipelineRekeysSendPathFromSharedRawEpochWithoutRestartingRTP(t *testing.T) {
	callKey := iota32()
	rawKey := bytes.Repeat([]byte{0x5a}, 32)
	self := "111111111111111:14@lid"
	peer := "222222222222222:7@lid"
	const ssrc = 0x55667788
	sender, err := NewMediaPipeline(callKey, self, peer, ssrc, FrameSamples)
	if err != nil {
		t.Fatalf("sender: %v", err)
	}
	oldReceiver, err := NewMediaPipeline(callKey, peer, self, ssrc, FrameSamples)
	if err != nil {
		t.Fatalf("old receiver: %v", err)
	}
	newReceiver, err := NewMediaPipeline(rawKey, peer, self, ssrc, FrameSamples)
	if err != nil {
		t.Fatalf("new receiver: %v", err)
	}
	if err = newReceiver.RekeyRecvFromRawPreservingROC(rawKey, self); err != nil {
		t.Fatalf("new receiver raw epoch: %v", err)
	}

	firstPayload := []byte{1, 2, 3}
	first, err := sender.ProtectAudio(firstPayload)
	if err != nil {
		t.Fatalf("protect first: %v", err)
	}
	if _, got, ok := oldReceiver.UnprotectAudio(first); !ok || !bytes.Equal(got, firstPayload) {
		t.Fatal("old call-key packet did not authenticate before rekey")
	}

	if err = sender.RekeySendFromRaw(rawKey, self); err != nil {
		t.Fatalf("RekeySendFromRaw: %v", err)
	}
	secondPayload := []byte{4, 5, 6}
	second, err := sender.ProtectAudio(secondPayload)
	if err != nil {
		t.Fatalf("protect second: %v", err)
	}
	header, ok := rtp.ParseRtpHeader(second)
	if !ok {
		t.Fatal("parse second RTP header")
	}
	if header.Ssrc != ssrc || header.SequenceNumber != 2 || header.Timestamp != FrameSamples {
		t.Fatalf(
			"continued RTP header = ssrc %#x seq %d timestamp %d, want %#x/2/%d",
			header.Ssrc,
			header.SequenceNumber,
			header.Timestamp,
			ssrc,
			FrameSamples,
		)
	}
	if _, _, ok = oldReceiver.UnprotectAudio(second); ok {
		t.Fatal("old call-key receiver authenticated post-epoch packet")
	}
	if _, got, ok := newReceiver.UnprotectAudio(second); !ok || !bytes.Equal(got, secondPayload) {
		t.Fatal("shared raw-epoch receiver rejected post-epoch packet")
	}
}

func TestMediaPipelineRawReceiveEpochPreservesROC(t *testing.T) {
	callKey := iota32()
	rawKey := bytes.Repeat([]byte{0x6b}, 32)
	self := "111111111111111:14@lid"
	peer := "222222222222222:7@lid"
	const ssrc = 0x55667788
	receiver, err := NewMediaPipeline(callKey, self, peer, ssrc, FrameSamples)
	if err != nil {
		t.Fatalf("receiver: %v", err)
	}
	protectAt := func(key []byte, raw bool, sequence uint16, roc uint32, payload []byte) []byte {
		t.Helper()
		var keys srtp.E2eSrtpKeys
		var deriveErr error
		participantID := rtp.FormatE2ESrtpParticipantID(peer)
		if raw {
			keys, deriveErr = srtp.DeriveE2eKeysFromRaw(key, participantID)
		} else {
			keys, deriveErr = srtp.DeriveE2eKeys(key, participantID)
		}
		if deriveErr != nil {
			t.Fatalf("derive receive keys: %v", deriveErr)
		}
		header := rtp.RtpHeader{
			PayloadType:    rtp.RtpPayloadTypeOpus,
			SequenceNumber: sequence,
			Timestamp:      uint32(sequence) * FrameSamples,
			Ssrc:           ssrc,
		}
		packet := rtp.EncodeRtpHeader(&header)
		encrypted, cryptErr := srtp.CryptPayload(&keys, ssrc, sequence, roc, payload)
		if cryptErr != nil {
			t.Fatalf("encrypt receive packet: %v", cryptErr)
		}
		packet = append(packet, encrypted...)
		return srtp.AppendWarpMITag(keys.AuthKey[:], packet, roc, srtp.WarpMITagLen)
	}

	for _, sequence := range []uint16{0xfffe, 0xffff} {
		packet := protectAt(callKey, false, sequence, 0, []byte{byte(sequence)})
		if _, _, ok := receiver.UnprotectAudio(packet); !ok {
			t.Fatalf("call-key packet seq %#x failed", sequence)
		}
	}
	if err = receiver.RekeyRecvFromRawPreservingROC(rawKey, peer); err != nil {
		t.Fatalf("RekeyRecvFromRawPreservingROC: %v", err)
	}
	wrapped := protectAt(rawKey, true, 0, 1, []byte{9, 8, 7})
	if _, got, ok := receiver.UnprotectAudio(wrapped); !ok || !bytes.Equal(got, []byte{9, 8, 7}) {
		t.Fatal("raw receive epoch reset ROC across continuing stream")
	}
}

// TestProtectUsesSelfLidForSend pins the send keystream to the self LID.
func TestProtectUsesSelfLidForSend(t *testing.T) {
	callKey := iota32()
	selfLid, peerLid := "111111111111111:0@lid", "222222222222222:0@lid"
	ssrc := uint32(0x12345678)
	pipe, err := NewMediaPipeline(callKey, selfLid, peerLid, ssrc, 960)
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	opus := []byte{0x10, 0x21, 0x32, 0x43}
	packet, err := pipe.ProtectAudio(opus)
	if err != nil {
		t.Fatalf("protect: %v", err)
	}
	withoutTag := packet[:len(packet)-srtp.WarpMITagLen]
	headerLen, ok := rtp.RtpHeaderByteLength(withoutTag)
	if !ok {
		t.Fatal("header length")
	}
	body := withoutTag[headerLen:]

	selfKeys, _ := srtp.DeriveE2eKeys(callKey, selfLid)
	expect, _ := srtp.CryptPayload(&selfKeys, ssrc, 1, 0, opus)
	if !bytes.Equal(body, expect) {
		t.Error("send must encrypt under the self LID")
	}
	peerKeys, _ := srtp.DeriveE2eKeys(callKey, peerLid)
	inverted, _ := srtp.CryptPayload(&peerKeys, ssrc, 1, 0, opus)
	if bytes.Equal(body, inverted) {
		t.Error("send must NOT encrypt under the peer LID")
	}
}

// TestRecvUsesPeerLidForRecv pins the recv keystream to the peer LID.
func TestRecvUsesPeerLidForRecv(t *testing.T) {
	callKey := iota32()
	selfLid, peerLid := "111111111111111:0@lid", "222222222222222:0@lid"
	ssrc := uint32(0x12345678)
	us, _ := NewMediaPipeline(callKey, selfLid, peerLid, ssrc, 960)
	peerTx, _ := NewMediaPipeline(callKey, peerLid, selfLid, ssrc, 960)

	opus := []byte{0x48, 0x01, 0x02, 0x03, 0x04, 0x05}
	fromPeer, err := peerTx.ProtectAudio(opus)
	if err != nil {
		t.Fatalf("peer protect: %v", err)
	}
	_, recovered, ok := us.UnprotectAudio(fromPeer)
	if !ok || !bytes.Equal(recovered, opus) {
		t.Error("recv must use the peer-LID keystream")
	}

	selfKeyedTx, _ := NewMediaPipeline(callKey, selfLid, peerLid, ssrc, 960)
	wrong, _ := selfKeyedTx.ProtectAudio(opus)
	us2, _ := NewMediaPipeline(callKey, selfLid, peerLid, ssrc, 960)
	if _, _, ok = us2.UnprotectAudio(wrong); ok {
		t.Error("recv must reject a self-LID-keyed packet")
	}

	tampered := append([]byte(nil), fromPeer...)
	tampered[len(tampered)-1] ^= 0x80
	us3, _ := NewMediaPipeline(callKey, selfLid, peerLid, ssrc, 960)
	if _, _, ok = us3.UnprotectAudio(tampered); ok {
		t.Error("recv must reject a packet with a tampered WARP MI tag")
	}
	if _, recovered, ok = us3.UnprotectAudio(fromPeer); !ok || !bytes.Equal(recovered, opus) {
		t.Error("rejected packet must not poison receive rollover state")
	}
}

func TestMediaPipelineTracksSenderStats(t *testing.T) {
	pipe, err := NewMediaPipeline(iota32(), "111111111111111:0@lid", "222222222222222:0@lid", 0x12345678, 960)
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	header := rtp.RtpHeader{
		Marker:         true,
		PayloadType:    rtp.RtpPayloadTypeH264,
		SequenceNumber: 0,
		Timestamp:      90000,
		Ssrc:           0x12345678,
		VideoExtension: &rtp.VideoRtpExtension{MediaFrameInfo: rtp.VideoMediaFrameInfoIDR},
	}
	if _, err := pipe.ProtectRTP(&header, []byte{0x65, 1, 2, 3}); err != nil {
		t.Fatalf("protect: %v", err)
	}
	stats := pipe.SenderStats()
	if stats.PacketsSent != 1 || stats.OctetsSent != 4 || stats.RtpTimestamp != 90000 {
		t.Errorf("stats = %+v, want packets=1 octets=4 timestamp=90000", stats)
	}
}
