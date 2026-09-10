package group

import (
	"context"
	cryptoRand "crypto/rand"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"strconv"
	"time"

	"whatsrook"
	"whatsrook/cmd/store"
	"whatsrook/external"
	"whatsrook/logger"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// HandleCaptcha processes incoming group info events for newcomer captcha challenges, timeouts, and cancellations.
func HandleCaptcha(ctx context.Context, cli *whatsmeow.Client, g *events.GroupInfo, startupTime time.Time) {
	if g == nil || cli == nil {
		return
	}

	logger.Debug("handleGroupCaptcha: received group info event",
		"group", g.JID.String(),
		"joins", len(g.Join),
		"leaves", len(g.Leave),
		"promotes", len(g.Promote),
		"timestamp", g.Timestamp,
	)

	// Do not process group events that happened before the bot started (with 2m clock skew tolerance)
	if !g.Timestamp.IsZero() && g.Timestamp.Before(startupTime.Add(-2*time.Minute)) {
		logger.Debug("handleGroupCaptcha: skipping stale group event before startup",
			"group", g.JID.String(),
			"timestamp", g.Timestamp,
		)
		return
	}

	// Cancel pending captcha and delete verification message if participant left or was removed
	if len(g.Leave) > 0 {
		for _, participant := range g.Leave {
			if pending, ok := RemovePendingCaptcha(g.JID, participant); ok && pending != nil {
				logger.Debug("handleGroupCaptcha: cancelled pending captcha for leaving participant", "group", g.JID.String(), "user", participant.String())
				if pending.MsgID != "" {
					_, _ = cli.SendMessage(ctx, g.JID, cli.BuildRevoke(g.JID, types.EmptyJID, pending.MsgID))
				}
			}
		}
	}
	// Cancel pending captcha and delete verification message if participant was promoted to admin
	if len(g.Promote) > 0 {
		for _, participant := range g.Promote {
			if pending, ok := RemovePendingCaptcha(g.JID, participant); ok && pending != nil {
				logger.Debug("handleGroupCaptcha: cancelled pending captcha for promoted admin", "group", g.JID.String(), "user", participant.String())
				if pending.MsgID != "" {
					_, _ = cli.SendMessage(ctx, g.JID, cli.BuildRevoke(g.JID, types.EmptyJID, pending.MsgID))
				}
			}
		}
	}
	if len(g.Join) == 0 {
		logger.Debug("handleGroupCaptcha: no joins present in event", "group", g.JID.String())
		return
	}

	go processGroupCaptchaJoins(cli, g)
}

func processGroupCaptchaJoins(cli *whatsmeow.Client, g *events.GroupInfo) {
	ctx := context.Background()
	if cli == nil {
		return
	}
	s, ok := cli.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return
	}

	chatKey := g.JID.ToNonAD().String()
	status, _ := store.GetSetting(ctx, s, "captcha_status:"+chatKey)
	if status == "" {
		status, _ = store.GetSetting(ctx, s, "captcha_status:"+g.JID.String())
	}
	logger.Debug("processGroupCaptchaJoins: checking captcha status for group", "group", chatKey, "status", status, "joinCount", len(g.Join))
	if status != "on" {
		logger.Debug("processGroupCaptchaJoins: captcha is disabled for group", "group", chatKey)
		return
	}

	info, err := cli.GetGroupInfo(ctx, g.JID)
	if err != nil || info == nil {
		logger.Warn("processGroupCaptchaJoins: failed to retrieve group info", "group", chatKey, "err", err)
		return
	}

	// This plugin should only work if this bot is an admin
	if !whatsrook.IsBotAdminRaw(ctx, cli, info) {
		logger.Warn("handleGroupCaptcha: bot is not an admin in group, skipping captcha verification", "group", chatKey)
		return
	}

	// If group only allows admins to send messages, no need for verification
	if info.IsAnnounce {
		logger.Debug("handleGroupCaptcha: group is announce-only, skipping captcha verification", "group", chatKey)
		return
	}

	groupName := g.JID.String()
	if info.Name != "" {
		groupName = info.Name
	}

	// Timeout setting (default 120s / 2 mins)
	timeoutSec := 120
	if rawTime, _ := store.GetSetting(ctx, s, "captcha_time:"+chatKey); rawTime != "" {
		if t, err := strconv.Atoi(rawTime); err == nil && t >= 10 {
			timeoutSec = t
		}
	} else if rawTime, _ := store.GetSetting(ctx, s, "captcha_time:"+g.JID.String()); rawTime != "" {
		if t, err := strconv.Atoi(rawTime); err == nil && t >= 10 {
			timeoutSec = t
		}
	}
	timeoutDisplay := formatTimeoutStr(timeoutSec)

	for _, participant := range g.Join {
		// Skip if participant is the bot itself
		if whatsrook.IsSameUserRaw(ctx, cli, participant, *cli.Store.ID) {
			logger.Debug("processGroupCaptchaJoins: skipping bot participant", "group", chatKey, "user", participant.String())
			continue
		}

		resolvedJIDs, username := whatsrook.ResolveMentionJIDs(ctx, cli, participant)
		resolvedJID := participant.ToNonAD()
		if len(resolvedJIDs) > 0 {
			resolvedJID = resolvedJIDs[0]
		}

		// Generate random 4-digit code
		codeInt, err := generateVerificationCode()
		if err != nil {
			logger.Error("Captcha number verification failed: %v", err)
		}
		code := whatsrook.Sprintf("%04d", codeInt)

		logger.Debug("processGroupCaptchaJoins: registering pending captcha challenge",
			"group", chatKey,
			"user", participant.String(),
			"username", username,
			"code", code,
			"timeoutSec", timeoutSec,
		)

		// Register pending captcha with timeout kick callback
		partCopy := participant
		resolvedCopy := resolvedJID
		userCopy := username
		RegisterPendingCaptcha(
			g.JID,
			partCopy,
			resolvedCopy,
			userCopy,
			code,
			time.Duration(timeoutSec)*time.Second,
			func() {
				logger.Debug("processGroupCaptchaJoins: captcha timeout triggered for user", "group", g.JID.String(), "user", partCopy.String(), "username", userCopy)
				// Timeout reached, kick user
				currentInfo, gErr := cli.GetGroupInfo(context.Background(), g.JID)
				if gErr != nil || currentInfo == nil {
					logger.Warn("processGroupCaptchaJoins: failed to get group info during timeout kick", "group", g.JID.String(), "err", gErr)
					return
				}
				if !whatsrook.IsBotAdminRaw(context.Background(), cli, currentInfo) {
					logger.Warn("handleGroupCaptcha: bot is no longer admin to kick unverified participant", "group", g.JID.String(), "user", partCopy.String())
					return
				}
				// Don't attempt to kick admins/creators if they didn't verify
				if whatsrook.IsAdminRaw(context.Background(), cli, currentInfo, partCopy) {
					logger.Warn("handleGroupCaptcha: unverified participant is an admin or creator, skipping kick", "group", g.JID.String(), "user", partCopy.String())
					return
				}

				_, kErr := cli.UpdateGroupParticipants(context.Background(), g.JID, []types.JID{partCopy}, whatsmeow.ParticipantChangeRemove)
				if kErr != nil {
					logger.Error("handleGroupCaptcha: failed to kick unverified participant", "user", partCopy.String(), "err", kErr)
					return
				}

				logger.Info("processGroupCaptchaJoins: unverified participant removed from group", "group", g.JID.String(), "user", partCopy.String(), "username", userCopy)

				kickTb := whatsrook.NewText()
				kickTb.Linef("@%s was removed from the group for failing to complete the captcha verification within %s.", userCopy, timeoutDisplay)
				sendGroupEventMessageWithMentions(context.Background(), cli, g.JID, kickTb.Trimmed(), []types.JID{resolvedCopy})
			},
		)

		// Generate 8-second captcha video using external captcha plugin
		logger.Debug("processGroupCaptchaJoins: generating animated captcha video", "group", chatKey, "user", username, "code", code)
		vidBytes, errGen := generateCaptchaVideo(ctx, code)

		var mediaUploaded *whatsmeow.UploadResponse
		if errGen == nil && len(vidBytes) > 0 {
			logger.Debug("processGroupCaptchaJoins: uploading animated captcha video", "group", chatKey, "bytes", len(vidBytes))
			uploaded, errUp := cli.Upload(ctx, vidBytes, whatsmeow.MediaVideo)
			if errUp == nil {
				mediaUploaded = &uploaded
				logger.Debug("processGroupCaptchaJoins: captcha video uploaded successfully", "group", chatKey, "url", uploaded.URL)
			} else {
				logger.Error("handleGroupCaptcha: video upload failed", "err", errUp)
			}
		} else {
			logger.Debug("handleGroupCaptcha: captcha video generation unavailable/failed", "err", errGen)
		}

		tbVid := whatsrook.NewText()
		tbVid.Linef("Welcome @%s! You are required to complete a verification code to join %s.", username, groupName)
		tbVid.Linef("Please watch the video and reply with the 4-digit verification code within %s, otherwise you will be automatically removed.", timeoutDisplay)
		formattedCaption := tbVid.Trimmed()

		// Send video message as gifplayback if generated successfully
		if mediaUploaded != nil {
			mimetype := "video/mp4"
			vidLen := uint64(len(vidBytes))
			vidMsg := &waE2E.Message{
				VideoMessage: &waE2E.VideoMessage{
					URL:           &mediaUploaded.URL,
					DirectPath:    &mediaUploaded.DirectPath,
					MediaKey:      mediaUploaded.MediaKey,
					Mimetype:      &mimetype,
					GifPlayback:   new(bool),
					FileEncSHA256: mediaUploaded.FileEncSHA256,
					FileSHA256:    mediaUploaded.FileSHA256,
					FileLength:    &vidLen,
					Caption:       &formattedCaption,
					ContextInfo: &waE2E.ContextInfo{
						MentionedJID: []string{resolvedJID.String()},
					},
				},
			}
			*vidMsg.VideoMessage.GifPlayback = true
			logger.Debug("processGroupCaptchaJoins: sending video verification challenge", "group", chatKey, "user", username)
			resp, errSend := cli.SendMessage(ctx, g.JID, vidMsg)
			if errSend != nil {
				logger.Error("handleGroupCaptcha: failed to send video verification message", "group", g.JID.String(), "user", partCopy.String(), "err", errSend)
			} else if resp.ID != "" {
				SetPendingCaptchaMsgID(g.JID, partCopy, resp.ID)
				logger.Debug("handleGroupCaptcha: video verification challenge sent", "group", g.JID.String(), "msg_id", resp.ID, "user", partCopy.String())
			}
		} else {
			// Fallback to text verification prompt if video generation/upload fails
			logger.Warn("handleGroupCaptcha: falling back to text verification prompt", "group", g.JID.String(), "user", partCopy.String())
			tbFallback := whatsrook.NewText()
			tbFallback.Header("Verification Required")
			tbFallback.Linef("Welcome @%s! You are required to complete a verification code to join %s.", username, groupName)
			tbFallback.Blank()
			tbFallback.Linef("Your verification code is: *%s*", code)
			tbFallback.Linef("Please reply with the 4-digit code (*%s*) within %s, otherwise you will be automatically removed.", code, timeoutDisplay)
			tbFallback.Mentions(resolvedJID)
			formattedFallback := whatsrook.FormatTextResponseRaw(tbFallback.Trimmed())
			txtMsg := &waE2E.Message{
				ExtendedTextMessage: &waE2E.ExtendedTextMessage{
					Text: &formattedFallback,
					ContextInfo: &waE2E.ContextInfo{
						MentionedJID: []string{resolvedJID.String()},
					},
				},
			}
			resp, errSend := cli.SendMessage(ctx, g.JID, txtMsg)
			if errSend != nil {
				logger.Error("handleGroupCaptcha: failed to send fallback text verification message", "group", g.JID.String(), "user", partCopy.String(), "err", errSend)
			} else if resp.ID != "" {
				SetPendingCaptchaMsgID(g.JID, partCopy, resp.ID)
				logger.Debug("handleGroupCaptcha: fallback text verification challenge sent", "group", g.JID.String(), "msg_id", resp.ID, "user", partCopy.String())
			}
		}
	}
}

// generateCaptchaVideo attempts to generate an animated verification video using the external captcha plugin binary.
func generateCaptchaVideo(ctx context.Context, code string) ([]byte, error) {
	pluginPath, err := external.DefaultDispatcher.PluginPath("captcha")
	if err != nil {
		return nil, fmt.Errorf("captcha plugin resolution: %w", err)
	}
	if _, err := os.Stat(pluginPath); err != nil {
		return nil, fmt.Errorf("captcha plugin binary not found: %w", err)
	}

	tmpFile, err := os.CreateTemp("", "captcha-out-*.mp4")
	if err != nil {
		return nil, fmt.Errorf("create temp video file: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	cmd := exec.CommandContext(ctx, pluginPath, code, tmpPath)
	cmd.Stderr = nil
	cmd.Stdout = nil
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("execute captcha plugin: %w", err)
	}

	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("read captcha video: %w", err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("captcha plugin produced empty output")
	}
	return data, nil
}

func formatTimeoutStr(sec int) string {
	if sec <= 0 {
		return "2 mins"
	}
	if sec%60 == 0 {
		mins := sec / 60
		if mins == 1 {
			return "1 min"
		}
		return whatsrook.Sprintf("%d mins", mins)
	}
	return whatsrook.Sprintf("%d seconds", sec)
}

func generateVerificationCode() (int, error) {
	digits := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}

	// Fisher-Yates shuffle
	for i := len(digits) - 1; i > 0; i-- {
		j, err := cryptoRand.Int(cryptoRand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return 0, err
		}
		digits[i], digits[j.Int64()] = digits[j.Int64()], digits[i]
	}

	code := digits[0]*1000 + digits[1]*100 + digits[2]*10 + digits[3]
	return code, nil
}
