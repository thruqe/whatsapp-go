package whatsrook

import (
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types/events"
)

// UnwrapMessageProto unwraps nested message wrappers such as EphemeralMessage, ViewOnceMessage,
// DocumentWithCaptionMessage, DeviceSentMessage, and EditedMessage to return the core message proto.
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
		if bfm := msg.GetBotForwardedMessage(); bfm != nil && bfm.GetMessage() != nil {
			msg = bfm.GetMessage()
			continue
		}
		if devSent := msg.GetDeviceSentMessage(); devSent != nil && devSent.GetMessage() != nil {
			msg = devSent.GetMessage()
			continue
		}
		break
	}
	return msg
}

// GetContextInfoFromProto extracts the ContextInfo from any waE2E.Message proto (unwrapping wrappers if needed).
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
	if tmpl := msg.GetTemplateMessage(); tmpl != nil && tmpl.GetContextInfo() != nil {
		return tmpl.GetContextInfo()
	}
	if inter := msg.GetInteractiveMessage(); inter != nil && inter.GetContextInfo() != nil {
		return inter.GetContextInfo()
	}
	if interResp := msg.GetInteractiveResponseMessage(); interResp != nil && interResp.GetContextInfo() != nil {
		return interResp.GetContextInfo()
	}
	if list := msg.GetListResponseMessage(); list != nil && list.GetContextInfo() != nil {
		return list.GetContextInfo()
	}
	if poll := msg.GetPollCreationMessage(); poll != nil && poll.GetContextInfo() != nil {
		return poll.GetContextInfo()
	}
	if evt := msg.GetEventMessage(); evt != nil && evt.GetContextInfo() != nil {
		return evt.GetContextInfo()
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
	if contArr := msg.GetContactsArrayMessage(); contArr != nil && contArr.GetContextInfo() != nil {
		return contArr.GetContextInfo()
	}
	if grpInv := msg.GetGroupInviteMessage(); grpInv != nil && grpInv.GetContextInfo() != nil {
		return grpInv.GetContextInfo()
	}
	return nil
}

// ExtractTextFromProto extracts conversation text, extended text, interactive body, or media caption from a waE2E.Message proto.
func ExtractTextFromProto(msg *waE2E.Message) string {
	msg = UnwrapMessageProto(msg)
	if msg == nil {
		return ""
	}
	if msg.GetConversation() != "" {
		return msg.GetConversation()
	}
	if ext := msg.GetExtendedTextMessage(); ext != nil && ext.GetText() != "" {
		return ext.GetText()
	}
	if img := msg.GetImageMessage(); img != nil {
		if img.GetCaption() != "" {
			return img.GetCaption()
		}
		return "📷 Photo"
	}
	if vid := msg.GetVideoMessage(); vid != nil {
		if vid.GetCaption() != "" {
			return vid.GetCaption()
		}
		return "🎥 Video"
	}
	if doc := msg.GetDocumentMessage(); doc != nil {
		if doc.GetCaption() != "" {
			return doc.GetCaption()
		}
		if doc.GetFileName() != "" {
			return "📄 " + doc.GetFileName()
		}
		return "📄 Document"
	}
	if aud := msg.GetAudioMessage(); aud != nil {
		if aud.GetPTT() {
			return "🎤 Voice message"
		}
		return "🎵 Audio"
	}
	if stk := msg.GetStickerMessage(); stk != nil {
		return "🎨 Sticker"
	}
	if loc := msg.GetLocationMessage(); loc != nil {
		if loc.GetName() != "" {
			return "📍 " + loc.GetName()
		}
		return "📍 Location"
	}
	if liveLoc := msg.GetLiveLocationMessage(); liveLoc != nil {
		if liveLoc.GetCaption() != "" {
			return liveLoc.GetCaption()
		}
		return "📍 Live location"
	}
	if cont := msg.GetContactMessage(); cont != nil {
		if cont.GetDisplayName() != "" {
			return "👤 " + cont.GetDisplayName()
		}
		return "👤 Contact"
	}
	if contArr := msg.GetContactsArrayMessage(); contArr != nil {
		if contArr.GetDisplayName() != "" {
			return "👥 " + contArr.GetDisplayName()
		}
		return "👥 Contacts"
	}
	if btn := msg.GetButtonsMessage(); btn != nil {
		if btn.GetContentText() != "" {
			return btn.GetContentText()
		}
		if btn.GetText() != "" {
			return btn.GetText()
		}
	}
	if tmpl := msg.GetTemplateMessage(); tmpl != nil && tmpl.GetHydratedTemplate() != nil && tmpl.GetHydratedTemplate().GetHydratedContentText() != "" {
		return tmpl.GetHydratedTemplate().GetHydratedContentText()
	}
	if inter := msg.GetInteractiveMessage(); inter != nil && inter.GetBody() != nil && inter.GetBody().GetText() != "" {
		return inter.GetBody().GetText()
	}
	if interResp := msg.GetInteractiveResponseMessage(); interResp != nil && interResp.GetBody() != nil && interResp.GetBody().GetText() != "" {
		return interResp.GetBody().GetText()
	}
	if btnResp := msg.GetButtonsResponseMessage(); btnResp != nil && btnResp.GetSelectedDisplayText() != "" {
		return btnResp.GetSelectedDisplayText()
	}
	if list := msg.GetListResponseMessage(); list != nil && list.GetTitle() != "" {
		return list.GetTitle()
	}
	if poll := msg.GetPollCreationMessage(); poll != nil && poll.GetName() != "" {
		return "📊 " + poll.GetName()
	}
	if evt := msg.GetEventMessage(); evt != nil && evt.GetName() != "" {
		return "📅 " + evt.GetName()
	}
	if rich := msg.GetRichResponseMessage(); rich != nil {
		for _, sub := range rich.GetSubmessages() {
			if sub.GetMessageText() != "" {
				return sub.GetMessageText()
			}
		}
	}
	if bfm := msg.GetBotForwardedMessage(); bfm != nil && bfm.GetMessage() != nil {
		return ExtractTextFromProto(bfm.GetMessage())
	}
	return ""
}

// ExtractMessageText extracts the primary text content from a whatsmeow *events.Message.
func ExtractMessageText(v *events.Message) string {
	if v == nil || v.Message == nil {
		return ""
	}
	return ExtractTextFromProto(v.Message)
}

// GetMediaType returns the simple media string ("image", "video", "audio", "document", "sticker", "contact", "location") from a waE2E.Message.
func GetMediaType(msg *waE2E.Message) string {
	msg = UnwrapMessageProto(msg)
	if msg == nil {
		return ""
	}
	switch {
	case msg.ImageMessage != nil:
		return "image"
	case msg.VideoMessage != nil:
		return "video"
	case msg.AudioMessage != nil:
		return "audio"
	case msg.DocumentMessage != nil:
		return "document"
	case msg.StickerMessage != nil:
		return "sticker"
	case msg.ContactMessage != nil || msg.ContactsArrayMessage != nil:
		return "contact"
	case msg.LocationMessage != nil || msg.LiveLocationMessage != nil:
		return "location"
	default:
		return ""
	}
}
