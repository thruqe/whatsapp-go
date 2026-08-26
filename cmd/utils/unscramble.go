// Package utils provides the core Unscramble word game engine, independent of command handling.
package cliutils

import (
	"math/rand"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

type UnscrambleState int

const (
	UnscrambleStateLobby UnscrambleState = iota
	UnscrambleStateInProgress
)

type UnscramblePlayer struct {
	LID            types.JID
	MentionJID     types.JID
	Tag            string
	Score          int
	Eliminated     bool
	CorrectGuesses int
	TotalTimeMs    int64
	GuessesCount   int
	JoinedAt       time.Time
}

type UnscrambleGame struct {
	Mu             sync.Mutex
	ChatKey        string
	State          UnscrambleState
	HostLID        types.JID
	HostMention    types.JID
	HostTag        string
	Players        []*UnscramblePlayer
	CurrentTurnIdx int
	WordLength     int // 3 to 16
	CurrentWord    string
	ScrambledWord  string
	TurnStartTime  time.Time
	LobbyTimer     *time.Timer
	TurnTimer      *time.Timer
	GameStartTime  time.Time
	Client         *whatsmeow.Client
	ChatJID        types.JID
}

var (
	unscrambleMu    sync.Mutex
	unscrambleGames = make(map[string]*UnscrambleGame) // chat key -> game
)

// wordList holds words by length (3-16 letters)
var wordList = map[int][]string{
	3:  {"cat", "dog", "bat", "rat", "hat", "sun", "run", "fun", "pen", "cup", "box", "fox", "map", "tap", "nap", "lip", "hip", "rib", "web", "mud"},
	4:  {"bird", "fish", "tree", "book", "desk", "lamp", "door", "road", "star", "moon", "hand", "foot", "head", "face", "time", "love", "hope", "fire", "wind", "rain"},
	5:  {"apple", "grape", "lemon", "mango", "peach", "chair", "table", "house", "water", "earth", "music", "dance", "smile", "laugh", "dream", "light", "night", "world", "peace", "power"},
	6:  {"banana", "orange", "cherry", "rabbit", "turtle", "window", "garden", "forest", "bridge", "castle", "island", "desert", "planet", "rocket", "puzzle", "secret", "winter", "summer", "spring", "autumn"},
	7:  {"elephant", "giraffe", "dolphin", "penguin", "leopard", "kitchen", "bedroom", "balcony", "station", "journey", "adventure", "mystery", "history", "science", "fiction", "fantasy", "silence", "thunder", "rainbow", "sunrise"},
	8:  {"dinosaur", "kangaroo", "elephant", "butterfly", "tomorrow", "yesterday", "mountain", "volcano", "tornado", "hurricane", "treasure", "diamond", "emerald", "sapphire", "midnight", "twilight", "starlight", "moonlight", "sunshine", "daylight"},
	9:  {"crocodile", "alligator", "chameleon", "hummingbird", "butterfly", "waterfall", "landscape", "adventure", "discovery", "knowledge", "wisdom", "strength", "courage", "patience", "kindness", "happiness", "sadness", "darkness", "brightness", "greatness"},
	10: {"rhinoceros", "hippopotamus", "chimpanzee", "orangutan", "salamander", "watermelon", "strawberry", "blueberry", "raspberry", "blackberry", "television", "telephone", "microscope", "telescope", "laboratory", "university", "dictionary", "vocabulary", "literature", "philosophy"},
	11: {"caterpillar", "grasshopper", "dragonfly", "hummingbird", "woodpecker", "championship", "competition", "preparation", "destination", "imagination", "information", "combination", "celebration", "conversation", "observation", "examination", "explanation", "application", "development", "environment"},
	12: {"hippopotamus", "parallelogram", "trigonometry", "biotechnology", "microbiology", "astrophysics", "meteorology", "oceanography", "anthropology", "archaeology", "psychiatrist", "ophthalmologist", "cardiologist", "dermatologist", "neurologist", "pediatrician", "veterinarian", "pharmacist", "nutritionist", "chiropractor"},
	13: {"extraordinarily", "characteristics", "responsibilities", "transportation", "communication", "recommendation", "representation", "administration", "demonstration", "investigation", "determination", "organization", "participation", "consideration", "establishment", "improvement", "achievement", "development", "environment", "relationship"},
	14: {"characteristics", "responsibilities", "transportation", "communication", "recommendation", "representation", "administration", "demonstration", "investigation", "determination", "organization", "participation", "consideration", "establishment", "improvement", "achievement", "development", "environment", "relationship", "international"},
	15: {"characterization", "responsibilities", "transportation", "communication", "recommendation", "representation", "administration", "demonstration", "investigation", "determination", "organization", "participation", "consideration", "establishment", "improvement", "achievement", "development", "environment", "relationship", "international"},
	16: {"characterizations", "responsibilities", "transportation", "communication", "recommendation", "representation", "administration", "demonstration", "investigation", "determination", "organization", "participation", "consideration", "establishment", "improvement", "achievement", "development", "environment", "relationship", "international"},
}

// IsUnscrambleGameActive returns true if there is an active Unscramble game (lobby or in-progress) in the chat.
func IsUnscrambleGameActive(chatKey string) bool {
	unscrambleMu.Lock()
	defer unscrambleMu.Unlock()
	_, exists := unscrambleGames[chatKey]
	return exists
}

// GetUnscrambleGame returns the active game for a chat key, or nil.
func GetUnscrambleGame(chatKey string) *UnscrambleGame {
	unscrambleMu.Lock()
	defer unscrambleMu.Unlock()
	return unscrambleGames[chatKey]
}

// CreateUnscrambleGame creates a new lobby for a chat.
func CreateUnscrambleGame(chatKey string, hostLID, hostMention types.JID, hostTag string, chatJID types.JID, client *whatsmeow.Client) *UnscrambleGame {
	game := &UnscrambleGame{
		ChatKey:     chatKey,
		State:       UnscrambleStateLobby,
		HostLID:     hostLID,
		HostMention: hostMention,
		HostTag:     hostTag,
		WordLength:  3,
		ChatJID:     chatJID,
		Client:      client,
	}

	// Host automatically joins
	game.Players = append(game.Players, &UnscramblePlayer{
		LID:        hostLID,
		MentionJID: hostMention,
		Tag:        hostTag,
		JoinedAt:   time.Now(),
	})

	unscrambleMu.Lock()
	unscrambleGames[chatKey] = game
	unscrambleMu.Unlock()

	return game
}

// IsHost returns true if the specified user JID is the game host.
func (g *UnscrambleGame) IsHost(user types.JID) bool {
	g.Mu.Lock()
	defer g.Mu.Unlock()
	u := user.ToNonAD()
	return g.HostLID.ToNonAD() == u || g.HostMention.ToNonAD() == u
}

// DeleteUnscrambleGame removes a game from the active map.
func DeleteUnscrambleGame(chatKey string) {
	unscrambleMu.Lock()
	defer unscrambleMu.Unlock()
	delete(unscrambleGames, chatKey)
}

// AddPlayer adds a player to an existing lobby.
func (g *UnscrambleGame) AddPlayer(lid, mentionJID types.JID, tag string) bool {
	g.Mu.Lock()
	defer g.Mu.Unlock()

	if g.State != UnscrambleStateLobby {
		return false
	}
	if g.FindPlayerIndex(lid) != -1 {
		return false
	}

	g.Players = append(g.Players, &UnscramblePlayer{
		LID:        lid,
		MentionJID: mentionJID,
		Tag:        tag,
		JoinedAt:   time.Now(),
	})
	return true
}

// FindPlayerIndex returns the index of a player by LID, or -1.
func (g *UnscrambleGame) FindPlayerIndex(lid types.JID) int {
	for i, p := range g.Players {
		if p.LID.User == lid.User {
			return i
		}
	}
	return -1
}

// GetActivePlayers returns all non-eliminated players.
func (g *UnscrambleGame) GetActivePlayers() []*UnscramblePlayer {
	var active []*UnscramblePlayer
	for _, p := range g.Players {
		if !p.Eliminated {
			active = append(active, p)
		}
	}
	return active
}

// StartGame transitions the game from lobby to in-progress.
func (g *UnscrambleGame) StartGame() bool {
	g.Mu.Lock()
	defer g.Mu.Unlock()

	if g.State == UnscrambleStateInProgress {
		return false
	}

	g.State = UnscrambleStateInProgress
	g.GameStartTime = time.Now()
	g.WordLength = 3
	g.CurrentTurnIdx = 0

	active := g.GetActivePlayers()
	if len(active) == 0 {
		DeleteUnscrambleGame(g.ChatKey)
		return false
	}

	return true
}

// StartTurn sets up a new turn with a scrambled word.
func (g *UnscrambleGame) StartTurn() (scrambled string, timeLimitSec int, currentPlayer *UnscramblePlayer) {
	g.Mu.Lock()
	defer g.Mu.Unlock()

	active := g.GetActivePlayers()
	if len(active) == 0 {
		return "", 0, nil
	}

	// Ensure currentTurnIdx points to valid active player
	if g.CurrentTurnIdx >= len(g.Players) {
		g.CurrentTurnIdx = 0
	}
	for g.Players[g.CurrentTurnIdx].Eliminated {
		g.CurrentTurnIdx = (g.CurrentTurnIdx + 1) % len(g.Players)
	}

	currentPlayer = g.Players[g.CurrentTurnIdx]

	word, scrambled := GetRandomWord(g.WordLength)
	g.CurrentWord = word
	g.ScrambledWord = scrambled
	g.TurnStartTime = time.Now()

	timeLimitSec = GetTurnTimeLimit(g.WordLength)

	return scrambled, timeLimitSec, currentPlayer
}

// ProcessGuess handles a player's guess. Returns: correct bool, gameOver bool, winner *UnscramblePlayer.
func (g *UnscrambleGame) ProcessGuess(guess string, senderLID types.JID) (correct bool, gameOver bool, winner *UnscramblePlayer, currentPlayer *UnscramblePlayer, elapsed time.Duration) {
	g.Mu.Lock()
	defer g.Mu.Unlock()

	if g.State != UnscrambleStateInProgress {
		return false, true, nil, nil, 0
	}

	// Check if sender is in game and is current turn
	pIdx := g.FindPlayerIndex(senderLID)
	if pIdx == -1 {
		return false, false, nil, nil, 0
	}

	active := g.GetActivePlayers()
	if len(active) == 0 {
		return false, true, nil, nil, 0
	}

	currentPlayer = g.Players[g.CurrentTurnIdx]
	if currentPlayer.LID.User != senderLID.User {
		return false, false, nil, currentPlayer, 0
	}

	guess = strings.ToLower(strings.TrimSpace(guess))
	elapsed = time.Since(g.TurnStartTime)

	if g.TurnTimer != nil {
		g.TurnTimer.Stop()
	}

	currentPlayer.GuessesCount++
	currentPlayer.TotalTimeMs += elapsed.Milliseconds()

	if guess == g.CurrentWord {
		// Correct!
		currentPlayer.Score += g.WordLength * 10
		currentPlayer.CorrectGuesses++

		if g.WordLength < 16 {
			g.WordLength++
		} else {
			// Reached max level - single player wins, or multiplayer continues until someone fails
			// For single player, this is a win condition
			rem := g.GetActivePlayers()
			if len(rem) == 1 {
				return true, true, rem[0], currentPlayer, elapsed
			}
		}

		g.advanceTurnUnsafe()
		return true, false, nil, currentPlayer, elapsed
	}

	// Wrong guess - eliminate player
	currentPlayer.Eliminated = true

	rem := g.GetActivePlayers()
	if len(rem) <= 1 {
		if len(rem) == 1 {
			winner = rem[0]
		}
		return false, true, winner, currentPlayer, elapsed
	}

	g.advanceTurnUnsafe()
	return false, false, nil, currentPlayer, elapsed
}

// advanceTurnUnsafe advances to next non-eliminated player. Must hold g.Mu.
func (g *UnscrambleGame) advanceTurnUnsafe() {
	g.CurrentTurnIdx = (g.CurrentTurnIdx + 1) % len(g.Players)
	for g.Players[g.CurrentTurnIdx].Eliminated {
		g.CurrentTurnIdx = (g.CurrentTurnIdx + 1) % len(g.Players)
	}
}

// AdvanceTurn moves to the next player.
func (g *UnscrambleGame) AdvanceTurn() {
	g.Mu.Lock()
	defer g.Mu.Unlock()
	g.advanceTurnUnsafe()
}

// EliminateCurrentPlayer eliminates the current turn player and advances.
func (g *UnscrambleGame) EliminateCurrentPlayer() (gameOver bool, winner *UnscramblePlayer) {
	g.Mu.Lock()
	defer g.Mu.Unlock()

	if g.CurrentTurnIdx < len(g.Players) {
		g.Players[g.CurrentTurnIdx].Eliminated = true
	}

	rem := g.GetActivePlayers()
	if len(rem) <= 1 {
		if len(rem) == 1 {
			winner = rem[0]
		}
		return true, winner
	}

	g.advanceTurnUnsafe()
	return false, nil
}

// FinishGame cleans up the game and returns final standings.
func (g *UnscrambleGame) FinishGame() (winner *UnscramblePlayer, standings []*UnscramblePlayer) {
	g.Mu.Lock()
	defer g.Mu.Unlock()

	if g.TurnTimer != nil {
		g.TurnTimer.Stop()
	}

	active := g.GetActivePlayers()
	if len(active) == 1 {
		winner = active[0]
	}

	// Sort by score descending
	standings = make([]*UnscramblePlayer, len(g.Players))
	copy(standings, g.Players)
	for i := 0; i < len(standings); i++ {
		for j := i + 1; j < len(standings); j++ {
			if standings[j].Score > standings[i].Score {
				standings[i], standings[j] = standings[j], standings[i]
			}
		}
	}

	DeleteUnscrambleGame(g.ChatKey)
	return winner, standings
}

// GetSortedPlayers returns players sorted by score descending.
func (g *UnscrambleGame) GetSortedPlayers() []*UnscramblePlayer {
	g.Mu.Lock()
	defer g.Mu.Unlock()

	sorted := make([]*UnscramblePlayer, len(g.Players))
	copy(sorted, g.Players)
	for i := range sorted {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].Score > sorted[i].Score {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	return sorted
}

// SetLobbyTimer sets the lobby countdown timer.
func (g *UnscrambleGame) SetLobbyTimer(timer *time.Timer) {
	g.Mu.Lock()
	defer g.Mu.Unlock()
	g.LobbyTimer = timer
}

// SetTurnTimer sets the turn countdown timer.
func (g *UnscrambleGame) SetTurnTimer(timer *time.Timer) {
	g.Mu.Lock()
	defer g.Mu.Unlock()
	g.TurnTimer = timer
}

// StopTimers stops both lobby and turn timers.
func (g *UnscrambleGame) StopTimers() {
	g.Mu.Lock()
	defer g.Mu.Unlock()
	if g.LobbyTimer != nil {
		g.LobbyTimer.Stop()
	}
	if g.TurnTimer != nil {
		g.TurnTimer.Stop()
	}
}

// GetTurnTimeLimit calculates turn duration: Level 3 (3-letter) = 30s, Level 16 (16-letter) = 6s.
func GetTurnTimeLimit(level int) int {
	if level <= 3 {
		return 30
	}
	if level >= 16 {
		return 6
	}
	// Linear interpolation: level 3 -> 30s, level 16 -> 6s
	t := 30 - int(float64(level-3)*(24.0/13.0))
	if t < 6 {
		return 6
	}
	if t > 30 {
		return 30
	}
	return t
}

// GetRandomWord returns a random word of the given length and its scrambled version.
func GetRandomWord(length int) (original string, scrambled string) {
	words, ok := wordList[length]
	if !ok || len(words) == 0 {
		// Fallback: generate random letters
		var b strings.Builder
		for range length {
			b.WriteByte(byte('a' + rand.Intn(26)))
		}
		original = b.String()
		scrambled = scrambleString(original)
		return original, scrambled
	}

	original = words[rand.Intn(len(words))]
	scrambled = scrambleString(original)
	return original, scrambled
}

// scrambleString returns a scrambled version of the string.
func scrambleString(s string) string {
	runes := []rune(s)
	// Try up to 10 times to get a different arrangement
	for range 10 {
		rand.Shuffle(len(runes), func(i, j int) {
			runes[i], runes[j] = runes[j], runes[i]
		})
		scrambled := string(runes)
		if scrambled != s {
			return scrambled
		}
	}
	// If all attempts produce same string (e.g., "aaa"), return as-is
	return string(runes)
}

// GetCXPTitle maps cumulative XP (CXP) to player titles.
func GetCXPTitle(xp int) string {
	switch {
	case xp >= 12000:
		return "👑 Legendary Master"
	case xp >= 7000:
		return "🌟 Legend"
	case xp >= 3500:
		return "⚡ Prolific"
	case xp >= 1500:
		return "🔥 Master"
	case xp >= 500:
		return "⚔️ Pro"
	case xp >= 100:
		return "🌱 Beginner"
	default:
		return "🐣 Novice"
	}
}
