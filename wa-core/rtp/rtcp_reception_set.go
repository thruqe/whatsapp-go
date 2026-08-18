package rtp

import (
	"sort"
	"sync"
)

// RtcpReceptionStatsSet retains independent RTCP reception state per inbound
// media SSRC.
type RtcpReceptionStatsSet struct {
	mu            sync.Mutex
	streams       map[uint32]*RtcpReceptionStats
	allowedSSRCs  map[uint32]struct{}
	authoritative bool
}

// Observe records one authenticated RTP packet in its SSRC-specific state.
func (s *RtcpReceptionStatsSet) Observe(
	ssrc uint32,
	sequence uint16,
	rtpTimestamp uint32,
	arrivalMs uint64,
	clockRate uint32,
) {
	// Source of truth: https://github.com/purpshell/wacaller/blob/7594217b4386a1c056d0e3ecd1049b30a1101241/datasheets/group-media-rtcp-feedback.md#L30-L38
	s.mu.Lock()
	// Source of truth: https://github.com/purpshell/wacaller/blob/bab582d4e799292478ccba2f8a86f2164d4737c3/datasheets/group-media-rtcp-feedback.md#L148-L153
	if s.authoritative {
		if _, ok := s.allowedSSRCs[ssrc]; !ok {
			s.mu.Unlock()
			return
		}
	}
	if s.streams == nil {
		s.streams = make(map[uint32]*RtcpReceptionStats)
	}
	stream := s.streams[ssrc]
	if stream == nil {
		stream = &RtcpReceptionStats{}
		s.streams[ssrc] = stream
	}
	s.mu.Unlock()
	stream.Observe(ssrc, sequence, rtpTimestamp, arrivalMs, clockRate)
}

// ObserveSenderReport records sender timing in the matching SSRC-specific state.
func (s *RtcpReceptionStatsSet) ObserveSenderReport(
	senderSSRC uint32,
	ntpSeconds uint32,
	ntpFraction uint32,
	arrivalMs uint64,
) {
	// Source of truth: https://github.com/purpshell/wacaller/blob/7594217b4386a1c056d0e3ecd1049b30a1101241/datasheets/group-media-rtcp-feedback.md#L30-L38
	s.mu.Lock()
	stream := s.streams[senderSSRC]
	s.mu.Unlock()
	if stream == nil {
		return
	}
	stream.ObserveSenderReport(senderSSRC, ntpSeconds, ntpFraction, arrivalMs)
}

// Retain removes reception state for SSRCs absent from the authoritative
// active roster.
func (s *RtcpReceptionStatsSet) Retain(ssrcs []uint32) {
	// Source of truth: https://github.com/purpshell/wacaller/blob/6e202a6d6ec5a9384bae6ccbe621966edeee6592/datasheets/group-media-rtcp-feedback.md#L133-L135
	active := make(map[uint32]struct{}, len(ssrcs))
	for _, ssrc := range ssrcs {
		active[ssrc] = struct{}{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// Source of truth: https://github.com/purpshell/wacaller/blob/bab582d4e799292478ccba2f8a86f2164d4737c3/datasheets/group-media-rtcp-feedback.md#L148-L153
	s.allowedSSRCs = active
	s.authoritative = true
	for ssrc := range s.streams {
		if _, ok := active[ssrc]; !ok {
			delete(s.streams, ssrc)
		}
	}
}

// Reports snapshots every tracked stream in ascending SSRC order.
func (s *RtcpReceptionStatsSet) Reports(nowMs uint64) []*RtcpReceptionReport {
	// Source of truth: https://github.com/purpshell/wacaller/blob/7594217b4386a1c056d0e3ecd1049b30a1101241/datasheets/group-media-rtcp-feedback.md#L64-L69
	s.mu.Lock()
	ssrcs := make([]uint32, 0, len(s.streams))
	streams := make(map[uint32]*RtcpReceptionStats, len(s.streams))
	for ssrc, stream := range s.streams {
		ssrcs = append(ssrcs, ssrc)
		streams[ssrc] = stream
	}
	s.mu.Unlock()
	sort.Slice(ssrcs, func(i, j int) bool {
		return ssrcs[i] < ssrcs[j]
	})
	reports := make([]*RtcpReceptionReport, 0, len(ssrcs))
	for _, ssrc := range ssrcs {
		if report := streams[ssrc].Report(nowMs); report != nil {
			reports = append(reports, report)
		}
	}
	return reports
}
