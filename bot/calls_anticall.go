package main

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/thruqe/whatsrook"

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

	Debug("anticall: received call offer event",
		"call_id", v.CallID,
		"caller", v.CallCreator.String(),
		"timestamp", v.Timestamp,
	)

	// Do not process call offers that occurred before bot startup (with 2m clock skew tolerance)
	if !v.Timestamp.IsZero() && v.Timestamp.Before(startupTime.Add(-2*time.Minute)) {
		Debug("anticall: skipping stale call offer before startup",
			"call_id", v.CallID,
			"caller", v.CallCreator.String(),
			"timestamp", v.Timestamp,
		)
		return
	}

	if cli.Store == nil || cli.Store.Identities == nil {
		return
	}
	s, ok := cli.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return
	}

	voicemailStatus, _ := GetSetting(ctx, s, VoicemailSettingKey)
	if voicemailStatus == "" {
		voicemailStatus, _ = GetSetting(ctx, s, "autoacceptcall_status")
	}
	if voicemailStatus == "on" {
		Debug("anticall: skipping reject because voicemail is enabled", "call_id", v.CallID, "caller", v.CallCreator.String())
		return
	}

	status, _ := GetSetting(ctx, s, "anticall_status")
	if status != "on" {
		Debug("anticall: feature is disabled, ignoring call offer", "call_id", v.CallID, "status", status)
		return
	}

	callerJID := v.CallCreator
	callerNum := callerJID.User

	contactsOnly, _ := GetSetting(ctx, s, "anticall_contacts_only")
	allowedCC, _ := GetSetting(ctx, s, "anticall_allowed_cc")

	reject := false

	if contactsOnly == "true" {
		var contactFound bool
		if cli.Store != nil && cli.Store.Contacts != nil {
			contact, err := cli.Store.Contacts.GetContact(ctx, callerJID)
			contactFound = err == nil && contact.Found && (contact.FirstName != "" || contact.FullName != "")
		}
		if !contactFound {
			Debug("anticall: caller is not in contacts and contacts-only mode is active", "caller", callerJID.String())
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
			Debug("anticall: caller country code is not in allowed CC whitelist", "caller", callerJID.String(), "allowedCC", allowedCC)
			reject = true
		}
	}

	if !reject && contactsOnly != "true" && allowedCC == "" {
		Debug("anticall: rejecting call because unconditional anticall is active", "caller", callerJID.String())
		reject = true
	}

	Debug("anticall: evaluation completed",
		"caller", callerJID.String(),
		"call_id", v.CallID,
		"contactsOnly", contactsOnly,
		"allowedCC", allowedCC,
		"reject", reject,
	)

	if reject {
		Warn("anticall: rejecting call offer", "from", callerJID.String(), "call_id", v.CallID)
		_ = cli.RejectCall(ctx, callerJID, v.CallID)

		warnKey := "anticall_warn:" + callerJID.String()
		rawWarn, _ := GetSetting(ctx, s, warnKey)
		warnCount, _ := strconv.Atoi(rawWarn)
		warnCount++
		_ = PutSetting(ctx, s, warnKey, strconv.Itoa(warnCount))

		rawMax, _ := GetSetting(ctx, s, "anticall_max_warn")
		maxWarn, _ := strconv.Atoi(rawMax)
		if maxWarn <= 0 {
			maxWarn = 3
		}

		Debug("anticall: caller warning updated", "caller", callerJID.String(), "warnCount", warnCount, "maxWarn", maxWarn)

		if warnCount >= maxWarn {
			warnText := Sprintf("Call rejected. You have reached the maximum warning threshold (%d/%d) and have been blocked.", warnCount, maxWarn)
			formatted := whatsrook.FormatTextResponseRaw(warnText)
			_, _ = cli.SendMessage(ctx, callerJID, &waE2E.Message{Conversation: &formatted})
			_, _ = cli.UpdateBlocklist(ctx, callerJID, events.BlocklistChangeActionBlock, "")
			Warn("anticall: caller blocked after reaching max warnings", "from", callerJID.String(), "warn_count", warnCount)
		} else {
			warnText := Sprintf("Call rejected. Warning %d/%d. Continued calls will result in being blocked.", warnCount, maxWarn)
			formatted := whatsrook.FormatTextResponseRaw(warnText)
			_, _ = cli.SendMessage(ctx, callerJID, &waE2E.Message{Conversation: &formatted})
		}
	}
}
