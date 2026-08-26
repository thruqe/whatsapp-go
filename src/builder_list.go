package src

import (
	Logger "whatsrook/src/logger"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

// ListRow is a single row entry within a list section.
type ListRow struct {
	ID          string
	Title       string
	Description string
}

// ListSection is a named group of ListRows.
type ListSection struct {
	Title string
	Rows  []ListRow
}

// ListBuilder builds and sends a WhatsApp list message and optionally registers
// a reactive callback for row selections.
type ListBuilder struct {
	rook       *WARook
	body       string
	buttonText string
	title      string
	footer     string
	sections   []ListSection
}

// Title sets the title shown at the top of the list panel.
func (l *ListBuilder) Title(text string) *ListBuilder {
	l.title = text
	return l
}

// Footer sets the footer text shown below the list button.
func (l *ListBuilder) Footer(text string) *ListBuilder {
	l.footer = text
	return l
}

// AddSection appends a named section.
func (l *ListBuilder) AddSection(title string) *ListBuilder {
	l.sections = append(l.sections, ListSection{Title: title})
	return l
}

// AddRow appends a row to the most recently added section.
// If no section has been added yet a default section is created.
func (l *ListBuilder) AddRow(id, title, description string) *ListBuilder {
	if len(l.sections) == 0 {
		l.sections = append(l.sections, ListSection{Title: ""})
	}
	l.sections[len(l.sections)-1].Rows = append(
		l.sections[len(l.sections)-1].Rows,
		ListRow{ID: id, Title: title, Description: description},
	)
	return l
}

// Send sends the list message to the given JID and optionally registers fn for row selections.
func (l *ListBuilder) Send(to types.JID, fn ...func(req ListRequest, res *Response)) error {
	if err := l.sendMsg(to); err != nil {
		return err
	}
	if len(fn) > 0 && fn[0] != nil {
		l.registerHandlers(false, fn[0])
	}
	return nil
}

// Reply sends the list message as a reply and optionally registers fn for row selections.
func (l *ListBuilder) Reply(fn ...func(req ListRequest, res *Response)) error {
	return l.Send(l.rook.ctx.Chat, fn...)
}

// Once sends the list message to the given JID and registers a one-shot handler.
func (l *ListBuilder) Once(to types.JID, fn ...func(req ListRequest, res *Response)) error {
	if err := l.sendMsg(to); err != nil {
		return err
	}
	if len(fn) > 0 && fn[0] != nil {
		l.registerHandlers(true, fn[0])
	}
	return nil
}

// OnceReply sends the list message as a reply with a one-shot handler.
func (l *ListBuilder) OnceReply(fn ...func(req ListRequest, res *Response)) error {
	return l.Once(l.rook.ctx.Chat, fn...)
}

func (l *ListBuilder) registerHandlers(once bool, fn func(req ListRequest, res *Response)) {
	if fn == nil {
		return
	}
	for _, sec := range l.sections {
		for _, row := range sec.Rows {
			RegisterListHandler(row.ID, once, fn)
		}
	}
}

func (l *ListBuilder) sendMsg(to types.JID) error {
	ctx := l.rook.ctx
	ctx.StopAutoLoader()

	var sections []*waE2E.ListMessage_Section
	for _, sec := range l.sections {
		var rows []*waE2E.ListMessage_Row
		for _, row := range sec.Rows {
			r := row
			rows = append(rows, &waE2E.ListMessage_Row{
				RowID:       &r.ID,
				Title:       &r.Title,
				Description: &r.Description,
			})
		}
		s := sec
		sections = append(sections, &waE2E.ListMessage_Section{
			Title: &s.Title,
			Rows:  rows,
		})
	}

	body := l.body
	btnText := l.buttonText
	if btnText == "" {
		btnText = "Open List"
	}
	title := l.title
	footer := l.footer
	if footer == "" {
		footer = ctx.GetBotName()
	}

	msg := &waE2E.Message{
		ListMessage: &waE2E.ListMessage{
			Title:       &title,
			Description: &body,
			ButtonText:  &btnText,
			FooterText:  &footer,
			ListType:    waE2E.ListMessage_SINGLE_SELECT.Enum(),
			Sections:    sections,
		},
	}

	Logger.Debug("WARook: sending list", "to", to.String(), "sections", len(sections))
	_, err := ctx.Client.SendMessage(ctx.GetSendContext(), to, msg)
	if err != nil {
		Logger.Error("WARook: sendListMsg failed", "to", to.String(), "err", err)
	}
	return err
}
