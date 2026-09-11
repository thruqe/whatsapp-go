package settings

import (
	"context"
	"math/rand"
	"time"

	"whatsrook/cmd/store"
	"whatsrook/util/logger"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// HandleStatusBroadcast processes incoming status broadcast messages for autoread, autostatussave, and likestatus.
func HandleStatusBroadcast(ctx context.Context, cli *whatsmeow.Client, v *events.Message, startupTime time.Time) {
	if cli == nil || v == nil {
		return
	}

	logger.Debug("likestatus: received status message event",
		"msgID", v.Info.ID,
		"sender", v.Info.Sender.String(),
		"chat", v.Info.Chat.String(),
		"timestamp", v.Info.Timestamp,
	)

	if !v.Info.Timestamp.IsZero() && v.Info.Timestamp.Before(startupTime.Add(-2*time.Minute)) {
		logger.Debug("likestatus: skipping stale status broadcast before startup",
			"msgID", v.Info.ID,
			"timestamp", v.Info.Timestamp,
		)
		return
	}

	s, ok := cli.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return
	}

	senderJID := v.Info.Sender
	if senderJID.IsEmpty() {
		senderJID = v.Info.Chat
	}

	statusView, _ := store.GetSetting(ctx, s, "autoread_status_view")
	generalRead, _ := store.GetSetting(ctx, s, "autoread_status")
	if statusView == "on" || (generalRead == "on" && statusView != "off") {
		_ = cli.MarkRead(ctx, []types.MessageID{v.Info.ID}, v.Info.Timestamp, types.StatusBroadcastJID, senderJID)
		logger.Debug("autoread: marked status broadcast as read", "msgID", v.Info.ID, "sender", senderJID.String())
	}

	statusSave, _ := store.GetSetting(ctx, s, "autostatussave")
	if statusSave == "on" && cli.Store != nil && cli.Store.ID != nil {
		ownerJID := cli.Store.ID.ToNonAD()
		_, errSave := cli.SendMessage(ctx, ownerJID, v.Message)
		if errSave != nil {
			logger.Error("autostatussave: failed to forward status broadcast to owner", "err", errSave, "msgID", v.Info.ID)
		} else {
			logger.Debug("autostatussave: forwarded status broadcast to owner", "owner", ownerJID.String(), "msgID", v.Info.ID)
		}
	}

	status, _ := store.GetSetting(ctx, s, "likestatus_status")
	if status != "on" {
		logger.Debug("likestatus: feature disabled, skipping auto-reaction", "status", status)
		return
	}

	loveEmojis := []string{"❤️", "💕", "💖", "💗", "💓", "💞", "💘", "💌", "🥰", "😍"}
	emoji := loveEmojis[rand.Intn(len(loveEmojis))]

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
		logger.Error("likestatus: failed to react to status broadcast", "err", err, "msgID", v.Info.ID)
	} else {
		logger.Debug("likestatus: liked status broadcast", "emoji", emoji, "sender", senderJID.String(), "msgID", v.Info.ID)
	}
}
