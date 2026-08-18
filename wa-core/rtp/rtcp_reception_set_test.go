package rtp

import "testing"

func TestGroupAudioReceptionReportsRetainEveryActiveSSRC(t *testing.T) {
	var set RtcpReceptionStatsSet
	const (
		ssrcA = uint32(0x59754A60)
		ssrcC = uint32(0x66FDE7F1)
	)
	set.Observe(ssrcA, 299, 28_704_000, 1_000, 16_000)
	set.Observe(ssrcC, 39, 3_744_000, 1_010, 16_000)
	set.Observe(ssrcA, 300, 28_704_960, 1_060, 16_000)
	set.Observe(ssrcC, 40, 3_744_960, 1_070, 16_000)
	set.ObserveSenderReport(ssrcA, 0x11223344, 0x55667788, 1_100)
	set.ObserveSenderReport(ssrcC, 0x99aabbcc, 0xddeeff00, 1_120)

	reports := set.Reports(1_620)
	if len(reports) != 2 {
		t.Fatalf("report count = %d, want 2", len(reports))
	}
	if reports[0].Ssrc != ssrcA || reports[0].ExtendedHighestSequence != 300 {
		t.Fatalf("A report = %+v", reports[0])
	}
	if reports[0].LastSenderReport != 0x33445566 {
		t.Fatalf("A LSR = %#x, want 0x33445566", reports[0].LastSenderReport)
	}
	if reports[1].Ssrc != ssrcC || reports[1].ExtendedHighestSequence != 40 {
		t.Fatalf("C report = %+v", reports[1])
	}
	if reports[1].LastSenderReport != 0xbbccddee {
		t.Fatalf("C LSR = %#x, want 0xbbccddee", reports[1].LastSenderReport)
	}
}

func TestGroupAudioReceptionReportsIgnoreUnknownSenderReport(t *testing.T) {
	var set RtcpReceptionStatsSet
	set.ObserveSenderReport(0x12345678, 1, 2, 3)
	if reports := set.Reports(4); len(reports) != 0 {
		t.Fatalf("reports = %+v, want none before authenticated RTP", reports)
	}
}

func TestGroupAudioReceptionReportsPruneDepartureAndResetRejoin(t *testing.T) {
	var set RtcpReceptionStatsSet
	const (
		ssrcA = uint32(0x59754A60)
		ssrcC = uint32(0x66FDE7F1)
	)
	set.Observe(ssrcA, 299, 28_704_000, 1_000, 16_000)
	set.Observe(ssrcC, 39, 3_744_000, 1_010, 16_000)

	set.Retain([]uint32{ssrcA})
	reports := set.Reports(1_100)
	if len(reports) != 1 || reports[0].Ssrc != ssrcA {
		t.Fatalf("post-departure reports = %+v, want only A", reports)
	}

	set.Retain([]uint32{ssrcA, ssrcC})
	set.Observe(ssrcC, 900, 8_640_000, 2_000, 16_000)
	reports = set.Reports(2_100)
	if len(reports) != 2 {
		t.Fatalf("post-rejoin report count = %d, want 2", len(reports))
	}
	if reports[1].Ssrc != ssrcC || reports[1].ExtendedHighestSequence != 900 || reports[1].CumulativeLost != 0 {
		t.Fatalf("rejoined C report = %+v, want fresh sequence 900 with no inherited loss", reports[1])
	}
}

func TestGroupAudioReceptionRetainAndObserveAreAuthoritativeInEitherOrder(t *testing.T) {
	const (
		activeSSRC   = uint32(0x59754A60)
		departedSSRC = uint32(0x66FDE7F1)
	)

	t.Run("observe before retain is pruned", func(t *testing.T) {
		var set RtcpReceptionStatsSet
		set.Observe(departedSSRC, 39, 3_744_000, 1_000, 16_000)
		set.Retain([]uint32{activeSSRC})

		if reports := set.Reports(1_100); len(reports) != 0 {
			t.Fatalf("reports after observe-then-retain = %+v, want none", reports)
		}
	})

	t.Run("observe after retain is ignored", func(t *testing.T) {
		var set RtcpReceptionStatsSet
		set.Retain([]uint32{activeSSRC})
		set.Observe(departedSSRC, 39, 3_744_000, 1_000, 16_000)

		if reports := set.Reports(1_100); len(reports) != 0 {
			t.Fatalf("reports after retain-then-observe = %+v, want none", reports)
		}

		set.Retain([]uint32{activeSSRC, departedSSRC})
		set.Observe(departedSSRC, 900, 8_640_000, 2_000, 16_000)
		reports := set.Reports(2_100)
		if len(reports) != 1 || reports[0].Ssrc != departedSSRC ||
			reports[0].ExtendedHighestSequence != 900 || reports[0].CumulativeLost != 0 {
			t.Fatalf("reallowed report = %+v, want fresh departed SSRC sequence 900", reports)
		}
	})
}
