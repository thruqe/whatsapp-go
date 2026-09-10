package calls

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

	"whatsrook"
	"whatsrook/cmd/dispatch"
	"whatsrook/cmd/store"
	"whatsrook/logger"
)

func logHandlerErr(name string, err error) {
	if err == nil {
		return
	}
	logger.Error("call handler error", "name", name, "err", err)
}

func init() {
	dispatch.Register(&dispatch.Command{
		Name:        "call",
		Alias:       "phone",
		Description: "Call a number or open the interactive call menu",
		Category:    "calls",
		IsPublic:    true,
		Handler:     handleCall,
	})
	dispatch.Register(&dispatch.Command{
		Name:        "callaudio",
		Alias:       "setcallaudio",
		Description: "Call a number with audio, or set your default call audio",
		Category:    "calls",
		IsPublic:    true,
		Handler:     handleCallAudio,
	})
	dispatch.Register(&dispatch.Command{
		Name:        "callvideo",
		Alias:       "videocall",
		Description: "Call a number with video, or set your default call video",
		Category:    "calls",
		IsPublic:    true,
		Handler:     handleCallVideo,
	})
	dispatch.Register(&dispatch.Command{
		Name:        "anticall",
		Description: "Configure anti-call security to automatically reject incoming WhatsApp calls",
		Category:    "calls",
		IsPublic:    true,
		Handler:     handleAntiCall,
	})
	dispatch.Register(&dispatch.Command{
		Name:        "voicemail",
		Description: "Toggle or check automated voicemail answering for incoming calls",
		Category:    "calls",
		IsPublic:    true,
		Handler:     handleVoicemail,
	})
}

var (
	pendingMu sync.Mutex
	pending   = map[types.JID]*PendingCall{}
)

func setPending(sender types.JID, p *PendingCall) {
	SetPendingCall(sender, p)
	pendingMu.Lock()
	defer pendingMu.Unlock()
	pending[sender] = p
}

func peekPending(sender types.JID) (*PendingCall, bool) {
	return PeekPendingCall(sender)
}

func popPending(sender types.JID) (*PendingCall, bool) {
	return PopPendingCall(sender)
}

// mediaStore extracts the concrete dispatch.StoreWrapper from a Context's client
func mediaStore(ctx *dispatch.Context) (*dispatch.StoreWrapper, error) {
	s, ok := dispatch.GetStore(ctx)
	if !ok {
		return nil, fmt.Errorf("unexpected store implementation")
	}
	return s, nil
}

// candidateJIDs returns the set of JIDs a call-media config might be stored
// under for a given client (sender, plus the bot's own JID/LID/ID variants).
func candidateJIDs(client *whatsmeow.Client, sender types.JID) []types.JID {
	candidates := []types.JID{sender.ToNonAD()}
	if client == nil || client.Store == nil {
		return candidates
	}
	candidates = append(candidates, client.Store.GetJID().ToNonAD(), client.Store.GetLID().ToNonAD())
	if client.Store.ID != nil {
		candidates = append(candidates, client.Store.ID.ToNonAD())
	}
	return candidates
}

// findExistingMediaFile scans dir for the first file with one of the given
// extensions, used as a fallback when no config-store entry is found.
func findExistingMediaFile(dir string, extensions ...string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		for _, ext := range extensions {
			if strings.HasSuffix(entry.Name(), ext) {
				return filepath.Join(dir, entry.Name())
			}
		}
	}
	return ""
}

func resolveSavedCallAudio(client *whatsmeow.Client, sender types.JID) string {
	if client == nil || client.Store == nil {
		return ""
	}
	s, ok := dispatch.GetSQLStore(client)
	if !ok {
		return ""
	}
	ctx := context.Background()
	for _, jid := range candidateJIDs(client, sender) {
		if jid.IsEmpty() {
			continue
		}
		if path, err := s.GetCallMediaConfig(ctx, jid, store.CallMediaAudio); err == nil && path != "" {
			if _, statErr := os.Stat(path); statErr == nil {
				return path
			}
		}
	}
	audioDir := dispatch.GetSessionMediaDir(client, "call-audio")
	return findExistingMediaFile(audioDir, ".mp3", ".ogg", ".wav")
}

func resolveSavedCallVideo(client *whatsmeow.Client, sender types.JID) string {
	if client == nil || client.Store == nil {
		return ""
	}
	s, ok := dispatch.GetSQLStore(client)
	if !ok {
		return ""
	}
	ctx := context.Background()
	for _, jid := range candidateJIDs(client, sender) {
		if jid.IsEmpty() {
			continue
		}
		if path, err := s.GetCallMediaConfig(ctx, jid, store.CallMediaVideo); err == nil && path != "" {
			if _, statErr := os.Stat(path); statErr == nil {
				return path
			}
		}
	}
	videoDir := dispatch.GetSessionMediaDir(client, "call-video")
	return findExistingMediaFile(videoDir, ".mp4", ".bin", ".3gp")
}

func getSavedAudio(ctx *dispatch.Context, sender types.JID) (string, bool) {
	path := resolveSavedCallAudio(ctx.Client, sender)
	return path, path != ""
}

func saveAudio(ctx *dispatch.Context, sender types.JID, path string) error {
	s, err := mediaStore(ctx)
	if err != nil {
		return err
	}
	_ = s.PutCallMediaConfig(ctx.Ctx, sender.ToNonAD(), store.CallMediaAudio, path)
	for _, jid := range candidateJIDs(ctx.Client, sender) {
		if jid.IsEmpty() || jid == sender.ToNonAD() {
			continue
		}
		_ = s.PutCallMediaConfig(ctx.Ctx, jid, store.CallMediaAudio, path)
	}
	return nil
}

func getSavedVideo(ctx *dispatch.Context, sender types.JID) (string, bool) {
	path := resolveSavedCallVideo(ctx.Client, sender)
	return path, path != ""
}

func saveVideo(ctx *dispatch.Context, sender types.JID, path string) error {
	s, err := mediaStore(ctx)
	if err != nil {
		return err
	}
	_ = s.PutCallMediaConfig(ctx.Ctx, sender.ToNonAD(), store.CallMediaVideo, path)
	for _, jid := range candidateJIDs(ctx.Client, sender) {
		if jid.IsEmpty() || jid == sender.ToNonAD() {
			continue
		}
		_ = s.PutCallMediaConfig(ctx.Ctx, jid, store.CallMediaVideo, path)
	}
	return nil
}

func handleCall(ctx *dispatch.Context) error {
	p := ctx.GetPrefix()
	targets := ctx.GetTargets()
	if len(targets) < 1 {
		body := dispatch.NewText().
			Header("Calls").
			Section("What would you like to do?").
			Bulletf("%scallaudio [number] — voice call, or set your call audio", p).
			Bulletf("%scallvideo [number] — video call, or set your call video", p).
			Bulletf("%svoicemail [on/off] — auto-answer incoming calls", p).
			Trimmed()
		options := []string{"Call Audio", "Call Video", "Voicemail"}
		return dispatch.SendPollReply(ctx, body, options)
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

	body := whatsrook.Sprintf("Ready to call %s — how should I place it?", targetMention)
	options := []string{"Audio Call", "Video Call"}
	return dispatch.SendPollReplyWithMentions(ctx, body, options, []types.JID{targetJID})
}

func handleCallAudio(ctx *dispatch.Context) error {
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
			return ctx.Replyf("Your default call audio is `%s`.\n\n• `%scallaudio <number>` to place a call\n• Reply to new audio with `%scallaudio` to change it", baseName, p, p)
		}
		return ctx.Replyf("Usage: `%scallaudio <number>`\n\nTo set a default, reply to any voice note or audio file with `%scallaudio`.", p, p)
	}

	target := targets[0].String()

	if path, ok := getSavedAudio(ctx, ctx.Sender); ok {
		return placeCallWithAudio(ctx, target, path)
	}

	setPending(ctx.Sender, &PendingCall{Target: target, Kind: store.CallMediaAudio})
	return ctx.Reply("Send an audio file to use for this call.\n" +
		"Reply \"save\" on that audio to make it your default for next time.")
}

func handleSetCallAudio(ctx *dispatch.Context) error {
	var audioMsg *waE2E.AudioMessage
	if ext := ctx.Evt.Message.GetExtendedTextMessage(); ext != nil {
		if ci := ext.GetContextInfo(); ci != nil && ci.QuotedMessage != nil {
			audioMsg = ci.QuotedMessage.GetAudioMessage()
		}
	}

	if audioMsg == nil {
		return ctx.Reply("Reply to the audio file you'd like to set as your default call audio.")
	}

	data, err := ctx.Client.Download(ctx.Ctx, audioMsg)
	if err != nil {
		return ctx.Replyf("Couldn't download that audio: %v", err)
	}

	targetAudioDir := dispatch.GetSessionMediaDir(ctx.Client, "call-audio")
	if err := os.MkdirAll(targetAudioDir, 0o755); err != nil {
		return ctx.Replyf("Couldn't set up storage for that: %v", err)
	}

	ext := ExtensionFor(audioMsg.GetMimetype())
	if ext == "" || ext == ".bin" {
		ext = ".mp3"
	}
	path := filepath.Join(targetAudioDir, SanitizeJID(ctx.Sender.String())+ext)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return ctx.Replyf("Couldn't save that audio: %v", err)
	}

	path, err = TranscodeToMP3(path)
	if err != nil {
		return ctx.Replyf("Couldn't process that audio: %v", err)
	}

	if err := saveAudio(ctx, ctx.Sender, path); err != nil {
		return ctx.Replyf("Couldn't save your call audio: %v", err)
	}

	return ctx.Reply("Done — that's your default call audio now.")
}

func handleCallVideo(ctx *dispatch.Context) error {
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
			return ctx.Replyf("Your default call video is `%s`.\n\n• `%scallvideo <number>` to place a call\n• Reply to new video with `%scallvideo` to change it", baseName, p, p)
		}
		return ctx.Replyf("Usage: `%scallvideo <number>`\n\nTo set a default, reply to any video with `%scallvideo`.", p, p)
	}

	target := targets[0].String()
	_ = ctx.Reply("Heads up: outgoing video calls are unreliable on WhatsApp Web's protocol, so this may not go through cleanly.")

	if videoMsg != nil {
		data, err := ctx.Client.Download(ctx.Ctx, videoMsg)
		if err == nil && len(data) > 0 {
			targetVideoDir := dispatch.GetSessionMediaDir(ctx.Client, "call-video")
			_ = os.MkdirAll(targetVideoDir, 0o755)
			ext := ExtensionFor(videoMsg.GetMimetype())
			if ext == "" || ext == ".bin" {
				ext = ".mp4"
			}
			path := filepath.Join(targetVideoDir, SanitizeJID(ctx.Sender.String())+ext)
			if err := os.WriteFile(path, data, 0o644); err == nil {
				_, _, _ = PrepareCallVideo(path)
				return placeVideoCallWithMedia(ctx, target, path)
			}
		}
	}

	if path, ok := getSavedVideo(ctx, ctx.Sender); ok {
		return placeVideoCallWithMedia(ctx, target, path)
	}

	setPending(ctx.Sender, &PendingCall{Target: target, Kind: store.CallMediaVideo})
	return ctx.Reply("Send a video to use for this call.\n" +
		"Reply \"save\" on that video to make it your default for next time.")
}

func handleSetVideoCall(ctx *dispatch.Context) error {
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
			return ctx.Replyf("Your default call video is currently `%s`.\n\nReply to a new video with `%ssetvideocall` to change it.", baseName, ctx.GetPrefix())
		}
		return ctx.Replyf("Reply to a video with `%ssetvideocall` to set your default call video.", ctx.GetPrefix())
	}

	data, err := ctx.Client.Download(ctx.Ctx, videoMsg)
	if err != nil {
		return ctx.Replyf("Couldn't download that video: %v", err)
	}

	targetVideoDir := dispatch.GetSessionMediaDir(ctx.Client, "call-video")
	if err := os.MkdirAll(targetVideoDir, 0o755); err != nil {
		return ctx.Replyf("Couldn't set up storage for that: %v", err)
	}

	ext := ExtensionFor(videoMsg.GetMimetype())
	if ext == "" || ext == ".bin" {
		ext = ".mp4"
	}
	path := filepath.Join(targetVideoDir, SanitizeJID(ctx.Sender.String())+ext)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return ctx.Replyf("Couldn't save that video: %v", err)
	}

	_, _, _ = PrepareCallVideo(path)

	if err := saveVideo(ctx, ctx.Sender, path); err != nil {
		return ctx.Replyf("Couldn't save your call video: %v", err)
	}

	return ctx.Reply("Done — that's your default call video now.")
}

func handleAntiCall(ctx *dispatch.Context) error {
	s, ok := dispatch.GetStore(ctx)
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
	case "on", "enable", "activate":
		_ = s.PutSetting(ctx.Ctx, "anticall_status", "on")
		return ctx.Reply("AntiCall is on — incoming calls will be automatically rejected.")

	case "off", "disable", "deactivate":
		_ = s.PutSetting(ctx.Ctx, "anticall_status", "off")
		return ctx.Reply("AntiCall is off.")

	case "toggle":
		curr, _ := s.GetSetting(ctx.Ctx, "anticall_status")
		if curr == "on" {
			_ = s.PutSetting(ctx.Ctx, "anticall_status", "off")
			return ctx.Reply("AntiCall is off.")
		}
		_ = s.PutSetting(ctx.Ctx, "anticall_status", "on")
		return ctx.Reply("AntiCall is on — incoming calls will be automatically rejected.")

	case "customize", "custom", "help":
		return sendAntiCallCustomizeGuide(ctx)

	case "contacts":
		if len(args) < 2 {
			curr, _ := s.GetSetting(ctx.Ctx, "anticall_contacts_only")
			label := "off"
			if curr == "true" {
				label = "on"
			}
			return ctx.Reply("Contacts-only mode is currently " + label + ".")
		}
		mode := strings.ToLower(args[1])
		switch mode {
		case "on", "true", "enable", "activate":
			_ = s.PutSetting(ctx.Ctx, "anticall_contacts_only", "true")
			return ctx.Reply("Got it — only your saved contacts will be able to call you.")
		case "off", "false", "disable", "deactivate":
			_ = s.PutSetting(ctx.Ctx, "anticall_contacts_only", "false")
			return ctx.Reply("Contacts-only restriction turned off.")
		case "toggle":
			curr, _ := s.GetSetting(ctx.Ctx, "anticall_contacts_only")
			if curr == "true" {
				_ = s.PutSetting(ctx.Ctx, "anticall_contacts_only", "false")
				return ctx.Reply("Contacts-only restriction turned off.")
			}
			_ = s.PutSetting(ctx.Ctx, "anticall_contacts_only", "true")
			return ctx.Reply("Got it — only your saved contacts will be able to call you.")
		}
		return ctx.Replyf("Usage: %santicall contacts [on|off|toggle]", p)

	case "cc":
		if len(args) < 2 {
			allowed, _ := s.GetSetting(ctx.Ctx, "anticall_allowed_cc")
			if allowed == "" {
				return ctx.Reply("No country codes are currently allowed through AntiCall.")
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
			return ctx.Reply("Added +" + cc + " to the allowed list.")

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
			return ctx.Reply("Removed +" + cc + " from the allowed list.")

		case "clear":
			_ = s.PutSetting(ctx.Ctx, "anticall_allowed_cc", "")
			return ctx.Reply("Cleared the allowed country codes list.")

		default:
			return ctx.Replyf("Usage: %santicall cc [add|del|clear]", p)
		}

	case "warn", "warnings":
		if len(args) < 2 {
			curr, _ := s.GetSetting(ctx.Ctx, "anticall_max_warn")
			if curr == "" {
				curr = "3"
			}
			return ctx.Reply("Callers get " + curr + " warning(s) before AntiCall blocks them.")
		}
		num, err := strconv.Atoi(args[1])
		if err != nil || num < 1 {
			return ctx.Reply("That doesn't look like a valid number — try something like `3`.")
		}
		_ = s.PutSetting(ctx.Ctx, "anticall_max_warn", strconv.Itoa(num))
		return ctx.Reply("Warning threshold set to " + strconv.Itoa(num) + ".")

	default:
		return ctx.Replyf("Usage: %santicall [on|off|toggle|customize|contacts|cc|warn]", p)
	}
}

func sendAntiCallMenu(ctx *dispatch.Context, s *dispatch.StoreWrapper) error {
	status, _ := s.GetSetting(ctx.Ctx, "anticall_status")
	if status == "" {
		status = "off"
	}

	bodyText := dispatch.NewText().
		Header("AntiCall").
		Field("Status", strings.ToUpper(status)).
		Blank().
		Line("Pick an option below.").
		Trimmed()

	actionText := "Activate"
	if status == "on" {
		actionText = "Deactivate"
	}

	options := []string{
		actionText,
		"Customize",
	}
	return dispatch.SendPollReply(ctx, bodyText, options)
}

func sendAntiCallCustomizeGuide(ctx *dispatch.Context) error {
	p := ctx.GetPrefix()
	return ctx.Text().
		Header("AntiCall Customization").
		Section("Options").
		Bulletf("Contacts only : %santicall contacts on | off", p).
		Bulletf("Country codes : %santicall cc add | del | clear <code>", p).
		Bulletf("Warning limit : %santicall warn <number>", p).
		Blank().
		Section("Examples").
		Numberedf(1, "%santicall contacts on — reject calls from anyone not in your contacts", p).
		Numberedf(2, "%santicall cc add 234 — allow calls from +234 numbers", p).
		Numberedf(3, "%santicall warn 3 — auto-block a caller after 3 warnings", p).
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

func handleVoicemail(ctx *dispatch.Context) error {
	s, ok := dispatch.GetStore(ctx)
	if !ok {
		return ctx.Reply("Database store is not available.")
	}

	if len(ctx.Args) > 0 {
		switch strings.ToLower(ctx.Args[0]) {
		case "on", "enable", "activate":
			_ = s.PutSetting(ctx.Ctx, VoicemailSettingKey, "on")
			return ctx.Reply("Voicemail is on — incoming calls will be answered automatically with your saved audio/video.")
		case "off", "disable", "deactivate":
			_ = s.PutSetting(ctx.Ctx, VoicemailSettingKey, "off")
			return ctx.Reply("Voicemail is off.")
		case "toggle":
			curr, _ := s.GetSetting(ctx.Ctx, VoicemailSettingKey)
			if curr == "" {
				curr, _ = s.GetSetting(ctx.Ctx, "autoacceptcall_status")
			}
			if curr == "on" {
				_ = s.PutSetting(ctx.Ctx, VoicemailSettingKey, "off")
				return ctx.Reply("Voicemail is off.")
			}
			_ = s.PutSetting(ctx.Ctx, VoicemailSettingKey, "on")
			return ctx.Reply("Voicemail is on — incoming calls will be answered automatically with your saved audio/video.")
		default:
			p := ctx.GetPrefix()
			return ctx.Replyf("Usage:\n- %svoicemail on\n- %svoicemail off\n- %svoicemail toggle", p, p, p)
		}
	}

	status, _ := s.GetSetting(ctx.Ctx, VoicemailSettingKey)
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
		audioStatus = "Not set"
	}
	videoStatus := "Set"
	if videoPath == "" {
		videoStatus = "Not set"
	}

	bodyText := dispatch.NewText().
		Header("Voicemail").
		Field("Status", strings.ToUpper(status)).
		Field("Audio", audioStatus).
		Field("Video", videoStatus).
		Blank().
		Line("Automatically answers incoming calls with your saved media.").
		Blank().
		Section("Usage").
		Bulletf("%svoicemail on", p).
		Bulletf("%svoicemail off", p).
		Trimmed()

	actionText := "Activate"
	if status == "on" {
		actionText = "Deactivate"
	}

	options := []string{
		actionText,
		"Call Audio",
		"Call Video",
	}

	return dispatch.SendPollReply(ctx, bodyText, options)
}

// SetupVoicemail wires the OnIncomingCall handler.
func SetupVoicemail(wa *whatsmeow.Client) {
	if wa == nil {
		logger.Error("SetupVoicemail: nil client")
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

	s, ok := dispatch.GetSQLStore(waClient)
	if !ok {
		return
	}

	status, _ := s.GetSetting(ctx, VoicemailSettingKey)
	if status == "" {
		status, _ = s.GetSetting(ctx, "autoacceptcall_status")
	}
	if status != "on" {
		logger.Debug("voicemail: incoming call offer ignored because voicemail is not enabled (enable using .voicemail on)", "call_id", call.ID(), "from", call.Peer().String(), "status", status)
		return
	}

	audioPath := resolveSavedCallAudio(waClient, types.EmptyJID)
	videoPath := resolveSavedCallVideo(waClient, types.EmptyJID)

	isVideo := call.IsVideo()
	logger.Debug("voicemail: answering incoming call", "from", call.Peer().String(), "call_id", call.ID(), "is_video", isVideo, "audio", audioPath, "video", videoPath)

	// Set up null receivers BEFORE answering
	call.Receive(whatsmeow.SinkFunc(func(pcm []float32) {}))
	if isVideo {
		call.ReceiveVideo(whatsmeow.VideoSinkFunc(func(accessUnit []byte) {}))
	}

	var mediaOnce sync.Once
	startMedia := func() {
		mediaOnce.Do(func() {
			switch {
			case isVideo && videoPath != "":
				startVideoMedia(call, videoPath)
			case isVideo && audioPath != "":
				startAudioMedia(call, audioPath)
			case isVideo:
				logger.Warn("voicemail: no video or audio media found for incoming video call")
			case audioPath != "":
				startAudioMedia(call, audioPath)
			default:
				logger.Warn("voicemail: no audio media found for incoming voice call")
			}
		})
	}

	// OnReady fires when first inbound RTP packet arrives
	call.OnReady(func() {
		logger.Debug("voicemail: OnReady fired, starting media", "call_id", call.ID())
		startMedia()
	})

	// Let wacaller handle the full signaling — Answer waits for mute_v2 then sends accept
	if err := call.Answer(); err != nil {
		logger.Error("voicemail: call.Answer() failed", "call_id", call.ID(), "err", err)
		return
	}

	// If OnReady hasn't fired within 10s, something is wrong with the media path.
	// Start anyway — the audio will queue until the relay connects, or fail gracefully.
	go func() {
		time.Sleep(10 * time.Second)
		if call.State() != whatsmeow.CallPhaseEnded {
			logger.Debug("voicemail: OnReady timeout, starting media anyway", "call_id", call.ID())
			startMedia()
		}
	}()
}

func startAudioMedia(call *whatsmeow.Call, audioPath string) {
	logger.Debug("voicemail: starting audio media", "call_id", call.ID(), "path", audioPath)

	src, err := openAudioSource(audioPath)
	if err != nil {
		logger.Error("voicemail: failed to load audio", "path", audioPath, "err", err)
		_ = call.Hangup()
		return
	}

	call.Play(src)

	duration, err := AudioDuration(audioPath)
	if err != nil || duration == 0 {
		duration = 30 * time.Second
	}

	go func() {
		time.Sleep(duration)
		if call.State() != whatsmeow.CallPhaseEnded {
			logger.Debug("voicemail: audio duration completed, hanging up", "call_id", call.ID())
			_ = call.Hangup()
		}
	}()
}

func startVideoMedia(call *whatsmeow.Call, videoPath string) {
	logger.Debug("voicemail: starting video media", "call_id", call.ID(), "path", videoPath)

	mp3Path, h264Path, err := PrepareCallVideo(videoPath)
	if err != nil {
		logger.Error("voicemail: failed to prepare video", "err", err)
		_ = call.Hangup()
		return
	}

	if err := call.SetVideoEnabled(true); err != nil {
		logger.Error("voicemail: SetVideoEnabled failed", "err", err)
	}

	audioFile := mp3Path
	if audioFile == "" {
		audioFile = videoPath
	}
	src, err := openAudioSource(audioFile)
	if err != nil {
		logger.Error("voicemail: failed to load audio", "path", audioFile, "err", err)
		_ = call.Hangup()
		return
	}
	call.Play(src)

	if h264Path == "" {
		logger.Warn("voicemail: no h264 track, audio-only for video call", "call_id", call.ID())
		return
	}

	h264Data, err := os.ReadFile(h264Path)
	if err != nil || len(h264Data) == 0 {
		logger.Error("voicemail: failed to read h264", "path", h264Path, "err", err)
		return
	}

	frames := SplitAnnexBAccessUnits(h264Data)
	if len(frames) == 0 {
		logger.Error("voicemail: no video frames", "path", h264Path)
		return
	}

	duration, err := AudioDuration(audioFile)
	if err != nil || duration == 0 {
		duration = 30 * time.Second
	}

	go runVideoFrameLoop(call, frames, duration, nil)
}

// runVideoFrameLoop drains frames to the call at a fixed frame interval
// until either the duration timer fires or frames run out, hanging up in
// either case. If requestKeyframe is non-nil, a true value causes playback
// to jump to the next available IDR keyframe (used for outgoing calls that
// react to peer PLI/FIR requests); pass nil to always play frames in order.
func runVideoFrameLoop(call *whatsmeow.Call, frames [][]byte, duration time.Duration, requestKeyframe *atomic.Bool) {
	const frameDur = 66 * time.Millisecond
	ticker := time.NewTicker(frameDur)
	defer ticker.Stop()
	timer := time.NewTimer(duration)
	defer timer.Stop()

	idrIndices := idrFrameIndices(frames, requestKeyframe)

	frameIdx, sent := 0, 0
	for {
		select {
		case <-timer.C:
			hangupIfActive(call, "media duration completed")
			return
		case <-ticker.C:
			if call.State() == whatsmeow.CallPhaseEnded {
				return
			}
			if requestKeyframe != nil && requestKeyframe.Swap(false) {
				frameIdx = nextIDRIndex(idrIndices, frameIdx)
				logger.Debug("videocall: keyframe triggered", "frame_idx", frameIdx)
			}
			if frameIdx >= len(frames) {
				hangupIfActive(call, "all video frames sent")
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
					logger.Debug("videocall: sent frame", "sent", sent, "access_unit", frameIdx, "bytes", len(frame))
				}
			}
			frameIdx++
			if frameIdx >= len(frames) {
				hangupIfActive(call, "reached last frame")
				return
			}
		}
	}
}

func idrFrameIndices(frames [][]byte, requestKeyframe *atomic.Bool) []int {
	if requestKeyframe == nil {
		return nil
	}
	var idx []int
	for i, f := range frames {
		if AnnexBHasIDR(f) {
			idx = append(idx, i)
		}
	}
	logger.Debug("videocall: found IDR keyframe positions", "idr_frames", len(idx), "total_frames", len(frames))
	return idx
}

func nextIDRIndex(idrIndices []int, from int) int {
	for _, idx := range idrIndices {
		if idx >= from {
			return idx
		}
	}
	return 0
}

func hangupIfActive(call *whatsmeow.Call, reason string) {
	if call.State() != whatsmeow.CallPhaseEnded {
		logger.Debug("voicemail/videocall: hanging up", "call_id", call.ID(), "reason", reason)
		_ = call.Hangup()
	}
}

func isSaveText(text string) bool {
	t := strings.TrimSpace(strings.ToLower(text))
	return t == "save" || t == "set" || t == "use" || t == "yes"
}

func getDirectMessageText(msg *waE2E.Message) string {
	if msg == nil {
		return ""
	}
	if msg.Conversation != nil {
		return *msg.Conversation
	}
	if msg.ExtendedTextMessage != nil && msg.ExtendedTextMessage.Text != nil {
		return *msg.ExtendedTextMessage.Text
	}
	return ""
}

func sendTextRaw(ctx context.Context, client *whatsmeow.Client, chat types.JID, text string) error {
	pctx := &dispatch.Context{Ctx: ctx, Client: client, Chat: chat}
	return pctx.Rook().NewMessage(text).To(chat).Send()
}

func HandlePendingAudioReply(ctx context.Context, client *whatsmeow.Client, evt *events.Message) bool {
	sender := evt.Info.Sender

	p, ok := peekPending(sender)
	if !ok {
		return false
	}

	if p.Kind == store.CallMediaVideo {
		var videoMsg *waE2E.VideoMessage
		saveRequested := false

		if msg := evt.Message.GetVideoMessage(); msg != nil {
			logger.Debug("Detected direct video message", "sender", sender.String())
			videoMsg = msg
			saveRequested = isSaveText(getDirectMessageText(evt.Message))
		} else if extText := evt.Message.GetExtendedTextMessage(); extText != nil && isSaveText(extText.GetText()) {
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
			cctx := &dispatch.Context{
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
		logger.Debug("Detected direct audio message", "sender", sender.String())
		audioMsg = msg
		saveRequested = isSaveText(getDirectMessageText(evt.Message))
	} else if extText := evt.Message.GetExtendedTextMessage(); extText != nil && isSaveText(extText.GetText()) {
		logger.Debug("Detected text message containing 'save', checking quoted audio...", "sender", sender.String())
		if ctxInfo := extText.GetContextInfo(); ctxInfo != nil && ctxInfo.QuotedMessage != nil {
			if quotedAudio := ctxInfo.QuotedMessage.GetAudioMessage(); quotedAudio != nil {
				logger.Debug("Found quoted audio message in reply", "sender", sender.String())
				audioMsg = quotedAudio
				saveRequested = true
			}
		}
	}

	if audioMsg == nil {
		logger.Debug("Message did not provide or quote an audio message, skipping pending intercept", "sender", sender.String())
		return false
	}

	popPending(sender)

	go func() {
		cctx := &dispatch.Context{
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

func handleAudioDownload(ctx context.Context, client *whatsmeow.Client, cctx *dispatch.Context, sender types.JID, evt *events.Message, audioMsg *waE2E.AudioMessage, p *PendingCall, saveRequested bool) {
	logger.Debug("Downloading audio payload", "sender", sender.String())
	data, err := client.Download(ctx, audioMsg)
	if err != nil {
		logger.Error("Download audio failed", "err", err)
		if sendErr := sendTextRaw(ctx, client, evt.Info.Chat, whatsrook.Sprintf("Couldn't download that audio: %v", err)); sendErr != nil {
			logger.Error("failed to notify user", "sendErr", sendErr)
		}
		return
	}

	targetAudioDir := dispatch.GetSessionMediaDir(client, "call-audio")
	if err := os.MkdirAll(targetAudioDir, 0o755); err != nil {
		logger.Error("Failed creating audio directory", "err", err)
		if sendErr := sendTextRaw(ctx, client, evt.Info.Chat, whatsrook.Sprintf("Couldn't set up storage for that: %v", err)); sendErr != nil {
			logger.Error("failed to notify user", "sendErr", sendErr)
		}
		return
	}

	ext := ExtensionFor(audioMsg.GetMimetype())
	if ext == "" || ext == ".bin" {
		ext = ".mp3"
	}
	path := filepath.Join(targetAudioDir, SanitizeJID(sender.String())+ext)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		logger.Error("File save failed", "err", err)
		if sendErr := sendTextRaw(ctx, client, evt.Info.Chat, whatsrook.Sprintf("Couldn't save that audio: %v", err)); sendErr != nil {
			logger.Error("failed to notify user", "sendErr", sendErr)
		}
		return
	}

	path, err = TranscodeToMP3(path)
	if err != nil {
		logger.Error("Transcode failed", "err", err)
		if sendErr := sendTextRaw(ctx, client, evt.Info.Chat, whatsrook.Sprintf("Couldn't process that audio: %v", err)); sendErr != nil {
			logger.Error("failed to notify user", "sendErr", sendErr)
		}
		return
	}

	if saveRequested {
		if err := saveAudio(cctx, sender, path); err != nil {
			logger.Error("saveAudio failed", "err", err)
			logHandlerErr("call-audio-save", err)
		}
	}

	logger.Debug("Triggering outgoing call to target", "target", p.Target, "media", path)
	if err := placeCallWithAudio(cctx, p.Target, path); err != nil {
		logger.Error("placeCallWithAudio failed", "err", err)
		logHandlerErr("call", err)
	}
}

func handleVideoDownload(ctx context.Context, client *whatsmeow.Client, cctx *dispatch.Context, sender types.JID, evt *events.Message, videoMsg *waE2E.VideoMessage, p *PendingCall, saveRequested bool) {
	logger.Debug("Downloading video payload", "sender", sender.String())
	data, err := client.Download(ctx, videoMsg)
	if err != nil {
		logger.Error("Download video failed", "err", err)
		if sendErr := sendTextRaw(ctx, client, evt.Info.Chat, whatsrook.Sprintf("Couldn't download that video: %v", err)); sendErr != nil {
			logger.Error("failed to notify user", "sendErr", sendErr)
		}
		return
	}

	targetVideoDir := dispatch.GetSessionMediaDir(client, "call-video")
	if err := os.MkdirAll(targetVideoDir, 0o755); err != nil {
		logger.Error("Failed creating video directory", "err", err)
		if sendErr := sendTextRaw(ctx, client, evt.Info.Chat, whatsrook.Sprintf("Couldn't set up storage for that: %v", err)); sendErr != nil {
			logger.Error("failed to notify user", "sendErr", sendErr)
		}
		return
	}

	ext := ExtensionFor(videoMsg.GetMimetype())
	if ext == "" || ext == ".bin" {
		ext = ".mp4"
	}
	path := filepath.Join(targetVideoDir, SanitizeJID(sender.String())+ext)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		logger.Error("File save failed", "err", err)
		if sendErr := sendTextRaw(ctx, client, evt.Info.Chat, whatsrook.Sprintf("Couldn't save that video: %v", err)); sendErr != nil {
			logger.Error("failed to notify user", "sendErr", sendErr)
		}
		return
	}

	_, _, _ = PrepareCallVideo(path)

	if saveRequested {
		if err := saveVideo(cctx, sender, path); err != nil {
			logger.Error("saveVideo failed", "err", err)
			logHandlerErr("call-video-save", err)
		}
	}

	logger.Debug("Triggering outgoing video call to target", "target", p.Target, "media", path)
	if err := placeVideoCallWithMedia(cctx, p.Target, path); err != nil {
		logger.Error("placeVideoCallWithMedia failed", "err", err)
		logHandlerErr("videocall", err)
	}
}

func meowLogger() zerolog.Logger {
	return logger.ZerologStyle("wacaller")
}

func RegisterWACaller(wa *whatsmeow.Client) *whatsmeow.Client {
	if wa == nil {
		return nil
	}
	wa.SetCallLogger(meowLogger())
	SetupVoicemail(wa)
	return wa
}

func resolveTargetJID(target string) types.JID {
	if strings.Contains(target, "@") {
		jid, _ := types.ParseJID(target)
		return jid
	}
	return types.NewJID(target, types.DefaultUserServer)
}

func placeCallWithAudio(ctx *dispatch.Context, target, audioPath string) error {
	client := ctx.Client
	targetJID := resolveTargetJID(target)
	userTag, mentionJID := ctx.FormatMention(targetJID)

	call, err := client.Call(context.Background(), target)
	if err != nil {
		return ctx.ReplyWithMentions(whatsrook.Sprintf("Couldn't call %s: %v", userTag, err), []types.JID{mentionJID})
	}

	// Drain incoming audio frames
	call.Receive(whatsmeow.SinkFunc(func(pcm []float32) {}))

	duration, durErr := AudioDuration(audioPath)
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

	call.OnPeerAccept(startMedia)
	call.OnReady(startMedia)

	call.OnEnd(func(reason string) {
		if err := ctx.ReplyWithMentions(whatsrook.Sprintf("Call with %s ended — %s.", userTag, friendlyCallEndReason(reason)), []types.JID{mentionJID}); err != nil {
			logHandlerErr("call", err)
		}
	})

	return ctx.ReplyWithMentions(whatsrook.Sprintf("Calling %s…", userTag), []types.JID{mentionJID})
}

// friendlyCallEndReason translates whatsmeow's raw call-end reason code
// into plain, conversational text suitable for a chat reply. Unrecognized
// reasons fall back to a generic phrase so an unmapped code never leaks
// a raw internal string into the reply.
func friendlyCallEndReason(reason string) string {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "hangup", "hang_up", "user_hangup", "cancel", "cancelled", "canceled":
		return "call was hung up"
	case "reject", "rejected", "decline", "declined":
		return "call was declined"
	case "timeout", "timed_out", "no_answer", "no-answer", "missed":
		return "no one answered"
	case "busy":
		return "the line was busy"
	case "offline":
		return "the other side is offline"
	case "failed", "error", "connection_error", "connectivity_error":
		return "the connection failed"
	case "":
		return "the call ended"
	default:
		return "call ended (" + reason + ")"
	}
}

func placeVideoCallWithMedia(ctx *dispatch.Context, target, videoPath string) error {
	client := ctx.Client
	targetJID := resolveTargetJID(target)
	userTag, mentionJID := ctx.FormatMention(targetJID)

	call, err := client.CallWithOptions(context.Background(), target, whatsmeow.CallOptions{Video: true})
	if err != nil {
		return ctx.ReplyWithMentions(whatsrook.Sprintf("Couldn't start a video call with %s: %v", userTag, err), []types.JID{mentionJID})
	}

	call.Receive(whatsmeow.SinkFunc(func(pcm []float32) {}))
	call.ReceiveVideo(whatsmeow.VideoSinkFunc(func(accessUnit []byte) {}))

	var requestKeyframe atomic.Bool
	requestKeyframe.Store(true)

	var startOnce sync.Once
	startMedia := func() {
		startOnce.Do(func() {
			logger.Debug("videocall: starting media playback", "state", call.State(), "video_path", videoPath)
			_ = call.SetVideoEnabled(true)

			if videoPath == "" {
				return
			}

			mp3Path, h264Path, prepErr := PrepareCallVideo(videoPath)
			if prepErr != nil {
				logHandlerErr("videocall", fmt.Errorf("failed to prepare call video: %w", prepErr))
			}
			logger.Debug("videocall: prep done", "mp3", mp3Path, "h264", h264Path, "err", prepErr)

			audioFile := mp3Path
			if audioFile == "" {
				audioFile = videoPath
			}

			duration, durErr := AudioDuration(audioFile)
			if durErr != nil || duration == 0 {
				duration, durErr = AudioDuration(videoPath)
			}
			if durErr != nil || duration == 0 {
				duration = 30 * time.Second
			}
			logger.Debug("videocall: media duration", "duration", duration)

			if src, err := openAudioSource(audioFile); err == nil {
				logger.Debug("videocall: audio source opened, starting playback", "audio_file", audioFile)
				call.Play(src)
			} else {
				logger.Debug("videocall: could not open audio source", "audio_file", audioFile, "err", err)
			}

			if h264Path == "" {
				return
			}
			h264Data, readErr := os.ReadFile(h264Path)
			if readErr != nil {
				logger.Debug("videocall: failed to read h264 file", "h264_path", h264Path, "err", readErr)
				return
			}
			if len(h264Data) == 0 {
				return
			}

			frames := SplitAnnexBAccessUnits(h264Data)
			logger.Debug("videocall: split h264 into access units", "access_units", len(frames), "bytes", len(h264Data))
			if len(frames) == 0 {
				return
			}

			go runVideoFrameLoop(call, frames, duration, &requestKeyframe)
		})
	}

	call.OnPeerAccept(func() {
		logger.Debug("videocall: peer accepted, queuing immediate IDR keyframe")
		requestKeyframe.Store(true)
		startMedia()
	})

	call.OnVideoKeyframeRequest(func() {
		logger.Debug("videocall: keyframe requested by peer PLI/FIR, queuing IDR keyframe")
		requestKeyframe.Store(true)
	})

	call.OnReady(func() {
		logger.Debug("videocall: media ready (inbound RTP flowing)")
		startMedia()
	})

	call.OnEnd(func(reason string) {
		if err := ctx.ReplyWithMentions(whatsrook.Sprintf("Video call with %s ended — %s.", userTag, friendlyCallEndReason(reason)), []types.JID{mentionJID}); err != nil {
			logHandlerErr("videocall", err)
		}
	})

	return ctx.ReplyWithMentions(whatsrook.Sprintf("Video calling %s…", userTag), []types.JID{mentionJID})
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
