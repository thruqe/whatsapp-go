package utils

import (
	"regexp"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/types"
)

var (
	GroupInviteLinkRegex = regexp.MustCompile(`(?i)(?:^|\s)(?:https?://)?chat\.whatsapp\.com/([A-Za-z0-9_-]+)(?:\b|\s|$)`)

	PresenceMu  sync.RWMutex
	PresenceMap = make(map[string]PresenceInfo)

	SpamTrackMu sync.Mutex
	SpamHistory = make(map[string][]time.Time)
)

type PresenceInfo struct {
	LastSeen time.Time
	IsOnline bool
}

func TrackPresence(jid types.JID, isOnline bool) {
	if jid.IsEmpty() {
		return
	}
	key := jid.ToNonAD().String()
	PresenceMu.Lock()
	PresenceMap[key] = PresenceInfo{
		LastSeen: time.Now(),
		IsOnline: isOnline,
	}
	PresenceMu.Unlock()
}
