package src

import (
	"fmt"
	"strings"

	Logger "whatsrook/src/logger"

	"go.mau.fi/whatsmeow"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

// ButtonBuilder builds and sends an interactive WhatsApp button message
// and optionally registers a reactive callback for any of its buttons.
type ButtonBuilder struct {
	rook     *WARook
	body     string
	footer   string
	buttons  []struct{ ID, Text string }
	mentions []types.JID
}

// Add appends a button with the given ID and display text.
func (b *ButtonBuilder) Add(id, text string) *ButtonBuilder {
	b.buttons = append(b.buttons, struct{ ID, Text string }{id, text})
	return b
}

// Footer sets the footer text shown below the button message.
func (b *ButtonBuilder) Footer(text string) *ButtonBuilder {
	b.footer = text
	return b
}

// Mentions attaches JIDs to be mentioned in the button message.
func (b *ButtonBuilder) Mentions(jids ...types.JID) *ButtonBuilder {
	for _, j := range jids {
		if !j.IsEmpty() {
			b.mentions = append(b.mentions, j)
		}
	}
	return b
}

// Send sends the button message to the given JID and optionally registers a
// persistent handler (fires on every click until manually deregistered).
func (b *ButtonBuilder) Send(to types.JID, fn ...func(req ButtonRequest, res *Response)) error {
	_, err := b.SendWithID(to, fn...)
	return err
}

// SendWithID sends the button message to the given JID, returns its MessageID, and optionally registers a handler.
func (b *ButtonBuilder) SendWithID(to types.JID, fn ...func(req ButtonRequest, res *Response)) (types.MessageID, error) {
	msgID, err := b.sendMsgWithID(to)
	if err != nil {
		return "", err
	}
	if len(fn) > 0 && fn[0] != nil {
		b.registerHandlers(false, fn[0])
	}
	return msgID, nil
}

// Reply sends the button message as a reply to the current event and
// optionally registers a persistent handler.
func (b *ButtonBuilder) Reply(fn ...func(req ButtonRequest, res *Response)) error {
	_, err := b.ReplyWithID(fn...)
	return err
}

// ReplyWithID sends the button message as a reply, returns its MessageID, and optionally registers a handler.
func (b *ButtonBuilder) ReplyWithID(fn ...func(req ButtonRequest, res *Response)) (types.MessageID, error) {
	return b.SendWithID(b.rook.ctx.Chat, fn...)
}

// Once sends the button message to the given JID and registers a
// one-shot handler (auto-deregisters after the first click).
func (b *ButtonBuilder) Once(to types.JID, fn ...func(req ButtonRequest, res *Response)) error {
	_, err := b.OnceWithID(to, fn...)
	return err
}

// OnceWithID sends the button message to the given JID, returns its MessageID, and registers a one-shot handler.
func (b *ButtonBuilder) OnceWithID(to types.JID, fn ...func(req ButtonRequest, res *Response)) (types.MessageID, error) {
	msgID, err := b.sendMsgWithID(to)
	if err != nil {
		return "", err
	}
	if len(fn) > 0 && fn[0] != nil {
		b.registerHandlers(true, fn[0])
	}
	return msgID, nil
}

// OnceReply sends the button message as a reply and registers a one-shot handler.
func (b *ButtonBuilder) OnceReply(fn ...func(req ButtonRequest, res *Response)) error {
	_, err := b.OnceReplyWithID(fn...)
	return err
}

// OnceReplyWithID sends the button message as a reply, returns its MessageID, and registers a one-shot handler.
func (b *ButtonBuilder) OnceReplyWithID(fn ...func(req ButtonRequest, res *Response)) (types.MessageID, error) {
	return b.OnceWithID(b.rook.ctx.Chat, fn...)
}

func (b *ButtonBuilder) registerHandlers(once bool, fn func(req ButtonRequest, res *Response)) {
	if fn == nil {
		return
	}
	for _, btn := range b.buttons {
		RegisterButtonHandler(btn.ID, once, fn)
	}
}

func (b *ButtonBuilder) sendMsgWithID(to types.JID) (types.MessageID, error) {
	ctx := b.rook.ctx
	footer := b.footer
	if footer == "" {
		footer = ctx.GetBotName()
	}
	Logger.Debug("WARook: sending button message", "to", to.String(), "body", b.body, "buttons", len(b.buttons), "mentions", len(b.mentions))
	return sendButtonMsg(ctx, to, b.body, footer, b.buttons, b.mentions)
}

// sendButtonMsg is the low-level button sender. It replicates the
// DocumentWithCaptionMessage+waBinary native-flow pattern used throughout
// WhatsRook so that buttons render correctly on all WhatsApp client versions.
func sendButtonMsg(ctx *PluginContext, to types.JID, bodyText, footerText string, buttons []struct{ ID, Text string }, mentions []types.JID) (types.MessageID, error) {
	ctx.StopAutoLoader()
	var btnList []*waE2E.ButtonsMessage_Button
	for _, b := range buttons {
		if strings.HasPrefix(b.ID, "http://") || strings.HasPrefix(b.ID, "https://") {
			name := "cta_url"
			params := fmt.Sprintf(`{"display_text":%q,"url":%q,"merchant_url":%q}`, b.Text, b.ID, b.ID)
			btnList = append(btnList, &waE2E.ButtonsMessage_Button{
				ButtonID:   &b.ID,
				ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: &b.Text},
				Type:       waE2E.ButtonsMessage_Button_NATIVE_FLOW.Enum(),
				NativeFlowInfo: &waE2E.ButtonsMessage_Button_NativeFlowInfo{
					Name:       &name,
					ParamsJSON: &params,
				},
			})
		} else {
			btnList = append(btnList, &waE2E.ButtonsMessage_Button{
				ButtonID:   &b.ID,
				ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: &b.Text},
				Type:       waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
			})
		}
	}

	var mentionedStrings []string
	for _, j := range mentions {
		if !j.IsEmpty() {
			mentionedStrings = append(mentionedStrings, j.ToNonAD().String())
		}
	}

	var cInfo *waE2E.ContextInfo
	if len(mentionedStrings) > 0 {
		cInfo = &waE2E.ContextInfo{
			MentionedJID: mentionedStrings,
		}
	}

	msg := &waE2E.Message{
		DocumentWithCaptionMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				ButtonsMessage: &waE2E.ButtonsMessage{
					ContentText: &bodyText,
					FooterText:  &footerText,
					HeaderType:  waE2E.ButtonsMessage_EMPTY.Enum(),
					Buttons:     btnList,
					ContextInfo: cInfo,
				},
			},
		},
	}

	bizNode := waBinary.Node{
		Tag:   "biz",
		Attrs: waBinary.Attrs{},
		Content: []waBinary.Node{
			{
				Tag: "interactive",
				Attrs: waBinary.Attrs{
					"type": "native_flow",
					"v":    "1",
				},
				Content: []waBinary.Node{
					{
						Tag: "native_flow",
						Attrs: waBinary.Attrs{
							"v":    "9",
							"name": "mixed",
						},
					},
				},
			},
		},
	}

	extra := whatsmeow.SendRequestExtra{
		AdditionalNodes: &[]waBinary.Node{bizNode},
	}
	resp, err := ctx.Client.SendMessage(ctx.GetSendContext(), to, msg, extra)
	if err != nil {
		Logger.Error("WARook: sendButtonMsg failed", "to", to.String(), "err", err)
		return "", err
	}
	return resp.ID, nil
}
