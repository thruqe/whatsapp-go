package extensions

import (
	"runtime"
	"strings"

	"whatsrook/cmd/dispatch"
	"whatsrook/util/cache"
	"whatsrook/util/external"
)

func init() {
	dispatch.Register(&dispatch.Command{
		Name:        "install",
		Description: "Install an extra plugin (Owner only)",
		Category:    "extensions",
		IsPublic:    false,
		Handler:     handlePluginInstall,
	})

	dispatch.Register(&dispatch.Command{
		Name:        "uninstall",
		Description: "Remove an installed plugin (Owner only)",
		Category:    "extensions",
		IsPublic:    false,
		Handler:     handlePluginUninstall,
	})

	dispatch.Register(&dispatch.Command{
		Name:        "plist",
		Alias:       "pluginlist",
		Description: "Show list of installed plugins",
		Category:    "extensions",
		IsPublic:    true,
		Handler:     handlePluginList,
	})
}

func getSession(ctx *dispatch.Context) string {
	if ctx != nil && ctx.Client != nil && ctx.Client.Store != nil && ctx.Client.Store.ID != nil {
		return ctx.Client.Store.ID.User
	}
	if ctx != nil && ctx.Sender.User != "" {
		return ctx.Sender.User
	}
	return ""
}

func handlePluginInstall(ctx *dispatch.Context) error {
	p := ctx.GetPrefix()
	sess := getSession(ctx)
	if len(ctx.Args) == 0 {
		return ctx.Text().
			Header("WhatsRook External Plugin Installer").
			Section("Usage:").
			Bulletf("%sinstall <name> (automatically downloads for host OS/arch from official registry)", p).
			Bulletf("%sinstall all (installs all %d official external plugins in parallel)", p, len(external.OfficialPlugins)).
			Bulletf("%sinstall <name> <local-path-or-url>", p).
			Blank().
			Section("Registry:").
			Bullet(external.DefaultDispatcher.RegistryURL()).
			Blank().
			Section("Official Plugins:").
			Bullets(external.OfficialPlugins...).
			Reply()
	}

	if len(ctx.Args) == 1 {
		first := strings.ToLower(strings.TrimSpace(ctx.Args[0]))
		if first == "all" {
			installed, failed := external.DefaultDispatcher.InstallAll(ctx.GetSendContext(), sess)

			tb := ctx.Text()
			if len(installed) > 0 {
				tb.Headerf("Installed %d external plugins:", len(installed)).
					Bullets(installed...).
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

		if err := external.DefaultDispatcher.Install(ctx.GetSendContext(), name, url, sess); err != nil {
			return ctx.Replyf("Plugin installation failed for %q:\n%v", name, err)
		}
		_ = cache.DeletePrefix(ctx.Ctx, "instruction:")
		return ctx.Replyf("External plugin %q installed successfully for %s/%s.", name, runtime.GOOS, runtime.GOARCH)
	}

	name, rawSource := ctx.Args[0], ctx.Args[1]
	source, err := external.DefaultDispatcher.NormalizePluginSource(rawSource)
	if err != nil {
		return ctx.Replyf("Platform resolution error: %v", err)
	}

	if err := external.DefaultDispatcher.Install(ctx.GetSendContext(), name, source, sess); err != nil {
		return ctx.Replyf("Plugin installation failed: %v", err)
	}
	_ = cache.DeletePrefix(ctx.Ctx, "instruction:")
	return ctx.Replyf("External plugin %q installed.", strings.ToLower(strings.TrimSpace(name)))
}

func handlePluginUninstall(ctx *dispatch.Context) error {
	p := ctx.GetPrefix()
	sess := getSession(ctx)
	if len(ctx.Args) != 1 {
		return dispatch.ErrUsage(p + "uninstall <name> (or " + p + "uninstall all)")
	}

	targetName := strings.ToLower(strings.TrimSpace(ctx.Args[0]))
	if targetName == "all" {
		removed, err := external.DefaultDispatcher.UninstallAll(sess)
		if err != nil {
			return ctx.Replyf("Failed to uninstall plugins: %v", err)
		}
		if len(removed) == 0 {
			return ctx.Reply("No external plugins currently installed.")
		}
		_ = cache.DeletePrefix(ctx.Ctx, "instruction:")
		return ctx.Replyf("Uninstalled %d external plugin(s): %s", len(removed), strings.Join(removed, ", "))
	}

	if err := external.DefaultDispatcher.Uninstall(targetName, sess); err != nil {
		return ctx.Replyf("Plugin uninstall failed: %v", err)
	}
	_ = cache.DeletePrefix(ctx.Ctx, "instruction:")
	return ctx.Replyf("External plugin %q uninstalled.", targetName)
}

func handlePluginList(ctx *dispatch.Context) error {
	sess := getSession(ctx)
	plugins, err := external.DefaultDispatcher.List(sess)
	if err != nil {
		return ctx.Replyf("Failed to list plugins: %v", err)
	}

	suffix, _ := external.DefaultDispatcher.ResolvePlatformSuffix()
	if len(plugins) == 0 {
		p := ctx.GetPrefix()
		return ctx.Replyf("No external plugins installed.\n\nType `%sinstall <name>` or `%sinstall all` to install plugins (detected platform: %s/%s).\n\nRegistry: %s", p, p, runtime.GOOS, runtime.GOARCH, external.DefaultDispatcher.RegistryURL())
	}

	tb := ctx.Text().Headerf("Installed External Plugins (%s):", suffix)
	for _, plugin := range plugins {
		if plugin.Description != "" {
			tb.Bulletf("%s - %s", dispatch.Bold(plugin.Name), plugin.Description)
		} else {
			tb.Bullet(dispatch.Bold(plugin.Name))
		}
	}
	return tb.Reply()
}
