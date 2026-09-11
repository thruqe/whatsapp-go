package whatsmeow

import (
	"slices"
	"sync"
)

const (
	participantAudioMixChunkSamples     = SampleRate / 100
	participantAudioMixerPrefillSamples = 2 * FrameSamples
	participantAudioMixerMaxSamples     = 4 * FrameSamples
)

type participantAudioMixQueue struct {
	pending []float32
	started bool
}

type participantAudioMixer struct {
	mu            sync.Mutex
	streams       map[string]*participantAudioMixQueue
	allowed       map[string]struct{}
	rosterApplied bool
}

type participantAudioSinkFramer struct {
	pending []float32
}

func newParticipantAudioMixer() *participantAudioMixer {
	// Source of truth: https://github.com/purpshell/wacaller/blob/fabad4acce2147da4e40c1e8c6a1643053ae8c59/datasheets/group-audio-mixer.md#L29-L38
	// NOT VALIDATED: validated once live group-call playout includes every connected remote participant.
	return &participantAudioMixer{
		streams: make(map[string]*participantAudioMixQueue),
		allowed: make(map[string]struct{}),
	}
}

func shouldStartParticipantMixing(activeParticipantIDs []string) bool {
	// Source of truth: https://github.com/purpshell/wacaller/blob/ca4ba64503efeb86c337ee37cb00c4da540c632c/datasheets/group-media-receive.md#L24-L35
	return len(activeParticipantIDs) > 1
}

func (m *participantAudioMixer) Add(participantID string, pcm []float32) bool {
	// Source of truth: https://github.com/purpshell/wacaller/blob/fabad4acce2147da4e40c1e8c6a1643053ae8c59/datasheets/group-audio-mixer.md#L31-L37
	// NOT VALIDATED: validated once live group-call playout includes every connected remote participant.
	if participantID == "" || len(pcm) == 0 {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.rosterApplied {
		if _, ok := m.allowed[participantID]; !ok {
			return false
		}
	}
	queue := m.streams[participantID]
	if queue == nil {
		queue = &participantAudioMixQueue{}
		m.streams[participantID] = queue
	}
	if len(pcm) >= participantAudioMixerMaxSamples {
		queue.pending = append(queue.pending[:0], pcm[len(pcm)-participantAudioMixerMaxSamples:]...)
		return true
	}
	overflow := len(queue.pending) + len(pcm) - participantAudioMixerMaxSamples
	if overflow > 0 {
		queue.pending = append([]float32(nil), queue.pending[overflow:]...)
	}
	queue.pending = append(queue.pending, pcm...)
	return true
}

func (m *participantAudioMixer) Retain(participantIDs []string) {
	// Source of truth: https://github.com/purpshell/wacaller/blob/fabad4acce2147da4e40c1e8c6a1643053ae8c59/datasheets/group-audio-mixer.md#L35-L37
	// NOT VALIDATED: validated once a departed participant stops contributing to live playout.
	allowed := make(map[string]struct{}, len(participantIDs))
	for _, participantID := range participantIDs {
		if participantID != "" {
			allowed[participantID] = struct{}{}
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.allowed = allowed
	m.rosterApplied = true
	for participantID := range m.streams {
		if _, ok := allowed[participantID]; !ok {
			delete(m.streams, participantID)
		}
	}
}

func (m *participantAudioMixer) MixChunk() ([]float32, bool) {
	// Source of truth: https://github.com/purpshell/wacaller/blob/fabad4acce2147da4e40c1e8c6a1643053ae8c59/datasheets/group-audio-mixer.md#L31-L38
	// NOT VALIDATED: validated once two live remote speakers are simultaneously audible without extending playout duration.
	m.mu.Lock()
	defer m.mu.Unlock()

	participantIDs := make([]string, 0, len(m.streams))
	for participantID := range m.streams {
		participantIDs = append(participantIDs, participantID)
	}
	slices.Sort(participantIDs)

	mixed := make([]float32, participantAudioMixChunkSamples)
	contributed := false
	for _, participantID := range participantIDs {
		queue := m.streams[participantID]
		if !queue.started {
			if len(queue.pending) < participantAudioMixerPrefillSamples {
				continue
			}
			queue.started = true
		}
		if len(queue.pending) < participantAudioMixChunkSamples {
			queue.started = false
			continue
		}
		for i := range mixed {
			mixed[i] += queue.pending[i]
		}
		queue.pending = queue.pending[participantAudioMixChunkSamples:]
		if len(queue.pending) == 0 {
			queue.pending = nil
		}
		contributed = true
	}
	if !contributed {
		return nil, false
	}
	for i, sample := range mixed {
		if sample > 1 {
			mixed[i] = 1
		} else if sample < -1 {
			mixed[i] = -1
		}
	}
	return mixed, true
}

func (f *participantAudioSinkFramer) Push(chunk []float32) ([]float32, bool) {
	// Source of truth: https://github.com/purpshell/wacaller/blob/fabad4acce2147da4e40c1e8c6a1643053ae8c59/datasheets/group-audio-mixer.md#L20-L26
	// NOT VALIDATED: validated once live mixed playout reaches a public AudioSink in 960-sample frames.
	f.pending = append(f.pending, chunk...)
	if len(f.pending) < FrameSamples {
		return nil, false
	}
	frame := append([]float32(nil), f.pending[:FrameSamples]...)
	f.pending = f.pending[FrameSamples:]
	if len(f.pending) == 0 {
		f.pending = nil
	}
	return frame, true
}
