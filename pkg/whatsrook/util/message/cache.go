package message

import (
	"sync"

	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// RecentMessageCache keeps an in-memory cache of recent incoming message events per JID.
type RecentMessageCache struct {
	mu       sync.RWMutex
	messages map[types.JID]*events.Message
}

// GlobalRecentMessages is the global cache of recently received messages.
var GlobalRecentMessages = &RecentMessageCache{
	messages: make(map[types.JID]*events.Message),
}

// RecordRecentMessage caches an incoming message event for participant lookup and reply context.
func RecordRecentMessage(evt *events.Message) {
	if evt == nil || evt.Info.Chat.IsEmpty() {
		return
	}
	GlobalRecentMessages.mu.Lock()
	defer GlobalRecentMessages.mu.Unlock()

	sender := evt.Info.Sender.ToNonAD()
	if !sender.IsEmpty() {
		GlobalRecentMessages.messages[sender] = evt
	}
	if !evt.Info.SenderAlt.IsEmpty() {
		GlobalRecentMessages.messages[evt.Info.SenderAlt.ToNonAD()] = evt
	}
}

// GetRecentMessageForJID returns the most recent message event recorded for a sender JID.
func GetRecentMessageForJID(jid types.JID) *events.Message {
	if jid.IsEmpty() {
		return nil
	}
	GlobalRecentMessages.mu.RLock()
	defer GlobalRecentMessages.mu.RUnlock()

	jidNonAD := jid.ToNonAD()
	if msg, ok := GlobalRecentMessages.messages[jidNonAD]; ok {
		return msg
	}
	for k, v := range GlobalRecentMessages.messages {
		if v != nil && (k.User == jidNonAD.User || v.Info.Sender.ToNonAD().User == jidNonAD.User || v.Info.SenderAlt.ToNonAD().User == jidNonAD.User) {
			return v
		}
	}
	return nil
}
