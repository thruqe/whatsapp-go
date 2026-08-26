package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"whatsrook/src/cache"

	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
)

type CallMediaKind string

const (
	CallMediaAudio CallMediaKind = "audio"
	CallMediaVideo CallMediaKind = "video"
)

const (
	getSettingQuery = `SELECT value FROM bot_settings WHERE our_jid=$1 AND key=$2`
	putSettingQuery = `
		INSERT INTO bot_settings (our_jid, key, value) VALUES ($1, $2, $3)
		ON CONFLICT (our_jid, key) DO UPDATE SET value=excluded.value
	`
	deleteSettingQuery = `DELETE FROM bot_settings WHERE our_jid=$1 AND key=$2`

	getCallMediaConfigQuery = `SELECT file_path FROM call_media_config WHERE our_jid=$1 AND jid=$2 AND kind=$3`
	putCallMediaConfigQuery = `
		INSERT INTO call_media_config (our_jid, jid, kind, file_path) VALUES ($1, $2, $3, $4)
		ON CONFLICT (our_jid, jid, kind) DO UPDATE SET file_path=excluded.file_path
	`
)

const settingCacheTTL = 5 * time.Second

func settingCacheKey(ourJID, key string) string {
	return "setting:" + ourJID + ":" + key
}

func ourJIDStr(s *sqlstore.SQLStore) string {
	if s == nil {
		return ""
	}
	if s.JID != "" {
		if parsed, err := types.ParseJID(s.JID); err == nil && !parsed.IsEmpty() {
			return parsed.ToNonAD().String()
		}
		return s.JID
	}
	return ""
}

// GetSetting retrieves a custom bot setting by key, scoped to this session's our_jid.
func GetSetting(ctx context.Context, s *sqlstore.SQLStore, key string) (string, error) {
	if s == nil {
		return "", nil
	}
	db := s.GetDB()
	if db == nil {
		return "", nil
	}
	InitTables(ctx, s)

	ourJID := ourJIDStr(s)
	cacheKey := settingCacheKey(ourJID, key)

	if val, ok, _ := cache.Get(ctx, cacheKey); ok {
		return val, nil
	}

	var val string
	var err error
	if ourJID != "" {
		err = db.QueryRow(ctx, getSettingQuery, ourJID, key).Scan(&val)
		if errors.Is(err, sql.ErrNoRows) && s.JID != "" && s.JID != ourJID {
			err = db.QueryRow(ctx, getSettingQuery, s.JID, key).Scan(&val)
		}
	}
	if ourJID == "" || errors.Is(err, sql.ErrNoRows) {
		err = db.QueryRow(ctx, `SELECT value FROM bot_settings WHERE (our_jid='' OR our_jid IS NULL) AND key=$1 LIMIT 1`, key).Scan(&val)
	}

	if errors.Is(err, sql.ErrNoRows) {
		_ = cache.Set(ctx, cacheKey, "", settingCacheTTL)
		return "", nil
	}
	if err == nil {
		_ = cache.Set(ctx, cacheKey, val, settingCacheTTL)
		if s.JID != "" && s.JID != ourJID {
			_ = cache.Set(ctx, settingCacheKey(s.JID, key), val, settingCacheTTL)
		}
	}
	return val, err
}

// PutSetting stores a custom bot setting key-value pair scoped to this session's our_jid.
func PutSetting(ctx context.Context, s *sqlstore.SQLStore, key, value string) error {
	if s == nil {
		return nil
	}
	db := s.GetDB()
	if db == nil {
		return nil
	}
	InitTables(ctx, s)

	ourJID := ourJIDStr(s)
	cacheKey := settingCacheKey(ourJID, key)
	_ = cache.Set(ctx, cacheKey, value, settingCacheTTL)
	if s.JID != "" && s.JID != ourJID {
		_ = cache.Set(ctx, settingCacheKey(s.JID, key), value, settingCacheTTL)
	}

	_, err := db.Exec(ctx, putSettingQuery, ourJID, key, value)
	return err
}

// DeleteSetting removes a custom bot setting by key for this session's our_jid.
func DeleteSetting(ctx context.Context, s *sqlstore.SQLStore, key string) error {
	if s == nil {
		return nil
	}
	db := s.GetDB()
	if db == nil {
		return nil
	}
	InitTables(ctx, s)

	ourJID := ourJIDStr(s)
	_ = cache.Delete(ctx, settingCacheKey(ourJID, key))
	if s.JID != "" && s.JID != ourJID {
		_ = cache.Delete(ctx, settingCacheKey(s.JID, key))
	}

	_, err := db.Exec(ctx, deleteSettingQuery, ourJID, key)
	if s.JID != "" && s.JID != ourJID {
		_, _ = db.Exec(ctx, deleteSettingQuery, s.JID, key)
	}
	return err
}

// GetCallMediaConfig retrieves call media file path for a user JID and media kind (audio/video), scoped to our_jid.
func GetCallMediaConfig(ctx context.Context, s *sqlstore.SQLStore, jid types.JID, kind CallMediaKind) (string, error) {
	if s == nil {
		return "", nil
	}
	db := s.GetDB()
	if db == nil {
		return "", nil
	}
	InitTables(ctx, s)

	ourJID := ourJIDStr(s)
	var path string
	if !jid.IsEmpty() {
		err := db.QueryRow(ctx, getCallMediaConfigQuery, ourJID, jid.String(), string(kind)).Scan(&path)
		if err == nil && path != "" {
			return path, nil
		}
		if nonAD := jid.ToNonAD().String(); nonAD != "" && nonAD != jid.String() {
			err = db.QueryRow(ctx, getCallMediaConfigQuery, ourJID, nonAD, string(kind)).Scan(&path)
			if err == nil && path != "" {
				return path, nil
			}
		}
		if s.JID != "" && s.JID != ourJID {
			err = db.QueryRow(ctx, getCallMediaConfigQuery, s.JID, jid.String(), string(kind)).Scan(&path)
			if err == nil && path != "" {
				return path, nil
			}
		}
	}
	if ourJID != "" {
		err := db.QueryRow(ctx, `SELECT file_path FROM call_media_config WHERE our_jid=$1 AND kind=$2 LIMIT 1`, ourJID, string(kind)).Scan(&path)
		if err == nil && path != "" {
			return path, nil
		}
	}
	err := db.QueryRow(ctx, `SELECT file_path FROM call_media_config WHERE (our_jid='' OR our_jid IS NULL) AND kind=$1 LIMIT 1`, string(kind)).Scan(&path)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return path, err
}

// PutCallMediaConfig stores call media file path for a user JID and media kind (audio/video), scoped to our_jid.
func PutCallMediaConfig(ctx context.Context, s *sqlstore.SQLStore, jid types.JID, kind CallMediaKind, filePath string) error {
	if s == nil {
		return nil
	}
	db := s.GetDB()
	if db == nil {
		return nil
	}
	InitTables(ctx, s)

	ourJID := ourJIDStr(s)
	_, err := db.Exec(ctx, putCallMediaConfigQuery, ourJID, jid.ToNonAD().String(), string(kind), filePath)
	if err == nil && jid.String() != jid.ToNonAD().String() {
		_, _ = db.Exec(ctx, putCallMediaConfigQuery, ourJID, jid.String(), string(kind), filePath)
	}
	return err
}
