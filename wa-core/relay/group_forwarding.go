package relay

const groupForwardingMarker byte = 0x09

// UnwrapGroupForwardingPacket removes a captured multi-participant relay
// forwarding header. Non-forwarded packets are returned unchanged.
func UnwrapGroupForwardingPacket(data []byte) (payload []byte, wrapped, valid bool) {
	// Source of truth: https://github.com/purpshell/wacaller/blob/4db38f0ea0645ac8449a105ffd9aa30c6e269689/diag/analysis/group-call-84987F9DE404B79ED999E6F254B0150A.md#L93-L100
	if len(data) == 0 || data[0] != groupForwardingMarker {
		return data, false, true
	}
	if len(data) < 2 {
		return nil, true, false
	}
	var headerBytes int
	switch data[1] {
	case 2:
		headerBytes = 8
	case 4:
		headerBytes = 12
	case 7:
		headerBytes = 18
	default:
		return nil, true, false
	}
	if len(data) < headerBytes+12 || data[headerBytes]>>6 != 2 {
		return nil, true, false
	}
	return data[headerBytes:], true, true
}
