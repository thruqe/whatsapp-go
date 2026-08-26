package src

import (
	"strings"
	"time"

	Logger "whatsrook/src/logger"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

// PollBuilder builds and sends a WhatsApp poll (with optional quoted reply)
// and registers a reactive callback for votes with automatic expiration and deletion.
type PollBuilder struct {
	rook       *WARook
	question   string
	options    []string
	single     bool // true = single-choice, false = multi-choice
	mentions   []types.JID
	asReply    bool
	autoDelete bool          // auto-delete poll message upon vote or timeout (default: true)
	timeout    time.Duration // timeout duration before auto-delete (default: 25s)
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
	RegisterPollRoute(PollRouteConfig{
		PollMsgID:      pollMsgID,
		PrecedingMsgID: precedingMsgID,
		Chat:           to,
		Client:         p.rook.ctx.Client,
		Options:        p.options,
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
	chat := p.rook.ctx.Chat
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
		Client:         p.rook.ctx.Client,
		Options:        p.options,
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
	RegisterPollRoute(PollRouteConfig{
		PollMsgID:      pollMsgID,
		PrecedingMsgID: precedingMsgID,
		Chat:           to,
		Client:         p.rook.ctx.Client,
		Options:        p.options,
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
	chat := p.rook.ctx.Chat
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
		Client:         p.rook.ctx.Client,
		Options:        p.options,
		Once:           true,
		AutoDelete:     p.autoDelete,
		Timeout:        p.timeout,
		Fn:             cb,
	})
	return pollMsgID, nil
}

func (p *PollBuilder) sendMsgWithID(to types.JID, asReply bool) (pollMsgID types.MessageID, precedingMsgID types.MessageID, err error) {
	ctx := p.rook.ctx
	ctx.StopAutoLoader()

	selectableCount := 1
	if !p.single {
		selectableCount = 0
	}

	q := strings.TrimSpace(p.question)
	// If the question is long (e.g. detailed menu or header text > 250 chars),
	// send the full descriptive text first as a reply/message, then present the poll with a clean question prompt.
	if len(q) > 250 {
		fullText := q
		Logger.Debug("PollBuilder: question exceeds 250 characters, transmitting preceding text body",
			"fullLength", len(fullText),
			"to", to.String(),
			"asReply", asReply,
		)
		formatted := ctx.formatTextResponse(fullText)
		var cinfo *waE2E.ContextInfo
		if asReply {
			cinfo = ctx.replyContextInfo()
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

		respPre, errPre := ctx.Client.SendMessage(ctx.GetSendContext(), to, textMsg)
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
	msg := ctx.Client.BuildPollCreation(q, opts, selectableCount)

	var cinfo *waE2E.ContextInfo
	if asReply {
		cinfo = ctx.replyContextInfo()
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

	resp, err := ctx.Client.SendMessage(ctx.GetSendContext(), to, msg)
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
