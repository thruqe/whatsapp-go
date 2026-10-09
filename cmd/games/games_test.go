package games

import (
	"strings"
	"testing"

	"go.mau.fi/whatsmeow/types"
)

func TestGenerateGameSession(t *testing.T) {
	jid := types.NewJID("1234567890", types.DefaultUserServer)
	sessionID, err := GenerateGameSession(jid, "PlayerOne", "ttt")
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

	if sess.UserJID != jid || sess.GameType != "ttt" || sess.PushName != "PlayerOne" {
		t.Fatalf("session metadata mismatch: %+v", sess)
	}
}

func TestGetGameServerURL(t *testing.T) {
	url := GetGameServerURL()
	if url == "" {
		t.Fatalf("expected non-empty game server URL")
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

