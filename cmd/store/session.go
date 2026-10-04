package store

import (
	"context"
	"fmt"
	"strings"

	"whatsrook"
	"whatsrook/util/cache"
	"whatsrook/util/logger"

	"github.com/thruqe/duosql"
	"go.mau.fi/util/dbutil"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
)

func init() {
	whatsrook.GlobalSessionPurger = func(ctx context.Context, db *dbutil.Database, session string) error {
		return PurgeSessionData(ctx, db, session)
	}
}

// BuildSessionPredicates constructs duosql predicates matching all possible representations
// of a session identifier (raw string, stripped prefix, phone digits, JID, device suffix, etc.).
func BuildSessionPredicates(session string) []duosql.Predicate {
	trimmed := strings.TrimSpace(session)
	if trimmed == "" {
		return nil
	}
	clean := strings.TrimPrefix(trimmed, "+")

	// Extract phone/user digits
	user := clean
	if idx := strings.IndexAny(user, ":@."); idx != -1 {
		user = user[:idx]
	}

	seen := make(map[string]bool)
	var preds []duosql.Predicate

	addEq := func(val string) {
		val = strings.TrimSpace(val)
		if val != "" && !seen[val] {
			seen[val] = true
			preds = append(preds, duosql.Eq("our_jid", val))
		}
	}

	addEq(trimmed)
	addEq(clean)
	if user != "" {
		addEq(user)
		addEq(user + "@s.whatsapp.net")
		addEq(user + "@" + types.DefaultUserServer)
		preds = append(preds,
			duosql.Like("our_jid", user+":%"),
			duosql.Like("our_jid", user+"@%"),
			duosql.Like("our_jid", user+".%"),
		)
	}
	return preds
}

// PurgeSessionData removes all records associated with the specified session/phone
// across all bot domain tables using duosql ORM builders, and purges matching cache keys.
func PurgeSessionData(ctx context.Context, db *dbutil.Database, session string) error {
	if db == nil || db.RawDB == nil {
		return fmt.Errorf("database handle is nil")
	}

	preds := BuildSessionPredicates(session)
	if len(preds) == 0 {
		logger.Debug("PurgeSessionData: session identifier is empty; skipping deletion to prevent data loss")
		return nil
	}

	duo, err := getDuoDBFromDB(db)
	if err != nil {
		return fmt.Errorf("failed to get duosql db: %w", err)
	}

	whereClause := duosql.Or(preds...)

	// Purge all custom domain models scoped by our_jid
	if _, err := duosql.Delete[BotSetting](duo).Where(whereClause).Exec(ctx); err != nil {
		logger.Warn("PurgeSessionData: failed to purge bot_settings", "err", err, "session", session)
	}
	if _, err := duosql.Delete[CallMediaConfig](duo).Where(whereClause).Exec(ctx); err != nil {
		logger.Warn("PurgeSessionData: failed to purge call_media_config", "err", err, "session", session)
	}
	if _, err := duosql.Delete[BotFilter](duo).Where(whereClause).Exec(ctx); err != nil {
		logger.Warn("PurgeSessionData: failed to purge bot_filters", "err", err, "session", session)
	}
	if _, err := duosql.Delete[BotBGM](duo).Where(whereClause).Exec(ctx); err != nil {
		logger.Warn("PurgeSessionData: failed to purge bot_bgm", "err", err, "session", session)
	}
	if _, err := duosql.Delete[BotStickerCmd](duo).Where(whereClause).Exec(ctx); err != nil {
		logger.Warn("PurgeSessionData: failed to purge bot_sticker_cmds", "err", err, "session", session)
	}
	if _, err := duosql.Delete[GroupStats](duo).Where(whereClause).Exec(ctx); err != nil {
		logger.Warn("PurgeSessionData: failed to purge group_stats", "err", err, "session", session)
	}
	if _, err := duosql.Delete[BotUserXP](duo).Where(whereClause).Exec(ctx); err != nil {
		logger.Warn("PurgeSessionData: failed to purge bot_user_xp", "err", err, "session", session)
	}
	if _, err := duosql.Delete[BotGroupUserXP](duo).Where(whereClause).Exec(ctx); err != nil {
		logger.Warn("PurgeSessionData: failed to purge bot_group_user_xp", "err", err, "session", session)
	}
	if _, err := duosql.Delete[CachedGroupParticipant](duo).Where(whereClause).Exec(ctx); err != nil {
		logger.Warn("PurgeSessionData: failed to purge cached_group_participants", "err", err, "session", session)
	}
	if _, err := duosql.Delete[CachedGroup](duo).Where(whereClause).Exec(ctx); err != nil {
		logger.Warn("PurgeSessionData: failed to purge cached_groups", "err", err, "session", session)
	}
	if _, err := duosql.Delete[CachedNewsletter](duo).Where(whereClause).Exec(ctx); err != nil {
		logger.Warn("PurgeSessionData: failed to purge cached_newsletters", "err", err, "session", session)
	}
	if _, err := duosql.Delete[BotPlatformCookie](duo).Where(whereClause).Exec(ctx); err != nil {
		logger.Warn("PurgeSessionData: failed to purge bot_platform_cookies", "err", err, "session", session)
	}

	// Purge in-memory caches for all resolved session keys
	purgeSessionCache(ctx, session)

	logger.Debug("PurgeSessionData: successfully purged all session data", "session", session)
	return nil
}

// PurgeSessionDataFromStore resolves the database handle from an active sqlstore and purges session data.
func PurgeSessionDataFromStore(ctx context.Context, s *sqlstore.SQLStore, session string) error {
	if s == nil {
		return nil
	}
	db, err := getDBFromStore(s)
	if err != nil {
		return err
	}
	if session == "" {
		session = ourJIDStr(s)
	}
	return PurgeSessionData(ctx, db, session)
}

func purgeSessionCache(ctx context.Context, session string) {
	trimmed := strings.TrimSpace(session)
	if trimmed == "" {
		return
	}
	clean := strings.TrimPrefix(trimmed, "+")
	user := clean
	if idx := strings.IndexAny(user, ":@."); idx != -1 {
		user = user[:idx]
	}

	keys := []string{trimmed, clean}
	if user != "" {
		keys = append(keys, user, user+"@s.whatsapp.net", user+"@"+types.DefaultUserServer)
	}

	seen := make(map[string]bool)
	for _, k := range keys {
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		_ = cache.DeletePrefix(ctx, "setting:"+k+":")
		_ = cache.DeletePrefix(ctx, "filter:"+k+":")
		_ = cache.DeletePrefix(ctx, "bgm:"+k+":")
		_ = cache.DeletePrefix(ctx, "stkcmd:"+k+":")
	}
}
