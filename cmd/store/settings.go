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

// PrewarmSettings loads all bot settings into cache at startup.
func PrewarmSettings(ctx context.Context, s *sqlstore.SQLStore) error {
	if s == nil {
		return nil
	}
	duo, err := getDuoDB(s)
	if err != nil {
		return err
	}
	ourJID := ourJIDStr(s)

	items, err := duosql.Select[BotSetting](duo).
		Where(duosql.Or(
			duosql.Eq("our_jid", ourJID),
			duosql.Eq("our_jid", s.JID),
			duosql.Eq("our_jid", ""),
			duosql.IsNull("our_jid"),
		)).
		All(ctx)
	if err != nil {
		return err
	}

	for _, item := range items {
		_ = cache.Set(ctx, settingCacheKey(ourJID, item.Key), item.Value, settingCacheTTL)
		if s.JID != "" && s.JID != ourJID {
			_ = cache.Set(ctx, settingCacheKey(s.JID, item.Key), item.Value, settingCacheTTL)
		}
	}

	logger.Debug("[PERF] Prewarmed bot settings into cache", "count", len(items))
	return nil
}

// GetSetting returns the configured value for key, falling through cache to duosql.
func GetSetting(ctx context.Context, s *sqlstore.SQLStore, key string) (string, error) {
	if s == nil {
		return "", nil
	}
	ourJID := ourJIDStr(s)
	cacheKey := settingCacheKey(ourJID, key)

	if val, ok, _ := cache.Get(ctx, cacheKey); ok {
		if val == cacheSentinelNil {
			return "", nil
		}
		return val, nil
	}

	return fetchSettingFromDB(ctx, s, ourJID, cacheKey, key)
}

func fetchSettingFromDB(ctx context.Context, s *sqlstore.SQLStore, ourJID, cacheKey, key string) (string, error) {
	start := time.Now()
	duo, err := getDuoDB(s)
	if err != nil {
		return "", err
	}

	dbStart := time.Now()
	orderExpr := fmt.Sprintf(
		"CASE WHEN our_jid = %s THEN 1 WHEN our_jid = %s THEN 2 ELSE 3 END",
		sqlQuoteLiteral(ourJID), sqlQuoteLiteral(s.JID),
	)

	setting, err := duosql.Select[BotSetting](duo).
		Where(
			duosql.Or(
				duosql.Eq("our_jid", ourJID),
				duosql.Eq("our_jid", s.JID),
				duosql.Eq("our_jid", ""),
				duosql.IsNull("our_jid"),
			),
			duosql.Eq("key", key),
		).
		OrderBy(duosql.RawOrderBy(orderExpr)).
		Limit(1).
		One(ctx)

	durDB := time.Since(dbStart)
	if durDB > 1*time.Millisecond {
		logger.Debug("[PERF] store.GetSetting DB query", "key", key, "dbDuration", durDB, "total", time.Since(start))
	}

	if errors.Is(err, sql.ErrNoRows) {
		_ = cache.Set(ctx, cacheKey, cacheSentinelNil, settingNegativeCacheTTL)
		return "", nil
	}
	if err != nil {
		return "", err
	}

	_ = cache.Set(ctx, cacheKey, setting.Value, settingCacheTTL)
	if s.JID != "" && s.JID != ourJID {
		_ = cache.Set(ctx, settingCacheKey(s.JID, key), setting.Value, settingCacheTTL)
	}
	return setting.Value, nil
}

// PutSetting writes or updates a configuration flag using duosql upsert builders.
func PutSetting(ctx context.Context, s *sqlstore.SQLStore, key, value string) error {
	if s == nil {
		return nil
	}
	ourJID := ourJIDStr(s)
	cacheKey := settingCacheKey(ourJID, key)

	_ = cache.Set(ctx, cacheKey, value, settingCacheTTL)
	if s.JID != "" && s.JID != ourJID {
		_ = cache.Set(ctx, settingCacheKey(s.JID, key), value, settingCacheTTL)
	}

	duo, err := getDuoDB(s)
	if err != nil {
		return err
	}

	item := &BotSetting{
		OurJID: ourJID,
		Key:    key,
		Value:  value,
	}

	_, err = duosql.Insert[BotSetting](duo).
		Values(item).
		OnConflictDoUpdateAll("our_jid", "key").
		Exec(ctx)
	return err
}

// DeleteSetting clears a setting entry from both storage and in-memory caches.
func DeleteSetting(ctx context.Context, s *sqlstore.SQLStore, key string) error {
	if s == nil {
		return nil
	}
	ourJID := ourJIDStr(s)
	cacheKey := settingCacheKey(ourJID, key)

	_ = cache.Delete(ctx, cacheKey)
	if s.JID != "" && s.JID != ourJID {
		_ = cache.Delete(ctx, settingCacheKey(s.JID, key))
	}
	_ = cache.Delete(ctx, settingCacheKey("", key))

	duo, err := getDuoDB(s)
	if err != nil {
		return err
	}

	_, err = duosql.Delete[BotSetting](duo).
		Where(
			duosql.Or(
				duosql.Eq("our_jid", ourJID),
				duosql.Eq("our_jid", s.JID),
				duosql.Eq("our_jid", ""),
				duosql.IsNull("our_jid"),
			),
			duosql.Eq("key", key),
		).
		Exec(ctx)
	return err
}

// ListSettingsWithPrefixes queries settings matching any of the specified key prefixes.
func ListSettingsWithPrefixes(ctx context.Context, s *sqlstore.SQLStore, prefixes ...string) ([]BotSetting, error) {
	if s == nil {
		return nil, nil
	}
	duo, err := getDuoDB(s)
	if err != nil {
		return nil, err
	}
	ourJID := ourJIDStr(s)

	builder := duosql.Select[BotSetting](duo).
		Where(duosql.Or(
			duosql.Eq("our_jid", ourJID),
			duosql.Eq("our_jid", s.JID),
			duosql.Eq("our_jid", ""),
			duosql.IsNull("our_jid"),
		))

	if len(prefixes) > 0 {
		var likes []duosql.Predicate
		for _, p := range prefixes {
			likes = append(likes, duosql.Like("key", p+"%"))
		}
		builder = builder.Where(duosql.Or(likes...))
	}

	return builder.All(ctx)
}
