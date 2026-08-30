package store

import (
	"context"
	"time"

	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// LogGroupMessage records message activity counters per group and user per day,
// scoped to the active session's our_jid.
func LogGroupMessage(ctx context.Context, s *sqlstore.SQLStore, chat, sender types.JID) {
	if s == nil {
		return
	}
	gdb, err := GetORM(ctx, s)
	if err != nil {
		return
	}

	ourJID := ourJIDStr(s)
	groupJID := chat.String()
	userJID := sender.ToNonAD().String()
	dateStr := time.Now().Format("2006-01-02")

	stat := GroupStats{
		OurJID:   ourJID,
		GroupJID: groupJID,
		UserJID:  userJID,
		DateStr:  dateStr,
		MsgCount: 1,
	}

	_ = gdb.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "our_jid"}, {Name: "group_jid"}, {Name: "user_jid"}, {Name: "date_str"}},
		DoUpdates: clause.Assignments(map[string]any{
			"msg_count": gorm.Expr("group_stats.msg_count + ?", 1),
		}),
	}).Create(&stat).Error
}

// AddGroupUserTTTXP updates TTT game statistics and XP in group leaderboard.
func AddGroupUserTTTXP(ctx context.Context, s *sqlstore.SQLStore, groupJID, userJID string, amount, winInc, lossInc, drawInc int) error {
	if s == nil {
		return nil
	}
	gdb, err := GetORM(ctx, s)
	if err != nil {
		return err
	}
	ourJID := ourJIDStr(s)

	entry := BotGroupUserXP{
		OurJID:    ourJID,
		GroupJID:  groupJID,
		UserJID:   userJID,
		XP:        int64(amount),
		TTTWins:   winInc,
		TTTLosses: lossInc,
		TTTDraws:  drawInc,
		WCGRating: 1000,
	}

	return gdb.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "our_jid"}, {Name: "group_jid"}, {Name: "user_jid"}},
		DoUpdates: clause.Assignments(map[string]any{
			"xp":         gorm.Expr("CASE WHEN bot_group_user_xp.xp + ? < 0 THEN 0 ELSE bot_group_user_xp.xp + ? END", amount, amount),
			"ttt_wins":   gorm.Expr("bot_group_user_xp.ttt_wins + ?", winInc),
			"ttt_losses": gorm.Expr("bot_group_user_xp.ttt_losses + ?", lossInc),
			"ttt_draws":  gorm.Expr("bot_group_user_xp.ttt_draws + ?", drawInc),
		}),
	}).Create(&entry).Error
}

// AddGroupUserWCGXP updates WCG game statistics, rating, and XP in group leaderboard.
func AddGroupUserWCGXP(ctx context.Context, s *sqlstore.SQLStore, groupJID, userJID string, amount, winInc, gameInc, newRating int) error {
	if s == nil {
		return nil
	}
	gdb, err := GetORM(ctx, s)
	if err != nil {
		return err
	}
	ourJID := ourJIDStr(s)

	entry := BotGroupUserXP{
		OurJID:    ourJID,
		GroupJID:  groupJID,
		UserJID:   userJID,
		XP:        int64(amount),
		WCGWins:   winInc,
		WCGGames:  gameInc,
		WCGRating: newRating,
	}

	return gdb.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "our_jid"}, {Name: "group_jid"}, {Name: "user_jid"}},
		DoUpdates: clause.Assignments(map[string]any{
			"xp":         gorm.Expr("CASE WHEN bot_group_user_xp.xp + ? < 0 THEN 0 ELSE bot_group_user_xp.xp + ? END", amount, amount),
			"wcg_wins":   gorm.Expr("bot_group_user_xp.wcg_wins + ?", winInc),
			"wcg_games":  gorm.Expr("bot_group_user_xp.wcg_games + ?", gameInc),
			"wcg_rating": newRating,
		}),
	}).Create(&entry).Error
}

// GetGroupLeaderboard retrieves all ranked users in a group by XP.
func GetGroupLeaderboard(ctx context.Context, s *sqlstore.SQLStore, groupJID string) ([]BotGroupUserXP, error) {
	if s == nil {
		return nil, nil
	}
	gdb, err := GetORM(ctx, s)
	if err != nil {
		return nil, err
	}
	ourJID := ourJIDStr(s)

	var list []BotGroupUserXP
	err = gdb.WithContext(ctx).
		Where("our_jid = ? AND group_jid = ?", ourJID, groupJID).
		Order("xp DESC, ttt_wins DESC").
		Find(&list).Error
	return list, err
}
