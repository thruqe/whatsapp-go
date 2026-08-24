package utils

import (
	"whatsrook/logger"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

// PollBuilder builds and sends a WhatsApp poll and optionally registers a reactive
// callback for votes.
type PollBuilder struct {
	rook     *WARook
	question string
	options  []string
	single   bool // true = single-choice, false = multi-choice
}

// AddOption appends a poll option.
func (p *PollBuilder) AddOption(option string) *PollBuilder {
	p.options = append(p.options, option)
	return p
}

// SingleChoice restricts the poll to one selectable answer (default).
func (p *PollBuilder) SingleChoice() *PollBuilder {
	p.single = true
	return p
}

// MultiChoice allows multiple answers to be selected simultaneously.
func (p *PollBuilder) MultiChoice() *PollBuilder {
	p.single = false
	return p
}

// Send sends the poll to the given JID and optionally registers fn to receive votes.
func (p *PollBuilder) Send(to types.JID, fn ...func(req PollRequest, res *Response)) error {
	msgID, err := p.sendMsg(to)
	if err != nil {
		return err
	}
	if len(fn) > 0 && fn[0] != nil {
		RegisterPollHandler(msgID, p.options, fn[0])
	}
	return nil
}

// Reply sends the poll as a reply to the current event and optionally registers fn to receive votes.
func (p *PollBuilder) Reply(fn ...func(req PollRequest, res *Response)) error {
	return p.Send(p.rook.ctx.Chat, fn...)
}

func (p *PollBuilder) sendMsg(to types.JID) (types.MessageID, error) {
	ctx := p.rook.ctx
	ctx.StopAutoLoader()

	var maxSelectable uint32 = 1
	if !p.single {
		maxSelectable = 0
	}

	opts := make([]*waE2E.PollCreationMessage_Option, len(p.options))
	for i, name := range p.options {
		n := name
		opts[i] = &waE2E.PollCreationMessage_Option{OptionName: &n}
	}

	q := p.question
	msg := &waE2E.Message{
		PollCreationMessage: &waE2E.PollCreationMessage{
			Name:                   &q,
			Options:                opts,
			SelectableOptionsCount: &maxSelectable,
		},
	}

	Logger.Debug("WARook: sending poll", "to", to.String(), "question", p.question, "options", len(p.options), "single", p.single)
	resp, err := ctx.Client.SendMessage(ctx.GetSendContext(), to, msg)
	if err != nil {
		Logger.Error("WARook: sendPollMsg failed", "to", to.String(), "err", err)
		return "", err
	}
	return resp.ID, nil
}
