package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"go.mau.fi/whatsmeow/types"
)

var (
	// langCache caches resolved language preferences by JID string.
	langCache   = make(map[string]string)
	langCacheMu sync.RWMutex
)

// DefaultLanguage is the standard fallback language code.
const DefaultLanguage = "en"

// ResolveLanguage determines the active language code for a context or chat.
// Resolution order:
// 1. In-memory cache
// 2. Chat-level DB override (key: "lang:<chat_jid>")
// 3. Global bot-level DB override (key: "bot_language")
// 4. Auto-detect from phone number dial code
// 5. Fallback: DefaultLanguage ("en")
func ResolveLanguage(ctx context.Context, s *StoreWrapper, chat types.JID, sender types.JID) string {
	chatKey := chat.String()
	if chatKey != "" {
		langCacheMu.RLock()
		if cached, ok := langCache[chatKey]; ok && cached != "" {
			langCacheMu.RUnlock()
			return cached
		}
		langCacheMu.RUnlock()
	}

	if s != nil {
		// 1. Check chat-specific language setting
		if chatKey != "" {
			if val, err := s.GetSetting(ctx, "lang:"+chatKey); err == nil && val != "" {
				code := strings.ToLower(strings.TrimSpace(val))
				if _, exists := SupportedLanguages[code]; exists {
					setLangCache(chatKey, code)
					return code
				}
			}
		}

		// 2. Check global bot language setting
		if val, err := s.GetSetting(ctx, "bot_language"); err == nil && val != "" {
			code := strings.ToLower(strings.TrimSpace(val))
			if _, exists := SupportedLanguages[code]; exists {
				if chatKey != "" {
					setLangCache(chatKey, code)
				}
				return code
			}
		}
	}

	// 3. Auto-detect from sender phone prefix
	phone := sender.User
	if phone == "" {
		phone = chat.User
	}
	if phone != "" {
		if meta, ok := DetectOwnerCountryLanguage(phone); ok && meta.LangCode != "" {
			code := strings.ToLower(meta.LangCode)
			if _, exists := SupportedLanguages[code]; exists {
				if chatKey != "" {
					setLangCache(chatKey, code)
				}
				return code
			}
		}
	}

	if chatKey != "" {
		setLangCache(chatKey, DefaultLanguage)
	}
	return DefaultLanguage
}

func setLangCache(key, lang string) {
	langCacheMu.Lock()
	langCache[key] = lang
	langCacheMu.Unlock()
}

// InvalidateLangCache evicts a chat's cached language code.
func InvalidateLangCache(chatKey string) {
	langCacheMu.Lock()
	delete(langCache, chatKey)
	langCacheMu.Unlock()
}

// Translate retrieves a localized string by key and formats it with optional arguments.
// If the key is not found in the target language, it falls back to English.
// If not found in English, it returns the raw key.
func Translate(langCode, key string, args ...any) string {
	code := strings.ToLower(strings.TrimSpace(langCode))
	if code == "" {
		code = DefaultLanguage
	}

	var formatStr string
	var found bool

	if cat, ok := LocaleCatalog[code]; ok {
		formatStr, found = cat[key]
	}

	if !found && code != DefaultLanguage {
		if enCat, ok := LocaleCatalog[DefaultLanguage]; ok {
			formatStr, found = enCat[key]
		}
	}

	if !found {
		formatStr = key
	}

	if len(args) > 0 {
		return fmt.Sprintf(formatStr, args...)
	}
	return formatStr
}

// Lang returns the resolved language code for the current plugin context.
func (c *PluginContext) Lang() string {
	if c == nil {
		return DefaultLanguage
	}
	s, _ := GetStore(c)
	return ResolveLanguage(c.Ctx, s, c.Chat, c.Sender)
}

// T translates a message key for the current context's language.
func (c *PluginContext) T(key string, args ...any) string {
	return Translate(c.Lang(), key, args...)
}

func init() {
	Register(&Command{
		Name:        "language",
		Alias:       "lang,setlang",
		Description: "View or change the bot language for this chat",
		Category:    "settings",
		IsPublic:    true,
		Handler:     handleLanguageCommand,
	})
}

func handleLanguageCommand(ctx *Context) error {
	p := ctx.GetPrefix()
	s, ok := GetStore(ctx)
	if !ok {
		return ctx.Reply(ctx.T("common.store_unavailable"))
	}

	args := ctx.Args
	if len(args) == 0 {
		currentCode := ctx.Lang()
		currentName := SupportedLanguages[currentCode]
		if currentName == "" {
			currentName = currentCode
		}

		var list []string
		for code, name := range SupportedLanguages {
			list = append(list, fmt.Sprintf("• %s: %s", code, name))
		}
		sort.Strings(list)

		reply := fmt.Sprintf("%s\n\n%s",
			ctx.T("lang.current", currentCode, currentName),
			ctx.T("lang.available_list", strings.Join(list, "\n"), p, p),
		)
		return ctx.Reply(reply)
	}

	sub := strings.ToLower(strings.TrimSpace(args[0]))

	if sub == "reset" || sub == "default" {
		_ = s.PutSetting(ctx.Ctx, "lang:"+ctx.Chat.String(), "")
		InvalidateLangCache(ctx.Chat.String())
		return ctx.Reply(ctx.T("lang.reset"))
	}

	// Normalizing target language code
	targetCode := sub
	if len(args) >= 2 && (sub == "set" || sub == "to") {
		targetCode = strings.ToLower(strings.TrimSpace(args[1]))
	}

	name, exists := SupportedLanguages[targetCode]
	if !exists {
		return ctx.Reply(ctx.T("lang.invalid", targetCode))
	}

	// Save setting for this chat
	chatKey := ctx.Chat.String()
	if err := s.PutSetting(ctx.Ctx, "lang:"+chatKey, targetCode); err != nil {
		return ctx.Reply(ctx.T("common.error"))
	}
	setLangCache(chatKey, targetCode)

	return ctx.Reply(ctx.T("lang.set_success", targetCode, name))
}
