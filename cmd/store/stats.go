package store

import (
	"context"
	"time"

	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
)

// LogGroupMessage records message activity counters per group and user per day,
// scoped to the active session's our_jid.
func LogGroupMessage(ctx context.Context, s *sqlstore.SQLStore, chat, sender types.JID) {
	if s == nil {
		return
	}
	InitTables(ctx, s)
	db := s.GetDB()
	if db == nil {
		return
	}
	ourJID := s.JID
	dateStr := time.Now().Format("2006-01-02")
	query := `
		INSERT INTO group_stats (our_jid, group_jid, user_jid, date_str, msg_count)
		VALUES ($1, $2, $3, $4, 1)
		ON CONFLICT(our_jid, group_jid, user_jid, date_str) DO UPDATE SET msg_count = group_stats.msg_count + 1
	`
	_, err := db.Exec(ctx, query, ourJID, chat.String(), sender.ToNonAD().String(), dateStr)
	if err != nil {
		// Fallback for tables with legacy primary key
		fallbackQuery := `
			INSERT INTO group_stats (our_jid, group_jid, user_jid, date_str, msg_count)
			VALUES ($1, $2, $3, $4, 1)
			ON CONFLICT(group_jid, user_jid, date_str) DO UPDATE SET msg_count = group_stats.msg_count + 1
		`
		_, _ = db.Exec(ctx, fallbackQuery, ourJID, chat.String(), sender.ToNonAD().String(), dateStr)
	}
}
