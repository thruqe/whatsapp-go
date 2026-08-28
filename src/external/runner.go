package external

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"

	utils "whatsrook/src"
	Logger "whatsrook/src/logger"

	"go.mau.fi/whatsmeow/types"
)

// runProcess launches an external plugin executable and handles both streaming action frames and plain text stdout.
func (d *Dispatcher) runProcess(plugCtx *utils.PluginContext, path, name string, request Request) {
	liveCtx, liveCancel := context.WithTimeout(context.Background(), d.liveTimeout)
	defer liveCancel()

	// Update plugCtx context to liveCtx for outbound message sending
	plugCtx.Ctx = liveCtx

	sessionKey := d.sessionKey(request.Chat, name)

	// Start special animated external loader (activates after 350ms if plugin is still working)
	loader := startLoader(plugCtx, name, 350*time.Millisecond)
	defer loader.Delete()

	cmd := exec.CommandContext(liveCtx, path)
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		loader.Delete()
		Logger.Error("external plugin stdout pipe failed", "plugin", name, "err", err)
		return
	}
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		loader.Delete()
		Logger.Error("external plugin stdin pipe failed", "plugin", name, "err", err)
		return
	}

	if err := cmd.Start(); err != nil {
		loader.Delete()
		Logger.Error("external plugin start failed", "plugin", name, "err", err)
		_ = plugCtx.Replyf("Failed to start external plugin %q: %v", name, err)
		return
	}

	// Register live session for cancel command handling
	d.registerSession(sessionKey, liveCancel, name)

	defer func() {
		_ = stdinPipe.Close()
		_ = cmd.Wait()
		d.unregisterSession(sessionKey)
	}()

	// Serialize and write initial Request JSON line to stdin
	reqBytes, _ := json.Marshal(request)
	_, _ = fmt.Fprintf(stdinPipe, "%s\n", reqBytes)

	scanner := bufio.NewScanner(stdoutPipe)
	var firstLine string
	var isStreaming bool
	var readFirst bool

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if !readFirst {
			readFirst = true
			firstLine = line
			if strings.HasPrefix(trimmed, "{\"action\"") {
				isStreaming = true
			} else {
				// Plain text mode: close stdinPipe immediately so plugin receives EOF on stdin
				isStreaming = false
				_ = stdinPipe.Close()
				break
			}
		}

		if isStreaming {
			if err := d.handleActionFrame(plugCtx, loader, stdinPipe, line); err != nil {
				Logger.Debug("external plugin streaming action finished", "plugin", name, "err", err)
				break
			}
		}
	}

	if !isStreaming && readFirst {
		var sb strings.Builder
		sb.WriteString(firstLine)
		for scanner.Scan() {
			sb.WriteByte('\n')
			sb.WriteString(scanner.Text())
		}
		response := strings.TrimSpace(sb.String())
		if response != "" {
			_ = loader.Done(response)
		} else {
			loader.Delete()
		}
	}
}

// handleActionFrame processes a single action frame emitted by an external plugin on stdout.
func (d *Dispatcher) handleActionFrame(ctx *utils.PluginContext, loader *Loader, stdinPipe io.WriteCloser, line string) error {
	var frame Action
	if err := json.Unmarshal([]byte(line), &frame); err != nil {
		return err
	}

	switch frame.Action {
	case "reply":
		msgID, err := loader.DoneWithReply(frame.Text)
		d.sendAck(stdinPipe, err == nil, string(msgID), err)

	case "edit":
		loader.Stop()
		if frame.MsgID != "" && frame.Text != "" {
			_, _ = ctx.Edit(types.MessageID(frame.MsgID), frame.Text)
		}

	case "react":
		if frame.Emoji != "" {
			_ = ctx.React(frame.Emoji)
		}

	case "delete", "revoke":
		loader.Delete()
		if frame.MsgID != "" {
			_, _ = ctx.Delete(types.MessageID(frame.MsgID))
		}

	case "send_image":
		loader.Delete()
		data, err := resolveMediaData(frame.Data)
		if err != nil {
			d.sendAck(stdinPipe, false, "", err)
			return nil
		}
		mime := frame.MimeType
		if mime == "" {
			mime = "image/png"
		}
		err = ctx.ReplyWithImage(data, mime, frame.Caption)
		d.sendAck(stdinPipe, err == nil, "", err)

	case "send_audio":
		loader.Delete()
		data, err := resolveMediaData(frame.Data)
		if err != nil {
			d.sendAck(stdinPipe, false, "", err)
			return nil
		}
		mime := frame.MimeType
		if mime == "" {
			mime = "audio/ogg; codecs=opus"
		}
		err = ctx.ReplyWithAudio(data, mime)
		d.sendAck(stdinPipe, err == nil, "", err)

	case "send_video":
		loader.Delete()
		data, err := resolveMediaData(frame.Data)
		if err != nil {
			d.sendAck(stdinPipe, false, "", err)
			return nil
		}
		mime := frame.MimeType
		if mime == "" {
			mime = "video/mp4"
		}
		if frame.GifPlayback {
			err = ctx.ReplyWithVideoGif(data, mime, frame.Caption)
		} else {
			err = ctx.ReplyWithVideo(data, mime, frame.Caption)
		}
		d.sendAck(stdinPipe, err == nil, "", err)

	case "send_document":
		loader.Delete()
		data, err := resolveMediaData(frame.Data)
		if err != nil {
			d.sendAck(stdinPipe, false, "", err)
			return nil
		}
		filename := frame.Filename
		if filename == "" {
			filename = "document"
		}
		mime := frame.MimeType
		if mime == "" {
			mime = "application/octet-stream"
		}
		err = ctx.ReplyWithDocument(data, mime, filename, frame.Caption)
		d.sendAck(stdinPipe, err == nil, "", err)

	case "send_sticker":
		loader.Delete()
		data, err := resolveMediaData(frame.Data)
		if err != nil {
			d.sendAck(stdinPipe, false, "", err)
			return nil
		}
		err = ctx.ReplyWithSticker(data)
		d.sendAck(stdinPipe, err == nil, "", err)

	case "poll":
		loader.Delete()
		if frame.Question != "" && len(frame.Options) > 0 {
			poll := ctx.Poll(frame.Question).AddOptions(frame.Options...)
			if frame.Selectable > 1 {
				poll = poll.MultiChoice()
			}
			msgID, err := poll.ReplyWithID()
			d.sendAck(stdinPipe, err == nil, string(msgID), err)
		}

	case "loader":
		if frame.Text != "" {
			ctx.StartAutoLoader()
		} else {
			ctx.StopAutoLoader()
		}

	case "done":
		loader.Delete()
		return io.EOF

	default:
		Logger.Debug("external plugin: unknown action frame", "action", frame.Action)
	}

	return nil
}

func (d *Dispatcher) sendAck(stdinPipe io.WriteCloser, ok bool, msgID string, err error) {
	ack := Ack{OK: ok, MsgID: msgID}
	if err != nil {
		ack.Error = err.Error()
	}
	data, _ := json.Marshal(ack)
	_, _ = fmt.Fprintf(stdinPipe, "%s\n", data)
}

// resolveMediaData parses base64 media payload or downloads from HTTP/HTTPS URL.
func resolveMediaData(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("media data is empty")
	}

	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		client := &http.Client{Timeout: 15 * time.Second}
		resp, err := client.Get(raw)
		if err != nil {
			return nil, fmt.Errorf("download media failed: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("download media returned status %d", resp.StatusCode)
		}
		return io.ReadAll(io.LimitReader(resp.Body, MaxPluginBinarySize))
	}

	// Remove data URI prefix if present (e.g. "data:image/png;base64,...")
	if idx := strings.Index(raw, ";base64,"); idx != -1 {
		raw = raw[idx+8:]
	}

	return base64.StdEncoding.DecodeString(raw)
}
