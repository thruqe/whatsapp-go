package games

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/types"

	"whatsrook/cmd/dispatch"
	"whatsrook/util/logger"
)

// DefaultGameServerPort is the default port the whatsapp-games server runs on.
const DefaultGameServerPort = "8088"

var (
	gameSessionsLock sync.RWMutex
	gameSessions     = make(map[string]GameSession)
)

// GameSession holds metadata for an authenticated game session.
type GameSession struct {
	SessionID string
	UserJID   types.JID
	PushName  string
	GameType  string
	CreatedAt time.Time
}

func init() {
	dispatch.Register(&dispatch.Command{
		Name:        "games",
		Alias:       "game",
		Description: "Play real-time multiplayer games in WhatsApp Webview",
		Category:    "games",
		IsPublic:    true,
		Handler:     handleGames,
	})

	// Also register alias 'play'
	dispatch.Register(&dispatch.Command{
		Name:         "play",
		Description:  "Quickly launch a multiplayer webview game",
		Category:     "games",
		HideFromMenu: true,
		IsPublic:     true,
		Handler:      handleGames,
	})
}

// GenerateGameSession creates a new cryptographically secure session ID.
func GenerateGameSession(userJID types.JID, pushName, gameType string) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random session id: %w", err)
	}
	sessionID := "gsess_" + hex.EncodeToString(b)

	gameSessionsLock.Lock()
	gameSessions[sessionID] = GameSession{
		SessionID: sessionID,
		UserJID:   userJID,
		PushName:  pushName,
		GameType:  gameType,
		CreatedAt: time.Now(),
	}
	gameSessionsLock.Unlock()

	return sessionID, nil
}

// GetGameServerURL returns the public or local URL where the games server is hosted.
func GetGameServerURL() string {
	if u := os.Getenv("WHATSAPP_GAMES_URL"); u != "" {
		return strings.TrimRight(u, "/")
	}
	// Fallback to local server port
	return "http://localhost:" + DefaultGameServerPort
}

func parseGameType(s string) (gameType string, title string) {
	norm := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(s, "-", ""), " ", ""))
	switch {
	case strings.Contains(norm, "ttt") || strings.Contains(norm, "tictactoe"):
		return "ttt", "Tic-Tac-Toe"
	case strings.Contains(norm, "c4") || strings.Contains(norm, "connect4"):
		return "c4", "Connect 4"
	case strings.Contains(norm, "wcg") || strings.Contains(norm, "wordchain"):
		return "wcg", "Word Chain Game"
	default:
		return "global", "Global"
	}
}

func handleGames(ctx *dispatch.Context) error {
	fullArg := strings.ToLower(strings.TrimSpace(strings.Join(ctx.Args, " ")))
	normalized := strings.ReplaceAll(strings.ReplaceAll(fullArg, "-", ""), " ", "")

	if normalized == "" {
		return showGamesMenu(ctx)
	}

	// Check for leaderboard query
	if strings.HasPrefix(normalized, "lb") ||
		strings.HasPrefix(normalized, "leaderboard") ||
		strings.HasPrefix(normalized, "rank") ||
		strings.HasPrefix(normalized, "top") ||
		strings.Contains(normalized, "leaderboard") ||
		strings.Contains(normalized, "ranking") {

		gameType, gameTitle := parseGameType(normalized)
		if len(ctx.Args) > 1 {
			gameType, gameTitle = parseGameType(ctx.Args[1])
		}
		return handleGamesLeaderboard(ctx, gameType, gameTitle)
	}

	switch {
	case normalized == "ttt" || normalized == "tictactoe" || normalized == "1":
		return launchGame(ctx, "ttt", "Tic-Tac-Toe")
	case normalized == "c4" || normalized == "connect4" || normalized == "2":
		return launchGame(ctx, "c4", "Connect 4")
	case normalized == "wcg" || normalized == "wordchain" || normalized == "wordchaingame" || normalized == "3":
		return launchGame(ctx, "wcg", "Word Chain Game")
	default:
		return showGamesMenu(ctx)
	}
}

func showGamesMenu(ctx *dispatch.Context) error {
	p := ctx.GetPrefix()
	question := "WhatsRook Multiplayer Webview Games\n\n" +
		"Play live real-time games against other players inside WhatsApp webview.\n\n" +
		"Available:\n" +
		"1. Tic-Tac-Toe (" + p + "games ttt)\n" +
		"2. Connect 4 (" + p + "games c4)\n" +
		"3. Word Chain Game (" + p + "games wcg)\n" +
		"4. Leaderboard (" + p + "games lb [ttt|c4|wcg])\n\n" +
		"Select a game or leaderboard:"

	options := []string{
		"Tic-Tac-Toe",
		"Connect 4",
		"Word Chain Game",
		"Global Leaderboard",
		"Tic-Tac-Toe Leaderboard",
		"Connect 4 Leaderboard",
		"Word Chain Leaderboard",
	}

	return dispatch.SendPollReply(ctx, question, options)
}

func launchGame(ctx *dispatch.Context, gameType, gameTitle string) error {
	userJID := ctx.Sender.ToNonAD()
	pushName := ctx.Evt.Info.PushName
	if pushName == "" {
		pushName = userJID.User
	}

	sessionID, err := GenerateGameSession(userJID, pushName, gameType)
	if err != nil {
		logger.Error("failed to generate game session", "err", err)
		return ctx.Reply("Failed to generate game session. Please try again.")
	}

	baseURL := GetGameServerURL()
	gameURL := fmt.Sprintf("%s/?session=%s&user=%s&game=%s&name=%s",
		baseURL, sessionID, userJID.String(), gameType, pushName)

	msg := fmt.Sprintf("%s Webview Session Ready\n\n"+
		"Game: %s (Real-Time Multiplayer)\n"+
		"Player: @%s\n"+
		"Session ID: %s\n\n"+
		"Tap to Play in WhatsApp Webview:\n%s\n\n"+
		"Note: Choose your unique username upon entering. Matchmaking pairs you with live players.",
		gameTitle, gameTitle, userJID.User, sessionID, gameURL)

	return ctx.ReplyWithMentions(msg, []types.JID{userJID})
}

// LeaderboardEntry matches the structure returned by the games server API.
type LeaderboardEntry struct {
	Username   string `json:"username"`
	GameType   string `json:"game_type"`
	Points     int    `json:"points"`
	Wins       int    `json:"wins"`
	Losses     int    `json:"losses"`
	Draws      int    `json:"draws"`
	PointsDiff int    `json:"points_diff"`
}

func handleGamesLeaderboard(ctx *dispatch.Context, gameType, gameTitle string) error {
	baseURL := GetGameServerURL()
	apiURL := fmt.Sprintf("%s/api/leaderboard", baseURL)
	if gameType != "" && gameType != "global" {
		apiURL = fmt.Sprintf("%s/api/leaderboard?game=%s", baseURL, gameType)
	}

	reqCtx, cancel := context.WithTimeout(ctx.Ctx, 4*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, apiURL, nil)
	if err != nil {
		return ctx.Reply("Failed to create leaderboard request.")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ctx.Reply("Leaderboard is currently unavailable (Game server offline or initializing).")
	}
	defer resp.Body.Close()

	var entries []LeaderboardEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return ctx.Reply("Failed to parse leaderboard data.")
	}

	if len(entries) == 0 {
		return ctx.Reply(fmt.Sprintf("Webview Games Leaderboard (%s)\n\nNo players ranked yet. Play %sgames to claim the top spot.", gameTitle, ctx.GetPrefix()))
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Webview Games Leaderboard (%s)\n\n", gameTitle))
	for i, entry := range entries {
		diffSign := ""
		if entry.PointsDiff > 0 {
			diffSign = "+"
		}
		sb.WriteString(fmt.Sprintf("#%d %s\nPoints: %d | %dW / %dL / %dD | Diff: %s%d\n\n",
			i+1, entry.Username, entry.Points, entry.Wins, entry.Losses, entry.Draws, diffSign, entry.PointsDiff))
	}

	return ctx.Reply(strings.TrimSpace(sb.String()))
}
