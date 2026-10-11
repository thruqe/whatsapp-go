package main

import (
	"strings"
	"testing"
	"unicode"
)

// isEmoji returns true if a rune is within common emoji Unicode ranges.
func isEmoji(r rune) bool {
	return (r >= 0x1F300 && r <= 0x1FAFF) ||
		(r >= 0x2600 && r <= 0x27BF) ||
		(r >= 0xFE00 && r <= 0xFE0F) ||
		(r >= 0x1F900 && r <= 0x1F9FF) ||
		(r >= 0x1F600 && r <= 0x1F64F) ||
		(r >= 0x1F680 && r <= 0x1F6FF) ||
		(r >= 0x2300 && r <= 0x23FF)
}

func TestSupportedLanguages(t *testing.T) {
	expectedLangs := []string{"en", "es", "id", "pt", "fr", "ar", "ru", "de"}
	for _, lang := range expectedLangs {
		if _, ok := SupportedLanguages[lang]; !ok {
			t.Errorf("expected %q to be in SupportedLanguages", lang)
		}
		if _, ok := LocaleCatalog[lang]; !ok {
			t.Errorf("expected %q to be in LocaleCatalog", lang)
		}
	}
}

func TestLocaleCatalogConsistency(t *testing.T) {
	enCat, ok := LocaleCatalog["en"]
	if !ok {
		t.Fatal("en catalog missing")
	}

	for lang, cat := range LocaleCatalog {
		for key := range enCat {
			val, exists := cat[key]
			if !exists {
				t.Errorf("lang %q missing key %q present in en catalog", lang, key)
			} else if strings.TrimSpace(val) == "" {
				t.Errorf("lang %q has empty string for key %q", lang, key)
			}
		}

		for key := range cat {
			if _, exists := enCat[key]; !exists {
				t.Errorf("lang %q has extra key %q not in en catalog", lang, key)
			}
		}
	}
}

func TestRegisteredCommandsLocalized(t *testing.T) {
	registryMu.RLock()
	defer registryMu.RUnlock()

	for name, cmd := range registry {
		// Only test primary command name, not aliases
		if strings.ToLower(cmd.Name) != name {
			continue
		}

		key := "cmd." + name + ".desc"
		for lang := range SupportedLanguages {
			desc := cmd.LocalizedDescription(lang)
			if desc == "" {
				t.Errorf("command %q has empty localized description for lang %q", name, lang)
			}
			val := Translate(lang, key)
			if val == key {
				t.Errorf("command %q missing translation key %q for lang %q", name, key, lang)
			}
		}
	}
}

func TestOfficialPluginsLocalized(t *testing.T) {
	for _, pluginName := range OfficialPlugins {
		key := "cmd." + strings.ToLower(pluginName) + ".desc"
		for lang := range SupportedLanguages {
			val := Translate(lang, key)
			if val == key || strings.TrimSpace(val) == "" {
				t.Errorf("plugin %q missing localized description key %q for lang %q", pluginName, key, lang)
			}
		}
	}
}

func TestNoForbiddenFormattingAndNoEmojis(t *testing.T) {
	forbidden := []string{"*", "_", "~", "`", ">"}

	for lang, cat := range LocaleCatalog {
		for key, val := range cat {
			for _, f := range forbidden {
				if strings.Contains(val, f) {
					t.Errorf("lang %q key %q contains forbidden character %q: %s", lang, key, f, val)
				}
			}

			for _, r := range val {
				if isEmoji(r) {
					t.Errorf("lang %q key %q contains emoji character %q (U+%04X): %s", lang, key, r, r, val)
					break
				}
				// Verify no unprintable non-spacing control characters except standard whitespace
				if unicode.IsControl(r) && r != '\n' && r != '\t' && r != '\r' {
					t.Errorf("lang %q key %q contains control character (U+%04X): %s", lang, key, r, val)
					break
				}
			}
		}
	}
}

func TestTranslateFallback(t *testing.T) {
	// Unknown language falls back to English
	fallback := Translate("nonexistent_lang", "common.error")
	expected := LocaleCatalog["en"]["common.error"]
	if fallback != expected {
		t.Errorf("expected fallback to %q, got %q", expected, fallback)
	}

	// Unknown key returns key itself
	unknownKey := Translate("en", "unknown.nonexistent.key")
	if unknownKey != "unknown.nonexistent.key" {
		t.Errorf("expected key itself, got %q", unknownKey)
	}

	// Argument formatting
	formatted := Translate("en", "ping.pong_ms", 42)
	if formatted != "Pong! 42 ms" {
		t.Errorf("unexpected formatted output: %q", formatted)
	}
}
