package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"whatsrook"
	"whatsrook/cmd/tui"
	Logger "whatsrook/src/logger"

	"golang.org/x/term"
)

// runStandby initiates standby mode, automatically detecting interactive TTY vs headless cloud environments.
func runStandby(ctx context.Context, defaultDB string) error {
	isInteractive := term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
	if !isInteractive {
		return runIdleHeadless(ctx)
	}

	return runInteractiveStandby(ctx, defaultDB)
}

func runIdleHeadless(ctx context.Context) error {
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

	fmt.Printf("\rWhatsRook standby (port :%d) • waiting for session • %s\n", boundPort, time.Now().Format("15:04:05"))
	<-ctx.Done()
	return nil
}

func startStandbyHTTPServer() (net.Listener, *http.Server, int, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, "WhatsRook standby • waiting for session • %s\n", time.Now().Format("15:04:05"))
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintln(w, "OK")
	})

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return nil, nil, 0, fmt.Errorf("failed to bind standby server: %w", err)
	}

	boundPort := listener.Addr().(*net.TCPAddr).Port
	Logger.Info("standby HTTP server online", "port", boundPort, "addr", listener.Addr().String())

	server := &http.Server{Handler: mux}
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			Logger.Error("standby HTTP server error", "err", err)
		}
	}()

	return listener, server, boundPort, nil
}

func runInteractiveStandby(ctx context.Context, defaultDB string) error {
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

	res, shouldRun, err := tui.Run(ctx, defaultDB, boundPort)
	if err != nil {
		return err
	}
	if !shouldRun {
		return nil
	}

	botCfg := BotConfig{
		Session:         res.Session,
		Pair:            res.Pair,
		QRCode:          res.QRCode,
		ClientType:      res.ClientType,
		Database:        res.Database,
		Verbose:         res.Verbose,
		WSPort:          0,
		AsyncMessageAck: true,
	}

	return launchBotWithConfig(ctx, botCfg)
}

func launchBotWithConfig(ctx context.Context, cfg BotConfig) error {
	if cfg.Verbose {
		Logger.SetVerbose(true)
	}

	bot := NewBot(cfg)
	if err := bot.Start(ctx); err != nil {
		if errors.Is(err, whatsrook.ErrLoggedOut) {
			if ctx.Err() != nil {
				return nil
			}
			Logger.Info("session was logged out and removed; switching to standby mode")
			return runStandby(ctx, cfg.Database)
		}
		return err
	}
	return nil
}
