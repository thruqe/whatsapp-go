package cliutils

import (
	"sync"
	"time"

	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"whatsrook/utils"
)

type RecentMessageEntry struct {
	ID           string
	Chat         types.JID
	Sender       types.JID
	PushName     string
	Text         string
	Timestamp    time.Time
	HasQuoted    bool
	QuotedID     string
	QuotedSender types.JID
	QuotedName   string
	QuotedText   string
}

const MaxRecentMessages = 1000

var (
	RecentMessagesMu sync.RWMutex
	RecentMessages   = make(map[string]RecentMessageEntry)
	RecentMsgOrder   []string
)

func RecordRecentMessage(evt *events.Message) {
	if evt == nil || evt.Message == nil || evt.Info.ID == "" {
		return
	}
	RecentMessagesMu.Lock()
	defer RecentMessagesMu.Unlock()

	sender := evt.Info.Sender
	pushName := evt.Info.PushName
	text := utils.ExtractMessageText(evt)

	entry := RecentMessageEntry{
		ID:        evt.Info.ID,
		Chat:      evt.Info.Chat,
		Sender:    sender,
		PushName:  pushName,
		Text:      text,
		Timestamp: evt.Info.Timestamp,
	}

	ci := utils.GetContextInfoFromProto(evt.Message)
	if ci != nil && ci.QuotedMessage != nil {
		entry.HasQuoted = true
		if ci.StanzaID != nil {
			entry.QuotedID = *ci.StanzaID
		}
		if ci.Participant != nil && *ci.Participant != "" {
			if pj, err := types.ParseJID(*ci.Participant); err == nil {
				entry.QuotedSender = pj
			}
		} else if ci.RemoteJID != nil && *ci.RemoteJID != "" {
			if pj, err := types.ParseJID(*ci.RemoteJID); err == nil {
				entry.QuotedSender = pj
			}
		}
		entry.QuotedText = utils.ExtractTextFromProto(ci.QuotedMessage)
	}

	if _, exists := RecentMessages[evt.Info.ID]; !exists {
		if len(RecentMsgOrder) >= MaxRecentMessages {
			oldest := RecentMsgOrder[0]
			RecentMsgOrder = RecentMsgOrder[1:]
			delete(RecentMessages, oldest)
		}
		RecentMsgOrder = append(RecentMsgOrder, evt.Info.ID)
	}
	RecentMessages[evt.Info.ID] = entry
}

func GetRecentMessage(id string) (RecentMessageEntry, bool) {
	RecentMessagesMu.RLock()
	defer RecentMessagesMu.RUnlock()
	entry, ok := RecentMessages[id]
	return entry, ok
}
