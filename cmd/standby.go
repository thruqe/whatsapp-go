package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"whatsrook"
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

	reader := bufio.NewReader(os.Stdin)

	for {
		if ctx.Err() != nil {
			return nil
		}

		fmt.Println()
		fmt.Printf("⚡ WhatsRook standby (port :%d) • waiting for session • %s\n", boundPort, time.Now().Format("15:04:05"))
		fmt.Println("────────────────────────────────────────────────────────────")
		fmt.Println("  [1] Connect to an existing session")
		fmt.Println("  [2] Create a new session")
		fmt.Println("  [3] Exit")
		fmt.Println("────────────────────────────────────────────────────────────")
		fmt.Print("Select an option [1-3]: ")

		input, err := reader.ReadString('\n')
		if err != nil {
			return nil
		}
		choice := strings.TrimSpace(input)

		switch choice {
		case "1":
			botCfg, shouldRun := handleConnectExisting(ctx, reader, defaultDB)
			if shouldRun {
				return launchBotWithConfig(ctx, botCfg)
			}
		case "2":
			botCfg, shouldRun := handleCreateNewSession(ctx, reader, defaultDB)
			if shouldRun {
				return launchBotWithConfig(ctx, botCfg)
			}
		case "3", "exit", "q":
			fmt.Println("Exiting WhatsRook.")
			return nil
		default:
			fmt.Println("Invalid choice. Please enter 1, 2, or 3.")
		}
	}
}

func handleConnectExisting(ctx context.Context, reader *bufio.Reader, defaultDB string) (BotConfig, bool) {
	dataDir := whatsrook.DefaultDataDir()
	sessions, err := whatsrook.ListStoredSessions(ctx, dataDir, defaultDB)
	if err != nil {
		fmt.Printf("\n❌ Error querying saved sessions: %v\n", err)
		return BotConfig{}, false
	}

	if len(sessions) == 0 {
		fmt.Println("\n⚠️  No saved sessions found in database.")
		fmt.Print("Press [Enter] to return to main menu...")
		_, _ = reader.ReadString('\n')
		return BotConfig{}, false
	}

	for {
		fmt.Println("\n📱 Saved WhatsApp Sessions:")
		for i, s := range sessions {
			name := s.PushName
			if name == "" {
				name = "Personal"
			}
			fmt.Printf("  [%d] +%s (%s • %s)\n", i+1, s.User, name, s.Platform)
		}
		fmt.Println("  [0] Back to main menu")
		fmt.Print("\nSelect a session: ")

		input, err := reader.ReadString('\n')
		if err != nil {
			return BotConfig{}, false
		}
		choiceStr := strings.TrimSpace(input)
		if choiceStr == "0" || choiceStr == "back" {
			return BotConfig{}, false
		}

		idx, err := strconv.Atoi(choiceStr)
		if err != nil || idx < 1 || idx > len(sessions) {
			fmt.Println("Invalid selection. Please choose a valid session number.")
			continue
		}

		chosen := sessions[idx-1]
		phone := "+" + chosen.User

		clientType := whatsrook.ClientChrome
		if strings.EqualFold(chosen.Platform, "Android") {
			clientType = whatsrook.ClientAndroid
		} else if strings.EqualFold(chosen.Platform, "iOS") {
			clientType = whatsrook.ClientIos
		}

		botCfg := BotConfig{
			Session:         phone,
			ClientType:      clientType,
			Database:        defaultDB,
			Verbose:         false,
			WSPort:          0,
			AsyncMessageAck: true,
		}

		for {
			fmt.Printf("\n⚙️  Session Actions (+%s • %s):\n", chosen.User, chosen.PushName)
			fmt.Println("  [1] Run session")
			fmt.Println("  [2] Edit session variables")
			fmt.Println("  [3] Delete session")
			fmt.Println("  [0] Back")
			fmt.Print("Select option [1-3]: ")

			actionInput, err := reader.ReadString('\n')
			if err != nil {
				return BotConfig{}, false
			}
			action := strings.TrimSpace(actionInput)

			switch action {
			case "1":
				return botCfg, true
			case "2":
				botCfg = handleEditSessionVariables(reader, botCfg)
			case "3":
				fmt.Printf("⚠️  Are you sure you want to delete session +%s? [y/N]: ", chosen.User)
				confirm, _ := reader.ReadString('\n')
				if strings.ToLower(strings.TrimSpace(confirm)) == "y" {
					if err := whatsrook.DeleteStoredSession(ctx, dataDir, defaultDB, phone); err != nil {
						fmt.Printf("❌ Failed to delete session: %v\n", err)
					} else {
						fmt.Printf("✅ Session +%s deleted successfully.\n", chosen.User)
						return BotConfig{}, false
					}
				}
			case "0", "back":
				// back to session list
			default:
				fmt.Println("Invalid option.")
				continue
			}
			if action == "0" || action == "back" {
				break
			}
		}
	}
}

func handleEditSessionVariables(reader *bufio.Reader, cfg BotConfig) BotConfig {
	for {
		clientName := "Default (Chrome)"
		switch cfg.ClientType {
		case whatsrook.ClientAndroid:
			clientName = "Android"
		case whatsrook.ClientIos:
			clientName = "iOS"
		}

		logLevel := "Standard (INFO)"
		if cfg.Verbose {
			logLevel = "Verbose (DEBUG)"
		}

		dbName := cfg.Database
		if dbName == "" || dbName == "default" {
			dbName = "default (SQLite)"
		}

		fmt.Printf("\n📝 Current Variables for %s:\n", cfg.Session)
		fmt.Printf("  [1] Client Profile: %s\n", clientName)
		fmt.Printf("  [2] Logging Level: %s\n", logLevel)
		fmt.Printf("  [3] Database: %s\n", dbName)
		fmt.Println("  [4] Done editing")
		fmt.Print("Select variable to modify [1-4]: ")

		input, _ := reader.ReadString('\n')
		switch strings.TrimSpace(input) {
		case "1":
			fmt.Println("\nChoose Client Platform:")
			fmt.Println("  [1] Default (Desktop / Chrome)")
			fmt.Println("  [2] Android")
			fmt.Println("  [3] iOS")
			fmt.Print("Choice [1-3]: ")
			cInput, _ := reader.ReadString('\n')
			switch strings.TrimSpace(cInput) {
			case "2":
				cfg.ClientType = whatsrook.ClientAndroid
			case "3":
				cfg.ClientType = whatsrook.ClientIos
			default:
				cfg.ClientType = whatsrook.ClientChrome
			}
		case "2":
			fmt.Println("\nChoose Logging Level:")
			fmt.Println("  [1] Standard (INFO)")
			fmt.Println("  [2] Verbose (DEBUG)")
			fmt.Print("Choice [1-2]: ")
			lInput, _ := reader.ReadString('\n')
			cfg.Verbose = strings.TrimSpace(lInput) == "2"
		case "3":
			fmt.Print("\nEnter Database URL (or 'default' for SQLite): ")
			dInput, _ := reader.ReadString('\n')
			dVal := strings.TrimSpace(dInput)
			if dVal != "" {
				cfg.Database = dVal
			}
		case "4", "done", "0":
			return cfg
		}
	}
}

func handleCreateNewSession(ctx context.Context, reader *bufio.Reader, defaultDB string) (BotConfig, bool) {
	fmt.Println("\n🔑 Create New WhatsApp Session")
	fmt.Println("────────────────────────────────────────────────────────────")
	fmt.Println("Select Authentication Method:")
	fmt.Println("  [1] QR Code (scan from WhatsApp Linked Devices)")
	fmt.Println("  [2] Pairing Code (enter phone number & receive 8-digit code)")
	fmt.Println("  [0] Back")
	fmt.Print("Choice [1-2, 0 to back]: ")

	authInput, _ := reader.ReadString('\n')
	authChoice := strings.TrimSpace(authInput)

	if authChoice == "0" || authChoice == "back" {
		return BotConfig{}, false
	}

	isPair := authChoice == "2"
	isQR := authChoice == "1" || (!isPair)

	sessionPhone := ""
	if isPair {
		for {
			fmt.Print("\nEnter phone number with country code (e.g. +2348062795602): ")
			pInput, _ := reader.ReadString('\n')
			phone := strings.TrimSpace(pInput)
			clean := strings.TrimPrefix(phone, "+")
			if len(clean) >= 7 && len(clean) <= 15 && isNumeric(clean) {
				sessionPhone = phone
				break
			}
			fmt.Println("Invalid phone number format. Please include country code without spaces.")
		}
	} else {
		fmt.Print("\nEnter session name or phone number [leave blank to auto-detect]: ")
		pInput, _ := reader.ReadString('\n')
		sessionPhone = strings.TrimSpace(pInput)
		if sessionPhone == "" {
			sessionPhone = "session_" + strconv.FormatInt(time.Now().Unix(), 10)
		}
	}

	// Choose client platform profile
	fmt.Println("\nSelect Client Platform:")
	fmt.Println("  [1] Default (Desktop / Chrome)")
	fmt.Println("  [2] Android")
	fmt.Println("  [3] iOS")
	fmt.Print("Choice [1-3]: ")
	cInput, _ := reader.ReadString('\n')
	clientType := whatsrook.ClientChrome
	switch strings.TrimSpace(cInput) {
	case "2":
		clientType = whatsrook.ClientAndroid
	case "3":
		clientType = whatsrook.ClientIos
	}

	// Choose logging level
	fmt.Println("\nSelect Logging Level:")
	fmt.Println("  [1] Standard (INFO)")
	fmt.Println("  [2] Verbose (DEBUG)")
	fmt.Print("Choice [1-2]: ")
	lInput, _ := reader.ReadString('\n')
	verbose := strings.TrimSpace(lInput) == "2"

	botCfg := BotConfig{
		Session:         sessionPhone,
		Pair:            isPair,
		QRCode:          isQR,
		ClientType:      clientType,
		Database:        defaultDB,
		Verbose:         verbose,
		WSPort:          0,
		AsyncMessageAck: true,
	}

	return botCfg, true
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
