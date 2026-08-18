package main

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	commands "whatsrook/cli/plugins"
	cliutils "whatsrook/cli/utils"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types/events"
)

func (b *Bot) handleAutoAcceptCall(ctx context.Context, v *events.CallOffer) {
	cli := b.client.WAClient()
	if cli == nil || v == nil {
		return
	}
	commands.HandleAutoAcceptIncomingCall(ctx, cli, v)
}

func (b *Bot) handleAntiCall(ctx context.Context, v *events.CallOffer) {
	cli := b.client.WAClient()
	if cli == nil || v == nil {
		return
	}
	s, ok := cli.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return
	}

	autoAcceptStatus, _ := s.GetSetting(ctx, cliutils.AutoAcceptCallSettingKey)
	if autoAcceptStatus == "on" {
		slog.Debug("anticall: skipping reject because autoacceptcall is enabled", "call_id", v.CallID)
		return
	}

	status, _ := s.GetSetting(ctx, "anticall_status")
	if status != "on" {
		return
	}

	callerJID := v.CallCreator
	callerNum := callerJID.User

	contactsOnly, _ := s.GetSetting(ctx, "anticall_contacts_only")
	allowedCC, _ := s.GetSetting(ctx, "anticall_allowed_cc")

	reject := false

	if contactsOnly == "true" {
		contact, err := cli.Store.Contacts.GetContact(ctx, callerJID)
		if err != nil || (!contact.Found || (contact.FirstName == "" && contact.FullName == "")) {
			reject = true
		}
	}

	if !reject && allowedCC != "" {
		codes := strings.Split(allowedCC, ",")
		matched := false
		for _, cc := range codes {
			cc = strings.TrimSpace(strings.TrimPrefix(cc, "+"))
			if cc != "" && strings.HasPrefix(callerNum, cc) {
				matched = true
				break
			}
		}
		if !matched {
			reject = true
		}
	}

	if !reject && contactsOnly != "true" && allowedCC == "" {
		reject = true
	}

	if reject {
		slog.Warn("anticall: rejecting call offer", "from", callerJID.String(), "call_id", v.CallID)
		_ = cli.RejectCall(ctx, callerJID, v.CallID)

		warnKey := "anticall_warn:" + callerJID.String()
		rawWarn, _ := s.GetSetting(ctx, warnKey)
		warnCount, _ := strconv.Atoi(rawWarn)
		warnCount++
		_ = s.PutSetting(ctx, warnKey, strconv.Itoa(warnCount))

		rawMax, _ := s.GetSetting(ctx, "anticall_max_warn")
		maxWarn, _ := strconv.Atoi(rawMax)
		if maxWarn <= 0 {
			maxWarn = 3
		}

		if warnCount >= maxWarn {
			_, _ = cli.UpdateBlocklist(ctx, callerJID, events.BlocklistChangeActionBlock)
			slog.Warn("anticall: caller blocked after reaching max warnings", "from", callerJID.String(), "warn_count", warnCount)
			warnText := fmt.Sprintf("Call rejected. You have reached the maximum warning threshold (%d/%d) and have been blocked.", warnCount, maxWarn)
			formatted := cliutils.FormatTextResponseRaw(warnText)
			_, _ = cli.SendMessage(ctx, callerJID, &waE2E.Message{Conversation: &formatted})
		} else {
			warnText := fmt.Sprintf("Call rejected. Warning %d/%d. Continued calls will result in being blocked.", warnCount, maxWarn)
			formatted := cliutils.FormatTextResponseRaw(warnText)
			_, _ = cli.SendMessage(ctx, callerJID, &waE2E.Message{Conversation: &formatted})
		}
	}
}
