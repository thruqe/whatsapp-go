package calls

import (
	"sync"

	"whatsrook/cmd/store"

	"go.mau.fi/whatsmeow/types"
)

const (
	VoicemailSettingKey = "voicemail_status"
)

type PendingCall struct {
	Target string
	Kind   store.CallMediaKind
}

var (
	PendingCallMu sync.Mutex
	PendingCalls  = map[types.JID]*PendingCall{}
)

func SetPendingCall(sender types.JID, p *PendingCall) {
	PendingCallMu.Lock()
	defer PendingCallMu.Unlock()
	PendingCalls[sender] = p
}

func PeekPendingCall(sender types.JID) (*PendingCall, bool) {
	PendingCallMu.Lock()
	defer PendingCallMu.Unlock()
	p, ok := PendingCalls[sender]
	return p, ok
}

func PopPendingCall(sender types.JID) (*PendingCall, bool) {
	PendingCallMu.Lock()
	defer PendingCallMu.Unlock()
	p, ok := PendingCalls[sender]
	if ok {
		delete(PendingCalls, sender)
	}
	return p, ok
}
