package tools

import (
	"context"
	"testing"

	"whatsrook/bot/dispatch"

	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

func TestDetectOwnerCountryLanguage(t *testing.T) {
	tests := []struct {
		phone       string
		wantCountry string
		wantLang    string
		wantFound   bool
	}{
		{phone: "+2348012345678", wantCountry: "Nigeria", wantLang: "en", wantFound: true},
		{phone: "2348012345678@s.whatsapp.net", wantCountry: "Nigeria", wantLang: "en", wantFound: true},
		{phone: "+14155552671", wantCountry: "United States / Canada", wantLang: "en", wantFound: true},
		{phone: "33612345678", wantCountry: "France", wantLang: "fr", wantFound: true},
		{phone: "+4915123456789", wantCountry: "Germany", wantLang: "de", wantFound: true},
		{phone: "+34612345678", wantCountry: "Spain", wantLang: "es", wantFound: true},
		{phone: "+5511912345678", wantCountry: "Brazil", wantLang: "pt", wantFound: true},
		{phone: "+919876543210", wantCountry: "India", wantLang: "hi", wantFound: true},
		{phone: "+819012345678", wantCountry: "Japan", wantLang: "ja", wantFound: true},
		{phone: "+201012345678", wantCountry: "Egypt", wantLang: "ar", wantFound: true},
		{phone: "+8613800138000", wantCountry: "China", wantLang: "zh-CN", wantFound: true},
		{phone: "+79161234567", wantCountry: "Russia / Kazakhstan", wantLang: "ru", wantFound: true},
		{phone: "+39061234567", wantCountry: "Italy", wantLang: "it", wantFound: true},
		{phone: "+31612345678", wantCountry: "Netherlands", wantLang: "nl", wantFound: true},
		{phone: "", wantCountry: "United States", wantLang: "en", wantFound: false},
	}

	for _, tt := range tests {
		got, found := DetectOwnerCountryLanguage(tt.phone)
		if found != tt.wantFound {
			t.Errorf("phone %q found=%v, want %v", tt.phone, found, tt.wantFound)
		}
		if got.CountryName != tt.wantCountry {
			t.Errorf("phone %q CountryName=%q, want %q", tt.phone, got.CountryName, tt.wantCountry)
		}
		if got.LangCode != tt.wantLang {
			t.Errorf("phone %q LangCode=%q, want %q", tt.phone, got.LangCode, tt.wantLang)
		}
	}
}

func TestNormalizeLanguageCode(t *testing.T) {
	tests := []struct {
		input    string
		wantCode string
		wantName string
		wantOk   bool
	}{
		{input: "es", wantCode: "es", wantName: "Spanish", wantOk: true},
		{input: "spanish", wantCode: "es", wantName: "Spanish", wantOk: true},
		{input: "to:fr", wantCode: "fr", wantName: "French", wantOk: true},
		{input: "[de]", wantCode: "de", wantName: "German", wantOk: true},
		{input: "japanese", wantCode: "ja", wantName: "Japanese", wantOk: true},
		{input: "ar", wantCode: "ar", wantName: "Arabic", wantOk: true},
		{input: "hausa", wantCode: "ha", wantName: "Hausa", wantOk: true},
		{input: "yoruba", wantCode: "yo", wantName: "Yoruba", wantOk: true},
		{input: "igbo", wantCode: "ig", wantName: "Igbo", wantOk: true},
		{input: "invalidlang123", wantCode: "", wantName: "", wantOk: false},
	}

	for _, tt := range tests {
		code, name, ok := NormalizeLanguageCode(tt.input)
		if ok != tt.wantOk {
			t.Errorf("input %q ok=%v, want %v", tt.input, ok, tt.wantOk)
		}
		if code != tt.wantCode {
			t.Errorf("input %q code=%q, want %q", tt.input, code, tt.wantCode)
		}
		if name != tt.wantName {
			t.Errorf("input %q name=%q, want %q", tt.input, name, tt.wantName)
		}
	}
}

func TestParseTranslateInput(t *testing.T) {
	tests := []struct {
		input            string
		wantTarget       string
		wantText         string
		wantExplicitLang bool
	}{
		{
			input:            `"Hello world"`,
			wantTarget:       "",
			wantText:         "Hello world",
			wantExplicitLang: false,
		},
		{
			input:            `'Hello world'`,
			wantTarget:       "",
			wantText:         "Hello world",
			wantExplicitLang: false,
		},
		{
			input:            `“Hello world”`,
			wantTarget:       "",
			wantText:         "Hello world",
			wantExplicitLang: false,
		},
		{
			input:            `es "Hello world"`,
			wantTarget:       "es",
			wantText:         "Hello world",
			wantExplicitLang: true,
		},
		{
			input:            `to:fr Bonjour le monde`,
			wantTarget:       "fr",
			wantText:         "Bonjour le monde",
			wantExplicitLang: true,
		},
		{
			input:            `spanish How are you doing?`,
			wantTarget:       "es",
			wantText:         "How are you doing?",
			wantExplicitLang: true,
		},
		{
			input:            `Hello world`,
			wantTarget:       "",
			wantText:         "Hello world",
			wantExplicitLang: false,
		},
	}

	for _, tt := range tests {
		targetLang, _, text, explicit := parseTranslateInput(tt.input)
		if explicit != tt.wantExplicitLang {
			t.Errorf("input %q explicit=%v, want %v", tt.input, explicit, tt.wantExplicitLang)
		}
		if targetLang != tt.wantTarget {
			t.Errorf("input %q targetLang=%q, want %q", tt.input, targetLang, tt.wantTarget)
		}
		if text != tt.wantText {
			t.Errorf("input %q text=%q, want %q", tt.input, text, tt.wantText)
		}
	}
}

func TestExecuteTranslation(t *testing.T) {
	ctx := &dispatch.Context{Ctx: context.Background()}
	translated, src, err := executeTranslation(ctx, "Hello world", "es")
	if err != nil {
		t.Skipf("skipping live translation test (network error): %v", err)
	}
	if translated == "" {
		t.Fatalf("expected non-empty translation, got empty string")
	}
	if src == "" {
		t.Fatalf("expected source language to be detected, got empty string")
	}
	t.Logf("translated 'Hello world' -> %q (source: %s)", translated, src)

	translatedPT, srcPT, errPT := executeTranslation(ctx, "tá rápido em", "en")
	if errPT == nil {
		t.Logf("translated 'tá rápido em' -> %q (source: %s)", translatedPT, srcPT)
		if !isSameLanguage(srcPT, "pt") {
			t.Errorf("expected source to be Portuguese (pt), got %s", srcPT)
		}
	}
}

func TestIsSameLanguage(t *testing.T) {
	tests := []struct {
		l1, l2 string
		want   bool
	}{
		{"pt", "pt", true},
		{"pt-BR", "pt", true},
		{"pt", "pt-PT", true},
		{"en", "en-US", true},
		{"ES", "es", true},
		{"pt", "es", false},
		{"en", "fr", false},
		{"", "pt", false},
		{"pt", "", false},
	}
	for _, tt := range tests {
		got := isSameLanguage(tt.l1, tt.l2)
		if got != tt.want {
			t.Errorf("isSameLanguage(%q, %q) = %v, want %v", tt.l1, tt.l2, got, tt.want)
		}
	}
}

func TestResolvePhoneNumber(t *testing.T) {
	// Standard phone number JID
	pnJID := types.NewJID("559181616484", types.DefaultUserServer)
	ctx := &dispatch.Context{Sender: pnJID}
	if got := resolvePhoneNumber(ctx, pnJID); got != "559181616484" {
		t.Errorf("resolvePhoneNumber for PN JID got %q, want %q", got, "559181616484")
	}

	// LID JID without mapping must NEVER return the LID number as a phone number
	lidJID := types.NewJID("258123456789", types.HiddenUserServer)
	ctxLID := &dispatch.Context{Sender: lidJID}
	if got := resolvePhoneNumber(ctxLID, lidJID); got != "" {
		t.Errorf("resolvePhoneNumber for LID JID without mapping got %q, want empty string", got)
	}

	// LID JID with SenderAlt PN
	ctxWithAlt := &dispatch.Context{
		Sender: lidJID,
		Evt: &events.Message{
			Info: types.MessageInfo{
				Sender:    lidJID,
				SenderAlt: types.NewJID("559181616484", types.DefaultUserServer),
			},
		},
	}
	if got := resolvePhoneNumber(ctxWithAlt, lidJID); got != "559181616484" {
		t.Errorf("resolvePhoneNumber for LID with SenderAlt got %q, want %q", got, "559181616484")
	}
}
