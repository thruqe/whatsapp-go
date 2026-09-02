//go:generate go -C ../scripts run . res

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"whatsrook"
	_ "whatsrook"
	"whatsrook/cache"
	"whatsrook/cmd/tui"
	"whatsrook/cmd/updater"
	"whatsrook/logger"
)

func main() {
	args := parseCLIArgs()

	if args.Version {
		fmt.Println(Version)
		return
	}

	if args.Verbose {
		logger.SetVerbose(true)
	}

	if args.Update {
		handleUpdate(args.UpdateOp)
		return
	}

	cache.Init(10000)
	defer func() {
		_ = cache.Close()
	}()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if args.Session == "" {
		if err := runStandby(ctx, args.Database); err != nil {
			logger.Error("standby error", "err", err)
			os.Exit(1)
		}
		return
	}

	clientType, ok := whatsrook.ParseClientType(args.Client)
	if !ok {
		clientType = whatsrook.ClientChrome
	}

	bot := NewBot(BotConfig{
		Session:         args.Session,
		Pair:            args.Auth == "pair",
		QRCode:          args.Auth == "qr",
		Logout:          args.Logout,
		ClientType:      clientType,
		Database:        args.Database,
		Verbose:         args.Verbose,
		WSPort:          0, // 0 instructs OS to bind to a random available port
		AsyncMessageAck: true,
	})

	err := bot.Start(ctx)
	tui.ClearTerminal()
	if err != nil {
		if errors.Is(err, whatsrook.ErrLoggedOut) {
			if ctx.Err() != nil {
				return
			}
			logger.Info("session was logged out and removed; switching to standby mode")
			if err := runStandby(context.Background(), args.Database); err != nil {
				logger.Error("standby error", "err", err)
				os.Exit(1)
			}
			return
		}
		// If interrupted with Ctrl+C, clear and switch smoothly to interactive standby
		if errors.Is(err, context.Canceled) {
			if err := runStandby(context.Background(), args.Database); err != nil {
				logger.Error("standby error", "err", err)
				os.Exit(1)
			}
			return
		}
		logger.Error("bot execution failure", "err", err)
		os.Exit(1)
	}
}

func handleUpdate(op string) {
	ctx := context.Background()
	current := updater.GetStoredChannel()

	if op == "check" {
		up := updater.New(updater.Options{
			Out:     os.Stdout,
			Channel: current,
		})
		if _, err := up.Check(ctx); err != nil {
			logger.Error("update check failed", "err", err)
			os.Exit(1)
		}
		return
	}

	if op == "stable" || op == "beta" {
		if op != current {
			fmt.Printf("==> Switching release channel: %s -> %s\n", current, op)
			if err := updater.SetStoredChannel(op); err != nil {
				logger.Error("failed to set release channel", "err", err)
				os.Exit(1)
			}
			current = op
		} else {
			fmt.Printf("==> Already tracking channel: %s\n", current)
		}
	}

	up := updater.New(updater.Options{
		Out:     os.Stdout,
		Channel: current,
	})

	res, err := up.Upgrade(ctx, current == "beta")
	if err != nil {
		logger.Error("upgrade procedure failed", "err", err)
		os.Exit(1)
	}

	if res.Updated {
		fmt.Println("==> Upgrade complete!")
	}
}
