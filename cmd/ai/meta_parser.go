// Response parsing and command-instruction generation for Meta AI requests.
package ai

import (
	"regexp"
	"strings"

	"go.mau.fi/whatsmeow/types"

	"whatsrook"
	"whatsrook/util/external"
)

const metaAiSystemPrompt = `[SYSTEM CONTEXT:
You are {NAME}, an advanced, autonomous agentic AI assistant on WhatsApp.
You operate with full agency, situational awareness, and tool mastery—just like an intelligent agent (Gemini).
You are equipped with a live suite of powerful built-in tools and plugins (listed in "Available bot commands" below) that allow you to take real actions, retrieve live data, process media, manage WhatsApp chats, and execute system tasks.

CORE AGENTIC PRINCIPLES:
1. Proactive Tool Readiness & Execution:
   - You are NOT a passive text-only chatbot. Do not merely give conversational explanations or claim inability when a tool is available to fulfill the user's intent.
   - When a user asks a question, makes a request, gives a task, or expresses an intent that matches the capability of an available command/tool, TAKE INITIATIVE and invoke the tool on their behalf!
   - Trigger a tool by responding with EXACTLY:
     RUN_COMMAND: {PREFIX}<command_name> [args]
     (with no conversational filler, markdown fences, or preamble).

2. Intent-to-Tool Mapping (Be Ready to Act!):
   - Media Downloads & Links: If the user shares or quotes a media link (YouTube, Instagram, TikTok, Twitter/X, Pinterest, Facebook, or direct URL) or asks to download/save/fetch media -> invoke:
     RUN_COMMAND: {PREFIX}dl <url>
   - Stickers & Visuals: If the user asks to create a sticker or quotes an image/video/sticker to convert, crop, or extract metadata -> invoke:
     RUN_COMMAND: {PREFIX}sticker
     (or {PREFIX}crop, {PREFIX}circle, {PREFIX}take as appropriate).
   - Real-Time & Live Lookups: If the user asks for current weather, Wikipedia information, translations, mathematical calculations, time, or dictionary lookups -> invoke:
     RUN_COMMAND: {PREFIX}weather <city>
     RUN_COMMAND: {PREFIX}wiki <query>
     RUN_COMMAND: {PREFIX}calc <expression>
     RUN_COMMAND: {PREFIX}tr <target_lang> <text>
     RUN_COMMAND: {PREFIX}time <location>
   - System Diagnostics & Performance: If the user asks about bot latency, ping, uptime, CPU, or memory status -> invoke:
     RUN_COMMAND: {PREFIX}ping
     (or {PREFIX}status, {PREFIX}cpu).
   - WhatsApp & Group Management: In group chats or management inquiries, if the user asks to mute/unmute, tag members, get group links, delete messages, or manage members -> invoke the matching group tool (e.g., {PREFIX}mute, {PREFIX}tagall, {PREFIX}delete, {PREFIX}link).
   - Plugin & Feature Management: When the user asks to install, uninstall, or list plugins -> invoke:
     RUN_COMMAND: {PREFIX}install <plugin_name>
     RUN_COMMAND: {PREFIX}uninstall <plugin_name>
     RUN_COMMAND: {PREFIX}plugins
   - Command Discovery & Help: When the user asks what features you have, what commands exist, or asks for help/menu -> invoke:
     RUN_COMMAND: {PREFIX}menu
   - System & Shell Administration: If the user is authorized (Status: Owner/Sudo) and asks to run shell commands or evaluate code -> invoke:
     RUN_COMMAND: {PREFIX}sh <command>
     RUN_COMMAND: {PREFIX}eval <code>

3. Multi-Turn Context & Pronoun Resolution:
   - Always maintain full conversational context. When the user uses pronouns or references previous statements (e.g. "yes install it for me", "do it", "install that", "run it", "check the weather there", "download that video"), identify the exact target, entity, URL, plugin, or query from recent conversation history or quoted messages and supply it as [args].
   - Example: If the previous message discusses the weather plugin and the user replies "yes install it for me", you MUST output:
     RUN_COMMAND: {PREFIX}install weather
   - NEVER output a bare command like "RUN_COMMAND: {PREFIX}install" without arguments when the user intended to install, view, or operate on a specific item!

4. Parameter Accuracy:
   - Always supply the required arguments expected by the tool (e.g. plugin name for {PREFIX}install, city for {PREFIX}weather, mathematical expression for {PREFIX}calc, URL for {PREFIX}dl).

5. Conversational Balance:
   - When the user asks a purely conversational, philosophical, conceptual, or creative question that does not require any tool, answer naturally, thoughtfully, intelligently, and directly in plain text without emojis.
   - When mentioning commands in conversation, always use the active prefix '{PREFIX}' (e.g., "{PREFIX}menu", "{PREFIX}install weather", "{PREFIX}help").
   - Address the user by their display name.

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

	cmdsTb := whatsrook.NewText()
	for _, c := range cmds {
		aliasStr := ""
		if c.Alias != "" {
			aliasStr = whatsrook.Sprintf(" (alias: %s%s)", prefix, c.Alias)
		}
		sudoStr := ""
		if !c.IsPublic {
			sudoStr = " [sudo-only]"
		}
		desc := c.Description
		nameLower := strings.ToLower(c.Name)
		switch nameLower {
		case "install":
			desc = whatsrook.Sprintf("Install an official or custom external plugin (`%sinstall <name>` or `%sinstall all`). Official plugins: %s", prefix, prefix, strings.Join(external.OfficialPlugins, ", "))
		case "uninstall":
			desc = whatsrook.Sprintf("Uninstall an extra plugin (`%suninstall <name>` or `%suninstall all`)", prefix, prefix)
		default:
			if len(desc) > 120 {
				desc = desc[:117] + "..."
			}
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
// RUN_COMMAND: whether it appears alone, on its own line, or inside code blocks.
func ParseRunCommand(reply string) (cmdName string, rawArgs string, ok bool) {
	cleaned := strings.TrimSpace(reply)
	cleaned = strings.Trim(cleaned, "` \n\r\t")

	idx := strings.Index(cleaned, "RUN_COMMAND:")
	if idx == -1 {
		return "", "", false
	}

	cmdContent := cleaned[idx+len("RUN_COMMAND:"):]
	firstLine := cmdContent
	if endOfLine := strings.IndexAny(cmdContent, "\r\n"); endOfLine != -1 {
		firstLine = cmdContent[:endOfLine]
	}
	firstLine = strings.TrimSpace(firstLine)

	var cmdLine string
	if firstLine == "" {
		lines := strings.Split(cmdContent, "\n")
		for _, l := range lines {
			l = strings.Trim(strings.TrimSpace(l), "` \r\t")
			if l != "" {
				cmdLine = l
				break
			}
		}
	} else {
		cmdLine = firstLine
	}

	cmdLine = strings.ReplaceAll(cmdLine, "(link unavailable)", "")
	cmdLine = strings.ReplaceAll(cmdLine, "link unavailable", "")
	cmdLine = strings.Trim(cmdLine, "`* \n\r\t")

	// Strip leading prefix punctuation and spaces (. ! / # $ , ; : ~ `)
	cmdLine = strings.TrimLeft(cmdLine, ".!/#$,;:~` \t")

	// Handle optional "cmd:" or "command:" labels (e.g. RUN_COMMAND: cmd: ping)
	lower := strings.ToLower(cmdLine)
	if strings.HasPrefix(lower, "cmd:") {
		cmdLine = strings.TrimLeft(cmdLine[4:], ".!/#$,;:~` \t")
	} else if strings.HasPrefix(lower, "command:") {
		cmdLine = strings.TrimLeft(cmdLine[8:], ".!/#$,;:~` \t")
	}

	fields := strings.Fields(cmdLine)
	if len(fields) == 0 {
		return "", "", false
	}

	// Clean fields[0] of any residual punctuation
	firstFieldClean := strings.Trim(fields[0], ".!/#$,;:~` \t")
	if firstFieldClean == "" {
		if len(fields) > 1 {
			cmdName = strings.ToLower(strings.Trim(fields[1], ".!/#$,;:~` \t"))
			idxAfter := strings.Index(cmdLine, fields[1])
			if idxAfter != -1 {
				rawArgs = strings.TrimSpace(cmdLine[idxAfter+len(fields[1]):])
			}
		} else {
			return "", "", false
		}
	} else {
		cmdName = strings.ToLower(firstFieldClean)
		rawArgs = strings.TrimSpace(cmdLine[len(fields[0]):])
	}

	if cmdName == "" {
		return "", "", false
	}
	return cmdName, rawArgs, true
}

// RenderGroupContext turns GroupInfo into a text block appended to the
// query sent to Meta AI, so it has context about the group without a
// live API call on every message (the caller is expected to have already
// fetched/cached info via GetOrFetchGroupMeta).
func RenderGroupContext(info types.GroupInfo) string {
	name := strings.TrimSpace(info.GroupName.Name)
	if name == "" && info.JID.IsEmpty() && info.ParticipantCount == 0 {
		return ""
	}

	tb := whatsrook.NewText().
		Line("[GROUP CONTEXT]")

	if name != "" {
		tb.Linef("Group Name: %s", name)
	}

	if topic := strings.TrimSpace(info.GroupTopic.Topic); topic != "" {
		if len(topic) > 150 {
			topic = topic[:147] + "..."
		}
		tb.Linef("Group Description: %s", topic)
	}
	if info.ParticipantCount > 0 {
		tb.Linef("Participant Count: %d", info.ParticipantCount)
	}

	tb.Line("[/GROUP CONTEXT]").Blank()
	return tb.String()
}

// RenderUserContext turns user info into a text block appended to the query sent to Meta AI.
func RenderUserContext(d Data) string {
	displayName := strings.TrimSpace(d.PushName)
	if displayName == "" {
		displayName = "User"
	}

	tb := whatsrook.NewText().
		Line("[USER CONTEXT]").
		Linef("User: %s", displayName)

	if d.IsSudo {
		tb.Line("Status: Owner/Sudo")
	}
	tb.Line("Instruction: Address the user in conversation using their name above. Do not output or address them using technical IDs, phone numbers, JIDs, or LIDs.").
		Line("[/USER CONTEXT]").
		Blank()

	return tb.String()
}

// RenderQuotedContext turns quoted-message info on Data into a text block
// giving Meta AI context about what message the user is replying to, if any.
func RenderQuotedContext(d Data) string {
	if d.QuotedMessageOfQuestion == "" && d.QuotedMessageType == "" {
		return ""
	}

	tb := whatsrook.NewText().
		Line("[REPLYING TO A MESSAGE — EXTRACTED CONTEXT]")

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
	tb.Line("[/REPLYING TO A MESSAGE — EXTRACTED CONTEXT]").Blank()

	return tb.String()
}

// RenderCurrentMessage formats the triggering user's current query or message
// so that Meta AI can clearly distinguish dialogue turns and replying context.
func RenderCurrentMessage(d Data) string {
	cleanQuestion := strings.TrimSpace(d.Question)
	displayName := strings.TrimSpace(d.PushName)
	if displayName == "" {
		displayName = "User"
	}

	tb := whatsrook.NewText().
		Line("[CURRENT MESSAGE]")

	tb.Linef("From: %s", displayName)
	if cleanQuestion != "" {
		tb.Linef("Message: %s", cleanQuestion)
	} else {
		tb.Line("Instruction: The user called the bot to respond to the quoted message above. Provide a helpful, direct response.")
	}
	if d.QuotedMessageOfQuestion != "" {
		tb.Line("Note: The user is replying to the quoted message referenced above in this chat.")
	}
	tb.Line("[/CURRENT MESSAGE]").Blank()

	return tb.String()
}

// RenderConversationHistory turns recent multi-turn chat history into a text block for Meta AI.
func RenderConversationHistory(history string) string {
	history = strings.TrimSpace(history)
	if history == "" {
		return ""
	}

	tb := whatsrook.NewText().
		Line("[RECENT CONVERSATION HISTORY]").
		Line(history).
		Line("[/RECENT CONVERSATION HISTORY]").
		Blank()

	return tb.String()
}

// BuildAiQuery compiles instructions, personality prompts, group context,
// quoted message context, and user prompt into a structured query for Meta AI.
func BuildAiQuery(instruction, customPrompt string, data Data) string {
	if isMediaGenerationPrompt(data.Question) {
		return data.Question
	}

	var b strings.Builder
	b.WriteString(instruction)

	if customPrompt != "" {
		b.WriteString("\n[GLOBAL BOT PERSONALITY & RELATIONSHIP BEHAVIOR INSTRUCTION]\n")
		b.WriteString(customPrompt)
		b.WriteString("\n\n")
	}

	if data.ChatType == "group" {
		if grp := RenderGroupContext(data.GroupMetaData); grp != "" {
			b.WriteString(grp)
		}
	}

	if usr := RenderUserContext(data); usr != "" {
		b.WriteString(usr)
	}

	if data.ConversationHistory != "" {
		if conv := RenderConversationHistory(data.ConversationHistory); conv != "" {
			b.WriteString(conv)
		}
	}

	if qtd := RenderQuotedContext(data); qtd != "" {
		b.WriteString(qtd)
	}

	if cur := RenderCurrentMessage(data); cur != "" {
		b.WriteString(cur)
	}

	return b.String()
}

// CleanAiResponseText strips any echoed system prompts, context scaffolding, or prompt delimiters from the AI response.
func CleanAiResponseText(text string) string {
	cleaned := text

	// 1. Strip [SYSTEM CONTEXT: ... ] if echoed by the LLM
	if start := strings.Index(cleaned, "[SYSTEM CONTEXT:"); start != -1 {
		sub := cleaned[start:]
		if _, after, ok := strings.Cut(sub, "\n]"); ok {
			cleaned = cleaned[:start] + after
		} else if _, after, ok := strings.Cut(sub, "]"); ok {
			cleaned = cleaned[:start] + after
		}
	}

	// 2. Strip [GLOBAL BOT PERSONALITY & RELATIONSHIP BEHAVIOR INSTRUCTION]
	if start := strings.Index(cleaned, "[GLOBAL BOT PERSONALITY & RELATIONSHIP BEHAVIOR INSTRUCTION]"); start != -1 {
		sub := cleaned[start:]
		if _, after, ok := strings.Cut(sub, "\n\n"); ok {
			cleaned = cleaned[:start] + after
		}
	}

	// 3. Strip [RECENT CONVERSATION HISTORY] ... [/RECENT CONVERSATION HISTORY]
	if start := strings.Index(cleaned, "[RECENT CONVERSATION HISTORY]"); start != -1 {
		if end := strings.Index(cleaned, "[/RECENT CONVERSATION HISTORY]"); end != -1 {
			cleaned = cleaned[:start] + cleaned[end+len("[/RECENT CONVERSATION HISTORY]"):]
		}
	}

	// 4. Strip [CURRENT MESSAGE] ... [/CURRENT MESSAGE]
	if start := strings.Index(cleaned, "[CURRENT MESSAGE]"); start != -1 {
		if end := strings.Index(cleaned, "[/CURRENT MESSAGE]"); end != -1 {
			cleaned = cleaned[:start] + cleaned[end+len("[/CURRENT MESSAGE]"):]
		}
	}

	// 5. Strip [GROUP CONTEXT] ... [/GROUP CONTEXT]
	if start := strings.Index(cleaned, "[GROUP CONTEXT]"); start != -1 {
		if end := strings.Index(cleaned, "[/GROUP CONTEXT]"); end != -1 {
			cleaned = cleaned[:start] + cleaned[end+len("[/GROUP CONTEXT]"):]
		}
	}

	// 6. Strip [USER CONTEXT] ... [/USER CONTEXT]
	if start := strings.Index(cleaned, "[USER CONTEXT]"); start != -1 {
		if end := strings.Index(cleaned, "[/USER CONTEXT]"); end != -1 {
			cleaned = cleaned[:start] + cleaned[end+len("[/USER CONTEXT]"):]
		}
	}

	// 7. Strip [REPLYING TO A MESSAGE — EXTRACTED CONTEXT] ... [/REPLYING TO A MESSAGE — EXTRACTED CONTEXT]
	if start := strings.Index(cleaned, "[REPLYING TO A MESSAGE — EXTRACTED CONTEXT]"); start != -1 {
		if end := strings.Index(cleaned, "[/REPLYING TO A MESSAGE — EXTRACTED CONTEXT]"); end != -1 {
			cleaned = cleaned[:start] + cleaned[end+len("[/REPLYING TO A MESSAGE — EXTRACTED CONTEXT]"):]
		}
	}

	// 8. Strip [QUOTED MESSAGE] ... [/QUOTED MESSAGE]
	if start := strings.Index(cleaned, "[QUOTED MESSAGE]"); start != -1 {
		if end := strings.Index(cleaned, "[/QUOTED MESSAGE]"); end != -1 {
			cleaned = cleaned[:start] + cleaned[end+len("[/QUOTED MESSAGE]"):]
		}
	}

	// 9. Strip any stray context tags
	strayTags := []string{
		"[/RECENT CONVERSATION HISTORY]",
		"[RECENT CONVERSATION HISTORY]",
		"[/CURRENT MESSAGE]",
		"[/GROUP CONTEXT]",
		"[/USER CONTEXT]",
		"[/REPLYING TO A MESSAGE — EXTRACTED CONTEXT]",
		"[/QUOTED MESSAGE]",
		"[CURRENT MESSAGE]",
		"[GROUP CONTEXT]",
		"[USER CONTEXT]",
		"[REPLYING TO A MESSAGE — EXTRACTED CONTEXT]",
		"[QUOTED MESSAGE]",
	}
	for _, tag := range strayTags {
		cleaned = strings.ReplaceAll(cleaned, tag, "")
	}

	cleaned = StripMarkdown(cleaned)

	return strings.TrimSpace(cleaned)
}

var (
	reCodeBlock        = regexp.MustCompile("(?s)```[a-zA-Z0-9_-]*\\n?(.*?)\\n?```")
	reUnclosedCodeOpen = regexp.MustCompile("```[a-zA-Z0-9_-]*\\n?")
	reInlineCode       = regexp.MustCompile("`([^`\n]+)`")
	reHeader           = regexp.MustCompile(`(?m)^[ \t]*#{1,6}[ \t]+`)
	reBlockquote       = regexp.MustCompile(`(?m)^[ \t]*>[ \t]?`)
	reHorizontalRule   = regexp.MustCompile(`(?m)^[ \t]*[-*_]{3,}[ \t]*$`)
	reBoldItalic1      = regexp.MustCompile(`\*\*\*([^*]+)\*\*\*`)
	reBoldItalic2      = regexp.MustCompile(`___([^_]+)___`)
	reBold1            = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	reBold2            = regexp.MustCompile(`__([^_]+)__`)
	reItalic1          = regexp.MustCompile(`\*([^*\n]+)\*`)
	reItalic2          = regexp.MustCompile(`_([^_\n]+)_`)
	reStrike1          = regexp.MustCompile(`~~([^~]+)~~`)
	reStrike2          = regexp.MustCompile(`~([^~\n]+)~`)
	reBullet           = regexp.MustCompile(`(?m)^[ \t]*[\*\+][ \t]+`)
	reMdImage          = regexp.MustCompile(`!\[([^\]]*)\]\([^)]+\)`)
	reMdLink           = regexp.MustCompile(`\[([^\]]+)\]\(([^)]*)\)`)
	reHTMLTags         = regexp.MustCompile(`</?[a-zA-Z][a-zA-Z0-9]*[^>]*>`)
	reExtraNewlines    = regexp.MustCompile(`\n{3,}`)
)

// StripMarkdown strips all markdown formatting from the given text, returning plain text.
func StripMarkdown(text string) string {
	if text == "" {
		return ""
	}

	// 1. Strip images: ![alt](url) -> alt
	text = reMdImage.ReplaceAllString(text, "$1")

	// 2. Format links: [text](url) -> text (url) or text
	text = reMdLink.ReplaceAllStringFunc(text, func(m string) string {
		sub := reMdLink.FindStringSubmatch(m)
		if len(sub) < 3 {
			return m
		}
		title := strings.TrimSpace(sub[1])
		url := strings.TrimSpace(sub[2])
		if url == "" || strings.EqualFold(url, "link unavailable") || strings.EqualFold(url, "(link unavailable)") {
			return title
		}
		if title == "" || title == url || strings.HasPrefix(url, title) {
			return url
		}
		return title + " (" + url + ")"
	})
	text = strings.ReplaceAll(text, "(link unavailable)", "")
	text = strings.ReplaceAll(text, "link unavailable", "")

	// 3. Code blocks: extract content, strip fence
	text = reCodeBlock.ReplaceAllString(text, "$1")
	text = reUnclosedCodeOpen.ReplaceAllString(text, "")
	text = strings.ReplaceAll(text, "```", "")

	// 4. Inline code
	text = reInlineCode.ReplaceAllString(text, "$1")
	text = strings.ReplaceAll(text, "`", "")

	// 5. Headers: # Title -> Title
	text = reHeader.ReplaceAllString(text, "")

	// 6. Blockquotes: > quote -> quote
	text = reBlockquote.ReplaceAllString(text, "")

	// 7. Horizontal rules: --- -> ""
	text = reHorizontalRule.ReplaceAllString(text, "")

	// 8. Convert asterisk/plus bullets to standard hyphen bullets before stripping *
	text = reBullet.ReplaceAllString(text, "- ")

	// 9. Bold, Italic, Strikethrough
	text = reBoldItalic1.ReplaceAllString(text, "$1")
	text = reBoldItalic2.ReplaceAllString(text, "$1")
	text = reBold1.ReplaceAllString(text, "$1")
	text = reBold2.ReplaceAllString(text, "$1")
	text = reItalic1.ReplaceAllString(text, "$1")
	text = reItalic2.ReplaceAllString(text, "$1")
	text = reStrike1.ReplaceAllString(text, "$1")
	text = reStrike2.ReplaceAllString(text, "$1")

	// 10. Stray asterisks or tildes
	text = strings.ReplaceAll(text, "**", "")
	text = strings.ReplaceAll(text, "~~", "")
	text = strings.ReplaceAll(text, "*", "")
	text = strings.ReplaceAll(text, "~", "")

	// 11. HTML tags
	text = reHTMLTags.ReplaceAllString(text, "")

	// 12. Normalize extra newlines
	text = reExtraNewlines.ReplaceAllString(text, "\n\n")

	return strings.TrimSpace(text)
}
