//go:generate go -C ../scripts run . res

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/thruqe/whatsrook"
)

// Version is the application version (set at build time).
var Version = "dev"

// CLIArgs holds parsed runtime arguments.
type CLIArgs struct {
	Session       string // Phone number identifying the session
	Auth          string // "pair" or "qr" (default: "qr")
	Client        string // "default", "android", or "ios"
	Business      bool   // WhatsApp Business mode (SMB platform signature)
	Database      string // Database connection URL / file path
	Logout        bool   // Flush credentials/session data and exit
	Update        bool   // True if an update action was requested
	UpdateOp      string // "check", "stable", "beta", or "" (direct update)
	AutoUpdate    bool   // True if autoupdate action was requested
	AutoUpdateVal string // "on" or "off"
	Icon          bool   // Apply and verify binary icon and exit
	IconTarget    string // Optional target binary path for icon verification
	Verbose       bool   // Enable verbose / debug logging
	Version       bool   // Print version and exit
}

func main() {
	defer func() {
		if r := recover(); r != nil {
			crashPath := RecordCrash(r, "top-level process panic")
			Error("Fatal runtime crash", "panic", r, "crash_log", crashPath)
			os.Exit(1)
		}
	}()

	EnsureBinaryIcon()

	args := parseCLIArgs()

	if args.Version {
		v := Version
		if v == "dev" || v == "" {
			v = GetBinaryVersion()
		}
		fmt.Println(v)
		return
	}

	if args.Icon {
		target := args.IconTarget
		if target == "" {
			exe, err := os.Executable()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: failed to resolve executable path: %v\n", err)
				os.Exit(1)
			}
			target = exe
		}
		report, err := VerifyBinaryIcon(target)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Icon verification failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Verified %s (%s) icon on %s\n", report.OS, report.BinaryFormat, report.BinaryPath)
		for _, detail := range report.Details {
			fmt.Printf("  - %s\n", detail)
		}
		return
	}

	if args.Verbose {
		SetVerbose(true)
	}

	if args.Update {
		handleUpdate(args.UpdateOp)
		return
	}

	if args.AutoUpdate {
		handleAutoUpdateCLI(args.AutoUpdateVal)
		return
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if GetStoredAutoUpdate() && os.Getenv("_WHATSRROK_AUTOUPDATE_RESTARTED") != "1" {
		ctxAuto, cancelAuto := context.WithTimeout(ctx, 45*time.Second)
		updated, err := PerformAutoUpdate(ctxAuto, os.Stdout)
		cancelAuto()
		if err != nil {
			Warn("auto-update check failed", "err", err)
		} else if updated {
			fmt.Println("==> Auto-update applied successfully! Restarting WhatsRook...")
			_ = os.Setenv("_WHATSRROK_AUTOUPDATE_RESTARTED", "1")
			err := RestartProcess()
			Error("failed to restart process after auto-update", "err", err)
			return
		}
	}

	// Resolve database target: keep args.Database empty only when using default SQLite fallback
	rawDBArg := strings.TrimSpace(args.Database)
	dialect, resolvedDB := ResolveDatabaseTarget(rawDBArg)
	if dialect == "postgres" || rawDBArg != "" {
		args.Database = resolvedDB
	} else {
		args.Database = ""
	}

	if args.Logout {
		handleLogoutCLI(ctx, args)
		return
	}

	Init(10000)
	defer func() {
		_ = CacheClose()
	}()

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
		Business:        args.Business,
		Database:        args.Database,
		Verbose:         args.Verbose,
		AsyncMessageAck: true,
	})

	err := bot.Start(ctx)
	if err != nil {
		Error("bot execution failure", "err", err)
		os.Exit(1)
	}
}

// printCLIUsage prints clean plain-word command line usage.
func printCLIUsage() {
	fmt.Print(`Usage: whatsrook [options] [<phone> | <SE_ID:...>]
       whatsrook update [check | stable | beta]
       whatsrook autoupdate [on | off]
       whatsrook logout [<phone>]
       whatsrook icon [binary]
       whatsrook version

Arguments:
  <phone>                    Phone number for the WhatsApp account
  <SE_ID:...>                Session restore token

Commands & Options:
  auth <pair | qr>           Login method: "pair" (phone code) or "qr" (QR code) (default: qr)
  autoupdate <on | off>      Turn automatic updates on or off
  business                   Connect as WhatsApp Business
  client <type>              Device style: default, android, ios (default: default)
  db <url>                   Custom database URL or file path (default: local database)
  icon [binary]              Apply and verify OS desktop icon for the binary
  logout                     Log out and disconnect session
  update [action]            Check or install updates (check, stable, beta)
  verbose                    Show detailed technical logs
  version                    Show app version
  help                       Show this help guide
`)
}

// parseCLIArgs resolves environment configuration and parses CLI args.
func parseCLIArgs() CLIArgs {
	candidateFiles := []string{".env", "../.env"}
	if exe, err := os.Executable(); err == nil && exe != "" {
		exeDir := filepath.Dir(exe)
		candidateFiles = append(candidateFiles, filepath.Join(exeDir, ".env"), filepath.Join(exeDir, "..", ".env"))
	}
	loadDotEnv(candidateFiles...)
	return parseCLIArgsFrom(os.Args[1:])
}

// parseCLIArgsFrom parses arguments deterministically with strict single-token behaviors.
func parseCLIArgsFrom(cmdArgs []string) CLIArgs {
	var (
		sessionVal    string
		authVal       string
		clientVal     string
		businessVal   bool
		dbVal         string
		logoutVal     bool
		isUpdate      bool
		updateOp      string
		isAutoUpdate  bool
		autoUpdateVal string
		isIcon        bool
		iconTarget    string
		verboseVal    bool
		versionVal    bool
	)

	for i := 0; i < len(cmdArgs); i++ {
		raw := strings.TrimSpace(cmdArgs[i])
		if raw == "" {
			continue
		}

		switch raw {
		case "help":
			printCLIUsage()
			os.Exit(0)

		case "version":
			versionVal = true

		case "icon":
			isIcon = true
			if i+1 < len(cmdArgs) {
				next := strings.TrimSpace(cmdArgs[i+1])
				if next != "" {
					iconTarget = next
					i++
				}
			}

		case "verbose":
			verboseVal = true

		case "logout":
			logoutVal = true

		case "business":
			businessVal = true

		case "update":
			isUpdate = true
			if i+1 < len(cmdArgs) {
				next := strings.TrimSpace(cmdArgs[i+1])
				if next == "check" || next == "stable" || next == "beta" {
					updateOp = next
					i++
				}
			}

		case "autoupdate":
			isAutoUpdate = true
			if i+1 < len(cmdArgs) {
				next := strings.TrimSpace(cmdArgs[i+1])
				if next == "on" || next == "off" {
					autoUpdateVal = next
					i++
				}
			}

		case "auth":
			if i+1 < len(cmdArgs) {
				next := strings.TrimSpace(cmdArgs[i+1])
				if next == "pair" || next == "qr" {
					authVal = next
					i++
				}
			}

		case "client":
			if i+1 < len(cmdArgs) {
				next := strings.TrimSpace(cmdArgs[i+1])
				if next == "android" || next == "ios" || next == "default" {
					clientVal = next
					i++
				}
			}

		case "db":
			if i+1 < len(cmdArgs) {
				dbVal = strings.TrimSpace(cmdArgs[i+1])
				i++
			}

		default:
			if strings.HasPrefix(raw, "SE_ID:") {
				sessionVal = raw
				continue
			}

			cleanArg := strings.TrimPrefix(raw, "+")
			if len(cleanArg) >= 7 && len(cleanArg) <= 15 && isNumeric(cleanArg) {
				sessionVal = raw
			}
		}
	}

	if sessionVal == "" && !isUpdate {
		if val := os.Getenv("SESSION_ID"); strings.TrimSpace(val) != "" {
			sessionVal = strings.TrimSpace(val)
		} else if val := os.Getenv("SESSION"); strings.TrimSpace(val) != "" {
			sessionVal = strings.TrimSpace(val)
		}
	}

	if strings.HasPrefix(sessionVal, "SE_ID:") {
		payload, err := DecryptSessionID(sessionVal)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to decrypt session credential: %v\n", err)
			os.Exit(1)
		}

		sessionVal = payload.Phone

		if authVal == "" && payload.Auth != "" {
			authVal = payload.Auth
		}
		if clientVal == "" && payload.Client != "" {
			clientVal = payload.Client
		}
		if !businessVal && payload.Business {
			businessVal = true
		}
		if dbVal == "" && payload.DB != "" {
			dbVal = payload.DB
		}
		if !verboseVal && payload.Verbose {
			verboseVal = true
		}
	}

	if authVal == "" {
		authVal = os.Getenv("AUTH")
	}
	if authVal != "pair" && authVal != "qr" {
		authVal = "qr"
	}

	if clientVal == "" {
		clientVal = os.Getenv("CLIENT")
	}
	if clientVal != "android" && clientVal != "ios" {
		clientVal = "default"
	}

	if !businessVal {
		businessVal = os.Getenv("BUSINESS") == "true"
	}

	if dbVal == "" {
		dbVal = os.Getenv("DATABASE_URL")
	}

	if !verboseVal {
		verboseVal = os.Getenv("VERBOSE") == "true"
	}

	return CLIArgs{
		Session:       sessionVal,
		Auth:          authVal,
		Client:        clientVal,
		Business:      businessVal,
		Database:      dbVal,
		Logout:        logoutVal,
		Update:        isUpdate,
		UpdateOp:      updateOp,
		AutoUpdate:    isAutoUpdate,
		AutoUpdateVal: autoUpdateVal,
		Icon:          isIcon,
		IconTarget:    iconTarget,
		Verbose:       verboseVal,
		Version:       versionVal,
	}
}

func isNumeric(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func loadDotEnv(filenames ...string) {
	seen := make(map[string]bool)
	for _, filename := range filenames {
		if filename == "" || seen[filename] {
			continue
		}
		seen[filename] = true
		data, err := os.ReadFile(filename)
		if err != nil {
			continue
		}
		for rawLine := range strings.SplitSeq(string(data), "\n") {
			line := strings.TrimSpace(rawLine)
			line = strings.TrimPrefix(line, "export ")
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])

			if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'')) {
				val = val[1 : len(val)-1]
			} else if idx := strings.Index(val, " #"); idx != -1 {
				val = strings.TrimSpace(val[:idx])
			}

			if os.Getenv(key) == "" {
				_ = os.Setenv(key, val)
			}
		}
	}
}

func handleLogoutCLI(ctx context.Context, args CLIArgs) {
	dataDir := whatsrook.DefaultDataDir()
	session := args.Session

	if session == "" {
		sessions, err := whatsrook.ListStoredSessions(ctx, dataDir, args.Database)
		if err != nil {
			Error("failed to list stored sessions for logout", "err", err)
			fmt.Printf("Error: failed to query stored sessions: %v\n", err)
			os.Exit(1)
		}
		if len(sessions) == 0 {
			fmt.Println("No active stored sessions found to logout.")
			return
		}
		if len(sessions) == 1 {
			session = sessions[0].User
			fmt.Printf("Found single stored session +%s (%s). Initiating logout...\n", session, sessions[0].Platform)
		} else {
			fmt.Println("\nStored sessions:")
			for i, s := range sessions {
				fmt.Printf("  [%d] +%s (%s)\n", i+1, s.User, s.Platform)
			}
			fmt.Print("\nEnter session number or phone to logout (or Ctrl+C to exit): ")
			var input string
			if _, err := fmt.Scan(&input); err != nil {
				return
			}
			input = strings.TrimSpace(input)
			if idx, err := strconv.Atoi(input); err == nil && idx >= 1 && idx <= len(sessions) {
				session = sessions[idx-1].User
			} else {
				cleanInput := strings.TrimPrefix(input, "+")
				for _, s := range sessions {
					if s.User == cleanInput || s.User == input {
						session = s.User
						break
					}
				}
				if session == "" {
					session = cleanInput
				}
			}
		}
	}

	cleanSession := strings.TrimPrefix(session, "+")
	Debug("initiating logout for session", "session", cleanSession)
	fmt.Printf("Logging out session +%s...\n", cleanSession)

	if err := whatsrook.DeleteStoredSession(ctx, dataDir, args.Database, cleanSession); err != nil {
		Error("failed to complete session logout", "session", cleanSession, "err", err)
		fmt.Printf("Error: failed to complete logout for session +%s: %v\n", cleanSession, err)
		os.Exit(1)
	}

	Debug("session logged out and credentials removed", "session", cleanSession)
	fmt.Printf("Session +%s logged out and credentials purged successfully.\n", cleanSession)
}

func handleUpdate(op string) {
	ctx := context.Background()

	var targetChannel string
	switch op {
	case "stable", "beta":
		targetChannel = op
		current := GetStoredChannel()
		if op != current {
			fmt.Printf("==> Switching release channel: %s -> %s\n", current, op)
			if err := SetStoredChannel(op); err != nil {
				Error("failed to set release channel", "err", err)
				os.Exit(1)
			}
		} else {
			fmt.Printf("==> Already tracking channel: %s\n", current)
		}
	default:
		if CurrentIsBeta() {
			targetChannel = "beta"
		} else {
			targetChannel = "stable"
		}
	}

	if op == "check" {
		up := New(Options{
			Out:     os.Stdout,
			Channel: targetChannel,
		})
		if _, err := up.Check(ctx); err != nil {
			Error("update check failed", "err", err)
			os.Exit(1)
		}
		return
	}

	up := New(Options{
		Out:     os.Stdout,
		Channel: targetChannel,
	})

	res, err := up.Upgrade(ctx, targetChannel == "beta")
	if err != nil {
		Error("upgrade procedure failed", "err", err)
		os.Exit(1)
	}

	if res.Updated {
		fmt.Println("==> Upgrade complete!")
	}
}

func handleAutoUpdateCLI(val string) {
	switch val {
	case "on":
		if err := SetStoredAutoUpdate(true); err != nil {
			Error("failed to enable auto-update", "err", err)
			os.Exit(1)
		}
		fmt.Println("==> Auto-update enabled. WhatsRook will automatically check and apply updates on startup.")
	case "off":
		if err := SetStoredAutoUpdate(false); err != nil {
			Error("failed to disable auto-update", "err", err)
			os.Exit(1)
		}
		fmt.Println("==> Auto-update disabled.")
	default:
		if GetStoredAutoUpdate() {
			fmt.Println("Auto-update is currently enabled (ON).")
		} else {
			fmt.Println("Auto-update is currently disabled (OFF).")
		}
	}
}
