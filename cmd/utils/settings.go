package utils

import (
	"context"
	"math/rand"
	"sync"
	"time"
	"whatsrook/cmd/store"

	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
)

const (
	DefaultPrefix    = "."
	PrefixSettingKey = "prefix"

	AFKStatusKey   = "afk_status"
	AFKReasonKey   = "afk_reason"
	AFKTimeKey     = "afk_time"
	AFKTemplateKey = "afk_template"
	AFKMediaKey    = "afk_media"

	AFKLastActiveKey = "owner_last_active"

	BotNameSettingKey          = "bot_name"
	BotNamePromptDismissedKey  = "botname_prompt_dismissed"
	BotNameAwaitingInputPrefix = "botname_awaiting_input:"
	WizardSessionTTL           = 5 * time.Minute
)

type UserAFKState struct {
	LastSent time.Time
	HasSent  bool
}

type WizardSession struct {
	Step      string
	UpdatedAt time.Time
}

var (
	BotNamePromptDismissedCacheMu sync.RWMutex
	BotNamePromptDismissedCache   = make(map[string]bool)

	AFKMu              sync.RWMutex
	LastActiveCache    time.Time
	DefaultAFKTemplate = "I am currently AFK.\n\nReason: {reason}\nTime: {time}\nLast Seen: {last_available}\n\n{quote}"

	AfkUserTracker     = make(map[string]*UserAFKState)
	AfkUserTrackerLock sync.RWMutex

	AutoBioRng      = rand.New(rand.NewSource(time.Now().UnixNano()))
	AutoBioRngMutex sync.Mutex

	BioQuotes = []string{
		"Believe you can and you're halfway there.",
		"The only way to do great work is to love what you do.",
		"Act as if what you do makes a difference. It does.",
		"Dream big and dare to fail.",
		"Stay hungry, stay foolish.",
		"Turn your wounds into wisdom.",
		"Change your thoughts and you change your world.",
		"Simplicity is the soul of efficiency.",
		"Make each day your masterpiece.",
		"Keep your face always toward the sunshine—and shadows will fall behind you.",
	}

	BotWizardMu        sync.RWMutex
	PendingWizardState = make(map[string]WizardSession)

	SupportedTimezones = []string{
		"Africa/Abidjan",
		"Africa/Accra",
		"Africa/Addis_Ababa",
		"Africa/Algiers",
		"Africa/Cairo",
		"Africa/Casablanca",
		"Africa/Johannesburg",
		"Africa/Lagos",
		"Africa/Nairobi",
		"America/Argentina/Buenos_Aires",
		"America/Bogota",
		"America/Caracas",
		"America/Chicago",
		"America/Denver",
		"America/Halifax",
		"America/Lima",
		"America/Los_Angeles",
		"America/Mexico_City",
		"America/New_York",
		"America/Phoenix",
		"America/Santiago",
		"America/Sao_Paulo",
		"America/Toronto",
		"America/Vancouver",
		"Asia/Bangkok",
		"Asia/Colombo",
		"Asia/Dhaka",
		"Asia/Dubai",
		"Asia/Hong_Kong",
		"Asia/Jakarta",
		"Asia/Karachi",
		"Asia/Kolkata",
		"Asia/Kuala_Lumpur",
		"Asia/Manila",
		"Asia/Riyadh",
		"Asia/Seoul",
		"Asia/Shanghai",
		"Asia/Singapore",
		"Asia/Tokyo",
		"Atlantic/Reykjavik",
		"Australia/Adelaide",
		"Australia/Brisbane",
		"Australia/Melbourne",
		"Australia/Perth",
		"Australia/Sydney",
		"Europe/Amsterdam",
		"Europe/Athens",
		"Europe/Berlin",
		"Europe/Brussels",
		"Europe/Dublin",
		"Europe/Istanbul",
		"Europe/Lisbon",
		"Europe/London",
		"Europe/Madrid",
		"Europe/Moscow",
		"Europe/Paris",
		"Europe/Rome",
		"Europe/Warsaw",
		"Pacific/Auckland",
		"Pacific/Fiji",
		"Pacific/Honolulu",
		"UTC",
	}
)

func DismissBotNamePrompt(ctx context.Context, s *sqlstore.SQLStore) {
	if s == nil {
		return
	}
	_ = store.PutSetting(ctx, s, BotNamePromptDismissedKey, "true")
	BotNamePromptDismissedCacheMu.Lock()
	BotNamePromptDismissedCache[s.JID] = true
	if s.JID != "" {
		if parsed, err := types.ParseJID(s.JID); err == nil && !parsed.IsEmpty() {
			BotNamePromptDismissedCache[parsed.ToNonAD().String()] = true
		}
	}
	BotNamePromptDismissedCacheMu.Unlock()
}

func ResetBotNamePromptDismissed(ctx context.Context, s *sqlstore.SQLStore) {
	if s == nil {
		return
	}
	_ = store.PutSetting(ctx, s, BotNamePromptDismissedKey, "false")
	BotNamePromptDismissedCacheMu.Lock()
	delete(BotNamePromptDismissedCache, s.JID)
	if s.JID != "" {
		if parsed, err := types.ParseJID(s.JID); err == nil && !parsed.IsEmpty() {
			delete(BotNamePromptDismissedCache, parsed.ToNonAD().String())
		}
	}
	BotNamePromptDismissedCacheMu.Unlock()
}
