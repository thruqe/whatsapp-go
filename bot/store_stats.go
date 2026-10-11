package store

import (
	"context"
	"fmt"
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

// AddGroupUserTTTXP credits Tic-Tac-Toe XP, win/loss/draw tallies to a group participant.
func AddGroupUserTTTXP(ctx context.Context, s *sqlstore.SQLStore, groupJID, userJID string, amount, winInc, lossInc, drawInc int) error {
	if s == nil {
		return nil
	}
	duo, err := getDuoDB(s)
	if err != nil {
		return err
	}
	ourJID := ourJIDStr(s)

	item := &BotGroupUserXP{
		OurJID:    ourJID,
		GroupJID:  groupJID,
		UserJID:   userJID,
		XP:        int64(amount),
		TTTWins:   winInc,
		TTTLosses: lossInc,
		TTTDraws:  drawInc,
		WCGRating: 1000,
	}

	_, err = duosql.Insert[BotGroupUserXP](duo).
		Values(item).
		OnConflictDoUpdateRaw(
			[]string{"our_jid", "group_jid", "user_jid"},
			[]string{
				"xp = CASE WHEN COALESCE(bot_group_user_xp.xp, 0) + EXCLUDED.xp < 0 THEN 0 ELSE COALESCE(bot_group_user_xp.xp, 0) + EXCLUDED.xp END",
				"ttt_wins = COALESCE(bot_group_user_xp.ttt_wins, 0) + EXCLUDED.ttt_wins",
				"ttt_losses = COALESCE(bot_group_user_xp.ttt_losses, 0) + EXCLUDED.ttt_losses",
				"ttt_draws = COALESCE(bot_group_user_xp.ttt_draws, 0) + EXCLUDED.ttt_draws",
			},
		).
		Exec(ctx)
	return err
}

// AddGroupUserWCGXP records Word Chain Game experience, wins, and ELO rating adjustments.
func AddGroupUserWCGXP(ctx context.Context, s *sqlstore.SQLStore, groupJID, userJID string, amount, winInc, gameInc, ratingDelta int) error {
	if s == nil {
		return nil
	}
	duo, err := getDuoDB(s)
	if err != nil {
		return err
	}
	ourJID := ourJIDStr(s)
	initRating := max(1000+ratingDelta, 100)

	item := &BotGroupUserXP{
		OurJID:    ourJID,
		GroupJID:  groupJID,
		UserJID:   userJID,
		XP:        int64(amount),
		WCGWins:   winInc,
		WCGGames:  gameInc,
		WCGRating: initRating,
	}

	ratingUpdateExpr := fmt.Sprintf(
		"wcg_rating = CASE WHEN COALESCE(bot_group_user_xp.wcg_rating, 1000) + (%d) < 100 THEN 100 ELSE COALESCE(bot_group_user_xp.wcg_rating, 1000) + (%d) END",
		ratingDelta, ratingDelta,
	)

	_, err = duosql.Insert[BotGroupUserXP](duo).
		Values(item).
		OnConflictDoUpdateRaw(
			[]string{"our_jid", "group_jid", "user_jid"},
			[]string{
				"xp = CASE WHEN COALESCE(bot_group_user_xp.xp, 0) + EXCLUDED.xp < 0 THEN 0 ELSE COALESCE(bot_group_user_xp.xp, 0) + EXCLUDED.xp END",
				"wcg_wins = COALESCE(bot_group_user_xp.wcg_wins, 0) + EXCLUDED.wcg_wins",
				"wcg_games = COALESCE(bot_group_user_xp.wcg_games, 0) + EXCLUDED.wcg_games",
				ratingUpdateExpr,
			},
		).
		Exec(ctx)
	return err
}

// AddGroupUserUnscrambleXP registers solved word puzzle XP and score increments.
func AddGroupUserUnscrambleXP(ctx context.Context, s *sqlstore.SQLStore, groupJID, userJID string, amount, winInc, scoreInc int) error {
	if s == nil {
		return nil
	}
	duo, err := getDuoDB(s)
	if err != nil {
		return err
	}
	ourJID := ourJIDStr(s)

	item := &BotGroupUserXP{
		OurJID:          ourJID,
		GroupJID:        groupJID,
		UserJID:         userJID,
		XP:              int64(amount),
		UnscrambleWins:  winInc,
		UnscrambleScore: scoreInc,
		WCGRating:       1000,
	}

	_, err = duosql.Insert[BotGroupUserXP](duo).
		Values(item).
		OnConflictDoUpdateRaw(
			[]string{"our_jid", "group_jid", "user_jid"},
			[]string{
				"xp = CASE WHEN COALESCE(bot_group_user_xp.xp, 0) + EXCLUDED.xp < 0 THEN 0 ELSE COALESCE(bot_group_user_xp.xp, 0) + EXCLUDED.xp END",
				"unscramble_wins = COALESCE(bot_group_user_xp.unscramble_wins, 0) + EXCLUDED.unscramble_wins",
				"unscramble_score = COALESCE(bot_group_user_xp.unscramble_score, 0) + EXCLUDED.unscramble_score",
			},
		).
		Exec(ctx)
	return err
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
