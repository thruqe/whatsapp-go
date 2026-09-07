package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Version is the application version (set at build time).
var Version = "dev"

// CLIArgs holds parsed runtime arguments.
type CLIArgs struct {
	Session  string // Phone number identifying the session
	Auth     string // "pair" or "qr" (default: "qr")
	Client   string // "default", "android", or "ios"
	Database string // "default" (sqlite) or PostgreSQL connection URL
	Logout   bool   // Flush credentials/session data and exit
	Update   bool   // True if an update action was requested
	UpdateOp string // "check", "stable", "beta", or "" (direct update)
	Verbose  bool   // Enable verbose / debug logging
	Version  bool   // Print version and exit
}

// parseCLIArgs resolves environment configuration and parses CLI flags.
func parseCLIArgs() CLIArgs {
	candidateFiles := []string{".env", "../.env"}
	if exe, err := os.Executable(); err == nil && exe != "" {
		exeDir := filepath.Dir(exe)
		candidateFiles = append(candidateFiles, filepath.Join(exeDir, ".env"), filepath.Join(exeDir, "..", ".env"))
	}
	loadDotEnv(candidateFiles...)
	return parseCLIArgsFrom(os.Args[1:])
}

// parseCLIArgsFrom parses arguments from an explicit string slice.
// The phone number (session) can appear anywhere in the argument list —
// before, after, or interleaved with flag arguments.
func parseCLIArgsFrom(cmdArgs []string) CLIArgs {
	// Handle --version early before other parsing
	if slices.Contains(cmdArgs, "--version") {
		return CLIArgs{Version: true}
	}

	fs := flag.NewFlagSet("whatsrook", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var (
		auth      = fs.String("auth", "", "Authentication method: pair | qr")
		client    = fs.String("client", "", "Client platform profile: default | android | ios")
		dbURL     = fs.String("db-url", "", "Database URL: default | postgres connection string")
		logout    = fs.Bool("logout", false, "Remove session credentials and terminate")
		updateVal = fs.String("update", "__unset__", "Update operation: check | stable | beta | (empty for direct)")
		verbose   = fs.Bool("verbose", false, "Enable verbose debug logging")
		version   = fs.Bool("version", false, "")
	)

	// Short flag aliases
	fs.StringVar(auth, "a", "", "Authentication method (alias)")
	fs.StringVar(client, "c", "", "Client platform profile (alias)")
	fs.StringVar(dbURL, "db", "", "Database URL (alias)")
	fs.BoolVar(logout, "l", false, "Remove session credentials (alias)")
	fs.StringVar(updateVal, "u", "__unset__", "Update operation (alias)")
	fs.BoolVar(verbose, "v", false, "Enable verbose debug logging (alias)")

	fs.Usage = func() {
		fmt.Print(`Usage: whatsrook [OPTIONS] [<phone>]
       whatsrook update [check | stable | beta]
       whatsrook logout [<phone>]

Arguments:
  <phone>                       Phone number used to identify the session
                                (can appear before or after any options)

Options:
  -a, --auth <pair | qr>        Authentication method (default: qr)
  -c, --client <type>           Client profile: default (chrome), android, ios (default: default)
  --db-url, -db <url>           Database: default (sqlite) or PostgreSQL connection URL
  -l, --logout                  Remove session credentials and exit
  -u, --update [action]         Check or apply update (actions: check, stable, beta, or empty for direct)
  -v, --verbose                 Enable verbose debug logging
  --version                     Print version and exit
  -h, --help                    Show this help message
`)
	}

	explicitFlags := make(map[string]bool)
	var positional []string
	argsToParse := cmdArgs
	for len(argsToParse) > 0 {
		if err := fs.Parse(argsToParse); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				os.Exit(0)
			}
			break
		}
		fs.Visit(func(f *flag.Flag) {
			explicitFlags[f.Name] = true
		})
		remaining := fs.Args()
		if len(remaining) == 0 {
			break
		}
		positional = append(positional, remaining[0])
		argsToParse = remaining[1:]
	}
	fs.Visit(func(f *flag.Flag) {
		explicitFlags[f.Name] = true
	})

	for _, p := range positional {
		if p == "help" || p == "--help" || p == "-h" {
			fs.Usage()
			os.Exit(0)
		}
	}

	// 1. Positional subcommand parsing (e.g., `whatsrook update check`, `whatsrook logout`)
	isUpdate := false
	updateOp := ""
	for i, p := range positional {
		if strings.ToLower(p) == "update" {
			isUpdate = true
			if i+1 < len(positional) {
				op := strings.ToLower(strings.TrimSpace(positional[i+1]))
				if op == "check" || op == "stable" || op == "beta" {
					updateOp = op
				}
			}
			break
		}
	}

	isLogout := false
	for _, p := range positional {
		if strings.ToLower(p) == "logout" {
			isLogout = true
			break
		}
	}

	// 2. Flag-based update parsing (-update, -u, --update=beta, etc.)
	if explicitFlags["update"] || explicitFlags["u"] {
		isUpdate = true
		val := strings.ToLower(strings.TrimSpace(*updateVal))
		if val != "__unset__" && (val == "check" || val == "stable" || val == "beta") {
			updateOp = val
		}
	}

	// 3. Session resolution: scan all original args (before and after flags) for a
	//    phone number, then fall back to the SESSION env variable.
	sessionVal := ""
	if !isUpdate {
		// Search through ALL original args (not just positional remainders) so the
		// phone number can appear anywhere — e.g. `whatsrook +1234 -v` or
		// `whatsrook -v +1234`.
		for _, arg := range cmdArgs {
			// Skip anything that looks like a flag or a flag value
			if strings.HasPrefix(arg, "-") {
				continue
			}
			// Skip known subcommand words
			lower := strings.ToLower(strings.TrimSpace(arg))
			if lower == "update" || lower == "check" || lower == "stable" || lower == "beta" || lower == "logout" {
				continue
			}
			cleanArg := strings.TrimPrefix(strings.TrimSpace(arg), "+")
			if len(cleanArg) >= 7 && len(cleanArg) <= 15 && isNumeric(cleanArg) {
				sessionVal = arg
				break
			}
		}
		if sessionVal == "" {
			sessionVal = os.Getenv("SESSION")
		}
	}

	// 4. Auth resolution (Flag > AUTH env > default "qr")
	authVal := strings.ToLower(strings.TrimSpace(*auth))
	if authVal == "" {
		authVal = strings.ToLower(strings.TrimSpace(os.Getenv("AUTH")))
	}
	if authVal != "pair" && authVal != "qr" {
		authVal = "qr"
	}

	// 5. Client platform resolution (Flag > CLIENT env > default "default")
	clientVal := strings.ToLower(strings.TrimSpace(*client))
	if clientVal == "" {
		clientVal = strings.ToLower(strings.TrimSpace(os.Getenv("CLIENT")))
	}
	switch clientVal {
	case "android", "ios":
	default:
		clientVal = "default"
	}

	// 6. Database resolution (Flag > DATABASE_URL_<phone> > DATABASE_URL > DB_URL > "default")
	dbVal := strings.TrimSpace(*dbURL)
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
			dbVal = "default"
		}
	}

	// 7. Logout resolution (Subcommand > Flag > LOGOUT env)
	logoutVal := *logout || isLogout
	if !explicitFlags["logout"] && !explicitFlags["l"] && !isLogout {
		envLogout := strings.ToLower(os.Getenv("LOGOUT"))
		logoutVal = envLogout == "true" || envLogout == "1"
	}

	// 8. Verbose resolution (Flag > VERBOSE env > DEBUG env > LOG_LEVEL)
	verboseVal := *verbose
	if !explicitFlags["verbose"] && !explicitFlags["v"] {
		envVerbose := strings.ToLower(strings.TrimSpace(os.Getenv("VERBOSE")))
		envDebug := strings.ToLower(strings.TrimSpace(os.Getenv("DEBUG")))
		envLogLevel := strings.ToLower(strings.TrimSpace(os.Getenv("LOG_LEVEL")))
		verboseVal = envVerbose == "true" || envVerbose == "1" || envVerbose == "yes" || envVerbose == "on" ||
			envDebug == "true" || envDebug == "1" || envDebug == "yes" || envDebug == "on" ||
			envLogLevel == "debug" || envLogLevel == "trace"
	}

	return CLIArgs{
		Session:  sessionVal,
		Auth:     authVal,
		Client:   clientVal,
		Database: dbVal,
		Logout:   logoutVal,
		Update:   isUpdate,
		UpdateOp: updateOp,
		Verbose:  verboseVal,
		Version:  *version,
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
