package whatsmeow

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/rtp"
	"go.mau.fi/whatsmeow/srtp"
	"go.mau.fi/whatsmeow/types"
)

type recordingParticipantDecoder struct {
	payloads [][]byte
}

type blockingParticipantDecoder struct {
	entered chan struct{}
	release chan struct{}
}

func (d *recordingParticipantDecoder) Decode(payload []byte) []float32 {
	d.payloads = append(d.payloads, append([]byte(nil), payload...))
	if len(payload) == 0 {
		return nil
	}
	return []float32{float32(payload[0])}
}

func (d *blockingParticipantDecoder) Decode(payload []byte) []float32 {
	close(d.entered)
	<-d.release
	return []float32{float32(payload[0])}
}

func mediaTestJID(user string, device uint16) types.JID {
	return types.JID{User: user, Device: device, Server: types.HiddenUserServer}
}

func mediaTestGroupUpdate(self, peer, added, pending types.JID, transactionID uint32, includeAdded bool) groupCallUpdate {
	participants := []groupCallParticipant{
		{
			JID:   self.ToNonAD(),
			State: "connected",
			Devices: []groupCallDevice{{
				JID: self, PID: 1, HasPID: true,
			}},
		},
		{
			JID:   peer.ToNonAD(),
			State: "connected",
			Devices: []groupCallDevice{{
				JID: peer, PID: 0, HasPID: true,
			}},
		},
		{
			JID:   pending.ToNonAD(),
			State: "receipt",
			Devices: []groupCallDevice{
				{JID: pending, PID: 3, HasPID: true},
			},
		},
	}
	if includeAdded {
		participants = append(participants, groupCallParticipant{
			JID:   added.ToNonAD(),
			State: "connected",
			Devices: []groupCallDevice{
				{JID: added, PID: 2, HasPID: true},
				{JID: mediaTestJID(added.User, added.Device+1)},
			},
		})
	} else {
		participants = append(participants, groupCallParticipant{
			JID:     added.ToNonAD(),
			State:   "invited",
			Devices: []groupCallDevice{{JID: added}},
		})
	}
	return groupCallUpdate{CallID: "CID", TransactionID: transactionID, Participants: participants}
}

func protectParticipantAudio(t *testing.T, callKey []byte, self, sender types.JID, payload []byte) ([]byte, uint32) {
	t.Helper()
	participantID := rtp.FormatE2ESrtpParticipantID(sender.String())
	ssrc, err := rtp.DeriveWasmParticipantSsrc("CID", participantID, 0)
	if err != nil {
		t.Fatalf("derive sender SSRC: %v", err)
	}
	tx, err := NewMediaPipeline(callKey, sender.String(), self.String(), ssrc, FrameSamples)
	if err != nil {
		t.Fatalf("sender pipeline: %v", err)
	}
	packet, err := tx.ProtectAudio(payload)
	if err != nil {
		t.Fatalf("protect sender audio: %v", err)
	}
	return packet, ssrc
}

func protectRawParticipantAudio(t *testing.T, rawKey []byte, sender types.JID, payload []byte) ([]byte, uint32) {
	t.Helper()
	participantID := rtp.FormatE2ESrtpParticipantID(sender.String())
	ssrc, err := rtp.DeriveWasmParticipantSsrc("CID", participantID, 0)
	if err != nil {
		t.Fatalf("derive raw-key sender SSRC: %v", err)
	}
	keys, err := srtp.DeriveE2eKeysFromRaw(rawKey, participantID)
	if err != nil {
		t.Fatalf("derive raw participant keys: %v", err)
	}
	header := rtp.RtpHeader{
		PayloadType:    rtp.RtpPayloadTypeOpus,
		SequenceNumber: 1,
		Timestamp:      FrameSamples,
		Ssrc:           ssrc,
	}
	packet := rtp.EncodeRtpHeader(&header)
	encrypted, err := srtp.CryptPayload(&keys, ssrc, header.SequenceNumber, 0, payload)
	if err != nil {
		t.Fatalf("encrypt raw participant packet: %v", err)
	}
	packet = append(packet, encrypted...)
	return srtp.AppendWarpMITag(keys.AuthKey[:], packet, 0, srtp.WarpMITagLen), ssrc
}

func protectRawParticipantRTP(
	t *testing.T,
	rawKey []byte,
	sender types.JID,
	slot uint32,
	payloadType uint8,
	payload []byte,
) ([]byte, uint32) {
	t.Helper()
	participantID := rtp.FormatE2ESrtpParticipantID(sender.String())
	ssrc, err := rtp.DeriveWasmParticipantSsrc("CID", participantID, slot)
	if err != nil {
		t.Fatalf("derive raw participant stream SSRC: %v", err)
	}
	keys, err := srtp.DeriveE2eKeysFromRaw(rawKey, participantID)
	if err != nil {
		t.Fatalf("derive raw participant keys: %v", err)
	}
	header := rtp.RtpHeader{
		PayloadType:    payloadType,
		SequenceNumber: 1,
		Timestamp:      3000,
		Ssrc:           ssrc,
	}
	packet := rtp.EncodeRtpHeader(&header)
	encrypted, err := srtp.CryptPayload(&keys, ssrc, header.SequenceNumber, 0, payload)
	if err != nil {
		t.Fatalf("encrypt raw participant packet: %v", err)
	}
	packet = append(packet, encrypted...)
	return srtp.AppendWarpMITag(keys.AuthKey[:], packet, 0, srtp.WarpMITagLen), ssrc
}

func protectParticipantRTP(
	t *testing.T,
	callKey []byte,
	self, sender types.JID,
	slot uint32,
	payloadType uint8,
	payload []byte,
) ([]byte, uint32) {
	t.Helper()
	participantID := rtp.FormatE2ESrtpParticipantID(sender.String())
	ssrc, err := rtp.DeriveWasmParticipantSsrc("CID", participantID, slot)
	if err != nil {
		t.Fatalf("derive participant stream SSRC: %v", err)
	}
	pipe, err := NewMediaPipeline(callKey, sender.String(), self.String(), ssrc, FrameSamples)
	if err != nil {
		t.Fatalf("new participant stream pipeline: %v", err)
	}
	packet, err := pipe.ProtectRTP(&rtp.RtpHeader{
		PayloadType: payloadType, SequenceNumber: 1, Timestamp: 3000, Ssrc: ssrc,
	}, payload)
	if err != nil {
		t.Fatalf("protect participant RTP: %v", err)
	}
	return packet, ssrc
}

func TestParticipantReceiveRegistryAppliesSharedRawEpochToSenderAndEveryReceiver(t *testing.T) {
	callKey := iota32()
	rawKey := bytes.Repeat([]byte{0xa5}, 32)
	self := mediaTestJID("111111111111111", 14)
	peer := mediaTestJID("222222222222222", 0)
	added := mediaTestJID("333333333333333", 43)
	pending := mediaTestJID("444444444444444", 63)
	registry, err := newParticipantReceiveRegistry("CID", callKey, self.String(), peer.String(), func() participantAudioDecoder {
		return &recordingParticipantDecoder{}
	})
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	const selfSSRC = 0x10203040
	sender, err := NewMediaPipeline(callKey, self.String(), peer.String(), selfSSRC, FrameSamples)
	if err != nil {
		t.Fatalf("sender pipeline: %v", err)
	}
	registry.attachSendPipeline(sender)
	if err = registry.ApplyGroupUpdate(mediaTestGroupUpdate(self, peer, added, pending, 17, true)); err != nil {
		t.Fatalf("apply group roster: %v", err)
	}

	if err = registry.ApplyGroupRawEpoch(17, rawKey); err != nil {
		t.Fatalf("apply shared raw epoch: %v", err)
	}
	for _, participant := range []types.JID{peer, added} {
		packet, _ := protectRawParticipantAudio(t, rawKey, participant, []byte{byte(participant.Device + 1)})
		audio, ok := registry.DecodeAudio(packet)
		if !ok || audio.DeviceJID != participant {
			t.Fatalf("raw epoch did not activate receiver %s: %+v", participant, audio)
		}
		oldPacket, _ := protectParticipantAudio(t, callKey, self, participant, []byte{0xee})
		if _, ok = registry.DecodeAudio(oldPacket); ok {
			t.Fatalf("old call-key packet authenticated for %s", participant)
		}
	}

	remoteUnderRaw, err := NewMediaPipeline(callKey, peer.String(), self.String(), selfSSRC, FrameSamples)
	if err != nil {
		t.Fatalf("remote raw receiver: %v", err)
	}
	if err = remoteUnderRaw.RekeyRecvFromRawPreservingROC(rawKey, self.String()); err != nil {
		t.Fatalf("remote raw receive epoch: %v", err)
	}
	remoteUnderCallKey, err := NewMediaPipeline(callKey, peer.String(), self.String(), selfSSRC, FrameSamples)
	if err != nil {
		t.Fatalf("remote call-key receiver: %v", err)
	}
	outbound, err := sender.ProtectAudio([]byte{7, 8, 9})
	if err != nil {
		t.Fatalf("protect outbound after epoch: %v", err)
	}
	if _, _, ok := remoteUnderCallKey.UnprotectAudio(outbound); ok {
		t.Fatal("outbound packet remained under the old call key")
	}
	if _, got, ok := remoteUnderRaw.UnprotectAudio(outbound); !ok || !bytes.Equal(got, []byte{7, 8, 9}) {
		t.Fatal("outbound packet did not use the shared raw epoch")
	}

	if err = registry.ApplyGroupRawEpoch(17, rawKey); err != nil {
		t.Fatalf("identical epoch duplicate: %v", err)
	}
	if err = registry.ApplyGroupRawEpoch(17, bytes.Repeat([]byte{0x5a}, 32)); err == nil {
		t.Fatal("conflicting transaction-wide epoch was accepted")
	}
}

func TestParticipantReceiveRegistryRetiresParticipantStateAndClearsDiscardedRegistry(t *testing.T) {
	callKey := iota32()
	rawKey := bytes.Repeat([]byte{0xa5}, 32)
	self := mediaTestJID("111111111111111", 14)
	peer := mediaTestJID("222222222222222", 0)
	added := mediaTestJID("333333333333333", 43)
	pending := mediaTestJID("444444444444444", 63)
	registry, err := newParticipantReceiveRegistry(
		"CID",
		callKey,
		self.String(),
		peer.String(),
		nil,
	)
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	if err = registry.ApplyGroupUpdate(
		mediaTestGroupUpdate(self, peer, added, pending, 17, true),
	); err != nil {
		t.Fatalf("apply group roster: %v", err)
	}
	if err = registry.ApplyGroupRawEpoch(17, rawKey); err != nil {
		t.Fatalf("apply shared raw epoch: %v", err)
	}
	addedID := rtp.FormatE2ESrtpParticipantID(added.String())
	retired := registry.byDeviceID[addedID]
	if retired == nil || retired.pipe.recvKeys == (srtp.E2eSrtpKeys{}) {
		t.Fatal("added participant did not receive epoch keys")
	}

	if err = registry.ApplyGroupUpdate(
		mediaTestGroupUpdate(self, peer, added, pending, 18, false),
	); err != nil {
		t.Fatalf("retire participant: %v", err)
	}
	_, active := registry.ActiveReceiverSnapshot()
	if _, ok := active[retired]; ok {
		t.Fatal("retired participant remained active")
	}
	if retired.pipe.recvKeys != (srtp.E2eSrtpKeys{}) {
		t.Fatal("retired participant retained media keys")
	}

	registry.clear()
	if registry.callKey != nil ||
		registry.installedEpoch.rawKey != nil ||
		registry.byDeviceID != nil {
		t.Fatal("discarded registry retained key or participant state")
	}
}

func TestParticipantReceiveRegistryAppliesSharedRawEpochToSRTCP(t *testing.T) {
	callKey := iota32()
	rawKey := bytes.Repeat([]byte{0x6d}, 32)
	self := mediaTestJID("111111111111111", 14)
	peer := mediaTestJID("222222222222222", 0)
	added := mediaTestJID("333333333333333", 43)
	pending := mediaTestJID("444444444444444", 63)
	registry, err := newParticipantReceiveRegistry("CID", callKey, self.String(), peer.String(), nil)
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}

	const selfAudioSSRC = 0x10203040
	audioRTCP, err := newMediaSrtcpSender(callKey, self.String(), selfAudioSSRC, false)
	if err != nil {
		t.Fatalf("new audio SRTCP sender: %v", err)
	}
	if err = registry.attachSRTCPSender(audioRTCP); err != nil {
		t.Fatalf("attach audio SRTCP sender: %v", err)
	}
	if _, err = audioRTCP.senderReport(rtp.RtcpSenderStats{}, 1, nil); err != nil {
		t.Fatalf("send pre-epoch report: %v", err)
	}

	sender, err := NewMediaPipeline(callKey, self.String(), peer.String(), selfAudioSSRC, FrameSamples)
	if err != nil {
		t.Fatalf("sender pipeline: %v", err)
	}
	registry.attachSendPipeline(sender)
	if err = registry.ApplyGroupUpdate(mediaTestGroupUpdate(self, peer, added, pending, 17, true)); err != nil {
		t.Fatalf("apply group roster: %v", err)
	}
	if err = registry.ApplyGroupRawEpoch(17, rawKey); err != nil {
		t.Fatalf("apply shared raw epoch: %v", err)
	}

	rawSelfKeys, err := srtp.DeriveE2eSRTCPKeysFromRaw(rawKey, rtp.FormatE2ESrtpParticipantID(self.String()))
	if err != nil {
		t.Fatalf("derive raw self SRTCP keys: %v", err)
	}
	oldSelfKeys, err := srtp.DeriveE2eSrtcpKeys(callKey, rtp.FormatE2ESrtpParticipantID(self.String()))
	if err != nil {
		t.Fatalf("derive old self SRTCP keys: %v", err)
	}
	outbound, err := audioRTCP.senderReport(rtp.RtcpSenderStats{}, 2, nil)
	if err != nil {
		t.Fatalf("send post-epoch report: %v", err)
	}
	if _, _, ok := srtp.UnprotectSrtcp(&oldSelfKeys, selfAudioSSRC, outbound); ok {
		t.Fatal("post-epoch SRTCP sender report authenticated under the old call key")
	}
	if _, index, ok := srtp.UnprotectSrtcp(&rawSelfKeys, selfAudioSSRC, outbound); !ok || index != 2 {
		t.Fatalf("post-epoch SRTCP report = index %d authenticated %t, want index 2 authenticated", index, ok)
	}

	for _, tc := range []struct {
		participant types.JID
		slot        uint32
	}{
		{participant: peer, slot: 0},
		{participant: added, slot: rtp.VideoSlotWord},
	} {
		participantID := rtp.FormatE2ESrtpParticipantID(tc.participant.String())
		senderSSRC, deriveErr := rtp.DeriveWasmParticipantSsrc("CID", participantID, tc.slot)
		if deriveErr != nil {
			t.Fatalf("derive remote stream SSRC: %v", deriveErr)
		}
		keys, deriveErr := srtp.DeriveE2eSRTCPKeysFromRaw(rawKey, participantID)
		if deriveErr != nil {
			t.Fatalf("derive remote SRTCP keys: %v", deriveErr)
		}
		var stats rtp.RtcpSenderStats
		var cnameEntropy [12]byte
		cname := rtp.BuildWhatsappRtcpCname(cnameEntropy)
		plain := rtp.BuildSenderReportWithSdesAndReception(
			senderSSRC,
			&stats,
			3,
			&cname,
			nil,
			tc.slot == rtp.VideoSlotWord,
		)
		protected, protectErr := srtp.ProtectSrtcp(&keys, senderSSRC, 7, plain)
		if protectErr != nil {
			t.Fatalf("protect remote SRTCP: %v", protectErr)
		}
		recovered, index, ok := registry.UnprotectSRTCP(senderSSRC, protected)
		if !ok || index != 7 || !bytes.Equal(recovered, plain) {
			t.Fatalf("participant %s SRTCP = (%x, %d, %t), want authenticated index 7", tc.participant, recovered, index, ok)
		}
	}
	if _, _, ok := registry.UnprotectSRTCP(0xdeadbeef, outbound); ok {
		t.Fatal("SRTCP from an unknown stream SSRC authenticated")
	}

	lateVideoRTCP, err := newMediaSrtcpSender(callKey, self.String(), 0x50607080, true)
	if err != nil {
		t.Fatalf("new late video SRTCP sender: %v", err)
	}
	if err = registry.attachSRTCPSender(lateVideoRTCP); err != nil {
		t.Fatalf("attach late video SRTCP sender: %v", err)
	}
	latePacket, err := lateVideoRTCP.senderReport(rtp.RtcpSenderStats{}, 4, nil)
	if err != nil {
		t.Fatalf("send late-attached report: %v", err)
	}
	if _, index, ok := srtp.UnprotectSrtcp(&rawSelfKeys, 0x50607080, latePacket); !ok || index != 1 {
		t.Fatalf("late-attached SRTCP report = index %d authenticated %t, want index 1 authenticated", index, ok)
	}
}

func TestParticipantReceiveRegistryAppliesSharedRawEpochToAllRTPStreams(t *testing.T) {
	callKey := iota32()
	rawKey := bytes.Repeat([]byte{0x91}, 32)
	self := mediaTestJID("111111111111111", 14)
	peer := mediaTestJID("222222222222222", 0)
	added := mediaTestJID("333333333333333", 43)
	pending := mediaTestJID("444444444444444", 63)
	registry, err := newParticipantReceiveRegistry("CID", callKey, self.String(), peer.String(), nil)
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}

	newSendPipe := func(ssrc uint32) *MediaPipeline {
		t.Helper()
		pipe, pipeErr := NewMediaPipeline(callKey, self.String(), peer.String(), ssrc, FrameSamples)
		if pipeErr != nil {
			t.Fatalf("new send pipeline: %v", pipeErr)
		}
		registry.attachSendPipeline(pipe)
		return pipe
	}
	audioSend := newSendPipe(0x10101010)
	videoSend := newSendPipe(0x20202020)
	if err = registry.ApplyGroupUpdate(mediaTestGroupUpdate(self, peer, added, pending, 17, true)); err != nil {
		t.Fatalf("apply roster: %v", err)
	}
	if err = registry.ApplyGroupRawEpoch(17, rawKey); err != nil {
		t.Fatalf("apply raw epoch: %v", err)
	}

	videoPacket, videoSSRC := protectRawParticipantRTP(
		t, rawKey, added, rtp.VideoSlotWord, rtp.RtpPayloadTypeH264, []byte{0x65, 0x01},
	)
	video, ok := registry.UnprotectVideo(videoPacket)
	if !ok || video.DeviceJID != added || video.Header.Ssrc != videoSSRC || !bytes.Equal(video.Payload, []byte{0x65, 0x01}) {
		t.Fatalf("added participant video = %+v authenticated %t", video, ok)
	}
	oldVideoPacket, _ := protectParticipantRTP(
		t, callKey, self, added, rtp.VideoSlotWord, rtp.RtpPayloadTypeH264, []byte{0x65, 0x02},
	)
	if _, ok = registry.UnprotectVideo(oldVideoPacket); ok {
		t.Fatal("added participant video authenticated under the old call key")
	}
	appPacket, appSSRC := protectRawParticipantRTP(
		t, rawKey, peer, rtp.AppDataSlotWord, rtp.RtpPayloadTypeAppData, []byte{0x08, 0x01},
	)
	appData, ok := registry.UnprotectAppData(appPacket)
	if !ok || appData.DeviceJID != peer || appData.Header.Ssrc != appSSRC || !bytes.Equal(appData.Payload, []byte{0x08, 0x01}) {
		t.Fatalf("peer app-data = %+v authenticated %t", appData, ok)
	}
	oldAppPacket, _ := protectParticipantRTP(
		t, callKey, self, peer, rtp.AppDataSlotWord, rtp.RtpPayloadTypeAppData, []byte{0x08, 0x02},
	)
	if _, ok = registry.UnprotectAppData(oldAppPacket); ok {
		t.Fatal("peer app-data authenticated under the old call key")
	}
	unknown := mediaTestJID("555555555555555", 7)
	unknownAppPacket, _ := protectRawParticipantRTP(
		t, rawKey, unknown, rtp.AppDataSlotWord, rtp.RtpPayloadTypeAppData, []byte{0x08, 0x03},
	)
	if _, ok = registry.UnprotectAppData(unknownAppPacket); ok {
		t.Fatal("unknown participant app-data authenticated")
	}

	assertRawOutbound := func(pipe *MediaPipeline, ssrc uint32, payloadType uint8) {
		t.Helper()
		header := &rtp.RtpHeader{
			PayloadType: payloadType, SequenceNumber: 1, Timestamp: 3000, Ssrc: ssrc,
		}
		packet, protectErr := pipe.ProtectRTP(header, []byte{7, 8, 9})
		if protectErr != nil {
			t.Fatalf("protect outbound RTP: %v", protectErr)
		}
		oldReceiver, receiverErr := NewMediaPipeline(callKey, peer.String(), self.String(), ssrc, FrameSamples)
		if receiverErr != nil {
			t.Fatalf("new old-key receiver: %v", receiverErr)
		}
		if _, _, ok := oldReceiver.UnprotectAudio(packet); ok {
			t.Fatal("post-epoch outbound RTP authenticated under the old call key")
		}
		rawReceiver, receiverErr := NewMediaPipeline(callKey, peer.String(), self.String(), ssrc, FrameSamples)
		if receiverErr != nil {
			t.Fatalf("new raw-key receiver: %v", receiverErr)
		}
		if receiverErr = rawReceiver.RekeyRecvFromRawPreservingROC(rawKey, self.String()); receiverErr != nil {
			t.Fatalf("rekey raw receiver: %v", receiverErr)
		}
		if _, payload, ok := rawReceiver.UnprotectAudio(packet); !ok || !bytes.Equal(payload, []byte{7, 8, 9}) {
			t.Fatal("post-epoch outbound RTP did not authenticate under the raw root")
		}
	}
	assertRawOutbound(audioSend, 0x10101010, rtp.RtpPayloadTypeOpus)
	assertRawOutbound(videoSend, 0x20202020, rtp.RtpPayloadTypeH264)

	lateAppDataSend := newSendPipe(0x30303030)
	assertRawOutbound(lateAppDataSend, 0x30303030, rtp.RtpPayloadTypeAppData)
}

func TestParticipantReceiveRegistryRekeysDirectFallbackToAnsweringDevice(t *testing.T) {
	callKey := iota32()
	self := mediaTestJID("111111111111111", 14)
	offeredPeer := mediaTestJID("222222222222222", 0)
	answeringPeer := mediaTestJID("222222222222222", 23)
	registry, err := newParticipantReceiveRegistry("CID", callKey, self.String(), offeredPeer.String(), nil)
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	if err = registry.RekeyFallback(answeringPeer.String()); err != nil {
		t.Fatalf("rekey fallback: %v", err)
	}

	protect := func(peer types.JID) (uint32, []byte) {
		t.Helper()
		participantID := rtp.FormatE2ESrtpParticipantID(peer.String())
		ssrc, deriveErr := rtp.DeriveWasmParticipantSsrc("CID", participantID, 0)
		if deriveErr != nil {
			t.Fatalf("derive peer SSRC: %v", deriveErr)
		}
		keys, deriveErr := srtp.DeriveE2eSrtcpKeys(callKey, participantID)
		if deriveErr != nil {
			t.Fatalf("derive peer SRTCP keys: %v", deriveErr)
		}
		var stats rtp.RtcpSenderStats
		var entropy [12]byte
		cname := rtp.BuildWhatsappRtcpCname(entropy)
		plain := rtp.BuildSenderReportWithSdesAndReception(ssrc, &stats, 1, &cname, nil, false)
		packet, protectErr := srtp.ProtectSrtcp(&keys, ssrc, 1, plain)
		if protectErr != nil {
			t.Fatalf("protect peer SRTCP: %v", protectErr)
		}
		return ssrc, packet
	}
	answeringSSRC, answeringPacket := protect(answeringPeer)
	if _, _, ok := registry.UnprotectSRTCP(answeringSSRC, answeringPacket); !ok {
		t.Fatal("answering device SRTCP did not authenticate through fallback registry")
	}
	offeredSSRC, offeredPacket := protect(offeredPeer)
	if _, _, ok := registry.UnprotectSRTCP(offeredSSRC, offeredPacket); ok {
		t.Fatal("superseded offered device SRTCP remained active")
	}
	for _, tc := range []struct {
		name        string
		slot        uint32
		payloadType uint8
		unprotect   func([]byte) (unprotectedParticipantMedia, bool)
	}{
		{
			name: "video", slot: rtp.VideoSlotWord, payloadType: rtp.RtpPayloadTypeH264,
			unprotect: registry.UnprotectVideo,
		},
		{
			name: "app-data", slot: rtp.AppDataSlotWord, payloadType: rtp.RtpPayloadTypeAppData,
			unprotect: registry.UnprotectAppData,
		},
	} {
		answeringPacket, _ := protectParticipantRTP(
			t, callKey, self, answeringPeer, tc.slot, tc.payloadType, []byte{1, 2, 3},
		)
		if media, ok := tc.unprotect(answeringPacket); !ok || media.DeviceJID != answeringPeer {
			t.Fatalf("answering device %s = %+v authenticated %t", tc.name, media, ok)
		}
		offeredPacket, _ := protectParticipantRTP(
			t, callKey, self, offeredPeer, tc.slot, tc.payloadType, []byte{4, 5, 6},
		)
		if _, ok := tc.unprotect(offeredPacket); ok {
			t.Fatalf("superseded offered device %s remained active", tc.name)
		}
	}
}

func TestParticipantReceiveRegistryCarriesSRTCPEpochAcrossRosterChanges(t *testing.T) {
	callKey := iota32()
	rawKey := bytes.Repeat([]byte{0x7e}, 32)
	self := mediaTestJID("111111111111111", 14)
	peer := mediaTestJID("222222222222222", 0)
	added := mediaTestJID("333333333333333", 43)
	pending := mediaTestJID("444444444444444", 63)
	registry, err := newParticipantReceiveRegistry("CID", callKey, self.String(), peer.String(), nil)
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	sender, err := NewMediaPipeline(callKey, self.String(), peer.String(), 0x10203040, FrameSamples)
	if err != nil {
		t.Fatalf("sender pipeline: %v", err)
	}
	registry.attachSendPipeline(sender)
	if err = registry.ApplyGroupUpdate(mediaTestGroupUpdate(self, peer, added, pending, 17, false)); err != nil {
		t.Fatalf("apply initial roster: %v", err)
	}
	if err = registry.ApplyGroupRawEpoch(17, rawKey); err != nil {
		t.Fatalf("apply group epoch: %v", err)
	}
	if err = registry.ApplyGroupUpdate(mediaTestGroupUpdate(self, peer, added, pending, 18, true)); err != nil {
		t.Fatalf("apply expanded roster: %v", err)
	}

	addedID := rtp.FormatE2ESrtpParticipantID(added.String())
	addedVideoSSRC, err := rtp.DeriveWasmParticipantSsrc("CID", addedID, rtp.VideoSlotWord)
	if err != nil {
		t.Fatalf("derive added video SSRC: %v", err)
	}
	addedKeys, err := srtp.DeriveE2eSRTCPKeysFromRaw(rawKey, addedID)
	if err != nil {
		t.Fatalf("derive added SRTCP keys: %v", err)
	}
	var stats rtp.RtcpSenderStats
	var entropy [12]byte
	cname := rtp.BuildWhatsappRtcpCname(entropy)
	plain := rtp.BuildSenderReportWithSdesAndReception(addedVideoSSRC, &stats, 5, &cname, nil, true)
	packet, err := srtp.ProtectSrtcp(&addedKeys, addedVideoSSRC, 9, plain)
	if err != nil {
		t.Fatalf("protect added SRTCP: %v", err)
	}
	if _, index, ok := registry.UnprotectSRTCP(addedVideoSSRC, packet); !ok || index != 9 {
		t.Fatalf("new participant SRTCP = index %d authenticated %t, want index 9 authenticated", index, ok)
	}
	addedVideo, _ := protectRawParticipantRTP(
		t, rawKey, added, rtp.VideoSlotWord, rtp.RtpPayloadTypeH264, []byte{0x65, 0x03},
	)
	if _, ok := registry.UnprotectVideo(addedVideo); !ok {
		t.Fatal("new participant video did not inherit the current raw epoch")
	}
	addedAppData, _ := protectRawParticipantRTP(
		t, rawKey, added, rtp.AppDataSlotWord, rtp.RtpPayloadTypeAppData, []byte{0x08, 0x04},
	)
	if _, ok := registry.UnprotectAppData(addedAppData); !ok {
		t.Fatal("new participant app-data did not inherit the current raw epoch")
	}

	if err = registry.ApplyGroupUpdate(mediaTestGroupUpdate(self, peer, added, pending, 19, false)); err != nil {
		t.Fatalf("apply participant departure: %v", err)
	}
	if _, _, ok := registry.UnprotectSRTCP(addedVideoSSRC, packet); ok {
		t.Fatal("departed participant SRTCP remained active")
	}
	if _, ok := registry.UnprotectVideo(addedVideo); ok {
		t.Fatal("departed participant video remained active")
	}
	if _, ok := registry.UnprotectAppData(addedAppData); ok {
		t.Fatal("departed participant app-data remained active")
	}
}

func TestParticipantReceiveRegistrySRTCPRekeyIsConcurrentSafe(t *testing.T) {
	callKey := iota32()
	rawKey := bytes.Repeat([]byte{0x8f}, 32)
	self := mediaTestJID("111111111111111", 14)
	peer := mediaTestJID("222222222222222", 0)
	added := mediaTestJID("333333333333333", 43)
	pending := mediaTestJID("444444444444444", 63)
	registry, err := newParticipantReceiveRegistry("CID", callKey, self.String(), peer.String(), nil)
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	const selfSSRC = 0x10203040
	sender, err := NewMediaPipeline(callKey, self.String(), peer.String(), selfSSRC, FrameSamples)
	if err != nil {
		t.Fatalf("sender pipeline: %v", err)
	}
	registry.attachSendPipeline(sender)
	srtcpSender, err := newMediaSrtcpSender(callKey, self.String(), selfSSRC, false)
	if err != nil {
		t.Fatalf("new SRTCP sender: %v", err)
	}
	if err = registry.attachSRTCPSender(srtcpSender); err != nil {
		t.Fatalf("attach SRTCP sender: %v", err)
	}
	if err = registry.ApplyGroupUpdate(mediaTestGroupUpdate(self, peer, added, pending, 17, true)); err != nil {
		t.Fatalf("apply initial roster: %v", err)
	}
	if err = registry.ApplyGroupRawEpoch(17, rawKey); err != nil {
		t.Fatalf("apply initial epoch: %v", err)
	}

	peerID := rtp.FormatE2ESrtpParticipantID(peer.String())
	peerSSRC, err := rtp.DeriveWasmParticipantSsrc("CID", peerID, 0)
	if err != nil {
		t.Fatalf("derive peer SSRC: %v", err)
	}
	peerKeys, err := srtp.DeriveE2eSRTCPKeysFromRaw(rawKey, peerID)
	if err != nil {
		t.Fatalf("derive peer SRTCP keys: %v", err)
	}
	var stats rtp.RtcpSenderStats
	var entropy [12]byte
	cname := rtp.BuildWhatsappRtcpCname(entropy)
	plain := rtp.BuildSenderReportWithSdesAndReception(peerSSRC, &stats, 1, &cname, nil, false)
	peerPacket, err := srtp.ProtectSrtcp(&peerKeys, peerSSRC, 1, plain)
	if err != nil {
		t.Fatalf("protect peer SRTCP: %v", err)
	}

	errs := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := range 200 {
			if _, reportErr := srtcpSender.senderReport(rtp.RtcpSenderStats{}, uint64(i), nil); reportErr != nil {
				errs <- reportErr
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for range 200 {
			_, _, _ = registry.UnprotectSRTCP(peerSSRC, peerPacket)
		}
	}()
	for transactionID := uint32(18); transactionID <= 28; transactionID++ {
		update := mediaTestGroupUpdate(self, peer, added, pending, transactionID, true)
		if err = registry.ApplyGroupUpdate(update); err != nil {
			t.Fatalf("apply roster transaction %d: %v", transactionID, err)
		}
		nextRaw := bytes.Repeat([]byte{byte(transactionID)}, 32)
		if err = registry.ApplyGroupRawEpoch(transactionID, nextRaw); err != nil {
			t.Fatalf("apply raw epoch transaction %d: %v", transactionID, err)
		}
	}
	wg.Wait()
	close(errs)
	for concurrentErr := range errs {
		t.Fatalf("concurrent SRTCP operation: %v", concurrentErr)
	}
	if srtcpSender.index != 201 {
		t.Fatalf("SRTCP sender index = %d, want 201", srtcpSender.index)
	}
	if got := registry.installedEpoch.transactionID; got != 28 {
		t.Fatalf("installed epoch transaction = %d, want 28", got)
	}
	if len(registry.bySRTCPSSRC) != 18 {
		t.Fatalf("active participant stream SSRCs = %d, want 18", len(registry.bySRTCPSSRC))
	}
	if !registry.hasEpoch {
		t.Fatal("group epoch was lost during concurrent SRTCP traffic")
	}
}

func TestParticipantReceiveRegistryBuffersSharedRawEpochUntilRoster(t *testing.T) {
	callKey := iota32()
	rawKey := bytes.Repeat([]byte{0xb6}, 32)
	self := mediaTestJID("111111111111111", 14)
	peer := mediaTestJID("222222222222222", 0)
	added := mediaTestJID("333333333333333", 43)
	pending := mediaTestJID("444444444444444", 63)
	registry, err := newParticipantReceiveRegistry("CID", callKey, self.String(), peer.String(), nil)
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	sender, err := NewMediaPipeline(callKey, self.String(), peer.String(), 0x10203040, FrameSamples)
	if err != nil {
		t.Fatalf("sender pipeline: %v", err)
	}
	registry.attachSendPipeline(sender)
	if err = registry.ApplyGroupRawEpoch(17, rawKey); err != nil {
		t.Fatalf("buffer shared raw epoch: %v", err)
	}
	if err = registry.ApplyGroupUpdate(mediaTestGroupUpdate(self, peer, added, pending, 17, true)); err != nil {
		t.Fatalf("apply matching roster: %v", err)
	}
	for _, participant := range []types.JID{peer, added} {
		packet, _ := protectRawParticipantAudio(t, rawKey, participant, []byte{1})
		if _, ok := registry.DecodeAudio(packet); !ok {
			t.Fatalf("buffered shared epoch did not activate %s", participant)
		}
	}
}

func TestParticipantReceiveRegistryCarriesCurrentEpochToNewRosterMembers(t *testing.T) {
	callKey := iota32()
	rawKey := bytes.Repeat([]byte{0xc7}, 32)
	self := mediaTestJID("111111111111111", 14)
	peer := mediaTestJID("222222222222222", 0)
	added := mediaTestJID("333333333333333", 43)
	pending := mediaTestJID("444444444444444", 63)
	registry, err := newParticipantReceiveRegistry("CID", callKey, self.String(), peer.String(), nil)
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	sender, err := NewMediaPipeline(callKey, self.String(), peer.String(), 0x10203040, FrameSamples)
	if err != nil {
		t.Fatalf("sender pipeline: %v", err)
	}
	registry.attachSendPipeline(sender)
	if err = registry.ApplyGroupUpdate(mediaTestGroupUpdate(self, peer, added, pending, 17, false)); err != nil {
		t.Fatalf("apply initial roster: %v", err)
	}
	if err = registry.ApplyGroupRawEpoch(17, rawKey); err != nil {
		t.Fatalf("apply current epoch: %v", err)
	}
	if err = registry.ApplyGroupUpdate(mediaTestGroupUpdate(self, peer, added, pending, 18, true)); err != nil {
		t.Fatalf("apply expanded roster: %v", err)
	}
	addedPacket, _ := protectRawParticipantAudio(t, rawKey, added, []byte{1})
	if _, ok := registry.DecodeAudio(addedPacket); !ok {
		t.Fatal("new roster member did not inherit the current shared epoch")
	}
	staleKey := bytes.Repeat([]byte{0xd8}, 32)
	if err = registry.ApplyGroupRawEpoch(16, staleKey); err != nil {
		t.Fatalf("stale epoch: %v", err)
	}
	peerPacket, _ := protectRawParticipantAudio(t, rawKey, peer, []byte{2})
	if _, ok := registry.DecodeAudio(peerPacket); !ok {
		t.Fatal("stale epoch replaced the current shared epoch")
	}
}

func TestParticipantReceiveRegistryRejectsMalformedEpochWithoutChangingMedia(t *testing.T) {
	callKey := iota32()
	self := mediaTestJID("111111111111111", 14)
	peer := mediaTestJID("222222222222222", 0)
	added := mediaTestJID("333333333333333", 43)
	pending := mediaTestJID("444444444444444", 63)
	registry, err := newParticipantReceiveRegistry("CID", callKey, self.String(), peer.String(), nil)
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	const selfSSRC = 0x10203040
	sender, err := NewMediaPipeline(callKey, self.String(), peer.String(), selfSSRC, FrameSamples)
	if err != nil {
		t.Fatalf("sender pipeline: %v", err)
	}
	registry.attachSendPipeline(sender)
	if err = registry.ApplyGroupUpdate(mediaTestGroupUpdate(self, peer, added, pending, 17, true)); err != nil {
		t.Fatalf("apply roster: %v", err)
	}
	if err = registry.ApplyGroupRawEpoch(17, bytes.Repeat([]byte{1}, 31)); err == nil {
		t.Fatal("short group raw epoch was accepted")
	}
	peerPacket, _ := protectParticipantAudio(t, callKey, self, peer, []byte{3})
	if _, ok := registry.DecodeAudio(peerPacket); !ok {
		t.Fatal("malformed epoch changed a working receive key")
	}
	remote, err := NewMediaPipeline(callKey, peer.String(), self.String(), selfSSRC, FrameSamples)
	if err != nil {
		t.Fatalf("remote call-key receiver: %v", err)
	}
	outbound, err := sender.ProtectAudio([]byte{4})
	if err != nil {
		t.Fatalf("protect after malformed epoch: %v", err)
	}
	if _, got, ok := remote.UnprotectAudio(outbound); !ok || !bytes.Equal(got, []byte{4}) {
		t.Fatal("malformed epoch changed a working send key")
	}
}

func TestParticipantReceiveRegistryPreservesFallbackAcrossPrePIDGroupUpdate(t *testing.T) {
	callKey := iota32()
	self := mediaTestJID("111111111111111", 14)
	peer := mediaTestJID("222222222222222", 0)
	invited := mediaTestJID("333333333333333", 43)
	registry, err := newParticipantReceiveRegistry("CID", callKey, self.String(), peer.String(), func() participantAudioDecoder {
		return &recordingParticipantDecoder{}
	})
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	fallback := registry.byDeviceID[registry.fallbackID]
	update := groupCallUpdate{
		CallID:        "CID",
		TransactionID: 9,
		Participants: []groupCallParticipant{
			{
				JID:     self.ToNonAD(),
				State:   "connected",
				Devices: []groupCallDevice{{JID: self}},
			},
			{
				JID:     peer.ToNonAD(),
				State:   "connected",
				Devices: []groupCallDevice{{JID: peer}},
			},
			{
				JID:     invited.ToNonAD(),
				State:   "receipt",
				Devices: []groupCallDevice{{JID: invited}},
			},
		},
	}
	if err = registry.ApplyGroupUpdate(update); err != nil {
		t.Fatalf("apply pre-PID group update: %v", err)
	}
	if registry.transactionID != 9 || !registry.hasGroupUpdate {
		t.Fatalf("pre-PID update state = transaction %d, applied %t", registry.transactionID, registry.hasGroupUpdate)
	}
	if registry.byDeviceID[registry.fallbackID] != fallback {
		t.Fatal("pre-PID group update replaced the authenticated direct receiver")
	}
	packet, _ := protectParticipantAudio(t, callKey, self, peer, []byte{0x61})
	if _, ok := registry.DecodeAudio(packet); !ok {
		t.Fatal("pre-PID group update stopped direct peer audio")
	}
}

func TestParticipantReceiveRegistryPreservesFallbackWhenOnlySelfHasPID(t *testing.T) {
	callKey := iota32()
	self := mediaTestJID("111111111111111", 14)
	peer := mediaTestJID("222222222222222", 0)
	invited := mediaTestJID("333333333333333", 43)
	registry, err := newParticipantReceiveRegistry("CID", callKey, self.String(), peer.String(), func() participantAudioDecoder {
		return &recordingParticipantDecoder{}
	})
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	fallback := registry.byDeviceID[registry.fallbackID]
	peerID := rtp.FormatE2ESrtpParticipantID(peer.String())
	peerSSRC, err := rtp.DeriveWasmParticipantSsrc("CID", peerID, 0)
	if err != nil {
		t.Fatalf("derive peer SSRC: %v", err)
	}
	sender, err := NewMediaPipeline(callKey, peer.String(), self.String(), peerSSRC, FrameSamples)
	if err != nil {
		t.Fatalf("new peer sender: %v", err)
	}
	before, err := sender.ProtectAudio([]byte{0x60})
	if err != nil {
		t.Fatalf("protect pre-update audio: %v", err)
	}
	if _, ok := registry.DecodeAudio(before); !ok {
		t.Fatal("direct fallback did not authenticate before group update")
	}

	update := groupCallUpdate{
		CallID:        "CID",
		TransactionID: 9,
		Participants: []groupCallParticipant{
			{
				JID:   self.ToNonAD(),
				State: "connected",
				Devices: []groupCallDevice{{
					JID: self, PID: 1, HasPID: true,
				}},
			},
			{
				JID:     peer.ToNonAD(),
				State:   "connected",
				Devices: []groupCallDevice{{JID: peer}},
			},
			{
				JID:     invited.ToNonAD(),
				State:   "receipt",
				Devices: []groupCallDevice{{JID: invited}},
			},
		},
	}
	if err = registry.ApplyGroupUpdate(update); err != nil {
		t.Fatalf("apply self-PID transitional update: %v", err)
	}
	if registry.byDeviceID[registry.fallbackID] != fallback {
		t.Fatal("local PID suppressed the authenticated direct fallback receiver")
	}
	after, err := sender.ProtectAudio([]byte{0x61})
	if err != nil {
		t.Fatalf("protect post-update audio: %v", err)
	}
	audio, ok := registry.DecodeAudio(after)
	if !ok {
		t.Fatal("local PID stopped direct peer audio during transitional update")
	}
	if audio.ParticipantID != peerID || audio.HasPID {
		t.Fatalf("fallback metadata = %+v, want participant %s without PID", audio, peerID)
	}
}

func TestParticipantReceiveRegistryPrePIDUpdatePrunesNonFallbackReceivers(t *testing.T) {
	callKey := iota32()
	self := mediaTestJID("111111111111111", 14)
	peer := mediaTestJID("222222222222222", 0)
	added := mediaTestJID("333333333333333", 43)
	pending := mediaTestJID("444444444444444", 63)
	registry, err := newParticipantReceiveRegistry("CID", callKey, self.String(), peer.String(), func() participantAudioDecoder {
		return &recordingParticipantDecoder{}
	})
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	if err = registry.ApplyGroupUpdate(mediaTestGroupUpdate(self, peer, added, pending, 8, true)); err != nil {
		t.Fatalf("apply PID-bearing group update: %v", err)
	}
	fallback := registry.byDeviceID[registry.fallbackID]
	addedReceiver := registry.byPID[2]
	if fallback == nil || addedReceiver == nil {
		t.Fatal("PID-bearing update did not activate both remotes")
	}

	update := groupCallUpdate{
		CallID:        "CID",
		TransactionID: 9,
		Participants: []groupCallParticipant{
			{
				JID:     self.ToNonAD(),
				State:   "connected",
				Devices: []groupCallDevice{{JID: self}},
			},
			{
				JID:     peer.ToNonAD(),
				State:   "connected",
				Devices: []groupCallDevice{{JID: peer}},
			},
		},
	}
	if err = registry.ApplyGroupUpdate(update); err != nil {
		t.Fatalf("apply pre-PID group update: %v", err)
	}
	if len(registry.byDeviceID) != 1 || registry.byDeviceID[registry.fallbackID] != fallback {
		t.Fatalf("pre-PID active devices = %v, want only fallback", registry.ActiveParticipantIDs())
	}
	if len(registry.byPID) != 0 {
		t.Fatalf("pre-PID active PID count = %d, want 0", len(registry.byPID))
	}
	if _, ok := registry.bySSRC[addedReceiver.ssrc]; ok {
		t.Fatal("pre-PID update retained non-fallback participant SSRC")
	}
}

func TestParticipantReceiveRegistryActivatesAndRoutesConnectedPIDDevices(t *testing.T) {
	callKey := iota32()
	self := mediaTestJID("111111111111111", 14)
	peer := mediaTestJID("222222222222222", 0)
	added := mediaTestJID("333333333333333", 43)
	pending := mediaTestJID("444444444444444", 63)
	var decoders []*recordingParticipantDecoder
	registry, err := newParticipantReceiveRegistry("CID", callKey, self.String(), peer.String(), func() participantAudioDecoder {
		decoder := &recordingParticipantDecoder{}
		decoders = append(decoders, decoder)
		return decoder
	})
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	peerID := rtp.FormatE2ESrtpParticipantID(peer.String())
	originalPeer := registry.byDeviceID[peerID]
	if originalPeer == nil || originalPeer.hasPID {
		t.Fatal("direct-call fallback receiver was not seeded without a PID")
	}

	if err = registry.ApplyGroupUpdate(mediaTestGroupUpdate(self, peer, added, pending, 16, true)); err != nil {
		t.Fatalf("apply group update: %v", err)
	}
	if len(registry.byPID) != 2 {
		t.Fatalf("active remote PID count = %d, want 2", len(registry.byPID))
	}
	addedID := rtp.FormatE2ESrtpParticipantID(added.String())
	activeIDs := registry.ActiveParticipantIDs()
	if len(activeIDs) != 2 || activeIDs[0] != peerID || activeIDs[1] != addedID {
		t.Fatalf("active participant IDs = %v, want [%s %s]", activeIDs, peerID, addedID)
	}
	if registry.byPID[0] != originalPeer {
		t.Fatal("original peer receiver was replaced instead of promoted to PID 0")
	}
	addedReceiver := registry.byPID[2]
	if addedReceiver == nil || addedReceiver.deviceJID != added {
		t.Fatalf("added receiver = %#v, want winning device %s", addedReceiver, added)
	}
	wantAudioSSRCs := []uint32{originalPeer.ssrc, addedReceiver.ssrc}
	slices.Sort(wantAudioSSRCs)
	if got := registry.ActiveAudioSSRCs(); !slices.Equal(got, wantAudioSSRCs) {
		t.Fatalf("active audio SSRCs = %v, want %v", got, wantAudioSSRCs)
	}
	wantVideoSSRCs := []uint32{originalPeer.videoSSRC, addedReceiver.videoSSRC}
	slices.Sort(wantVideoSSRCs)
	if got := registry.ActiveVideoSSRCs(); !slices.Equal(got, wantVideoSSRCs) {
		t.Fatalf("active video SSRCs = %v, want %v", got, wantVideoSSRCs)
	}
	if _, ok := registry.byPID[1]; ok {
		t.Fatal("local PID was activated as a remote receiver")
	}
	if _, ok := registry.byPID[3]; ok {
		t.Fatal("receipt-state participant with a PID was activated")
	}

	peerPacket, peerSSRC := protectParticipantAudio(t, callKey, self, peer, []byte{0x11})
	peerAudio, ok := registry.DecodeAudio(peerPacket)
	if !ok {
		t.Fatal("authenticated original-peer packet was rejected")
	}
	if peerAudio.ParticipantID != peerID || peerAudio.PID != 0 || !peerAudio.HasPID || peerAudio.SSRC != peerSSRC || peerAudio.DeviceJID != peer {
		t.Fatalf("original-peer metadata = %+v", peerAudio)
	}

	addedPacket, addedSSRC := protectParticipantAudio(t, callKey, self, added, []byte{0x22})
	addedAudio, ok := registry.DecodeAudio(addedPacket)
	if !ok {
		t.Fatal("authenticated added-participant packet was rejected")
	}
	if addedAudio.ParticipantID != addedID || addedAudio.PID != 2 || !addedAudio.HasPID || addedAudio.SSRC != addedSSRC || addedAudio.DeviceJID != added {
		t.Fatalf("added-participant metadata = %+v", addedAudio)
	}
	if len(decoders) != 2 || !bytes.Equal(decoders[0].payloads[0], []byte{0x11}) || !bytes.Equal(decoders[1].payloads[0], []byte{0x22}) {
		t.Fatalf("decoder histories = %#v", decoders)
	}

	parentSSRC, err := rtp.DeriveWasmParticipantSsrc("CID", rtp.FormatE2ESrtpParticipantID(added.ToNonAD().String()), 0)
	if err != nil {
		t.Fatalf("derive parent SSRC: %v", err)
	}
	if _, ok = registry.bySSRC[parentSSRC]; ok && parentSSRC != addedSSRC {
		t.Fatal("parent or nonwinning device SSRC was activated")
	}
}

func TestParticipantReceiveRegistryRemovesDepartedParticipantWithoutResettingPeer(t *testing.T) {
	callKey := iota32()
	self := mediaTestJID("111111111111111", 14)
	peer := mediaTestJID("222222222222222", 0)
	added := mediaTestJID("333333333333333", 43)
	pending := mediaTestJID("444444444444444", 63)
	registry, err := newParticipantReceiveRegistry("CID", callKey, self.String(), peer.String(), func() participantAudioDecoder {
		return &recordingParticipantDecoder{}
	})
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	if err = registry.ApplyGroupUpdate(mediaTestGroupUpdate(self, peer, added, pending, 16, true)); err != nil {
		t.Fatalf("apply connected update: %v", err)
	}
	peerReceiver := registry.byPID[0]
	addedReceiver := registry.byPID[2]
	addedPacket, addedSSRC := protectParticipantAudio(t, callKey, self, added, []byte{0x31})
	if _, ok := registry.DecodeAudio(addedPacket); !ok {
		t.Fatal("added participant did not route before departure")
	}

	if err = registry.ApplyGroupUpdate(mediaTestGroupUpdate(self, peer, added, pending, 18, false)); err != nil {
		t.Fatalf("apply departure update: %v", err)
	}
	if registry.byPID[0] != peerReceiver {
		t.Fatal("departure reset the original peer receiver")
	}
	if _, ok := registry.byPID[2]; ok {
		t.Fatal("departed participant remains indexed by PID")
	}
	if _, ok := registry.bySSRC[addedSSRC]; ok {
		t.Fatal("departed participant remains indexed by SSRC")
	}
	if _, ok := registry.DecodeAudio(addedPacket); ok {
		t.Fatal("late departed-participant packet was accepted")
	}
	if addedReceiver == nil {
		t.Fatal("test did not create the added receiver")
	}

	if err = registry.ApplyGroupUpdate(mediaTestGroupUpdate(self, peer, added, pending, 16, true)); err != nil {
		t.Fatalf("apply stale update: %v", err)
	}
	if _, ok := registry.byPID[2]; ok {
		t.Fatal("stale group update reactivated a departed participant")
	}

	peerPacket, _ := protectParticipantAudio(t, callKey, self, peer, []byte{0x41})
	if _, ok := registry.DecodeAudio(peerPacket); !ok {
		t.Fatal("original peer stopped routing after participant departure")
	}
}

func TestParticipantReceiveRegistryRejectedUpdateIsAtomic(t *testing.T) {
	callKey := iota32()
	self := mediaTestJID("111111111111111", 14)
	peer := mediaTestJID("222222222222222", 0)
	added := mediaTestJID("333333333333333", 43)
	pending := mediaTestJID("444444444444444", 63)
	registry, err := newParticipantReceiveRegistry("CID", callKey, self.String(), peer.String(), func() participantAudioDecoder {
		return &recordingParticipantDecoder{}
	})
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	if err = registry.ApplyGroupUpdate(mediaTestGroupUpdate(self, peer, added, pending, 16, true)); err != nil {
		t.Fatalf("apply connected update: %v", err)
	}
	peerReceiver := registry.byPID[0]
	addedReceiver := registry.byPID[2]
	peerUser := peerReceiver.userJID
	activeIDs := registry.ActiveParticipantIDs()

	invalid := mediaTestGroupUpdate(self, peer, added, pending, 20, true)
	invalid.Participants[1].JID = mediaTestJID("555555555555555", 0).ToNonAD()
	invalid.Participants[len(invalid.Participants)-1].Devices[0].PID = 0
	err = registry.ApplyGroupUpdate(invalid)
	if err == nil {
		t.Fatal("duplicate PID update was accepted")
	}
	if registry.transactionID != 16 {
		t.Fatalf("rejected update advanced transaction to %d", registry.transactionID)
	}
	if registry.byPID[0] != peerReceiver || registry.byPID[2] != addedReceiver {
		t.Fatal("rejected update replaced active receiver maps")
	}
	if peerReceiver.userJID != peerUser || peerReceiver.pid != 0 {
		t.Fatalf("rejected update mutated original peer metadata: user=%s pid=%d", peerReceiver.userJID, peerReceiver.pid)
	}
	if got := registry.ActiveParticipantIDs(); len(got) != len(activeIDs) || got[0] != activeIDs[0] || got[1] != activeIDs[1] {
		t.Fatalf("rejected update changed active IDs from %v to %v", activeIDs, got)
	}
}

func TestParticipantReceiveRegistryExternalFailureIsAtomicAndRetryable(t *testing.T) {
	// Source of truth: https://github.com/purpshell/wacaller/blob/65b1dbf33f365db7392e438c3e3bf3651decb6cf/datasheets/group-media-receive.md#L100-L141
	callKey := iota32()
	self := mediaTestJID("111111111111111", 14)
	peer := mediaTestJID("222222222222222", 0)
	added := mediaTestJID("333333333333333", 43)
	pending := mediaTestJID("444444444444444", 63)
	registry, err := newParticipantReceiveRegistry("CID", callKey, self.String(), peer.String(), func() participantAudioDecoder {
		return &recordingParticipantDecoder{}
	})
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	sendPipe, err := NewMediaPipeline(callKey, self.String(), peer.String(), 0x01020304, FrameSamples)
	if err != nil {
		t.Fatalf("new send pipeline: %v", err)
	}
	if err = registry.attachSendPipeline(sendPipe); err != nil {
		t.Fatalf("attach send pipeline: %v", err)
	}
	if registry.HasCommittedGroupUpdate() {
		t.Fatal("registry reports committed group update before initial roster")
	}
	if err = registry.ApplyGroupUpdate(mediaTestGroupUpdate(self, peer, added, pending, 16, false)); err != nil {
		t.Fatalf("apply initial roster: %v", err)
	}
	if !registry.HasCommittedGroupUpdate() {
		t.Fatal("initial committed roster did not enable group mode")
	}
	initialRaw := bytes.Repeat([]byte{0x16}, 32)
	if err = registry.ApplyGroupRawEpoch(16, initialRaw); err != nil {
		t.Fatalf("apply initial epoch: %v", err)
	}
	nextRaw := bytes.Repeat([]byte{0x18}, 32)
	if err = registry.ApplyGroupRawEpoch(18, nextRaw); err != nil {
		t.Fatalf("buffer next epoch: %v", err)
	}

	initialPeer := registry.byPID[0]
	initialReceiveKeys := initialPeer.pipe.recvKeys
	initialSendKeys := sendPipe.sendKeys
	applyErr := fmt.Errorf("relay unavailable")
	applyCalls := 0
	commitCalls := 0
	update := mediaTestGroupUpdate(self, peer, added, pending, 18, true)
	err = registry.ApplyGroupUpdateTransaction(update, func(func()) error {
		applyCalls++
		return applyErr
	})
	if !errors.Is(err, applyErr) {
		t.Fatalf("failed transaction error = %v, want %v", err, applyErr)
	}
	if applyCalls != 1 {
		t.Fatalf("failed external apply calls = %d, want 1", applyCalls)
	}
	if registry.transactionID != 16 || registry.byPID[0] != initialPeer {
		t.Fatalf("failed transaction changed active roster: tx=%d peer=%p", registry.transactionID, registry.byPID[0])
	}
	if _, ok := registry.byPID[2]; ok {
		t.Fatal("failed transaction installed added participant")
	}
	if registry.installedEpoch.transactionID != 16 || !bytes.Equal(registry.installedEpoch.rawKey, initialRaw) {
		t.Fatalf("failed transaction changed installed epoch: %+v", registry.installedEpoch)
	}
	if got := registry.pendingEpochs[18]; !bytes.Equal(got, nextRaw) {
		t.Fatalf("failed transaction consumed pending epoch: %x", got)
	}
	if initialPeer.pipe.recvKeys != initialReceiveKeys || sendPipe.sendKeys != initialSendKeys {
		t.Fatal("failed transaction changed active media keys")
	}
	if !registry.HasCommittedGroupUpdate() {
		t.Fatal("failed transaction discarded the prior committed group mode")
	}

	err = registry.ApplyGroupUpdateTransaction(update, func(commit func()) error {
		applyCalls++
		commit()
		commitCalls++
		return nil
	})
	if err != nil {
		t.Fatalf("retry transaction: %v", err)
	}
	if applyCalls != 2 || commitCalls != 1 {
		t.Fatalf("retry apply/commit calls = (%d, %d), want (2, 1)", applyCalls, commitCalls)
	}
	if registry.transactionID != 18 || registry.byPID[0] != initialPeer || registry.byPID[2] == nil {
		t.Fatalf("retry did not atomically advance roster: tx=%d pids=%v", registry.transactionID, registry.byPID)
	}
	if registry.installedEpoch.transactionID != 18 || !bytes.Equal(registry.installedEpoch.rawKey, nextRaw) {
		t.Fatalf("retry did not install pending epoch: %+v", registry.installedEpoch)
	}
	if _, ok := registry.pendingEpochs[18]; ok {
		t.Fatal("successful retry retained consumed pending epoch")
	}
	if initialPeer.pipe.recvKeys == initialReceiveKeys || sendPipe.sendKeys == initialSendKeys {
		t.Fatal("successful retry did not rotate active media keys")
	}
	if !registry.HasCommittedGroupUpdate() {
		t.Fatal("successful retry did not enable committed group mode")
	}

	err = registry.ApplyGroupUpdateTransaction(update, func(func()) error {
		applyCalls++
		return nil
	})
	if err != nil {
		t.Fatalf("duplicate transaction: %v", err)
	}
	if applyCalls != 2 || commitCalls != 1 {
		t.Fatalf("duplicate invoked apply/commit: (%d, %d), want (2, 1)", applyCalls, commitCalls)
	}
}

func TestParticipantReceiveRegistryCarriesEpochToLateAttachedSender(t *testing.T) {
	// Source of truth: https://github.com/purpshell/wacaller/blob/cbe1446dabb5842362b1a4362d4100ec15d8254f/datasheets/group-media-key-epoch.md#L104-L136
	callKey := iota32()
	rawKey := bytes.Repeat([]byte{0x5d}, 32)
	self := mediaTestJID("111111111111111", 14)
	peer := mediaTestJID("222222222222222", 0)
	added := mediaTestJID("333333333333333", 43)
	pending := mediaTestJID("444444444444444", 63)
	registry, err := newParticipantReceiveRegistry(
		"CID", callKey, self.String(), peer.String(), nil,
	)
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	if err = registry.ApplyGroupUpdate(
		mediaTestGroupUpdate(self, peer, added, pending, 17, false),
	); err != nil {
		t.Fatalf("apply roster before sender: %v", err)
	}
	if err = registry.ApplyGroupRawEpoch(17, rawKey); err != nil {
		t.Fatalf("install epoch before sender: %v", err)
	}
	sender, err := NewMediaPipeline(
		callKey, self.String(), peer.String(), 0x01020304, FrameSamples,
	)
	if err != nil {
		t.Fatalf("new sender: %v", err)
	}
	if err = registry.attachSendPipeline(sender); err != nil {
		t.Fatalf("attach sender after epoch: %v", err)
	}
	want, err := srtp.DeriveE2eKeysFromRaw(rawKey, rtp.FormatE2ESrtpParticipantID(self.String()))
	if err != nil {
		t.Fatalf("derive expected sender keys: %v", err)
	}
	if sender.sendKeys != want {
		t.Fatal("late-attached sender did not inherit the installed group epoch")
	}
}

func TestParticipantReceiveRegistryDepartureWaitsForInFlightDecode(t *testing.T) {
	callKey := iota32()
	self := mediaTestJID("111111111111111", 14)
	peer := mediaTestJID("222222222222222", 0)
	added := mediaTestJID("333333333333333", 43)
	pending := mediaTestJID("444444444444444", 63)
	addedDecoder := &blockingParticipantDecoder{
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	decoderIndex := 0
	registry, err := newParticipantReceiveRegistry("CID", callKey, self.String(), peer.String(), func() participantAudioDecoder {
		decoderIndex++
		if decoderIndex == 2 {
			return addedDecoder
		}
		return &recordingParticipantDecoder{}
	})
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	if err = registry.ApplyGroupUpdate(mediaTestGroupUpdate(self, peer, added, pending, 16, true)); err != nil {
		t.Fatalf("apply connected update: %v", err)
	}
	addedPacket, _ := protectParticipantAudio(t, callKey, self, added, []byte{0x51})
	decodeDone := make(chan bool, 1)
	go func() {
		_, ok := registry.DecodeAudio(addedPacket)
		decodeDone <- ok
	}()
	<-addedDecoder.entered

	updateDone := make(chan error, 1)
	updateStarted := make(chan struct{})
	go func() {
		close(updateStarted)
		updateDone <- registry.ApplyGroupUpdate(mediaTestGroupUpdate(self, peer, added, pending, 18, false))
	}()
	<-updateStarted
	select {
	case err = <-updateDone:
		t.Fatalf("departure completed during in-flight decode: %v", err)
	case <-time.After(20 * time.Millisecond):
	}

	close(addedDecoder.release)
	if ok := <-decodeDone; !ok {
		t.Fatal("in-flight authenticated decode was rejected")
	}
	if err = <-updateDone; err != nil {
		t.Fatalf("apply departure: %v", err)
	}
	if _, ok := registry.DecodeAudio(addedPacket); ok {
		t.Fatal("departed participant produced audio after pruning completed")
	}
}
