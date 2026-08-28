package plugins

import (
	"runtime"
	"strings"

	utils "whatsrook/src"
	"whatsrook/src/external"
)

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
		Description: "List all installed external plugins",
		Category:    "Plugins",
		IsPublic:    true,
		Handler:     handlePluginList,
	})
}

func handlePluginInstall(ctx *Context) error {
	p := ctx.GetPrefix()
	if len(ctx.Args) == 0 {
		var b strings.Builder
		b.WriteString("🔌 *WhatsRook External Plugin Installer*\n\n")
		b.WriteString("*Usage:*\n")
		b.WriteString(utils.Sprintf("• `%sinstall <name>` (automatically downloads for host OS/arch from official registry)\n", p))
		b.WriteString(utils.Sprintf("• `%sinstall all` (installs all 13 official external plugins in parallel)\n", p))
		b.WriteString(utils.Sprintf("• `%sinstall <name> <local-path-or-url>`\n\n", p))
		b.WriteString("*Official Plugins:*\n")
		b.WriteString("`" + strings.Join(external.OfficialPlugins, "`, `") + "`")
		return ctx.Reply(b.String())
	}

	if len(ctx.Args) == 1 {
		first := strings.ToLower(strings.TrimSpace(ctx.Args[0]))
		if first == "all" {
			ctx.StartAutoLoader()
			defer ctx.StopAutoLoader()

			installed, failed := external.DefaultDispatcher.InstallAll(ctx.GetSendContext())

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
		url, err := external.DefaultDispatcher.ResolveDefaultPluginURL(name)
		if err != nil {
			return ctx.Replyf("Platform detection failed: %v", err)
		}

		ctx.StartAutoLoader()
		defer ctx.StopAutoLoader()

		if err := external.DefaultDispatcher.Install(ctx.GetSendContext(), name, url); err != nil {
			return ctx.Replyf("Plugin installation failed for %q:\n%v", name, err)
		}
		return ctx.Replyf("External plugin %q installed successfully for %s/%s.", name, runtime.GOOS, runtime.GOARCH)
	}

	name, rawSource := ctx.Args[0], ctx.Args[1]
	source, err := external.DefaultDispatcher.NormalizePluginSource(rawSource)
	if err != nil {
		return ctx.Replyf("Platform resolution error: %v", err)
	}

	ctx.StartAutoLoader()
	defer ctx.StopAutoLoader()

	if err := external.DefaultDispatcher.Install(ctx.GetSendContext(), name, source); err != nil {
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
		removed, err := external.DefaultDispatcher.UninstallAll()
		if err != nil {
			return ctx.Replyf("Failed to uninstall plugins: %v", err)
		}
		if len(removed) == 0 {
			return ctx.Reply("No external plugins currently installed.")
		}
		return ctx.Replyf("Uninstalled %d external plugin(s): %s", len(removed), strings.Join(removed, ", "))
	}

	if err := external.DefaultDispatcher.Uninstall(targetName); err != nil {
		return ctx.Replyf("Plugin uninstall failed: %v", err)
	}
	return ctx.Replyf("External plugin %q uninstalled.", targetName)
}

func handlePluginList(ctx *Context) error {
	plugins, err := external.DefaultDispatcher.List()
	if err != nil {
		return ctx.Replyf("Failed to list plugins: %v", err)
	}

	suffix, _ := external.DefaultDispatcher.ResolvePlatformSuffix()
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
