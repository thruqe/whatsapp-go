// Response parsing and command-instruction generation for Meta AI requests.
package cliutils

import (
	_ "embed"
	"fmt"
	"os"
	"strings"

	stripmd "github.com/writeas/go-strip-markdown/v2"
	"go.mau.fi/whatsmeow/types"
)

//go:embed prompts/meta_ai.txt
var embeddedMetaAiPrompt string

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
func BuildRunCommandInstruction(cmds []CommandInfo) string {
	return BuildRunCommandInstructionWithName(cmds, "WhatsRook")
}

func BuildRunCommandInstructionWithName(cmds []CommandInfo, botName string) string {
	if botName == "" {
		botName = "WhatsRook"
	}
	promptTmpl := embeddedMetaAiPrompt
	if data, err := os.ReadFile("prompts/meta_ai.txt"); err == nil && len(data) > 0 {
		promptTmpl = string(data)
	} else if data, err := os.ReadFile("meta/prompts/meta_ai.txt"); err == nil && len(data) > 0 {
		promptTmpl = string(data)
	}

	promptTmpl = strings.ReplaceAll(promptTmpl, "{NAME}", botName)
	promptTmpl = strings.ReplaceAll(promptTmpl, "WhatsRook", botName)

	var cmdsBuf strings.Builder
	for _, c := range cmds {
		fmt.Fprintf(&cmdsBuf, "- !%s", c.Name)
		if c.Alias != "" {
			fmt.Fprintf(&cmdsBuf, " (alias: %s)", c.Alias)
		}
		if !c.IsPublic {
			cmdsBuf.WriteString(" [sudo-only]")
		}
		cmdsBuf.WriteString(": ")
		cmdsBuf.WriteString(c.Description)
		cmdsBuf.WriteString("\n")
	}

	res := strings.ReplaceAll(promptTmpl, "{{COMMANDS_LIST}}", strings.TrimRight(cmdsBuf.String(), "\n"))
	return res + "\n\n"
}

// ParseRunCommand checks whether an AI reply is requesting that the bot
// run one of its own registered commands, using the convention:
//
//	RUN_COMMAND: !<command_name> [args...]
//
// It returns the command name (lowercased) and its raw argument string,
// and ok=true if the reply matched this convention. This only recognizes
// the fixed marker text — it does not interpret, generate, or execute
// anything itself; the caller is responsible for looking the command name
// up in its own registry and deciding whether to run it.
func ParseRunCommand(reply string) (cmdName string, rawArgs string, ok bool) {
	cleaned := strings.TrimSpace(reply)
	cmdContent, found := strings.CutPrefix(cleaned, "RUN_COMMAND:")
	if !found {
		return "", "", false
	}

	cmdLine := strings.TrimSpace(cmdContent)
	cmdLine = strings.ReplaceAll(cmdLine, "(link unavailable)", "")
	cmdLine = strings.ReplaceAll(cmdLine, "link unavailable", "")
	cmdLine = strings.TrimSpace(cmdLine)
	cmdLine = strings.TrimLeft(cmdLine, ".!/ ")

	fields := strings.Fields(cmdLine)
	if len(fields) == 0 {
		return "", "", false
	}

	cmdName = strings.ToLower(fields[0])
	rawArgs = strings.TrimSpace(cmdLine[len(fields[0]):])
	return cmdName, rawArgs, true
}

// AnswerParserString converts an AI-generated response written in Markdown
// into plain, unformatted text.
//
// It strips common Markdown syntax — headers, emphasis (bold/italic),
// strikethrough, inline and fenced code blocks, links (keeping the link
// text, dropping the URL), images (keeping alt text), blockquotes, and
// list markers — leaving only the underlying human-readable text.
//
// The input pointer is mutated in place: *ai_response_string is replaced
// with its plain-text form.
func AnswerParserString(ai_response_string *string) {
	if ai_response_string == nil {
		return
	}

	plain := stripmd.Strip(*ai_response_string)
	plain = strings.TrimSpace(plain)

	*ai_response_string = plain
}

// RenderGroupContext turns GroupInfo into a text block appended to the
// query sent to Meta AI, so it has context about the group without a
// live API call on every message (the caller is expected to have already
// fetched/cached info via GetOrFetchGroupMeta).
func RenderGroupContext(info types.GroupInfo) string {
	var b strings.Builder
	b.WriteString("[GROUP CONTEXT]\n")
	fmt.Fprintf(&b, "Group name: %s\n", info.GroupName.Name)
	if info.GroupTopic.Topic != "" {
		fmt.Fprintf(&b, "Group description: %s\n", info.GroupTopic.Topic)
	}
	fmt.Fprintf(&b, "Participant count: %d\n", info.ParticipantCount)

	var admins []string
	for _, p := range info.Participants {
		if p.IsAdmin || p.IsSuperAdmin {
			admins = append(admins, p.JID.User)
		}
	}
	if len(admins) > 0 {
		fmt.Fprintf(&b, "Admins: %s\n", strings.Join(admins, ", "))
	}
	b.WriteString("[/GROUP CONTEXT]\n\n")
	return b.String()
}

// RenderUserContext turns user info into a text block appended to the query sent to Meta AI.
func RenderUserContext(d Data) string {
	if d.PushName == "" && d.User.User == "" && d.MessageID == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("[USER & MESSAGE OBJECT CONTEXT]\n")
	displayName := d.PushName
	if displayName == "" {
		displayName = "User"
	}
	fmt.Fprintf(&b, "User name: %s\n", displayName)
	if d.MessageID != "" {
		fmt.Fprintf(&b, "Message ID: %s\n", d.MessageID)
	}
	if d.IsSudo {
		b.WriteString("Status: Owner/Sudo\n")
	}
	b.WriteString("Instruction: Address the user in conversation using their User name above. Do not output or address them using technical IDs, phone numbers, JIDs, or LIDs.\n")
	b.WriteString("[/USER & MESSAGE OBJECT CONTEXT]\n\n")
	return b.String()
}

// RenderQuotedContext turns quoted-message info on Data into a text block
// giving Meta AI context about what message the user is replying to, if
// any.
func RenderQuotedContext(d Data) string {
	if d.QuotedMessageOfQuestion == "" && d.QuotedImageBase64 == "" && d.QuotedMessageType == "" && d.QuotedMessageID == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("[REPLYING TO A MESSAGE — EXTRACTED CONTEXT]\n")
	if d.QuotedMessageID != "" {
		fmt.Fprintf(&b, "Quoted Message ID: %s\n", d.QuotedMessageID)
	}
	if d.UserOfQuotedMessage != "" {
		fmt.Fprintf(&b, "From: %s", d.UserOfQuotedMessage)
		if d.QuotedMessageParticipantRole != "" {
			b.WriteString(fmt.Sprintf(" (%s)", d.QuotedMessageParticipantRole))
		}
		b.WriteString("\n")
	}
	if d.QuotedMessageType != "" {
		fmt.Fprintf(&b, "Message Type: %s\n", d.QuotedMessageType)
	}
	if d.QuotedMessageOfQuestion != "" {
		fmt.Fprintf(&b, "Message Content: %s\n", d.QuotedMessageOfQuestion)
	}
	if d.QuotedImageBase64 != "" {
		fmt.Fprintf(&b, "Image Base64: data:%s;base64,%s\n", d.QuotedImageMimeType, d.QuotedImageBase64)
	}
	b.WriteString("[/REPLYING TO A MESSAGE — EXTRACTED CONTEXT]\n\n")
	return b.String()
}
