package main

import (
	"whatsrook/utils"

	"go.mau.fi/whatsmeow/types/events"
)

func buildIncomingMessagePayload(v *events.Message) IncomingMessagePayload {
	text := utils.ExtractMessageText(v)
	mediaType := utils.GetMediaType(v.Message)

	var quotedID string
	var quotedText string

	if ext := v.Message.GetExtendedTextMessage(); ext != nil && ext.GetContextInfo() != nil {
		ci := ext.GetContextInfo()
		quotedID = ci.GetStanzaID()
		if ci.QuotedMessage != nil {
			quotedText = utils.ExtractTextFromProto(ci.QuotedMessage)
		}
	}

	return IncomingMessagePayload{
		From:       v.Info.Chat.String(),
		Chat:       v.Info.Chat.String(),
		Sender:     v.Info.Sender.String(),
		Text:       text,
		MessageID:  v.Info.ID,
		PushName:   v.Info.PushName,
		Timestamp:  v.Info.Timestamp,
		IsGroup:    v.Info.IsGroup,
		IsFromMe:   v.Info.IsFromMe,
		MediaType:  mediaType,
		QuotedID:   quotedID,
		QuotedText: quotedText,
	}
}
