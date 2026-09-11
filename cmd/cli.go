package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"whatsrook"
	"whatsrook/util/logger"
)

// Version is the application version (set at build time).
var Version = "dev"

// CLIArgs holds parsed runtime arguments.
type CLIArgs struct {
	Session       string // Phone number identifying the session
	Auth          string // "pair" or "qr" (default: "qr")
	Client        string // "default", "android", or "ios"
	Business      bool   // WhatsApp Business mode (SMB platform signature)
	Database      string // PostgreSQL connection URL
	Logout        bool   // Flush credentials/session data and exit
	Update        bool   // True if an update action was requested
	UpdateOp      string // "check", "stable", "beta", or "" (direct update)
	AutoUpdate    bool   // True if autoupdate action was requested
	AutoUpdateVal string // "on", "off", or "status"
	Verbose       bool   // Enable verbose / debug logging
	Version       bool   // Print version and exit
}

// printCLIUsage prints clean plain-word command line usage without short flags or leading dashes.
func printCLIUsage() {
	fmt.Print(`Usage: whatsrook [options] [<phone>]
       whatsrook update [check | stable | beta]
       whatsrook autoupdate [on | off]
       whatsrook logout [<phone>]
       whatsrook version

Arguments:
  <phone>                       Phone number used to identify the session
                                (can appear anywhere in the argument list)

Commands & Options:
  auth <pair | qr>              Authentication method (default: qr)
  autoupdate <on | off>         Toggle automatic update checks and restarts on launch
  business                      Enable WhatsApp Business mode (uses SMB platform signature)
  client <type>                 Client profile: default (chrome), android, ios (default: default)
  db <url>                      Database: PostgreSQL connection URL
  logout                        Remove session credentials and exit
  update [action]               Check or apply update (actions: check, stable, beta, or empty for direct)
  verbose                       Enable verbose debug logging
  version                       Print version and exit
  help                          Show this help message
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

// parseCLIArgsFrom parses arguments from an explicit string slice using plain words.
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
		verboseVal    bool
		versionVal    bool
	)

	for i := 0; i < len(cmdArgs); i++ {
		raw := strings.TrimSpace(cmdArgs[i])
		if raw == "" {
			continue
		}

		// Normalize plain word: strip optional leading dashes for backward tolerance,
		// but do NOT recognize single-letter short args (e.g. -a, -c, -v, -l, -u, -h, -db).
		norm := strings.ToLower(raw)
		for strings.HasPrefix(norm, "-") {
			norm = norm[1:]
		}

		switch {
		case norm == "help" || raw == "?":
			printCLIUsage()
			os.Exit(0)

		case norm == "version":
			versionVal = true

		case norm == "verbose":
			verboseVal = true

		case norm == "logout":
			logoutVal = true

		case norm == "update":
			isUpdate = true
			if i+1 < len(cmdArgs) {
				next := strings.ToLower(strings.TrimSpace(cmdArgs[i+1]))
				for strings.HasPrefix(next, "-") {
					next = next[1:]
				}
				if next == "check" || next == "stable" || next == "beta" {
					updateOp = next
					i++
				}
			}

		case norm == "autoupdate" || norm == "auto-update":
			isAutoUpdate = true
			if i+1 < len(cmdArgs) {
				next := strings.ToLower(strings.TrimSpace(cmdArgs[i+1]))
				for strings.HasPrefix(next, "-") {
					next = next[1:]
				}
				if next == "on" || next == "off" || next == "status" || next == "enable" || next == "disable" || next == "true" || next == "false" {
					autoUpdateVal = next
					i++
				}
			}
		case strings.HasPrefix(norm, "autoupdate=") || strings.HasPrefix(norm, "auto-update="):
			isAutoUpdate = true
			_, val, _ := strings.Cut(raw, "=")
			autoUpdateVal = strings.ToLower(strings.TrimSpace(val))

		case norm == "auth":
			if i+1 < len(cmdArgs) {
				next := strings.ToLower(strings.TrimSpace(cmdArgs[i+1]))
				for strings.HasPrefix(next, "-") {
					next = next[1:]
				}
				if next == "pair" || next == "qr" {
					authVal = next
					i++
				}
			}
		case strings.HasPrefix(norm, "auth="):
			val := strings.TrimPrefix(norm, "auth=")
			if val == "pair" || val == "qr" {
				authVal = val
			}
		case norm == "pair" || norm == "qr":
			authVal = norm

		case norm == "business" || norm == "biz" || norm == "smb":
			businessVal = true

		case strings.HasPrefix(norm, "business=") || strings.HasPrefix(norm, "biz=") || strings.HasPrefix(norm, "smb="):
			_, val, _ := strings.Cut(raw, "=")
			val = strings.ToLower(strings.TrimSpace(val))
			businessVal = val == "true" || val == "1" || val == "yes" || val == "on"

		case norm == "client":
			if i+1 < len(cmdArgs) {
				next := strings.ToLower(strings.TrimSpace(cmdArgs[i+1]))
				for strings.HasPrefix(next, "-") {
					next = next[1:]
				}
				if next == "android" || next == "ios" || next == "chrome" || next == "default" ||
					next == "smb_android" || next == "smba" || next == "smb_ios" || next == "smbi" {
					clientVal = next
					i++
				}
			}
		case strings.HasPrefix(norm, "client="):
			val := strings.TrimPrefix(norm, "client=")
			if val == "android" || val == "ios" || val == "chrome" || val == "default" ||
				val == "smb_android" || val == "smba" || val == "smb_ios" || val == "smbi" {
				clientVal = val
			}

		case norm == "db" || norm == "database" || norm == "db-url" || norm == "dburl":
			if i+1 < len(cmdArgs) {
				dbVal = strings.TrimSpace(cmdArgs[i+1])
				i++
			}
		case strings.HasPrefix(norm, "db=") || strings.HasPrefix(norm, "database=") || strings.HasPrefix(norm, "db-url="):
			_, val, _ := strings.Cut(raw, "=")
			dbVal = strings.TrimSpace(val)

		default:
			// Match session phone numbers (e.g. 2348000000000, +2348000000000)
			cleanArg := strings.TrimPrefix(raw, "+")
			if len(cleanArg) >= 7 && len(cleanArg) <= 15 && isNumeric(cleanArg) {
				sessionVal = raw
			}
		}
	}

	// 1. Session resolution from env fallback
	if sessionVal == "" && !isUpdate {
		sessionVal = os.Getenv("SESSION")
	}

	// 2. Auth resolution (Plain Word > AUTH env > default "qr")
	if authVal == "" {
		authVal = strings.ToLower(strings.TrimSpace(os.Getenv("AUTH")))
	}
	if authVal != "pair" && authVal != "qr" {
		authVal = "qr"
	}

	// 3. Client platform resolution (Plain Word > CLIENT env > default "default")
	if clientVal == "" {
		clientVal = strings.ToLower(strings.TrimSpace(os.Getenv("CLIENT")))
	}
	switch clientVal {
	case "smb_android", "smba":
		clientVal = "android"
		businessVal = true
	case "smb_ios", "smbi":
		clientVal = "ios"
		businessVal = true
	case "android", "ios":
	default:
		clientVal = "default"
	}

	// 3b. Business resolution (Plain Word > BUSINESS / WA_BUSINESS env > default false)
	if !businessVal {
		envBiz := strings.ToLower(strings.TrimSpace(os.Getenv("BUSINESS")))
		envWABiz := strings.ToLower(strings.TrimSpace(os.Getenv("WA_BUSINESS")))
		businessVal = envBiz == "true" || envBiz == "1" || envBiz == "yes" || envBiz == "on" ||
			envWABiz == "true" || envWABiz == "1" || envWABiz == "yes" || envWABiz == "on"
	}

	// 4. Database resolution (Plain Word > DATABASE_URL_<phone> > DATABASE_URL > POSTGRES_URL > DB_URL > default PostgreSQL URL)
	if dbVal == "" {
		phone := strings.TrimPrefix(sessionVal, "+")
		if phone != "" && os.Getenv("DATABASE_URL_"+phone) != "" {
			dbVal = os.Getenv("DATABASE_URL_" + phone)
		} else if envDB := os.Getenv("DATABASE_URL"); envDB != "" {
			dbVal = envDB
		} else if envPG := os.Getenv("POSTGRES_URL"); envPG != "" {
			dbVal = envPG
		} else if envDBURL := os.Getenv("DB_URL"); envDBURL != "" {
			dbVal = envDBURL
		} else {
			dbVal = "postgres://postgres:postgres@localhost:5432/whatsrook?sslmode=disable"
		}
	}
	if dbVal == "default" || dbVal == "postgres" || dbVal == "postgresql" {
		dbVal = "postgres://postgres:postgres@localhost:5432/whatsrook?sslmode=disable"
	}

	// 5. Logout resolution (Plain Word > LOGOUT env)
	if !logoutVal {
		envLogout := strings.ToLower(os.Getenv("LOGOUT"))
		logoutVal = envLogout == "true" || envLogout == "1"
	}

	// 6. Verbose resolution (Plain Word > VERBOSE env > DEBUG env > LOG_LEVEL)
	if !verboseVal {
		envVerbose := strings.ToLower(strings.TrimSpace(os.Getenv("VERBOSE")))
		envDebug := strings.ToLower(strings.TrimSpace(os.Getenv("DEBUG")))
		envLogLevel := strings.ToLower(strings.TrimSpace(os.Getenv("LOG_LEVEL")))
		verboseVal = envVerbose == "true" || envVerbose == "1" || envVerbose == "yes" || envVerbose == "on" ||
			envDebug == "true" || envDebug == "1" || envDebug == "yes" || envDebug == "on" ||
			envLogLevel == "debug" || envLogLevel == "trace"
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

// handleLogoutCLI manages the command-line session logout and unpairing flow.
func handleLogoutCLI(ctx context.Context, args CLIArgs) {
	dataDir := whatsrook.DefaultDataDir()
	session := args.Session

	if session == "" {
		sessions, err := whatsrook.ListStoredSessions(ctx, dataDir, args.Database)
		if err != nil {
			logger.Error("failed to list stored sessions for logout", "err", err)
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
	logger.Info("initiating logout for session", "session", cleanSession)
	fmt.Printf("Logging out session +%s...\n", cleanSession)

	if err := whatsrook.DeleteStoredSession(ctx, dataDir, args.Database, cleanSession); err != nil {
		logger.Error("failed to complete session logout", "session", cleanSession, "err", err)
		fmt.Printf("Error: failed to complete logout for session +%s: %v\n", cleanSession, err)
		os.Exit(1)
	}

	logger.Info("session logged out and credentials removed", "session", cleanSession)
	fmt.Printf("Session +%s logged out and credentials purged successfully.\n", cleanSession)
}
