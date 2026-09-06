package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"whatsrook"
	"whatsrook/logger"
)

func runStandby(ctx context.Context, defaultDB string) error {
	if isTerminalInteractive() {
		return runInteractiveStandby(ctx, defaultDB)
	}
	return runHeadlessStandby(ctx)
}

func isTerminalInteractive() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func runHeadlessStandby(ctx context.Context) error {
	listener, server, boundPort, err := startStandbyHTTPServer()
	if err != nil {
		return err
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		_ = listener.Close()
	}()

	logger.Info("headless standby mode active; awaiting configuration", "port", boundPort)
	<-ctx.Done()
	return nil
}

// runInteractiveStandby prints a prompt to stdin listing stored sessions and
// waits for the user to type a phone number, then launches the bot with it.
// On bot exit the prompt loops back so the user can pick again.
func runInteractiveStandby(ctx context.Context, defaultDB string) error {
	for {
		listener, server, _, err := startStandbyHTTPServer()
		if err != nil {
			return err
		}

		dataDir := whatsrook.DefaultDataDir()
		sessions, _ := whatsrook.ListStoredSessions(ctx, dataDir, defaultDB)

		fmt.Println()
		if len(sessions) > 0 {
			fmt.Println("Stored sessions:")
			for i, s := range sessions {
				fmt.Printf("  [%d] +%s (%s)\n", i+1, s.User, s.Platform)
			}
			fmt.Println()
		}
		fmt.Print("Enter phone number to connect (or Ctrl+C to exit): ")

		var phone string
		if _, err := fmt.Scan(&phone); err != nil {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			_ = server.Shutdown(shutdownCtx)
			_ = listener.Close()
			cancel()
			return nil
		}

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = server.Shutdown(shutdownCtx)
		_ = listener.Close()
		cancel()

		phone = strings.TrimSpace(phone)
		if phone == "" {
			continue
		}

		botCfg := BotConfig{
			Session:         phone,
			QRCode:          true,
			ClientType:      whatsrook.ClientChrome,
			Database:        defaultDB,
			WSPort:          0,
			AsyncMessageAck: true,
		}

		if err := launchBotWithConfig(ctx, botCfg); err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			if !errors.Is(err, whatsrook.ErrLoggedOut) {
				logger.Warn("session disconnected, returning to standby", "err", err)
			}
		}

		if ctx.Err() != nil {
			return nil
		}
	}
}

func startStandbyHTTPServer() (net.Listener, *http.Server, int, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintln(w, `{"status":"standby","message":"WhatsRook interactive standby engine online"}`)
	})

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return nil, nil, 0, fmt.Errorf("failed to bind standby HTTP listener: %w", err)
	}

	boundPort := listener.Addr().(*net.TCPAddr).Port
	logger.Info("standby HTTP server online", "port", boundPort, "addr", listener.Addr().String())

	server := &http.Server{Handler: mux}
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("standby HTTP server error", "err", err)
		}
	}()

	return listener, server, boundPort, nil
}

func launchBotWithConfig(ctx context.Context, cfg BotConfig) error {
	if cfg.Verbose {
		logger.SetVerbose(true)
	}

	bot := NewBot(cfg)
	if err := bot.Start(ctx); err != nil {
		if errors.Is(err, whatsrook.ErrLoggedOut) {
			if ctx.Err() != nil {
				return nil
			}
			logger.Info("session was logged out and removed; switching to standby mode")
			return runStandby(ctx, cfg.Database)
		}
		return err
	}
	return nil
}
