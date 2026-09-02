package owner

import (
	"bytes"
	"context"
	"io"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	utils "whatsrook"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

func ExtractMediaFromEvent(evt *events.Message) (whatsmeow.DownloadableMessage, bool, string) {
	if evt == nil || evt.Message == nil {
		return nil, false, ""
	}
	msg := utils.UnwrapMessageProto(evt.Message)
	if msg == nil {
		return nil, false, ""
	}
	if img := msg.GetImageMessage(); img != nil {
		return img, false, img.GetMimetype()
	}
	if vid := msg.GetVideoMessage(); vid != nil {
		return vid, true, vid.GetMimetype()
	}
	return nil, false, ""
}

var (
	StatusBroadcastJID = types.JID{User: "status", Server: "broadcast"}

	ActiveShellSessions   = make(map[string]*ShellSession)
	ActiveShellSessionsMu sync.Mutex
	AnsiEscapeRegex       = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]|\x1b\([a-zA-Z]|\x1b\][0-9];[^\a\x1b]*(?:\a|\x1b\\)`)
)

type ShellSession struct {
	Chat           types.JID
	Sender         types.JID
	MsgID          types.MessageID
	Cmd            *exec.Cmd
	Stdin          io.WriteCloser
	Cancel         context.CancelFunc
	Buf            *bytes.Buffer
	Mu             sync.Mutex
	StartTime      time.Time
	CommandStr     string
	UpdateCh       chan struct{}
	Done           chan struct{}
	UserTerminated bool
}

func CleanShellOutput(raw string) string {
	cleaned := AnsiEscapeRegex.ReplaceAllString(raw, "")
	lines := strings.Split(cleaned, "\n")
	var resultLines []string
	for _, line := range lines {
		if strings.Contains(line, "\r") {
			parts := strings.Split(line, "\r")
			var last string
			for _, part := range slices.Backward(parts) {
				trimmed := strings.TrimRight(part, " \t")
				if trimmed != "" {
					last = trimmed
					break
				}
			}
			if last != "" {
				resultLines = append(resultLines, last)
			}
		} else {
			resultLines = append(resultLines, line)
		}
	}
	return strings.Join(resultLines, "\n")
}
