package cliutils

import (
	"regexp"
	"sync"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"go.mau.fi/whatsmeow/types"
)

var (
	TitleCaser           = cases.Title(language.English)
	GroupInviteLinkRegex = regexp.MustCompile(`(?i)(?:^|\s)(?:https?://)?chat\.whatsapp\.com/([A-Za-z0-9_-]+)(?:\b|\s|$)`)

	PresenceMu  sync.RWMutex
	PresenceMap = make(map[string]PresenceInfo)

	AutoMuteSchedulerOnce sync.Once

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

type MuteSchedule struct {
	GroupJID   string `json:"group_jid"`
	MuteTime   string `json:"mute_time"`
	UnmuteTime string `json:"unmute_time"`
	Enabled    bool   `json:"enabled"`
}
