package group

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"sync"
	"time"

	utils "whatsrook"
	"whatsrook/cmd/dispatch"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

func sendPollReply(ctx *dispatch.Context, body string, options []string) error {
	poll := ctx.Poll(body)
	for _, opt := range options {
		poll.AddOption(opt)
	}
	return poll.Reply()
}

func sendPollReplyWithMentions(ctx *dispatch.Context, body string, options []string, mentions []types.JID) error {
	poll := ctx.Poll(body).Mentions(mentions...)
	for _, opt := range options {
		poll.AddOption(opt)
	}
	return poll.Reply()
}

func splitCSV(s string) []string {
	var parts []string
	for _, p := range strings.Split(s, ",") {
		if tr := strings.TrimSpace(p); tr != "" {
			parts = append(parts, tr)
		}
	}
	return parts
}

func NormalizeUserJID(_ any, _ any, jid types.JID) types.JID {
	return jid.ToNonAD()
}

func parseUserJID(raw string) (types.JID, error) {
	clean := strings.TrimLeft(strings.TrimSpace(raw), "@+")
	if clean == "" {
		return types.EmptyJID, errors.New("empty JID")
	}
	if strings.Contains(clean, "@") {
		return types.ParseJID(clean)
	}
	return types.NewJID(clean, types.DefaultUserServer), nil
}

func getUserTimezone(ctx context.Context, s *dispatch.StoreWrapper) string {
	if s != nil {
		if tz, err := s.GetSetting(ctx, "timezone"); err == nil && tz != "" {
			return tz
		}
	}
	return "UTC"
}

func ExtractMediaFromEvent(evt *events.Message) (whatsmeow.DownloadableMessage, bool, string) {
	if evt == nil || evt.Message == nil {
		return nil, false, ""
	}
	msg := utils.UnwrapMessageProto(evt.Message)
	if msg == nil {
		return nil, false, ""
	}
	if img := msg.GetImageMessage(); img != nil {
		return img, false, img.GetMimetype()
	}
	if vid := msg.GetVideoMessage(); vid != nil {
		return vid, true, vid.GetMimetype()
	}
	return nil, false, ""
}

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
