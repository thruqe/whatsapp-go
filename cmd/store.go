package main

import (
	"context"

	clistore "whatsrook/cmd/store"

	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
)

// Store wraps custom WhatsRook bot database tables and settings operations.
type Store struct {
	SQLStore *sqlstore.SQLStore
}

// Init ensures all custom tables and schema migrations are applied.
func (s *Store) Init(ctx context.Context) {
	if s != nil && s.SQLStore != nil {
		clistore.InitTables(ctx, s.SQLStore)
	}
}

// GetSetting retrieves a setting value from the bot_settings table.
func (s *Store) GetSetting(ctx context.Context, key string) (string, error) {
	if s == nil || s.SQLStore == nil {
		return "", nil
	}
	s.Init(ctx)
	return clistore.GetSetting(ctx, s.SQLStore, key)
}

// PutSetting saves a key-value setting to the bot_settings table.
func (s *Store) PutSetting(ctx context.Context, key, value string) error {
	if s == nil || s.SQLStore == nil {
		return nil
	}
	s.Init(ctx)
	return clistore.PutSetting(ctx, s.SQLStore, key, value)
}

// DeleteSetting removes a key-value setting from the bot_settings table.
func (s *Store) DeleteSetting(ctx context.Context, key string) error {
	if s == nil || s.SQLStore == nil {
		return nil
	}
	s.Init(ctx)
	return clistore.DeleteSetting(ctx, s.SQLStore, key)
}

// GetCallMediaConfig retrieves call media configuration from the call_media_config table.
func (s *Store) GetCallMediaConfig(ctx context.Context, sender types.JID, kind clistore.CallMediaKind) (string, error) {
	if s == nil || s.SQLStore == nil {
		return "", nil
	}
	s.Init(ctx)
	return clistore.GetCallMediaConfig(ctx, s.SQLStore, sender, kind)
}

// PutCallMediaConfig stores call media configuration in the call_media_config table.
func (s *Store) PutCallMediaConfig(ctx context.Context, sender types.JID, kind clistore.CallMediaKind, filePath string) error {
	if s == nil || s.SQLStore == nil {
		return nil
	}
	s.Init(ctx)
	return clistore.PutCallMediaConfig(ctx, s.SQLStore, sender, kind, filePath)
}
