package plugins

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"

	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"
	"whatsrook/logger"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"

	"whatsrook/cmd/updater"
	cliutils "whatsrook/cmd/utils"
	"whatsrook/utils"
)

func init() {
	Register(&Command{
		Name:        "bio",
		Alias:       "setbio",
		Description: "Update the bot's WhatsApp status bio message",
		Category:    "owner",
		IsPublic:    false,
		Handler:     handleBio,
	})
	Register(&Command{
		Name:        "blocklist",
		Alias:       "blocks",
		Description: "Display list of all currently blocked contacts",
		Category:    "owner",
		IsPublic:    false,
		Handler:     handleBlocklist,
	})
	Register(&Command{
		Name:        "pp",
		Alias:       "setpp",
		Description: "Update the bot's WhatsApp profile picture (replying to an image or image upload)",
		Category:    "owner",
		IsPublic:    false,
		Handler:     handleSetBotPP,
	})
	Register(&Command{
		Name:        "sh",
		Alias:       "exec",
		Description: "Execute a shell command with real-time log streaming and stdin input (sudoers only).",
		Category:    "owner",
		IsPublic:    false,
		Handler:     handleSh,
	})
	Register(&Command{
		Name:        "stop",
		Alias:       "kill",
		Description: "Stop/terminate any running interactive shell session in this chat",
		Category:    "owner",
		IsPublic:    false,
		Handler:     handleStopShell,
	})
	Register(&Command{
		Name:        "status",
		Alias:       "poststatus",
		Description: "Post a status update (text or media) to WhatsApp status broadcast",
		Category:    "owner",
		IsPublic:    false,
		Handler:     handleStatus,
	})
	Register(&Command{
		Name:        "setsudo",
		Description: "Add a user to the sudo list (replied user or numbers)",
		Category:    "owner",
		Handler:     handleSetSudo,
	})
	Register(&Command{
		Name:        "delsudo",
		Description: "Remove a user from the sudo list (replied user or numbers)",
		Category:    "owner",
		Handler:     handleDelSudo,
	})
	Register(&Command{
		Name:        "listsudo",
		Description: "List all sudo users",
		Category:    "owner",
		Handler:     handleListSudo,
	})
	Register(&Command{
		Name:        "ban",
		Description: "Block a user from using the bot commands (replied user or numbers)",
		Category:    "owner",
		Handler:     handleBan,
	})
	Register(&Command{
		Name:        "unban",
		Description: "Unblock a user (replied user or numbers)",
		Category:    "owner",
		Handler:     handleUnban,
	})
	Register(&Command{
		Name:        "mode",
		Description: "Toggle bot mode (public/private)",
		Category:    "owner",
		Handler:     handleMode,
	})
	Register(&Command{
		Name:        "update",
		Description: "Check for updates and manage update configuration",
		Category:    "owner",
		IsPublic:    false,
		Handler:     handleUpdateCommand,
	})
	Register(&Command{
		Name:        "upgrade",
		Description: "Upgrade the bot to the latest system binary build",
		Category:    "owner",
		IsPublic:    false,
		Handler:     handleUpgradeCommand,
	})
}

func handleBio(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("Restricted to bot owner and sudoers.")
	}

	if len(ctx.Args) == 0 {
		p := ctx.GetPrefix()
		return ctx.Replyf("Usage: %sbio <new WhatsApp status bio text>\n\nExample: %sbio Available | WhatsRook AI Bot", p, p)
	}

	newBio := ctx.RawArgs
	err := ctx.Client.SetStatusMessage(ctx.Ctx, types.SetStatusInput{Text: &newBio})
	if err != nil {
		return ctx.Replyf("Failed to update status bio: %v", err)
	}

	return ctx.Replyf("Bot status bio successfully updated to:\n\"%s\"", newBio)
}

func handleBlocklist(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("Restricted to sudoers only.")
	}

	bl, err := ctx.Client.GetBlocklist(ctx.Ctx)
	if err != nil || bl == nil {
		return ctx.Replyf("Failed to fetch blocklist: %v", err)
	}

	if len(bl.JIDs) == 0 {
		return ctx.Reply("Your blocklist is currently empty.")
	}

	tb := ctx.Text().Headerf("BLOCKED CONTACTS (%d total)", len(bl.JIDs))

	var mentions []types.JID
	for i, jid := range bl.JIDs {
		bare := jid.ToNonAD()
		mentions = append(mentions, bare)
		tb.Numberedf(i+1, "+%s (@%s)", bare.User, bare.User)
	}

	p := ctx.GetPrefix()
	tb.Blank().Linef("To unblock a contact: %sunblock @user", p)

	return ctx.ReplyWithMentions(tb.String(), mentions)
}

func handleSetBotPP(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("Restricted to bot owner and sudoers.")
	}

	downloadable, _, _ := ExtractMediaFromEvent(ctx.Evt)
	if downloadable == nil {
		return ctx.Replyf("Please upload or reply to an image to set as profile picture. Usage: %spp", ctx.GetPrefix())
	}

	rawBytes, err := ctx.Client.Download(ctx.Ctx, downloadable)
	if err != nil || len(rawBytes) == 0 {
		return ctx.Replyf("Failed to download image: %v", err)
	}

	jpegData, errConv := utils.EnsureJPEG(ctx.Ctx, rawBytes)
	if errConv != nil || len(jpegData) == 0 {
		return ctx.Replyf("Failed to process profile image format: %v", errConv)
	}

	ownJID := types.EmptyJID
	if ctx.Client != nil && ctx.Client.Store != nil && ctx.Client.Store.ID != nil {
		ownJID = ctx.Client.Store.ID.ToNonAD()
	}

	Logger.Info("handleSetBotPP: Setting bot profile picture", "rawBytes", len(rawBytes), "jpegBytes", len(jpegData), "targetJID", ownJID.String())
	picID, errSet := ctx.Client.SetGroupPhoto(ctx.Ctx, ownJID, jpegData)
	if errSet != nil {
		Logger.Error("handleSetBotPP failed", "err", errSet)
		return ctx.Replyf("Failed to update profile picture: %v", errSet)
	}

	return ctx.Replyf("Bot profile picture updated successfully! (Picture ID: %s)", picID)
}

func handleStopShell(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("Restricted to bot owner and sudoers.")
	}

	cliutils.ActiveShellSessionsMu.Lock()
	session, exists := cliutils.ActiveShellSessions[ctx.Chat.String()]
	cliutils.ActiveShellSessionsMu.Unlock()

	if !exists || session == nil {
		return ctx.Reply("No active shell session running in this chat.")
	}

	session.Mu.Lock()
	session.UserTerminated = true
	cancel := session.Cancel
	session.Mu.Unlock()

	if cancel != nil {
		cancel()
	}
	return ctx.Reply("🛑 Shell command terminated.")
}

// HandleShellInput checks if there is an active shell session in the chat and feeds stdin input to it.
func HandleShellInput(ctx *Context, text string) bool {
	if text == "" {
		return false
	}

	cliutils.ActiveShellSessionsMu.Lock()
	session, exists := cliutils.ActiveShellSessions[ctx.Chat.String()]
	cliutils.ActiveShellSessionsMu.Unlock()

	if !exists || session == nil {
		return false
	}

	if !ctx.IsSudo() {
		return false
	}

	trimmed := strings.TrimSpace(text)
	lower := strings.ToLower(trimmed)

	// Check if this is a command to stop the shell session
	if lower == ".stop" || lower == "!stop" || lower == "stop" || lower == "cancel" || lower == "exit" || lower == "^c" || lower == "ctrl+c" {
		session.Mu.Lock()
		session.UserTerminated = true
		cancel := session.Cancel
		session.Mu.Unlock()
		if cancel != nil {
			cancel()
			_ = ctx.Reply("🛑 Shell command terminated.")
			return true
		}
	}

	// If the text starts with a registered command prefix other than stop, don't capture as stdin
	for _, p := range activePrefixes(ctx.Ctx, ctx.Client) {
		if p != "" && strings.HasPrefix(trimmed, p) {
			body := strings.TrimSpace(trimmed[len(p):])
			fields := strings.Fields(body)
			if len(fields) > 0 {
				cmdWord := strings.ToLower(fields[0])
				if _, isCmd := Get(cmdWord); isCmd {
					return false
				}
			}
		}
	}

	// Feed input into stdin
	session.Mu.Lock()
	defer session.Mu.Unlock()

	if session.Stdin != nil {
		_, err := session.Stdin.Write([]byte(text + "\n"))
		if err != nil {
			_ = ctx.Replyf("Failed to write to stdin: %v", err)
			return true
		}
		_ = ctx.React("⌨️")
		session.Buf.WriteString(Sprintf("\n[stdin] %s\n", text))
		select {
		case session.UpdateCh <- struct{}{}:
		default:
		}
		return true
	}

	return false
}

func handleSh(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("Restricted to bot owner and sudoers.")
	}

	commandStr := strings.TrimSpace(ctx.RawArgs)
	if commandStr == "" {
		p := ctx.GetPrefix()
		return ctx.Replyf("Usage: %ssh <command line>\n\nExample:\n%ssh yt-dlp \"https://...\" -t mp4\n%ssh ls -la", p, p, p)
	}

	chatKey := ctx.Chat.String()

	// Cancel any active session in this chat
	cliutils.ActiveShellSessionsMu.Lock()
	if old, exists := cliutils.ActiveShellSessions[chatKey]; exists && old != nil {
		old.Mu.Lock()
		old.UserTerminated = true
		if old.Cancel != nil {
			old.Cancel()
		}
		old.Mu.Unlock()
	}
	cliutils.ActiveShellSessionsMu.Unlock()

	shell := "bash"
	if _, err := exec.LookPath("bash"); err != nil {
		shell = "sh"
	}

	// If stdbuf exists, use it to force unbuffered / line-buffered stdout and stderr
	execCmdStr := commandStr
	if _, err := exec.LookPath("stdbuf"); err == nil {
		execCmdStr = "stdbuf -oL -eL " + commandStr
	}

	execCtx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	cmd := exec.CommandContext(execCtx, shell, "-c", execCmdStr)

	// Set unbuffered terminal environment variables
	cmd.Env = append(os.Environ(),
		"TERM=xterm-256color",
		"PYTHONUNBUFFERED=1",
		"FORCE_COLOR=1",
		"CLICOLOR_FORCE=1",
		"CI=1",
		"HOMEBREW_NO_AUTO_UPDATE=1",
	)

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return ctx.Replyf("Failed to open stdin pipe: %v", err)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return ctx.Replyf("Failed to open stdout pipe: %v", err)
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return ctx.Replyf("Failed to open stderr pipe: %v", err)
	}

	initialMsg := Sprintf("🖥️ *Executing Shell Command...*\n`%s`\n\n```\n(starting process...)\n```\n💡 _Type in chat to send stdin input. Type `.stop` to kill._", commandStr)
	msgID, err := ctx.ReplyWithID(initialMsg)
	if err != nil {
		cancel()
		return errors.New("failed to send initial shell message: " + err.Error())
	}

	session := &cliutils.ShellSession{
		Chat:       ctx.Chat,
		Sender:     ctx.Sender,
		MsgID:      msgID,
		Cmd:        cmd,
		Stdin:      stdinPipe,
		Cancel:     cancel,
		Buf:        new(bytes.Buffer),
		StartTime:  time.Now(),
		CommandStr: commandStr,
		UpdateCh:   make(chan struct{}, 20),
		Done:       make(chan struct{}),
	}

	cliutils.ActiveShellSessionsMu.Lock()
	cliutils.ActiveShellSessions[chatKey] = session
	cliutils.ActiveShellSessionsMu.Unlock()

	if err := cmd.Start(); err != nil {
		cliutils.ActiveShellSessionsMu.Lock()
		delete(cliutils.ActiveShellSessions, chatKey)
		cliutils.ActiveShellSessionsMu.Unlock()
		cancel()
		_, _ = ctx.Edit(msgID, Sprintf("🖥️ *Shell Error:*\n`%s`\n\n```\nFailed to start: %v\n```", commandStr, err))
		return nil
	}

	// Concurrent stdout and stderr readers
	var readWg sync.WaitGroup
	readStream := func(r io.Reader) {
		defer readWg.Done()
		buf := make([]byte, 1024)
		for {
			n, rErr := r.Read(buf)
			if n > 0 {
				session.Mu.Lock()
				session.Buf.Write(buf[:n])
				session.Mu.Unlock()
				select {
				case session.UpdateCh <- struct{}{}:
				default:
				}
			}
			if rErr != nil {
				break
			}
		}
	}

	readWg.Add(2)
	go readStream(stdoutPipe)
	go readStream(stderrPipe)

	// Background streaming editor
	go func() {
		ticker := time.NewTicker(800 * time.Millisecond)
		defer ticker.Stop()

		var lastEditedText string
		var lastEditTime time.Time

		doEdit := func() {
			session.Mu.Lock()
			rawOutput := session.Buf.String()
			session.Mu.Unlock()

			cleaned := cliutils.CleanShellOutput(rawOutput)
			if cleaned == "" {
				cleaned = "(running...)"
			}

			if len(cleaned) > 3500 {
				cleaned = "... (truncated)\n" + cleaned[len(cleaned)-3400:]
			}

			if cleaned != lastEditedText && time.Since(lastEditTime) >= 800*time.Millisecond {
				updateText := Sprintf("🖥️ *Executing Shell Command...*\n`%s`\n\n```\n%s\n```\n💡 _Type in chat to send stdin input. Type `.stop` to kill._", session.CommandStr, cleaned)
				_, _ = ctx.Edit(session.MsgID, updateText)
				lastEditedText = cleaned
				lastEditTime = time.Now()
			}
		}

		for {
			select {
			case <-session.Done:
				return
			case <-session.UpdateCh:
				doEdit()
			case <-ticker.C:
				doEdit()
			}
		}
	}()

	// Background process waiter
	go func() {
		waitErr := cmd.Wait()
		readWg.Wait()
		duration := time.Since(session.StartTime).Round(100 * time.Millisecond)
		close(session.Done)

		cliutils.ActiveShellSessionsMu.Lock()
		delete(cliutils.ActiveShellSessions, chatKey)
		cliutils.ActiveShellSessionsMu.Unlock()

		session.Mu.Lock()
		rawOutput := session.Buf.String()
		userKilled := session.UserTerminated
		session.Mu.Unlock()

		cancel()

		cleaned := cliutils.CleanShellOutput(rawOutput)
		if cleaned == "" {
			cleaned = "(no output)"
		}

		statusStr := Sprintf("Success (exited in %s)", duration)
		if userKilled {
			statusStr = Sprintf("Terminated by user (ran for %s)", duration)
		} else if execCtx.Err() == context.DeadlineExceeded {
			statusStr = Sprintf("Timed out after %s", duration)
		} else if waitErr != nil {
			statusStr = Sprintf("Failed: %v (ran for %s)", waitErr, duration)
		}

		if len(cleaned) > 3500 {
			cleaned = "... (truncated)\n" + cleaned[len(cleaned)-3400:]
		}

		finalMsg := Sprintf("🖥️ *Shell Output*\nCommand: `%s`\nStatus: *%s*\n\n```\n%s\n```", session.CommandStr, statusStr, cleaned)
		_, _ = ctx.Edit(session.MsgID, finalMsg)
	}()

	return nil
}

func handleStatus(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("Only owner/sudo users can post status updates.")
	}

	text := strings.TrimSpace(ctx.RawArgs)

	mediaBytes, mimetype, err := ctx.GetMedia()
	if err == nil && len(mediaBytes) > 0 {
		isImage := strings.HasPrefix(mimetype, "image")
		isVideo := strings.HasPrefix(mimetype, "video") || strings.Contains(mimetype, "gif")

		if !isImage && !isVideo {
			if strings.HasPrefix(mimetype, "audio") {
				return ctx.Reply("Only image and video media can be posted to status broadcast.")
			}
			isImage = true
		}

		if isImage {
			if mimetype == "" {
				mimetype = "image/jpeg"
			}
			uploaded, uErr := ctx.Client.Upload(ctx.Ctx, mediaBytes, whatsmeow.MediaImage)
			if uErr != nil {
				Logger.Error("handleStatus: image upload failed", "err", uErr)
				return ctx.Replyf("Failed to upload status image: %v", uErr)
			}
			msg := &waE2E.Message{
				ImageMessage: &waE2E.ImageMessage{
					URL:           &uploaded.URL,
					DirectPath:    &uploaded.DirectPath,
					MediaKey:      uploaded.MediaKey,
					Mimetype:      &mimetype,
					FileEncSHA256: uploaded.FileEncSHA256,
					FileSHA256:    uploaded.FileSHA256,
					FileLength:    new(uint64),
				},
			}
			*msg.ImageMessage.FileLength = uint64(len(mediaBytes))
			if text != "" {
				msg.ImageMessage.Caption = &text
			}

			_, sendErr := ctx.Client.SendMessage(ctx.Ctx, cliutils.StatusBroadcastJID, msg)
			if sendErr != nil {
				Logger.Error("handleStatus: send image status failed", "err", sendErr)
				return ctx.Replyf("Failed to post image status: %v", sendErr)
			}
			return ctx.Reply("Successfully posted image status update.")
		}

		if isVideo {
			if mimetype == "" {
				mimetype = "video/mp4"
			}
			uploaded, uErr := ctx.Client.Upload(ctx.Ctx, mediaBytes, whatsmeow.MediaVideo)
			if uErr != nil {
				Logger.Error("handleStatus: video upload failed", "err", uErr)
				return ctx.Replyf("Failed to upload status video: %v", uErr)
			}
			msg := &waE2E.Message{
				VideoMessage: &waE2E.VideoMessage{
					URL:           &uploaded.URL,
					DirectPath:    &uploaded.DirectPath,
					MediaKey:      uploaded.MediaKey,
					Mimetype:      &mimetype,
					FileEncSHA256: uploaded.FileEncSHA256,
					FileSHA256:    uploaded.FileSHA256,
					FileLength:    new(uint64),
				},
			}
			*msg.VideoMessage.FileLength = uint64(len(mediaBytes))
			if text != "" {
				msg.VideoMessage.Caption = &text
			}

			_, sendErr := ctx.Client.SendMessage(ctx.Ctx, cliutils.StatusBroadcastJID, msg)
			if sendErr != nil {
				Logger.Error("handleStatus: send video status failed", "err", sendErr)
				return ctx.Replyf("Failed to post video status: %v", sendErr)
			}
			return ctx.Reply("Successfully posted video status update.")
		}
	}

	if text == "" {
		p := ctx.GetPrefix()
		return ctx.Replyf("Usage:\n- %sstatus <text>\n- Reply to image/video with %sstatus [optional caption]", p, p)
	}

	msg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: &text,
		},
	}

	_, sendErr := ctx.Client.SendMessage(ctx.Ctx, cliutils.StatusBroadcastJID, msg)
	if sendErr != nil {
		Logger.Error("handleStatus: send text status failed", "err", sendErr)
		return ctx.Replyf("Failed to post text status: %v", sendErr)
	}
	return ctx.Reply("Successfully posted text status update.")
}

func handleSetSudo(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("You are not authorized to use this command.")
	}

	_, hasQuoted := ctx.GetQuotedSender()
	if (len(ctx.Args) == 0 && !hasQuoted && len(ctx.GetMentionedJIDs()) == 0) || len(ctx.GetTargets()) == 0 {
		p := ctx.GetPrefix()
		return ctx.Replyf("Usage:\n- %ssetsudo @user\n- %ssetsudo 1234567890\n- Reply to a user's message with %ssetsudo", p, p, p)
	}
	targets := ctx.GetTargets()

	s, ok := getStore(ctx)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}

	raw, err := s.GetSetting(ctx.Ctx, "sudoers")
	if err != nil {
		return err
	}

	sudoers := strings.Fields(raw)
	var addedJIDs []types.JID
	var displayNames []string

	for _, target := range targets {
		targetStr := target.ToNonAD().String()
		already := slices.Contains(sudoers, targetStr)
		if !already {
			sudoers = append(sudoers, targetStr)
			resolvedJID, username := ctx.ResolveMention(target)
			addedJIDs = append(addedJIDs, resolvedJID)
			displayNames = append(displayNames, "@"+username)
		}
	}

	if len(addedJIDs) == 0 {
		return ctx.Reply("Target(s) already in the sudo list.")
	}

	if err := s.PutSetting(ctx.Ctx, "sudoers", strings.Join(sudoers, " ")); err != nil {
		Logger.Error("handleSetSudo: PutSetting failed", "err", err, "sudoers", sudoers)
		return ctx.Replyf("Failed to update sudoers list: %v", err)
	}

	return ctx.ReplyWithMentions(Sprintf("Added to sudo: %s", strings.Join(displayNames, ", ")), addedJIDs)
}

func handleDelSudo(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("You are not authorized to use this command.")
	}
	if !ctx.IsOwner() {
		return ctx.Reply("Only the bot owner can remove users from the sudo list.")
	}

	_, hasQuoted := ctx.GetQuotedSender()
	if (len(ctx.Args) == 0 && !hasQuoted && len(ctx.GetMentionedJIDs()) == 0) || len(ctx.GetTargets()) == 0 {
		p := ctx.GetPrefix()
		return ctx.Replyf("Usage:\n- %sdelsudo @user\n- %sdelsudo 1234567890\n- Reply to a user's message with %sdelsudo", p, p, p)
	}
	targets := ctx.GetTargets()
	if slices.ContainsFunc(targets, ctx.IsTargetOwner) {
		return ctx.Reply("⚠️ Cannot remove the bot owner from sudoers.")
	}

	s, ok := getStore(ctx)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}

	raw, err := s.GetSetting(ctx.Ctx, "sudoers")
	if err != nil {
		return err
	}

	sudoers := strings.Fields(raw)
	var removedJIDs []types.JID
	var displayNames []string
	newSudoers := []string{}

	for _, sdr := range sudoers {
		matched := false
		for _, target := range targets {
			if sdr == target.ToNonAD().String() {
				matched = true
				resolvedJID, username := ctx.ResolveMention(target)
				removedJIDs = append(removedJIDs, resolvedJID)
				displayNames = append(displayNames, "@"+username)
				break
			}
		}
		if !matched {
			newSudoers = append(newSudoers, sdr)
		}
	}

	if len(removedJIDs) == 0 {
		return ctx.Reply("Target(s) not found in the sudo list.")
	}

	if err := s.PutSetting(ctx.Ctx, "sudoers", strings.Join(newSudoers, " ")); err != nil {
		Logger.Error("handleDelSudo: PutSetting failed", "err", err, "newSudoers", newSudoers)
		return ctx.Replyf("Failed to update sudoers list: %v", err)
	}

	return ctx.ReplyWithMentions(Sprintf("Removed from sudo: %s", strings.Join(displayNames, ", ")), removedJIDs)
}

func handleListSudo(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("You are not authorized to use this command.")
	}

	s, ok := getStore(ctx)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}

	raw, err := s.GetSetting(ctx.Ctx, "sudoers")
	if err != nil {
		return err
	}

	sudoers := strings.Fields(raw)
	var mentions []types.JID
	tb := ctx.Text().Header("Sudo List")

	if ctx.Client.Store.ID != nil {
		ownerJID := ctx.Client.Store.ID.ToNonAD()
		resolvedJID, username := ctx.ResolveMention(ownerJID)
		tb.Bulletf("@%s (Owner)", username)
		mentions = append(mentions, resolvedJID)
	}

	for _, sdr := range sudoers {
		sudoerJID, err := types.ParseJID(sdr)
		if err == nil {
			sudoerJID = sudoerJID.ToNonAD()
			if ctx.Client.Store.ID != nil && ctx.IsSameUser(sudoerJID, *ctx.Client.Store.ID) {
				continue
			}
			resolvedJID, username := ctx.ResolveMention(sudoerJID)
			tb.Bulletf("@%s", username)
			mentions = append(mentions, resolvedJID)
		}
	}

	return ctx.ReplyWithMentions(tb.String(), mentions)
}

func handleBan(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("You are not authorized to use this command.")
	}

	targets := ctx.GetTargets()
	if len(targets) == 0 {
		p := ctx.GetPrefix()
		return ctx.Replyf("Usage:\n- %sban @user\n- %sban 1234567890\n- Reply to a user's message with %sban", p, p, p)
	}

	s, ok := getStore(ctx)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}

	rawSudo, _ := s.GetSetting(ctx.Ctx, "sudoers")
	sudoers := strings.Fields(rawSudo)

	rawBanned, err := s.GetSetting(ctx.Ctx, "banned_users")
	if err != nil {
		return err
	}
	bannedUsers := strings.Fields(rawBanned)

	var bannedJIDs []types.JID
	var displayNames []string

	for _, target := range targets {
		targetStr := target.ToNonAD().String()

		if ctx.Client.Store.ID != nil {
			if ctx.IsSameUser(target, *ctx.Client.Store.ID) {
				continue
			}
		}

		isSudo := false
		for _, sdr := range sudoers {
			sj, err := types.ParseJID(sdr)
			if err == nil && ctx.IsSameUser(target, sj) {
				isSudo = true
				break
			}
		}
		if isSudo {
			continue
		}

		already := slices.Contains(bannedUsers, targetStr)

		if !already {
			bannedUsers = append(bannedUsers, targetStr)
			resolvedJID, username := ctx.ResolveMention(target)
			bannedJIDs = append(bannedJIDs, resolvedJID)
			displayNames = append(displayNames, "@"+username)
		}
	}

	if len(bannedJIDs) == 0 {
		return ctx.Reply("Target(s) could not be banned (already banned, owner, or sudo).")
	}

	if err := s.PutSetting(ctx.Ctx, "banned_users", strings.Join(bannedUsers, " ")); err != nil {
		return ctx.Reply("Failed to update banned users list.")
	}

	return ctx.ReplyWithMentions(Sprintf("Banned from commands: %s", strings.Join(displayNames, ", ")), bannedJIDs)
}

func handleUnban(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("You are not authorized to use this command.")
	}

	targets := ctx.GetTargets()
	if len(targets) == 0 {
		p := ctx.GetPrefix()
		return ctx.Replyf("Usage:\n- %sunban @user\n- %sunban 1234567890\n- Reply to a user's message with %sunban", p, p, p)
	}

	s, ok := getStore(ctx)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}

	rawBanned, err := s.GetSetting(ctx.Ctx, "banned_users")
	if err != nil {
		return err
	}
	bannedUsers := strings.Fields(rawBanned)

	var unbannedJIDs []types.JID
	var displayNames []string
	newBanned := []string{}

	for _, b := range bannedUsers {
		matched := false
		for _, target := range targets {
			bj, err := types.ParseJID(b)
			if err == nil && ctx.IsSameUser(target, bj) {
				matched = true
				resolvedJID, username := ctx.ResolveMention(target)
				unbannedJIDs = append(unbannedJIDs, resolvedJID)
				displayNames = append(displayNames, "@"+username)
				break
			}
		}
		if !matched {
			newBanned = append(newBanned, b)
		}
	}

	if len(unbannedJIDs) == 0 {
		return ctx.Reply("Target(s) not found in the banned list.")
	}

	if err := s.PutSetting(ctx.Ctx, "banned_users", strings.Join(newBanned, " ")); err != nil {
		return ctx.Reply("Failed to update banned users list.")
	}

	return ctx.ReplyWithMentions(Sprintf("Unbanned from commands: %s", strings.Join(displayNames, ", ")), unbannedJIDs)
}

func handleMode(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("You are not authorized to use this command.")
	}

	s, ok := getStore(ctx)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}

	p := ctx.GetPrefix()
	if len(ctx.Args) == 0 {
		current, err := s.GetSetting(ctx.Ctx, "mode")
		if err != nil {
			return ctx.Reply("Failed to retrieve bot mode.")
		}
		if current == "" {
			current = "public"
		}
		return ctx.Replyf("Current bot mode: %s\n\nUsage:\n- %smode public\n- %smode private", current, p, p)
	}

	mode := strings.ToLower(ctx.Args[0])
	if mode != "public" && mode != "private" {
		return ctx.Reply("Invalid mode. Usage: mode [public/private]")
	}

	err := s.PutSetting(ctx.Ctx, "mode", mode)
	if err != nil {
		return ctx.Reply("Failed to update bot mode.")
	}

	return ctx.Replyf("Bot mode set to %s.", mode)
}

func handleUpdateCommand(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("You are not authorized to use this command.")
	}

	s, _ := getStore(ctx)
	var sqlS *sqlstore.SQLStore
	if s != nil {
		sqlS = s.SQLStore
	}
	channel := updater.GetChannel(ctx.Ctx, sqlS)

	if len(ctx.Args) == 0 {
		return showUpdateStatus(ctx, channel)
	}

	p := ctx.GetPrefix()
	sub := strings.ToLower(ctx.Args[0])
	switch sub {
	case "check":
		return performCheck(ctx)
	case "stable":
		if s != nil {
			_ = updater.SetChannel(ctx.Ctx, s.SQLStore, "stable")
		}
		return ctx.Replyf("Update channel set to stable. Run %supdate check to verify available releases.", p)
	case "beta":
		if s != nil {
			_ = updater.SetChannel(ctx.Ctx, s.SQLStore, "beta")
		}
		return ctx.Replyf("Update channel set to beta. Run %supdate check to verify available releases.", p)
	case "channel":
		if len(ctx.Args) > 1 {
			ch := strings.ToLower(ctx.Args[1])
			if ch == "stable" || ch == "beta" {
				if s != nil {
					_ = updater.SetChannel(ctx.Ctx, s.SQLStore, ch)
				}
				return ctx.Replyf("Update channel set to %s.", ch)
			}
		}
		return ctx.Replyf("Usage: %supdate channel stable | beta", p)
	case "now", "confirm", "apply":
		return performUpgrade(ctx, channel == "beta")
	default:
		return showUpdateStatus(ctx, channel)
	}
}

func handleUpgradeCommand(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("You are not authorized to use this command.")
	}

	s, _ := getStore(ctx)
	var sqlS *sqlstore.SQLStore
	if s != nil {
		sqlS = s.SQLStore
	}
	channel := updater.GetChannel(ctx.Ctx, sqlS)
	return performUpgrade(ctx, channel == "beta")
}

func showUpdateStatus(ctx *Context, channel string) error {
	currentVer, err := updater.ReadLocalVersion(updater.VersionFile)
	if err != nil {
		currentVer = "unknown"
	}

	platform := updater.GetPlatform()
	p := ctx.GetPrefix()

	msg := Sprintf(
		"WhatsRook Updater Status\nSystem: %s\nCurrent Version: %s\nChannel: %s\n\nSubcommands:\n- %supdate check: Check for new release\n- %supdate stable: Switch to stable channel\n- %supdate beta: Switch to beta channel\n- %supdate now: Apply update and restart",
		platform, currentVer, channel, p, p, p, p,
	)
	return ctx.Reply(msg)
}

func performCheck(ctx *Context) error {
	check, err := updater.CheckUpdate()
	if err != nil {
		Logger.Error("update check failed", "err", err)
		return ctx.Replyf("Update check failed: %v", err)
	}

	p := ctx.GetPrefix()
	if !check.HasNewVersion {
		return ctx.Replyf("WhatsRook is up to date (Version %s, Platform %s).", check.CurrentVersion, check.Platform)
	}

	return ctx.Replyf(
		"Update available!\nCurrent Version: %s\nLatest Version: %s\nPlatform: %s\n\nRun %supdate now or %supgrade to install the new binary release.",
		check.CurrentVersion, check.LatestVersion, check.Platform, p, p,
	)
}

func performUpgrade(ctx *Context, isBeta bool) error {
	res, err := updater.PerformUpdate(isBeta)
	if err != nil {
		Logger.Error("update execution failed", "err", err)
		return ctx.Replyf("Update failed: %v", err)
	}

	_ = ctx.Replyf("%s\nRestarting process now...", res.Message)

	err = updater.RestartProcess()
	Logger.Error("failed to restart process after update", "err", err)
	return ctx.Replyf("Updated binary successfully, but process restart failed: %v", err)
}
