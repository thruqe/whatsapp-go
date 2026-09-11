package whatsmeow

import "testing"

type constantFrameParticipantDecoder struct {
	value float32
}

func (d *constantFrameParticipantDecoder) Decode([]byte) []float32 {
	return constantPCM(FrameSamples, d.value)
}

func constantPCM(samples int, value float32) []float32 {
	pcm := make([]float32, samples)
	for i := range pcm {
		pcm[i] = value
	}
	return pcm
}

func TestParticipantAudioMixerPreservesSingleSpeakerGainAfterPrefill(t *testing.T) {
	mixer := newParticipantAudioMixer()
	if !mixer.Add("peer", constantPCM(FrameSamples, 0.25)) {
		t.Fatal("first peer frame was rejected")
	}
	if chunk, ok := mixer.MixChunk(); ok || chunk != nil {
		t.Fatalf("mix before prefill = (%d samples, %v), want no chunk", len(chunk), ok)
	}
	if !mixer.Add("peer", constantPCM(FrameSamples, 0.25)) {
		t.Fatal("second peer frame was rejected")
	}
	chunk, ok := mixer.MixChunk()
	if !ok || len(chunk) != participantAudioMixChunkSamples {
		t.Fatalf("mixed chunk = (%d samples, %v)", len(chunk), ok)
	}
	for i, sample := range chunk {
		if sample != 0.25 {
			t.Fatalf("single-speaker sample %d = %f, want 0.25", i, sample)
		}
	}
}

func TestParticipantAudioMixerSumsConcurrentStreamsAndClamps(t *testing.T) {
	mixer := newParticipantAudioMixer()
	peer := constantPCM(participantAudioMixerPrefillSamples, 0.4)
	added := constantPCM(participantAudioMixerPrefillSamples, 0.7)
	for i := 1; i < len(peer); i += 2 {
		peer[i] = -0.6
		added[i] = -0.7
	}
	mixer.Add("peer", peer)
	mixer.Add("added", added)

	chunk, ok := mixer.MixChunk()
	if !ok || len(chunk) != participantAudioMixChunkSamples {
		t.Fatalf("mixed chunk = (%d samples, %v)", len(chunk), ok)
	}
	for i, sample := range chunk {
		want := float32(1)
		if i%2 == 1 {
			want = -1
		}
		if sample != want {
			t.Fatalf("mixed sample %d = %f, want %f", i, sample, want)
		}
	}
}

func TestParticipantAudioMixerDoesNotBlockStartedPeerForUnreadyParticipant(t *testing.T) {
	mixer := newParticipantAudioMixer()
	mixer.Add("peer", constantPCM(participantAudioMixerPrefillSamples, 0.2))
	mixer.Add("added", constantPCM(FrameSamples, 0.7))

	chunk, ok := mixer.MixChunk()
	if !ok {
		t.Fatal("started peer was blocked by an unready added participant")
	}
	for i, sample := range chunk {
		if sample != 0.2 {
			t.Fatalf("peer-only sample %d = %f, want 0.2", i, sample)
		}
	}

	mixer.Add("added", constantPCM(FrameSamples, 0.7))
	chunk, ok = mixer.MixChunk()
	if !ok {
		t.Fatal("ready participants did not produce a chunk")
	}
	for i, sample := range chunk {
		if sample != 0.9 {
			t.Fatalf("joined sample %d = %f, want 0.9", i, sample)
		}
	}
}

func TestParticipantAudioMixerRetainRemovesAndGatesDepartedParticipant(t *testing.T) {
	mixer := newParticipantAudioMixer()
	mixer.Retain([]string{"peer", "added"})
	mixer.Add("peer", constantPCM(participantAudioMixerPrefillSamples, 0.2))
	mixer.Add("added", constantPCM(participantAudioMixerPrefillSamples, 0.8))

	mixer.Retain([]string{"peer"})
	if mixer.Add("added", constantPCM(FrameSamples, 0.8)) {
		t.Fatal("late departed-participant PCM was accepted")
	}
	chunk, ok := mixer.MixChunk()
	if !ok {
		t.Fatal("remaining peer did not produce a chunk")
	}
	for i, sample := range chunk {
		if sample != 0.2 {
			t.Fatalf("post-departure sample %d = %f, want peer-only 0.2", i, sample)
		}
	}
}

func TestParticipantAudioMixerBoundsQueueByDroppingOldestAudio(t *testing.T) {
	mixer := newParticipantAudioMixer()
	pcm := append(
		constantPCM(FrameSamples, 0.1),
		constantPCM(participantAudioMixerMaxSamples, 0.5)...,
	)
	if !mixer.Add("peer", pcm) {
		t.Fatal("peer PCM was rejected")
	}
	pcm[FrameSamples] = 0.9

	chunk, ok := mixer.MixChunk()
	if !ok {
		t.Fatal("bounded queue did not produce a chunk")
	}
	for i, sample := range chunk {
		if sample != 0.5 {
			t.Fatalf("bounded sample %d = %f, want retained newest audio 0.5", i, sample)
		}
	}
}

func TestParticipantReceiveRegistryFeedsAddedParticipantMixer(t *testing.T) {
	callKey := iota32()
	self := mediaTestJID("111111111111111", 14)
	peer := mediaTestJID("222222222222222", 0)
	added := mediaTestJID("333333333333333", 43)
	pending := mediaTestJID("444444444444444", 63)
	decoderIndex := 0
	registry, err := newParticipantReceiveRegistry("CID", callKey, self.String(), peer.String(), func() participantAudioDecoder {
		decoderIndex++
		value := float32(0.2)
		if decoderIndex == 2 {
			value = 0.7
		}
		return &constantFrameParticipantDecoder{value: value}
	})
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	if err = registry.ApplyGroupUpdate(mediaTestGroupUpdate(self, peer, added, pending, 16, true)); err != nil {
		t.Fatalf("apply group update: %v", err)
	}
	mixer := newParticipantAudioMixer()
	mixer.Retain(registry.ActiveParticipantIDs())
	for range 2 {
		peerPacket, _ := protectParticipantAudio(t, callKey, self, peer, []byte{0x11})
		peerAudio, ok := registry.DecodeAudio(peerPacket)
		if !ok || !mixer.Add(peerAudio.ParticipantID, peerAudio.PCM) {
			t.Fatal("original-peer media did not reach mixer")
		}
		addedPacket, _ := protectParticipantAudio(t, callKey, self, added, []byte{0x22})
		addedAudio, ok := registry.DecodeAudio(addedPacket)
		if !ok || !mixer.Add(addedAudio.ParticipantID, addedAudio.PCM) {
			t.Fatal("added-participant media did not reach mixer")
		}
	}
	chunk, ok := mixer.MixChunk()
	if !ok {
		t.Fatal("participant media did not produce mixed playout")
	}
	for i, sample := range chunk {
		if sample != 0.9 {
			t.Fatalf("registry-to-mixer sample %d = %f, want 0.9", i, sample)
		}
	}
}

func TestParticipantAudioSinkFramerPreservesPublicFrameContract(t *testing.T) {
	var framer participantAudioSinkFramer
	for chunkIndex := range FrameSamples / participantAudioMixChunkSamples {
		chunk := constantPCM(participantAudioMixChunkSamples, float32(chunkIndex+1)/10)
		frame, ok := framer.Push(chunk)
		if chunkIndex < FrameSamples/participantAudioMixChunkSamples-1 {
			if ok || frame != nil {
				t.Fatalf("sink frame became ready after chunk %d", chunkIndex)
			}
			continue
		}
		if !ok || len(frame) != FrameSamples {
			t.Fatalf("public sink frame = (%d samples, %v), want (%d, true)", len(frame), ok, FrameSamples)
		}
		for i, sample := range frame {
			want := float32(i/participantAudioMixChunkSamples+1) / 10
			if sample != want {
				t.Fatalf("public sink sample %d = %f, want %f", i, sample, want)
			}
		}
	}
}

func TestParticipantAudioMixerTransitionWaitsForSecondRemote(t *testing.T) {
	playout := newAudioPlayoutBuffer()
	sink := &playoutTestSink{}
	if _, err := playout.Push(0, constantPCM(FrameSamples, 0.2), sink); err != nil {
		t.Fatal(err)
	}
	if _, err := playout.Push(FrameSamples, constantPCM(FrameSamples, 0.3), sink); err != nil {
		t.Fatal(err)
	}
	if shouldStartParticipantMixing([]string{"peer"}) {
		t.Fatal("invite-stage roster with one remote enabled group mixing")
	}
	if len(sink.frames) != 0 || playout.pending == nil || len(playout.prefill) != 1 {
		t.Fatal("invite-stage roster disturbed buffered direct audio")
	}
	if !shouldStartParticipantMixing([]string{"peer", "added"}) {
		t.Fatal("connected second remote did not enable group mixing")
	}
	if err := playout.Drain(sink); err != nil {
		t.Fatalf("transition drain: %v", err)
	}
	if got := frameLengths(sink.frames); len(got) != 2 || got[0] != FrameSamples || got[1] != FrameSamples {
		t.Fatalf("transition handoff frames = %v, want [%d %d]", got, FrameSamples, FrameSamples)
	}
}
