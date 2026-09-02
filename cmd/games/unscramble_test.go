package games

import (
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

func TestUnscrambleWordListLengths(t *testing.T) {
	for length := 3; length <= 16; length++ {
		words, ok := wordList[length]
		if !ok || len(words) == 0 {
			t.Fatalf("wordList missing words for length %d", length)
		}
		for _, w := range words {
			if len(w) != length {
				t.Errorf("word '%s' in level %d has length %d, expected %d", w, length, len(w), length)
			}
		}
	}
}

func TestScrambleString(t *testing.T) {
	original := "elephant"
	scrambled := scrambleString(original)

	if len(scrambled) != len(original) {
		t.Errorf("scrambled length %d does not match original length %d", len(scrambled), len(original))
	}

	origCounts := make(map[rune]int)
	for _, r := range original {
		origCounts[r]++
	}
	scrambCounts := make(map[rune]int)
	for _, r := range scrambled {
		scrambCounts[r]++
	}
	for r, c := range origCounts {
		if scrambCounts[r] != c {
			t.Errorf("scrambled string missing character %c count: %d vs %d", r, scrambCounts[r], c)
		}
	}
}

func TestCleanAndMaskDefinition(t *testing.T) {
	word := "banana"
	rawDef := "A long curved fruit which grows in clusters on a banana tree."
	pos := "noun"

	cleaned := cleanAndMaskDefinition(rawDef, word, pos)
	if strings.Contains(strings.ToLower(cleaned), "banana") {
		t.Errorf("expected target word '%s' to be masked, got: %s", word, cleaned)
	}
	if !strings.Contains(cleaned, "____") {
		t.Errorf("expected ____ mask in definition, got: %s", cleaned)
	}
	if !strings.HasPrefix(cleaned, "[noun] ") {
		t.Errorf("expected [noun] prefix, got: %s", cleaned)
	}
}

func TestFetchWordMeaning_LiveAPI(t *testing.T) {
	words := []string{"cat", "banana", "elephant", "helicopter", "caterpillar", "hippopotamus", "parallelogram", "accomplishment", "congratulations", "characterization"}
	for _, w := range words {
		t0 := time.Now()
		def := FetchWordMeaning(w)
		elapsed := time.Since(t0)
		t.Logf("Word: %-16s | Elapsed: %v | Hint: %s", w, elapsed, def)
		if def == "" {
			t.Errorf("expected non-empty definition for '%s'", w)
		}
	}
}

func TestAllLevelsDefinitionFetching(t *testing.T) {
	for level := 3; level <= 16; level++ {
		words := wordList[level]
		testWord := words[0]
		t0 := time.Now()
		def := FetchWordMeaning(testWord)
		elapsed := time.Since(t0)
		t.Logf("Level %2d: Word=%-16s | Elapsed=%v | Hint=%s", level, testWord, elapsed, def)
		if def == "" {
			t.Errorf("Level %d word '%s' returned empty definition", level, testWord)
		}
	}
}

func TestUnscrambleGame_StartAndGuessFlow(t *testing.T) {
	chatKey := "12345@s.whatsapp.net"
	hostLID := types.NewJID("11111", "lid")
	hostMention := types.NewJID("11111", "s.whatsapp.net")
	chatJID := types.NewJID("12345", "s.whatsapp.net")

	game := CreateUnscrambleGame(chatKey, hostLID, hostMention, "@Host", chatJID, nil)
	defer DeleteUnscrambleGame(chatKey)

	if game.State != UnscrambleStateLobby {
		t.Fatalf("expected lobby state, got %d", game.State)
	}

	if !game.IsHost(hostLID) {
		t.Fatalf("expected hostLID to be recognized as host")
	}

	// Player 2 joins
	p2LID := types.NewJID("22222", "lid")
	p2Mention := types.NewJID("22222", "s.whatsapp.net")
	if !game.AddPlayer(p2LID, p2Mention, "@Player2") {
		t.Fatalf("failed to add Player2 to lobby")
	}
	if len(game.Players) != 2 {
		t.Fatalf("expected 2 players, got %d", len(game.Players))
	}

	// Host starts game
	if !game.StartGame() {
		t.Fatalf("failed to start game")
	}
	if game.State != UnscrambleStateInProgress {
		t.Fatalf("expected InProgress state, got %d", game.State)
	}

	scrambled, hint, timeLimit, currentP := game.StartTurn()
	if scrambled == "" || hint == "" || timeLimit <= 0 || currentP == nil {
		t.Fatalf("invalid turn parameters: scrambled='%s', hint='%s', timeLimit=%d, currentP=%+v", scrambled, hint, timeLimit, currentP)
	}

	// Guess correct
	correct, gameOver, winner, guesser, elapsed := game.ProcessGuess(game.CurrentWord, currentP.LID)
	if !correct || gameOver || winner != nil || guesser == nil || elapsed < 0 {
		t.Fatalf("expected correct guess, got correct=%v, gameOver=%v, winner=%v", correct, gameOver, winner)
	}
	if guesser.CorrectGuesses != 1 || guesser.Score != 30 {
		t.Fatalf("expected score 30 and 1 correct guess, got score=%d, correct=%d", guesser.Score, guesser.CorrectGuesses)
	}
}

func TestDictionaryDB_SQLiteLookup(t *testing.T) {
	db, err := getDictionaryDB()
	if err != nil || db == nil {
		t.Skip("skipping SQLite dictionary test: sqlite driver not configured")
		return
	}

	testWords := []string{"cat", "dog", "apple", "banana", "elephant", "crocodile", "hippopotamus", "parallelogram"}
	for _, w := range testWords {
		t0 := time.Now()
		meaning := fetchFromSQLiteDictionary(w)
		elapsed := time.Since(t0)
		t.Logf("SQLite Word=%-14s | Time=%v | Def=%s", w, elapsed, meaning)
		if meaning == "" {
			t.Errorf("expected definition from SQLite Dictionary.db for '%s'", w)
		}
	}
}

func TestUnscramble_IsHost(t *testing.T) {
	hostLID := types.NewJID("123456", types.DefaultUserServer)
	playerLID := types.NewJID("789012", types.DefaultUserServer)
	chatJID := types.NewJID("group1", types.GroupServer)

	game := CreateUnscrambleGame("group1@g.us", hostLID, hostLID, "@host", chatJID, nil)
	defer DeleteUnscrambleGame("group1@g.us")

	if !game.IsHost(hostLID) {
		t.Errorf("expected host to be recognized as host")
	}
	if game.IsHost(playerLID) {
		t.Errorf("expected non-host player to NOT be recognized as host")
	}
}

func TestWCG_IsHost(t *testing.T) {
	hostLID := types.NewJID("123456", types.DefaultUserServer)
	playerLID := types.NewJID("789012", types.DefaultUserServer)
	chatJID := types.NewJID("group1", types.GroupServer)

	game := CreateWCGGame("group1@g.us", hostLID, hostLID, "@host", chatJID, nil)
	defer DeleteWCGGame("group1@g.us")

	if !game.IsHost(hostLID) {
		t.Errorf("expected host to be recognized as host")
	}
	if game.IsHost(playerLID) {
		t.Errorf("expected non-host player to NOT be recognized as host")
	}
}
