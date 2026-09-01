package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"whatsrook/cache"

	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type CallMediaKind string

const (
	CallMediaAudio CallMediaKind = "audio"
	CallMediaVideo CallMediaKind = "video"
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
	gdb, err := GetORM(ctx, s)
	if err != nil {
		return "", err
	}

	ourJID := ourJIDStr(s)
	cacheKey := settingCacheKey(ourJID, key)

	if val, ok, _ := cache.Get(ctx, cacheKey); ok {
		return val, nil
	}

	var setting BotSetting
	findErr := gdb.WithContext(ctx).
		Where("our_jid = ? AND key = ?", ourJID, key).
		First(&setting).Error

	if errors.Is(findErr, gorm.ErrRecordNotFound) && s.JID != "" && s.JID != ourJID {
		findErr = gdb.WithContext(ctx).
			Where("our_jid = ? AND key = ?", s.JID, key).
			First(&setting).Error
	}
	if ourJID == "" || errors.Is(findErr, gorm.ErrRecordNotFound) {
		findErr = gdb.WithContext(ctx).
			Where("(our_jid = '' OR our_jid IS NULL) AND key = ?", key).
			First(&setting).Error
	}

	if errors.Is(findErr, gorm.ErrRecordNotFound) {
		_ = cache.Set(ctx, cacheKey, "", settingCacheTTL)
		return "", nil
	}
	if findErr == nil {
		_ = cache.Set(ctx, cacheKey, setting.Value, settingCacheTTL)
		if s.JID != "" && s.JID != ourJID {
			_ = cache.Set(ctx, settingCacheKey(s.JID, key), setting.Value, settingCacheTTL)
		}
		return setting.Value, nil
	}
	return "", findErr
}

// PutSetting stores a custom bot setting key-value pair scoped to this session's our_jid.
func PutSetting(ctx context.Context, s *sqlstore.SQLStore, key, value string) error {
	if s == nil {
		return nil
	}
	gdb, err := GetORM(ctx, s)
	if err != nil {
		return err
	}

	ourJID := ourJIDStr(s)
	cacheKey := settingCacheKey(ourJID, key)
	_ = cache.Set(ctx, cacheKey, value, settingCacheTTL)
	if s.JID != "" && s.JID != ourJID {
		_ = cache.Set(ctx, settingCacheKey(s.JID, key), value, settingCacheTTL)
	}

	setting := BotSetting{
		OurJID: ourJID,
		Key:    key,
		Value:  value,
	}
	return gdb.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "our_jid"}, {Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value"}),
	}).Create(&setting).Error
}

// DeleteSetting removes a custom bot setting by key for this session's our_jid.
func DeleteSetting(ctx context.Context, s *sqlstore.SQLStore, key string) error {
	if s == nil {
		return nil
	}
	gdb, err := GetORM(ctx, s)
	if err != nil {
		return err
	}

	ourJID := ourJIDStr(s)
	_ = cache.Delete(ctx, settingCacheKey(ourJID, key))
	if s.JID != "" && s.JID != ourJID {
		_ = cache.Delete(ctx, settingCacheKey(s.JID, key))
	}

	return gdb.WithContext(ctx).
		Where("(our_jid = ? OR our_jid = ? OR our_jid = '' OR our_jid IS NULL) AND key = ?", ourJID, s.JID, key).
		Delete(&BotSetting{}).Error
}

// ListSettingsWithPrefixes retrieves all settings matching any of the specified key prefixes.
func ListSettingsWithPrefixes(ctx context.Context, s *sqlstore.SQLStore, prefixes ...string) ([]BotSetting, error) {
	if s == nil {
		return nil, nil
	}
	gdb, err := GetORM(ctx, s)
	if err != nil {
		return nil, err
	}
	ourJID := ourJIDStr(s)

	query := gdb.WithContext(ctx).Where("our_jid = ? OR our_jid = ? OR our_jid = '' OR our_jid IS NULL", ourJID, s.JID)
	if len(prefixes) > 0 {
		var conds []string
		var args []any
		for _, p := range prefixes {
			conds = append(conds, "key LIKE ?")
			args = append(args, p+"%")
		}
		query = query.Where(strings.Join(conds, " OR "), args...)
	}

	var results []BotSetting
	err = query.Find(&results).Error
	return results, err
}

// GetCallMediaConfig retrieves call media file path for a user JID and media kind (audio/video), scoped to our_jid.
func GetCallMediaConfig(ctx context.Context, s *sqlstore.SQLStore, jid types.JID, kind CallMediaKind) (string, error) {
	if s == nil {
		return "", nil
	}
	gdb, err := GetORM(ctx, s)
	if err != nil {
		return "", err
	}

	ourJID := ourJIDStr(s)
	jidStr := jid.ToNonAD().String()

	var cfg CallMediaConfig
	findErr := gdb.WithContext(ctx).
		Where("our_jid = ? AND jid = ? AND kind = ?", ourJID, jidStr, string(kind)).
		First(&cfg).Error

	if errors.Is(findErr, gorm.ErrRecordNotFound) && s.JID != "" && s.JID != ourJID {
		findErr = gdb.WithContext(ctx).
			Where("our_jid = ? AND jid = ? AND kind = ?", s.JID, jidStr, string(kind)).
			First(&cfg).Error
	}
	if ourJID == "" || errors.Is(findErr, gorm.ErrRecordNotFound) {
		findErr = gdb.WithContext(ctx).
			Where("(our_jid = '' OR our_jid IS NULL) AND jid = ? AND kind = ?", jidStr, string(kind)).
			First(&cfg).Error
	}

	if errors.Is(findErr, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if findErr != nil {
		return "", findErr
	}
	return cfg.FilePath, nil
}

// PutCallMediaConfig stores call media file path for a user JID and media kind (audio/video), scoped to our_jid.
func PutCallMediaConfig(ctx context.Context, s *sqlstore.SQLStore, jid types.JID, kind CallMediaKind, filePath string) error {
	if s == nil {
		return nil
	}
	gdb, err := GetORM(ctx, s)
	if err != nil {
		return err
	}

	ourJID := ourJIDStr(s)
	cfg := CallMediaConfig{
		OurJID:    ourJID,
		JID:       jid.ToNonAD().String(),
		Kind:      string(kind),
		FilePath:  filePath,
		UpdatedAt: time.Now().Unix(),
	}

	return gdb.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "our_jid"}, {Name: "jid"}, {Name: "kind"}},
		DoUpdates: clause.AssignmentColumns([]string{"file_path", "updated_at"}),
	}).Create(&cfg).Error
}
