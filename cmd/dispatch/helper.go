// dispatch package provides helper functions for database storage access,
// authorization validation, JID normalization, and reactive message inspection.
package dispatch

import (
	"context"
	"path/filepath"
	"strings"
	"sync"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"whatsrook/cmd/store"
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
	return store.GetSetting(ctx, w.SQLStore, key)
}

// PutSetting writes or updates a configuration value in the bot_settings table.
func (w *StoreWrapper) PutSetting(ctx context.Context, key, value string) error {
	if w == nil || w.SQLStore == nil {
		return nil
	}
	return store.PutSetting(ctx, w.SQLStore, key, value)
}

// DeleteSetting removes a configuration setting from the bot_settings table.
func (w *StoreWrapper) DeleteSetting(ctx context.Context, key string) error {
	if w == nil || w.SQLStore == nil {
		return nil
	}
	return store.DeleteSetting(ctx, w.SQLStore, key)
}

// GetCallMediaConfig retrieves the configured media file path for VoIP calls.
func (w *StoreWrapper) GetCallMediaConfig(ctx context.Context, jid types.JID, kind store.CallMediaKind) (string, error) {
	if w == nil || w.SQLStore == nil {
		return "", nil
	}
	return store.GetCallMediaConfig(ctx, w.SQLStore, jid, kind)
}

// PutCallMediaConfig persists the configured media file path for VoIP calls.
func (w *StoreWrapper) PutCallMediaConfig(ctx context.Context, jid types.JID, kind store.CallMediaKind, filePath string) error {
	if w == nil || w.SQLStore == nil {
		return nil
	}
	return store.PutCallMediaConfig(ctx, w.SQLStore, jid, kind, filePath)
}

// GetSessionMediaDir returns the directory path for storing session media assets.
func GetSessionMediaDir(client *whatsmeow.Client, subdirs ...string) string {
	baseDir := "media"
	if client != nil && client.Store != nil && client.Store.ID != nil {
		user := client.Store.ID.User
		if user != "" {
			baseDir = filepath.Join("sessions", user, "media")
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

// RecentMessageStore caches recent incoming messages for call responses and media conversion lookups.
type RecentMessageStore struct {
	mu       sync.RWMutex
	messages map[types.JID]*events.Message
}

var recentMessages = &RecentMessageStore{
	messages: make(map[types.JID]*events.Message),
}

// RecordRecentMessage caches an incoming message event indexed by its chat JID.
func RecordRecentMessage(evt *events.Message) {
	if evt == nil {
		return
	}
	recentMessages.mu.Lock()
	defer recentMessages.mu.Unlock()
	recentMessages.messages[evt.Info.Chat] = evt
}

// GetRecentMessage returns the most recent message event recorded for a chat JID.
func GetRecentMessage(chat types.JID) (*events.Message, bool) {
	recentMessages.mu.RLock()
	defer recentMessages.mu.RUnlock()
	evt, ok := recentMessages.messages[chat]
	return evt, ok
}

// DismissBotNamePrompt marks the bot name setup prompt as dismissed.
func DismissBotNamePrompt(ctx context.Context, s *StoreWrapper) {
	if s != nil {
		_ = s.PutSetting(ctx, "bot_name_dismissed", "true")
	}
}

// ResetBotNamePromptDismissed clears the bot name setup prompt dismissal state.
func ResetBotNamePromptDismissed(ctx context.Context, s *StoreWrapper) {
	if s != nil {
		_ = s.DeleteSetting(ctx, "bot_name_dismissed")
	}
}
