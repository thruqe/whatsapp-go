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
	"whatsrook/cmd/updater"
	"whatsrook/src/cache"
	Logger "whatsrook/src/logger"
)

func main() {
	args := parseCLIArgs()

	if args.Verbose {
		Logger.SetVerbose(true)
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
			Logger.Error("standby error", "err", err)
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

	if err := bot.Start(ctx); err != nil {
		if errors.Is(err, whatsrook.ErrLoggedOut) {
			if ctx.Err() != nil {
				return
			}
			Logger.Info("session was logged out and removed; switching to standby mode")
			if err := runStandby(ctx, args.Database); err != nil {
				Logger.Error("standby error", "err", err)
				os.Exit(1)
			}
			return
		}
		Logger.Error("bot execution failure", "err", err)
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
			Logger.Error("update check failed", "err", err)
			os.Exit(1)
		}
		return
	}

	if op == "stable" || op == "beta" {
		if op != current {
			fmt.Printf("==> Switching release channel: %s -> %s\n", current, op)
			if err := updater.SetStoredChannel(op); err != nil {
				Logger.Error("failed to set release channel", "err", err)
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
		Logger.Error("upgrade procedure failed", "err", err)
		os.Exit(1)
	}

	if res.Updated {
		fmt.Println("==> Restarting process with upgraded binary...")
		if err := updater.RestartProcess(); err != nil {
			Logger.Error("failed to restart binary process", "err", err)
			os.Exit(1)
		}
		os.Exit(0) // exit after handing off on Windows
	}
}
