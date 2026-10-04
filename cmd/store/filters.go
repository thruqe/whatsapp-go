package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"whatsrook/util/cache"
	"whatsrook/util/logger"

	"github.com/thruqe/duosql"
	"go.mau.fi/whatsmeow/store/sqlstore"
)

// GetFilter retrieves a bot filter by keyword trigger, checking the in-memory cache first.
func GetFilter(ctx context.Context, s *sqlstore.SQLStore, trigger string) (string, error) {
	if s == nil {
		return "", nil
	}
	start := time.Now()
	ourJID := ourJIDStr(s)
	cacheKey := filterCacheKey(ourJID, trigger)

	if val, ok, _ := cache.Get(ctx, cacheKey); ok {
		if val == cacheSentinelNil {
			return "", nil
		}
		return val, nil
	}

	duo, err := getDuoDB(s)
	if err != nil {
		return "", err
	}

	dbStart := time.Now()
	orderExpr := fmt.Sprintf(
		"CASE WHEN our_jid = %s THEN 1 WHEN our_jid = %s THEN 2 ELSE 3 END",
		sqlQuoteLiteral(ourJID), sqlQuoteLiteral(s.JID),
	)

	filter, err := duosql.Select[BotFilter](duo).
		Where(
			duosql.Or(
				duosql.Eq("our_jid", ourJID),
				duosql.Eq("our_jid", s.JID),
				duosql.Eq("our_jid", ""),
				duosql.IsNull("our_jid"),
			),
			duosql.Eq("trigger_word", trigger),
		).
		OrderBy(duosql.RawOrderBy(orderExpr)).
		Limit(1).
		One(ctx)

	durDB := time.Since(dbStart)
	if durDB > 1*time.Millisecond {
		logger.Debug("[PERF] store.GetFilter DB query", "trigger", trigger, "dbDuration", durDB, "total", time.Since(start))
	}

	if errors.Is(err, sql.ErrNoRows) {
		_ = cache.Set(ctx, cacheKey, cacheSentinelNil, filterNegativeCacheTTL)
		return "", nil
	}
	if err != nil {
		return "", err
	}

	_ = cache.Set(ctx, cacheKey, filter.MessageProto, filterCacheTTL)
	if s.JID != "" && s.JID != ourJID {
		_ = cache.Set(ctx, filterCacheKey(s.JID, trigger), filter.MessageProto, filterCacheTTL)
	}
	return filter.MessageProto, nil
}

// PutFilter inserts or updates a bot auto-responder trigger using duosql upsert builders.
func PutFilter(ctx context.Context, s *sqlstore.SQLStore, trigger, messageProto string) error {
	if s == nil {
		return nil
	}
	duo, err := getDuoDB(s)
	if err != nil {
		return err
	}
	ourJID := ourJIDStr(s)

	cacheKey := filterCacheKey(ourJID, trigger)
	_ = cache.Set(ctx, cacheKey, messageProto, filterCacheTTL)
	if s.JID != "" && s.JID != ourJID {
		_ = cache.Set(ctx, filterCacheKey(s.JID, trigger), messageProto, filterCacheTTL)
	}

	item := &BotFilter{
		OurJID:       ourJID,
		TriggerWord:  trigger,
		MessageProto: messageProto,
	}

	_, err = duosql.Insert[BotFilter](duo).
		Values(item).
		OnConflictDoUpdateAll("our_jid", "trigger_word").
		Exec(ctx)
	return err
}

// DeleteFilter removes an auto-responder filter definition and purges associated caches.
func DeleteFilter(ctx context.Context, s *sqlstore.SQLStore, trigger string) error {
	if s == nil {
		return nil
	}
	duo, err := getDuoDB(s)
	if err != nil {
		return err
	}
	ourJID := ourJIDStr(s)

	_ = cache.Delete(ctx, filterCacheKey(ourJID, trigger))
	if s.JID != "" && s.JID != ourJID {
		_ = cache.Delete(ctx, filterCacheKey(s.JID, trigger))
	}
	_ = cache.Delete(ctx, filterCacheKey("", trigger))

	_, err = duosql.Delete[BotFilter](duo).
		Where(
			duosql.Or(
				duosql.Eq("our_jid", ourJID),
				duosql.Eq("our_jid", s.JID),
				duosql.Eq("our_jid", ""),
				duosql.IsNull("our_jid"),
			),
			duosql.Eq("trigger_word", trigger),
		).
		Exec(ctx)
	return err
}

// ListFilters retrieves all registered filter triggers scoped to the current bot session.
func ListFilters(ctx context.Context, s *sqlstore.SQLStore) ([]string, error) {
	if s == nil {
		return nil, nil
	}
	duo, err := getDuoDB(s)
	if err != nil {
		return nil, err
	}
	ourJID := ourJIDStr(s)

	q := duosql.Select[BotFilter](duo, "trigger_word").
		Where(duosql.Or(
			duosql.Eq("our_jid", ourJID),
			duosql.Eq("our_jid", s.JID),
			duosql.Eq("our_jid", ""),
			duosql.IsNull("our_jid"),
		)).
		OrderBy(duosql.Asc("trigger_word"))

	return duosql.Pluck[string, BotFilter](q, ctx, "trigger_word")
}

// GetBGM retrieves a background music track configuration by keyword trigger.
func GetBGM(ctx context.Context, s *sqlstore.SQLStore, trigger string) (string, error) {
	if s == nil {
		return "", nil
	}
	start := time.Now()
	ourJID := ourJIDStr(s)
	cacheKey := bgmCacheKey(ourJID, trigger)

	if val, ok, _ := cache.Get(ctx, cacheKey); ok {
		if val == cacheSentinelNil {
			return "", nil
		}
		return val, nil
	}

	duo, err := getDuoDB(s)
	if err != nil {
		return "", err
	}

	dbStart := time.Now()
	orderExpr := fmt.Sprintf(
		"CASE WHEN our_jid = %s THEN 1 WHEN our_jid = %s THEN 2 ELSE 3 END",
		sqlQuoteLiteral(ourJID), sqlQuoteLiteral(s.JID),
	)

	bgm, err := duosql.Select[BotBGM](duo).
		Where(
			duosql.Or(
				duosql.Eq("our_jid", ourJID),
				duosql.Eq("our_jid", s.JID),
				duosql.Eq("our_jid", ""),
				duosql.IsNull("our_jid"),
			),
			duosql.Eq("trigger_word", trigger),
		).
		OrderBy(duosql.RawOrderBy(orderExpr)).
		Limit(1).
		One(ctx)

	durDB := time.Since(dbStart)
	if durDB > 1*time.Millisecond {
		logger.Debug("[PERF] store.GetBGM DB query", "trigger", trigger, "dbDuration", durDB, "total", time.Since(start))
	}

	if errors.Is(err, sql.ErrNoRows) {
		_ = cache.Set(ctx, cacheKey, cacheSentinelNil, filterNegativeCacheTTL)
		return "", nil
	}
	if err != nil {
		return "", err
	}

	_ = cache.Set(ctx, cacheKey, bgm.MessageProto, filterCacheTTL)
	if s.JID != "" && s.JID != ourJID {
		_ = cache.Set(ctx, bgmCacheKey(s.JID, trigger), bgm.MessageProto, filterCacheTTL)
	}
	return bgm.MessageProto, nil
}

// PutBGM inserts or updates a background music trigger configuration.
func PutBGM(ctx context.Context, s *sqlstore.SQLStore, trigger, messageProto string) error {
	if s == nil {
		return nil
	}
	duo, err := getDuoDB(s)
	if err != nil {
		return err
	}
	ourJID := ourJIDStr(s)

	cacheKey := bgmCacheKey(ourJID, trigger)
	_ = cache.Set(ctx, cacheKey, messageProto, filterCacheTTL)
	if s.JID != "" && s.JID != ourJID {
		_ = cache.Set(ctx, bgmCacheKey(s.JID, trigger), messageProto, filterCacheTTL)
	}

	item := &BotBGM{
		OurJID:       ourJID,
		TriggerWord:  trigger,
		MessageProto: messageProto,
	}

	_, err = duosql.Insert[BotBGM](duo).
		Values(item).
		OnConflictDoUpdateAll("our_jid", "trigger_word").
		Exec(ctx)
	return err
}

// DeleteBGM deletes a background music trigger entry and evicts it from cache.
func DeleteBGM(ctx context.Context, s *sqlstore.SQLStore, trigger string) error {
	if s == nil {
		return nil
	}
	duo, err := getDuoDB(s)
	if err != nil {
		return err
	}
	ourJID := ourJIDStr(s)

	_ = cache.Delete(ctx, bgmCacheKey(ourJID, trigger))
	if s.JID != "" && s.JID != ourJID {
		_ = cache.Delete(ctx, bgmCacheKey(s.JID, trigger))
	}
	_ = cache.Delete(ctx, bgmCacheKey("", trigger))

	_, err = duosql.Delete[BotBGM](duo).
		Where(
			duosql.Or(
				duosql.Eq("our_jid", ourJID),
				duosql.Eq("our_jid", s.JID),
				duosql.Eq("our_jid", ""),
				duosql.IsNull("our_jid"),
			),
			duosql.Eq("trigger_word", trigger),
		).
		Exec(ctx)
	return err
}

// ListBGMs lists all available background music trigger phrases.
func ListBGMs(ctx context.Context, s *sqlstore.SQLStore) ([]string, error) {
	if s == nil {
		return nil, nil
	}
	duo, err := getDuoDB(s)
	if err != nil {
		return nil, err
	}
	ourJID := ourJIDStr(s)

	q := duosql.Select[BotBGM](duo, "trigger_word").
		Where(duosql.Or(
			duosql.Eq("our_jid", ourJID),
			duosql.Eq("our_jid", s.JID),
			duosql.Eq("our_jid", ""),
			duosql.IsNull("our_jid"),
		)).
		OrderBy(duosql.Asc("trigger_word"))

	return duosql.Pluck[string, BotBGM](q, ctx, "trigger_word")
}
