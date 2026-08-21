package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"time"

	"go.mau.fi/util/dbutil"
	"go.mau.fi/whatsmeow/types"
)

type CallMediaKind string

const (
	CallMediaAudio CallMediaKind = "audio"
	CallMediaVideo CallMediaKind = "video"
)

const (
	// Settings are scoped per-session via our_jid.
	getSettingQuery = `SELECT value FROM bot_settings WHERE our_jid=$1 AND key=$2`
	putSettingQuery = `
		INSERT INTO bot_settings (our_jid, key, value) VALUES ($1, $2, $3)
		ON CONFLICT (our_jid, key) DO UPDATE SET value=excluded.value
	`
	deleteSettingQuery = `DELETE FROM bot_settings WHERE our_jid=$1 AND key=$2`

	createBotSettingsTableQuery = `
		CREATE TABLE IF NOT EXISTS bot_settings (
			our_jid TEXT NOT NULL DEFAULT '',
			key     TEXT NOT NULL,
			value   TEXT NOT NULL,
			PRIMARY KEY (our_jid, key)
		);
	`
	createCallMediaConfigTableQuery = `
		CREATE TABLE IF NOT EXISTS call_media_config (
			our_jid   TEXT NOT NULL DEFAULT '',
			jid       TEXT NOT NULL,
			kind      TEXT NOT NULL,
			file_path TEXT NOT NULL,
			PRIMARY KEY (our_jid, jid, kind)
		);
	`
	getCallMediaConfigQuery = `SELECT file_path FROM call_media_config WHERE our_jid=$1 AND jid=$2 AND kind=$3`
	putCallMediaConfigQuery = `
		INSERT INTO call_media_config (our_jid, jid, kind, file_path) VALUES ($1, $2, $3, $4)
		ON CONFLICT (our_jid, jid, kind) DO UPDATE SET file_path=excluded.file_path
	`
)

var (
	// settingCache is keyed by "ourJID:key" to prevent cross-session cache bleed.
	settingCache    sync.Map
	settingCacheTTL = 5 * time.Second
)

type settingCacheItem struct {
	val       string
	updatedAt time.Time
}

func settingCacheKey(ourJID, key string) string {
	return ourJID + ":" + key
}

// GetDB returns the underlying dbutil.Database connection handle.
func (s *SQLStore) GetDB() *dbutil.Database {
	if s == nil {
		return nil
	}
	return s.db
}

func (s *SQLStore) ourJIDStr() string {
	if s == nil {
		return ""
	}
	return s.JID
}

// EnsureCustomTables creates WhatsRook custom tables if they do not exist yet.
func (s *SQLStore) EnsureCustomTables(ctx context.Context) error {
	if s == nil || s.db == nil {
		return nil
	}
	if _, err := s.db.Exec(ctx, createBotSettingsTableQuery); err != nil {
		return err
	}
	if _, err := s.db.Exec(ctx, createCallMediaConfigTableQuery); err != nil {
		return err
	}
	// Best-effort column additions
	_, _ = s.db.Exec(ctx, `ALTER TABLE bot_settings ADD COLUMN our_jid TEXT DEFAULT ''`)
	_, _ = s.db.Exec(ctx, `ALTER TABLE call_media_config ADD COLUMN our_jid TEXT DEFAULT ''`)
	_, _ = s.db.Exec(ctx, `ALTER TABLE whatsmeow_contacts ADD COLUMN username TEXT`)
	return nil
}

// GetSetting retrieves a custom bot setting by key, scoped to this session's our_jid.
func (s *SQLStore) GetSetting(ctx context.Context, key string) (string, error) {
	ourJID := s.ourJIDStr()
	cacheKey := settingCacheKey(ourJID, key)

	if cached, ok := settingCache.Load(cacheKey); ok {
		item := cached.(settingCacheItem)
		if time.Since(item.updatedAt) < settingCacheTTL {
			return item.val, nil
		}
	}

	_ = s.EnsureCustomTables(ctx)
	if s == nil || s.db == nil {
		return "", nil
	}
	var val string
	err := s.db.QueryRow(ctx, getSettingQuery, ourJID, key).Scan(&val)
	if errors.Is(err, sql.ErrNoRows) {
		settingCache.Store(cacheKey, settingCacheItem{val: "", updatedAt: time.Now()})
		return "", nil
	}
	if err == nil {
		settingCache.Store(cacheKey, settingCacheItem{val: val, updatedAt: time.Now()})
	}
	return val, err
}

// PutSetting stores a custom bot setting key-value pair scoped to this session's our_jid.
func (s *SQLStore) PutSetting(ctx context.Context, key, value string) error {
	ourJID := s.ourJIDStr()
	cacheKey := settingCacheKey(ourJID, key)
	settingCache.Store(cacheKey, settingCacheItem{val: value, updatedAt: time.Now()})
	_ = s.EnsureCustomTables(ctx)
	if s == nil || s.db == nil {
		return nil
	}
	_, err := s.db.Exec(ctx, putSettingQuery, ourJID, key, value)
	return err
}

// DeleteSetting removes a custom bot setting by key for this session's our_jid.
func (s *SQLStore) DeleteSetting(ctx context.Context, key string) error {
	ourJID := s.ourJIDStr()
	settingCache.Delete(settingCacheKey(ourJID, key))
	_ = s.EnsureCustomTables(ctx)
	if s == nil || s.db == nil {
		return nil
	}
	_, err := s.db.Exec(ctx, deleteSettingQuery, ourJID, key)
	return err
}

// GetCallMediaConfig retrieves call media file path for a user JID and media kind (audio/video),
// scoped to this session's our_jid.
func (s *SQLStore) GetCallMediaConfig(ctx context.Context, jid types.JID, kind CallMediaKind) (string, error) {
	_ = s.EnsureCustomTables(ctx)
	if s == nil || s.db == nil {
		return "", nil
	}
	ourJID := s.ourJIDStr()
	var path string
	if !jid.IsEmpty() {
		err := s.db.QueryRow(ctx, getCallMediaConfigQuery, ourJID, jid.String(), string(kind)).Scan(&path)
		if err == nil && path != "" {
			return path, nil
		}
		if nonAD := jid.ToNonAD().String(); nonAD != "" && nonAD != jid.String() {
			err = s.db.QueryRow(ctx, getCallMediaConfigQuery, ourJID, nonAD, string(kind)).Scan(&path)
			if err == nil && path != "" {
				return path, nil
			}
		}
	}
	// Fallback: check if any media config exists for our session
	if ourJID != "" {
		err := s.db.QueryRow(ctx, `SELECT file_path FROM call_media_config WHERE our_jid=$1 AND kind=$2 LIMIT 1`, ourJID, string(kind)).Scan(&path)
		if err == nil && path != "" {
			return path, nil
		}
	}
	err := s.db.QueryRow(ctx, `SELECT file_path FROM call_media_config WHERE our_jid='' AND kind=$1 LIMIT 1`, string(kind)).Scan(&path)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return path, err
}

// PutCallMediaConfig stores call media file path for a user JID and media kind (audio/video),
// scoped to this session's our_jid.
func (s *SQLStore) PutCallMediaConfig(ctx context.Context, jid types.JID, kind CallMediaKind, filePath string) error {
	_ = s.EnsureCustomTables(ctx)
	if s == nil || s.db == nil {
		return nil
	}
	ourJID := s.ourJIDStr()
	_, err := s.db.Exec(ctx, putCallMediaConfigQuery, ourJID, jid.ToNonAD().String(), string(kind), filePath)
	if err == nil && jid.String() != jid.ToNonAD().String() {
		_, _ = s.db.Exec(ctx, putCallMediaConfigQuery, ourJID, jid.String(), string(kind), filePath)
	}
	return err
}
