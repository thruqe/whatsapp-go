package builder

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"
	"time"

	Logger "whatsrook/logger"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// PollRequest carries the data of a decrypted poll vote.
type PollRequest struct {
	PollMsgID       types.MessageID
	SelectedOptions []string
	Sender          types.JID
	Chat            types.JID
	Ctx             context.Context
}

type pollRoute struct {
	chat           types.JID
	client         *whatsmeow.Client
	pollMsgID      types.MessageID
	precedingMsgID types.MessageID
	options        []string
	allowedSenders []types.JID
	once           bool
	autoDelete     bool
	timer          *time.Timer
	fn             func(req PollRequest, res *Response)
}

var (
	pollRoutesMu sync.RWMutex
	pollRoutes   = make(map[types.MessageID]pollRoute)
)

// PollRouteConfig configures a reactive route for a poll message.
type PollRouteConfig struct {
	PollMsgID      types.MessageID
	PrecedingMsgID types.MessageID
	Chat           types.JID
	Client         *whatsmeow.Client
	Options        []string
	AllowedSenders []types.JID
	Once           bool
	AutoDelete     bool
	Timeout        time.Duration
	Fn             func(req PollRequest, res *Response)
}

// RegisterPollRoute registers a reactive route with full lifecycle, timeout, and auto-delete management.
func RegisterPollRoute(cfg PollRouteConfig) {
	if cfg.Timeout <= 0 && cfg.AutoDelete {
		cfg.Timeout = DefaultPollTimeout
	}

	var timer *time.Timer
	if cfg.AutoDelete && cfg.Timeout > 0 {
		timer = time.AfterFunc(cfg.Timeout, func() {
			pollRoutesMu.Lock()
			r, exists := pollRoutes[cfg.PollMsgID]
			if exists {
				delete(pollRoutes, cfg.PollMsgID)
			}
			remaining := len(pollRoutes)
			pollRoutesMu.Unlock()

			if exists {
				Logger.Debug("WARook: poll expired after timeout and auto-deleted",
					"pollMsgID", cfg.PollMsgID,
					"chat", cfg.Chat.String(),
					"timeout", cfg.Timeout,
					"remainingActivePollRoutes", remaining,
				)
				client := cfg.Client
				if client == nil && r.client != nil {
					client = r.client
				}
				chat := cfg.Chat
				if chat.IsEmpty() && !r.chat.IsEmpty() {
					chat = r.chat
				}
				if client != nil && !chat.IsEmpty() {
					go func(cli *whatsmeow.Client, ch types.JID, pID, preID types.MessageID) {
						revokeMsg := cli.BuildRevoke(ch, types.EmptyJID, pID)
						_, _ = cli.SendMessage(context.Background(), ch, revokeMsg)
						if preID != "" {
							preRevoke := cli.BuildRevoke(ch, types.EmptyJID, preID)
							_, _ = cli.SendMessage(context.Background(), ch, preRevoke)
						}
						Logger.Debug("WARook: auto-deleted expired poll and preceding messages", "pollMsgID", pID, "precedingMsgID", preID)
					}(client, chat, cfg.PollMsgID, cfg.PrecedingMsgID)
				}
			}
		})
	}

	pollRoutesMu.Lock()
	pollRoutes[cfg.PollMsgID] = pollRoute{
		chat:           cfg.Chat,
		client:         cfg.Client,
		pollMsgID:      cfg.PollMsgID,
		precedingMsgID: cfg.PrecedingMsgID,
		options:        cfg.Options,
		allowedSenders: cfg.AllowedSenders,
		once:           cfg.Once,
		autoDelete:     cfg.AutoDelete,
		timer:          timer,
		fn:             cfg.Fn,
	}
	total := len(pollRoutes)
	pollRoutesMu.Unlock()

	Logger.Debug("WARook: registered poll vote handler",
		"pollMsgID", cfg.PollMsgID,
		"precedingMsgID", cfg.PrecedingMsgID,
		"optionsCount", len(cfg.Options),
		"options", cfg.Options,
		"once", cfg.Once,
		"autoDelete", cfg.AutoDelete,
		"timeout", cfg.Timeout,
		"totalActivePollRoutes", total,
	)
}

// RegisterPollHandler registers a reactive handler for votes on a specific poll message.
func RegisterPollHandler(pollMsgID types.MessageID, options []string, once bool, fn func(req PollRequest, res *Response)) {
	RegisterPollRoute(PollRouteConfig{
		PollMsgID:  pollMsgID,
		Options:    options,
		Once:       once,
		AutoDelete: true,
		Timeout:    DefaultPollTimeout,
		Fn:         fn,
	})
}

// DeregisterPollHandler removes the registered handler for a poll message and cancels any pending timeout.
func DeregisterPollHandler(pollMsgID types.MessageID) {
	pollRoutesMu.Lock()
	if r, ok := pollRoutes[pollMsgID]; ok {
		if r.timer != nil {
			r.timer.Stop()
		}
		delete(pollRoutes, pollMsgID)
	}
	total := len(pollRoutes)
	pollRoutesMu.Unlock()
	Logger.Debug("WARook: deregistered poll vote handler",
		"pollMsgID", pollMsgID,
		"totalActivePollRoutes", total,
	)
}

// PollBuilder builds and sends a WhatsApp poll (with optional quoted reply)
// and registers a reactive callback for votes with automatic expiration and deletion.
type PollBuilder struct {
	rook           *WARook
	question       string
	options        []string
	single         bool // true = single-choice, false = multi-choice
	mentions       []types.JID
	allowedSenders []types.JID
	asReply        bool
	autoDelete     bool          // auto-delete poll message upon vote or timeout (default: true)
	timeout        time.Duration // timeout duration before auto-delete (default: 25s)
}

// NewPoll initializes a new PollBuilder for the given question (single-choice & 25s auto-delete by default).
func NewPoll(rook *WARook, question string) *PollBuilder {
	Logger.Debug("PollBuilder: initialized", "question", question, "single", true, "timeout", DefaultPollTimeout)
	return &PollBuilder{
		rook:       rook,
		question:   question,
		single:     true,
		autoDelete: true,
		timeout:    DefaultPollTimeout,
	}
}

// Question sets or updates the question text of the poll.
func (p *PollBuilder) Question(question string) *PollBuilder {
	Logger.Debug("PollBuilder: updated question", "prevQuestion", p.question, "newQuestion", question)
	p.question = question
	return p
}

// AddOption appends a poll option.
func (p *PollBuilder) AddOption(option string) *PollBuilder {
	opt := strings.TrimSpace(option)
	if opt != "" {
		p.options = append(p.options, opt)
		Logger.Debug("PollBuilder: option added", "option", opt, "totalOptions", len(p.options))
	}
	return p
}

// AddOptions appends multiple poll options.
func (p *PollBuilder) AddOptions(options ...string) *PollBuilder {
	for _, opt := range options {
		p.AddOption(opt)
	}
	return p
}

// SingleChoice restricts the poll to one selectable answer (default).
func (p *PollBuilder) SingleChoice() *PollBuilder {
	Logger.Debug("PollBuilder: choice mode configured", "single", true)
	p.single = true
	return p
}

// MultiChoice allows multiple answers to be selected simultaneously.
func (p *PollBuilder) MultiChoice() *PollBuilder {
	Logger.Debug("PollBuilder: choice mode configured", "single", false)
	p.single = false
	return p
}

// Mentions attaches JIDs to be mentioned in the poll message.
func (p *PollBuilder) Mentions(jids ...types.JID) *PollBuilder {
	for _, j := range jids {
		if !j.IsEmpty() {
			p.mentions = append(p.mentions, j)
		}
	}
	Logger.Debug("PollBuilder: mentions attached", "mentionsCount", len(p.mentions))
	return p
}

// AsReply configures the poll to be sent quoting the triggering message.
func (p *PollBuilder) AsReply() *PollBuilder {
	Logger.Debug("PollBuilder: configured as quoted reply")
	p.asReply = true
	return p
}

// AutoDelete configures whether the poll message should automatically be revoked/deleted
// after being voted on or timing out (default: true).
func (p *PollBuilder) AutoDelete(enable bool) *PollBuilder {
	Logger.Debug("PollBuilder: autoDelete configured", "enable", enable)
	p.autoDelete = enable
	return p
}

// Timeout sets the expiration timeout after which an unvoted poll is automatically deleted (default: 25s).
// Setting timeout <= 0 disables timeout auto-deletion.
func (p *PollBuilder) Timeout(d time.Duration) *PollBuilder {
	Logger.Debug("PollBuilder: timeout configured", "timeout", d)
	p.timeout = d
	if d <= 0 {
		p.autoDelete = false
	}
	return p
}

// AllowedSenders explicitly restricts poll votes to the specified user JIDs.
func (p *PollBuilder) AllowedSenders(jids ...types.JID) *PollBuilder {
	for _, j := range jids {
		if !j.IsEmpty() {
			p.allowedSenders = append(p.allowedSenders, j)
		}
	}
	Logger.Debug("PollBuilder: allowedSenders attached", "allowedCount", len(p.allowedSenders))
	return p
}

func (p *PollBuilder) getAllowedSenders() []types.JID {
	if len(p.allowedSenders) > 0 {
		return p.allowedSenders
	}
	if len(p.mentions) > 0 {
		return p.mentions
	}
	return nil
}

// Send sends the poll to the given JID and optionally registers fn to receive votes.
func (p *PollBuilder) Send(to types.JID, fn ...func(req PollRequest, res *Response)) error {
	_, err := p.SendWithID(to, fn...)
	return err
}

// SendWithID sends the poll to the given JID, returns its MessageID, and optionally registers fn to receive votes.
func (p *PollBuilder) SendWithID(to types.JID, fn ...func(req PollRequest, res *Response)) (types.MessageID, error) {
	hasCallback := len(fn) > 0 && fn[0] != nil
	Logger.Debug("PollBuilder: SendWithID called", "to", to.String(), "hasCallback", hasCallback, "asReply", p.asReply)
	pollMsgID, precedingMsgID, err := p.sendMsgWithID(to, p.asReply)
	if err != nil {
		Logger.Error("PollBuilder: SendWithID failed", "to", to.String(), "err", err)
		return "", err
	}
	var cb func(req PollRequest, res *Response)
	if hasCallback {
		cb = fn[0]
	}
	var cli *whatsmeow.Client
	if p.rook != nil && p.rook.sender != nil {
		cli = p.rook.sender.GetClient()
	}
	RegisterPollRoute(PollRouteConfig{
		PollMsgID:      pollMsgID,
		PrecedingMsgID: precedingMsgID,
		Chat:           to,
		Client:         cli,
		Options:        p.options,
		AllowedSenders: p.getAllowedSenders(),
		Once:           false,
		AutoDelete:     p.autoDelete,
		Timeout:        p.timeout,
		Fn:             cb,
	})
	return pollMsgID, nil
}

// Reply sends the poll as a reply to the current event and optionally registers fn to receive votes.
func (p *PollBuilder) Reply(fn ...func(req PollRequest, res *Response)) error {
	_, err := p.ReplyWithID(fn...)
	return err
}

// ReplyWithID sends the poll as a reply, returns its MessageID, and optionally registers fn to receive votes.
func (p *PollBuilder) ReplyWithID(fn ...func(req PollRequest, res *Response)) (types.MessageID, error) {
	if p.rook == nil || p.rook.sender == nil {
		return "", fmt.Errorf("PollBuilder: ReplyWithID called without Sender")
	}
	chat := p.rook.sender.GetChat()
	hasCallback := len(fn) > 0 && fn[0] != nil
	Logger.Debug("PollBuilder: ReplyWithID called", "chat", chat.String(), "hasCallback", hasCallback)
	pollMsgID, precedingMsgID, err := p.sendMsgWithID(chat, true)
	if err != nil {
		Logger.Error("PollBuilder: ReplyWithID failed", "chat", chat.String(), "err", err)
		return "", err
	}
	var cb func(req PollRequest, res *Response)
	if hasCallback {
		cb = fn[0]
	}
	RegisterPollRoute(PollRouteConfig{
		PollMsgID:      pollMsgID,
		PrecedingMsgID: precedingMsgID,
		Chat:           chat,
		Client:         p.rook.sender.GetClient(),
		Options:        p.options,
		AllowedSenders: p.getAllowedSenders(),
		Once:           false,
		AutoDelete:     p.autoDelete,
		Timeout:        p.timeout,
		Fn:             cb,
	})
	return pollMsgID, nil
}

// Once sends the poll to the given JID and registers a one-shot handler (auto-deregisters after first vote).
func (p *PollBuilder) Once(to types.JID, fn ...func(req PollRequest, res *Response)) error {
	_, err := p.OnceWithID(to, fn...)
	return err
}

// OnceWithID sends the poll to the given JID, returns its MessageID, and registers a one-shot handler.
func (p *PollBuilder) OnceWithID(to types.JID, fn ...func(req PollRequest, res *Response)) (types.MessageID, error) {
	hasCallback := len(fn) > 0 && fn[0] != nil
	Logger.Debug("PollBuilder: OnceWithID called", "to", to.String(), "hasCallback", hasCallback, "asReply", p.asReply)
	pollMsgID, precedingMsgID, err := p.sendMsgWithID(to, p.asReply)
	if err != nil {
		Logger.Error("PollBuilder: OnceWithID failed", "to", to.String(), "err", err)
		return "", err
	}
	var cb func(req PollRequest, res *Response)
	if hasCallback {
		cb = fn[0]
	}
	var cli *whatsmeow.Client
	if p.rook != nil && p.rook.sender != nil {
		cli = p.rook.sender.GetClient()
	}
	RegisterPollRoute(PollRouteConfig{
		PollMsgID:      pollMsgID,
		PrecedingMsgID: precedingMsgID,
		Chat:           to,
		Client:         cli,
		Options:        p.options,
		AllowedSenders: p.getAllowedSenders(),
		Once:           true,
		AutoDelete:     p.autoDelete,
		Timeout:        p.timeout,
		Fn:             cb,
	})
	return pollMsgID, nil
}

// OnceReply sends the poll as a reply and registers a one-shot handler (auto-deregisters after first vote).
func (p *PollBuilder) OnceReply(fn ...func(req PollRequest, res *Response)) error {
	_, err := p.OnceReplyWithID(fn...)
	return err
}

// OnceReplyWithID sends the poll as a reply, returns its MessageID, and registers a one-shot handler.
func (p *PollBuilder) OnceReplyWithID(fn ...func(req PollRequest, res *Response)) (types.MessageID, error) {
	if p.rook == nil || p.rook.sender == nil {
		return "", fmt.Errorf("PollBuilder: OnceReplyWithID called without Sender")
	}
	chat := p.rook.sender.GetChat()
	hasCallback := len(fn) > 0 && fn[0] != nil
	Logger.Debug("PollBuilder: OnceReplyWithID called", "chat", chat.String(), "hasCallback", hasCallback)
	pollMsgID, precedingMsgID, err := p.sendMsgWithID(chat, true)
	if err != nil {
		Logger.Error("PollBuilder: OnceReplyWithID failed", "chat", chat.String(), "err", err)
		return "", err
	}
	var cb func(req PollRequest, res *Response)
	if hasCallback {
		cb = fn[0]
	}
	RegisterPollRoute(PollRouteConfig{
		PollMsgID:      pollMsgID,
		PrecedingMsgID: precedingMsgID,
		Chat:           chat,
		Client:         p.rook.sender.GetClient(),
		Options:        p.options,
		AllowedSenders: p.getAllowedSenders(),
		Once:           true,
		AutoDelete:     p.autoDelete,
		Timeout:        p.timeout,
		Fn:             cb,
	})
	return pollMsgID, nil
}

func (p *PollBuilder) sendMsgWithID(to types.JID, asReply bool) (pollMsgID types.MessageID, precedingMsgID types.MessageID, err error) {
	if p.rook == nil || p.rook.sender == nil {
		return "", "", fmt.Errorf("PollBuilder: sender context is nil")
	}
	sender := p.rook.sender
	sender.StopAutoLoader()

	cli := sender.GetClient()
	if cli == nil {
		return "", "", fmt.Errorf("PollBuilder: raw whatsmeow client is nil")
	}

	selectableCount := 1
	if !p.single {
		selectableCount = 0
	}

	q := strings.TrimSpace(p.question)
	// If the question is long (> 250 chars), send descriptive body first then clean prompt
	if len(q) > 250 {
		fullText := q
		Logger.Debug("PollBuilder: question exceeds 250 characters, transmitting preceding text body",
			"fullLength", len(fullText),
			"to", to.String(),
			"asReply", asReply,
		)
		formatted := sender.FormatTextResponse(fullText)
		var cinfo *waE2E.ContextInfo
		if asReply {
			cinfo = sender.ReplyContextInfo()
		}
		if len(p.mentions) > 0 {
			if cinfo == nil {
				cinfo = &waE2E.ContextInfo{}
			}
			for _, m := range p.mentions {
				if !m.IsEmpty() {
					cinfo.MentionedJID = append(cinfo.MentionedJID, m.ToNonAD().String())
				}
			}
		}
		var textMsg *waE2E.Message
		if cinfo != nil {
			textMsg = &waE2E.Message{
				ExtendedTextMessage: &waE2E.ExtendedTextMessage{
					Text:        &formatted,
					ContextInfo: cinfo,
				},
			}
		} else {
			textMsg = &waE2E.Message{
				Conversation: &formatted,
			}
		}

		respPre, errPre := cli.SendMessage(sender.GetSendContext(), to, textMsg)
		if errPre == nil {
			precedingMsgID = respPre.ID
			Logger.Debug("PollBuilder: preceding text body sent successfully", "msgID", precedingMsgID)
		} else {
			Logger.Error("PollBuilder: failed to transmit preceding text body", "err", errPre)
		}

		lines := strings.Split(fullText, "\n")
		q = strings.TrimSpace(lines[0])
		if len(q) > 200 {
			q = q[:197] + "..."
		}
		if q == "" {
			q = "Select an option below:"
		}
		Logger.Debug("PollBuilder: extracted poll header prompt after text split", "headerPrompt", q)
	}

	if q == "" {
		q = "Select an option:"
	}

	opts := make([]string, 0, len(p.options))
	for _, opt := range p.options {
		cleaned := strings.TrimSpace(opt)
		if len(cleaned) > 95 {
			cleaned = cleaned[:92] + "..."
		}
		if cleaned != "" {
			opts = append(opts, cleaned)
		}
	}
	if len(opts) < 2 {
		if len(opts) == 1 {
			opts = append(opts, "Cancel")
		} else {
			opts = []string{"Yes", "No"}
		}
		Logger.Debug("PollBuilder: padded options to meet minimum requirement of 2", "finalOptions", opts)
	}

	Logger.Debug("PollBuilder: building poll creation payload",
		"question", q,
		"optionCount", len(opts),
		"selectableCount", selectableCount,
	)
	msg := cli.BuildPollCreation(q, opts, selectableCount)

	var cinfo *waE2E.ContextInfo
	if asReply {
		cinfo = sender.ReplyContextInfo()
	}
	if len(p.mentions) > 0 {
		if cinfo == nil {
			cinfo = &waE2E.ContextInfo{}
		}
		for _, j := range p.mentions {
			if !j.IsEmpty() {
				cinfo.MentionedJID = append(cinfo.MentionedJID, j.ToNonAD().String())
			}
		}
	}
	if cinfo != nil && msg.PollCreationMessage != nil {
		msg.PollCreationMessage.ContextInfo = cinfo
	}

	start := time.Now()
	Logger.Debug("PollBuilder: transmitting poll message",
		"to", to.String(),
		"question", q,
		"options", opts,
		"single", p.single,
		"asReply", asReply,
		"mentionsCount", len(p.mentions),
	)

	resp, err := cli.SendMessage(sender.GetSendContext(), to, msg)
	if err != nil {
		Logger.Error("PollBuilder: failed to transmit poll message",
			"to", to.String(),
			"question", q,
			"err", err,
			"duration", time.Since(start),
		)
		return "", precedingMsgID, err
	}

	Logger.Debug("PollBuilder: poll message sent successfully",
		"msgID", resp.ID,
		"to", to.String(),
		"serverTimestamp", resp.Timestamp,
		"duration", time.Since(start),
	)
	return resp.ID, precedingMsgID, nil
}

func isSameUser(ctx context.Context, client *whatsmeow.Client, a, b types.JID) bool {
	if a.IsEmpty() || b.IsEmpty() {
		return false
	}
	if a.User == b.User && (a.Server == b.Server || a.Server == types.DefaultUserServer || b.Server == types.DefaultUserServer) {
		return true
	}
	if a == b || a.ToNonAD() == b.ToNonAD() {
		return true
	}
	return false
}

// DispatchPollVoteEvent decrypts the poll vote in evt, matches selected option
// hashes against stored option names, auto-deletes the poll message, and fires the registered handler.
func DispatchPollVoteEvent(sender Sender, evt *events.Message) bool {
	if evt == nil || evt.Message == nil {
		return false
	}
	pollUpdate := evt.Message.GetPollUpdateMessage()
	if pollUpdate == nil {
		return false
	}
	key := pollUpdate.GetPollCreationMessageKey()
	if key == nil || key.GetID() == "" {
		Logger.Debug("WARook: poll vote message key is empty or nil",
			"sender", sender.GetSender().String(),
			"chat", sender.GetChat().String(),
		)
		return false
	}
	pollMsgID := types.MessageID(key.GetID())

	Logger.Debug("WARook: incoming poll vote event",
		"targetPollMsgID", pollMsgID,
		"sender", sender.GetSender().String(),
		"chat", sender.GetChat().String(),
		"senderTimestamp", evt.Info.Timestamp,
	)

	pollRoutesMu.RLock()
	route, ok := pollRoutes[pollMsgID]
	pollRoutesMu.RUnlock()
	if !ok {
		Logger.Debug("WARook: no registered reactive route for poll message",
			"targetPollMsgID", pollMsgID,
			"sender", sender.GetSender().String(),
			"chat", sender.GetChat().String(),
		)
		return false
	}

	// Verify voter authorization if restricted
	if len(route.allowedSenders) > 0 {
		client := sender.GetClient()
		if client == nil {
			client = route.client
		}
		senderJID := sender.GetSender()
		isAllowed := false

		for _, allowed := range route.allowedSenders {
			if !allowed.IsEmpty() {
				if allowed == senderJID || allowed.ToNonAD() == senderJID.ToNonAD() {
					isAllowed = true
					break
				}
				if client != nil && isSameUser(context.Background(), client, allowed, senderJID) {
					isAllowed = true
					break
				}
			}
		}

		if !isAllowed && client != nil && client.Store != nil && client.Store.ID != nil {
			if isSameUser(context.Background(), client, *client.Store.ID, senderJID) {
				isAllowed = true
			}
		}

		if !isAllowed {
			Logger.Debug("WARook: ignoring poll vote from non-authorized sender (poll, timer, and route preserved)",
				"targetPollMsgID", pollMsgID,
				"sender", senderJID.String(),
				"chat", sender.GetChat().String(),
				"allowedSendersCount", len(route.allowedSenders),
			)
			return false
		}
	}

	// Stop expiration timer immediately upon receiving vote
	if route.timer != nil {
		route.timer.Stop()
	}

	// Deregister the route immediately so the poll cannot be used again
	if route.once || route.autoDelete {
		pollRoutesMu.Lock()
		delete(pollRoutes, pollMsgID)
		remaining := len(pollRoutes)
		pollRoutesMu.Unlock()
		Logger.Debug("WARook: poll handler consumed and deregistered on vote",
			"targetPollMsgID", pollMsgID,
			"remainingActivePollRoutes", remaining,
		)
	}

	// Auto-delete the poll message (and any preceding text body) from the chat
	if route.autoDelete {
		client := sender.GetClient()
		if client == nil {
			client = route.client
		}
		chat := sender.GetChat()
		if chat.IsEmpty() {
			chat = route.chat
		}
		if client != nil && !chat.IsEmpty() {
			go func(cli *whatsmeow.Client, ch types.JID, pID, preID types.MessageID) {
				Logger.Debug("WARook: auto-deleting completed poll message on vote", "pollMsgID", pID, "chat", ch.String())
				revokeMsg := cli.BuildRevoke(ch, types.EmptyJID, pID)
				_, err := cli.SendMessage(context.Background(), ch, revokeMsg)
				if err != nil {
					Logger.Debug("WARook: auto-delete poll message failed", "pollMsgID", pID, "err", err)
				} else {
					Logger.Debug("WARook: auto-deleted completed poll message", "pollMsgID", pID)
				}
				if preID != "" {
					preRevoke := cli.BuildRevoke(ch, types.EmptyJID, preID)
					_, _ = cli.SendMessage(context.Background(), ch, preRevoke)
					Logger.Debug("WARook: auto-deleted completed preceding text message", "precedingMsgID", preID)
				}
			}(client, chat, pollMsgID, route.precedingMsgID)
		}
	}

	cli := sender.GetClient()
	if cli == nil {
		cli = route.client
	}
	if cli == nil {
		Logger.Error("WARook: no client available to decrypt poll vote", "targetPollMsgID", pollMsgID)
		return false
	}

	decryptStart := time.Now()
	decrypted, err := cli.DecryptPollVote(context.Background(), evt)
	if err != nil {
		Logger.Error("WARook: poll vote decryption failed",
			"targetPollMsgID", pollMsgID,
			"sender", sender.GetSender().String(),
			"chat", sender.GetChat().String(),
			"err", err,
			"duration", time.Since(decryptStart),
		)
		return false
	}

	Logger.Debug("WARook: poll vote decrypted successfully",
		"targetPollMsgID", pollMsgID,
		"selectedHashesCount", len(decrypted.SelectedOptions),
		"duration", time.Since(decryptStart),
	)

	var selectedOptions []string
	for _, optHash := range decrypted.SelectedOptions {
		matched := false
		for _, name := range route.options {
			h := sha256.Sum256([]byte(name))
			if bytes.Equal(h[:], optHash) {
				selectedOptions = append(selectedOptions, name)
				matched = true
				Logger.Debug("WARook: matched poll option hash to option name",
					"targetPollMsgID", pollMsgID,
					"optionName", name,
					"hashHex", fmt.Sprintf("%x", optHash),
				)
				break
			}
		}
		if !matched {
			Logger.Debug("WARook: unmapped option hash in poll vote",
				"targetPollMsgID", pollMsgID,
				"hashHex", fmt.Sprintf("%x", optHash),
				"expectedOptions", route.options,
			)
		}
	}

	Logger.Debug("WARook: dispatching poll vote to reactive callback",
		"targetPollMsgID", pollMsgID,
		"sender", sender.GetSender().String(),
		"chat", sender.GetChat().String(),
		"selectedOptions", selectedOptions,
	)

	reqCtx := sender.GetSendContext()
	if reqCtx == nil || reqCtx.Err() != nil {
		reqCtx = context.Background()
	}
	req := PollRequest{
		PollMsgID:       pollMsgID,
		SelectedOptions: selectedOptions,
		Sender:          sender.GetSender(),
		Chat:            sender.GetChat(),
		Ctx:             reqCtx,
	}
	res := NewResponse(sender)
	if route.fn != nil {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					Logger.Error("WARook: poll handler panicked",
						"targetPollMsgID", pollMsgID,
						"sender", sender.GetSender().String(),
						"chat", sender.GetChat().String(),
						"panic", r,
					)
				}
			}()
			handlerStart := time.Now()
			route.fn(req, res)
			Logger.Debug("WARook: poll callback finished successfully",
				"targetPollMsgID", pollMsgID,
				"duration", time.Since(handlerStart),
			)
		}()
		return true
	}

	Logger.Debug("WARook: poll vote decrypted but route has no custom callback function",
		"targetPollMsgID", pollMsgID,
		"selectedOptions", selectedOptions,
	)
	return true
}
