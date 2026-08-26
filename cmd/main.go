//go:generate go run github.com/josephspurrier/goversioninfo/cmd/goversioninfo@latest -platform-specific=true

package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"whatsrook"
	"whatsrook/cmd/updater"
	Logger "whatsrook/logger"
	"whatsrook/utils/cache"
)

func main() {
	args := parseCLIArgs()

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
		if err := runIdleMode(ctx); err != nil {
			Logger.Error("idle server error", "err", err)
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
		WSPort:          0, // 0 instructs OS to bind to a random available port
		AsyncMessageAck: true,
	})

	if err := bot.Start(ctx); err != nil {
		if errors.Is(err, whatsrook.ErrLoggedOut) {
			if ctx.Err() != nil {
				return
			}
			Logger.Info("session was logged out and removed; switching to standby mode")
			if err := runIdleMode(ctx); err != nil {
				Logger.Error("idle server error", "err", err)
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

func runIdleMode(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, "WhatsRook standby • waiting for session • %s\n", time.Now().Format("15:04:05"))
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintln(w, "OK")
	})

	// Bind to port :0 for random assignment
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return fmt.Errorf("failed to bind standby server: %w", err)
	}

	boundPort := listener.Addr().(*net.TCPAddr).Port
	Logger.Info("standby HTTP server online", "port", boundPort, "addr", listener.Addr().String())

	server := &http.Server{Handler: mux}
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			Logger.Error("standby HTTP server encountered error", "err", err)
		}
	}()

	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		_ = listener.Close()
	}()

	fmt.Printf("\rWhatsRook standby (port :%d) • waiting for session • %s", boundPort, time.Now().Format("15:04:05"))

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			fmt.Printf("\rWhatsRook standby (port :%d) • waiting for session • %s", boundPort, time.Now().Format("15:04:05"))
		case <-ctx.Done():
			fmt.Println()
			return nil
		}
	}
}
