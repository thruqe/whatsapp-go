package message

import (
	"go.mau.fi/whatsmeow/proto/waE2E"
)

// UnwrapMessageProto extracts the innermost conversational message payload from any wrapper layers.
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

// StripContextInfo zeroes out any nested ContextInfo within a message protobuf to eliminate cyclic pointer graphs.
func StripContextInfo(msg *waE2E.Message) {
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
}
