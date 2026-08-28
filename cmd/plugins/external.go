package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"whatsrook"
	utils "whatsrook/src"
	Logger "whatsrook/src/logger"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types/events"
)

const (
	externalPluginDirEnv  = "WHATSROOK_PLUGIN_DIR"
	maxExternalPluginSize = 64 << 20
	externalPluginTimeout = 30 * time.Second
)

var externalPluginNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

type externalPluginManifest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type externalPluginRequest struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
	RawArgs string   `json:"raw_args,omitempty"`
	Chat    string   `json:"chat"`
	Sender  string   `json:"sender"`
}

func init() {
	Register(&Command{
		Name:        "install",
		Description: "Install an external plugin from a local executable or URL (sudoers only)",
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

func handlePluginInstall(ctx *Context) error {
	if len(ctx.Args) != 2 {
		return ErrUsage(ctx.GetPrefix() + "install <name> <local-path-or-url>")
	}
	name, source := ctx.Args[0], ctx.Args[1]
	if err := installExternalPlugin(ctx.GetSendContext(), name, source); err != nil {
		return ctx.Replyf("Plugin installation failed: %v", err)
	}
	return ctx.Replyf("External plugin %q installed.", strings.ToLower(strings.TrimSpace(name)))
}

func handlePluginUninstall(ctx *Context) error {
	if len(ctx.Args) != 1 {
		return ErrUsage(ctx.GetPrefix() + "uninstall <name>")
	}
	if err := uninstallExternalPlugin(ctx.Args[0]); err != nil {
		return ctx.Replyf("Plugin uninstall failed: %v", err)
	}
	return ctx.Replyf("External plugin %q uninstalled.", strings.ToLower(strings.TrimSpace(ctx.Args[0])))
}

func handlePluginList(ctx *Context) error {
	plugins, err := listExternalPlugins()
	if err != nil {
		return ctx.Replyf("Failed to list plugins: %v", err)
	}
	if len(plugins) == 0 {
		return ctx.Reply("No external plugins installed.")
	}
	var b strings.Builder
	b.WriteString("Installed external plugins:\n")
	for _, plugin := range plugins {
		b.WriteString("- ")
		b.WriteString(plugin.Name)
		if plugin.Description != "" {
			b.WriteString(utils.Sprintf(" - %s", plugin.Description))
		}
		b.WriteByte('\n')
	}
	return ctx.Reply(strings.TrimSuffix(b.String(), "\n"))
}

func runExternalPlugin(ctx context.Context, client *whatsmeow.Client, evt *events.Message, name string, args []string, rawArgs string) bool {
	path, err := externalPluginPath(name)
	if err != nil {
		return false
	}
	if _, err := os.Stat(path); err != nil {
		return false
	}

	if ctx == nil {
		ctx = context.Background()
	}
	reqCtx, cancel := context.WithTimeout(ctx, externalPluginTimeout)
	defer cancel()
	request, _ := json.Marshal(externalPluginRequest{
		Command: name,
		Args:    args,
		RawArgs: rawArgs,
		Chat:    evt.Info.Chat.String(),
		Sender:  evt.Info.Sender.String(),
	})
	cmd := exec.CommandContext(reqCtx, path, args...)
	cmd.Stdin = strings.NewReader(string(request))
	output, err := cmd.Output()
	if err != nil {
		Logger.Error("external plugin failed", "plugin", name, "err", err)
		_ = (&Context{Ctx: reqCtx, Client: client, Evt: evt, Chat: evt.Info.Chat, Sender: evt.Info.Sender}).Replyf("External plugin %q failed: %v", name, err)
		return true
	}

	response := strings.TrimSpace(string(output))
	if response != "" {
		_ = (&Context{Ctx: reqCtx, Client: client, Evt: evt, Chat: evt.Info.Chat, Sender: evt.Info.Sender}).Reply(response)
	}
	return true
}

func isExternalPluginInstalled(name string) bool {
	path, err := externalPluginPath(name)
	if err != nil {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Mode().Perm()&0o111 != 0
}
