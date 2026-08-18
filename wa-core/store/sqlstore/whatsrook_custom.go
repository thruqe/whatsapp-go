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
	getSettingQuery = `SELECT value FROM bot_settings WHERE key=$1`
	putSettingQuery = `
		INSERT INTO bot_settings (our_jid, key, value) VALUES ($1, $2, $3)
		ON CONFLICT (key) DO UPDATE SET value=excluded.value, our_jid=CASE WHEN excluded.our_jid != '' THEN excluded.our_jid ELSE bot_settings.our_jid END
	`
	deleteSettingQuery = `DELETE FROM bot_settings WHERE key=$1`

	createBotSettingsTableQuery = `
		CREATE TABLE IF NOT EXISTS bot_settings (
			our_jid TEXT DEFAULT '',
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);
	`
	createCallMediaConfigTableQuery = `
		CREATE TABLE IF NOT EXISTS call_media_config (
			our_jid TEXT DEFAULT '',
			jid TEXT NOT NULL,
			kind TEXT NOT NULL,
			file_path TEXT NOT NULL,
			PRIMARY KEY (jid, kind)
		);
	`
	getCallMediaConfigQuery = `SELECT file_path FROM call_media_config WHERE jid=$1 AND kind=$2`
	putCallMediaConfigQuery = `
		INSERT INTO call_media_config (our_jid, jid, kind, file_path) VALUES ($1, $2, $3, $4)
		ON CONFLICT (jid, kind) DO UPDATE SET file_path=excluded.file_path, our_jid=CASE WHEN excluded.our_jid != '' THEN excluded.our_jid ELSE call_media_config.our_jid END
	`
)

var (
	settingCache    sync.Map // key: string -> settingCacheItem
	settingCacheTTL = 5 * time.Second
)

type settingCacheItem struct {
	val       string
	updatedAt time.Time
}

// GetDB returns the underlying dbutil.Database connection handle.
func (s *SQLStore) GetDB() *dbutil.Database {
	if s == nil {
		return nil
	}
	return s.db
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
	// Best-effort column addition and constraint adjustments for PostgreSQL & SQLite
	_, _ = s.db.Exec(ctx, `ALTER TABLE bot_settings ADD COLUMN our_jid TEXT DEFAULT ''`)
	_, _ = s.db.Exec(ctx, `ALTER TABLE call_media_config ADD COLUMN our_jid TEXT DEFAULT ''`)
	_, _ = s.db.Exec(ctx, `ALTER TABLE whatsmeow_contacts ADD COLUMN username TEXT`)
	if s.db.Dialect == dbutil.Postgres {
		_, _ = s.db.Exec(ctx, `ALTER TABLE bot_settings ALTER COLUMN our_jid SET DEFAULT ''`)
		_, _ = s.db.Exec(ctx, `ALTER TABLE bot_settings ALTER COLUMN our_jid DROP NOT NULL`)
		_, _ = s.db.Exec(ctx, `ALTER TABLE call_media_config ALTER COLUMN our_jid SET DEFAULT ''`)
		_, _ = s.db.Exec(ctx, `ALTER TABLE call_media_config ALTER COLUMN our_jid DROP NOT NULL`)
		_, _ = s.db.Exec(ctx, `CREATE UNIQUE INDEX IF NOT EXISTS bot_settings_key_idx ON bot_settings (key)`)
	}
	return nil
}

// GetSetting retrieves a custom bot setting by key with in-memory caching to eliminate loop DB query spam.
func (s *SQLStore) GetSetting(ctx context.Context, key string) (string, error) {
	if cached, ok := settingCache.Load(key); ok {
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
	err := s.db.QueryRow(ctx, getSettingQuery, key).Scan(&val)
	if errors.Is(err, sql.ErrNoRows) {
		settingCache.Store(key, settingCacheItem{val: "", updatedAt: time.Now()})
		return "", nil
	}
	if err == nil {
		settingCache.Store(key, settingCacheItem{val: val, updatedAt: time.Now()})
	}
	return val, err
}

// PutSetting stores a custom bot setting key-value pair and updates the in-memory cache.
func (s *SQLStore) PutSetting(ctx context.Context, key, value string) error {
	settingCache.Store(key, settingCacheItem{val: value, updatedAt: time.Now()})
	_ = s.EnsureCustomTables(ctx)
	if s == nil || s.db == nil {
		return nil
	}
	ourJID := ""
	if s.JID != "" {
		ourJID = s.JID
	}
	_, err := s.db.Exec(ctx, putSettingQuery, ourJID, key, value)
	return err
}

// DeleteSetting removes a custom bot setting by key and invalidates the in-memory cache.
func (s *SQLStore) DeleteSetting(ctx context.Context, key string) error {
	settingCache.Delete(key)
	_ = s.EnsureCustomTables(ctx)
	if s == nil || s.db == nil {
		return nil
	}
	_, err := s.db.Exec(ctx, deleteSettingQuery, key)
	return err
}

// GetCallMediaConfig retrieves call media file path for a user JID and media kind (audio/video).
func (s *SQLStore) GetCallMediaConfig(ctx context.Context, jid types.JID, kind CallMediaKind) (string, error) {
	_ = s.EnsureCustomTables(ctx)
	if s == nil || s.db == nil {
		return "", nil
	}
	var path string
	if !jid.IsEmpty() {
		err := s.db.QueryRow(ctx, getCallMediaConfigQuery, jid.String(), string(kind)).Scan(&path)
		if err == nil && path != "" {
			return path, nil
		}
		if nonAD := jid.ToNonAD().String(); nonAD != "" && nonAD != jid.String() {
			err = s.db.QueryRow(ctx, getCallMediaConfigQuery, nonAD, string(kind)).Scan(&path)
			if err == nil && path != "" {
				return path, nil
			}
		}
	}
	// Fallback: check if any media config exists for our session
	ourJID := ""
	if s.JID != "" {
		ourJID = s.JID
	}
	if ourJID != "" {
		err := s.db.QueryRow(ctx, `SELECT file_path FROM call_media_config WHERE (our_jid=$1 OR our_jid='') AND kind=$2 LIMIT 1`, ourJID, string(kind)).Scan(&path)
		if err == nil && path != "" {
			return path, nil
		}
	}
	err := s.db.QueryRow(ctx, `SELECT file_path FROM call_media_config WHERE kind=$1 LIMIT 1`, string(kind)).Scan(&path)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return path, err
}

// PutCallMediaConfig stores call media file path for a user JID and media kind (audio/video).
func (s *SQLStore) PutCallMediaConfig(ctx context.Context, jid types.JID, kind CallMediaKind, filePath string) error {
	_ = s.EnsureCustomTables(ctx)
	if s == nil || s.db == nil {
		return nil
	}
	ourJID := ""
	if s.JID != "" {
		ourJID = s.JID
	}
	_, err := s.db.Exec(ctx, putCallMediaConfigQuery, ourJID, jid.ToNonAD().String(), string(kind), filePath)
	if err == nil && jid.String() != jid.ToNonAD().String() {
		_, _ = s.db.Exec(ctx, putCallMediaConfigQuery, ourJID, jid.String(), string(kind), filePath)
	}
	return err
}
