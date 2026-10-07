// Package sdk provides the Go SDK for building WhatsRook external plugins.
//
// External plugins are standalone executables that WhatsRook spawns as child processes.
// Communication is conducted through standard I/O: WhatsRook writes a JSON request payload
// to standard input, and the plugin streams newline-delimited JSON action frames to standard output.
package sdk

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultUserAgent is the default browser User-Agent used by CreateHTTPClient.
const DefaultUserAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"

// QuotedMessage represents the context of a quoted message replied to by the user.
type QuotedMessage struct {
	ID     string `json:"id,omitempty"`
	Sender string `json:"sender,omitempty"`
	Text   string `json:"text,omitempty"`
}

// MediaPayload represents media attached to or quoted in the triggering message.
type MediaPayload struct {
	Path     string `json:"path,omitempty"`
	MimeType string `json:"mimetype,omitempty"`
	IsQuoted bool   `json:"is_quoted,omitempty"`
}

// Request is the incoming context payload written by WhatsRook to standard input.
type Request struct {
	Command         string         `json:"command"`
	Args            []string       `json:"args,omitempty"`
	RawArgs         string         `json:"raw_args,omitempty"`
	Chat            string         `json:"chat"`
	Sender          string         `json:"sender"`
	Prefix          string         `json:"prefix"`
	BotName         string         `json:"bot_name"`
	PushName        string         `json:"push_name,omitempty"`
	IsGroup         bool           `json:"is_group"`
	IsSudo          bool           `json:"is_sudo"`
	IsOwner         bool           `json:"is_owner"`
	IsAdmin         bool           `json:"is_admin,omitempty"`
	IsLiveSession   bool           `json:"live_session,omitempty"`
	IsCancelRequest bool           `json:"is_cancel_request,omitempty"`
	QuotedMessage   *QuotedMessage `json:"quoted_message,omitempty"`
	MentionedJIDs   []string       `json:"mentioned_jids,omitempty"`
	Media           *MediaPayload  `json:"media,omitempty"`
	StickerPack     string         `json:"sticker_pack,omitempty"`
	StickerAuthor   string         `json:"sticker_author,omitempty"`
}

var (
	stdinReader     *bufio.Reader
	stdinReaderOnce sync.Once
)

func getStdinReader() *bufio.Reader {
	stdinReaderOnce.Do(func() {
		stdinReader = bufio.NewReaderSize(os.Stdin, 64*1024)
	})
	return stdinReader
}

// isTerminal checks if a file descriptor is an interactive terminal.
func isTerminal(f *os.File) bool {
	stat, err := f.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

// Load reads the plugin request from stdin (first line as JSON).
// When stdin is a terminal or parsing fails, it falls back to a minimal Request constructed from os.Args.
func Load() *Request {
	if !isTerminal(os.Stdin) {
		reader := getStdinReader()
		line, err := reader.ReadString('\n')
		if err == nil || (errors.Is(err, io.EOF) && len(line) > 0) {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" {
				var req Request
				if err := json.Unmarshal([]byte(trimmed), &req); err == nil {
					if req.Command != "" || len(req.Args) > 0 || req.RawArgs != "" || req.Chat != "" {
						return &req
					}
				}
			}
		}
	}

	// CLI fallback for local development / testing
	cmdName := "plugin"
	if len(os.Args) > 0 {
		cmdName = filepath.Base(os.Args[0])
	}
	var args []string
	if len(os.Args) > 1 {
		args = os.Args[1:]
	}
	return &Request{
		Command:  cmdName,
		Args:     args,
		RawArgs:  strings.Join(args, " "),
		Prefix:   ".",
		BotName:  "WhatsRook",
		PushName: "User",
	}
}

// LoadStreaming is an alias for Load to make streaming plugin intent explicit.
func LoadStreaming() *Request {
	return Load()
}

// Query returns the trimmed raw arguments, falling back to joining args with spaces.
func (r *Request) Query() string {
	trimmed := strings.TrimSpace(r.RawArgs)
	if trimmed != "" {
		return trimmed
	}
	return strings.TrimSpace(strings.Join(r.Args, " "))
}

// EffectivePrefix returns the effective command prefix, falling back to ".".
func (r *Request) EffectivePrefix() string {
	if r.Prefix == "" {
		return "."
	}
	return r.Prefix
}

// EffectiveBotName returns the configured bot display name, falling back to "WhatsRook".
func (r *Request) EffectiveBotName() string {
	if r.BotName == "" {
		return "WhatsRook"
	}
	return r.BotName
}

// EffectivePushName returns the sender's push display name, falling back to "User".
func (r *Request) EffectivePushName() string {
	if r.PushName == "" {
		return "User"
	}
	return r.PushName
}

// QuotedText returns the text of the quoted message if present.
func (r *Request) QuotedText() string {
	if r.QuotedMessage != nil {
		return r.QuotedMessage.Text
	}
	return ""
}

// QuotedSender returns the sender JID of the quoted message if present.
func (r *Request) QuotedSender() string {
	if r.QuotedMessage != nil {
		return r.QuotedMessage.Sender
	}
	return ""
}

// QuotedID returns the message ID of the quoted message if present.
func (r *Request) QuotedID() string {
	if r.QuotedMessage != nil {
		return r.QuotedMessage.ID
	}
	return ""
}

// Arg returns the argument at 0-based index or empty string if out of range.
func (r *Request) Arg(index int) string {
	if index >= 0 && index < len(r.Args) {
		return r.Args[index]
	}
	return ""
}

// ArgAsInt parses the argument at index into an integer.
func (r *Request) ArgAsInt(index int) (int, error) {
	val := r.Arg(index)
	if val == "" {
		return 0, fmt.Errorf("arg at index %d is empty", index)
	}
	return strconv.Atoi(val)
}

// Subcommand returns the first argument.
func (r *Request) Subcommand() string {
	return r.Arg(0)
}

// SubcommandArgs returns all arguments following the first argument.
func (r *Request) SubcommandArgs() []string {
	if len(r.Args) <= 1 {
		return nil
	}
	return r.Args[1:]
}

// Flag checks whether a boolean flag (--name or -name) is present.
func (r *Request) Flag(name string) bool {
	long := "--" + name
	short := "-" + name
	for _, a := range r.Args {
		if a == long || a == short {
			return true
		}
	}
	return false
}

// FlagValue extracts the value for a flag (--name=val, --name val, -name=val, -name val).
func (r *Request) FlagValue(name string) string {
	longPrefix := "--" + name + "="
	shortPrefix := "-" + name + "="
	long := "--" + name
	short := "-" + name

	for i, a := range r.Args {
		if after, ok := strings.CutPrefix(a, longPrefix); ok {
			return after
		}
		if after, ok := strings.CutPrefix(a, shortPrefix); ok {
			return after
		}
		if (a == long || a == short) && i+1 < len(r.Args) {
			return r.Args[i+1]
		}
	}
	return ""
}

// IsDM returns true if invoked in a direct private chat.
func (r *Request) IsDM() bool {
	return !r.IsGroup
}

// IsLive returns true if an active live session exists.
func (r *Request) IsLive() bool {
	return r.IsLiveSession
}

// IsCancel returns true if cancellation of an active live session was requested.
func (r *Request) IsCancel() bool {
	return r.IsCancelRequest
}

// ChatID returns the chat JID.
func (r *Request) ChatID() string {
	return r.Chat
}

// SenderID returns the sender JID.
func (r *Request) SenderID() string {
	return r.Sender
}

// SenderPhone returns the phone number or user ID before the @ in the sender JID.
func (r *Request) SenderPhone() string {
	if r.Sender == "" {
		return ""
	}
	parts := strings.Split(r.Sender, "@")
	return parts[0]
}

// ChatTarget returns the target identifier before the @ in the chat JID.
func (r *Request) ChatTarget() string {
	if r.Chat == "" {
		return ""
	}
	parts := strings.Split(r.Chat, "@")
	return parts[0]
}

// Action represents a single JSON action frame sent by the plugin to WhatsRook via stdout.
type Action struct {
	Action      string   `json:"action"`                 // reply, edit, react, delete, send_image, send_audio, send_video, send_document, send_sticker, poll, loader, done
	Text        string   `json:"text,omitempty"`         // reply, edit, loader text
	MsgID       string   `json:"msg_id,omitempty"`       // edit, react, delete target ID
	Emoji       string   `json:"emoji,omitempty"`        // react emoji
	Data        string   `json:"data,omitempty"`         // base64 encoded media or URL
	MimeType    string   `json:"mimetype,omitempty"`     // media MIME type override
	Caption     string   `json:"caption,omitempty"`      // image, video, document caption
	Filename    string   `json:"filename,omitempty"`     // document filename
	Ptt         bool     `json:"ptt,omitempty"`          // voice note flag for audio
	GifPlayback bool     `json:"gif_playback,omitempty"` // looping GIF playback for video
	Question    string   `json:"question,omitempty"`     // poll question
	Options     []string `json:"options,omitempty"`      // poll options
	Selectable  int      `json:"selectable,omitempty"`   // max selectable count for poll
	Mentions    []string `json:"mentions,omitempty"`     // user JIDs to mention
}

// NewReplyAction creates a text reply action frame.
func NewReplyAction(text string) *Action {
	return &Action{Action: "reply", Text: text}
}

// NewEditAction creates an edit action frame targeting an existing message ID.
func NewEditAction(msgID, text string) *Action {
	return &Action{Action: "edit", MsgID: msgID, Text: text}
}

// NewReactAction creates a reaction action frame on the triggering message.
func NewReactAction(emoji string) *Action {
	return &Action{Action: "react", Emoji: emoji}
}

// NewReactToAction creates a reaction action frame targeting a specific message ID.
func NewReactToAction(msgID, emoji string) *Action {
	return &Action{Action: "react", MsgID: msgID, Emoji: emoji}
}

// NewDeleteAction creates a delete action frame to revoke a message.
func NewDeleteAction(msgID string) *Action {
	return &Action{Action: "delete", MsgID: msgID}
}

// NewImageAction creates an image action frame.
func NewImageAction(dataOrURL string) *Action {
	return &Action{Action: "send_image", Data: dataOrURL}
}

// NewAudioAction creates an audio action frame.
func NewAudioAction(dataOrURL string) *Action {
	return &Action{Action: "send_audio", Data: dataOrURL}
}

// NewVideoAction creates a video action frame.
func NewVideoAction(dataOrURL string) *Action {
	return &Action{Action: "send_video", Data: dataOrURL}
}

// NewDocumentAction creates a document attachment action frame.
func NewDocumentAction(dataOrURL, filename string) *Action {
	return &Action{Action: "send_document", Data: dataOrURL, Filename: filename}
}

// NewStickerAction creates a WebP sticker action frame.
func NewStickerAction(dataOrURL string) *Action {
	return &Action{Action: "send_sticker", Data: dataOrURL}
}

// NewPollAction creates a single-select interactive poll action frame.
func NewPollAction(question string, options []string) *Action {
	return &Action{Action: "poll", Question: question, Options: options, Selectable: 1}
}

// NewLoaderAction creates a loader indicator action frame.
func NewLoaderAction(text string) *Action {
	return &Action{Action: "loader", Text: text}
}

// NewDoneAction creates a done action frame signaling session completion.
func NewDoneAction() *Action {
	return &Action{Action: "done"}
}

// WithCaption sets the caption on an image, video, or document action.
func (a *Action) WithCaption(caption string) *Action {
	a.Caption = caption
	return a
}

// WithMimeType sets the MIME type override on media actions.
func (a *Action) WithMimeType(mimetype string) *Action {
	a.MimeType = mimetype
	return a
}

// AsPTT sets the push-to-talk voice note flag for audio actions.
func (a *Action) AsPTT(isPTT bool) *Action {
	a.Ptt = isPTT
	return a
}

// AsGIF sets looping GIF playback mode for video actions.
func (a *Action) AsGIF(isGIF bool) *Action {
	a.GifPlayback = isGIF
	return a
}

// WithSelectable configures the number of selectable options for a poll.
func (a *Action) WithSelectable(count int) *Action {
	a.Selectable = count
	return a
}

// Send immediately writes this action as a JSON line to stdout and flushes.
func (a *Action) Send() error {
	return SendAction(a)
}

// Ack represents an acknowledgement frame returned by WhatsRook on stdin.
type Ack struct {
	OK    bool   `json:"ok"`
	MsgID string `json:"msg_id,omitempty"`
	Error string `json:"error,omitempty"`
}

var stdoutMu sync.Mutex

// SendAction serializes an action frame to stdout and flushes immediately.
func SendAction(action *Action) error {
	stdoutMu.Lock()
	defer stdoutMu.Unlock()

	data, err := json.Marshal(action)
	if err != nil {
		return fmt.Errorf("marshal action: %w", err)
	}
	if _, err := os.Stdout.Write(append(data, '\n')); err != nil {
		return err
	}
	return os.Stdout.Sync()
}

// AwaitAck reads one Ack line from standard input.
func AwaitAck() (*Ack, error) {
	reader := getStdinReader()
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return nil, errors.New("empty ack received")
	}
	var ack Ack
	if err := json.Unmarshal([]byte(trimmed), &ack); err != nil {
		return nil, fmt.Errorf("parse ack: %w", err)
	}
	return &ack, nil
}

// SendReplyLive sends a text reply action, awaits acknowledgment, and returns the message ID.
func SendReplyLive(text string) (string, error) {
	if err := SendAction(NewReplyAction(text)); err != nil {
		return "", err
	}
	ack, err := AwaitAck()
	if err != nil {
		return "", err
	}
	if !ack.OK {
		return "", fmt.Errorf("reply rejected: %s", ack.Error)
	}
	return ack.MsgID, nil
}

// SendEditLive updates an existing message in-place using its message ID.
func SendEditLive(msgID, text string) error {
	return SendAction(NewEditAction(msgID, text))
}

// SendReact sends an emoji reaction to the triggering message.
func SendReact(emoji string) error {
	return SendAction(NewReactAction(emoji))
}

// SendReactTo sends an emoji reaction to a specific message ID.
func SendReactTo(msgID, emoji string) error {
	return SendAction(NewReactToAction(msgID, emoji))
}

// SendDelete revokes a message for everyone by its ID.
func SendDelete(msgID string) error {
	return SendAction(NewDeleteAction(msgID))
}

// SendImage sends an image from a URL or base64 data with an optional caption.
func SendImage(dataOrURL, caption string) error {
	action := NewImageAction(dataOrURL)
	if caption != "" {
		action.WithCaption(caption)
	}
	return SendAction(action)
}

// SendImageWithType sends an image with caption and MIME type override.
func SendImageWithType(dataOrURL, caption, mimetype string) error {
	action := NewImageAction(dataOrURL).WithCaption(caption).WithMimeType(mimetype)
	return SendAction(action)
}

// SendAudio sends audio or a voice note from a URL or base64 data.
func SendAudio(dataOrURL string, isPTT bool) error {
	action := NewAudioAction(dataOrURL).AsPTT(isPTT)
	return SendAction(action)
}

// SendAudioFull sends audio with explicit MIME type and PTT setting.
func SendAudioFull(dataOrURL string, isPTT bool, mimetype string) error {
	action := NewAudioAction(dataOrURL).AsPTT(isPTT).WithMimeType(mimetype)
	return SendAction(action)
}

// SendVideo sends a video from a URL or base64 data with an optional caption.
func SendVideo(dataOrURL, caption string) error {
	action := NewVideoAction(dataOrURL)
	if caption != "" {
		action.WithCaption(caption)
	}
	return SendAction(action)
}

// SendGIF sends a looping GIF from a URL or base64 data.
func SendGIF(dataOrURL, caption string) error {
	action := NewVideoAction(dataOrURL).AsGIF(true)
	if caption != "" {
		action.WithCaption(caption)
	}
	return SendAction(action)
}

// SendVideoFull sends a video with caption, MIME type override, and GIF playback option.
func SendVideoFull(dataOrURL, caption, mimetype string, isGIF bool) error {
	action := NewVideoAction(dataOrURL).WithCaption(caption).WithMimeType(mimetype).AsGIF(isGIF)
	return SendAction(action)
}

// SendDocument sends a document attachment from a URL or base64 data.
func SendDocument(dataOrURL, filename, caption string) error {
	action := NewDocumentAction(dataOrURL, filename)
	if caption != "" {
		action.WithCaption(caption)
	}
	return SendAction(action)
}

// SendDocumentFull sends a document with MIME type override.
func SendDocumentFull(dataOrURL, filename, caption, mimetype string) error {
	action := NewDocumentAction(dataOrURL, filename).WithCaption(caption).WithMimeType(mimetype)
	return SendAction(action)
}

// SendSticker sends a WebP sticker from a URL or base64 data.
func SendSticker(dataOrURL string) error {
	return SendAction(NewStickerAction(dataOrURL))
}

// SendPoll sends a single-select interactive poll.
func SendPoll(question string, options []string) error {
	return SendAction(NewPollAction(question, options))
}

// SendMultiPoll sends a multi-choice poll with selectable limit.
func SendMultiPoll(question string, options []string, selectable int) error {
	return SendAction(NewPollAction(question, options).WithSelectable(selectable))
}

// SendLoader displays a typing or processing indicator with optional status text.
func SendLoader(text string) error {
	return SendAction(NewLoaderAction(text))
}

// SendDone signals completion of a live session to WhatsRook.
func SendDone() error {
	return SendAction(NewDoneAction())
}

// Respond writes a simple plain-text response to stdout without JSON framing.
func Respond(output string) {
	fmt.Print(strings.TrimSpace(output))
}

// RespondErr prints an error to stderr and stdout, then terminates the process with code 1.
func RespondErr(errMsg string) {
	trimmed := strings.TrimSpace(errMsg)
	fmt.Fprintln(os.Stderr, trimmed)
	fmt.Print(trimmed)
	os.Exit(1)
}

// CreateHTTPClient constructs an http.Client preconfigured with timeout and browser User-Agent header.
func CreateHTTPClient(timeoutSecs int) *http.Client {
	timeout := time.Duration(timeoutSecs) * time.Second
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &http.Client{
		Timeout: timeout,
		Transport: &userAgentTransport{
			base:      http.DefaultTransport,
			userAgent: DefaultUserAgent,
		},
	}
}

type userAgentTransport struct {
	base      http.RoundTripper
	userAgent string
}

func (t *userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", t.userAgent)
	}
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}
