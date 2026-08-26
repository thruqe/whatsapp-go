package plugins

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	clistore "whatsrook/cmd/store"
	cliutils "whatsrook/cmd/utils"
	utils "whatsrook/src"
	Logger "whatsrook/src/logger"
)

func init() {
	Register(&Command{
		Name:        "call",
		Alias:       "phone",
		Description: "Call a number or open the interactive call menu",
		Category:    "calls",
		IsPublic:    true,
		Handler:     handleCall,
	})
	Register(&Command{
		Name:        "callaudio",
		Alias:       "setcallaudio",
		Description: "Call a number with audio, or set your default call audio",
		Category:    "calls",
		IsPublic:    true,
		Handler:     handleCallAudio,
	})
	Register(&Command{
		Name:        "callvideo",
		Alias:       "videocall",
		Description: "Call a number with video, or set your default call video",
		Category:    "calls",
		IsPublic:    true,
		Handler:     handleCallVideo,
	})
	Register(&Command{
		Name:        "anticall",
		Description: "Configure anti-call security to automatically reject incoming WhatsApp calls",
		Category:    "calls",
		IsPublic:    true,
		Handler:     handleAntiCall,
	})
	Register(&Command{
		Name:        "voicemail",
		Description: "Toggle or check automated voicemail answering for incoming calls",
		Category:    "calls",
		IsPublic:    true,
		Handler:     handleVoicemail,
	})
}

var (
	pendingMu sync.Mutex
	pending   = map[types.JID]*cliutils.PendingCall{}
)

func setPending(sender types.JID, p *cliutils.PendingCall) {
	cliutils.SetPendingCall(sender, p)
	pendingMu.Lock()
	defer pendingMu.Unlock()
	pending[sender] = p
}

func peekPending(sender types.JID) (*cliutils.PendingCall, bool) {
	return cliutils.PeekPendingCall(sender)
}

func popPending(sender types.JID) (*cliutils.PendingCall, bool) {
	return cliutils.PopPendingCall(sender)
}

// mediaStore extracts the concrete StoreWrapper from a Context's client
func mediaStore(ctx *Context) (*StoreWrapper, error) {
	s, ok := getStore(ctx)
	if !ok {
		return nil, fmt.Errorf("unexpected store implementation")
	}
	return s, nil
}

func resolveSavedCallAudio(client *whatsmeow.Client, sender types.JID) string {
	if client == nil || client.Store == nil {
		return ""
	}
	s, ok := getSQLStore(client)
	if !ok {
		return ""
	}
	ctx := context.Background()
	candidates := []types.JID{
		sender.ToNonAD(),
		client.Store.GetJID().ToNonAD(),
		client.Store.GetLID().ToNonAD(),
	}
	if client.Store.ID != nil {
		candidates = append(candidates, client.Store.ID.ToNonAD())
	}
	for _, jid := range candidates {
		if jid.IsEmpty() {
			continue
		}
		if path, err := s.GetCallMediaConfig(ctx, jid, clistore.CallMediaAudio); err == nil && path != "" {
			if _, statErr := os.Stat(path); statErr == nil {
				return path
			}
		}
	}
	// Fallback to searching files in session media call-audio folder
	audioDir := GetSessionMediaDir(client, "call-audio")
	if entries, err := os.ReadDir(audioDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && (strings.HasSuffix(entry.Name(), ".mp3") || strings.HasSuffix(entry.Name(), ".ogg") || strings.HasSuffix(entry.Name(), ".wav")) {
				p := filepath.Join(audioDir, entry.Name())
				return p
			}
		}
	}
	return ""
}

func resolveSavedCallVideo(client *whatsmeow.Client, sender types.JID) string {
	if client == nil || client.Store == nil {
		return ""
	}
	s, ok := getSQLStore(client)
	if !ok {
		return ""
	}
	ctx := context.Background()
	candidates := []types.JID{
		sender.ToNonAD(),
		client.Store.GetJID().ToNonAD(),
		client.Store.GetLID().ToNonAD(),
	}
	if client.Store.ID != nil {
		candidates = append(candidates, client.Store.ID.ToNonAD())
	}
	for _, jid := range candidates {
		if jid.IsEmpty() {
			continue
		}
		if path, err := s.GetCallMediaConfig(ctx, jid, clistore.CallMediaVideo); err == nil && path != "" {
			if _, statErr := os.Stat(path); statErr == nil {
				return path
			}
		}
	}
	// Fallback to searching files in session media call-video folder
	videoDir := GetSessionMediaDir(client, "call-video")
	if entries, err := os.ReadDir(videoDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && (strings.HasSuffix(entry.Name(), ".mp4") || strings.HasSuffix(entry.Name(), ".bin") || strings.HasSuffix(entry.Name(), ".3gp")) {
				p := filepath.Join(videoDir, entry.Name())
				return p
			}
		}
	}
	return ""
}

func getSavedAudio(ctx *Context, sender types.JID) (string, bool) {
	path := resolveSavedCallAudio(ctx.Client, sender)
	if path != "" {
		return path, true
	}
	return "", false
}

func saveAudio(ctx *Context, sender types.JID, path string) error {
	s, err := mediaStore(ctx)
	if err != nil {
		return err
	}
	_ = s.PutCallMediaConfig(ctx.Ctx, sender.ToNonAD(), clistore.CallMediaAudio, path)
	if ctx.Client != nil && ctx.Client.Store != nil {
		if jid := ctx.Client.Store.GetJID(); !jid.IsEmpty() {
			_ = s.PutCallMediaConfig(ctx.Ctx, jid.ToNonAD(), clistore.CallMediaAudio, path)
		}
		if lid := ctx.Client.Store.GetLID(); !lid.IsEmpty() {
			_ = s.PutCallMediaConfig(ctx.Ctx, lid.ToNonAD(), clistore.CallMediaAudio, path)
		}
		if ctx.Client.Store.ID != nil {
			_ = s.PutCallMediaConfig(ctx.Ctx, ctx.Client.Store.ID.ToNonAD(), clistore.CallMediaAudio, path)
		}
	}
	return nil
}

func getSavedVideo(ctx *Context, sender types.JID) (string, bool) {
	path := resolveSavedCallVideo(ctx.Client, sender)
	if path != "" {
		return path, true
	}
	return "", false
}

func saveVideo(ctx *Context, sender types.JID, path string) error {
	s, err := mediaStore(ctx)
	if err != nil {
		return err
	}
	_ = s.PutCallMediaConfig(ctx.Ctx, sender.ToNonAD(), clistore.CallMediaVideo, path)
	if ctx.Client != nil && ctx.Client.Store != nil {
		if jid := ctx.Client.Store.GetJID(); !jid.IsEmpty() {
			_ = s.PutCallMediaConfig(ctx.Ctx, jid.ToNonAD(), clistore.CallMediaVideo, path)
		}
		if lid := ctx.Client.Store.GetLID(); !lid.IsEmpty() {
			_ = s.PutCallMediaConfig(ctx.Ctx, lid.ToNonAD(), clistore.CallMediaVideo, path)
		}
		if ctx.Client.Store.ID != nil {
			_ = s.PutCallMediaConfig(ctx.Ctx, ctx.Client.Store.ID.ToNonAD(), clistore.CallMediaVideo, path)
		}
	}
	return nil
}

func handleCall(ctx *Context) error {
	p := ctx.GetPrefix()
	targets := ctx.GetTargets()
	if len(targets) < 1 {
		body := NewText().
			Header("Call Management").
			Section("Select an action below:").
			Bulletf("%scallaudio [number] - Audio call & media", p).
			Bulletf("%scallvideo [number] - Video call & media", p).
			Bulletf("%svoicemail [on/off] - Automated voicemail", p).
			Trimmed()
		buttons := []struct{ ID, Text string }{
			{ID: p + "callaudio", Text: "Call Audio"},
			{ID: p + "callvideo", Text: "Call Video"},
			{ID: p + "voicemail", Text: "Voicemail"},
		}
		return sendInteractiveButtons(ctx, body, Sprintf("%s Calls", ctx.GetBotName()), buttons)
	}

	targetJID := targets[0]
	targetMention := "@" + targetJID.User
	args := strings.Fields(ctx.RawArgs)
	if len(args) > 0 {
		sub := strings.ToLower(args[0])
		if sub == "video" || sub == "v" {
			return handleCallVideo(ctx)
		}
		if sub == "audio" || sub == "a" {
			return handleCallAudio(ctx)
		}
	}

	// Show choice buttons for target
	body := Sprintf("Place Call to %s\n\nSelect call type:", targetMention)
	buttons := []struct{ ID, Text string }{
		{ID: Sprintf("%scallaudio %s", p, targetJID.User), Text: "Audio Call"},
		{ID: Sprintf("%scallvideo %s", p, targetJID.User), Text: "Video Call"},
	}
	return sendInteractiveButtonsWithMentions(ctx, body, Sprintf("%s Calls", ctx.GetBotName()), buttons, []types.JID{targetJID})
}

func handleCallAudio(ctx *Context) error {
	var audioMsg *waE2E.AudioMessage
	if ext := ctx.Evt.Message.GetExtendedTextMessage(); ext != nil {
		if ci := ext.GetContextInfo(); ci != nil && ci.QuotedMessage != nil {
			audioMsg = ci.QuotedMessage.GetAudioMessage()
		}
	}

	if audioMsg != nil {
		return handleSetCallAudio(ctx)
	}

	targets := ctx.GetTargets()
	if len(targets) < 1 {
		p := ctx.GetPrefix()
		if path, ok := getSavedAudio(ctx, ctx.Sender); ok {
			baseName := filepath.Base(path)
			return ctx.Replyf("🎙️ *Default Call Audio Set*: `%s`\n\nUsage:\n• `%scallaudio <number>` to place audio call\n• Reply to new audio with `%scallaudio` to update", baseName, p, p)
		}
		return ctx.Replyf("Usage: `%scallaudio <number>`\n\nTo set your default call audio, reply to any voice note or audio file with `%scallaudio`.", p, p)
	}

	target := targets[0].String()
	_ = ctx.Reply("⚠️ Notice: Outgoing call commands are highly unstable on WhatsApp Web protocol and very unlikely to work reliably.")

	if path, ok := getSavedAudio(ctx, ctx.Sender); ok {
		return placeCallWithAudio(ctx, target, path)
	}

	setPending(ctx.Sender, &cliutils.PendingCall{Target: target, Kind: clistore.CallMediaAudio})
	return ctx.Reply("Reply to an audio file to use for the call.\n" +
		"Reply \"save\" to that audio to make it your default for future calls.")
}

func handleSetCallAudio(ctx *Context) error {
	var audioMsg *waE2E.AudioMessage
	if ext := ctx.Evt.Message.GetExtendedTextMessage(); ext != nil {
		if ci := ext.GetContextInfo(); ci != nil && ci.QuotedMessage != nil {
			audioMsg = ci.QuotedMessage.GetAudioMessage()
		}
	}

	if audioMsg == nil {
		return ctx.Reply("Reply to the audio file you want to set as your default call audio.")
	}

	data, err := ctx.Client.Download(ctx.Ctx, audioMsg)
	if err != nil {
		return ctx.Replyf("Failed to download audio: %v", err)
	}

	targetAudioDir := GetSessionMediaDir(ctx.Client, "call-audio")
	if err := os.MkdirAll(targetAudioDir, 0755); err != nil {
		return ctx.Replyf("Failed to create media directory: %v", err)
	}

	ext := cliutils.ExtensionFor(audioMsg.GetMimetype())
	if ext == "" || ext == ".bin" {
		ext = ".mp3"
	}
	path := filepath.Join(targetAudioDir, utils.SanitizeJID(ctx.Sender.String())+ext)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return ctx.Replyf("Failed to save audio: %v", err)
	}

	path, err = utils.TranscodeToMP3(path)
	if err != nil {
		return ctx.Replyf("Failed to transcode audio: %v", err)
	}

	if err := saveAudio(ctx, ctx.Sender, path); err != nil {
		return ctx.Replyf("Failed to save call audio: %v", err)
	}

	return ctx.Reply("Default call audio set successfully.")
}

func handleCallVideo(ctx *Context) error {
	var videoMsg *waE2E.VideoMessage
	if msg := ctx.Evt.Message.GetVideoMessage(); msg != nil {
		videoMsg = msg
	} else if ext := ctx.Evt.Message.GetExtendedTextMessage(); ext != nil {
		if ci := ext.GetContextInfo(); ci != nil && ci.QuotedMessage != nil {
			videoMsg = ci.QuotedMessage.GetVideoMessage()
		}
	}

	targets := ctx.GetTargets()
	if len(targets) < 1 {
		if videoMsg != nil {
			return handleSetVideoCall(ctx)
		}
		p := ctx.GetPrefix()
		if path, ok := getSavedVideo(ctx, ctx.Sender); ok {
			baseName := filepath.Base(path)
			return ctx.Replyf("📹 *Default Call Video Set*: `%s`\n\nUsage:\n• `%scallvideo <number>` to place video call\n• Reply to new video with `%scallvideo` to update", baseName, p, p)
		}
		return ctx.Replyf("Usage: `%scallvideo <number>`\n\nTo set your default call video, reply to any video with `%scallvideo`.", p, p)
	}

	target := targets[0].String()
	_ = ctx.Reply("⚠️ Notice: Outgoing video call commands are highly unstable on WhatsApp Web protocol and very unlikely to work reliably.")

	if videoMsg != nil {
		data, err := ctx.Client.Download(ctx.Ctx, videoMsg)
		if err == nil && len(data) > 0 {
			targetVideoDir := GetSessionMediaDir(ctx.Client, "call-video")
			_ = os.MkdirAll(targetVideoDir, 0755)
			ext := utils.ExtensionFor(videoMsg.GetMimetype())
			if ext == "" || ext == ".bin" {
				ext = ".mp4"
			}
			path := filepath.Join(targetVideoDir, utils.SanitizeJID(ctx.Sender.String())+ext)
			if err := os.WriteFile(path, data, 0644); err == nil {
				_, _, _ = utils.PrepareCallVideo(path)
				return placeVideoCallWithMedia(ctx, target, path)
			}
		}
	}

	if path, ok := getSavedVideo(ctx, ctx.Sender); ok {
		return placeVideoCallWithMedia(ctx, target, path)
	}

	setPending(ctx.Sender, &cliutils.PendingCall{Target: target, Kind: clistore.CallMediaVideo})
	return ctx.Reply("Reply to a video file to use for the video call.\n" +
		"Reply \"save\" to that video to make it your default for future video calls.")
}

func handleSetVideoCall(ctx *Context) error {
	var videoMsg *waE2E.VideoMessage
	if msg := ctx.Evt.Message.GetVideoMessage(); msg != nil {
		videoMsg = msg
	} else if ext := ctx.Evt.Message.GetExtendedTextMessage(); ext != nil {
		if ci := ext.GetContextInfo(); ci != nil && ci.QuotedMessage != nil {
			videoMsg = ci.QuotedMessage.GetVideoMessage()
		}
	}

	if videoMsg == nil {
		if path, ok := getSavedVideo(ctx, ctx.Sender); ok {
			baseName := filepath.Base(path)
			return ctx.Replyf("You currently have a default video call video set.\n\nFile: %s\n\nTo update it, reply to a new video message with `%ssetvideocall`.", baseName, ctx.GetPrefix())
		}
		return ctx.Replyf("Reply to or attach a video file with `%ssetvideocall` to set your default video for video calls.", ctx.GetPrefix())
	}

	data, err := ctx.Client.Download(ctx.Ctx, videoMsg)
	if err != nil {
		return ctx.Replyf("Failed to download video: %v", err)
	}

	targetVideoDir := GetSessionMediaDir(ctx.Client, "call-video")
	if err := os.MkdirAll(targetVideoDir, 0755); err != nil {
		return ctx.Replyf("Failed to create media directory: %v", err)
	}

	ext := utils.ExtensionFor(videoMsg.GetMimetype())
	if ext == "" || ext == ".bin" {
		ext = ".mp4"
	}
	path := filepath.Join(targetVideoDir, utils.SanitizeJID(ctx.Sender.String())+ext)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return ctx.Replyf("Failed to save video: %v", err)
	}

	_, _, _ = utils.PrepareCallVideo(path)

	if err := saveVideo(ctx, ctx.Sender, path); err != nil {
		return ctx.Replyf("Failed to save video call config: %v", err)
	}

	return ctx.Reply("Default video call video set successfully!")
}

func handleAntiCall(ctx *Context) error {
	s, ok := getStore(ctx)
	if !ok {
		return ctx.Reply("Database store not available.")
	}

	p := ctx.GetPrefix()
	args := strings.Fields(ctx.RawArgs)
	if len(args) == 0 {
		return sendAntiCallMenu(ctx, s)
	}

	sub := strings.ToLower(args[0])
	switch sub {
	case "on", "enable":
		_ = s.PutSetting(ctx.Ctx, "anticall_status", "on")
		return ctx.Reply("AntiCall enabled.")

	case "off", "disable":
		_ = s.PutSetting(ctx.Ctx, "anticall_status", "off")
		return ctx.Reply("AntiCall disabled.")

	case "toggle":
		curr, _ := s.GetSetting(ctx.Ctx, "anticall_status")
		if curr == "on" {
			_ = s.PutSetting(ctx.Ctx, "anticall_status", "off")
			return ctx.Reply("AntiCall disabled.")
		}
		_ = s.PutSetting(ctx.Ctx, "anticall_status", "on")
		return ctx.Reply("AntiCall enabled.")

	case "customize", "custom", "help":
		return sendAntiCallCustomizeGuide(ctx)

	case "contacts":
		if len(args) < 2 {
			curr, _ := s.GetSetting(ctx.Ctx, "anticall_contacts_only")
			return ctx.Reply("AntiCall contacts only setting: " + curr)
		}
		mode := strings.ToLower(args[1])
		switch mode {
		case "on", "true":
			_ = s.PutSetting(ctx.Ctx, "anticall_contacts_only", "true")
			return ctx.Reply("AntiCall set to allow calls from contacts only.")
		case "off", "false":
			_ = s.PutSetting(ctx.Ctx, "anticall_contacts_only", "false")
			return ctx.Reply("AntiCall contacts only restriction disabled.")
		case "toggle":
			curr, _ := s.GetSetting(ctx.Ctx, "anticall_contacts_only")
			if curr == "true" {
				_ = s.PutSetting(ctx.Ctx, "anticall_contacts_only", "false")
				return ctx.Reply("AntiCall contacts only restriction disabled.")
			}
			_ = s.PutSetting(ctx.Ctx, "anticall_contacts_only", "true")
			return ctx.Reply("AntiCall set to allow calls from contacts only.")
		}
		return ctx.Replyf("Usage: %santicall contacts [on|off|toggle]", p)

	case "cc":
		if len(args) < 2 {
			allowed, _ := s.GetSetting(ctx.Ctx, "anticall_allowed_cc")
			if allowed == "" {
				allowed = "none"
			}
			return ctx.Reply("Allowed country codes: " + allowed)
		}
		action := strings.ToLower(args[1])
		switch action {
		case "add":
			if len(args) < 3 {
				return ctx.Replyf("Usage: %santicall cc add <country_code>", p)
			}
			cc := strings.TrimPrefix(args[2], "+")
			allowed, _ := s.GetSetting(ctx.Ctx, "anticall_allowed_cc")
			codes := splitCSV(allowed)
			if !slices.Contains(codes, cc) {
				codes = append(codes, cc)
			}
			_ = s.PutSetting(ctx.Ctx, "anticall_allowed_cc", strings.Join(codes, ","))
			return ctx.Reply("Added country code +" + cc + " to allowed list.")

		case "del", "remove":
			if len(args) < 3 {
				return ctx.Replyf("Usage: %santicall cc del <country_code>", p)
			}
			cc := strings.TrimPrefix(args[2], "+")
			allowed, _ := s.GetSetting(ctx.Ctx, "anticall_allowed_cc")
			codes := splitCSV(allowed)
			newCodes := make([]string, 0, len(codes))
			for _, c := range codes {
				if c != cc {
					newCodes = append(newCodes, c)
				}
			}
			_ = s.PutSetting(ctx.Ctx, "anticall_allowed_cc", strings.Join(newCodes, ","))
			return ctx.Reply("Removed country code +" + cc + " from allowed list.")

		case "clear":
			_ = s.PutSetting(ctx.Ctx, "anticall_allowed_cc", "")
			return ctx.Reply("Cleared allowed country codes list.")

		default:
			return ctx.Replyf("Usage: %santicall cc [add|del|clear]", p)
		}

	case "warn", "warnings":
		if len(args) < 2 {
			curr, _ := s.GetSetting(ctx.Ctx, "anticall_max_warn")
			if curr == "" {
				curr = "3"
			}
			return ctx.Reply("Current call warning threshold: " + curr)
		}
		num, err := strconv.Atoi(args[1])
		if err != nil || num < 1 {
			return ctx.Reply("Please specify a valid warning count number (e.g. 3).")
		}
		_ = s.PutSetting(ctx.Ctx, "anticall_max_warn", strconv.Itoa(num))
		return ctx.Reply("Call warning threshold set to " + strconv.Itoa(num))

	default:
		return ctx.Replyf("Usage: %santicall [on|off|toggle|customize|contacts|cc|warn]", p)
	}
}

func sendAntiCallMenu(ctx *Context, s *StoreWrapper) error {
	status, _ := s.GetSetting(ctx.Ctx, "anticall_status")
	if status == "" {
		status = "off"
	}

	p := ctx.GetPrefix()
	bodyText := NewText().
		Header("ANTICALL CONFIGURATION").
		Field("Status", strings.ToUpper(status)).
		Blank().
		Line("Choose an option below to change status or view customization options.").
		Trimmed()

	var actionButton struct{ ID, Text string }
	if status == "on" {
		actionButton = struct{ ID, Text string }{ID: p + "anticall off", Text: "Deactivate"}
	} else {
		actionButton = struct{ ID, Text string }{ID: p + "anticall on", Text: "Activate"}
	}

	buttons := []struct{ ID, Text string }{
		actionButton,
		{ID: p + "anticall customize", Text: "Customize"},
	}

	return sendInteractiveButtons(ctx, bodyText, Sprintf("%s AntiCall Rejection", ctx.GetBotName()), buttons)
}

func sendAntiCallCustomizeGuide(ctx *Context) error {
	p := ctx.GetPrefix()
	return ctx.Text().
		Header("ANTICALL CUSTOMIZATION GUIDE").
		Section("Available Customizations").
		Bulletf("Contacts Only Restriction : %santicall contacts on | off", p).
		Bulletf("Country Code Whitelist    : %santicall cc add | del | clear <code>", p).
		Bulletf("Max Warning Threshold     : %santicall warn <number>", p).
		Blank().
		Section("Examples").
		Numberedf(1, "%santicall contacts on (Reject calls from non-contacts)", p).
		Numberedf(2, "%santicall cc add 234 (Allow calls from country code +234)", p).
		Numberedf(3, "%santicall warn 3 (Set warning limit before auto-block to 3)", p).
		Reply()
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func handleVoicemail(ctx *Context) error {
	s, ok := getStore(ctx)
	if !ok {
		return ctx.Reply("Database store is not available.")
	}

	if len(ctx.Args) > 0 {
		switch strings.ToLower(ctx.Args[0]) {
		case "on", "enable", "activate":
			_ = s.PutSetting(ctx.Ctx, cliutils.VoicemailSettingKey, "on")
			return ctx.Reply("Automated voicemail activated. Incoming calls will be automatically answered with your default call audio/video media.")
		case "off", "disable", "deactivate":
			_ = s.PutSetting(ctx.Ctx, cliutils.VoicemailSettingKey, "off")
			return ctx.Reply("Automated voicemail deactivated.")
		case "toggle":
			curr, _ := s.GetSetting(ctx.Ctx, cliutils.VoicemailSettingKey)
			if curr == "" {
				curr, _ = s.GetSetting(ctx.Ctx, "autoacceptcall_status")
			}
			if curr == "on" {
				_ = s.PutSetting(ctx.Ctx, cliutils.VoicemailSettingKey, "off")
				return ctx.Reply("Automated voicemail deactivated.")
			}
			_ = s.PutSetting(ctx.Ctx, cliutils.VoicemailSettingKey, "on")
			return ctx.Reply("Automated voicemail activated.")
		default:
			p := ctx.GetPrefix()
			return ctx.Replyf("Usage:\n- %svoicemail on\n- %svoicemail off\n- %svoicemail toggle", p, p, p)
		}
	} else {
		status, _ := s.GetSetting(ctx.Ctx, cliutils.VoicemailSettingKey)
		if status == "" {
			status, _ = s.GetSetting(ctx.Ctx, "autoacceptcall_status")
		}
		if status == "" {
			status = "off"
		}
		p := ctx.GetPrefix()
		audioPath, _ := getSavedAudio(ctx, ctx.Sender)
		videoPath, _ := getSavedVideo(ctx, ctx.Sender)

		audioStatus := "Set"
		if audioPath == "" {
			audioStatus = "Not Set"
		}
		videoStatus := "Set"
		if videoPath == "" {
			videoStatus = "Not Set"
		}

		bodyText := NewText().
			Header("VOICEMAIL CONFIGURATION").
			Field("Status", strings.ToUpper(status)).
			Field("Audio", audioStatus).
			Field("Video", videoStatus).
			Blank().
			Line("Automatically answer incoming calls with your saved call media.").
			Blank().
			Section("Usage").
			Bulletf("%svoicemail on", p).
			Bulletf("%svoicemail off", p).
			Trimmed()

		var actionButton struct{ ID, Text string }
		if status == "on" {
			actionButton = struct{ ID, Text string }{ID: p + "voicemail off", Text: "Deactivate"}
		} else {
			actionButton = struct{ ID, Text string }{ID: p + "voicemail on", Text: "Activate"}
		}

		buttons := []struct{ ID, Text string }{
			actionButton,
			{ID: p + "callaudio", Text: "Call Audio"},
			{ID: p + "callvideo", Text: "Call Video"},
		}

		return sendInteractiveButtons(ctx, bodyText, Sprintf("%s Voicemail", ctx.GetBotName()), buttons)
	}
}

// SetupVoicemail wires the OnIncomingCall handler.
func SetupVoicemail(wa *whatsmeow.Client) {
	if wa == nil {
		Logger.Error("SetupVoicemail: nil client")
		return
	}

	wa.OnIncomingCall(func(call *whatsmeow.Call) {
		handleIncomingCall(call, wa)
	})
}

func handleIncomingCall(call *whatsmeow.Call, waClient *whatsmeow.Client) {
	if call == nil || waClient == nil {
		return
	}

	ctx := context.Background()

	s, ok := getSQLStore(waClient)
	if !ok {
		return
	}

	status, _ := s.GetSetting(ctx, cliutils.VoicemailSettingKey)
	if status == "" {
		status, _ = s.GetSetting(ctx, "autoacceptcall_status")
	}
	if status != "on" {
		Logger.Info("voicemail: incoming call offer ignored because voicemail is not enabled (enable using .voicemail on)", "call_id", call.ID(), "from", call.Peer().String(), "status", status)
		return
	}

	audioPath := resolveSavedCallAudio(waClient, types.EmptyJID)
	videoPath := resolveSavedCallVideo(waClient, types.EmptyJID)

	isVideo := call.IsVideo()
	Logger.Info("voicemail: answering incoming call", "from", call.Peer().String(), "call_id", call.ID(), "is_video", isVideo, "audio", audioPath, "video", videoPath)

	// Set up null receivers BEFORE answering
	call.Receive(whatsmeow.SinkFunc(func(pcm []float32) {}))
	if isVideo {
		call.ReceiveVideo(whatsmeow.VideoSinkFunc(func(accessUnit []byte) {}))
	}

	var mediaOnce sync.Once
	startMedia := func() {
		mediaOnce.Do(func() {
			if isVideo {
				if videoPath != "" {
					startVideoMedia(call, videoPath)
				} else if audioPath != "" {
					startAudioMedia(call, audioPath)
				} else {
					Logger.Warn("voicemail: no video or audio media found for incoming video call")
				}
			} else {
				if audioPath != "" {
					startAudioMedia(call, audioPath)
				} else {
					Logger.Warn("voicemail: no audio media found for incoming voice call")
				}
			}
		})
	}

	// OnReady fires when first inbound RTP packet arrives
	call.OnReady(func() {
		Logger.Info("voicemail: OnReady fired, starting media", "call_id", call.ID())
		startMedia()
	})

	// Let wacaller handle the full signaling — Answer waits for mute_v2 then sends accept
	if err := call.Answer(); err != nil {
		Logger.Error("voicemail: call.Answer() failed", "call_id", call.ID(), "err", err)
		return
	}

	// If OnReady hasn't fired within 10s, something is wrong with the media path.
	// Start anyway — the audio will queue until the relay connects, or fail gracefully.
	go func() {
		time.Sleep(10 * time.Second)
		if call.State() != whatsmeow.CallPhaseEnded {
			Logger.Info("voicemail: OnReady timeout, starting media anyway", "call_id", call.ID())
			startMedia()
		}
	}()
}

func startAudioMedia(call *whatsmeow.Call, audioPath string) {
	Logger.Info("voicemail: starting audio media", "call_id", call.ID(), "path", audioPath)

	src, err := openAudioSource(audioPath)
	if err != nil {
		Logger.Error("voicemail: failed to load audio", "path", audioPath, "err", err)
		_ = call.Hangup()
		return
	}

	call.Play(src)

	duration, err := utils.AudioDuration(audioPath)
	if err != nil || duration == 0 {
		duration = 30 * time.Second
	}

	go func() {
		time.Sleep(duration)
		if call.State() != whatsmeow.CallPhaseEnded {
			Logger.Info("voicemail: audio duration completed, hanging up", "call_id", call.ID())
			_ = call.Hangup()
		}
	}()
}

func startVideoMedia(call *whatsmeow.Call, videoPath string) {
	Logger.Info("voicemail: starting video media", "call_id", call.ID(), "path", videoPath)

	mp3Path, h264Path, err := utils.PrepareCallVideo(videoPath)
	if err != nil {
		Logger.Error("voicemail: failed to prepare video", "err", err)
		_ = call.Hangup()
		return
	}

	if err := call.SetVideoEnabled(true); err != nil {
		Logger.Error("voicemail: SetVideoEnabled failed", "err", err)
	}

	audioFile := mp3Path
	if audioFile == "" {
		audioFile = videoPath
	}
	src, err := openAudioSource(audioFile)
	if err != nil {
		Logger.Error("voicemail: failed to load audio", "path", audioFile, "err", err)
		_ = call.Hangup()
		return
	}
	call.Play(src)

	if h264Path == "" {
		Logger.Warn("voicemail: no h264 track, audio-only for video call", "call_id", call.ID())
		return
	}

	h264Data, err := os.ReadFile(h264Path)
	if err != nil || len(h264Data) == 0 {
		Logger.Error("voicemail: failed to read h264", "path", h264Path, "err", err)
		return
	}

	frames := utils.SplitAnnexBAccessUnits(h264Data)
	if len(frames) == 0 {
		Logger.Error("voicemail: no video frames", "path", h264Path)
		return
	}

	duration, err := utils.AudioDuration(audioFile)
	if err != nil || duration == 0 {
		duration = 30 * time.Second
	}

	go func() {
		frameDur := 66 * time.Millisecond
		ticker := time.NewTicker(frameDur)
		defer ticker.Stop()

		timer := time.NewTimer(duration)
		defer timer.Stop()

		frameIdx := 0
		for {
			select {
			case <-timer.C:
				if call.State() != whatsmeow.CallPhaseEnded {
					Logger.Info("voicemail: video duration completed, hanging up immediately", "call_id", call.ID())
					_ = call.Hangup()
				}
				return
			case <-ticker.C:
				if call.State() == whatsmeow.CallPhaseEnded {
					return
				}
				if frameIdx >= len(frames) {
					if call.State() != whatsmeow.CallPhaseEnded {
						Logger.Info("voicemail: all video frames sent, hanging up immediately", "call_id", call.ID())
						_ = call.Hangup()
					}
					return
				}
				if err := call.SendVideoWithDuration(frames[frameIdx], frameDur); err != nil {
					if !strings.Contains(err.Error(), "has no active video media") {
						Logger.Error("voicemail: SendVideoWithDuration failed", "err", err)
					}
				}
				frameIdx++
				if frameIdx >= len(frames) {
					if call.State() != whatsmeow.CallPhaseEnded {
						Logger.Info("voicemail: reached last frame, hanging up immediately", "call_id", call.ID())
						_ = call.Hangup()
					}
					return
				}
			}
		}
	}()
}

func HandlePendingAudioReply(ctx context.Context, client *whatsmeow.Client, evt *events.Message) bool {
	sender := evt.Info.Sender

	p, ok := peekPending(sender)
	if !ok {
		return false
	}

	if p.Kind == clistore.CallMediaVideo {
		var videoMsg *waE2E.VideoMessage
		saveRequested := false

		if msg := evt.Message.GetVideoMessage(); msg != nil {
			Logger.Debug("Detected direct video message", "sender", sender.String())
			videoMsg = msg
			saveRequested = utils.IsSaveText(utils.GetDirectMessageText(evt.Message))
		} else if extText := evt.Message.GetExtendedTextMessage(); extText != nil && utils.IsSaveText(extText.GetText()) {
			if ctxInfo := extText.GetContextInfo(); ctxInfo != nil && ctxInfo.QuotedMessage != nil {
				if quotedVideo := ctxInfo.QuotedMessage.GetVideoMessage(); quotedVideo != nil {
					videoMsg = quotedVideo
					saveRequested = true
				}
			}
		}

		if videoMsg == nil {
			return false
		}

		popPending(sender)

		go func() {
			cctx := &Context{
				Ctx:    ctx,
				Client: client,
				Evt:    evt,
				Chat:   evt.Info.Chat,
				Sender: sender,
			}
			handleVideoDownload(ctx, client, cctx, sender, evt, videoMsg, p, saveRequested)
		}()

		return true
	}

	var audioMsg *waE2E.AudioMessage
	saveRequested := false

	if msg := evt.Message.GetAudioMessage(); msg != nil {
		Logger.Debug("Detected direct audio message", "sender", sender.String())
		audioMsg = msg
		saveRequested = utils.IsSaveText(utils.GetDirectMessageText(evt.Message))
	} else if extText := evt.Message.GetExtendedTextMessage(); extText != nil && utils.IsSaveText(extText.GetText()) {
		Logger.Debug("Detected text message containing 'save', checking quoted audio...", "sender", sender.String())
		if ctxInfo := extText.GetContextInfo(); ctxInfo != nil && ctxInfo.QuotedMessage != nil {
			if quotedAudio := ctxInfo.QuotedMessage.GetAudioMessage(); quotedAudio != nil {
				Logger.Debug("Found quoted audio message in reply", "sender", sender.String())
				audioMsg = quotedAudio
				saveRequested = true
			}
		}
	}

	if audioMsg == nil {
		Logger.Debug("Message did not provide or quote an audio message, skipping pending intercept", "sender", sender.String())
		return false
	}

	popPending(sender)

	go func() {
		cctx := &Context{
			Ctx:    ctx,
			Client: client,
			Evt:    evt,
			Chat:   evt.Info.Chat,
			Sender: sender,
		}
		handleAudioDownload(ctx, client, cctx, sender, evt, audioMsg, p, saveRequested)
	}()

	return true
}

func handleAudioDownload(ctx context.Context, client *whatsmeow.Client, cctx *Context, sender types.JID, evt *events.Message, audioMsg *waE2E.AudioMessage, p *cliutils.PendingCall, saveRequested bool) {
	Logger.Debug("Downloading audio payload", "sender", sender.String())
	data, err := client.Download(ctx, audioMsg)
	if err != nil {
		Logger.Error("Download audio failed", "err", err)
		if sendErr := sendTextRaw(ctx, client, evt.Info.Chat, Sprintf("failed to download audio: %v", err)); sendErr != nil {
			Logger.Error("failed to notify user", "sendErr", sendErr)
		}
		return
	}

	targetAudioDir := GetSessionMediaDir(client, "call-audio")
	if err := os.MkdirAll(targetAudioDir, 0755); err != nil {
		Logger.Error("Failed creating audio directory", "err", err)
		if sendErr := sendTextRaw(ctx, client, evt.Info.Chat, Sprintf("failed to prepare storage: %v", err)); sendErr != nil {
			Logger.Error("failed to notify user", "sendErr", sendErr)
		}
		return
	}

	ext := utils.ExtensionFor(audioMsg.GetMimetype())
	if ext == "" || ext == ".bin" {
		ext = ".mp3"
	}
	path := filepath.Join(targetAudioDir, utils.SanitizeJID(sender.String())+ext)
	if err := os.WriteFile(path, data, 0644); err != nil {
		Logger.Error("File save failed", "err", err)
		if sendErr := sendTextRaw(ctx, client, evt.Info.Chat, Sprintf("failed to save audio: %v", err)); sendErr != nil {
			Logger.Error("failed to notify user", "sendErr", sendErr)
		}
		return
	}

	path, err = utils.TranscodeToMP3(path)
	if err != nil {
		Logger.Error("Transcode failed", "err", err)
		if sendErr := sendTextRaw(ctx, client, evt.Info.Chat, Sprintf("failed to process audio: %v", err)); sendErr != nil {
			Logger.Error("failed to notify user", "sendErr", sendErr)
		}
		return
	}

	if saveRequested {
		if err := saveAudio(cctx, sender, path); err != nil {
			Logger.Error("saveAudio failed", "err", err)
			logHandlerErr("call-audio-save", err)
		}
	}

	Logger.Debug("Triggering outgoing call to target", "target", p.Target, "media", path)
	if err := placeCallWithAudio(cctx, p.Target, path); err != nil {
		Logger.Error("placeCallWithAudio failed", "err", err)
		logHandlerErr("call", err)
	}
}

func handleVideoDownload(ctx context.Context, client *whatsmeow.Client, cctx *Context, sender types.JID, evt *events.Message, videoMsg *waE2E.VideoMessage, p *cliutils.PendingCall, saveRequested bool) {
	Logger.Debug("Downloading video payload", "sender", sender.String())
	data, err := client.Download(ctx, videoMsg)
	if err != nil {
		Logger.Error("Download video failed", "err", err)
		if sendErr := sendTextRaw(ctx, client, evt.Info.Chat, Sprintf("failed to download video: %v", err)); sendErr != nil {
			Logger.Error("failed to notify user", "sendErr", sendErr)
		}
		return
	}

	targetVideoDir := GetSessionMediaDir(client, "call-video")
	if err := os.MkdirAll(targetVideoDir, 0755); err != nil {
		Logger.Error("Failed creating video directory", "err", err)
		if sendErr := sendTextRaw(ctx, client, evt.Info.Chat, Sprintf("failed to prepare storage: %v", err)); sendErr != nil {
			Logger.Error("failed to notify user", "sendErr", sendErr)
		}
		return
	}

	ext := utils.ExtensionFor(videoMsg.GetMimetype())
	if ext == "" || ext == ".bin" {
		ext = ".mp4"
	}
	path := filepath.Join(targetVideoDir, utils.SanitizeJID(sender.String())+ext)
	if err := os.WriteFile(path, data, 0644); err != nil {
		Logger.Error("File save failed", "err", err)
		if sendErr := sendTextRaw(ctx, client, evt.Info.Chat, Sprintf("failed to save video: %v", err)); sendErr != nil {
			Logger.Error("failed to notify user", "sendErr", sendErr)
		}
		return
	}

	_, _, _ = utils.PrepareCallVideo(path)

	if saveRequested {
		if err := saveVideo(cctx, sender, path); err != nil {
			Logger.Error("saveVideo failed", "err", err)
			logHandlerErr("call-video-save", err)
		}
	}

	Logger.Debug("Triggering outgoing video call to target", "target", p.Target, "media", path)
	if err := placeVideoCallWithMedia(cctx, p.Target, path); err != nil {
		Logger.Error("placeVideoCallWithMedia failed", "err", err)
		logHandlerErr("videocall", err)
	}
}

func meowLogger() zerolog.Logger {
	return Logger.ZerologStyle("wacaller")
}

func RegisterWACaller(wa *whatsmeow.Client) *whatsmeow.Client {
	if wa == nil {
		return nil
	}
	wa.SetCallLogger(meowLogger())
	SetupVoicemail(wa)
	return wa
}

func getWACallerClient(wa *whatsmeow.Client) *whatsmeow.Client {
	return wa
}

func placeCallWithAudio(ctx *Context, target, audioPath string) error {
	client := getWACallerClient(ctx.Client)

	var targetJID types.JID
	if strings.Contains(target, "@") {
		targetJID, _ = types.ParseJID(target)
	} else {
		targetJID = types.NewJID(target, types.DefaultUserServer)
	}

	userTag, mentionJID := ctx.FormatMention(targetJID)

	call, err := client.Call(context.Background(), target)
	if err != nil {
		return ctx.ReplyWithMentions(Sprintf("Call to %s failed: %v", userTag, err), []types.JID{mentionJID})
	}

	// Drain incoming audio frames
	call.Receive(whatsmeow.SinkFunc(func(pcm []float32) {}))

	duration, durErr := utils.AudioDuration(audioPath)
	if durErr != nil || duration == 0 {
		logHandlerErr("call", fmt.Errorf("could not determine audio duration, using 30s fallback: %w", durErr))
		duration = 30 * time.Second
	}

	var startOnce sync.Once
	startMedia := func() {
		startOnce.Do(func() {
			src, err := openAudioSource(audioPath)
			if err != nil {
				logHandlerErr("call", err)
				if hErr := call.Hangup(); hErr != nil {
					logHandlerErr("call", hErr)
				}
				return
			}
			call.Play(src)

			go func() {
				time.Sleep(duration + 1*time.Second)
				if call.State() != whatsmeow.CallPhaseEnded {
					if hErr := call.Hangup(); hErr != nil {
						logHandlerErr("call", hErr)
					}
				}
			}()
		})
	}

	call.OnPeerAccept(func() {
		startMedia()
	})

	call.OnReady(func() {
		startMedia()
	})

	call.OnEnd(func(reason string) {
		if err := ctx.ReplyWithMentions(Sprintf("Call with %s ended: %s", userTag, reason), []types.JID{mentionJID}); err != nil {
			logHandlerErr("call", err)
		}
	})

	return ctx.ReplyWithMentions(Sprintf("Calling %s...", userTag), []types.JID{mentionJID})
}

func placeVideoCallWithMedia(ctx *Context, target, videoPath string) error {
	client := getWACallerClient(ctx.Client)

	var targetJID types.JID
	if strings.Contains(target, "@") {
		targetJID, _ = types.ParseJID(target)
	} else {
		targetJID = types.NewJID(target, types.DefaultUserServer)
	}

	userTag, mentionJID := ctx.FormatMention(targetJID)

	call, err := client.CallWithOptions(context.Background(), target, whatsmeow.CallOptions{Video: true})
	if err != nil {
		return ctx.ReplyWithMentions(Sprintf("Video call to %s failed: %v", userTag, err), []types.JID{mentionJID})
	}

	call.Receive(whatsmeow.SinkFunc(func(pcm []float32) {}))
	call.ReceiveVideo(whatsmeow.VideoSinkFunc(func(accessUnit []byte) {}))

	var requestKeyframe atomic.Bool
	requestKeyframe.Store(true)

	var startOnce sync.Once
	startMedia := func() {
		startOnce.Do(func() {
			Logger.Debug("videocall: starting media playback", "state", call.State(), "video_path", videoPath)

			_ = call.SetVideoEnabled(true)

			if videoPath != "" {
				mp3Path, h264Path, prepErr := utils.PrepareCallVideo(videoPath)
				if prepErr != nil {
					logHandlerErr("videocall", fmt.Errorf("failed to prepare call video: %w", prepErr))
				}
				Logger.Debug("videocall: prep done", "mp3", mp3Path, "h264", h264Path, "err", prepErr)

				audioFile := mp3Path
				if audioFile == "" {
					audioFile = videoPath
				}

				duration, durErr := utils.AudioDuration(audioFile)
				if durErr != nil || duration == 0 {
					duration, durErr = utils.AudioDuration(videoPath)
				}
				if durErr != nil || duration == 0 {
					duration = 30 * time.Second
				}
				Logger.Debug("videocall: media duration", "duration", duration)

				if src, err := openAudioSource(audioFile); err == nil {
					Logger.Debug("videocall: audio source opened, starting playback", "audio_file", audioFile)
					call.Play(src)
				} else {
					Logger.Debug("videocall: could not open audio source", "audio_file", audioFile, "err", err)
				}

				if h264Path != "" {
					h264Data, readErr := os.ReadFile(h264Path)
					if readErr != nil {
						Logger.Debug("videocall: failed to read h264 file", "h264_path", h264Path, "err", readErr)
					} else if len(h264Data) > 0 {
						frames := utils.SplitAnnexBAccessUnits(h264Data)
						Logger.Debug("videocall: split h264 into access units", "access_units", len(frames), "bytes", len(h264Data))
						if len(frames) > 0 {
							var idrIndices []int
							for i, f := range frames {
								if utils.AnnexBHasIDR(f) {
									idrIndices = append(idrIndices, i)
								}
							}
							Logger.Debug("videocall: found IDR keyframe positions", "idr_frames", len(idrIndices), "total_frames", len(frames))

							go func() {
								frameDur := 66 * time.Millisecond
								ticker := time.NewTicker(frameDur)
								defer ticker.Stop()

								timer := time.NewTimer(duration)
								defer timer.Stop()

								frameIdx := 0
								sent := 0
								for {
									select {
									case <-timer.C:
										if call.State() != whatsmeow.CallPhaseEnded {
											Logger.Debug("videocall: media duration completed, hanging up immediately", "duration", duration)
											_ = call.Hangup()
										}
										return
									case <-ticker.C:
										if call.State() == whatsmeow.CallPhaseEnded {
											Logger.Debug("videocall: call ended after sending frames", "sent", sent)
											return
										}

										if requestKeyframe.Swap(false) {
											bestIdx := 0
											for _, idx := range idrIndices {
												if idx >= frameIdx {
													bestIdx = idx
													break
												}
											}
											frameIdx = bestIdx
											Logger.Debug("videocall: keyframe triggered", "frame_idx", frameIdx)
										}

										if frameIdx >= len(frames) {
											if call.State() != whatsmeow.CallPhaseEnded {
												Logger.Debug("videocall: all video frames sent, hanging up immediately", "sent", sent)
												_ = call.Hangup()
											}
											return
										}

										frame := frames[frameIdx]
										if err := call.SendVideoWithDuration(frame, frameDur); err != nil {
											if !strings.Contains(err.Error(), "has no active video media") {
												logHandlerErr("videocall", err)
											}
										} else {
											sent++
											if sent == 1 || sent%30 == 0 {
												Logger.Debug("videocall: sent frame", "sent", sent, "access_unit", frameIdx, "bytes", len(frame))
											}
										}

										frameIdx++
										if frameIdx >= len(frames) {
											if call.State() != whatsmeow.CallPhaseEnded {
												Logger.Debug("videocall: reached last frame, hanging up immediately", "sent", sent)
												_ = call.Hangup()
											}
											return
										}
									}
								}
							}()
						}
					}
				}
			}
		})
	}

	call.OnPeerAccept(func() {
		Logger.Debug("videocall: peer accepted, queuing immediate IDR keyframe")
		requestKeyframe.Store(true)
		startMedia()
	})

	call.OnVideoKeyframeRequest(func() {
		Logger.Debug("videocall: keyframe requested by peer PLI/FIR, queuing IDR keyframe")
		requestKeyframe.Store(true)
	})

	call.OnReady(func() {
		Logger.Debug("videocall: media ready (inbound RTP flowing)")
		startMedia()
	})

	call.OnEnd(func(reason string) {
		if err := ctx.ReplyWithMentions(Sprintf("Video call with %s ended: %s", userTag, reason), []types.JID{mentionJID}); err != nil {
			logHandlerErr("videocall", err)
		}
	})

	return ctx.ReplyWithMentions(Sprintf("Video calling %s...", userTag), []types.JID{mentionJID})
}

func openAudioSource(path string) (whatsmeow.AudioSource, error) {
	switch {
	case strings.HasSuffix(path, ".mp3"):
		return whatsmeow.MP3File(path)
	case strings.HasSuffix(path, ".wav"):
		return whatsmeow.WAVFile(path)
	case strings.HasSuffix(path, ".opus"), strings.HasSuffix(path, ".ogg"):
		return whatsmeow.OpusFile(path)
	default:
		return nil, fmt.Errorf("unsupported audio extension for %s", path)
	}
}
