package plugins

import (
	"strings"

	utils "whatsrook/src"

	"go.mau.fi/whatsmeow/proto/waE2E"
)

func init() {
	Register(&Command{
		Name:        "filter",
		Description: "Add, delete, or list auto-response filters (P2P chats only). Usage: filter [word] [response text] (or reply to media), filter del [word], filter list",
		Category:    "filters",
		IsPublic:    true,
		Handler:     handleFilter,
	})
	Register(&Command{
		Name:        "bgm",
		Description: "Add, delete, or list audio auto-responses (P2P chats only). Usage: bgm [word] (replying to audio), bgm del [word], bgm list",
		Category:    "filters",
		IsPublic:    true,
		Handler:     handleBGM,
	})
	Register(&Command{
		Name:        "mention",
		Description: "Configure auto-response when the bot is tagged. Usage: mention [text...], mention add (replying to a message), mention del, mention list",
		Category:    "filters",
		IsPublic:    true,
		Handler:     handleMention,
	})
	Register(&Command{
		Name:        "filteradd",
		Alias:       "addfilter",
		Description: "Add an auto-response filter for a trigger word. Usage: filteradd [word] [response text] (or reply to a message)",
		Category:    "filters",
		IsPublic:    true,
		Handler:     handleAddFilter,
	})
	Register(&Command{
		Name:        "filterget",
		Alias:       "getfilter",
		Description: "Get the auto-response message for a trigger word. Usage: filterget [word]",
		Category:    "filters",
		IsPublic:    true,
		Handler:     handleGetFilter,
	})
	Register(&Command{
		Name:        "filters",
		Alias:       "listfilters",
		Description: "List all active auto-response filters. Usage: filters",
		Category:    "filters",
		IsPublic:    true,
		Handler:     handleListFilters,
	})
	Register(&Command{
		Name:        "filterdel",
		Alias:       "delfilter",
		Description: "Remove an auto-response filter. Usage: filterdel [word]",
		Category:    "filters",
		IsPublic:    true,
		Handler:     handleDelFilter,
	})
}

func handleFilter(ctx *Context) error {
	s, ok := getStore(ctx)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}
	db := s.GetDB()
	if db == nil {
		return ctx.Reply("Database unavailable.")
	}

	ourJID := ctx.Client.Store.ID.ToNonAD().String()

	if len(ctx.Args) == 0 {
		return ctx.Reply("Usage:\n- filter [word] [response text]\n- filter [word] (replying to response message)\n- filter del [word]\n- filter list")
	}

	var trigger string
	var responseProtoMsg *waE2E.Message
	quoted := ctx.GetQuotedMessage()

	action := strings.ToLower(ctx.Args[0])
	switch action {
	case "add":
		if len(ctx.Args) < 2 {
			return ctx.Reply("Please specify the trigger word.")
		}
		trigger = strings.ToLower(ctx.Args[1])

		if quoted != nil {
			responseProtoMsg = quoted
		} else {
			if len(ctx.Args) < 3 {
				return ctx.Reply("Please reply to a message or provide response text.")
			}
			textVal := strings.Join(ctx.Args[2:], " ")
			responseProtoMsg = &waE2E.Message{
				Conversation: &textVal,
			}
		}

	case "del", "remove":
		if len(ctx.Args) < 2 {
			return ctx.Reply("Please specify the trigger word to remove.")
		}
		trigger = strings.ToLower(ctx.Args[1])

		res, err := db.Exec(ctx.Ctx, `DELETE FROM bot_filters WHERE our_jid=$1 AND trigger_word=$2`, ourJID, trigger)
		if err != nil {
			return ctx.Reply("Failed to delete filter.")
		}
		rows, _ := res.RowsAffected()
		if rows == 0 {
			return ctx.Replyf("Filter for word %q not found.", trigger)
		}
		return ctx.Replyf("Filter for word %q removed.", trigger)

	case "list":
		rows, err := db.Query(ctx.Ctx, `SELECT trigger_word FROM bot_filters WHERE our_jid=$1`, ourJID)
		if err != nil {
			return ctx.Reply("Failed to query filters.")
		}
		defer rows.Close()

		var triggers []string
		for rows.Next() {
			var t string
			if err := rows.Scan(&t); err == nil {
				triggers = append(triggers, t)
			}
		}

		if len(triggers) == 0 {
			return ctx.Reply("No filters configured.")
		}
		return ctx.Replyf("Active Filters:\n- %s", strings.Join(triggers, "\n- "))

	default:
		trigger = strings.ToLower(ctx.Args[0])

		if quoted != nil {
			responseProtoMsg = quoted
		} else {
			if len(ctx.Args) < 2 {
				return ctx.Reply("Please specify response text or reply to a message.")
			}
			textVal := strings.Join(ctx.Args[1:], " ")
			responseProtoMsg = &waE2E.Message{
				Conversation: &textVal,
			}
		}
	}

	if responseProtoMsg != nil {
		encoded, err := utils.EncodeProtoMessage(responseProtoMsg)
		if err != nil {
			return ctx.Replyf("Failed to encode filter message: %v", err)
		}

		_, err = db.Exec(ctx.Ctx, `
			INSERT INTO bot_filters (our_jid, trigger_word, message_proto)
			VALUES ($1, $2, $3)
			ON CONFLICT(our_jid, trigger_word) DO UPDATE SET message_proto=excluded.message_proto
		`, ourJID, trigger, encoded)
		if err != nil {
			return ctx.Reply("Failed to save filter.")
		}

		return ctx.Replyf("Filter added for word %q.", trigger)
	}

	return nil
}

func handleBGM(ctx *Context) error {
	s, ok := getStore(ctx)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}
	db := s.GetDB()
	if db == nil {
		return ctx.Reply("Database unavailable.")
	}

	ourJID := ctx.Client.Store.ID.ToNonAD().String()

	if len(ctx.Args) == 0 {
		return ctx.Reply("Usage:\n- bgm [word] (replying to audio)\n- bgm del [word]\n- bgm list")
	}

	var trigger string
	var responseProtoMsg *waE2E.Message
	quoted := ctx.GetQuotedMessage()

	action := strings.ToLower(ctx.Args[0])
	switch action {
	case "add":
		if len(ctx.Args) < 2 {
			return ctx.Reply("Please specify the trigger word.")
		}
		trigger = strings.ToLower(ctx.Args[1])

		if quoted == nil {
			return ctx.Reply("Please reply to the audio message you want to set as the BGM.")
		}
		if quoted.AudioMessage == nil {
			return ctx.Reply("The replied message must be an audio/voice note.")
		}
		responseProtoMsg = quoted

	case "del", "remove":
		if len(ctx.Args) < 2 {
			return ctx.Reply("Please specify the trigger word to remove.")
		}
		trigger = strings.ToLower(ctx.Args[1])

		res, err := db.Exec(ctx.Ctx, `DELETE FROM bot_bgm WHERE our_jid=$1 AND trigger_word=$2`, ourJID, trigger)
		if err != nil {
			return ctx.Reply("Failed to delete BGM.")
		}
		rows, _ := res.RowsAffected()
		if rows == 0 {
			return ctx.Replyf("BGM for word %q not found.", trigger)
		}
		return ctx.Replyf("BGM for word %q removed.", trigger)

	case "list":
		rows, err := db.Query(ctx.Ctx, `SELECT trigger_word FROM bot_bgm WHERE our_jid=$1`, ourJID)
		if err != nil {
			return ctx.Reply("Failed to query BGMs.")
		}
		defer rows.Close()

		var triggers []string
		for rows.Next() {
			var t string
			if err := rows.Scan(&t); err == nil {
				triggers = append(triggers, t)
			}
		}

		if len(triggers) == 0 {
			return ctx.Reply("No BGMs configured.")
		}
		return ctx.Replyf("Active BGMs:\n- %s", strings.Join(triggers, "\n- "))

	default:
		trigger = strings.ToLower(ctx.Args[0])

		if quoted == nil {
			return ctx.Reply("Please reply to the audio message you want to set as the BGM.")
		}
		if quoted.AudioMessage == nil {
			return ctx.Reply("The replied message must be an audio/voice note.")
		}
		responseProtoMsg = quoted
	}

	if responseProtoMsg != nil {
		encoded, err := utils.EncodeProtoMessage(responseProtoMsg)
		if err != nil {
			return ctx.Replyf("Failed to encode BGM message: %v", err)
		}

		_, err = db.Exec(ctx.Ctx, `
			INSERT INTO bot_bgm (our_jid, trigger_word, message_proto)
			VALUES ($1, $2, $3)
			ON CONFLICT(our_jid, trigger_word) DO UPDATE SET message_proto=excluded.message_proto
		`, ourJID, trigger, encoded)
		if err != nil {
			return ctx.Reply("Failed to save BGM.")
		}

		return ctx.Replyf("BGM added for word %q.", trigger)
	}

	return nil
}

func handleMention(ctx *Context) error {
	s, ok := getStore(ctx)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}
	db := s.GetDB()
	if db == nil {
		return ctx.Reply("Database unavailable.")
	}

	ourJID := ctx.Client.Store.ID.ToNonAD().String()

	if len(ctx.Args) == 0 {
		return ctx.Reply("Usage:\n- mention [text...]\n- mention add (replying to a message)\n- mention del\n- mention list")
	}

	action := strings.ToLower(ctx.Args[0])
	switch action {
	case "add":
		quoted := ctx.GetQuotedMessage()
		if quoted == nil {
			if len(ctx.Args) < 2 {
				return ctx.Reply("Please reply to a message or specify response text.")
			}
			textVal := strings.Join(ctx.Args[1:], " ")
			quoted = &waE2E.Message{
				Conversation: &textVal,
			}
		}

		encoded, err := utils.EncodeProtoMessage(quoted)
		if err != nil {
			return ctx.Replyf("Failed to encode mention message: %v", err)
		}

		_, err = db.Exec(ctx.Ctx, `
			INSERT INTO bot_settings (our_jid, key, value) VALUES ($1, 'mention_proto', $2)
			ON CONFLICT(our_jid, key) DO UPDATE SET value=excluded.value
		`, ourJID, encoded)
		if err != nil {
			return ctx.Reply("Failed to save mention setting.")
		}

		return ctx.Reply("Tag auto-response configured.")

	case "del", "remove":
		_, err := db.Exec(ctx.Ctx, `DELETE FROM bot_settings WHERE our_jid=$1 AND key='mention_proto'`, ourJID)
		if err != nil {
			return ctx.Reply("Failed to delete mention setting.")
		}
		return ctx.Reply("Tag auto-response removed.")

	case "list", "show":
		var mentionProto string
		err := db.QueryRow(ctx.Ctx, `SELECT value FROM bot_settings WHERE our_jid=$1 AND key='mention_proto'`, ourJID).Scan(&mentionProto)
		if err != nil || mentionProto == "" {
			return ctx.Reply("No tag auto-response configured.")
		}
		return ctx.Reply("Tag auto-response is currently configured.")

	default:
		textVal := strings.Join(ctx.Args, " ")
		quoted := &waE2E.Message{
			Conversation: &textVal,
		}

		encoded, err := utils.EncodeProtoMessage(quoted)
		if err != nil {
			return ctx.Replyf("Failed to encode mention message: %v", err)
		}

		_, err = db.Exec(ctx.Ctx, `
			INSERT INTO bot_settings (our_jid, key, value) VALUES ($1, 'mention_proto', $2)
			ON CONFLICT(our_jid, key) DO UPDATE SET value=excluded.value
		`, ourJID, encoded)
		if err != nil {
			return err
		}

		return ctx.Reply("Tag auto-response configured.")
	}
}

func handleAddFilter(ctx *Context) error {
	s, ok := getStore(ctx)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}
	db := s.GetDB()
	if db == nil {
		return ctx.Reply("Database unavailable.")
	}

	ourJID := ctx.Client.Store.ID.ToNonAD().String()

	if len(ctx.Args) == 0 {
		return ctx.Reply("Usage: addfilter [word] [response text] (or reply to a message)")
	}

	trigger := strings.ToLower(ctx.Args[0])
	var responseProtoMsg *waE2E.Message
	quoted := ctx.GetQuotedMessage()

	if quoted != nil {
		responseProtoMsg = quoted
	} else {
		if len(ctx.Args) < 2 {
			return ctx.Reply("Please specify the response text or reply to a message.")
		}
		textVal := strings.Join(ctx.Args[1:], " ")
		responseProtoMsg = &waE2E.Message{
			Conversation: &textVal,
		}
	}

	encoded, err := utils.EncodeProtoMessage(responseProtoMsg)
	if err != nil {
		return ctx.Replyf("Failed to encode filter message: %v", err)
	}

	_, err = db.Exec(ctx.Ctx, `
		INSERT INTO bot_filters (our_jid, trigger_word, message_proto)
		VALUES ($1, $2, $3)
		ON CONFLICT(our_jid, trigger_word) DO UPDATE SET message_proto=excluded.message_proto
	`, ourJID, trigger, encoded)
	if err != nil {
		return ctx.Reply("Failed to save filter.")
	}

	return ctx.Replyf("Filter added for word %q.", trigger)
}

func handleGetFilter(ctx *Context) error {
	s, ok := getStore(ctx)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}
	db := s.GetDB()
	if db == nil {
		return ctx.Reply("Database unavailable.")
	}

	ourJID := ctx.Client.Store.ID.ToNonAD().String()

	if len(ctx.Args) == 0 {
		return ctx.Reply("Usage: getfilter [word]")
	}

	trigger := strings.ToLower(ctx.Args[0])

	var filterProto string
	err := db.QueryRow(ctx.Ctx, `SELECT message_proto FROM bot_filters WHERE our_jid=$1 AND trigger_word=$2`, ourJID, trigger).Scan(&filterProto)
	if err != nil {
		return ctx.Replyf("Filter for word %q not found.", trigger)
	}

	msg, err := utils.DecodeProtoMessage(filterProto)
	if err != nil {
		return ctx.Replyf("Failed to decode filter: %v", err)
	}

	_, err = ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, msg)
	return err
}

func handleListFilters(ctx *Context) error {
	s, ok := getStore(ctx)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}
	db := s.GetDB()
	if db == nil {
		return ctx.Reply("Database unavailable.")
	}

	ourJID := ctx.Client.Store.ID.ToNonAD().String()

	rows, err := db.Query(ctx.Ctx, `SELECT trigger_word FROM bot_filters WHERE our_jid=$1`, ourJID)
	if err != nil {
		return ctx.Reply("Failed to query filters.")
	}
	defer rows.Close()

	var triggers []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err == nil {
			triggers = append(triggers, t)
		}
	}

	if len(triggers) == 0 {
		return ctx.Reply("No filters configured.")
	}
	return ctx.Replyf("Active Filters:\n- %s", strings.Join(triggers, "\n- "))
}

func handleDelFilter(ctx *Context) error {
	s, ok := getStore(ctx)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}
	db := s.GetDB()
	if db == nil {
		return ctx.Reply("Database unavailable.")
	}

	ourJID := ctx.Client.Store.ID.ToNonAD().String()

	if len(ctx.Args) == 0 {
		return ctx.Reply("Usage: delfilter [word]")
	}

	trigger := strings.ToLower(ctx.Args[0])

	res, err := db.Exec(ctx.Ctx, `DELETE FROM bot_filters WHERE our_jid=$1 AND trigger_word=$2`, ourJID, trigger)
	if err != nil {
		return ctx.Reply("Failed to delete filter.")
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ctx.Replyf("Filter for word %q not found.", trigger)
	}
	return ctx.Replyf("Filter for word %q removed.", trigger)
}
