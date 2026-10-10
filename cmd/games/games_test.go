package games

import (
	"strings"
	"testing"

	"go.mau.fi/whatsmeow/types"
)

func TestGenerateGameSession(t *testing.T) {
	jid := types.NewJID("1234567890", types.DefaultUserServer)
	sessionID, err := GenerateGameSession(jid, "PlayerOne", "wcg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if sessionID == "" {
		t.Fatalf("expected non-empty session id")
	}

	gameSessionsLock.RLock()
	sess, exists := gameSessions[sessionID]
	gameSessionsLock.RUnlock()

	if !exists {
		t.Fatalf("expected session %s to exist in session store", sessionID)
	}

	if sess.UserJID != jid || sess.GameType != "wcg" || sess.PushName != "PlayerOne" {
		t.Fatalf("session metadata mismatch: %+v", sess)
	}
}

func TestGetGameServerURL(t *testing.T) {
	url := GetGameServerURL()
	if url != "https://whatsrook-games.onrender.com" {
		t.Fatalf("expected DefaultGameServerURL to be https://whatsrook-games.onrender.com, got: %s", url)
	}
}

func TestBuildGameWebviewPayload(t *testing.T) {
	target := "https://whatsrook-games.onrender.com/?session=123"
	payload, err := buildGameWebviewPayload("🎮 Play Connect 4", target)
	if err != nil {
		t.Fatalf("failed to build webview payload: %v", err)
	}

	if !strings.Contains(payload, `"webview_presentation":"full"`) ||
		!strings.Contains(payload, `"webview_interaction":true`) ||
		!strings.Contains(payload, target) {
		t.Fatalf("unexpected payload content: %s", payload)
	}
}

func TestNormalizeGameArgs(t *testing.T) {
	tests := []struct {
		args     []string
		expected string
	}{
		{[]string{"Tic-Tac-Toe"}, "tictactoe"},
		{[]string{"ttt"}, "ttt"},
		{[]string{"1"}, "1"},
		{[]string{"Connect", "4"}, "connect4"},
		{[]string{"c4"}, "c4"},
		{[]string{"Word", "Chain", "Game"}, "wordchaingame"},
		{[]string{"wcg"}, "wcg"},
		{[]string{"Leaderboard"}, "leaderboard"},
		{[]string{"lb"}, "lb"},
	}

	for _, tc := range tests {
		fullArg := strings.ToLower(strings.TrimSpace(strings.Join(tc.args, " ")))
		normalized := strings.ReplaceAll(strings.ReplaceAll(fullArg, "-", ""), " ", "")
		if normalized != tc.expected {
			t.Errorf("for args %v: expected %s, got %s", tc.args, tc.expected, normalized)
		}
	}
}

func TestParseGameType(t *testing.T) {
	tests := []struct {
		input     string
		expectedT string
		expectedN string
	}{
		{"ttt", "ttt", "Tic-Tac-Toe"},
		{"tictactoe", "ttt", "Tic-Tac-Toe"},
		{"c4", "c4", "Connect 4"},
		{"connect4", "c4", "Connect 4"},
		{"wcg", "wcg", "Word Chain Game"},
		{"wordchain", "wcg", "Word Chain Game"},
		{"global", "global", "Global"},
		{"", "global", "Global"},
	}

	for _, tc := range tests {
		gt, gn := parseGameType(tc.input)
		if gt != tc.expectedT || gn != tc.expectedN {
			t.Errorf("parseGameType(%q) = (%q, %q), want (%q, %q)", tc.input, gt, gn, tc.expectedT, tc.expectedN)
		}
	}
}
