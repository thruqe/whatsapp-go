// Response parsing and command-instruction generation for Meta AI requests.
package ai

import (
	"strings"

	"go.mau.fi/whatsmeow/types"

	utils "whatsrook"
)

const metaAiSystemPrompt = `[SYSTEM CONTEXT:
You are {NAME}, a helpful, intelligent, and capable AI assistant on WhatsApp.
The bot has registered commands available to perform actions. When the user asks to run a command, execute an action, or confirms with 'yes' (for example: "run the menu command", "open menu", "show commands", "check ping", "search repo", "yes"), you must trigger the command by responding with EXACTLY:
RUN_COMMAND: {PREFIX}<command_name> [args]
(with no other text, markdown, or commentary).

When the user is chatting, asking a general knowledge question, or having a conversation, answer naturally, thoughtfully, and directly.
Do NOT use emojis anywhere in your responses.
When referencing commands in natural conversation, always use the active prefix '{PREFIX}' (for example: "{PREFIX}menu", "{PREFIX}help", "{PREFIX}ping").
Address the user by their display name.

Available bot commands:
{{COMMANDS_LIST}}
]`

// CommandInfo mirrors commands.CommandInfo — kept as a separate type here
// so meta has no import dependency on the commands package (which
// would create an import cycle, since commands imports meta).
type CommandInfo struct {
	Name        string
	Alias       string
	Description string
	IsPublic    bool
}

// BuildRunCommandInstruction builds the instruction block prepended to
// every Meta AI request, listing the bot's actual registered commands so
// Meta AI can both (a) decide to invoke one via RUN_COMMAND, and (b)
// answer questions about how to use a command, using real data instead
// of guessing.
func BuildRunCommandInstructionWithNameAndPrefix(cmds []CommandInfo, botName, prefix string) string {
	if botName == "" {
		botName = "WhatsRook"
	}
	if prefix == "" {
		prefix = "!"
	}
	promptTmpl := metaAiSystemPrompt

	promptTmpl = strings.ReplaceAll(promptTmpl, "{NAME}", botName)
	promptTmpl = strings.ReplaceAll(promptTmpl, "WhatsRook", botName)
	promptTmpl = strings.ReplaceAll(promptTmpl, "{PREFIX}", prefix)

	cmdsTb := utils.NewText()
	for _, c := range cmds {
		aliasStr := ""
		if c.Alias != "" {
			aliasStr = utils.Sprintf(" (alias: %s%s)", prefix, c.Alias)
		}
		sudoStr := ""
		if !c.IsPublic {
			sudoStr = " [sudo-only]"
		}
		desc := c.Description
		if len(desc) > 80 {
			desc = desc[:77] + "..."
		}
		cmdsTb.Linef("- %s%s%s%s: %s", prefix, c.Name, aliasStr, sudoStr, desc)
	}

	res := strings.ReplaceAll(promptTmpl, "{{COMMANDS_LIST}}", cmdsTb.Trimmed())
	return res + "\n\n"
}

// ParseRunCommand checks whether an AI reply is requesting that the bot
// run one of its own registered commands, using the convention:
//
//	RUN_COMMAND: <prefix><command_name> [args...]
//
// It returns the command name (lowercased) and its raw argument string,
// and ok=true if the reply matched this convention. This only recognizes
// the fixed marker text — it does not interpret, generate, or execute
// anything itself; the caller is responsible for looking the command name
// up in its own registry and deciding whether to run it.
func ParseRunCommand(reply string) (cmdName string, rawArgs string, ok bool) {
	cleaned := strings.TrimSpace(reply)
	cleaned = strings.Trim(cleaned, "` \n\r\t")
	cmdContent, found := strings.CutPrefix(cleaned, "RUN_COMMAND:")
	if !found {
		return "", "", false
	}

	cmdLine := strings.TrimSpace(cmdContent)
	cmdLine = strings.ReplaceAll(cmdLine, "(link unavailable)", "")
	cmdLine = strings.ReplaceAll(cmdLine, "link unavailable", "")
	cmdLine = strings.Trim(cmdLine, "` \n\r\t")
	cmdLine = strings.TrimLeft(cmdLine, ".!/# ")

	fields := strings.Fields(cmdLine)
	if len(fields) == 0 {
		return "", "", false
	}

	cmdName = strings.ToLower(fields[0])
	rawArgs = strings.TrimSpace(cmdLine[len(fields[0]):])
	return cmdName, rawArgs, true
}

// RenderGroupContext turns GroupInfo into a text block appended to the
// query sent to Meta AI, so it has context about the group without a
// live API call on every message (the caller is expected to have already
// fetched/cached info via GetOrFetchGroupMeta).
func RenderGroupContext(info types.GroupInfo) string {
	tb := utils.NewText().
		Line("[GROUP CONTEXT]").
		Linef("Group name: %s", info.GroupName.Name)

	if topic := strings.TrimSpace(info.GroupTopic.Topic); topic != "" {
		if len(topic) > 150 {
			topic = topic[:147] + "..."
		}
		tb.Linef("Group description: %s", topic)
	}
	tb.Linef("Participant count: %d", info.ParticipantCount)

	var admins []string
	for _, p := range info.Participants {
		if p.IsAdmin || p.IsSuperAdmin {
			admins = append(admins, p.JID.User)
			if len(admins) >= 5 {
				break
			}
		}
	}
	if len(admins) > 0 {
		tb.Linef("Admins: %s", strings.Join(admins, ", "))
	}
	tb.Line("[/GROUP CONTEXT]").Blank()
	return tb.String()
}

// RenderUserContext turns user info into a text block appended to the query sent to Meta AI.
func RenderUserContext(d Data) string {
	if d.PushName == "" && d.User.User == "" && d.MessageID == "" {
		return ""
	}
	displayName := d.PushName
	if displayName == "" {
		displayName = "User"
	}

	tb := utils.NewText().
		Line("[USER & MESSAGE OBJECT CONTEXT]").
		Linef("User name: %s", displayName)

	if d.MessageID != "" {
		tb.Linef("Message ID: %s", d.MessageID)
	}
	if d.IsSudo {
		tb.Line("Status: Owner/Sudo")
	}
	tb.Line("Instruction: Address the user in conversation using their User name above. Do not output or address them using technical IDs, phone numbers, JIDs, or LIDs.").
		Line("[/USER & MESSAGE OBJECT CONTEXT]").
		Blank()

	return tb.String()
}

// RenderQuotedContext turns quoted-message info on Data into a text block
// giving Meta AI context about what message the user is replying to, if
// any.
func RenderQuotedContext(d Data) string {
	if d.QuotedMessageOfQuestion == "" && d.QuotedImageBase64 == "" && d.QuotedMessageType == "" && d.QuotedMessageID == "" {
		return ""
	}

	tb := utils.NewText().
		Line("[REPLYING TO A MESSAGE — EXTRACTED CONTEXT]")

	if d.QuotedMessageID != "" {
		tb.Linef("Quoted Message ID: %s", d.QuotedMessageID)
	}
	if d.UserOfQuotedMessage != "" {
		if d.QuotedMessageParticipantRole != "" {
			tb.Linef("From: %s (%s)", d.UserOfQuotedMessage, d.QuotedMessageParticipantRole)
		} else {
			tb.Linef("From: %s", d.UserOfQuotedMessage)
		}
	}
	if d.QuotedMessageType != "" {
		tb.Linef("Message Type: %s", d.QuotedMessageType)
	}
	if d.QuotedMessageOfQuestion != "" {
		msgContent := d.QuotedMessageOfQuestion
		if len(msgContent) > 500 {
			msgContent = msgContent[:497] + "..."
		}
		tb.Linef("Message Content: %s", msgContent)
	}
	if d.QuotedImageBase64 != "" && len(d.QuotedImageBase64) <= 2048 {
		tb.Linef("Image Base64: data:%s;base64,%s", d.QuotedImageMimeType, d.QuotedImageBase64)
	}
	tb.Line("[/REPLYING TO A MESSAGE — EXTRACTED CONTEXT]").Blank()
	return tb.String()
}
