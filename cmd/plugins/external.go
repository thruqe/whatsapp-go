package plugins

import (
	"runtime"
	"strings"

	"whatsrook/external"
)

func init() {
	Register(&Command{
		Name:        "install",
		Description: "Install an external plugin from official registry, custom URL, or local binary (sudoers only)",
		Category:    "extensions",
		IsPublic:    false,
		Handler:     handlePluginInstall,
	})

	Register(&Command{
		Name:        "uninstall",
		Description: "Uninstall an external plugin (sudoers only)",
		Category:    "extensions",
		IsPublic:    false,
		Handler:     handlePluginUninstall,
	})

	Register(&Command{
		Name:        "plist",
		Alias:       "pluginlist",
		Description: "List all installed external plugins",
		Category:    "extensions",
		IsPublic:    true,
		Handler:     handlePluginList,
	})
}

func handlePluginInstall(ctx *Context) error {
	p := ctx.GetPrefix()
	if len(ctx.Args) == 0 {
		return ctx.Text().
			Header("WhatsRook External Plugin Installer").
			Section("Usage:").
			Bulletf("%sinstall <name> (automatically downloads for host OS/arch from official registry)", p).
			Bulletf("%sinstall all (installs all %d official external plugins in parallel)", p, len(external.OfficialPlugins)).
			Bulletf("%sinstall <name> <local-path-or-url>", p).
			Blank().
			Section("Official Plugins:").
			Line(strings.Join(external.OfficialPlugins, ", ")).
			Reply()
	}

	if len(ctx.Args) == 1 {
		first := strings.ToLower(strings.TrimSpace(ctx.Args[0]))
		if first == "all" {
			ctx.StartAutoLoader()
			defer ctx.StopAutoLoader()

			installed, failed := external.DefaultDispatcher.InstallAll(ctx.GetSendContext())

			tb := ctx.Text()
			if len(installed) > 0 {
				tb.Headerf("Installed %d external plugins:", len(installed)).
					Bullet(strings.Join(installed, ", ")).
					Blank()
			}
			if len(failed) > 0 {
				tb.Headerf("Failed to install (%d):", len(failed))
				for _, f := range failed {
					tb.Bullet(f)
				}
			}
			return tb.Reply()
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

	tb := ctx.Text().Headerf("Installed External Plugins (%s):", suffix)
	for _, plugin := range plugins {
		if plugin.Description != "" {
			tb.Bulletf("%s - %s", Bold(plugin.Name), plugin.Description)
		} else {
			tb.Bullet(Bold(plugin.Name))
		}
	}
	return tb.Reply()
}
