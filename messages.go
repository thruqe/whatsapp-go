package whatsrook

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"sync"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

// GetMediaType returns the human-readable media classification string.
func GetMediaType(msg *waE2E.Message) string {
	if msg == nil {
		return "text"
	}
	if msg.ImageMessage != nil {
		return "image"
	}
	if msg.VideoMessage != nil {
		if msg.VideoMessage.GetGifPlayback() {
			return "gif"
		}
		return "video"
	}
	if msg.AudioMessage != nil {
		if msg.AudioMessage.GetPTT() {
			return "voice"
		}
		return "audio"
	}
	if msg.DocumentMessage != nil {
		return "document"
	}
	if msg.StickerMessage != nil {
		return "sticker"
	}
	return "text"
}

// AttachContextInfo attaches ContextInfo metadata to inner proto payloads.
func AttachContextInfo(msg *waE2E.Message, ci *waE2E.ContextInfo) {
	if msg == nil || ci == nil {
		return
	}
	if msg.ExtendedTextMessage != nil {
		msg.ExtendedTextMessage.ContextInfo = ci
	} else if msg.ImageMessage != nil {
		msg.ImageMessage.ContextInfo = ci
	} else if msg.VideoMessage != nil {
		msg.VideoMessage.ContextInfo = ci
	} else if msg.AudioMessage != nil {
		msg.AudioMessage.ContextInfo = ci
	} else if msg.DocumentMessage != nil {
		msg.DocumentMessage.ContextInfo = ci
	} else if msg.StickerMessage != nil {
		msg.StickerMessage.ContextInfo = ci
	} else if msg.Conversation != nil {
		text := *msg.Conversation
		msg.Conversation = nil
		msg.ExtendedTextMessage = &waE2E.ExtendedTextMessage{
			Text:        &text,
			ContextInfo: ci,
		}
	}
}

// stripContextInfo zeroes out any nested ContextInfo within a message protobuf to eliminate cyclic pointer graphs.
func stripContextInfo(msg *waE2E.Message) {
	if msg == nil {
		return
	}
	if ext := msg.ExtendedTextMessage; ext != nil {
		ext.ContextInfo = nil
	}
	if img := msg.ImageMessage; img != nil {
		img.ContextInfo = nil
	}
	if vid := msg.VideoMessage; vid != nil {
		vid.ContextInfo = nil
	}
	if aud := msg.AudioMessage; aud != nil {
		aud.ContextInfo = nil
	}
	if doc := msg.DocumentMessage; doc != nil {
		doc.ContextInfo = nil
	}
	if stk := msg.StickerMessage; stk != nil {
		stk.ContextInfo = nil
	}
	if btn := msg.ButtonsMessage; btn != nil {
		btn.ContextInfo = nil
	}
	if btnResp := msg.ButtonsResponseMessage; btnResp != nil {
		btnResp.ContextInfo = nil
	}
	if list := msg.ListResponseMessage; list != nil {
		list.ContextInfo = nil
	}
	if poll := msg.PollCreationMessage; poll != nil {
		poll.ContextInfo = nil
	}
}

// ─── Protocol Helper Utilities ─────────────────────────────────────────────

// UnwrapMessageProto unwraps nested message envelopes (ViewOnce, Ephemeral, Edited, Forwarded, BotInvoke, etc.).
func UnwrapMessageProto(msg *waE2E.Message) *waE2E.Message {
	if msg == nil {
		return nil
	}
	for {
		if ephem := msg.GetEphemeralMessage(); ephem != nil && ephem.GetMessage() != nil {
			msg = ephem.GetMessage()
			continue
		}
		if vo := msg.GetViewOnceMessage(); vo != nil && vo.GetMessage() != nil {
			msg = vo.GetMessage()
			continue
		}
		if vo2 := msg.GetViewOnceMessageV2(); vo2 != nil && vo2.GetMessage() != nil {
			msg = vo2.GetMessage()
			continue
		}
		if vo2ext := msg.GetViewOnceMessageV2Extension(); vo2ext != nil && vo2ext.GetMessage() != nil {
			msg = vo2ext.GetMessage()
			continue
		}
		if docCap := msg.GetDocumentWithCaptionMessage(); docCap != nil && docCap.GetMessage() != nil {
			msg = docCap.GetMessage()
			continue
		}
		if edited := msg.GetEditedMessage(); edited != nil && edited.GetMessage() != nil {
			msg = edited.GetMessage()
			continue
		}
		if pm := msg.GetProtocolMessage(); pm != nil && pm.GetEditedMessage() != nil {
			msg = pm.GetEditedMessage()
			continue
		}
		if bfm := msg.GetBotForwardedMessage(); bfm != nil && bfm.GetMessage() != nil {
			msg = bfm.GetMessage()
			continue
		}
		if bim := msg.GetBotInvokeMessage(); bim != nil && bim.GetMessage() != nil {
			msg = bim.GetMessage()
			continue
		}
		if gmm := msg.GetGroupMentionedMessage(); gmm != nil && gmm.GetMessage() != nil {
			msg = gmm.GetMessage()
			continue
		}
		if acm := msg.GetAssociatedChildMessage(); acm != nil && acm.GetMessage() != nil {
			msg = acm.GetMessage()
			continue
		}
		if smm := msg.GetStatusMentionMessage(); smm != nil && smm.GetMessage() != nil {
			msg = smm.GetMessage()
			continue
		}
		if lsm := msg.GetLimitSharingMessage(); lsm != nil && lsm.GetMessage() != nil {
			msg = lsm.GetMessage()
			continue
		}
		if devSent := msg.GetDeviceSentMessage(); devSent != nil && devSent.GetMessage() != nil {
			msg = devSent.GetMessage()
			continue
		}
		if lot := msg.GetLottieStickerMessage(); lot != nil && lot.GetMessage() != nil {
			msg = lot.GetMessage()
			continue
		}
		if spm := msg.GetSpoilerMessage(); spm != nil && spm.GetMessage() != nil {
			msg = spm.GetMessage()
			continue
		}
		if eci := msg.GetEventCoverImage(); eci != nil && eci.GetMessage() != nil {
			msg = eci.GetMessage()
			continue
		}
		if gsm := msg.GetGroupStatusMessage(); gsm != nil && gsm.GetMessage() != nil {
			msg = gsm.GetMessage()
			continue
		}
		if gsm2 := msg.GetGroupStatusMessageV2(); gsm2 != nil && gsm2.GetMessage() != nil {
			msg = gsm2.GetMessage()
			continue
		}
		if gsmm := msg.GetGroupStatusMentionMessage(); gsmm != nil && gsmm.GetMessage() != nil {
			msg = gsmm.GetMessage()
			continue
		}
		if qrm := msg.GetQuestionReplyMessage(); qrm != nil && qrm.GetMessage() != nil {
			msg = qrm.GetMessage()
			continue
		}
		if pcm4 := msg.GetPollCreationMessageV4(); pcm4 != nil && pcm4.GetMessage() != nil {
			msg = pcm4.GetMessage()
			continue
		}
		break
	}
	return msg
}

// GetContextInfoFromProto retrieves ContextInfo from any supported protobuf message variant.
func GetContextInfoFromProto(msg *waE2E.Message) *waE2E.ContextInfo {
	msg = UnwrapMessageProto(msg)
	if msg == nil {
		return nil
	}
	if ext := msg.GetExtendedTextMessage(); ext != nil && ext.GetContextInfo() != nil {
		return ext.GetContextInfo()
	}
	if img := msg.GetImageMessage(); img != nil && img.GetContextInfo() != nil {
		return img.GetContextInfo()
	}
	if vid := msg.GetVideoMessage(); vid != nil && vid.GetContextInfo() != nil {
		return vid.GetContextInfo()
	}
	if aud := msg.GetAudioMessage(); aud != nil && aud.GetContextInfo() != nil {
		return aud.GetContextInfo()
	}
	if doc := msg.GetDocumentMessage(); doc != nil && doc.GetContextInfo() != nil {
		return doc.GetContextInfo()
	}
	if stk := msg.GetStickerMessage(); stk != nil && stk.GetContextInfo() != nil {
		return stk.GetContextInfo()
	}
	if btn := msg.GetButtonsMessage(); btn != nil && btn.GetContextInfo() != nil {
		return btn.GetContextInfo()
	}
	if btnResp := msg.GetButtonsResponseMessage(); btnResp != nil && btnResp.GetContextInfo() != nil {
		return btnResp.GetContextInfo()
	}
	if list := msg.GetListResponseMessage(); list != nil && list.GetContextInfo() != nil {
		return list.GetContextInfo()
	}
	if listMsg := msg.GetListMessage(); listMsg != nil && listMsg.GetContextInfo() != nil {
		return listMsg.GetContextInfo()
	}
	if poll := msg.GetPollCreationMessage(); poll != nil && poll.GetContextInfo() != nil {
		return poll.GetContextInfo()
	}
	if poll2 := msg.GetPollCreationMessageV2(); poll2 != nil && poll2.GetContextInfo() != nil {
		return poll2.GetContextInfo()
	}
	if poll3 := msg.GetPollCreationMessageV3(); poll3 != nil && poll3.GetContextInfo() != nil {
		return poll3.GetContextInfo()
	}
	if poll5 := msg.GetPollCreationMessageV5(); poll5 != nil && poll5.GetContextInfo() != nil {
		return poll5.GetContextInfo()
	}
	if poll6 := msg.GetPollCreationMessageV6(); poll6 != nil && poll6.GetContextInfo() != nil {
		return poll6.GetContextInfo()
	}
	if im := msg.GetInteractiveMessage(); im != nil && im.GetContextInfo() != nil {
		return im.GetContextInfo()
	}
	if irm := msg.GetInteractiveResponseMessage(); irm != nil && irm.GetContextInfo() != nil {
		return irm.GetContextInfo()
	}
	if tbr := msg.GetTemplateButtonReplyMessage(); tbr != nil && tbr.GetContextInfo() != nil {
		return tbr.GetContextInfo()
	}
	if tm := msg.GetTemplateMessage(); tm != nil && tm.GetContextInfo() != nil {
		return tm.GetContextInfo()
	}
	if ptv := msg.GetPtvMessage(); ptv != nil && ptv.GetContextInfo() != nil {
		return ptv.GetContextInfo()
	}
	if loc := msg.GetLocationMessage(); loc != nil && loc.GetContextInfo() != nil {
		return loc.GetContextInfo()
	}
	if liveLoc := msg.GetLiveLocationMessage(); liveLoc != nil && liveLoc.GetContextInfo() != nil {
		return liveLoc.GetContextInfo()
	}
	if cont := msg.GetContactMessage(); cont != nil && cont.GetContextInfo() != nil {
		return cont.GetContextInfo()
	}
	if conts := msg.GetContactsArrayMessage(); conts != nil && conts.GetContextInfo() != nil {
		return conts.GetContextInfo()
	}
	if gi := msg.GetGroupInviteMessage(); gi != nil && gi.GetContextInfo() != nil {
		return gi.GetContextInfo()
	}
	if evtMsg := msg.GetEventMessage(); evtMsg != nil && evtMsg.GetContextInfo() != nil {
		return evtMsg.GetContextInfo()
	}
	return nil
}

// ExtractMessageText extracts the human-readable text string from any incoming message event.
func ExtractMessageText(evt *events.Message) string {
	if evt == nil {
		return ""
	}
	if evt.Message != nil {
		if text := ExtractTextFromProto(evt.Message); text != "" {
			return text
		}
	}
	if evt.RawMessage != nil {
		if text := ExtractTextFromProto(evt.RawMessage); text != "" {
			return text
		}
	}
	return ""
}

// ExtractTextFromProto extracts human-readable text from a raw protobuf message.
func ExtractTextFromProto(msg *waE2E.Message) string {
	msg = UnwrapMessageProto(msg)
	if msg == nil {
		return ""
	}
	if c := msg.GetConversation(); c != "" {
		return c
	}
	if ext := msg.GetExtendedTextMessage(); ext != nil && ext.GetText() != "" {
		return ext.GetText()
	}
	if img := msg.GetImageMessage(); img != nil && img.GetCaption() != "" {
		return img.GetCaption()
	}
	if vid := msg.GetVideoMessage(); vid != nil && vid.GetCaption() != "" {
		return vid.GetCaption()
	}
	if doc := msg.GetDocumentMessage(); doc != nil && doc.GetCaption() != "" {
		return doc.GetCaption()
	}
	if ptv := msg.GetPtvMessage(); ptv != nil && ptv.GetCaption() != "" {
		return ptv.GetCaption()
	}
	if poll := msg.GetPollCreationMessage(); poll != nil && poll.GetName() != "" {
		return poll.GetName()
	}
	if poll := msg.GetPollCreationMessageV2(); poll != nil && poll.GetName() != "" {
		return poll.GetName()
	}
	if poll := msg.GetPollCreationMessageV3(); poll != nil && poll.GetName() != "" {
		return poll.GetName()
	}
	if poll := msg.GetPollCreationMessageV5(); poll != nil && poll.GetName() != "" {
		return poll.GetName()
	}
	if poll := msg.GetPollCreationMessageV6(); poll != nil && poll.GetName() != "" {
		return poll.GetName()
	}
	if im := msg.GetInteractiveMessage(); im != nil {
		if body := im.GetBody(); body != nil && body.GetText() != "" {
			return body.GetText()
		}
		if hdr := im.GetHeader(); hdr != nil && hdr.GetTitle() != "" {
			return hdr.GetTitle()
		}
	}
	if irm := msg.GetInteractiveResponseMessage(); irm != nil {
		if body := irm.GetBody(); body != nil && body.GetText() != "" {
			return body.GetText()
		}
		if nfr := irm.GetNativeFlowResponseMessage(); nfr != nil {
			if nfr.GetParamsJSON() != "" {
				return nfr.GetParamsJSON()
			}
			if nfr.GetName() != "" {
				return nfr.GetName()
			}
		}
	}
	if btn := msg.GetButtonsResponseMessage(); btn != nil {
		if btn.GetSelectedDisplayText() != "" {
			return btn.GetSelectedDisplayText()
		}
		if btn.GetSelectedButtonID() != "" {
			return btn.GetSelectedButtonID()
		}
	}
	if tbr := msg.GetTemplateButtonReplyMessage(); tbr != nil {
		if tbr.GetSelectedDisplayText() != "" {
			return tbr.GetSelectedDisplayText()
		}
		if tbr.GetSelectedID() != "" {
			return tbr.GetSelectedID()
		}
	}
	if list := msg.GetListResponseMessage(); list != nil {
		if list.GetTitle() != "" {
			return list.GetTitle()
		}
		if single := list.GetSingleSelectReply(); single != nil && single.GetSelectedRowID() != "" {
			return single.GetSelectedRowID()
		}
		if list.GetDescription() != "" {
			return list.GetDescription()
		}
	}
	if btn := msg.GetButtonsMessage(); btn != nil && btn.GetContentText() != "" {
		return btn.GetContentText()
	}
	if tm := msg.GetTemplateMessage(); tm != nil {
		if h := tm.GetHydratedTemplate(); h != nil && h.GetHydratedContentText() != "" {
			return h.GetHydratedContentText()
		}
		if h4 := tm.GetHydratedFourRowTemplate(); h4 != nil && h4.GetHydratedContentText() != "" {
			return h4.GetHydratedContentText()
		}
	}
	if gi := msg.GetGroupInviteMessage(); gi != nil {
		if cap := gi.GetCaption(); cap != "" {
			return cap
		}
		if name := gi.GetGroupName(); name != "" {
			return name
		}
	}
	if evtMsg := msg.GetEventMessage(); evtMsg != nil {
		if name := evtMsg.GetName(); name != "" {
			return name
		}
		if desc := evtMsg.GetDescription(); desc != "" {
			return desc
		}
	}
	return ""
}

// ExtractMediaFromEvent extracts a downloadable media attachment (image, video, or media document),
// checking both the primary message and any quoted reply message.
func ExtractMediaFromEvent(evt *events.Message) (whatsmeow.DownloadableMessage, bool, string) {
	if evt == nil || evt.Message == nil {
		return nil, false, ""
	}
	extract := func(msg *waE2E.Message) (whatsmeow.DownloadableMessage, bool, string) {
		msg = UnwrapMessageProto(msg)
		if msg == nil {
			return nil, false, ""
		}
		if img := msg.GetImageMessage(); img != nil {
			return img, false, img.GetMimetype()
		}
		if vid := msg.GetVideoMessage(); vid != nil {
			return vid, true, vid.GetMimetype()
		}
		if doc := msg.GetDocumentMessage(); doc != nil {
			mime := doc.GetMimetype()
			filename := strings.ToLower(doc.GetFileName())
			if strings.HasPrefix(mime, "video/") || strings.HasSuffix(filename, ".mp4") || strings.HasSuffix(filename, ".mkv") {
				return doc, true, mime
			}
			if strings.HasPrefix(mime, "image/") || strings.HasSuffix(filename, ".jpg") || strings.HasSuffix(filename, ".png") || strings.HasSuffix(filename, ".jpeg") {
				return doc, false, mime
			}
		}
		return nil, false, ""
	}

	if dl, isVid, mime := extract(evt.Message); dl != nil {
		return dl, isVid, mime
	}
	if ci := GetContextInfoFromProto(evt.Message); ci != nil && ci.QuotedMessage != nil {
		if dl, isVid, mime := extract(ci.QuotedMessage); dl != nil {
			return dl, isVid, mime
		}
	}
	return nil, false, ""
}

// ParticipantMatchesUser checks if a group participant matches the target JID,
// comparing primary JID, LID, and PhoneNumber, as well as resolving through the client's store.
func ParticipantMatchesUser(ctx context.Context, client *whatsmeow.Client, p types.GroupParticipant, target types.JID) bool {
	if target.IsEmpty() {
		return false
	}
	cleanTarget := target.ToNonAD()

	// 1. Direct match with p.JID
	if !p.JID.IsEmpty() {
		cleanPJID := p.JID.ToNonAD()
		if cleanPJID == cleanTarget || (cleanPJID.User == cleanTarget.User && (cleanPJID.Server == cleanTarget.Server || cleanTarget.Server == "")) {
			return true
		}
	}

	// 2. Direct match with p.LID
	if !p.LID.IsEmpty() {
		cleanPLID := p.LID.ToNonAD()
		if cleanPLID == cleanTarget || (cleanPLID.User == cleanTarget.User && (cleanPLID.Server == cleanTarget.Server || cleanTarget.Server == "")) {
			return true
		}
	}

	// 3. Direct match with p.PhoneNumber
	if !p.PhoneNumber.IsEmpty() {
		cleanPPN := p.PhoneNumber.ToNonAD()
		if cleanPPN == cleanTarget || (cleanPPN.User == cleanTarget.User && (cleanPPN.Server == cleanTarget.Server || cleanTarget.Server == "")) {
			return true
		}
	}

	// 4. Check if target is the bot and p matches bot's ID or LID
	if client != nil && client.Store != nil {
		botID := client.Store.ID
		botLID := client.Store.LID

		targetIsBot := (botID != nil && !botID.IsEmpty() && IsSameUserRaw(ctx, client, cleanTarget, *botID)) ||
			(!botLID.IsEmpty() && IsSameUserRaw(ctx, client, cleanTarget, botLID))

		if targetIsBot {
			if botID != nil && !botID.IsEmpty() {
				cleanBotID := botID.ToNonAD()
				if (!p.JID.IsEmpty() && p.JID.ToNonAD() == cleanBotID) ||
					(!p.PhoneNumber.IsEmpty() && p.PhoneNumber.ToNonAD() == cleanBotID) ||
					(!p.LID.IsEmpty() && p.LID.ToNonAD() == cleanBotID) {
					return true
				}
			}
			if !botLID.IsEmpty() {
				cleanBotLID := botLID.ToNonAD()
				if (!p.JID.IsEmpty() && p.JID.ToNonAD() == cleanBotLID) ||
					(!p.LID.IsEmpty() && p.LID.ToNonAD() == cleanBotLID) ||
					(!p.PhoneNumber.IsEmpty() && p.PhoneNumber.ToNonAD() == cleanBotLID) {
					return true
				}
			}
		}
	}

	// 5. Cross-check using IsSameUserRaw across all non-empty JIDs on p
	if !p.JID.IsEmpty() && IsSameUserRaw(ctx, client, p.JID, cleanTarget) {
		return true
	}
	if !p.LID.IsEmpty() && IsSameUserRaw(ctx, client, p.LID, cleanTarget) {
		return true
	}
	if !p.PhoneNumber.IsEmpty() && IsSameUserRaw(ctx, client, p.PhoneNumber, cleanTarget) {
		return true
	}

	return false
}

// IsAdminRaw checks if a participant JID is an admin or superadmin in groupInfo.
func IsAdminRaw(ctx context.Context, client *whatsmeow.Client, groupInfo *types.GroupInfo, userJID types.JID) bool {
	if groupInfo == nil || userJID.IsEmpty() {
		return false
	}
	for _, p := range groupInfo.Participants {
		if ParticipantMatchesUser(ctx, client, p, userJID) {
			return p.IsAdmin || p.IsSuperAdmin
		}
	}
	return false
}

// IsBotAdminRaw checks if the bot itself has admin or superadmin privileges in groupInfo.
func IsBotAdminRaw(ctx context.Context, client *whatsmeow.Client, groupInfo *types.GroupInfo) bool {
	if client == nil || client.Store == nil || groupInfo == nil {
		return false
	}
	if client.Store.ID != nil && !client.Store.ID.IsEmpty() && IsAdminRaw(ctx, client, groupInfo, *client.Store.ID) {
		return true
	}
	if !client.Store.LID.IsEmpty() && IsAdminRaw(ctx, client, groupInfo, client.Store.LID) {
		return true
	}
	return false
}

// RecentMessageStore caches recent incoming messages for call responses, target resolution, and LID mapping lookups.
type RecentMessageStore struct {
	mu       sync.RWMutex
	messages map[types.JID]*events.Message
}

var GlobalRecentMessages = &RecentMessageStore{
	messages: make(map[types.JID]*events.Message),
}

// RecordRecentMessage caches an incoming message event indexed by its chat and sender JIDs.
func RecordRecentMessage(evt *events.Message) {
	if evt == nil {
		return
	}
	GlobalRecentMessages.mu.Lock()
	defer GlobalRecentMessages.mu.Unlock()
	if !evt.Info.Chat.IsEmpty() {
		GlobalRecentMessages.messages[evt.Info.Chat.ToNonAD()] = evt
	}
	if !evt.Info.Sender.IsEmpty() {
		GlobalRecentMessages.messages[evt.Info.Sender.ToNonAD()] = evt
	}
	if !evt.Info.SenderAlt.IsEmpty() {
		GlobalRecentMessages.messages[evt.Info.SenderAlt.ToNonAD()] = evt
	}
}

// GetRecentMessageForJID retrieves the most recent message associated with a JID.
func GetRecentMessageForJID(jid types.JID) *events.Message {
	if jid.IsEmpty() {
		return nil
	}
	GlobalRecentMessages.mu.RLock()
	defer GlobalRecentMessages.mu.RUnlock()
	jidNonAD := jid.ToNonAD()
	if evt, ok := GlobalRecentMessages.messages[jidNonAD]; ok {
		return evt
	}
	for k, v := range GlobalRecentMessages.messages {
		if v != nil && (k.User == jidNonAD.User || v.Info.Sender.ToNonAD().User == jidNonAD.User || v.Info.SenderAlt.ToNonAD().User == jidNonAD.User) {
			return v
		}
	}
	return nil
}

// SettingGetter retrieves a setting value for a client and key from the database store.
type SettingGetter func(ctx context.Context, client *whatsmeow.Client, key string) (string, error)

// SettingSetter updates a setting value for a client and key in the database store.
type SettingSetter func(ctx context.Context, client *whatsmeow.Client, key, value string) error

// SettingDeleter removes a setting value for a client and key from the database store.
type SettingDeleter func(ctx context.Context, client *whatsmeow.Client, key string) error

var (
	GlobalSettingGetter  SettingGetter
	GlobalSettingSetter  SettingSetter
	GlobalSettingDeleter SettingDeleter
)

// GetClientSetting retrieves a configuration value from the global setting getter or identity store fallback.
func GetClientSetting(ctx context.Context, client *whatsmeow.Client, key string) (string, error) {
	if GlobalSettingGetter != nil {
		if val, err := GlobalSettingGetter(ctx, client, key); err == nil && val != "" {
			return val, nil
		}
	}
	if client != nil && client.Store != nil && client.Store.Identities != nil {
		if s, ok := client.Store.Identities.(interface {
			GetSetting(ctx context.Context, key string) (string, error)
		}); ok {
			return s.GetSetting(ctx, key)
		}
	}
	return "", nil
}

// PutClientSetting writes a configuration value to the global setting setter or identity store fallback.
func PutClientSetting(ctx context.Context, client *whatsmeow.Client, key, value string) error {
	if GlobalSettingSetter != nil {
		return GlobalSettingSetter(ctx, client, key, value)
	}
	if client != nil && client.Store != nil && client.Store.Identities != nil {
		if s, ok := client.Store.Identities.(interface {
			PutSetting(ctx context.Context, key, value string) error
		}); ok {
			return s.PutSetting(ctx, key, value)
		}
	}
	return nil
}

// DeleteClientSetting deletes a configuration value using the global setting deleter or identity store fallback.
func DeleteClientSetting(ctx context.Context, client *whatsmeow.Client, key string) error {
	if GlobalSettingDeleter != nil {
		return GlobalSettingDeleter(ctx, client, key)
	}
	if client != nil && client.Store != nil && client.Store.Identities != nil {
		if s, ok := client.Store.Identities.(interface {
			DeleteSetting(ctx context.Context, key string) error
		}); ok {
			return s.DeleteSetting(ctx, key)
		}
	}
	return nil
}

// IsSudoRaw checks if a sender JID has sudo/owner privileges stored in database settings or environment.
func IsSudoRaw(ctx context.Context, client *whatsmeow.Client, sender types.JID) bool {
	if client == nil || sender.IsEmpty() {
		return false
	}

	// 1. Check if sender is the bot owner (Store.ID or Store.LID)
	if client.Store != nil {
		if client.Store.ID != nil && !client.Store.ID.IsEmpty() && IsSameUserRaw(ctx, client, sender, *client.Store.ID) {
			return true
		}
		if !client.Store.LID.IsEmpty() && IsSameUserRaw(ctx, client, sender, client.Store.LID) {
			return true
		}
	}

	// 2. Check environment variables (SUDOERS, SUDO, OWNER)
	for _, envKey := range []string{"SUDOERS", "SUDO", "OWNER"} {
		if envVal := strings.TrimSpace(os.Getenv(envKey)); envVal != "" {
			for entry := range strings.FieldsSeq(envVal) {
				cleanEntry := strings.TrimPrefix(entry, "+")
				if parsed, err := types.ParseJID(entry); err == nil {
					if IsSameUserRaw(ctx, client, sender, parsed) {
						return true
					}
				} else if cleanEntry != "" && (sender.ToNonAD().User == cleanEntry || strings.TrimPrefix(sender.ToNonAD().User, "+") == cleanEntry) {
					return true
				}
			}
		}
	}

	// 3. Check database settings (database "sudoers" list)
	if raw, err := GetClientSetting(ctx, client, "sudoers"); err == nil && raw != "" {
		// Resolve the sender's contact push name for username-token matching.
		var senderPushName string
		if client.Store != nil && client.Store.Contacts != nil {
			lookupJID := sender.ToNonAD()
			// If sender is a LID, try to get the PN for contact lookup.
			if lookupJID.Server == types.HiddenUserServer && client.Store.LIDs != nil {
				if pn, pnErr := client.Store.LIDs.GetPNForLID(ctx, lookupJID); pnErr == nil && !pn.IsEmpty() {
					lookupJID = pn.ToNonAD()
				}
			}
			if contact, cErr := client.Store.Contacts.GetContact(ctx, lookupJID); cErr == nil && contact.Found {
				if contact.Username != "" {
					senderPushName = strings.ToLower(contact.Username)
				} else if contact.PushName != "" {
					senderPushName = strings.ToLower(contact.PushName)
				} else if contact.FullName != "" {
					senderPushName = strings.ToLower(contact.FullName)
				}
			}
		}
		if senderPushName == "" {
			if recent := GetRecentMessageForJID(sender); recent != nil && recent.Info.PushName != "" {
				senderPushName = strings.ToLower(recent.Info.PushName)
			}
		}

		for sudoerStr := range strings.FieldsSeq(raw) {
			cleanSudoer := strings.TrimPrefix(sudoerStr, "+")
			if strings.Contains(sudoerStr, "@") {
				if sudoerJID, err := types.ParseJID(sudoerStr); err == nil {
					if IsSameUserRaw(ctx, client, sender, sudoerJID) {
						return true
					}
				}
			} else if cleanSudoer != "" {
				// Bare phone number match.
				senderUser := sender.ToNonAD().User
				if senderUser == cleanSudoer || strings.TrimPrefix(senderUser, "+") == cleanSudoer {
					return true
				}
				// Push name / username token match.
				if senderPushName != "" && strings.EqualFold(senderPushName, cleanSudoer) {
					return true
				}
			}
		}
	}

	// Backward compatibility fallback for per-JID key
	if val, err := GetClientSetting(ctx, client, "sudo:"+sender.ToNonAD().String()); err == nil && val == "true" {
		return true
	}
	if val, err := GetClientSetting(ctx, client, "sudo:"+sender.ToNonAD().User); err == nil && val == "true" {
		return true
	}
	if recent := GetRecentMessageForJID(sender); recent != nil && !recent.Info.SenderAlt.IsEmpty() {
		if val, err := GetClientSetting(ctx, client, "sudo:"+recent.Info.SenderAlt.ToNonAD().String()); err == nil && val == "true" {
			return true
		}
		if val, err := GetClientSetting(ctx, client, "sudo:"+recent.Info.SenderAlt.ToNonAD().User); err == nil && val == "true" {
			return true
		}
	}

	return false
}

// IsSameUserRaw compares two JIDs ignoring device and agent AD suffixes,
// resolving and matching Phone Numbers and Linked Identities (LIDs).
func IsSameUserRaw(ctx context.Context, client *whatsmeow.Client, a, b types.JID) bool {
	a = a.ToNonAD()
	b = b.ToNonAD()
	if a.IsEmpty() || b.IsEmpty() {
		return false
	}

	// 1. Direct match (same JID or same server + user)
	if a == b || (a.Server == b.Server && a.User == b.User) {
		return true
	}

	// 2. Direct comparison between client's stored companion ID (PN) and LID
	if client != nil && client.Store != nil {
		id := client.Store.ID
		lid := client.Store.LID
		if id != nil && !id.IsEmpty() && !lid.IsEmpty() {
			idNonAD := id.ToNonAD()
			lidNonAD := lid.ToNonAD()
			if (a == idNonAD && b == lidNonAD) || (a == lidNonAD && b == idNonAD) {
				return true
			}
			if (a.User == idNonAD.User && b.User == lidNonAD.User) || (a.User == lidNonAD.User && b.User == idNonAD.User) {
				return true
			}
		}
	}

	// 3. Resolve LIDs to Phone Numbers via whatsmeow LID mapping store
	aPN := a
	bPN := b
	if a.Server == types.HiddenUserServer && client != nil && client.Store != nil && client.Store.LIDs != nil {
		if pn, err := client.Store.LIDs.GetPNForLID(ctx, a); err == nil && !pn.IsEmpty() {
			aPN = pn.ToNonAD()
		}
	}
	if b.Server == types.HiddenUserServer && client != nil && client.Store != nil && client.Store.LIDs != nil {
		if pn, err := client.Store.LIDs.GetPNForLID(ctx, b); err == nil && !pn.IsEmpty() {
			bPN = pn.ToNonAD()
		}
	}

	// Fallback to recent messages for LID->PN resolution if LID store lookup was empty
	if a.Server == types.HiddenUserServer && aPN == a {
		if recent := GetRecentMessageForJID(a); recent != nil && !recent.Info.SenderAlt.IsEmpty() && recent.Info.SenderAlt.Server == types.DefaultUserServer {
			aPN = recent.Info.SenderAlt.ToNonAD()
			if client != nil && client.Store != nil && client.Store.LIDs != nil {
				_ = client.Store.LIDs.PutLIDMapping(ctx, a, aPN)
			}
		}
	}
	if b.Server == types.HiddenUserServer && bPN == b {
		if recent := GetRecentMessageForJID(b); recent != nil && !recent.Info.SenderAlt.IsEmpty() && recent.Info.SenderAlt.Server == types.DefaultUserServer {
			bPN = recent.Info.SenderAlt.ToNonAD()
			if client != nil && client.Store != nil && client.Store.LIDs != nil {
				_ = client.Store.LIDs.PutLIDMapping(ctx, b, bPN)
			}
		}
	}

	if !aPN.IsEmpty() && !bPN.IsEmpty() && (aPN == bPN || (aPN.Server == bPN.Server && aPN.User == bPN.User)) {
		return true
	}

	// 4. Resolve Phone Numbers to LIDs via whatsmeow LID mapping store
	aLID := a
	bLID := b
	if a.Server == types.DefaultUserServer && client != nil && client.Store != nil && client.Store.LIDs != nil {
		if lid, err := client.Store.LIDs.GetLIDForPN(ctx, a); err == nil && !lid.IsEmpty() {
			aLID = lid.ToNonAD()
		}
	}
	if b.Server == types.DefaultUserServer && client != nil && client.Store != nil && client.Store.LIDs != nil {
		if lid, err := client.Store.LIDs.GetLIDForPN(ctx, b); err == nil && !lid.IsEmpty() {
			bLID = lid.ToNonAD()
		}
	}

	// Fallback to recent messages for PN->LID resolution
	if a.Server == types.DefaultUserServer && aLID == a {
		if recent := GetRecentMessageForJID(a); recent != nil && recent.Info.Sender.Server == types.HiddenUserServer {
			aLID = recent.Info.Sender.ToNonAD()
			if client != nil && client.Store != nil && client.Store.LIDs != nil {
				_ = client.Store.LIDs.PutLIDMapping(ctx, aLID, a)
			}
		}
	}
	if b.Server == types.DefaultUserServer && bLID == b {
		if recent := GetRecentMessageForJID(b); recent != nil && recent.Info.Sender.Server == types.HiddenUserServer {
			bLID = recent.Info.Sender.ToNonAD()
			if client != nil && client.Store != nil && client.Store.LIDs != nil {
				_ = client.Store.LIDs.PutLIDMapping(ctx, bLID, b)
			}
		}
	}

	if !aLID.IsEmpty() && !bLID.IsEmpty() && (aLID == bLID || (aLID.Server == bLID.Server && aLID.User == bLID.User)) {
		return true
	}

	return !aLID.IsEmpty() && !bLID.IsEmpty() && (aLID == bLID || (aLID.Server == bLID.Server && aLID.User == bLID.User))
}

// ResolveMentionJIDs resolves a participant JID to the full set of JIDs that must be
// included in ContextInfo.MentionedJID, and the parsed user identifier (phone number or
// LID user) to be used as "@" + tagUser in message text. In WhatsApp protocol, interactive
// mentions require the exact user part of the JID/LID (e.g. "@2348012345678"), NOT a push name.
func ResolveMentionJIDs(ctx context.Context, client *whatsmeow.Client, participant types.JID) ([]types.JID, string) {
	resolved := participant.ToNonAD()

	// Build the full set of JIDs: primary + paired LID or PN
	var pnJID, lidJID types.JID
	switch resolved.Server {
	case types.HiddenUserServer:
		lidJID = resolved
		if client != nil && client.Store != nil && client.Store.LIDs != nil {
			if pn, err := client.Store.LIDs.GetPNForLID(ctx, resolved); err == nil && !pn.IsEmpty() {
				pnJID = pn.ToNonAD()
			}
		}
	default:
		pnJID = resolved
		if client != nil && client.Store != nil && client.Store.LIDs != nil {
			if lid, err := client.Store.LIDs.GetLIDForPN(ctx, resolved); err == nil && !lid.IsEmpty() {
				lidJID = lid.ToNonAD()
			}
		}
	}

	seen := make(map[string]bool)
	var jids []types.JID
	for _, j := range []types.JID{pnJID, lidJID} {
		if !j.IsEmpty() {
			key := j.String()
			if !seen[key] {
				seen[key] = true
				jids = append(jids, j)
			}
		}
	}
	if len(jids) == 0 {
		jids = append(jids, resolved)
	}

	// In WhatsApp protocol, interactive text mentions require @<phone_number> or @<lid_user>
	// (the JID User part). WhatsApp clients match this against ContextInfo.MentionedJID to
	// highlight and render the clickable mention with the user's name on their UI.
	tagUser := pnJID.User
	if tagUser == "" {
		tagUser = resolved.User
	}
	if tagUser == "" {
		tagUser = resolved.String()
	}
	if tagUser == "" {
		tagUser = "User"
	}
	return jids, tagUser
}

// ResolveMentionRaw resolves a participant JID to its primary non-AD JID and user string (without @).
// The returned string is the parsed JID/LID user part (phone number or LID user) required by WhatsApp
// protocol for interactive "@" + user mentions, NOT the contact push name.
func ResolveMentionRaw(ctx context.Context, client *whatsmeow.Client, participant types.JID) (types.JID, string) {
	jids, tagUser := ResolveMentionJIDs(ctx, client, participant)
	if len(jids) > 0 {
		return jids[0], tagUser
	}
	return participant.ToNonAD(), tagUser
}

// ResolveContactName returns the push name or full name of a contact, or falls back to phone number / user ID.
func ResolveContactName(ctx context.Context, client *whatsmeow.Client, participant types.JID) string {
	resolved := participant.ToNonAD()
	if client != nil && client.Store != nil && client.Store.Contacts != nil {
		if contact, err := client.Store.Contacts.GetContact(ctx, resolved); err == nil && contact.Found {
			if contact.PushName != "" {
				return contact.PushName
			} else if contact.FullName != "" {
				return contact.FullName
			}
		}
	}
	return resolved.User
}

// RemoveEmojis strips emoji characters from text strings.
func RemoveEmojis(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 0x1F000 && r <= 0x1FFFF || r >= 0x2600 && r <= 0x27BF {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// IsViewOnceMessage returns true if the message is a ViewOnce container or contains a ViewOnce media attachment.
func IsViewOnceMessage(msg *waE2E.Message) bool {
	if msg == nil {
		return false
	}
	if msg.EphemeralMessage != nil && msg.EphemeralMessage.Message != nil {
		return IsViewOnceMessage(msg.EphemeralMessage.Message)
	}
	if msg.ViewOnceMessage != nil || msg.ViewOnceMessageV2 != nil || msg.ViewOnceMessageV2Extension != nil {
		return true
	}
	if img := msg.GetImageMessage(); img != nil && img.GetViewOnce() {
		return true
	}
	if vid := msg.GetVideoMessage(); vid != nil && vid.GetViewOnce() {
		return true
	}
	if aud := msg.GetAudioMessage(); aud != nil && aud.GetViewOnce() {
		return true
	}
	return false
}

// ExtractViewOnceMessage extracts and returns the inner media message from any ViewOnce wrapper.
func ExtractViewOnceMessage(msg *waE2E.Message) *waE2E.Message {
	msg = UnwrapMessageProto(msg)
	if msg == nil {
		return nil
	}
	res := &waE2E.Message{}
	if img := msg.GetImageMessage(); img != nil {
		cloned := proto.Clone(img).(*waE2E.ImageMessage)
		cloned.ViewOnce = new(false)
		res.ImageMessage = cloned
		return res
	}
	if vid := msg.GetVideoMessage(); vid != nil {
		cloned := proto.Clone(vid).(*waE2E.VideoMessage)
		cloned.ViewOnce = new(false)
		res.VideoMessage = cloned
		return res
	}
	if aud := msg.GetAudioMessage(); aud != nil {
		cloned := proto.Clone(aud).(*waE2E.AudioMessage)
		cloned.ViewOnce = new(false)
		res.AudioMessage = cloned
		return res
	}
	return msg
}

// UnwrapAndSendViewOnceMessage downloads encrypted ViewOnce media, re-uploads it with fresh keys, and sends the clean unwrapped message to target JID.
func UnwrapAndSendViewOnceMessage(ctx context.Context, client *whatsmeow.Client, msg *waE2E.Message, senderJID types.JID, pushName string, targetJID types.JID, quoteID string, sourceChat ...types.JID) error {
	if msg == nil || client == nil {
		return fmt.Errorf("invalid arguments")
	}

	unwrapped := ExtractViewOnceMessage(msg)
	if unwrapped == nil {
		return fmt.Errorf("failed to extract inner ViewOnce message")
	}

	if img := unwrapped.GetImageMessage(); img != nil {
		data, err := client.Download(ctx, img)
		if err != nil {
			return fmt.Errorf("download image: %w", err)
		}
		uploaded, err := client.Upload(ctx, data, whatsmeow.MediaImage)
		if err != nil {
			return fmt.Errorf("upload image: %w", err)
		}
		img.URL = &uploaded.URL
		img.DirectPath = &uploaded.DirectPath
		img.MediaKey = uploaded.MediaKey
		img.FileEncSHA256 = uploaded.FileEncSHA256
		img.FileSHA256 = uploaded.FileSHA256
		img.FileLength = new(uint64(len(data)))
		img.ViewOnce = new(false)
	} else if vid := unwrapped.GetVideoMessage(); vid != nil {
		data, err := client.Download(ctx, vid)
		if err != nil {
			return fmt.Errorf("download video: %w", err)
		}
		uploaded, err := client.Upload(ctx, data, whatsmeow.MediaVideo)
		if err != nil {
			return fmt.Errorf("upload video: %w", err)
		}
		vid.URL = &uploaded.URL
		vid.DirectPath = &uploaded.DirectPath
		vid.MediaKey = uploaded.MediaKey
		vid.FileEncSHA256 = uploaded.FileEncSHA256
		vid.FileSHA256 = uploaded.FileSHA256
		vid.FileLength = new(uint64(len(data)))
		vid.ViewOnce = new(false)
	} else if aud := unwrapped.GetAudioMessage(); aud != nil {
		data, err := client.Download(ctx, aud)
		if err != nil {
			return fmt.Errorf("download audio: %w", err)
		}
		uploaded, err := client.Upload(ctx, data, whatsmeow.MediaAudio)
		if err != nil {
			return fmt.Errorf("upload audio: %w", err)
		}
		aud.URL = &uploaded.URL
		aud.DirectPath = &uploaded.DirectPath
		aud.MediaKey = uploaded.MediaKey
		aud.FileEncSHA256 = uploaded.FileEncSHA256
		aud.FileSHA256 = uploaded.FileSHA256
		aud.FileLength = new(uint64(len(data)))
		aud.ViewOnce = new(false)
	}

	_, err := client.SendMessage(ctx, targetJID, unwrapped)
	return err
}

// FormatTextResponseRaw applies optional global font styles or formatting transformations.
func FormatTextResponseRaw(text string) string {
	return text
}

// EncodeProtoMessage serializes a protobuf message into a hex-encoded string.
func EncodeProtoMessage(msg proto.Message) (string, error) {
	data, err := proto.Marshal(msg)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}

// DecodeProtoMessage decodes a hex-encoded or base64-encoded protobuf message.
func DecodeProtoMessage(encoded string) (*waE2E.Message, error) {
	data, err := hex.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		var b64Err error
		data, b64Err = base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
		if b64Err != nil {
			return nil, fmt.Errorf("decode proto hex: %v, b64: %v", err, b64Err)
		}
	}
	msg := &waE2E.Message{}
	if err := proto.Unmarshal(data, msg); err != nil {
		return nil, err
	}
	return msg, nil
}

// Sprintf formats text according to format specifier.
func Sprintf(format string, args ...any) string {
	return fmt.Sprintf(format, args...)
}
