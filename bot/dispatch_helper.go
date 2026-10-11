// dispatch package provides helper functions for database storage access,
// authorization validation, JID normalization, and reactive message inspection.
package main

import (
	"context"
	"path/filepath"
	"strings"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"github.com/thruqe/whatsrook"
)

// StoreWrapper wraps sqlstore.SQLStore to expose strongly-typed settings access methods.
type StoreWrapper struct {
	*sqlstore.SQLStore
}

// Wrap wraps an active sqlstore.SQLStore instance.
func Wrap(s *sqlstore.SQLStore) *StoreWrapper {
	if s == nil {
		return nil
	}
	return &StoreWrapper{SQLStore: s}
}

// GetSetting retrieves a configuration value from the bot_settings table.
func (w *StoreWrapper) GetSetting(ctx context.Context, key string) (string, error) {
	if w == nil || w.SQLStore == nil {
		return "", nil
	}
	return GetSetting(ctx, w.SQLStore, key)
}

// PutSetting writes or updates a configuration value in the bot_settings table.
func (w *StoreWrapper) PutSetting(ctx context.Context, key, value string) error {
	if w == nil || w.SQLStore == nil {
		return nil
	}
	return PutSetting(ctx, w.SQLStore, key, value)
}

// DeleteSetting removes a configuration setting from the bot_settings table.
func (w *StoreWrapper) DeleteSetting(ctx context.Context, key string) error {
	if w == nil || w.SQLStore == nil {
		return nil
	}
	return DeleteSetting(ctx, w.SQLStore, key)
}

// GetCallMediaConfig retrieves the configured media file path for VoIP calls.
func (w *StoreWrapper) GetCallMediaConfig(ctx context.Context, jid types.JID, kind CallMediaKind) (string, error) {
	if w == nil || w.SQLStore == nil {
		return "", nil
	}
	return GetCallMediaConfig(ctx, w.SQLStore, jid, kind)
}

// PutCallMediaConfig persists the configured media file path for VoIP calls.
func (w *StoreWrapper) PutCallMediaConfig(ctx context.Context, jid types.JID, kind CallMediaKind, filePath string) error {
	if w == nil || w.SQLStore == nil {
		return nil
	}
	return PutCallMediaConfig(ctx, w.SQLStore, jid, kind, filePath)
}

// PutPlatformCookie stores Netscape cookies for a specific platform.
func (w *StoreWrapper) PutPlatformCookie(ctx context.Context, platform, domain, cookies string) error {
	if w == nil || w.SQLStore == nil {
		return nil
	}
	return PutPlatformCookie(ctx, w.SQLStore, platform, domain, cookies)
}

// GetPlatformCookie retrieves Netscape cookies for a specific platform.
func (w *StoreWrapper) GetPlatformCookie(ctx context.Context, platform string) (string, error) {
	if w == nil || w.SQLStore == nil {
		return "", nil
	}
	return GetPlatformCookie(ctx, w.SQLStore, platform)
}

// DeletePlatformCookie deletes Netscape cookies for a specific platform.
func (w *StoreWrapper) DeletePlatformCookie(ctx context.Context, platform string) error {
	if w == nil || w.SQLStore == nil {
		return nil
	}
	return DeletePlatformCookie(ctx, w.SQLStore, platform)
}

// DeleteAllPlatformCookies deletes all platform cookies.
func (w *StoreWrapper) DeleteAllPlatformCookies(ctx context.Context) error {
	if w == nil || w.SQLStore == nil {
		return nil
	}
	return DeleteAllPlatformCookies(ctx, w.SQLStore)
}

// ListPlatformCookies returns all platform cookies configured for this session.
func (w *StoreWrapper) ListPlatformCookies(ctx context.Context) ([]BotPlatformCookie, error) {
	if w == nil || w.SQLStore == nil {
		return nil, nil
	}
	return ListPlatformCookies(ctx, w.SQLStore)
}

// GetAllPlatformCookiesMerged returns all platform cookies combined into a single Netscape cookie file.
func (w *StoreWrapper) GetAllPlatformCookiesMerged(ctx context.Context) (string, error) {
	if w == nil || w.SQLStore == nil {
		return "", nil
	}
	return GetAllPlatformCookiesMerged(ctx, w.SQLStore)
}

// GetSessionMediaDir returns the directory path for storing session media assets inside .whatsrook.
func GetSessionMediaDir(client *whatsmeow.Client, subdirs ...string) string {
	baseDir := filepath.Join(whatsrook.DefaultDataDir(), "sessions", "default", "media")
	if client != nil && client.Store != nil && client.Store.ID != nil {
		user := client.Store.ID.User
		if user != "" {
			baseDir = filepath.Join(whatsrook.DefaultDataDir(), "sessions", user, "media")
		}
	}
	if len(subdirs) > 0 {
		return filepath.Join(append([]string{baseDir}, subdirs...)...)
	}
	return baseDir
}

// GetBotName retrieves the customized bot name or default "WhatsRook".
func GetBotName(ctx context.Context, s *StoreWrapper) string {
	if s != nil {
		if name, err := s.GetSetting(ctx, "bot_name"); err == nil && strings.TrimSpace(name) != "" {
			return strings.TrimSpace(name)
		}
	}
	return "WhatsRook"
}

// GetSQLStore extracts the StoreWrapper from the active client instance.
func GetSQLStore(client *whatsmeow.Client) (*StoreWrapper, bool) {
	if client == nil || client.Store == nil || client.Store.Identities == nil {
		return nil, false
	}
	s, ok := client.Store.Identities.(*sqlstore.SQLStore)
	if !ok || s == nil {
		return nil, false
	}
	return Wrap(s), true
}

// GetStore extracts the StoreWrapper from the Context.
func GetStore(ctx *Context) (*StoreWrapper, bool) {
	if ctx == nil {
		return nil, false
	}
	return GetSQLStore(ctx.Client)
}

// RecordRecentMessage caches an incoming message event.
func RecordRecentMessage(evt *events.Message) {
	whatsrook.RecordRecentMessage(evt)
}

func init() {
	GlobalSettingGetter = func(ctx context.Context, client *whatsmeow.Client, key string) (string, error) {
		if s, ok := GetSQLStore(client); ok {
			return s.GetSetting(ctx, key)
		}
		return "", nil
	}
}
