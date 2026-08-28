package plugins

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"whatsrook"
	utils "whatsrook/src"
	Logger "whatsrook/src/logger"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

const (
	externalPluginDirEnv      = "WHATSROOK_PLUGIN_DIR"
	defaultExternalPluginRepo = "https://github.com/Thruqe/whatsrook-externals/releases/latest/download"
	maxExternalPluginSize     = 64 << 20
	externalPluginTimeout     = 30 * time.Second
	externalLivePluginTimeout = 5 * time.Minute
)

var officialExternalPlugins = []string{
	"weather", "urban", "shorturl", "calc", "fact",
	"quotes", "joke", "rizz", "btc", "markets",
	"news", "wabeta", "why",
}

var externalPluginNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

// livePluginSession tracks an active streaming external plugin process.
type livePluginSession struct {
	cancel     context.CancelFunc
	pluginName string
}

var (
	liveSessionsMu sync.Mutex
	liveSessions   = make(map[string]*livePluginSession) // key = "chatJID:pluginName"
)

type externalPluginManifest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	IsPublic    bool   `json:"is_public,omitempty"`
}

type externalPluginRequest struct {
	Command         string   `json:"command"`
	Args            []string `json:"args,omitempty"`
	RawArgs         string   `json:"raw_args,omitempty"`
	Chat            string   `json:"chat"`
	Sender          string   `json:"sender"`
	Prefix          string   `json:"prefix"`
	BotName         string   `json:"bot_name"`
	PushName        string   `json:"push_name,omitempty"`
	IsGroup         bool     `json:"is_group"`
	IsSudo          bool     `json:"is_sudo"`
	IsLiveSession   bool     `json:"live_session,omitempty"`
	IsCancelRequest bool     `json:"is_cancel_request,omitempty"`
}

// externalPluginAction is a single streaming action frame written by the plugin to stdout.
type externalPluginAction struct {
	Action string `json:"action"`           // "reply" | "edit" | "done"
	Text   string `json:"text,omitempty"`   // for reply and edit
	MsgID  string `json:"msg_id,omitempty"` // for edit
}

// externalPluginAck is sent by WhatsRook to the plugin stdin after a "reply" action.
type externalPluginAck struct {
	OK    bool   `json:"ok"`
	MsgID string `json:"msg_id,omitempty"`
	Error string `json:"error,omitempty"`
}

func init() {
	Register(&Command{
		Name:        "install",
		Description: "Install an external plugin from official registry, custom URL, or local binary (sudoers only)",
		Category:    "Plugins",
		IsPublic:    false,
		Handler:     handlePluginInstall,
	})
	Register(&Command{
		Name:        "uninstall",
		Description: "Uninstall an external plugin (sudoers only)",
		Category:    "Plugins",
		IsPublic:    false,
		Handler:     handlePluginUninstall,
	})
	Register(&Command{
		Name:        "plist",
		Alias:       "pluginlist",
		Description: "List installed external plugins",
		Category:    "Plugins",
		IsPublic:    false,
		Handler:     handlePluginList,
	})
}

// ResolvePlatformSuffix returns the binary target suffix corresponding to the current OS and architecture.
func ResolvePlatformSuffix() (string, error) {
	osName := runtime.GOOS
	arch := runtime.GOARCH

	switch osName {
	case "linux", "android":
		switch arch {
		case "arm64", "aarch64":
			return "linux-arm64", nil
		case "amd64", "x86_64":
			return "linux-amd64", nil
		case "arm":
			return "linux-arm", nil
		default:
			return "", fmt.Errorf("unsupported Linux/Android architecture %q (supported: arm64, amd64)", arch)
		}
	case "darwin":
		switch arch {
		case "arm64":
			return "darwin-arm64", nil
		case "amd64":
			return "darwin-amd64", nil
		default:
			return "", fmt.Errorf("unsupported macOS architecture %q (supported: arm64, amd64)", arch)
		}
	case "windows":
		switch arch {
		case "amd64":
			return "windows-amd64.exe", nil
		case "arm64":
			return "windows-arm64.exe", nil
		default:
			return "", fmt.Errorf("unsupported Windows architecture %q (supported: amd64, arm64)", arch)
		}
	default:
		return "", fmt.Errorf("unsupported operating system %q (supported: linux, darwin, windows)", osName)
	}
}

// ResolveDefaultPluginURL resolves the download URL for a plugin from the official whatsrook-externals repository.
func ResolveDefaultPluginURL(name string) (string, error) {
	suffix, err := ResolvePlatformSuffix()
	if err != nil {
		return "", err
	}
	name = strings.ToLower(strings.TrimSpace(name))
	return fmt.Sprintf("%s/%s-%s", defaultExternalPluginRepo, name, suffix), nil
}

func externalPluginDir() (string, error) {
	if dir := strings.TrimSpace(os.Getenv(externalPluginDirEnv)); dir != "" {
		return filepath.Abs(dir)
	}
	return filepath.Join(whatsrook.DefaultDataDir(), "plugins"), nil
}

func validateExternalPluginName(name string) error {
	if !externalPluginNamePattern.MatchString(name) {
		return errors.New("plugin name must be 1-64 characters and contain only letters, numbers, '_' or '-'")
	}
	return nil
}

func externalPluginPath(name string) (string, error) {
	if err := validateExternalPluginName(name); err != nil {
		return "", err
	}
	dir, err := externalPluginDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

func readExternalPluginManifest(path string) externalPluginManifest {
	manifest := externalPluginManifest{Name: filepath.Base(path)}
	data, err := os.ReadFile(path + ".json")
	if err == nil {
		if json.Unmarshal(data, &manifest) == nil && manifest.Name != "" {
			return manifest
		}
	}
	return manifest
}

func listExternalPlugins() ([]externalPluginManifest, error) {
	dir, err := externalPluginDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []externalPluginManifest{}, nil
	}
	if err != nil {
		return nil, WrapError("read plugin directory", err)
	}

	plugins := make([]externalPluginManifest, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || strings.HasSuffix(entry.Name(), ".json") || !externalPluginNamePattern.MatchString(entry.Name()) {
			continue
		}
		plugins = append(plugins, readExternalPluginManifest(filepath.Join(dir, entry.Name())))
	}
	sort.Slice(plugins, func(i, j int) bool { return strings.ToLower(plugins[i].Name) < strings.ToLower(plugins[j].Name) })
	return plugins, nil
}

func installExternalPlugin(ctx context.Context, name, source string) error {
	name = strings.ToLower(strings.TrimSpace(name))
	target, err := externalPluginPath(name)
	if err != nil {
		return err
	}
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return WrapError("create plugin directory", err)
	}

	tmp, err := os.CreateTemp(dir, ".plugin-*")
	if err != nil {
		return WrapError("create temporary plugin", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	var reader io.Reader
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
		if reqErr != nil {
			_ = tmp.Close()
			return WrapError("create download request", reqErr)
		}
		resp, reqErr := http.DefaultClient.Do(req)
		if reqErr != nil {
			_ = tmp.Close()
			return WrapError("download plugin", reqErr)
		}
		defer resp.Body.Close()
		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			_ = tmp.Close()
			return errors.New(utils.Sprintf("download plugin: unexpected HTTP status %s", resp.Status))
		}
		reader = io.LimitReader(resp.Body, maxExternalPluginSize+1)
	} else {
		file, openErr := os.Open(source)
		if openErr != nil {
			_ = tmp.Close()
			return WrapError("open plugin source", openErr)
		}
		defer file.Close()
		reader = io.LimitReader(file, maxExternalPluginSize+1)
	}

	written, err := io.Copy(tmp, reader)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return WrapError("copy plugin", err)
	}
	if written == 0 || written > maxExternalPluginSize {
		return errors.New(utils.Sprintf("plugin size must be between 1 byte and %d MiB", maxExternalPluginSize/(1<<20)))
	}
	if err := os.Chmod(tmpPath, 0o700); err != nil {
		return WrapError("make plugin executable", err)
	}
	if err := os.Rename(tmpPath, target); err != nil {
		return WrapError("install plugin", err)
	}

	manifest := externalPluginManifest{Name: name}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		_ = os.Remove(target)
		return WrapError("encode plugin metadata", err)
	}
	if err := os.WriteFile(target+".json", append(data, '\n'), 0o600); err != nil {
		_ = os.Remove(target)
		return WrapError("write plugin metadata", err)
	}
	return nil
}

func uninstallExternalPlugin(name string) error {
	name = strings.ToLower(strings.TrimSpace(name))
	target, err := externalPluginPath(name)
	if err != nil {
		return err
	}
	if _, err := os.Stat(target); errors.Is(err, os.ErrNotExist) {
		return errors.New(utils.Sprintf("plugin %q is not installed", name))
	} else if err != nil {
		return err
	}
	if err := os.Remove(target); err != nil {
		return WrapError("remove plugin", err)
	}
	if err := os.Remove(target + ".json"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return WrapError("remove plugin metadata", err)
	}
	return nil
}

// normalizePluginSource ensures remote URLs are resolved with the platform suffix if missing.
func normalizePluginSource(source string) (string, error) {
	suffix, err := ResolvePlatformSuffix()
	if err != nil {
		return source, err
	}

	source = strings.ReplaceAll(source, "{platform}", suffix)
	source = strings.ReplaceAll(source, "{target}", suffix)
	source = strings.ReplaceAll(source, "{suffix}", suffix)

	if !strings.HasPrefix(source, "http://") && !strings.HasPrefix(source, "https://") {
		return source, nil
	}

	knownSuffixes := []string{
		"linux-amd64", "linux-arm64", "linux-musl-amd64", "linux-musl-arm64",
		"darwin-amd64", "darwin-arm64", "windows-amd64.exe", "windows-arm64.exe",
		".exe", ".tar.gz", ".zip",
	}
	lowerSource := strings.ToLower(source)
	for _, ks := range knownSuffixes {
		if strings.HasSuffix(lowerSource, ks) {
			return source, nil
		}
	}

	source = strings.TrimSuffix(source, "/")
	return source + "-" + suffix, nil
}

func handlePluginInstall(ctx *Context) error {
	p := ctx.GetPrefix()
	if len(ctx.Args) == 0 {
		var b strings.Builder
		b.WriteString("🔌 *WhatsRook External Plugin Installer*\n\n")
		b.WriteString("*Usage:*\n")
		b.WriteString(utils.Sprintf("• `%sinstall <name>` (automatically downloads for host OS/arch from official registry)\n", p))
		b.WriteString(utils.Sprintf("• `%sinstall all` (installs all 13 official external plugins)\n", p))
		b.WriteString(utils.Sprintf("• `%sinstall <name> <local-path-or-url>`\n\n", p))
		b.WriteString("*Official Plugins:*\n")
		b.WriteString("`" + strings.Join(officialExternalPlugins, "`, `") + "`")
		return ctx.Reply(b.String())
	}

	if len(ctx.Args) == 1 {
		first := strings.ToLower(strings.TrimSpace(ctx.Args[0]))
		if first == "all" {
			ctx.StartAutoLoader()
			defer ctx.StopAutoLoader()

			type result struct {
				name string
				err  error
			}
			resChan := make(chan result, len(officialExternalPlugins))
			var wg sync.WaitGroup

			for _, name := range officialExternalPlugins {
				wg.Add(1)
				go func(plugName string) {
					defer wg.Done()
					url, err := ResolveDefaultPluginURL(plugName)
					if err != nil {
						resChan <- result{name: plugName, err: err}
						return
					}
					err = installExternalPlugin(ctx.GetSendContext(), plugName, url)
					resChan <- result{name: plugName, err: err}
				}(name)
			}

			wg.Wait()
			close(resChan)

			var installed []string
			var failed []string
			for res := range resChan {
				if res.err != nil {
					failed = append(failed, utils.Sprintf("%s (%v)", res.name, res.err))
				} else {
					installed = append(installed, res.name)
				}
			}
			sort.Strings(installed)
			sort.Strings(failed)

			var b strings.Builder
			if len(installed) > 0 {
				b.WriteString(utils.Sprintf("✅ *Installed %d external plugins:*\n• %s\n", len(installed), strings.Join(installed, ", ")))
			}
			if len(failed) > 0 {
				if b.Len() > 0 {
					b.WriteByte('\n')
				}
				b.WriteString(utils.Sprintf("❌ *Failed to install (%d):*\n• %s", len(failed), strings.Join(failed, "\n• ")))
			}
			return ctx.Reply(b.String())
		}

		name := first
		url, err := ResolveDefaultPluginURL(name)
		if err != nil {
			return ctx.Replyf("Platform detection failed: %v", err)
		}

		ctx.StartAutoLoader()
		defer ctx.StopAutoLoader()

		if err := installExternalPlugin(ctx.GetSendContext(), name, url); err != nil {
			return ctx.Replyf("Plugin installation failed for %q:\n%v", name, err)
		}
		return ctx.Replyf("External plugin %q installed successfully for %s/%s.", name, runtime.GOOS, runtime.GOARCH)
	}

	name, rawSource := ctx.Args[0], ctx.Args[1]
	source, err := normalizePluginSource(rawSource)
	if err != nil {
		return ctx.Replyf("Platform resolution error: %v", err)
	}

	ctx.StartAutoLoader()
	defer ctx.StopAutoLoader()

	if err := installExternalPlugin(ctx.GetSendContext(), name, source); err != nil {
		return ctx.Replyf("Plugin installation failed: %v", err)
	}
	return ctx.Replyf("External plugin %q installed.", strings.ToLower(strings.TrimSpace(name)))
}

func handlePluginUninstall(ctx *Context) error {
	p := ctx.GetPrefix()
	if len(ctx.Args) != 1 {
		return ErrUsage(p + "uninstall <name> (or " + p + "uninstall all)")
	}

	targetName := strings.ToLower(strings.TrimSpace(ctx.Args[0]))
	if targetName == "all" {
		plugins, err := listExternalPlugins()
		if err != nil {
			return ctx.Replyf("Failed to list plugins: %v", err)
		}
		if len(plugins) == 0 {
			return ctx.Reply("No external plugins currently installed.")
		}

		var removed []string
		for _, plug := range plugins {
			if err := uninstallExternalPlugin(plug.Name); err == nil {
				removed = append(removed, plug.Name)
			}
		}
		return ctx.Replyf("Uninstalled %d external plugin(s): %s", len(removed), strings.Join(removed, ", "))
	}

	if err := uninstallExternalPlugin(targetName); err != nil {
		return ctx.Replyf("Plugin uninstall failed: %v", err)
	}
	return ctx.Replyf("External plugin %q uninstalled.", targetName)
}

func handlePluginList(ctx *Context) error {
	plugins, err := listExternalPlugins()
	if err != nil {
		return ctx.Replyf("Failed to list plugins: %v", err)
	}

	suffix, _ := ResolvePlatformSuffix()
	if len(plugins) == 0 {
		p := ctx.GetPrefix()
		return ctx.Replyf("No external plugins installed.\n\nType `%sinstall <name>` or `%sinstall all` to install plugins (detected platform: %s/%s).", p, p, runtime.GOOS, runtime.GOARCH)
	}

	var b strings.Builder
	b.WriteString(utils.Sprintf("🔌 *Installed External Plugins* (%s):\n", suffix))
	for _, plugin := range plugins {
		b.WriteString("• *")
		b.WriteString(plugin.Name)
		b.WriteString("*")
		if plugin.Description != "" {
			b.WriteString(utils.Sprintf(" - %s", plugin.Description))
		}
		b.WriteByte('\n')
	}
	return ctx.Reply(strings.TrimSuffix(b.String(), "\n"))
}

func liveSessionKey(chatJID, pluginName string) string {
	return chatJID + ":" + pluginName
}

func cancelLiveSession(chatJID, pluginName string) bool {
	key := liveSessionKey(chatJID, pluginName)
	liveSessionsMu.Lock()
	sess, ok := liveSessions[key]
	if ok && sess != nil {
		if sess.cancel != nil {
			sess.cancel()
		}
		delete(liveSessions, key)
	}
	liveSessionsMu.Unlock()
	return ok
}

func runExternalPlugin(ctx context.Context, client *whatsmeow.Client, evt *events.Message, name string, args []string, rawArgs string) bool {
	path, err := externalPluginPath(name)
	if err != nil {
		return false
	}
	if _, err := os.Stat(path); err != nil {
		return false
	}

	chatKey := evt.Info.Chat.String()
	sessionKey := liveSessionKey(chatKey, name)

	// Handle cancel request (.btc stop / .btc cancel)
	isCancelRequest := len(args) > 0 && isStopArg(args[0])
	if isCancelRequest {
		if cancelLiveSession(chatKey, name) {
			go func() {
				_ = (&Context{
					Ctx: context.Background(), Client: client, Evt: evt,
					Chat: evt.Info.Chat, Sender: evt.Info.Sender,
				}).Replyf("🛑 Live %s tracking stopped.", name)
			}()
		} else {
			go func() {
				_ = (&Context{
					Ctx: context.Background(), Client: client, Evt: evt,
					Chat: evt.Info.Chat, Sender: evt.Info.Sender,
				}).Replyf("No active %s session running in this chat.", name)
			}()
		}
		return true
	}

	// Cancel any existing live session before starting a new one
	liveSessionsMu.Lock()
	if prev, ok := liveSessions[sessionKey]; ok && prev != nil && prev.cancel != nil {
		prev.cancel()
		delete(liveSessions, sessionKey)
	}
	liveSessionsMu.Unlock()

	// Launch external plugin execution asynchronously so incoming message dispatch is NEVER blocked
	go func() {
		liveCtx, liveCancel := context.WithTimeout(context.Background(), externalLivePluginTimeout)
		defer liveCancel()

		plugCtx := &Context{
			Ctx:     liveCtx,
			Client:  client,
			Evt:     evt,
			Chat:    evt.Info.Chat,
			Sender:  evt.Info.Sender,
			Command: name,
			Args:    args,
			RawArgs: rawArgs,
		}

		prefix := plugCtx.GetPrefix()
		botName := plugCtx.GetBotName()
		isSudo := utils.IsSudoRaw(liveCtx, client, evt.Info.Sender)

		requestJSON, _ := json.Marshal(externalPluginRequest{
			Command:         name,
			Args:            args,
			RawArgs:         rawArgs,
			Chat:            chatKey,
			Sender:          evt.Info.Sender.String(),
			Prefix:          prefix,
			BotName:         botName,
			PushName:        evt.Info.PushName,
			IsGroup:         evt.Info.IsGroup,
			IsSudo:          isSudo,
			IsLiveSession:   false,
			IsCancelRequest: false,
		})

		cmd := exec.CommandContext(liveCtx, path)
		stdoutPipe, err := cmd.StdoutPipe()
		if err != nil {
			Logger.Error("external plugin stdout pipe failed", "plugin", name, "err", err)
			return
		}
		stdinPipe, err := cmd.StdinPipe()
		if err != nil {
			Logger.Error("external plugin stdin pipe failed", "plugin", name, "err", err)
			return
		}

		if err := cmd.Start(); err != nil {
			Logger.Error("external plugin start failed", "plugin", name, "err", err)
			_ = plugCtx.Replyf("Failed to start external plugin %q: %v", name, err)
			return
		}

		// Register session so .btc stop can cancel it while it runs
		liveSessionsMu.Lock()
		liveSessions[sessionKey] = &livePluginSession{cancel: liveCancel, pluginName: name}
		liveSessionsMu.Unlock()

		defer func() {
			_ = stdinPipe.Close()
			_ = cmd.Wait()
			liveSessionsMu.Lock()
			if curr, ok := liveSessions[sessionKey]; ok && curr != nil && curr.cancel != nil {
				delete(liveSessions, sessionKey)
			}
			liveSessionsMu.Unlock()
		}()

		// Write JSON request + newline to stdin
		_, _ = fmt.Fprintf(stdinPipe, "%s\n", requestJSON)

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
					// Plain text mode
					isStreaming = false
					break
				}
			}

			if isStreaming {
				if err := handleActionFrame(plugCtx, stdinPipe, line); err != nil {
					Logger.Debug("external plugin action frame ended", "plugin", name, "err", err)
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
				_ = plugCtx.Reply(response)
			}
		}
	}()

	return true
}

// handleActionFrame parses and executes a single JSON action frame from a streaming external plugin.
// Returns an error to signal the loop should stop.
func handleActionFrame(plugCtx *Context, stdinPipe io.WriteCloser, line string) error {
	var frame externalPluginAction
	if err := json.Unmarshal([]byte(line), &frame); err != nil {
		return err
	}

	switch frame.Action {
	case "reply":
		msgID, err := plugCtx.ReplyWithID(frame.Text)
		ack := externalPluginAck{OK: err == nil}
		if err == nil {
			ack.MsgID = string(msgID)
		} else {
			ack.Error = err.Error()
		}
		ackJSON, _ := json.Marshal(ack)
		_, _ = fmt.Fprintf(stdinPipe, "%s\n", ackJSON)

	case "edit":
		if frame.MsgID != "" && frame.Text != "" {
			_, _ = plugCtx.Edit(types.MessageID(frame.MsgID), frame.Text)
		}

	case "done":
		return io.EOF

	default:
		Logger.Debug("external plugin: unknown action", "action", frame.Action)
	}
	return nil
}

func isStopArg(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "stop" || s == "cancel" || s == "end" || s == "off"
}

func isExternalPluginInstalled(name string) bool {
	path, err := externalPluginPath(name)
	if err != nil {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Mode().Perm()&0o111 != 0
}

// isExternalPluginPublic checks the plugin manifest for IsPublic setting.
// If no manifest or field absent, defaults to false (sudoers only).
func isExternalPluginPublic(name string) bool {
	path, err := externalPluginPath(name)
	if err != nil {
		return false
	}
	manifest := readExternalPluginManifest(path)
	return manifest.IsPublic
}
