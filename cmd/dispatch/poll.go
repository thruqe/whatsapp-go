package dispatch

import (
	"context"
	"strconv"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"whatsrook/builder"
	Logger "whatsrook/logger"
)

// MapOptionToCommandArgs maps a plain poll option label back to its intended subcommand payload.
func MapOptionToCommandArgs(cmdName, option string) string {
	optTrim := strings.TrimSpace(option)
	optLower := strings.ToLower(optTrim)

	// Clean emojis like ▶️, ◀️
	cleaned := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(optLower, "▶️", ""), "◀️", ""))

	// If option contains font indicator e.g. "(#14)"
	if strings.Contains(optLower, "(#") {
		idx := strings.Index(optLower, "(#")
		end := strings.Index(optLower[idx:], ")")
		if end > 2 {
			fontNum := strings.TrimSpace(optLower[idx+2 : idx+end])
			return cmdName + " " + fontNum
		}
	}

	// If option starts with a number like "1. Option Name"
	if strings.Contains(optLower, ".") {
		parts := strings.SplitN(optLower, ".", 2)
		if idx, err := strconv.Atoi(strings.TrimSpace(parts[0])); err == nil && idx > 0 {
			switch cmdName {
			case "timezone":
				return "timezone setidx " + strconv.Itoa(idx)
			case "csai":
				return "csai " + strconv.Itoa(idx)
			case "why":
				rest := strings.TrimSpace(parts[1])
				if rest != "" {
					return "why " + rest
				}
			case "setbot":
				rest := strings.TrimSpace(parts[1])
				fields := strings.Fields(rest)
				if len(fields) > 0 {
					return "setbot " + fields[0]
				}
				return "setbot " + strconv.Itoa(idx)
			default:
				return cmdName + " " + strings.TrimSpace(parts[1])
			}
		}
	}

	// Page navigation
	if strings.HasPrefix(cleaned, "next (page ") || strings.HasPrefix(cleaned, "page ") {
		if _, after, ok := strings.Cut(cleaned, "page "); ok {
			numStr := strings.TrimRight(strings.TrimSpace(after), ")")
			return cmdName + " page " + numStr
		}
	}
	if cleaned == "first page" {
		return cmdName + " page 1"
	}
	if cleaned == "next" {
		if cmdName == "setbot" {
			return "setbot page 2"
		}
		return cmdName + " next"
	}
	if cleaned == "back" {
		if cmdName == "setbot" {
			return "setbot page 1"
		}
		return cmdName + " back"
	}

	switch cleaned {
	case "1 min", "1 minute":
		return cmdName + " time 60"
	case "2 mins", "2 minutes":
		return cmdName + " time 120"
	case "3 mins", "3 minutes":
		return cmdName + " time 180"
	case "activate":
		return cmdName + " activate"
	case "deactivate":
		return cmdName + " deactivate"
	case "action mode":
		return cmdName + " action"
	case "customize", "customize mode":
		switch cmdName {
		case "antilink":
			return "antilink mode"
		case "vv":
			return "vv customize"
		default:
			return cmdName + " customize"
		}
	case "delete only":
		return cmdName + " action delete"
	case "kick user":
		return cmdName + " action kick"
	case "warn user":
		return cmdName + " action warn"
	case "default links":
		return cmdName + " default"
	case "custom urls":
		return cmdName + " custom"
	case "set limit (3)":
		return cmdName + " setwarn 3"
	case "target list", "list words", "list targets":
		return cmdName + " list"
	case "clear targets":
		return cmdName + " clear"
	case "set timeout":
		return cmdName + " time"
	case "help / guide", "guide", "format guide", "help":
		return cmdName + " guide"
	case "scope: all", "all chats":
		return cmdName + " scope all"
	case "scope: group", "groups only":
		return cmdName + " scope group"
	case "scope: dm", "dm only":
		return cmdName + " scope dm"
	case "status view: on":
		return cmdName + " status on"
	case "status view: off":
		return cmdName + " status off"
	case "set emojis":
		return cmdName + " emoji"
	case "preview":
		return cmdName + " preview"
	case "change timezone":
		return "timezone"
	case "set owner dm":
		return "vv dm"
	case "switch to dm (private)":
		return cmdName + " mode dm"
	case "switch to public (chat)":
		return cmdName + " mode public"
	case "call audio", "audio call":
		return "callaudio"
	case "call video", "video call":
		return "callvideo"
	case "voicemail":
		return "voicemail"
	case "start match":
		return cmdName + " start"
	case "leaderboard":
		return cmdName + " leaderboard"
	case "confirm leave":
		return "leave confirm"
	case "cancel":
		return cmdName + " cancel"
	case "wizard", "start wizard", "customize bot":
		return "reconfigure"
	case "keep default":
		return "setbot setup_ignore"
	case "skip":
		return "setbot skip"
	case "done", "i'm done", "finish":
		return "setbot done"
	case "start over", "restart", "redo":
		return "setbot startover"
	case "continue":
		return "continue"
	default:
		if cmdName != "" {
			return cmdName + " " + cleaned
		}
		return cleaned
	}
}

// SendPollReplyWithMentions creates an interactive poll reply quoting the triggering message.
// If no custom callback fn is provided, it automatically registers a reactive route that maps
// selected options to command invocations and executes them.
func SendPollReplyWithMentions(ctx *Context, question string, options []string, jids []types.JID, fn ...func(req PollRequest, res *builder.Response)) error {
	cmdName := ctx.Command
	Logger.Debug("SendPollReplyWithMentions: creating poll reply",
		"command", cmdName,
		"question", question,
		"optionsCount", len(options),
		"options", options,
		"mentionsCount", len(jids),
		"hasCallback", len(fn) > 0 && fn[0] != nil,
	)
	poll := ctx.Poll(question).Mentions(jids...)
	for _, opt := range options {
		poll.AddOption(opt)
	}
	if len(fn) > 0 && fn[0] != nil {
		return poll.Reply(fn[0])
	}

	client := ctx.Client

	return poll.Reply(func(req PollRequest, res *builder.Response) {
		for _, selected := range req.SelectedOptions {
			cmdLine := MapOptionToCommandArgs(cmdName, selected)
			Logger.Debug("Interactive poll selection triggered",
				"command", cmdName,
				"selected", selected,
				"dispatchedCommand", cmdLine,
				"sender", req.Sender.String(),
				"chat", req.Chat.String(),
			)
			if cmdLine == "" || strings.TrimSpace(cmdLine) == cmdName {
				continue
			}
			fakeEvt := &events.Message{
				Info: types.MessageInfo{
					Chat:      req.Chat,
					Sender:    req.Sender,
					IsGroup:   req.Chat.Server == "g.us",
					ID:        req.PollMsgID,
					Timestamp: time.Now(),
				},
				Message: &waE2E.Message{
					Conversation: &cmdLine,
				},
			}
			voteCtx := req.Ctx
			if voteCtx == nil || voteCtx.Err() != nil {
				voteCtx = context.Background()
			}
			runCommand(voteCtx, client, fakeEvt, cmdLine)
		}
	})
}

// SendPollReply creates an interactive poll reply quoting the triggering message with automatic command routing.
func SendPollReply(ctx *Context, question string, options []string, fn ...func(req PollRequest, res *builder.Response)) error {
	return SendPollReplyWithMentions(ctx, question, options, nil, fn...)
}

func init() {
	builder.SetDefaultPollCallback(func(req builder.PollRequest, res *builder.Response) {
		Logger.Debug("Default poll callback received vote",
			"pollMsgID", req.PollMsgID,
			"sender", req.Sender.String(),
			"chat", req.Chat.String(),
			"selectedOptions", req.SelectedOptions,
		)
	})
}
