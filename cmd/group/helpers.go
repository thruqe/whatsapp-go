package group

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	utils "whatsrook"
	"whatsrook/cmd/dispatch"
	Logger "whatsrook/logger"

	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

func sendPollReply(ctx *dispatch.Context, body string, options []string) error {
	return dispatch.SendPollReply(ctx, body, options)
}

func sendPollReplyWithMentions(ctx *dispatch.Context, body string, options []string, mentions []types.JID) error {
	return dispatch.SendPollReplyWithMentions(ctx, body, options, mentions)
}

func splitCSV(s string) []string {
	var parts []string
	for p := range strings.SplitSeq(s, ",") {
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

// HandleGroupModeration evaluates active group moderation filters (AntiMsg, AntiSpam, AntiLink, AntiWord).
func HandleGroupModeration(c *dispatch.Context, text string) bool {
	if c.Chat.Server != "g.us" {
		return false
	}
	ctx := c.Ctx
	client := c.Client
	evt := c.Evt
	chatStr := c.Chat.String()
	sender := c.Sender.ToNonAD()

	s, ok := dispatch.GetStore(c)
	if !ok {
		return false
	}

	// 1. AntiMsg: check if sender is on antimsg list
	if !c.IsSudo() {
		rawAntiMsgStatus, _ := s.GetSetting(ctx, "antimsg_status:"+chatStr)
		if rawAntiMsgStatus == "on" {
			rawAntiMsgUsers, _ := s.GetSetting(ctx, "antimsg_users:"+chatStr)
			if rawAntiMsgUsers != "" {
				targetUsers := strings.SplitSeq(rawAntiMsgUsers, ",")
				for uStr := range targetUsers {
					uStr = strings.TrimSpace(uStr)
					if uStr == "" {
						continue
					}
					uJID, err := types.ParseJID(uStr)
					if err != nil {
						continue
					}
					if utils.IsSameUserRaw(ctx, client, uJID, c.Sender) {
						Logger.Debug("antimsg: deleting message from targeted participant", "chat", chatStr, "sender", c.Sender.String())
						_, _ = client.SendMessage(ctx, c.Chat, client.BuildRevoke(c.Chat, c.Sender, evt.Info.ID))
						return true
					}
				}
			}
		}
	}

	// 2. AntiSpam
	rawAntiSpamStatus, _ := s.GetSetting(ctx, "antispam_status:"+chatStr)
	if rawAntiSpamStatus == "on" {
		info, err := client.GetGroupInfo(ctx, c.Chat)
		if err == nil && !utils.IsAdminRaw(ctx, client, info, sender) && !c.IsSudo() {
			rawMax, _ := s.GetSetting(ctx, "antispam_max:"+chatStr)
			maxMsgs, _ := strconv.Atoi(rawMax)
			if maxMsgs <= 0 {
				maxMsgs = 5
			}
			if checkSpamLimit(chatStr, sender.String(), maxMsgs) {
				action, _ := s.GetSetting(ctx, "antispam_action:"+chatStr)
				if action == "" {
					action = "delete"
				}
				botIsAdmin := false
				if client.Store.ID != nil {
					botIsAdmin = utils.IsAdminRaw(ctx, client, info, *client.Store.ID)
				}
				if botIsAdmin {
					_, _ = client.SendMessage(ctx, c.Chat, client.BuildRevoke(c.Chat, c.Sender, evt.Info.ID))
					if action == "kick" {
						_, _ = client.UpdateGroupParticipants(ctx, c.Chat, []types.JID{c.Sender}, whatsmeow.ParticipantChangeRemove)
					}
					resolvedJID, username := utils.ResolveMentionRaw(ctx, client, c.Sender)
					textMsg := dispatch.Sprintf("AntiSpam: @%s message rate limit exceeded (action: %s).", username, action)
					formatted := utils.FormatTextResponseRaw(textMsg)
					_, _ = client.SendMessage(ctx, c.Chat, &waE2E.Message{
						ExtendedTextMessage: &waE2E.ExtendedTextMessage{
							Text: &formatted,
							ContextInfo: &waE2E.ContextInfo{
								MentionedJID: []string{resolvedJID.String()},
							},
						},
					})
					return true
				}
			}
		}
	}

	// 3. AntiLink & AntiWord
	antiLinkEnabled := false
	rawLink, _ := s.GetSetting(ctx, "antilink:"+chatStr)
	if rawLink == "on" {
		antiLinkEnabled = true
	}

	var bannedWords []string
	rawWord, _ := s.GetSetting(ctx, "antiword:"+chatStr)
	if rawWord != "" {
		bannedWords = strings.Fields(strings.ToLower(rawWord))
	}

	if !antiLinkEnabled && len(bannedWords) == 0 {
		return false
	}

	info, err := client.GetGroupInfo(ctx, c.Chat)
	if err != nil {
		return false
	}

	if utils.IsAdminRaw(ctx, client, info, sender) || c.IsSudo() {
		return false
	}

	violation := false
	reason := ""
	violationType := ""

	if antiLinkEnabled {
		lowerText := strings.ToLower(text)
		mode, _ := s.GetSetting(ctx, "antilink_mode:"+chatStr)
		if mode == "custom" {
			customStr, _ := s.GetSetting(ctx, "antilink_custom:"+chatStr)
			if customStr == "" {
				customStr = "chat.whatsapp.com"
			}
			domains := strings.SplitSeq(customStr, ",")
			for d := range domains {
				d = strings.TrimSpace(strings.ToLower(d))
				if d != "" && strings.Contains(lowerText, d) {
					violation = true
					reason = dispatch.Sprintf("banned link (%s)", d)
					violationType = "antilink"
					break
				}
			}
		} else {
			if strings.Contains(lowerText, "http://") || strings.Contains(lowerText, "https://") || strings.Contains(lowerText, "chat.whatsapp.com") || strings.Contains(lowerText, "t.me") || strings.Contains(lowerText, "www.") || strings.Contains(lowerText, ".com") || strings.Contains(lowerText, ".net") || strings.Contains(lowerText, ".org") {
				violation = true
				reason = "links"
				violationType = "antilink"
			}
		}
	}

	if !violation && len(bannedWords) > 0 {
		lowerText := strings.ToLower(text)
		for _, w := range bannedWords {
			if strings.Contains(lowerText, w) {
				violation = true
				reason = dispatch.Sprintf("banned word (%s)", w)
				violationType = "antiword"
				break
			}
		}
	}

	if violation {
		botIsAdmin := false
		if client.Store.ID != nil {
			botIsAdmin = utils.IsAdminRaw(ctx, client, info, *client.Store.ID)
		}

		if botIsAdmin {
			_, _ = client.SendMessage(ctx, c.Chat, client.BuildRevoke(c.Chat, c.Sender, evt.Info.ID))
			resolvedJID, username := utils.ResolveMentionRaw(ctx, client, c.Sender)

			actionKey := violationType + "_action:" + chatStr
			action, _ := s.GetSetting(ctx, actionKey)
			action = strings.ToLower(strings.TrimSpace(action))

			switch action {
			case "kick":
				_, _ = client.UpdateGroupParticipants(ctx, c.Chat, []types.JID{c.Sender}, whatsmeow.ParticipantChangeRemove)
				textMsg := dispatch.Sprintf("Message from @%s deleted and participant kicked: contains %s.", username, reason)
				formatted := utils.FormatTextResponseRaw(textMsg)
				_, _ = client.SendMessage(ctx, c.Chat, &waE2E.Message{
					ExtendedTextMessage: &waE2E.ExtendedTextMessage{
						Text: &formatted,
						ContextInfo: &waE2E.ContextInfo{
							MentionedJID: []string{resolvedJID.String()},
						},
					},
				})

			case "warn":
				maxWarnKey := violationType + "_maxwarn:" + chatStr
				maxWarnStr, _ := s.GetSetting(ctx, maxWarnKey)
				maxWarn := 3
				if parsed, err := strconv.Atoi(maxWarnStr); err == nil && parsed > 0 {
					maxWarn = parsed
				}

				warnsKey := violationType + "_warns:" + chatStr + ":" + c.Sender.ToNonAD().String()
				currWarnStr, _ := s.GetSetting(ctx, warnsKey)
				currWarns := 0
				if parsed, err := strconv.Atoi(currWarnStr); err == nil {
					currWarns = parsed
				}
				currWarns++

				if currWarns >= maxWarn {
					_, _ = client.UpdateGroupParticipants(ctx, c.Chat, []types.JID{c.Sender}, whatsmeow.ParticipantChangeRemove)
					_ = s.PutSetting(ctx, warnsKey, "0")
					textMsg := dispatch.Sprintf("⚠️ @%s reached maximum warnings (%d/%d) for %s! Message deleted and participant kicked.", username, currWarns, maxWarn, reason)
					formatted := utils.FormatTextResponseRaw(textMsg)
					_, _ = client.SendMessage(ctx, c.Chat, &waE2E.Message{
						ExtendedTextMessage: &waE2E.ExtendedTextMessage{
							Text: &formatted,
							ContextInfo: &waE2E.ContextInfo{
								MentionedJID: []string{resolvedJID.String()},
							},
						},
					})
				} else {
					_ = s.PutSetting(ctx, warnsKey, strconv.Itoa(currWarns))
					textMsg := dispatch.Sprintf("⚠️ Warning for @%s (%d/%d): Message deleted for %s. Reaching %d warnings will result in a kick!", username, currWarns, maxWarn, reason, maxWarn)
					formatted := utils.FormatTextResponseRaw(textMsg)
					_, _ = client.SendMessage(ctx, c.Chat, &waE2E.Message{
						ExtendedTextMessage: &waE2E.ExtendedTextMessage{
							Text: &formatted,
							ContextInfo: &waE2E.ContextInfo{
								MentionedJID: []string{resolvedJID.String()},
							},
						},
					})
				}

			default:
				textMsg := dispatch.Sprintf("Message from @%s deleted: contains %s.", username, reason)
				formatted := utils.FormatTextResponseRaw(textMsg)
				_, _ = client.SendMessage(ctx, c.Chat, &waE2E.Message{
					ExtendedTextMessage: &waE2E.ExtendedTextMessage{
						Text: &formatted,
						ContextInfo: &waE2E.ContextInfo{
							MentionedJID: []string{resolvedJID.String()},
						},
					},
				})
			}
			return true
		}
	}

	return false
}

func checkSpamLimit(chatStr, senderStr string, maxMsgs int) bool {
	SpamTrackMu.Lock()
	defer SpamTrackMu.Unlock()

	key := chatStr + ":" + senderStr
	now := time.Now()
	cutoff := now.Add(-5 * time.Second)

	history := SpamHistory[key]
	var recent []time.Time
	for _, t := range history {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	recent = append(recent, now)
	SpamHistory[key] = recent

	return len(recent) > maxMsgs
}
