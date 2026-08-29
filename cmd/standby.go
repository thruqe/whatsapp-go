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
	"whatsrook/cmd/tui"
	Logger "whatsrook/src/logger"
)

func runStandby(ctx context.Context, defaultDB string) error {
	if isTerminalInteractive() {
		return runInteractiveStandby(defaultDB)
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

	Logger.Info("headless standby mode active; awaiting configuration", "port", boundPort)
	<-ctx.Done()
	return nil
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
	Logger.Info("standby HTTP server online", "port", boundPort, "addr", listener.Addr().String())

	server := &http.Server{Handler: mux}
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			Logger.Error("standby HTTP server error", "err", err)
		}
	}()

	return listener, server, boundPort, nil
}

func runInteractiveStandby(defaultDB string) error {
	for {
		listener, server, boundPort, err := startStandbyHTTPServer()
		if err != nil {
			return err
		}

		tuiCtx, tuiCancel := context.WithCancel(context.Background())
		res, shouldRun, err := tui.Run(tuiCtx, defaultDB, boundPort)
		tuiCancel()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = server.Shutdown(shutdownCtx)
		_ = listener.Close()
		cancel()

		if err != nil {
			return err
		}
		if !shouldRun {
			tui.ClearTerminal()
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

		// When bot runs, listen for Ctrl+C to interrupt the bot and cycle back to the TUI
		botCtx, botCancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		errBot := launchBotWithConfig(botCtx, botCfg)
		botCancel()

		// Always clear console screen when session exits/interrupts, then loop back to interactive standby
		tui.ClearTerminal()

		if errBot != nil && !errors.Is(errBot, context.Canceled) && !errors.Is(errBot, whatsrook.ErrLoggedOut) {
			Logger.Warn("session disconnected, returning to standby menu", "err", errBot)
		}
	}
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
