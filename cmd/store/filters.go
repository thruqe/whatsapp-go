package store

import (
	"context"
	"errors"

	"go.mau.fi/whatsmeow/store/sqlstore"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GetFilter retrieves a custom filter message proto by trigger word.
func GetFilter(ctx context.Context, s *sqlstore.SQLStore, trigger string) (string, error) {
	if s == nil {
		return "", nil
	}
	gdb, err := GetORM(ctx, s)
	if err != nil {
		return "", err
	}
	ourJID := ourJIDStr(s)

	var f BotFilter
	findErr := gdb.WithContext(ctx).
		Where("our_jid = ? AND trigger_word = ?", ourJID, trigger).
		First(&f).Error

	if errors.Is(findErr, gorm.ErrRecordNotFound) && s.JID != "" && s.JID != ourJID {
		findErr = gdb.WithContext(ctx).
			Where("our_jid = ? AND trigger_word = ?", s.JID, trigger).
			First(&f).Error
	}
	if ourJID == "" || errors.Is(findErr, gorm.ErrRecordNotFound) {
		findErr = gdb.WithContext(ctx).
			Where("(our_jid = '' OR our_jid IS NULL) AND trigger_word = ?", trigger).
			First(&f).Error
	}

	if errors.Is(findErr, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if findErr != nil {
		return "", findErr
	}
	return f.MessageProto, nil
}

// PutFilter saves or updates a custom trigger filter.
func PutFilter(ctx context.Context, s *sqlstore.SQLStore, trigger, messageProto string) error {
	if s == nil {
		return nil
	}
	gdb, err := GetORM(ctx, s)
	if err != nil {
		return err
	}
	ourJID := ourJIDStr(s)

	f := BotFilter{
		OurJID:       ourJID,
		TriggerWord:  trigger,
		MessageProto: messageProto,
	}
	return gdb.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "our_jid"}, {Name: "trigger_word"}},
		DoUpdates: clause.AssignmentColumns([]string{"message_proto"}),
	}).Create(&f).Error
}

// DeleteFilter removes a trigger filter.
func DeleteFilter(ctx context.Context, s *sqlstore.SQLStore, trigger string) error {
	if s == nil {
		return nil
	}
	gdb, err := GetORM(ctx, s)
	if err != nil {
		return err
	}
	ourJID := ourJIDStr(s)
	return gdb.WithContext(ctx).
		Where("(our_jid = ? OR our_jid = ? OR our_jid = '' OR our_jid IS NULL) AND trigger_word = ?", ourJID, s.JID, trigger).
		Delete(&BotFilter{}).Error
}

// ListFilters returns all trigger words registered for this session.
func ListFilters(ctx context.Context, s *sqlstore.SQLStore) ([]string, error) {
	if s == nil {
		return nil, nil
	}
	gdb, err := GetORM(ctx, s)
	if err != nil {
		return nil, err
	}
	ourJID := ourJIDStr(s)

	var triggers []string
	err = gdb.WithContext(ctx).
		Model(&BotFilter{}).
		Where("our_jid = ? OR our_jid = ? OR our_jid = '' OR our_jid IS NULL", ourJID, s.JID).
		Order("trigger_word ASC").
		Pluck("trigger_word", &triggers).Error

	return triggers, err
}

// GetBGM retrieves a background music message proto by trigger word.
func GetBGM(ctx context.Context, s *sqlstore.SQLStore, trigger string) (string, error) {
	if s == nil {
		return "", nil
	}
	gdb, err := GetORM(ctx, s)
	if err != nil {
		return "", err
	}
	ourJID := ourJIDStr(s)

	var b BotBGM
	findErr := gdb.WithContext(ctx).
		Where("our_jid = ? AND trigger_word = ?", ourJID, trigger).
		First(&b).Error

	if errors.Is(findErr, gorm.ErrRecordNotFound) && s.JID != "" && s.JID != ourJID {
		findErr = gdb.WithContext(ctx).
			Where("our_jid = ? AND trigger_word = ?", s.JID, trigger).
			First(&b).Error
	}
	if ourJID == "" || errors.Is(findErr, gorm.ErrRecordNotFound) {
		findErr = gdb.WithContext(ctx).
			Where("(our_jid = '' OR our_jid IS NULL) AND trigger_word = ?", trigger).
			First(&b).Error
	}

	if errors.Is(findErr, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if findErr != nil {
		return "", findErr
	}
	return b.MessageProto, nil
}

// PutBGM saves or updates a background music trigger.
func PutBGM(ctx context.Context, s *sqlstore.SQLStore, trigger, messageProto string) error {
	if s == nil {
		return nil
	}
	gdb, err := GetORM(ctx, s)
	if err != nil {
		return err
	}
	ourJID := ourJIDStr(s)

	b := BotBGM{
		OurJID:       ourJID,
		TriggerWord:  trigger,
		MessageProto: messageProto,
	}
	return gdb.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "our_jid"}, {Name: "trigger_word"}},
		DoUpdates: clause.AssignmentColumns([]string{"message_proto"}),
	}).Create(&b).Error
}

// DeleteBGM removes a background music trigger.
func DeleteBGM(ctx context.Context, s *sqlstore.SQLStore, trigger string) error {
	if s == nil {
		return nil
	}
	gdb, err := GetORM(ctx, s)
	if err != nil {
		return err
	}
	ourJID := ourJIDStr(s)
	return gdb.WithContext(ctx).
		Where("(our_jid = ? OR our_jid = ? OR our_jid = '' OR our_jid IS NULL) AND trigger_word = ?", ourJID, s.JID, trigger).
		Delete(&BotBGM{}).Error
}

// ListBGMs returns all background music trigger words registered for this session.
func ListBGMs(ctx context.Context, s *sqlstore.SQLStore) ([]string, error) {
	if s == nil {
		return nil, nil
	}
	gdb, err := GetORM(ctx, s)
	if err != nil {
		return nil, err
	}
	ourJID := ourJIDStr(s)

	var triggers []string
	err = gdb.WithContext(ctx).
		Model(&BotBGM{}).
		Where("our_jid = ? OR our_jid = ? OR our_jid = '' OR our_jid IS NULL", ourJID, s.JID).
		Order("trigger_word ASC").
		Pluck("trigger_word", &triggers).Error

	return triggers, err
}

// GetStickerCmd retrieves command name associated with a sticker SHA256.
func GetStickerCmd(ctx context.Context, s *sqlstore.SQLStore, shaHex string) (string, error) {
	if s == nil {
		return "", nil
	}
	gdb, err := GetORM(ctx, s)
	if err != nil {
		return "", err
	}
	ourJID := ourJIDStr(s)

	var sc BotStickerCmd
	findErr := gdb.WithContext(ctx).
		Where("our_jid = ? AND sticker_sha256 = ?", ourJID, shaHex).
		First(&sc).Error

	if errors.Is(findErr, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if findErr != nil {
		return "", findErr
	}
	return sc.CommandName, nil
}

// PutStickerCmd stores a sticker hash to command name mapping.
func PutStickerCmd(ctx context.Context, s *sqlstore.SQLStore, shaHex, cmdName string) error {
	if s == nil {
		return nil
	}
	gdb, err := GetORM(ctx, s)
	if err != nil {
		return err
	}
	ourJID := ourJIDStr(s)

	sc := BotStickerCmd{
		OurJID:        ourJID,
		StickerSHA256: shaHex,
		CommandName:   cmdName,
	}
	return gdb.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "our_jid"}, {Name: "sticker_sha256"}},
		DoUpdates: clause.AssignmentColumns([]string{"command_name"}),
	}).Create(&sc).Error
}

// DeleteStickerCmdBySHA removes a sticker command mapping by its sha256 hash.
func DeleteStickerCmdBySHA(ctx context.Context, s *sqlstore.SQLStore, shaHex string) error {
	if s == nil {
		return nil
	}
	gdb, err := GetORM(ctx, s)
	if err != nil {
		return err
	}
	ourJID := ourJIDStr(s)
	return gdb.WithContext(ctx).
		Where("our_jid = ? AND sticker_sha256 = ?", ourJID, shaHex).
		Delete(&BotStickerCmd{}).Error
}

// DeleteStickerCmdByName removes all sticker command mappings matching command name.
func DeleteStickerCmdByName(ctx context.Context, s *sqlstore.SQLStore, cmdName string) error {
	if s == nil {
		return nil
	}
	gdb, err := GetORM(ctx, s)
	if err != nil {
		return err
	}
	ourJID := ourJIDStr(s)
	return gdb.WithContext(ctx).
		Where("our_jid = ? AND command_name = ?", ourJID, cmdName).
		Delete(&BotStickerCmd{}).Error
}

// ListStickerCmds retrieves all sticker command mappings for this session.
func ListStickerCmds(ctx context.Context, s *sqlstore.SQLStore) ([]BotStickerCmd, error) {
	if s == nil {
		return nil, nil
	}
	gdb, err := GetORM(ctx, s)
	if err != nil {
		return nil, err
	}
	ourJID := ourJIDStr(s)

	var list []BotStickerCmd
	err = gdb.WithContext(ctx).
		Where("our_jid = ?", ourJID).
		Find(&list).Error
	return list, err
}
