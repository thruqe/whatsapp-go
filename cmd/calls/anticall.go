package calls

import (
	"context"
	"strconv"
	"strings"
	"time"

	"whatsrook"
	"whatsrook/cmd/store"
	"whatsrook/util/logger"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types/events"
)

// HandleAntiCallEvent processes incoming call offer events according to the user's anticall and voicemail settings.
func HandleAntiCallEvent(ctx context.Context, cli *whatsmeow.Client, v *events.CallOffer, startupTime time.Time) {
	if cli == nil || v == nil {
		return
	}

	logger.Debug("anticall: received call offer event",
		"call_id", v.CallID,
		"caller", v.CallCreator.String(),
		"timestamp", v.Timestamp,
	)

	// Do not process call offers that occurred before bot startup (with 2m clock skew tolerance)
	if !v.Timestamp.IsZero() && v.Timestamp.Before(startupTime.Add(-2*time.Minute)) {
		logger.Debug("anticall: skipping stale call offer before startup",
			"call_id", v.CallID,
			"caller", v.CallCreator.String(),
			"timestamp", v.Timestamp,
		)
		return
	}

	s, ok := cli.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return
	}

	voicemailStatus, _ := store.GetSetting(ctx, s, VoicemailSettingKey)
	if voicemailStatus == "" {
		voicemailStatus, _ = store.GetSetting(ctx, s, "autoacceptcall_status")
	}
	if voicemailStatus == "on" {
		logger.Debug("anticall: skipping reject because voicemail is enabled", "call_id", v.CallID, "caller", v.CallCreator.String())
		return
	}

	status, _ := store.GetSetting(ctx, s, "anticall_status")
	if status != "on" {
		logger.Debug("anticall: feature is disabled, ignoring call offer", "call_id", v.CallID, "status", status)
		return
	}

	callerJID := v.CallCreator
	callerNum := callerJID.User

	contactsOnly, _ := store.GetSetting(ctx, s, "anticall_contacts_only")
	allowedCC, _ := store.GetSetting(ctx, s, "anticall_allowed_cc")

	reject := false

	if contactsOnly == "true" {
		contact, err := cli.Store.Contacts.GetContact(ctx, callerJID)
		if err != nil || (!contact.Found || (contact.FirstName == "" && contact.FullName == "")) {
			logger.Debug("anticall: caller is not in contacts and contacts-only mode is active", "caller", callerJID.String())
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
			logger.Debug("anticall: caller country code is not in allowed CC whitelist", "caller", callerJID.String(), "allowedCC", allowedCC)
			reject = true
		}
	}

	if !reject && contactsOnly != "true" && allowedCC == "" {
		logger.Debug("anticall: rejecting call because unconditional anticall is active", "caller", callerJID.String())
		reject = true
	}

	logger.Debug("anticall: evaluation completed",
		"caller", callerJID.String(),
		"call_id", v.CallID,
		"contactsOnly", contactsOnly,
		"allowedCC", allowedCC,
		"reject", reject,
	)

	if reject {
		logger.Warn("anticall: rejecting call offer", "from", callerJID.String(), "call_id", v.CallID)
		_ = cli.RejectCall(ctx, callerJID, v.CallID)

		warnKey := "anticall_warn:" + callerJID.String()
		rawWarn, _ := store.GetSetting(ctx, s, warnKey)
		warnCount, _ := strconv.Atoi(rawWarn)
		warnCount++
		_ = store.PutSetting(ctx, s, warnKey, strconv.Itoa(warnCount))

		rawMax, _ := store.GetSetting(ctx, s, "anticall_max_warn")
		maxWarn, _ := strconv.Atoi(rawMax)
		if maxWarn <= 0 {
			maxWarn = 3
		}

		logger.Debug("anticall: caller warning updated", "caller", callerJID.String(), "warnCount", warnCount, "maxWarn", maxWarn)

		if warnCount >= maxWarn {
			warnText := whatsrook.Sprintf("Call rejected. You have reached the maximum warning threshold (%d/%d) and have been blocked.", warnCount, maxWarn)
			formatted := whatsrook.FormatTextResponseRaw(warnText)
			_, _ = cli.SendMessage(ctx, callerJID, &waE2E.Message{Conversation: &formatted})
			_, _ = cli.UpdateBlocklist(ctx, callerJID, events.BlocklistChangeActionBlock)
			logger.Warn("anticall: caller blocked after reaching max warnings", "from", callerJID.String(), "warn_count", warnCount)
		} else {
			warnText := whatsrook.Sprintf("Call rejected. Warning %d/%d. Continued calls will result in being blocked.", warnCount, maxWarn)
			formatted := whatsrook.FormatTextResponseRaw(warnText)
			_, _ = cli.SendMessage(ctx, callerJID, &waE2E.Message{Conversation: &formatted})
		}
	}
}
