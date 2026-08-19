package main

import (
	"flag"
	"fmt"
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
	fs := flag.NewFlagSet("whatsrook", flag.ExitOnError)

	defaultPort := getEnvInt("PORT", getEnvInt("WS_PORT", 3000))
	var (
		session  = fs.String("s", os.Getenv("SESSION"), "")
		pair     = fs.Bool("p", false, "")
		client   = fs.String("c", "chrome", "")
		database = fs.String("db", "", "")
		port     = fs.Int("P", defaultPort, "")
		qr       = fs.Bool("q", false, "")
		logout   = fs.Bool("l", false, "")
		update   = fs.Bool("u", false, "")
		verbose  = fs.Bool("v", false, "")
		noSkip   = fs.Bool("no-skip-old", false, "")
	)

	fs.StringVar(session, "session", *session, "")
	fs.BoolVar(pair, "pair", *pair, "")
	fs.StringVar(client, "client", *client, "")
	fs.StringVar(database, "database", *database, "")
	fs.IntVar(port, "port", *port, "")
	fs.BoolVar(qr, "qrcode", *qr, "")
	fs.BoolVar(logout, "logout", *logout, "")
	fs.BoolVar(update, "update", *update, "")
	fs.BoolVar(verbose, "verbose", *verbose, "")

	fs.Usage = func() {
		fmt.Print(`Usage: whatsrook [-session <phone_number>] [OPTIONS]
       whatsrook update [check | upgrade]
       whatsrook --update [stable | beta]

Options:
  -s, --session <phone>  Phone number used to identify the session (runs in idle mode if omitted)
  -p, --pair             Request a pair code using the --session phone number
  -P, --port <port>      WebSocket/HTTP server port (default: 3000 or $PORT)
  -c, --client <type>    Client type: chrome (default), android, ios
  -db, --database <url>  Database connection: sqlite (default) or postgres URL
  -q, --qrcode           Print the QR code to stdout for scanning
  -l, --logout           Remove the session auth files and exit
  -u, --update [channel] Check and perform update; optionally pass "stable" or "beta" to
                         switch channels (prints a notice if already on the requested channel)
  -v, --verbose          Enable verbose logging
  --no-skip-old          Process messages sent while the bot was offline (default: skip them)
  -h, --help             Show this help message
`)
	}

	_ = fs.Parse(os.Args[1:])

	sessionVal := *session
	var updateChannel string
	if fs.NArg() > 0 {
		for _, arg := range fs.Args() {
			lower := strings.ToLower(strings.TrimSpace(arg))
			// Capture an explicit channel switch alongside -u/--update.
			if *update && (lower == "stable" || lower == "beta") {
				updateChannel = lower
				continue
			}
			// Existing phone-number positional arg detection.
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

	clientVal := *client
	if clientVal == "chrome" && os.Getenv("CLIENT") != "" {
		clientVal = os.Getenv("CLIENT")
	}

	dbVal := *database
	if dbVal == "" {
		if envDB := os.Getenv("DATABASE_URL"); envDB != "" {
			dbVal = envDB
		} else if envPG := os.Getenv("POSTGRES_URL"); envPG != "" {
			dbVal = envPG
		} else if envDBURL := os.Getenv("DB_URL"); envDBURL != "" {
			dbVal = envDBURL
		} else {
			dbVal = "sqlite"
		}
	}

	return CLIArgs{
		Session:         sessionVal,
		Pair:            *pair || getEnvBool("PAIR"),
		QRCode:          *qr || getEnvBool("QRCODE"),
		Logout:          *logout || getEnvBool("LOGOUT"),
		Update:          *update || getEnvBool("UPDATE"),
		UpdateChannel:   updateChannel,
		Verbose:         *verbose || getEnvBool("VERBOSE"),
		Client:          clientVal,
		Database:        dbVal,
		SkipOldMessages: !*noSkip,
		Port:            *port,
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
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
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
