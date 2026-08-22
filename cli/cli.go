package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// CLIArgs holds parsed command-line flags.
type CLIArgs struct {
	Session         string
	Pair            bool
	QRCode          bool
	Logout          bool
	Update          bool
	UpdateChannel   string // "stable", "beta", or "" (use stored preference)
	Verbose         bool
	Client          string
	Database        string
	SkipOldMessages bool
	Port            int
}

func parseCLIArgs() CLIArgs {
	loadDotEnv(".env", "../.env")
	return parseCLIArgsFrom(os.Args[1:])
}

func parseCLIArgsFrom(cmdArgs []string) CLIArgs {
	fs := flag.NewFlagSet("whatsrook", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	defaultPort := getEnvInt("PORT", getEnvInt("WS_PORT", 3000))
	var (
		session  = fs.String("s", "", "")
		pair     = fs.Bool("p", false, "")
		client   = fs.String("c", "", "")
		database = fs.String("db", "", "")
		port     = fs.Int("P", defaultPort, "")
		qr       = fs.Bool("q", false, "")
		logout   = fs.Bool("l", false, "")
		update   = fs.Bool("u", false, "")
		verbose  = fs.Bool("v", false, "")
		noSkip   = fs.Bool("no-skip-old", false, "")
	)

	fs.StringVar(session, "session", "", "")
	fs.BoolVar(pair, "pair", false, "")
	fs.StringVar(client, "client", "", "")
	fs.StringVar(database, "database", "", "")
	fs.IntVar(port, "port", defaultPort, "")
	fs.BoolVar(qr, "qrcode", false, "")
	fs.BoolVar(logout, "logout", false, "")
	fs.BoolVar(update, "update", false, "")
	fs.BoolVar(verbose, "verbose", false, "")

	fs.Usage = func() {
		fmt.Print(`Usage: whatsrook [-session <phone_number>] [OPTIONS]
       whatsrook update [check | upgrade]
       whatsrook --update [stable | beta]

Options:
  -s, --session <phone>  Phone number used to identify the session (runs in idle mode if omitted)
  -p, --pair             Request a pair code using the --session phone number
  -P, --port <port>      WebSocket/HTTP server port (default: 3000 or $PORT)
  -c, --client <type>    Client type: chrome (default), android, ios
  -db, --database <url>  Database connection: sqlite (default) or postgres URL. Per-session override: DATABASE_URL_<phone>
  -q, --qrcode           Print the QR code to stdout for scanning
  -l, --logout           Remove the session auth files and exit
  -u, --update [channel] Check and perform update; optionally pass "stable" or "beta" to
                         switch channels (prints a notice if already on the requested channel)
  -v, --verbose          Enable verbose logging
  --no-skip-old          Process messages sent while the bot was offline (default: skip them)
  -h, --help             Show this help message
`)
	}

	_ = fs.Parse(cmdArgs)

	// Record which flags were explicitly set via command-line arguments
	explicitFlags := make(map[string]bool)
	fs.Visit(func(f *flag.Flag) {
		explicitFlags[f.Name] = true
	})

	// 1. Session resolution (CLI flag > Positional arg > SESSION env var)
	sessionVal := ""
	if explicitFlags["s"] || explicitFlags["session"] {
		sessionVal = *session
	}

	var updateChannel string
	if fs.NArg() > 0 {
		for _, arg := range fs.Args() {
			lower := strings.ToLower(strings.TrimSpace(arg))
			// Capture an explicit channel switch alongside -u/--update.
			if (explicitFlags["u"] || explicitFlags["update"]) && (lower == "stable" || lower == "beta") {
				updateChannel = lower
				continue
			}
			// Positional phone number detection
			if sessionVal == "" {
				cleanArg := strings.TrimPrefix(arg, "+")
				if len(cleanArg) >= 7 && len(cleanArg) <= 15 {
					allDigits := true
					for _, r := range cleanArg {
						if r < '0' || r > '9' {
							allDigits = false
							break
						}
					}
					if allDigits {
						sessionVal = arg
					}
				}
			}
		}
	}
	if sessionVal == "" {
		sessionVal = os.Getenv("SESSION")
	}

	// 2. Pair vs QRCode resolution (CLI flags strictly override env vars)
	isQRFlag := explicitFlags["q"] || explicitFlags["qrcode"]
	isPairFlag := explicitFlags["p"] || explicitFlags["pair"]

	var pairVal bool
	var qrVal bool

	if isQRFlag {
		qrVal = *qr
		pairVal = false // Explicit -q forces QR mode and cancels any PAIR in env
	} else if isPairFlag {
		pairVal = *pair
		qrVal = false // Explicit -p forces Pair mode and cancels any QRCODE in env
	} else {
		// Fallback to environment variables if neither flag was specified
		qrVal = getEnvBool("QRCODE")
		pairVal = getEnvBool("PAIR")
		if qrVal && pairVal {
			pairVal = false // Default to QR if both were set in env
		}
	}

	// 3. Client identity resolution (CLI flag > CLIENT env var > default "chrome")
	clientVal := "chrome"
	if explicitFlags["c"] || explicitFlags["client"] {
		clientVal = *client
	} else if envClient := os.Getenv("CLIENT"); envClient != "" {
		clientVal = envClient
	}

	// 4. Database URL resolution (CLI flag > DATABASE_URL_<phone> > DATABASE_URL/POSTGRES_URL/DB_URL > "sqlite")
	var dbVal string
	if explicitFlags["db"] || explicitFlags["database"] {
		dbVal = *database
	} else {
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
			dbVal = "sqlite"
		}
	}

	// 5. Port resolution (CLI flag > PORT / WS_PORT env var > 3000)
	portVal := defaultPort
	if explicitFlags["P"] || explicitFlags["port"] {
		portVal = *port
	}

	// 6. Other boolean flags (CLI flag > env var)
	logoutVal := *logout
	if !explicitFlags["l"] && !explicitFlags["logout"] {
		logoutVal = getEnvBool("LOGOUT")
	}

	updateVal := *update
	if !explicitFlags["u"] && !explicitFlags["update"] {
		updateVal = getEnvBool("UPDATE")
	}

	verboseVal := *verbose
	if !explicitFlags["v"] && !explicitFlags["verbose"] {
		verboseVal = getEnvBool("VERBOSE")
	}

	skipOldVal := true
	if explicitFlags["no-skip-old"] {
		skipOldVal = !*noSkip
	} else if envSkip := os.Getenv("SKIP_OLD_MESSAGES"); envSkip != "" {
		skipOldVal = getEnvBool("SKIP_OLD_MESSAGES")
	}

	return CLIArgs{
		Session:         sessionVal,
		Pair:            pairVal,
		QRCode:          qrVal,
		Logout:          logoutVal,
		Update:          updateVal,
		UpdateChannel:   updateChannel,
		Verbose:         verboseVal,
		Client:          clientVal,
		Database:        dbVal,
		SkipOldMessages: skipOldVal,
		Port:            portVal,
	}
}

func getEnvInt(key string, defaultVal int) int {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	if p, err := strconv.Atoi(v); err == nil && p > 0 {
		return p
	}
	return defaultVal
}

func getEnvBool(key string) bool {
	v := strings.ToLower(os.Getenv(key))
	return v == "true" || v == "1"
}

func loadDotEnv(filenames ...string) {
	for _, filename := range filenames {
		data, err := os.ReadFile(filename)
		if err != nil {
			continue
		}
		lines := strings.SplitSeq(string(data), "\n")
		for line := range lines {
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
