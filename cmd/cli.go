package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

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
}

// parseCLIArgs resolves environment configuration and parses CLI flags.
func parseCLIArgs() CLIArgs {
	loadDotEnv(".env", "../.env")
	return parseCLIArgsFrom(os.Args[1:])
}

// parseCLIArgsFrom parses arguments from an explicit string slice.
func parseCLIArgsFrom(cmdArgs []string) CLIArgs {
	fs := flag.NewFlagSet("whatsrook", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var (
		session   = fs.String("session", "", "Session phone number")
		auth      = fs.String("auth", "", "Authentication method: pair | qr")
		client    = fs.String("client", "", "Client platform profile: default | android | ios")
		dbURL     = fs.String("db-url", "", "Database URL: default | postgres connection string")
		logout    = fs.Bool("logout", false, "Remove session credentials and terminate")
		updateVal = fs.String("update", "__unset__", "Update operation: check | stable | beta | (empty for direct)")
		standby   = fs.Bool("standby", false, "Enter interactive standby mode")
		verbose   = fs.Bool("verbose", false, "Enable verbose debug logging")
	)

	// Short flag aliases
	fs.StringVar(session, "s", "", "Session phone number (alias)")
	fs.StringVar(auth, "a", "", "Authentication method (alias)")
	fs.StringVar(client, "c", "", "Client platform profile (alias)")
	fs.StringVar(dbURL, "db", "", "Database URL (alias)")
	fs.BoolVar(logout, "l", false, "Remove session credentials (alias)")
	fs.StringVar(updateVal, "u", "__unset__", "Update operation (alias)")
	fs.BoolVar(standby, "i", false, "Enter interactive standby mode (alias)")
	fs.BoolVar(verbose, "v", false, "Enable verbose debug logging (alias)")

	fs.Usage = func() {
		fmt.Print(`Usage: whatsrook [OPTIONS]
       whatsrook update [check | stable | beta]
       whatsrook standby

Options:
  -s, --session <phone>         Phone number used to identify the session
  -a, --auth <pair | qr>        Authentication method (default: qr)
  -c, --client <type>           Client profile: default (chrome), android, ios (default: default)
  --db-url, -db <url>           Database: default (sqlite) or PostgreSQL connection URL
  -l, --logout                  Remove session credentials and exit
  -i, --standby                 Force interactive standby session manager
  -u, --update [action]         Check or apply update (actions: check, stable, beta, or empty for direct)
  -v, --verbose                 Enable verbose debug logging
  -h, --help                    Show this help message
`)
	}

	if err := fs.Parse(cmdArgs); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
	}
	if fs.NArg() > 0 && (fs.Arg(0) == "help" || fs.Arg(0) == "--help" || fs.Arg(0) == "-h") {
		fs.Usage()
		os.Exit(0)
	}

	explicitFlags := make(map[string]bool)
	fs.Visit(func(f *flag.Flag) {
		explicitFlags[f.Name] = true
	})

	isStandby := *standby || explicitFlags["standby"] || explicitFlags["i"]
	if fs.NArg() > 0 {
		sub := strings.ToLower(strings.TrimSpace(fs.Arg(0)))
		if sub == "standby" || sub == "menu" || sub == "interactive" {
			isStandby = true
		}
	}

	// 1. Positional subcommand parsing (e.g., `whatsrook update check`)
	isUpdate := false
	updateOp := ""

	if fs.NArg() > 0 && strings.ToLower(fs.Arg(0)) == "update" {
		isUpdate = true
		if fs.NArg() > 1 {
			op := strings.ToLower(strings.TrimSpace(fs.Arg(1)))
			if op == "check" || op == "stable" || op == "beta" {
				updateOp = op
			}
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

	// 3. Session resolution (Flag > Positional phone number > SESSION env)
	sessionVal := ""
	if !isStandby {
		if explicitFlags["session"] || explicitFlags["s"] {
			sessionVal = strings.TrimSpace(*session)
		} else if fs.NArg() > 0 && !isUpdate {
			for _, arg := range fs.Args() {
				cleanArg := strings.TrimPrefix(strings.TrimSpace(arg), "+")
				if len(cleanArg) >= 7 && len(cleanArg) <= 15 && isNumeric(cleanArg) {
					sessionVal = arg
					break
				}
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

	// 7. Logout resolution (Flag > LOGOUT env)
	logoutVal := *logout
	if !explicitFlags["logout"] && !explicitFlags["l"] {
		envLogout := strings.ToLower(os.Getenv("LOGOUT"))
		logoutVal = envLogout == "true" || envLogout == "1"
	}

	// 8. Verbose resolution (Flag > VERBOSE env)
	verboseVal := *verbose
	if !explicitFlags["verbose"] && !explicitFlags["v"] {
		envVerbose := strings.ToLower(os.Getenv("VERBOSE"))
		verboseVal = envVerbose == "true" || envVerbose == "1"
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
	for _, filename := range filenames {
		data, err := os.ReadFile(filename)
		if err != nil {
			continue
		}
		for line := range strings.SplitSeq(string(data), "\n") {
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
			}
			if os.Getenv(key) == "" {
				_ = os.Setenv(key, val)
			}
		}
	}
}
