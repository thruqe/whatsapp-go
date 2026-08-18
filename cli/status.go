package main

import (
	"context"
	"log/slog"
	"math/rand"
	"time"

	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types/events"
)

func (b *Bot) handleLikeStatus(ctx context.Context, v *events.Message) {
	cli := b.client.WAClient()
	if cli == nil || v == nil {
		return
	}
	s, ok := cli.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return
	}

	status, _ := s.GetSetting(ctx, "likestatus_status")
	if status != "on" {
		return
	}

	loveEmojis := []string{"❤️", "💕", "💖", "💗", "💓", "💞", "💘", "💌", "🥰", "😍"}
	emoji := loveEmojis[rand.Intn(len(loveEmojis))]

	senderJID := v.Info.Sender
	if senderJID.IsEmpty() {
		senderJID = v.Info.Chat
	}

	reaction := &waE2E.Message{
		ReactionMessage: &waE2E.ReactionMessage{
			Key: &waCommon.MessageKey{
				RemoteJID:   new(v.Info.Chat.String()),
				FromMe:      new(v.Info.IsFromMe),
				ID:          new(v.Info.ID),
				Participant: new(senderJID.String()),
			},
			Text:              new(emoji),
			SenderTimestampMS: new(time.Now().UnixMilli()),
		},
	}

	_, err := cli.SendMessage(ctx, v.Info.Chat, reaction)
	if err != nil {
		slog.Error("failed to react to status broadcast", "err", err)
	} else {
		slog.Debug("liked status broadcast", "emoji", emoji, "sender", senderJID.String())
	}
}
