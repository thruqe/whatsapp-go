package main

import (
	"context"
	"time"

	"github.com/thruqe/duosql"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
)

// StoreGroupMessage tracks a participant's message count increment for the current calendar day.
func StoreGroupMessage(ctx context.Context, s *sqlstore.SQLStore, chat, sender types.JID) {
	if s == nil {
		return
	}
	duo, err := getDuoDB(s)
	if err != nil {
		return
	}

	ourJID := ourJIDStr(s)
	groupJID := chat.String()
	userJID := sender.ToNonAD().String()
	dateStr := time.Now().Format("2006-01-02")

	item := &GroupStats{
		OurJID:   ourJID,
		GroupJID: groupJID,
		UserJID:  userJID,
		DateStr:  dateStr,
		MsgCount: 1,
	}

	_, _ = duosql.Insert[GroupStats](duo).
		Values(item).
		OnConflictDoUpdateRaw(
			[]string{"our_jid", "group_jid", "user_jid", "date_str"},
			[]string{"msg_count = COALESCE(group_stats.msg_count, 0) + 1"},
		).
		Exec(ctx)
}

// GetGroupLeaderboard returns all player rankings ordered by total XP and victories.
func GetGroupLeaderboard(ctx context.Context, s *sqlstore.SQLStore, groupJID string) ([]BotGroupUserXP, error) {
	if s == nil {
		return nil, nil
	}
	duo, err := getDuoDB(s)
	if err != nil {
		return nil, err
	}
	ourJID := ourJIDStr(s)

	return duosql.Select[BotGroupUserXP](duo).
		Where(
			duosql.Eq("our_jid", ourJID),
			duosql.Eq("group_jid", groupJID),
		).
		OrderBy(duosql.Desc("xp"), duosql.Desc("ttt_wins")).
		All(ctx)
}
