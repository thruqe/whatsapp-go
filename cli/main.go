//go:generate go run github.com/josephspurrier/goversioninfo/cmd/goversioninfo@latest -platform-specific=true

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"whatsrook"
	"whatsrook/cli/updater"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "update" {
		ctx := context.Background()
		up := updater.New(updater.Options{
			Out: os.Stdout,
		})

		subcmd := "check"
		if len(os.Args) > 2 {
			subcmd = os.Args[2]
		}

		switch subcmd {
		case "check":
			_, err := up.Check(ctx)
			if err != nil {
				slog.Error("update check failed", "err", err)
				os.Exit(1)
			}
			return

		case "upgrade", "apply", "now":
			isBeta := updater.GetStoredChannel() == "beta"
			res, err := up.Upgrade(ctx, isBeta)
			if err != nil {
				slog.Error("upgrade failed", "err", err)
				os.Exit(1)
			}
			if res.Updated {
				fmt.Printf("==> %s\n", res.Message)
				fmt.Println("==> Upgrade complete! Run whatsrook to start.")
			} else {
				fmt.Printf("==> %s\n", res.Message)
			}
			return

		default:
			fmt.Fprintf(os.Stderr, "Unknown update subcommand %q. Usage: whatsrook update [check|upgrade]\n", subcmd)
			os.Exit(1)
		}
	}

	args := parseCLIArgs()

	if args.Update {
		ctx := context.Background()
		up := updater.New(updater.Options{Out: os.Stdout})

		current := updater.GetStoredChannel()
		requested := args.UpdateChannel // "stable", "beta", or ""

		shouldDownload := true
		if requested != "" {
			if requested == current {
				fmt.Printf("==> Already on the %s channel.\n", current)
				shouldDownload = false
			} else {
				fmt.Printf("==> Switching from %s to %s channel...\n", current, requested)
				if err := updater.SetStoredChannel(requested); err != nil {
					slog.Error("failed to set channel", "err", err)
					os.Exit(1)
				}
				current = requested
			}
		}

		if shouldDownload {
			isBeta := current == "beta"
			res, err := up.Upgrade(ctx, isBeta)
			if err != nil {
				slog.Error("update failed", "err", err)
				os.Exit(1)
			}
			if res.Updated {
				fmt.Println("==> Restarting process with new binary...")
				if err := updater.RestartProcess(); err != nil {
					slog.Error("failed to restart process", "err", err)
					os.Exit(1)
				}
				return
			}
		}

		if args.Session == "" && os.Getenv("SESSION") == "" {
			fmt.Println("==> No active session requested. Exiting.")
			return
		}
	}

	clientType, ok := whatsrook.ParseClientType(args.Client)
	if !ok {
		fmt.Fprintf(os.Stderr, "Error: unknown --client %q. Valid options: chrome, android, ios\n", args.Client)
		os.Exit(1)
	}

	if args.Session == "" {
		fmt.Fprintln(os.Stderr, "Error: --session <phone_number> or $SESSION environment variable is required. Run with -h for help.")
		os.Exit(1)
	}

	bot := NewBot(BotConfig{
		Session:         args.Session,
		Pair:            args.Pair,
		QRCode:          args.QRCode,
		Logout:          args.Logout,
		Verbose:         args.Verbose,
		ClientType:      clientType,
		Database:        args.Database,
		WSPort:          args.Port,
		SkipOldMessages: args.SkipOldMessages,
	})

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := bot.Start(ctx); err != nil {
		slog.Error("bot error", "err", err)
		os.Exit(1)
	}
}
