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

func handleGames(ctx *dispatch.Context) error {
	fullArg := strings.ToLower(strings.TrimSpace(strings.Join(ctx.Args, " ")))
	normalized := strings.ReplaceAll(strings.ReplaceAll(fullArg, "-", ""), " ", "")

	switch {
	case normalized == "lb" || normalized == "leaderboard" || normalized == "rank" || normalized == "ranking" || normalized == "top":
		return handleGamesLeaderboard(ctx)
	case normalized == "ttt" || normalized == "tictactoe" || normalized == "1":
		return launchGame(ctx, "ttt", "Tic-Tac-Toe")
	case normalized == "c4" || normalized == "connect4" || normalized == "2":
		return launchGame(ctx, "c4", "Connect 4")
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
		"3. Leaderboard (" + p + "games lb)\n\n" +
		"Select a game or view rankings:"

	options := []string{
		"Tic-Tac-Toe",
		"Connect 4",
		"Leaderboard",
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
	Username string `json:"username"`
	Rating   int    `json:"rating"`
	Wins     int    `json:"wins"`
	Losses   int    `json:"losses"`
	Draws    int    `json:"draws"`
}

func handleGamesLeaderboard(ctx *dispatch.Context) error {
	apiURL := fmt.Sprintf("%s/api/leaderboard", GetGameServerURL())
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
		return ctx.Reply("Webview Games Leaderboard\n\nNo players ranked yet. Play " + ctx.GetPrefix() + "games to claim the top spot.")
	}

	var sb strings.Builder
	sb.WriteString("Webview Games Global Leaderboard\n\n")
	for i, entry := range entries {
		sb.WriteString(fmt.Sprintf("#%d %s\nRating: %d | %dW / %dL / %dD\n\n",
			i+1, entry.Username, entry.Rating, entry.Wins, entry.Losses, entry.Draws))
	}

	return ctx.Reply(strings.TrimSpace(sb.String()))
}
