package plugins

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/url"

	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"go.mau.fi/whatsmeow/types"

	"whatsrook/cmd/store"
	cliutils "whatsrook/cmd/utils"
	Logger "whatsrook/logger"
)

func init() {
	Register(&Command{
		Name:        "ttt",
		Alias:       "tictactoe",
		Description: "Play Tic-Tac-Toe against the bot AI or another user",
		Category:    "games",
		IsPublic:    true,
		NoLoader:    true,
		Handler:     handleTicTacToe,
	})

	Register(&Command{
		Name:        "lb",
		Alias:       "leaderboard",
		Description: "Show overall XP & game leaderboard",
		Category:    "games",
		IsPublic:    true,
		Handler:     handleLeaderboard,
	})

	Register(&Command{
		Name:        "unscramble",
		Alias:       "wordunscramble",
		Description: "Unscramble word game with 30s lobby, dynamic time limits, performance ratings & XP",
		Category:    "games",
		IsPublic:    true,
		NoLoader:    true,
		Handler:     handleUnscramble,
	})

	Register(&Command{
		Name:        "wcg",
		Alias:       "wordchain",
		Description: "Word Chain Game – submit valid English words matching the required starting letter",
		Category:    "games",
		IsPublic:    true,
		NoLoader:    true,
		Handler:     handleWCGChain,
	})
}

func IsTTTGameActive(chatJID string) bool {
	return cliutils.IsTTTGameActive(chatJID)
}

func handleTicTacToe(ctx *Context) error {
	ctx.StopAutoLoader()
	cliutils.TTTMu.Lock()
	defer cliutils.TTTMu.Unlock()

	chatKey := ctx.Chat.String()
	game, exists := cliutils.TTTGames[chatKey]

	if len(ctx.Args) == 0 {
		if !exists {
			return ctx.Reply("No Tic-Tac-Toe game active in this chat.\nStart a game against AI:\n.ttt bot\nOr play against a friend:\n.ttt @user")
		}
		return ctx.Reply(renderTTTBoard(game))
	}

	arg0 := strings.ToLower(ctx.Args[0])

	if arg0 == "cancel" || arg0 == "reset" || arg0 == "stop" {
		if !exists {
			return ctx.Reply("No active Tic-Tac-Toe game to cancel.")
		}
		delete(cliutils.TTTGames, chatKey)
		return ctx.Reply("Tic-Tac-Toe game cancelled.")
	}

	if !exists {
		var playerO types.JID
		var oTag string
		isBotGame := false

		// rawSenderLID is used for turn tracking — always LID format matching incoming senders.
		rawSenderLID := ctx.Sender.ToNonAD()
		userMentionJID, username := ctx.ResolveMention(rawSenderLID)
		userTag := "@" + username

		var playerOMention types.JID
		if arg0 == "bot" || arg0 == "ai" || arg0 == "me" || arg0 == "solo" {
			// Use the bot's own JID so WhatsApp renders it as a real interactive mention.
			rawBot := ctx.Client.Store.ID.ToNonAD()
			playerO = rawBot
			playerOMention, _ = ctx.ResolveMention(rawBot)
			oTag, _ = ctx.FormatMention(rawBot)
			isBotGame = true
		} else if len(ctx.Evt.Message.GetExtendedTextMessage().GetContextInfo().GetMentionedJID()) > 0 {
			mentionedRaw := ctx.Evt.Message.GetExtendedTextMessage().GetContextInfo().GetMentionedJID()[0]
			parsedJID, err := types.ParseJID(mentionedRaw)
			if err != nil {
				return ctx.Reply("Invalid user mention for opponent.")
			}
			// Store raw LID for turn tracking, resolved phone JID for mentions.
			playerO = parsedJID.ToNonAD()
			playerOMention, _ = ctx.ResolveMention(playerO)
			oTag, _ = ctx.FormatMention(playerO)
		} else {
			return ctx.Reply("To start a Tic-Tac-Toe game, play against AI:\n.ttt bot\nOr tag an opponent:\n.ttt @user")
		}

		botStarts := isBotGame && cliutils.GameRng.Intn(2) == 0
		firstTurn := rawSenderLID
		firstTag := userTag
		if botStarts {
			firstTurn = cliutils.BotJID
			firstTag = oTag
		}

		newGame := &cliutils.TTTGame{
			PlayerX:        rawSenderLID,
			PlayerO:        playerO,
			PlayerXMention: userMentionJID,
			PlayerOMention: playerOMention,
			Turn:           firstTurn,
			PlayerXTag:     userTag,
			PlayerOTag:     oTag,
			IsBotGame:      isBotGame,
		}

		botFirstMsg := ""
		if botStarts {
			botMove := bestTTTMove(&newGame.Board)
			if botMove != -1 {
				newGame.Board[botMove] = "O"
			}
			newGame.Turn = rawSenderLID
			botFirstMsg = Sprintf("\n\nAI decided to go first and placed move at position %d!", botMove+1)
		}

		Logger.Debug("[TTT] Creating new game", "chat", chatKey, "rawSenderLID", rawSenderLID.String(), "mentionJID", userMentionJID.String(), "botStarts", botStarts, "firstTurn", firstTurn.String())

		cliutils.TTTGames[chatKey] = newGame

		msg := Sprintf("Tic-Tac-Toe Started!\n\nPlayer X: %s\nPlayer O: %s\n\nTurn: %s (X)%s\n\n%s\n\nMake a move by sending a number 1-9",
			userTag, oTag, firstTag, botFirstMsg, renderTTTGrid(&newGame.Board))

		mentions := []types.JID{userMentionJID, playerOMention}
		return ctx.ReplyWithMentions(msg, mentions)
	}

	pos, err := strconv.Atoi(arg0)
	if err != nil || pos < 1 || pos > 9 {
		return ctx.Reply("Invalid move. Enter a position from 1 to 9, or '.ttt cancel' to reset game.")
	}

	idx := pos - 1
	if game.Board[idx] != "" {
		return ctx.Reply("Position already taken. Choose an empty spot (1-9).")
	}

	// Incoming sender is always LID format; game.PlayerX/Turn are also stored as LID.
	senderLID := ctx.Sender.ToNonAD()
	Logger.Debug("[TTT] Processing move", "chat", chatKey, "senderLID", senderLID.String(), "senderUser", senderLID.User, "gameTurnUser", game.Turn.User, "gameTurnJID", game.Turn.String(), "playerXUser", game.PlayerX.User, "isBotGame", game.IsBotGame)

	if senderLID.User != game.Turn.User {
		Logger.Warn("[TTT] Move rejected: not sender's turn", "senderLID", senderLID.String(), "senderUser", senderLID.User, "expectedTurnUser", game.Turn.User)
		return ctx.Reply("It is not your turn.")
	}

	symbol := "X"
	if senderLID.User == game.PlayerO.User && !game.IsBotGame {
		symbol = "O"
	}

	game.Board[idx] = symbol

	if winner := checkTTTWinner(&game.Board); winner != "" {
		delete(cliutils.TTTGames, chatKey)
		winnerTag := game.PlayerXTag
		if winner == "O" {
			winnerTag = game.PlayerOTag
		}

		if game.IsBotGame {
			if winner == "X" {
				awardTTTXP(ctx, game.PlayerXMention, 50, "win")
			} else {
				awardTTTXP(ctx, game.PlayerXMention, 10, "loss")
			}
		} else {
			awardTTTXP(ctx, game.PlayerXMention, 50, "win")
			awardTTTXP(ctx, game.PlayerOMention, 10, "loss")
		}

		var winnerMentionJID types.JID
		if winner == "O" {
			winnerMentionJID = game.PlayerOMention
		} else {
			winnerMentionJID = game.PlayerXMention
		}
		msg := Sprintf("Game Over!\n\nWinner: %s (%s)\n+50 XP awarded!\n\n%s", winnerTag, winner, renderTTTGrid(&game.Board))
		return ctx.ReplyWithMentions(msg, []types.JID{winnerMentionJID})
	}

	if isTTTFull(&game.Board) {
		delete(cliutils.TTTGames, chatKey)
		awardTTTXP(ctx, game.PlayerXMention, 20, "draw")
		if !game.IsBotGame {
			awardTTTXP(ctx, game.PlayerOMention, 20, "draw")
		}
		msg := Sprintf("Game Over! It's a draw!\n+20 XP awarded to both players!\n\n%s", renderTTTGrid(&game.Board))
		return ctx.Reply(msg)
	}

	if game.IsBotGame {
		botMove := bestTTTMove(&game.Board)
		if botMove != -1 {
			game.Board[botMove] = "O"
		}

		if winner := checkTTTWinner(&game.Board); winner != "" {
			delete(cliutils.TTTGames, chatKey)
			awardTTTXP(ctx, game.PlayerXMention, 10, "loss")
			msg := Sprintf("Game Over!\n\nWinner: %s (O)\nBetter luck next time (+10 XP)!\n\n%s", game.PlayerOTag, renderTTTGrid(&game.Board))
			return ctx.ReplyWithMentions(msg, []types.JID{game.PlayerXMention, game.PlayerOMention})
		}

		if isTTTFull(&game.Board) {
			delete(cliutils.TTTGames, chatKey)
			awardTTTXP(ctx, game.PlayerX, 20, "draw")
			msg := Sprintf("Game Over! It's a draw!\n+20 XP awarded!\n\n%s", renderTTTGrid(&game.Board))
			return ctx.Reply(msg)
		}

		game.Turn = game.PlayerX // PlayerX is the raw LID — correct for turn tracking
		msg := Sprintf("Move placed!\n\nAI played position %d.\nTurn: %s (X)\n\n%s\n\nSend 1-9 to make your next move",
			botMove+1, game.PlayerXTag, renderTTTGrid(&game.Board))
		return ctx.ReplyWithMentions(msg, []types.JID{game.PlayerXMention, game.PlayerOMention})
	}

	nextTurn := game.PlayerO
	nextTag := game.PlayerOTag
	nextMention := game.PlayerOMention
	if senderLID.User == game.PlayerO.User {
		nextTurn = game.PlayerX
		nextTag = game.PlayerXTag
		nextMention = game.PlayerXMention
	}
	game.Turn = nextTurn

	msg := Sprintf("Move placed!\n\nTurn: %s (%s)\n\n%s\n\nSend 1-9 to make your move",
		nextTag, getSymbol(game, nextTurn), renderTTTGrid(&game.Board))
	return ctx.ReplyWithMentions(msg, []types.JID{nextMention})
}

func awardTTTXP(ctx *Context, userJID types.JID, amount int, resultType string) {
	// Scores in DM games are not added to group leaderboards
	if ctx.Chat.Server != "g.us" {
		return
	}

	s, ok := getStore(ctx)
	if !ok {
		return
	}

	winInc, lossInc, drawInc := 0, 0, 0
	switch resultType {
	case "win":
		winInc = 1
	case "loss":
		lossInc = 1
	case "draw":
		drawInc = 1
	}

	groupJID := ctx.Chat.ToNonAD().String()
	normJID := NormalizeUserJID(ctx.Ctx, ctx.Client, userJID)
	cleanJID := normJID.String()

	_ = store.AddGroupUserTTTXP(ctx.Ctx, s.SQLStore, groupJID, cleanJID, amount, winInc, lossInc, drawInc)
}

func handleLeaderboard(ctx *Context) error {
	if ctx.Chat.Server != "g.us" {
		return ctx.Reply("Leaderboards are group-specific! Please use .leaderboard inside a group chat to view that group's leaderboard.")
	}

	s, ok := getStore(ctx)
	if !ok {
		return ctx.Reply("Leaderboard store unavailable.")
	}

	groupJID := ctx.Chat.ToNonAD().String()

	groupName := "Group"
	if info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat); err == nil && info != nil {
		if info.GroupName.Name != "" {
			groupName = info.GroupName.Name
		} else if info.Name != "" {
			groupName = info.Name
		}
	}

	groupLeaderboard, err := store.GetGroupLeaderboard(ctx.Ctx, s.SQLStore, groupJID)
	if err != nil {
		return ctx.Reply("Failed to fetch group leaderboard.")
	}

	type lbEntry struct {
		jid       types.JID
		tag       string
		xp        int
		title     string
		tttWins   int
		tttLosses int
		tttDraws  int
		wcgWins   int
		wcgGames  int
		rating    int
	}

	mergedMap := make(map[string]*lbEntry)
	var mapKeys []string

	for _, row := range groupLeaderboard {
		rating := row.WCGRating
		if rating == 0 {
			rating = 1000
		}
		parsed, pErr := types.ParseJID(row.UserJID)
		if pErr != nil {
			continue
		}
		normJID := NormalizeUserJID(ctx.Ctx, ctx.Client, parsed)
		key := normJID.String()

		existing, found := mergedMap[key]
		if !found {
			tag, resolved := ctx.FormatMention(normJID)
			entry := &lbEntry{
				jid:       resolved,
				tag:       tag,
				xp:        int(row.XP),
				tttWins:   row.TTTWins,
				tttLosses: row.TTTLosses,
				tttDraws:  row.TTTDraws,
				wcgWins:   row.WCGWins,
				wcgGames:  row.WCGGames,
				rating:    rating,
			}
			mergedMap[key] = entry
			mapKeys = append(mapKeys, key)
		} else {
			existing.xp += int(row.XP)
			existing.tttWins += row.TTTWins
			existing.tttLosses += row.TTTLosses
			existing.tttDraws += row.TTTDraws
			existing.wcgWins += row.WCGWins
			existing.wcgGames += row.WCGGames
			if rating > existing.rating {
				existing.rating = rating
			}
		}
	}

	var entries []lbEntry
	for _, k := range mapKeys {
		e := mergedMap[k]
		e.title = cliutils.GetCXPTitle(e.xp)
		entries = append(entries, *e)
	}

	slices.SortFunc(entries, func(a, b lbEntry) int {
		return b.xp - a.xp
	})

	if len(entries) > 10 {
		entries = entries[:10]
	}

	if len(entries) == 0 {
		return ctx.Replyf("%s Leaderboard is currently empty! Play games in this group to earn points and rank up.", groupName)
	}

	var mentions []types.JID
	tb := ctx.Text().Header(groupName + " Leaderboard")

	for i, e := range entries {
		tb.Numberedf(i+1, "%s — %s (%d CXP)\n   Rating: %d | TTT: %dW/%dL/%dD | WCG: %dW/%dG\n",
			e.tag, e.title, e.xp, e.rating, e.tttWins, e.tttLosses, e.tttDraws, e.wcgWins, e.wcgGames)
		mentions = append(mentions, e.jid)
	}

	return ctx.ReplyWithMentions(tb.Trimmed(), mentions)
}

func getSymbol(g *cliutils.TTTGame, p types.JID) string {
	if p.User == g.PlayerX.User {
		return "X"
	}
	return "O"
}

func renderTTTBoard(g *cliutils.TTTGame) string {
	// Turn is stored as LID; compare against PlayerX (also LID) or bot JID.
	turnTag := g.PlayerXTag
	if g.Turn.User == g.PlayerO.User || g.Turn.User == cliutils.BotJID.User {
		turnTag = g.PlayerOTag
	}
	return Sprintf("Tic-Tac-Toe Game\n\nPlayer X: %s\nPlayer O: %s\nTurn: %s\n\n%s",
		g.PlayerXTag, g.PlayerOTag, turnTag, renderTTTGrid(&g.Board))
}

func renderTTTGrid(board *[9]string) string {
	display := make([]string, 9)
	for i := range 9 {
		if board[i] == "" {
			display[i] = strconv.Itoa(i + 1)
		} else {
			display[i] = board[i]
		}
	}
	return Sprintf(
		" %s | %s | %s \n"+
			"---+---+---\n"+
			" %s | %s | %s \n"+
			"---+---+---\n"+
			" %s | %s | %s ",
		display[0], display[1], display[2],
		display[3], display[4], display[5],
		display[6], display[7], display[8],
	)
}

func checkTTTWinner(b *[9]string) string {
	lines := [][3]int{
		{0, 1, 2}, {3, 4, 5}, {6, 7, 8},
		{0, 3, 6}, {1, 4, 7}, {2, 5, 8},
		{0, 4, 8}, {2, 4, 6},
	}
	for _, l := range lines {
		if b[l[0]] != "" && b[l[0]] == b[l[1]] && b[l[1]] == b[l[2]] {
			return b[l[0]]
		}
	}
	return ""
}

func isTTTFull(b *[9]string) bool {
	for _, cell := range b {
		if cell == "" {
			return false
		}
	}
	return true
}

func bestTTTMove(board *[9]string) int {
	bestScore := math.MinInt32
	move := -1

	for i := range 9 {
		if board[i] == "" {
			board[i] = "O"
			score := minimax(board, 0, false)
			board[i] = ""
			if score > bestScore {
				bestScore = score
				move = i
			}
		}
	}
	return move
}

func minimax(board *[9]string, depth int, isMaximizing bool) int {
	winner := checkTTTWinner(board)
	if winner == "O" {
		return 10 - depth
	}
	if winner == "X" {
		return depth - 10
	}
	if isTTTFull(board) {
		return 0
	}

	if isMaximizing {
		bestScore := math.MinInt32
		for i := range 9 {
			if board[i] == "" {
				board[i] = "O"
				score := minimax(board, depth+1, false)
				board[i] = ""
				if score > bestScore {
					bestScore = score
				}
			}
		}
		return bestScore
	} else {
		bestScore := math.MaxInt32
		for i := range 9 {
			if board[i] == "" {
				board[i] = "X"
				score := minimax(board, depth+1, true)
				board[i] = ""
				if score < bestScore {
					bestScore = score
				}
			}
		}
		return bestScore
	}
}

func HandleUnscrambleInput(ctx *Context, text string) bool {
	chatKey := ctx.Chat.String()

	game := cliutils.GetUnscrambleGame(chatKey)
	if game == nil {
		return false
	}

	game.Mu.Lock()

	if game.State == cliutils.UnscrambleStateLobby {
		game.Mu.Unlock()
		return false
	}

	senderLID := ctx.Sender.ToNonAD()

	if isPureEmoji(text) || strings.TrimSpace(text) == "" {
		Logger.Debug("[Unscramble] Ignored emoji/empty input", "chat", chatKey, "sender", senderLID.String())
		game.Mu.Unlock()
		return true
	}

	trimmed := strings.ToLower(strings.TrimSpace(text))
	clean := strings.Trim(trimmed, ".!/#$%^&*()_+-=`~")
	if clean == "cancel" || clean == "stop" || clean == "end" || clean == "kill" || clean == "unscramble end" || clean == "unscramble cancel" {
		game.Mu.Unlock()
		isOwner := ctx.IsOwner()
		isSudo := ctx.IsSudo()
		isHost := game.IsHost(ctx.Sender)
		isBotOwner := isOwner || isSudo

		Logger.Debug("[Unscramble Input] Cancel/End authorization check",
			"chat", chatKey,
			"sender", ctx.Sender.String(),
			"hostLID", game.HostLID.String(),
			"hostMention", game.HostMention.String(),
			"isOwner", isOwner,
			"isSudo", isSudo,
			"isHost", isHost,
			"allowed", isBotOwner || isHost,
		)

		if !isBotOwner && !isHost {
			_ = ctx.Reply("Only the game initiator, a sudo, or the bot owner can end this match.")
			return true
		}
		game.StopTimers()
		cliutils.DeleteUnscrambleGame(chatKey)
		_ = ctx.Reply("Unscramble game cancelled.")
		return true
	}

	pIdx := game.FindPlayerIndex(senderLID)
	if pIdx == -1 {
		Logger.Debug("[Unscramble] Ignored input from non-player", "chat", chatKey, "sender", senderLID.String())
		game.Mu.Unlock()
		return false
	}

	activePlayers := game.GetActivePlayers()
	if len(activePlayers) == 0 {
		game.Mu.Unlock()
		return false
	}

	if pIdx != game.CurrentTurnIdx {
		Logger.Debug("[Unscramble] Ignored input from player whose turn it is not", "chat", chatKey, "sender", senderLID.String())
		game.Mu.Unlock()
		return false
	}

	// Process the guess (release lock first, ProcessGuess needs its own lock)
	game.Mu.Unlock()
	correct, gameOver, _, currentPlayer, elapsed := game.ProcessGuess(text, senderLID)

	if correct {
		_ = ctx.React("✅")
		msg := Sprintf("Correct! %s guessed '%s' in %.1fs! (+%d pts)\n\nAdvancing to the next level!",
			currentPlayer.Tag, game.CurrentWord, elapsed.Seconds(), game.WordLength*10)
		_ = ctx.ReplyWithMentions(msg, []types.JID{currentPlayer.MentionJID})

		if gameOver {
			finishUnscrambleGame(ctx, game)
			return true
		}

		startUnscrambleTurn(ctx, game)
		return true
	}

	_ = ctx.React("❌")
	msg := Sprintf("Incorrect guess by %s!\nThe correct word was: '%s'.\n%s has been eliminated from this match!",
		currentPlayer.Tag, game.CurrentWord, currentPlayer.Tag)
	_ = ctx.ReplyWithMentions(msg, []types.JID{currentPlayer.MentionJID})

	if gameOver {
		finishUnscrambleGame(ctx, game)
		return true
	}

	startUnscrambleTurn(ctx, game)
	return true
}

func handleUnscramble(ctx *Context) error {
	ctx.StopAutoLoader()
	chatKey := ctx.Chat.String()

	existingGame := cliutils.GetUnscrambleGame(chatKey)

	arg0 := ""
	if len(ctx.Args) > 0 {
		arg0 = strings.ToLower(ctx.Args[0])
	}
	if arg0 == "lb" || arg0 == "leaderboard" {
		return handleUnscrambleLeaderboard(ctx)
	}

	if arg0 == "cancel" || arg0 == "stop" || arg0 == "end" || arg0 == "kill" || arg0 == "reset" {
		if existingGame == nil {
			return ctx.Reply("No active Unscramble game to end.")
		}

		isOwner := ctx.IsOwner()
		isSudo := ctx.IsSudo()
		isHost := existingGame.IsHost(ctx.Sender)
		isBotOwner := isOwner || isSudo

		Logger.Debug("[Unscramble] Cancel/End authorization check",
			"chat", chatKey,
			"sender", ctx.Sender.String(),
			"hostLID", existingGame.HostLID.String(),
			"hostMention", existingGame.HostMention.String(),
			"isOwner", isOwner,
			"isSudo", isSudo,
			"isHost", isHost,
			"allowed", isBotOwner || isHost,
		)

		if !isBotOwner && !isHost {
			return ctx.Reply("Only the game initiator, a sudo, or the bot owner can end this match.")
		}

		existingGame.StopTimers()
		cliutils.DeleteUnscrambleGame(chatKey)
		return ctx.Reply("Unscramble game cancelled.")
	}

	if !cliutils.IsDictionaryDBReady() {
		resp, err := ctx.Send("Downloading game resources (Dictionary DB)...")
		var msgID types.MessageID
		if err == nil {
			msgID = resp.ID
		}
		if errDL := cliutils.DownloadDictionaryDB(ctx.Ctx); errDL != nil {
			if msgID != "" {
				_, _ = ctx.Delete(msgID)
			}
			return ctx.Replyf("Failed to download Dictionary DB: %v. Please try again later.", errDL)
		}
		if msgID != "" {
			_, _ = ctx.Delete(msgID)
		}
	}

	if arg0 == "join" {
		if existingGame == nil {
			return ctx.Replyf("No active Unscramble lobby in this chat. Start one with %sunscramble", ctx.GetPrefix())
		}
		existingGame.Mu.Lock()
		if existingGame.State != cliutils.UnscrambleStateLobby {
			existingGame.Mu.Unlock()
			return ctx.Reply("Unscramble game is already in progress!")
		}
		senderLID := ctx.Sender.ToNonAD()
		if existingGame.FindPlayerIndex(senderLID) != -1 {
			existingGame.Mu.Unlock()
			return ctx.Reply("You have already joined the Unscramble match!")
		}
		existingGame.Mu.Unlock()

		mentionJID, username := ctx.ResolveMention(senderLID)
		tag := "@" + username
		if !existingGame.AddPlayer(senderLID, mentionJID, tag) {
			return ctx.Reply("Unscramble match has already started!")
		}

		msg := Sprintf("%s joined the Unscramble match! (%d players in lobby)\nType 'join' to join or wait for the host to start.", tag, len(existingGame.Players))
		return ctx.ReplyWithMentions(msg, []types.JID{mentionJID})
	}

	if arg0 == "start" || arg0 == "create" || arg0 == "begin" {
		if existingGame != nil {
			existingGame.Mu.Lock()
			state := existingGame.State
			playerCount := len(existingGame.Players)
			existingGame.Mu.Unlock()

			if state == cliutils.UnscrambleStateLobby {
				isOwner := ctx.IsOwner()
				isSudo := ctx.IsSudo()
				isHost := existingGame.IsHost(ctx.Sender)
				isBotOwner := isOwner || isSudo

				Logger.Debug("[Unscramble] Start request authorization check",
					"chat", chatKey,
					"sender", ctx.Sender.String(),
					"hostLID", existingGame.HostLID.String(),
					"hostMention", existingGame.HostMention.String(),
					"isOwner", isOwner,
					"isSudo", isSudo,
					"isHost", isHost,
					"allowed", isBotOwner || isHost,
					"playersCount", playerCount,
				)

				if !isBotOwner && !isHost {
					return ctx.Reply("Only the game initiator, a sudo, or the bot owner can start the match!")
				}

				if playerCount == 0 {
					return ctx.Reply("No players in lobby yet! Type `join` to join first.")
				}

				existingGame.StopTimers()
				startUnscrambleGame(ctx, existingGame)
				return nil
			}
			return ctx.Reply("Unscramble game is already in progress!")
		}
		return ctx.Replyf("No active Unscramble lobby in this chat. Start one with %sunscramble", ctx.GetPrefix())
	}

	if existingGame != nil {
		existingGame.Mu.Lock()
		state := existingGame.State
		playerCount := len(existingGame.Players)
		existingGame.Mu.Unlock()

		if state == cliutils.UnscrambleStateLobby {
			return ctx.Replyf("Unscramble Lobby Open! (%d players)\nType `join` to join or .unscramble start to begin!", playerCount)
		}
		return ctx.Reply("An Unscramble game is already in progress in this chat!")
	}

	hostLID := ctx.Sender.ToNonAD()
	hostMention, hostUser := ctx.ResolveMention(hostLID)
	hostTag := "@" + hostUser

	newGame := cliutils.CreateUnscrambleGame(chatKey, hostLID, hostMention, hostTag, ctx.Chat, ctx.Client)

	timer := time.AfterFunc(30*time.Second, func() {
		newGame.Mu.Lock()
		if newGame.State != cliutils.UnscrambleStateLobby {
			newGame.Mu.Unlock()
			return
		}
		newGame.Mu.Unlock()

		cctx := &Context{
			Ctx:    context.Background(),
			Client: ctx.Client,
			Chat:   ctx.Chat,
			Sender: ctx.Sender,
		}
		startUnscrambleGame(cctx, newGame)
	})
	newGame.SetLobbyTimer(timer)

	err := sendUnscrambleInteractiveMenu(ctx, hostTag, hostMention)
	if err != nil {
		p := ctx.GetPrefix()
		textMsg := Sprintf("UNSCRAMBLE GAME\n\nHosted by: %s\n\nLobby is open for 30 SECONDS!\nType '%sunscramble join' to join\nType '%sunscramble start' to begin now\nType '%sunscramble lb' for Leaderboard", hostTag, p, p, p)
		return ctx.ReplyWithMentions(textMsg, []types.JID{hostMention})
	}

	return nil
}

func startUnscrambleGame(ctx *Context, game *cliutils.UnscrambleGame) {
	ctx.StopAutoLoader()
	if !game.StartGame() {
		_ = ctx.Reply("Cannot start game: Need at least 1 player!")
		return
	}

	_ = ctx.Reply("Game Started! Preparing first word...")
	startUnscrambleTurn(ctx, game)
}

func startUnscrambleTurn(ctx *Context, game *cliutils.UnscrambleGame) {
	scrambled, hint, timeLimit, currentPlayer := game.StartTurn()
	if currentPlayer == nil {
		Logger.Error("startUnscrambleTurn: No current player available")
		return
	}

	hintText := hint
	if hintText == "" {
		hintText = "Unscramble the letters to form a valid English word."
	}

	hintMsg := Sprintf("Unscramble the word: *%s* (%d letters)\nHint: %s\n\nTurn for %s (%ds time limit)",
		scrambled, len(game.CurrentWord), hintText, currentPlayer.Tag, timeLimit)
	_ = ctx.ReplyWithMentions(hintMsg, []types.JID{currentPlayer.MentionJID})

	timer := time.AfterFunc(time.Duration(timeLimit)*time.Second, func() {
		game.Mu.Lock()
		defer game.Mu.Unlock()

		inProgress := game.State == cliutils.UnscrambleStateInProgress
		if !inProgress {
			return
		}

		Logger.Info("Unscramble turn timed out for player", "chat", game.ChatKey, "player", currentPlayer.Tag)
		cctx := &Context{
			Ctx:    context.Background(),
			Client: ctx.Client,
			Chat:   game.ChatJID,
		}

		word := game.CurrentWord
		gameOver, _ := game.EliminateCurrentPlayer()

		timeoutMsg := Sprintf("⏰ Time's up for %s!\nThe correct word was: '%s'.\n%s has been eliminated from this match!",
			currentPlayer.Tag, word, currentPlayer.Tag)
		_ = cctx.ReplyWithMentions(timeoutMsg, []types.JID{currentPlayer.MentionJID})

		if gameOver {
			finishUnscrambleGame(cctx, game)
		} else {
			startUnscrambleTurn(cctx, game)
		}
	})

	game.SetTurnTimer(timer)
}

func finishUnscrambleGame(ctx *Context, game *cliutils.UnscrambleGame) {
	winner, standings := game.FinishGame()
	saveUnscrambleStats(ctx, game, winner)

	tb := NewText().Header("UNSCRAMBLE MATCH OVER!")

	var mentions []types.JID

	isMultiplayer := len(game.Players) > 1

	if isMultiplayer {
		if winner != nil {
			tb.Linef("🏆 Winner (Last Standing): %s\nTotal Score: %d pts | Correct Guesses: %d",
				winner.Tag, winner.Score, winner.CorrectGuesses).Blank()
			mentions = append(mentions, winner.MentionJID)
		} else {
			tb.Line("All players have been eliminated! No winner this round.").Blank()
		}
	} else {
		// Single player game
		if winner != nil {
			tb.Linef("🏆 Congratulations %s! You completed all 16 levels with %d points!",
				winner.Tag, winner.Score).Blank()
			mentions = append(mentions, winner.MentionJID)
		} else if len(standings) > 0 {
			tb.Linef("Game Over for %s! Total Score: %d pts (%d correct guesses)",
				standings[0].Tag, standings[0].Score, standings[0].CorrectGuesses).Blank()
			mentions = append(mentions, standings[0].MentionJID)
		} else {
			tb.Line("Game Over!").Blank()
		}
	}

	tb.Section("Final Standings:")
	for idx, p := range standings {
		status := "Eliminated"
		if winner != nil && p.LID.User == winner.LID.User {
			if isMultiplayer {
				status = "Last Standing"
			} else {
				status = "Winner"
			}
		}
		tb.Numberedf(idx+1, "%s — %d pts (%d correct) [%s]", p.Tag, p.Score, p.CorrectGuesses, status)
		if !p.MentionJID.IsEmpty() {
			mentions = append(mentions, p.MentionJID)
		}
	}

	_ = ctx.ReplyWithMentions(tb.Trimmed(), mentions)
}

func saveUnscrambleStats(ctx *Context, game *cliutils.UnscrambleGame, winner *cliutils.UnscramblePlayer) {
	// Scores in DM games are not added to group leaderboards
	if game.ChatJID.Server != "g.us" {
		return
	}

	s, ok := getSQLStore(game.Client)
	if !ok {
		return
	}
	db := s.GetDB()
	if db == nil {
		return
	}

	groupJID := game.ChatJID.ToNonAD().String()

	for _, p := range game.Players {
		if p.LID.User == "" || p.LID.User == "whatsrook_bot" {
			continue
		}

		isWin := winner != nil && p.LID.User == winner.LID.User
		winInc := 0
		xpEarned := p.Score
		if isWin {
			winInc = 1
			xpEarned += 100
		} else {
			xpEarned += 10
		}

		avgTimeMs := int64(0)
		if p.GuessesCount > 0 {
			avgTimeMs = p.TotalTimeMs / int64(p.GuessesCount)
		}

		ratingDelta := -15
		if isWin {
			ratingDelta = 30
		}
		if p.CorrectGuesses > 0 && avgTimeMs > 0 {
			if avgTimeMs < 3000 {
				ratingDelta += 15
			} else if avgTimeMs > 10000 {
				ratingDelta -= 10
			}
		}
		if p.CorrectGuesses == 0 {
			ratingDelta -= 15
			if xpEarned > 20 {
				xpEarned = 5
			}
		}

		normJID := NormalizeUserJID(ctx.Ctx, game.Client, p.MentionJID)
		cleanJID := normJID.String()

		_ = store.AddGroupUserWCGXP(ctx.Ctx, s.SQLStore, groupJID, cleanJID, xpEarned, winInc, 1, ratingDelta)
	}
}

func handleUnscrambleLeaderboard(ctx *Context) error {
	chatKey := ctx.Chat.String()

	game := cliutils.GetUnscrambleGame(chatKey)
	if game == nil {
		return ctx.Replyf("No active Unscramble game in this chat. Start one with %sunscramble", ctx.GetPrefix())
	}

	sorted := game.GetSortedPlayers()
	if len(sorted) == 0 {
		return ctx.Reply("No players in the current Unscramble game.")
	}

	game.Mu.Lock()
	state := game.State
	game.Mu.Unlock()

	tb := ctx.Text()
	if state == cliutils.UnscrambleStateLobby {
		tb.Header("UNSCRAMBLE LOBBY STANDINGS")
	} else {
		tb.Header("UNSCRAMBLE MATCH STANDINGS")
	}

	var mentions []types.JID
	for i, p := range sorted {
		status := ""
		if p.Eliminated {
			status = " (Eliminated)"
		} else if state == cliutils.UnscrambleStateInProgress {
			status = " (Active)"
		}
		tb.Numberedf(i+1, "%s — %d pts (%d correct)%s", p.Tag, p.Score, p.CorrectGuesses, status)
		mentions = append(mentions, p.MentionJID)
	}

	return ctx.ReplyWithMentions(tb.Trimmed(), mentions)
}

func sendUnscrambleInteractiveMenu(ctx *Context, hostTag string, hostMention types.JID) error {
	p := ctx.GetPrefix()
	bodyText := Sprintf("UNSCRAMBLE GAME\n\nHosted by %s\n\n30s Join Window Open!\nType 'join' or '%sunscramble join' to play.\n\nRules:\n- Words progress from 3 to 16 letters\n- Word meanings & clues fetched in real-time from Dictionary API\n- Turn time decreases as difficulty rises (30s -> 6s)\n- Non-players are ignored\n- Win XP and climb performance ratings!", hostTag, p)

	options := []string{
		"Start Match",
		"Leaderboard",
	}

	return sendPollReplyWithMentions(ctx, bodyText, options, []types.JID{hostMention})
}

func isPureEmoji(s string) bool {
	hasRune := false
	for _, r := range strings.TrimSpace(s) {
		if unicode.IsSpace(r) {
			continue
		}
		hasRune = true
		if !isEmojiRune(r) {
			return false
		}
	}
	return hasRune
}

func isEmojiRune(r rune) bool {
	return (r >= 0x1F600 && r <= 0x1F64F) ||
		(r >= 0x1F300 && r <= 0x1F5FF) ||
		(r >= 0x1F680 && r <= 0x1F6FF) ||
		(r >= 0x1F700 && r <= 0x1F77F) ||
		(r >= 0x1F780 && r <= 0x1F7FF) ||
		(r >= 0x1F800 && r <= 0x1F8FF) ||
		(r >= 0x1F900 && r <= 0x1F9FF) ||
		(r >= 0x1FA00 && r <= 0x1FA6F) ||
		(r >= 0x1FA70 && r <= 0x1FAFF) ||
		(r >= 0x2600 && r <= 0x26FF) ||
		(r >= 0x2700 && r <= 0x27BF) ||
		(r >= 0xFE00 && r <= 0xFE0F) ||
		(r >= 0x1F1E6 && r <= 0x1F1FF)
}

func HandleUnscrambleLobbyInput(ctx *Context, text string) bool {
	chatKey := ctx.Chat.String()

	game := cliutils.GetUnscrambleGame(chatKey)
	if game == nil {
		return false
	}

	game.Mu.Lock()
	if game.State != cliutils.UnscrambleStateLobby {
		game.Mu.Unlock()
		return false
	}
	game.Mu.Unlock()

	trimmed := strings.ToLower(strings.TrimSpace(text))
	clean := strings.Trim(trimmed, ".!/#$%^&*()_+-=`~")

	// 1. Check start/begin
	if clean == "start" || clean == "begin" || clean == "unscramble start" || clean == "unscramble begin" {
		isOwner := ctx.IsOwner()
		isSudo := ctx.IsSudo()
		isHost := game.IsHost(ctx.Sender)
		isBotOwner := isOwner || isSudo

		Logger.Debug("[Unscramble Lobby Input] Start request authorization check",
			"chat", chatKey,
			"sender", ctx.Sender.String(),
			"hostLID", game.HostLID.String(),
			"hostMention", game.HostMention.String(),
			"isOwner", isOwner,
			"isSudo", isSudo,
			"isHost", isHost,
			"allowed", isBotOwner || isHost,
			"playersCount", len(game.Players),
		)

		if !isBotOwner && !isHost {
			_ = ctx.Reply("Only the game initiator, a sudo, or the bot owner can start the match!")
			return true
		}
		game.Mu.Lock()
		if len(game.Players) == 0 {
			game.Mu.Unlock()
			_ = ctx.Reply("No players in lobby yet! Type `join` to join first.")
			return true
		}
		game.StopTimers()
		game.Mu.Unlock()
		startUnscrambleGame(ctx, game)
		return true
	}

	// 2. Check cancel/stop/end
	if clean == "cancel" || clean == "stop" || clean == "end" || clean == "kill" || clean == "unscramble end" || clean == "unscramble cancel" {
		isOwner := ctx.IsOwner()
		isSudo := ctx.IsSudo()
		isHost := game.IsHost(ctx.Sender)
		isBotOwner := isOwner || isSudo

		Logger.Debug("[Unscramble Lobby Input] Cancel/End authorization check",
			"chat", chatKey,
			"sender", ctx.Sender.String(),
			"hostLID", game.HostLID.String(),
			"hostMention", game.HostMention.String(),
			"isOwner", isOwner,
			"isSudo", isSudo,
			"isHost", isHost,
			"allowed", isBotOwner || isHost,
		)

		if !isBotOwner && !isHost {
			_ = ctx.Reply("Only the game initiator, a sudo, or the bot owner can end this match.")
			return true
		}
		game.StopTimers()
		cliutils.DeleteUnscrambleGame(chatKey)
		_ = ctx.Reply("Unscramble game cancelled.")
		return true
	}

	// 3. Check join
	isJoinCmd := clean == "join" || clean == "unscramble join" || strings.HasSuffix(clean, "unscramble join")
	if !isJoinCmd {
		return false
	}

	game.Mu.Lock()
	if game.State != cliutils.UnscrambleStateLobby {
		game.Mu.Unlock()
		return false
	}

	senderLID := ctx.Sender.ToNonAD()
	if game.FindPlayerIndex(senderLID) != -1 {
		game.Mu.Unlock()
		mentionJID, username := ctx.ResolveMention(senderLID)
		tag := "@" + username
		_ = ctx.ReplyWithMentions(Sprintf("%s you have already joined the Unscramble match!", tag), []types.JID{mentionJID})
		return true
	}
	game.Mu.Unlock()

	mentionJID, username := ctx.ResolveMention(senderLID)
	tag := "@" + username
	if !game.AddPlayer(senderLID, mentionJID, tag) {
		return true
	}

	msg := Sprintf("%s joined the Unscramble match! (%d players in lobby)\nType 'join' to join or wait for the host to start.", tag, len(game.Players))
	_ = ctx.ReplyWithMentions(msg, []types.JID{mentionJID})
	return true
}

func ValidateWordParallel(word string) bool {
	word = strings.ToLower(strings.TrimSpace(word))
	if len(word) == 0 {
		return false
	}

	if isBuiltinWord(word) {
		return true
	}

	type apiCheck func(word string) bool
	apiChecks := []apiCheck{
		func(w string) bool {
			resp, err := cliutils.GameHTTPClient.Get("https://api.dictionaryapi.dev/api/v2/entries/en/" + url.PathEscape(w))
			if err != nil {
				return false
			}
			defer resp.Body.Close()
			return resp.StatusCode == http.StatusOK
		},
		func(w string) bool {
			resp, err := cliutils.GameHTTPClient.Get("https://api.datamuse.com/words?sp=" + url.PathEscape(w) + "&max=1")
			if err != nil {
				return false
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return false
			}
			var results []struct {
				Word string `json:"word"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&results); err == nil && len(results) > 0 {
				return strings.EqualFold(results[0].Word, w)
			}
			return false
		},
		func(w string) bool {
			resp, err := cliutils.GameHTTPClient.Get("https://api.dictionaryapi.dev/api/v2/entries/en_US/" + url.PathEscape(w))
			if err != nil {
				return false
			}
			defer resp.Body.Close()
			return resp.StatusCode == http.StatusOK
		},
		func(w string) bool {
			reqURL := Sprintf("https://en.wiktionary.org/w/api.php?action=query&titles=%s&format=json", url.QueryEscape(w))
			resp, err := cliutils.GameHTTPClient.Get(reqURL)
			if err != nil {
				return false
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return false
			}
			var res struct {
				Query struct {
					Pages map[string]struct {
						PageID int `json:"pageid"`
					} `json:"pages"`
				} `json:"query"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&res); err == nil {
				for id := range res.Query.Pages {
					if id != "-1" {
						return true
					}
				}
			}
			return false
		},
		func(w string) bool {
			resp, err := cliutils.GameHTTPClient.Get("https://api.dictionaryapi.dev/api/v2/entries/en_GB/" + url.PathEscape(w))
			if err != nil {
				return false
			}
			defer resp.Body.Close()
			return resp.StatusCode == http.StatusOK
		},
	}

	resCh := make(chan bool, len(apiChecks))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, check := range apiChecks {
		fn := check
		go func() {
			select {
			case <-ctx.Done():
				resCh <- false
			default:
				resCh <- fn(word)
			}
		}()
	}

	for range apiChecks {
		if <-resCh {
			// If at least 1 reliable API validates the word, accept it immediately!
			cancel()
			return true
		}
	}

	return false
}

func isBuiltinWord(w string) bool {
	for l := 3; l <= 16; l++ {
		words, ok := cliutils.WCGDictionary[l]
		if !ok {
			continue
		}
		for _, item := range words {
			if strings.EqualFold(item, w) {
				return true
			}
		}
	}
	return false
}

func HandleWCGInput(ctx *Context, text string) bool {
	chatKey := ctx.Chat.String()

	game := cliutils.GetWCGGame(chatKey)
	if game == nil {
		return false
	}

	game.Mu.Lock()

	if game.State == cliutils.WCGStateLobby {
		game.Mu.Unlock()
		return false
	}

	senderLID := ctx.Sender.ToNonAD()

	if isPureEmoji(text) || strings.TrimSpace(text) == "" {
		game.Mu.Unlock()
		return true
	}

	trimmed := strings.ToLower(strings.TrimSpace(text))
	clean := strings.Trim(trimmed, ".!/#$%^&*()_+-=`~")
	if clean == "cancel" || clean == "stop" || clean == "end" || clean == "kill" || clean == "wcg end" || clean == "wcg cancel" {
		game.Mu.Unlock()
		isOwner := ctx.IsOwner()
		isSudo := ctx.IsSudo()
		isHost := game.IsHost(ctx.Sender)
		isBotOwner := isOwner || isSudo

		Logger.Debug("[WCG Input] Cancel/End authorization check",
			"chat", chatKey,
			"sender", ctx.Sender.String(),
			"hostLID", game.HostLID.String(),
			"hostMention", game.HostMention.String(),
			"isOwner", isOwner,
			"isSudo", isSudo,
			"isHost", isHost,
			"allowed", isBotOwner || isHost,
			"gameState", game.State,
		)

		if !isBotOwner && !isHost {
			_ = ctx.Reply("Only the game initiator, a sudo, or the bot owner can end this match.")
			return true
		}
		game.StopTimers()
		cliutils.DeleteWCGGame(chatKey)
		_ = ctx.Reply("Word Chain Game (WCG) ended.")
		return true
	}

	pIdx := game.FindPlayerIndex(senderLID)
	if pIdx == -1 {
		game.Mu.Unlock()
		return false
	}

	activePlayers := game.GetActivePlayers()
	if len(activePlayers) == 0 {
		game.Mu.Unlock()
		return false
	}

	currentTurnPlayer := game.Players[game.CurrentTurnIdx]
	if pIdx != game.CurrentTurnIdx {
		game.Mu.Unlock()
		return false
	}

	game.Mu.Unlock()

	guess := strings.ToLower(strings.TrimSpace(text))

	if len(guess) < game.MinLength {
		_ = ctx.React("❌")
		failMsg := Sprintf("Word too short! Must be at least %d characters long (got %d).\n%s has been eliminated!", game.MinLength, len(guess), currentTurnPlayer.Tag)
		_ = ctx.ReplyWithMentions(failMsg, []types.JID{currentTurnPlayer.MentionJID})
		eliminateAndAdvanceWCG(ctx, game)
		return true
	}

	if len(guess) == 0 || unicode.ToUpper(rune(guess[0])) != unicode.ToUpper(game.RequiredChar) {
		_ = ctx.React("❌")
		failMsg := Sprintf("Invalid start letter! Word must start with '%c'.\n%s has been eliminated!", unicode.ToUpper(game.RequiredChar), currentTurnPlayer.Tag)
		_ = ctx.ReplyWithMentions(failMsg, []types.JID{currentTurnPlayer.MentionJID})
		eliminateAndAdvanceWCG(ctx, game)
		return true
	}

	if game.IsWordUsed(guess) {
		_ = ctx.React("❌")
		failMsg := Sprintf("Word '%s' was already used in this match!\n%s has been eliminated!", guess, currentTurnPlayer.Tag)
		_ = ctx.ReplyWithMentions(failMsg, []types.JID{currentTurnPlayer.MentionJID})
		eliminateAndAdvanceWCG(ctx, game)
		return true
	}

	if !ValidateWordParallel(guess) {
		_ = ctx.React("❌")
		failMsg := Sprintf("'%s' is not recognized as a valid English word across dictionary sources!\n%s has been eliminated!", guess, currentTurnPlayer.Tag)
		_ = ctx.ReplyWithMentions(failMsg, []types.JID{currentTurnPlayer.MentionJID})
		eliminateAndAdvanceWCG(ctx, game)
		return true
	}

	correct, gameOver, winner, currentPlayer, elapsed := game.ProcessGuess(guess, senderLID)

	if correct {
		_ = ctx.React("✅")
		game.Mu.Lock()
		nextReqChar := unicode.ToUpper(game.RequiredChar)
		game.Mu.Unlock()
		msg := Sprintf("Correct! %s submitted '%s' (%d letters) in %.1fs! (+%d pts)\n\nNext Required Letter: '%c' | Round %d Min Length: %d",
			currentPlayer.Tag, guess, len(guess), elapsed.Seconds(), len(guess)*10, nextReqChar, game.RoundCount, game.MinLength)
		_ = ctx.ReplyWithMentions(msg, []types.JID{currentPlayer.MentionJID})

		if gameOver {
			finishWCGChainGame(ctx, game, winner)
			return true
		}

		startWCGChainTurn(ctx, game)
		return true
	}

	return true
}

func handleWCGChain(ctx *Context) error {
	ctx.StopAutoLoader()
	chatKey := ctx.Chat.String()

	existingGame := cliutils.GetWCGGame(chatKey)

	arg0 := ""
	if len(ctx.Args) > 0 {
		arg0 = strings.ToLower(ctx.Args[0])
	}

	if arg0 == "lb" || arg0 == "leaderboard" {
		return handleWCGChainLeaderboard(ctx)
	}

	if arg0 == "cancel" || arg0 == "stop" || arg0 == "end" || arg0 == "kill" {
		if existingGame == nil {
			return ctx.Reply("No active WCG game to end.")
		}

		isOwner := ctx.IsOwner()
		isSudo := ctx.IsSudo()
		isHost := existingGame.IsHost(ctx.Sender)
		isBotOwner := isOwner || isSudo

		Logger.Debug("[WCG] Cancel/End authorization check",
			"chat", existingGame.ChatKey,
			"sender", ctx.Sender.String(),
			"hostLID", existingGame.HostLID.String(),
			"hostMention", existingGame.HostMention.String(),
			"isOwner", isOwner,
			"isSudo", isSudo,
			"isHost", isHost,
			"allowed", isBotOwner || isHost,
			"gameState", existingGame.State,
		)

		if !isBotOwner && !isHost {
			return ctx.Reply("Only the game initiator, a sudo, or the bot owner can end this match.")
		}

		if isHost && !isBotOwner {
			if existingGame.State == cliutils.WCGStateInProgress && existingGame.IsPlayerEliminated(ctx.Sender) {
				return ctx.Reply("You cannot end the match because you have been eliminated! Only the bot owner can end an active match after host elimination.")
			}
		}

		existingGame.StopTimers()
		cliutils.DeleteWCGGame(chatKey)
		return ctx.Reply("Word Chain Game (WCG) ended.")
	}

	if arg0 == "join" {
		if existingGame == nil {
			return ctx.Replyf("No active WCG lobby in this chat. Start one with %swcg", ctx.GetPrefix())
		}
		existingGame.Mu.Lock()
		if existingGame.State != cliutils.WCGStateLobby {
			existingGame.Mu.Unlock()
			return ctx.Reply("WCG game is already in progress!")
		}
		senderLID := ctx.Sender.ToNonAD()
		if existingGame.FindPlayerIndex(senderLID) != -1 {
			existingGame.Mu.Unlock()
			return ctx.Reply("You have already joined the WCG match!")
		}
		existingGame.Mu.Unlock()

		mentionJID, username := ctx.ResolveMention(senderLID)
		tag := "@" + username
		if !existingGame.AddPlayer(senderLID, mentionJID, tag) {
			return ctx.Reply("WCG match has already started!")
		}

		msg := Sprintf("%s joined the WCG match! (%d players in lobby)\nType 'join' to join or wait for the host to start.", tag, len(existingGame.Players))
		return ctx.ReplyWithMentions(msg, []types.JID{mentionJID})
	}

	if arg0 == "start" || arg0 == "create" || arg0 == "begin" {
		if existingGame != nil {
			existingGame.Mu.Lock()
			if existingGame.State == cliutils.WCGStateLobby {
				isOwner := ctx.IsOwner()
				isSudo := ctx.IsSudo()
				isHost := existingGame.IsHost(ctx.Sender)
				isBotOwner := isOwner || isSudo

				Logger.Debug("[WCG] Start request authorization check",
					"chat", existingGame.ChatKey,
					"sender", ctx.Sender.String(),
					"hostLID", existingGame.HostLID.String(),
					"hostMention", existingGame.HostMention.String(),
					"isOwner", isOwner,
					"isSudo", isSudo,
					"isHost", isHost,
					"allowed", isBotOwner || isHost,
					"playersCount", len(existingGame.Players),
				)

				if !isBotOwner && !isHost {
					existingGame.Mu.Unlock()
					return ctx.Reply("Only the game initiator, a sudo, or the bot owner can start the match!")
				}

				if len(existingGame.Players) == 0 {
					existingGame.Mu.Unlock()
					return ctx.Reply("No players in lobby yet! Type `join` to join first.")
				}
				if existingGame.LobbyTimer != nil {
					existingGame.LobbyTimer.Stop()
				}
				existingGame.Mu.Unlock()
				startWCGChainGame(ctx, existingGame)
				return nil
			}
			existingGame.Mu.Unlock()
			return ctx.Reply("WCG game is already in progress!")
		}
		return ctx.Replyf("No active WCG lobby in this chat. Start one with %swcg", ctx.GetPrefix())
	}

	if existingGame != nil {
		existingGame.Mu.Lock()
		defer existingGame.Mu.Unlock()
		if existingGame.State == cliutils.WCGStateLobby {
			return ctx.Replyf("WCG Lobby Open! (%d players)\nType `join` to join or .wcg start to begin!", len(existingGame.Players))
		}
		return ctx.Reply("A WCG game is already in progress in this chat!")
	}

	hostLID := ctx.Sender.ToNonAD()
	hostMention, hostUser := ctx.ResolveMention(hostLID)
	hostTag := "@" + hostUser

	newGame := cliutils.CreateWCGGame(chatKey, hostLID, hostMention, hostTag, ctx.Chat, ctx.Client)

	timer := time.AfterFunc(30*time.Second, func() {
		newGame.Mu.Lock()
		if newGame.State != cliutils.WCGStateLobby {
			newGame.Mu.Unlock()
			return
		}
		newGame.Mu.Unlock()

		cctx := &Context{
			Ctx:    context.Background(),
			Client: ctx.Client,
			Chat:   ctx.Chat,
			Sender: ctx.Sender,
		}
		startWCGChainGame(cctx, newGame)
	})
	newGame.SetLobbyTimer(timer)

	err := sendWCGChainInteractiveMenu(ctx, hostTag, hostMention)
	if err != nil {
		textMsg := Sprintf("WORD CHAIN GAME (WCG)\n\nHosted by: %s\n\nLobby is open for 30 SECONDS!\nType '`join`' to join\nType '.wcg start' to begin now\nType '.wcg lb' for Leaderboard", hostTag)
		return ctx.ReplyWithMentions(textMsg, []types.JID{hostMention})
	}

	return nil
}

func startWCGChainGame(ctx *Context, game *cliutils.WCGGame) {
	ctx.StopAutoLoader()
	if !game.StartGame() {
		_ = ctx.Reply("WCG Match cancelled — no players joined the lobby.")
		return
	}

	active := game.GetActivePlayers()
	Logger.Debug("[WCG] Starting Word Chain Game", "chat", game.ChatKey, "playersCount", len(active))

	var playerTags []string
	var mentions []types.JID
	for _, p := range active {
		playerTags = append(playerTags, p.Tag)
		mentions = append(mentions, p.MentionJID)
	}

	startRune := cliutils.GetRandomStartingLetter()
	game.Mu.Lock()
	game.RequiredChar = startRune
	game.MinLength = 3
	game.RoundCount = 1
	game.AnswersInRound = 0
	game.Mu.Unlock()

	msg := Sprintf("Word Chain Game (WCG) Started!\n\nPlayers (%d): %s\n\nStarting Letter: '%c' (Round 1 Min Length: 3)\nWords are validated in real-time across 5 dictionary APIs!",
		len(active), strings.Join(playerTags, ", "), unicode.ToUpper(startRune))
	_ = ctx.ReplyWithMentions(msg, mentions)

	startWCGChainTurn(ctx, game)
}

func startWCGChainTurn(ctx *Context, game *cliutils.WCGGame) {
	reqChar, minLen, timeSec, currentPlayer := game.StartTurn()
	if currentPlayer == nil {
		winner, _ := game.FinishGame()
		finishWCGChainGame(ctx, game, winner)
		return
	}

	msg := Sprintf("TURN: %s\n\nRound %d\nRequired Starting Letter: *%c*\nMinimum Word Length: *%d* characters\nTime Limit: %d seconds!\n\nType a valid English word matching the required letter!",
		currentPlayer.Tag, game.RoundCount, unicode.ToUpper(reqChar), minLen, timeSec)

	options := []string{
		"End Game",
		"Leaderboard",
	}

	err := sendPollReplyWithMentions(ctx, msg, options, []types.JID{currentPlayer.MentionJID})
	if err != nil {
		_ = ctx.ReplyWithMentions(msg, []types.JID{currentPlayer.MentionJID})
	}

	timeDuration := time.Duration(timeSec) * time.Second
	timer := time.AfterFunc(timeDuration, func() {
		game.Mu.Lock()
		inProgress := game.State == cliutils.WCGStateInProgress
		game.Mu.Unlock()

		if !inProgress {
			return
		}

		Logger.Debug("[WCG] Turn timed out", "player", currentPlayer.Tag)

		cctx := &Context{
			Ctx:    context.Background(),
			Client: game.Client,
			Chat:   game.ChatJID,
			Sender: ctx.Sender,
		}

		timeoutMsg := Sprintf("Time's up for %s!\nFailed to submit a valid word starting with '%c'.\n%s has been eliminated!",
			currentPlayer.Tag, unicode.ToUpper(reqChar), currentPlayer.Tag)
		_ = cctx.ReplyWithMentions(timeoutMsg, []types.JID{currentPlayer.MentionJID})

		eliminateAndAdvanceWCG(cctx, game)
	})
	game.SetTurnTimer(timer)
}

func eliminateAndAdvanceWCG(ctx *Context, game *cliutils.WCGGame) {
	game.StopTimers()
	gameOver, winner := game.EliminateCurrentPlayer()

	if gameOver {
		finishWCGChainGame(ctx, game, winner)
		return
	}

	startWCGChainTurn(ctx, game)
}

func finishWCGChainGame(ctx *Context, game *cliutils.WCGGame, winner *cliutils.WCGPlayer) {
	finalWinner, standings := game.FinishGame()
	if finalWinner != nil {
		winner = finalWinner
	}

	saveWCGChainStats(ctx, game, winner)

	tb := NewText().Header("WCG WORD CHAIN MATCH OVER!")

	var mentions []types.JID

	highestScore := 0
	var highestPlayer *cliutils.WCGPlayer
	if len(standings) > 0 {
		highestPlayer = standings[0]
		highestScore = standings[0].Score
	}

	if winner != nil {
		tb.Linef("Winner (Last Standing): %s (+100 Bonus XP!)\nTotal Score: %d pts | Correct Words: %d",
			winner.Tag, winner.Score, winner.CorrectGuesses).Blank()
		mentions = append(mentions, winner.MentionJID)
	} else {
		tb.Line("No winner — all players eliminated!").Blank()
	}

	tb.Section("Final Standings:")
	for i, p := range standings {
		avgTimeSec := 0.0
		if p.GuessesCount > 0 {
			avgTimeSec = float64(p.TotalTimeMs) / float64(p.GuessesCount) / 1000.0
		}
		status := "Eliminated"
		if winner != nil && p.LID.User == winner.LID.User {
			status = "Last Standing"
		}
		tb.Numberedf(i+1, "%s — %d pts (%d correct, avg %.1fs) [%s]", p.Tag, p.Score, p.CorrectGuesses, avgTimeSec, status)
		mentions = append(mentions, p.MentionJID)
	}

	_ = ctx.ReplyWithMentions(tb.Trimmed(), mentions)

	if winner != nil && highestPlayer != nil && winner.LID.User != highestPlayer.LID.User {
		winnerTag, winnerJID := ctx.FormatMention(winner.MentionJID)
		highestTag, highestJID := ctx.FormatMention(highestPlayer.MentionJID)

		promptMsg := Sprintf("Notice for %s:\nYou are the last player standing, but you do not have the highest score (Highest: %s with %d pts vs your %d pts).\nWould you like to continue playing solo to obtain higher points or end this game?",
			winnerTag, highestTag, highestScore, winner.Score)

		options := []string{
			"Continue Solo",
			"End Game",
		}

		_ = sendPollReplyWithMentions(ctx, promptMsg, options, []types.JID{winnerJID, highestJID})
	}
}

func saveWCGChainStats(ctx *Context, game *cliutils.WCGGame, winner *cliutils.WCGPlayer) {
	// Scores in DM games are not added to group leaderboards
	if game.ChatJID.Server != "g.us" {
		return
	}

	s, ok := getSQLStore(game.Client)
	if !ok {
		return
	}
	db := s.GetDB()
	if db == nil {
		return
	}

	groupJID := game.ChatJID.ToNonAD().String()

	for _, p := range game.Players {
		if p.LID.User == "" || p.LID.User == "whatsrook_bot" {
			continue
		}

		isWin := winner != nil && p.LID.User == winner.LID.User
		winInc := 0
		xpEarned := p.Score
		if isWin {
			winInc = 1
			xpEarned += 100
		} else {
			xpEarned += 10
		}

		avgTimeMs := int64(0)
		if p.GuessesCount > 0 {
			avgTimeMs = p.TotalTimeMs / int64(p.GuessesCount)
		}

		ratingDelta := -15
		if isWin {
			ratingDelta = 30
		}
		if p.CorrectGuesses > 0 && avgTimeMs > 0 {
			if avgTimeMs < 3000 {
				ratingDelta += 15
			} else if avgTimeMs > 10000 {
				ratingDelta -= 10
			}
		}
		if p.CorrectGuesses == 0 {
			ratingDelta -= 15
			if xpEarned > 20 {
				xpEarned = 5
			}
		}

		normJID := NormalizeUserJID(ctx.Ctx, game.Client, p.MentionJID)
		cleanJID := normJID.String()

		_ = store.AddGroupUserWCGXP(ctx.Ctx, s.SQLStore, groupJID, cleanJID, xpEarned, winInc, 1, ratingDelta)
	}
}

func handleWCGChainLeaderboard(ctx *Context) error {
	chatKey := ctx.Chat.String()

	game := cliutils.GetWCGGame(chatKey)
	if game == nil {
		return ctx.Replyf("No active WCG game in this chat. Start one with %swcg", ctx.GetPrefix())
	}

	sorted := game.GetSortedPlayers()
	if len(sorted) == 0 {
		return ctx.Reply("No players in the current WCG game.")
	}

	game.Mu.Lock()
	state := game.State
	game.Mu.Unlock()

	tb := ctx.Text()
	if state == cliutils.WCGStateLobby {
		tb.Header("WCG LOBBY STANDINGS")
	} else {
		tb.Header("WCG MATCH STANDINGS")
	}

	var mentions []types.JID
	for i, p := range sorted {
		status := ""
		if p.Eliminated {
			status = " (Eliminated)"
		} else if state == cliutils.WCGStateInProgress {
			status = " (Active)"
		}
		tb.Numberedf(i+1, "%s — %d pts (%d correct)%s", p.Tag, p.Score, p.CorrectGuesses, status)
		mentions = append(mentions, p.MentionJID)
	}

	return ctx.ReplyWithMentions(tb.Trimmed(), mentions)
}

func sendWCGChainInteractiveMenu(ctx *Context, hostTag string, hostMention types.JID) error {
	p := ctx.GetPrefix()
	bodyText := Sprintf("WORD CHAIN GAME (WCG)\n\nHosted by %s\n\n30s Join Window Open!\nType '%swcg join' to play.\n\nRules:\n- Starting letter is picked at random\n- Words must start with required letter and meet length limit\n- Validated in real-time across 5 parallel dictionary APIs\n- Non-players are ignored\n- Win XP and climb performance ratings!", hostTag, p)

	options := []string{
		"Start Match",
		"End Game",
	}

	return sendPollReplyWithMentions(ctx, bodyText, options, []types.JID{hostMention})
}

func HandleWCGLobbyInput(ctx *Context, text string) bool {
	chatKey := ctx.Chat.String()

	game := cliutils.GetWCGGame(chatKey)
	if game == nil {
		return false
	}

	game.Mu.Lock()
	if game.State != cliutils.WCGStateLobby {
		game.Mu.Unlock()
		return false
	}
	game.Mu.Unlock()

	trimmed := strings.ToLower(strings.TrimSpace(text))
	clean := strings.Trim(trimmed, ".!/#$%^&*()_+-=`~")

	// 1. Check start/begin
	if clean == "start" || clean == "begin" || clean == "wcg start" || clean == "wcg begin" {
		isOwner := ctx.IsOwner()
		isSudo := ctx.IsSudo()
		isHost := game.IsHost(ctx.Sender)
		isBotOwner := isOwner || isSudo

		Logger.Debug("[WCG Lobby Input] Start request authorization check",
			"chat", chatKey,
			"sender", ctx.Sender.String(),
			"hostLID", game.HostLID.String(),
			"hostMention", game.HostMention.String(),
			"isOwner", isOwner,
			"isSudo", isSudo,
			"isHost", isHost,
			"allowed", isBotOwner || isHost,
			"playersCount", len(game.Players),
		)

		if !isBotOwner && !isHost {
			_ = ctx.Reply("Only the game initiator, a sudo, or the bot owner can start the match!")
			return true
		}
		game.Mu.Lock()
		if len(game.Players) == 0 {
			game.Mu.Unlock()
			_ = ctx.Reply("No players in lobby yet! Type `join` to join first.")
			return true
		}
		game.StopTimers()
		game.Mu.Unlock()
		startWCGChainGame(ctx, game)
		return true
	}

	// 2. Check cancel/stop/end
	if clean == "cancel" || clean == "stop" || clean == "end" || clean == "kill" || clean == "wcg end" || clean == "wcg cancel" {
		isOwner := ctx.IsOwner()
		isSudo := ctx.IsSudo()
		isHost := game.IsHost(ctx.Sender)
		isBotOwner := isOwner || isSudo

		Logger.Debug("[WCG Lobby Input] Cancel/End authorization check",
			"chat", chatKey,
			"sender", ctx.Sender.String(),
			"hostLID", game.HostLID.String(),
			"hostMention", game.HostMention.String(),
			"isOwner", isOwner,
			"isSudo", isSudo,
			"isHost", isHost,
			"allowed", isBotOwner || isHost,
		)

		if !isBotOwner && !isHost {
			_ = ctx.Reply("Only the game initiator, a sudo, or the bot owner can end this match.")
			return true
		}
		game.StopTimers()
		cliutils.DeleteWCGGame(chatKey)
		_ = ctx.Reply("Word Chain Game (WCG) ended.")
		return true
	}

	// 3. Check join
	isJoinCmd := clean == "join" || clean == "wcg join" || strings.HasSuffix(clean, "wcg join")
	if !isJoinCmd {
		return false
	}

	game.Mu.Lock()
	if game.State != cliutils.WCGStateLobby {
		game.Mu.Unlock()
		return false
	}

	senderLID := ctx.Sender.ToNonAD()
	if game.FindPlayerIndex(senderLID) != -1 {
		game.Mu.Unlock()
		mentionJID, username := ctx.ResolveMention(senderLID)
		tag := "@" + username
		_ = ctx.ReplyWithMentions(Sprintf("%s you have already joined the WCG match!", tag), []types.JID{mentionJID})
		return true
	}
	game.Mu.Unlock()

	mentionJID, username := ctx.ResolveMention(senderLID)
	tag := "@" + username
	if !game.AddPlayer(senderLID, mentionJID, tag) {
		return true
	}

	msg := Sprintf("%s joined the WCG match! (%d players in lobby)\nType 'join' to join or wait for the host to start.", tag, len(game.Players))
	_ = ctx.ReplyWithMentions(msg, []types.JID{mentionJID})
	return true
}
