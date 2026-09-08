package owner

import (
	"reflect"
	"testing"
)

func TestIsValidUsername(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"thruqe", true},
		{"user_123", true},
		{"bot.admin-1", true},
		{"", false},
		{"user name", false},
		{"『𖥠』ємρєяσя{", false},
		{"𝕱𝖆𝖙𝖍𝖊𝖗", false},
		{"user@domain", false},
		{"user:pass", false},
		{"this_is_a_very_long_username_exceeding_thirty_chars", false},
	}

	for _, tt := range tests {
		got := isValidUsername(tt.input)
		if got != tt.want {
			t.Errorf("isValidUsername(%q) = %v; want %v", tt.input, got, tt.want)
		}
	}
}

func TestSanitizeSudoers(t *testing.T) {
	input := []string{
		"2348000000001@s.whatsapp.net",
		"123456789012346@lid",
		"『𖥠』ємρєяσя{",
		"𝕱𝖆𝖙𝖍𝖊𝖗",
		"𝖔𝖋",
		"𝕷𝖔𝖗𝖉𝖘",
		"}",
		"testuser",
		"2348000000001",
		"+2348000000001",
		"2348000000001@s.whatsapp.net", // duplicate
		"",
	}

	expected := []string{
		"2348000000001@s.whatsapp.net",
		"123456789012346@lid",
		"testuser",
		"2348000000001",
		"+2348000000001",
	}

	got := sanitizeSudoers(input)
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("sanitizeSudoers() = %v, want %v", got, expected)
	}
}
