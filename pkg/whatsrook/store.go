package whatsrook

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.mau.fi/util/dbutil"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

// SessionPurgerFunc defines a callback to purge custom bot domain data from the database.
type SessionPurgerFunc func(ctx context.Context, db *dbutil.Database, session string) error

// GlobalSessionPurger is an optional hook executed when purging session records.
var GlobalSessionPurger SessionPurgerFunc

func sanitizeDBURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	if strings.HasPrefix(rawURL, "postgres://") || strings.HasPrefix(rawURL, "postgresql://") {
		parts := strings.SplitN(rawURL, "@", 2)
		if len(parts) == 2 {
			schemeUser := parts[0]
			subParts := strings.SplitN(schemeUser, ":", 3)
			if len(subParts) == 3 {
				return fmt.Sprintf("%s:%s:****@%s", subParts[0], subParts[1], parts[1])
			}
			return fmt.Sprintf("%s:****@%s", parts[0], parts[1])
		}
	}
	return rawURL
}

func ensureSSLDisabled(rawURL string) string {
	if strings.HasSuffix(rawURL, "?sslmode=disable") {
		return rawURL
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		if strings.Contains(rawURL, "?") {
			return rawURL + "&sslmode=disable"
		}
		return rawURL + "?sslmode=disable"
	}
	q := u.Query()
	q.Set("sslmode", "disable")
	u.RawQuery = q.Encode()
	return u.String()
}

// ResolvePostgresURL extracts PostgreSQL connection strings from configuration or environment.
func ResolvePostgresURL(dbConf string, sessionPhone ...string) string {
	dbConf = strings.TrimSpace(dbConf)
	if dbConf != "" && dbConf != "default" && dbConf != "postgres" && dbConf != "postgresql" {
		if strings.HasPrefix(dbConf, "postgres://") || strings.HasPrefix(dbConf, "postgresql://") {
			return dbConf
		}
	}

	if len(sessionPhone) > 0 {
		phone := strings.TrimPrefix(sessionPhone[0], "+")
		phone = strings.ReplaceAll(phone, " ", "")
		phone = strings.ReplaceAll(phone, "-", "")
		if phone != "" {
			if envURL := os.Getenv("DATABASE_URL_" + phone); envURL != "" {
				return strings.TrimSpace(envURL)
			}
		}
	}

	if env := os.Getenv("DATABASE_URL"); env != "" {
		return strings.TrimSpace(env)
	}
	if env := os.Getenv("POSTGRES_URL"); env != "" {
		return strings.TrimSpace(env)
	}
	if env := os.Getenv("DB_URL"); env != "" {
		return strings.TrimSpace(env)
	}

	return ""
}

// OpenStoreContainer opens a sqlstore.Container for Postgres or SQLite.
func OpenStoreContainer(ctx context.Context, dataDir, database string, sessionPhone ...string) (*sqlstore.Container, error) {
	waLogger := defaultLogger

	trimmedDB := strings.TrimSpace(database)
	dbConn := ResolvePostgresURL(trimmedDB, sessionPhone...)

	if strings.HasPrefix(dbConn, "postgres://") || strings.HasPrefix(dbConn, "postgresql://") {
		defaultLogger.Debugf("attempting connection to PostgreSQL database... url=%s", sanitizeDBURL(dbConn))
		container, err := sqlstore.New(ctx, "postgres", dbConn, waLogger)
		if err == nil && container != nil {
			configureConnectionPool(container, dbConn)
			defaultLogger.Debugf("successfully connected to PostgreSQL database")
			defaultLogger.Infof("Database connected successfully")
			return container, nil
		}

		if !strings.HasSuffix(dbConn, "?sslmode=disable") && !strings.Contains(dbConn, "sslmode=disable") {
			disableURL := ensureSSLDisabled(dbConn)
			defaultLogger.Warnf("PostgreSQL SSL connection failed, attempting reconnection with sslmode=disable... err=%v url=%s", err, sanitizeDBURL(disableURL))
			container, errDisable := sqlstore.New(ctx, "postgres", disableURL, waLogger)
			if errDisable == nil && container != nil {
				configureConnectionPool(container, disableURL)
				defaultLogger.Debugf("successfully connected to PostgreSQL database with sslmode=disable")
				defaultLogger.Infof("Database connected successfully")
				return container, nil
			}
			return nil, fmt.Errorf("failed to connect to PostgreSQL (ssl retry also failed: %v): %w", errDisable, err)
		}

		return nil, fmt.Errorf("failed to connect to PostgreSQL database: %w", err)
	}

	targetPath := trimmedDB
	if targetPath == "" || targetPath == "default" {
		if dataDir == "" {
			dataDir = DefaultDataDir()
		}
		targetPath = filepath.Join(dataDir, "whatsrook.db")
	}

	sqliteDSN := targetPath
	if !strings.HasPrefix(sqliteDSN, "file:") {
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return nil, fmt.Errorf("failed to create sqlite storage directory: %w", err)
		}
		sqliteDSN = fmt.Sprintf(
			"file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(30000)&_pragma=cache_size(-64000)&_pragma=temp_store(MEMORY)&_pragma=mmap_size(268435456)",
			filepath.ToSlash(targetPath),
		)
	}

	defaultLogger.Debugf("attempting connection to SQLite database... dsn=%s", sqliteDSN)
	container, err := sqlstore.New(ctx, "sqlite", sqliteDSN, waLogger)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to SQLite database: %w", err)
	}

	if db := container.Database(); db != nil && db.RawDB != nil {
		db.RawDB.SetMaxOpenConns(1)
		db.RawDB.SetMaxIdleConns(1)
		db.RawDB.SetConnMaxLifetime(0)
	}

	defaultLogger.Debugf("successfully initialized SQLite database store")
	defaultLogger.Infof("Database connected successfully")
	return container, nil
}

func configureConnectionPool(container *sqlstore.Container, dbConn string) {
	if container != nil {
		if db := container.Database(); db != nil && db.RawDB != nil {
			maxOpen := 50
			maxIdle := 25

			if strings.Contains(dbConn, "pooler.supabase.com") || strings.Contains(dbConn, "supabase.co") {
				maxOpen = 8
				maxIdle = 4
			}

			if val := os.Getenv("DB_MAX_OPEN_CONNS"); val != "" {
				if parsed, err := strconv.Atoi(val); err == nil && parsed > 0 {
					maxOpen = parsed
				}
			}

			if val := os.Getenv("DB_MAX_IDLE_CONNS"); val != "" {
				if parsed, err := strconv.Atoi(val); err == nil && parsed > 0 {
					maxIdle = parsed
				}
			} else if maxIdle > maxOpen {
				maxIdle = maxOpen
			}

			db.RawDB.SetMaxOpenConns(maxOpen)
			db.RawDB.SetMaxIdleConns(maxIdle)
			db.RawDB.SetConnMaxLifetime(10 * time.Minute)
			db.RawDB.SetConnMaxIdleTime(2 * time.Minute)
			defaultLogger.Debugf("Configured database connection pool max_open=%d max_idle=%d", maxOpen, maxIdle)
		}
	}
}

// StoredSession represents stored session credentials.
type StoredSession struct {
	JID      string
	User     string
	PushName string
	Platform string
	Business bool
}

// ListStoredSessions retrieves all registered session device accounts in the database.
func ListStoredSessions(ctx context.Context, dataDir, database string) ([]StoredSession, error) {
	container, err := OpenStoreContainer(ctx, dataDir, database)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = container.Close()
	}()

	devices, err := container.GetAllDevices(ctx)
	if err != nil {
		return nil, err
	}

	var sessions []StoredSession
	for _, dev := range devices {
		if dev.ID == nil || dev.ID.User == "" {
			continue
		}
		platform := "Chrome"
		if dev.Platform != "" {
			platform = dev.Platform
		}
		sessions = append(sessions, StoredSession{
			JID:      dev.ID.String(),
			User:     dev.ID.User,
			PushName: dev.PushName,
			Platform: platform,
			Business: dev.BusinessName != "",
		})
	}
	return sessions, nil
}

// LogoutFromServer ensures the client is connected and authenticated with WhatsApp,
// then sends a server-side companion device logout request before disconnecting.
func LogoutFromServer(ctx context.Context, cli *whatsmeow.Client) {
	if cli == nil || cli.Store == nil || cli.Store.ID == nil {
		return
	}

	if !cli.IsConnected() || !cli.IsLoggedIn() {
		connected := make(chan struct{}, 1)
		failed := make(chan struct{}, 1)
		handlerID := cli.AddEventHandler(func(evt any) {
			switch evt.(type) {
			case *events.Connected:
				select {
				case connected <- struct{}{}:
				default:
				}
			case *events.LoggedOut, *events.ConnectFailure, *events.StreamReplaced:
				select {
				case failed <- struct{}{}:
				default:
				}
			}
		})
		defer cli.RemoveEventHandler(handlerID)

		if !cli.IsConnected() {
			if err := cli.Connect(); err != nil {
				defaultLogger.Debugf("failed to connect to WhatsApp prior to logout: %v", err)
				return
			}
		}

		connectCtx, connectCancel := context.WithTimeout(ctx, 15*time.Second)
		select {
		case <-connected:
			defaultLogger.Debugf("connected to WhatsApp servers; sending logout request")
		case <-failed:
			defaultLogger.Debugf("session already invalidated on WhatsApp servers")
			connectCancel()
			cli.Disconnect()
			return
		case <-connectCtx.Done():
			defaultLogger.Debugf("timed out waiting for WhatsApp connection before logout")
		}
		connectCancel()
	}

	logoutCtx, logoutCancel := context.WithTimeout(ctx, 15*time.Second)
	if err := cli.Logout(logoutCtx); err != nil {
		defaultLogger.Debugf("server logout request returned error: %v", err)
	}
	logoutCancel()
	cli.Disconnect()
}

// DeleteStoredSession connects to WhatsApp, performs a server-side logout,
// and purges all database records and local directories for the session.
func DeleteStoredSession(ctx context.Context, dataDir, database, phone string) error {
	if dataDir == "" {
		dataDir = DefaultDataDir()
	}

	client := NewClient(Config{
		DataDir:    dataDir,
		Database:   database,
		Session:    phone,
		ClientType: ClientChrome,
	})

	initCtx, initCancel := context.WithTimeout(ctx, 10*time.Second)
	errInit := client.InitSession(initCtx)
	initCancel()
	if errInit != nil {
		_ = client.Close()
		err := FallbackDeleteDevice(ctx, dataDir, database, phone)
		PurgeSessionFiles(dataDir, database, false, phone)
		return err
	}

	cli := client.WAClient()
	ids := collectDeviceIdentifiers(phone, cli)

	if cli != nil && cli.Store != nil && cli.Store.ID != nil {
		LogoutFromServer(ctx, cli)

		if GlobalSessionPurger != nil && client.container != nil {
			for _, id := range ids {
				_ = GlobalSessionPurger(ctx, client.container.GetDB(), id)
			}
		}

		_ = cli.Store.Delete(ctx)
		remaining := deleteMatchingDevicesInContainer(ctx, client.container, ids...)
		_ = client.Close()
		PurgeSessionFiles(dataDir, database, remaining == 0, ids...)
		return nil
	}

	_ = client.Close()
	err := FallbackDeleteDevice(ctx, dataDir, database, phone)
	PurgeSessionFiles(dataDir, database, false, ids...)
	return err
}

func collectDeviceIdentifiers(phone string, cli *whatsmeow.Client) []string {
	var raw []string
	if strings.TrimSpace(phone) != "" {
		raw = append(raw, phone, strings.TrimPrefix(strings.TrimSpace(phone), "+"))
	}
	if cli != nil && cli.Store != nil {
		if cli.Store.ID != nil && cli.Store.ID.User != "" {
			raw = append(raw, cli.Store.ID.User, cli.Store.ID.ToNonAD().String(), cli.Store.ID.String())
		}
		if !cli.Store.LID.IsEmpty() && cli.Store.LID.User != "" {
			raw = append(raw, cli.Store.LID.User, cli.Store.LID.ToNonAD().String(), cli.Store.LID.String())
		}
	}
	seen := make(map[string]bool, len(raw))
	var out []string
	for _, item := range raw {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" && !seen[trimmed] {
			seen[trimmed] = true
			out = append(out, trimmed)
		}
	}
	return out
}

func deleteMatchingDevicesInContainer(ctx context.Context, container *sqlstore.Container, identifiers ...string) int {
	if container == nil {
		return -1
	}
	users := extractCleanUsers(identifiers...)
	for _, u := range users {
		jid := types.NewJID(u, types.DefaultUserServer)
		if dev, err := container.GetDevice(ctx, jid); err == nil && dev != nil {
			_ = dev.Delete(ctx)
		}
	}

	devices, err := container.GetAllDevices(ctx)
	if err != nil {
		return -1
	}
	remaining := 0
	for _, d := range devices {
		if d.ID == nil {
			continue
		}
		matched := false
		for _, u := range users {
			if u != "" && (d.ID.User == u || d.LID.User == u || strings.Contains(d.ID.String(), u)) {
				matched = true
				break
			}
		}
		if matched {
			_ = d.Delete(ctx)
		} else {
			remaining++
		}
	}
	return remaining
}

// FallbackDeleteDevice deletes a device record by phone number directly from the container.
func FallbackDeleteDevice(ctx context.Context, dataDir, database, phone string) error {
	container, err := OpenStoreContainer(ctx, dataDir, database)
	if err != nil {
		PurgeSessionFiles(dataDir, database, false, phone)
		return fmt.Errorf("failed to open database container: %w", err)
	}

	var extraIDs []string
	if phone != "" {
		extraIDs = append(extraIDs, phone)
	}
	cleanPhone := strings.TrimPrefix(strings.TrimSpace(phone), "+")
	if devices, errScan := container.GetAllDevices(ctx); errScan == nil {
		for _, d := range devices {
			if d.ID != nil && cleanPhone != "" && (d.ID.User == cleanPhone || strings.Contains(d.ID.String(), cleanPhone)) {
				extraIDs = append(extraIDs, d.ID.User, d.ID.ToNonAD().String(), d.ID.String())
				if !d.LID.IsEmpty() && d.LID.User != "" {
					extraIDs = append(extraIDs, d.LID.User, d.LID.ToNonAD().String(), d.LID.String())
				}
			}
		}
	}

	if GlobalSessionPurger != nil {
		for _, id := range extraIDs {
			_ = GlobalSessionPurger(ctx, container.GetDB(), id)
		}
	}

	remaining := deleteMatchingDevicesInContainer(ctx, container, extraIDs...)
	_ = container.Close()
	PurgeSessionFiles(dataDir, database, remaining == 0, extraIDs...)
	return nil
}

// ClearSessionDB clears device, custom database records, and session folders.
func (c *Client) ClearSessionDB(ctx context.Context, phone string) {
	c.mu.Lock()
	raw := c.rawClient
	container := c.container
	c.mu.Unlock()

	ids := collectDeviceIdentifiers(phone, raw)
	if len(ids) == 0 && c.Config.Session != "" {
		ids = collectDeviceIdentifiers(c.Config.Session, raw)
	}

	if GlobalSessionPurger != nil {
		if container != nil && container.GetDB() != nil {
			for _, id := range ids {
				_ = GlobalSessionPurger(ctx, container.GetDB(), id)
			}
		} else if raw != nil && raw.Store != nil {
			if s, ok := raw.Store.Identities.(*sqlstore.SQLStore); ok && s != nil {
				for _, id := range ids {
					_ = GlobalSessionPurger(ctx, s.GetDB(), id)
				}
			}
		}
	}

	if raw != nil && raw.Store != nil {
		_ = raw.Store.Delete(ctx)
	}
	remaining := -1
	if container != nil {
		remaining = deleteMatchingDevicesInContainer(ctx, container, ids...)
	} else if len(ids) > 0 {
		_ = FallbackDeleteDevice(ctx, c.Config.DataDir, c.Config.Database, ids[0])
	}
	PurgeSessionFiles(c.Config.DataDir, c.Config.Database, remaining == 0, ids...)
}

func extractCleanUsers(identifiers ...string) []string {
	seen := make(map[string]bool)
	var users []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s != "" && !seen[s] && s != "." && s != "/" && s != "\\" {
			seen[s] = true
			users = append(users, s)
		}
	}
	for _, raw := range identifiers {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		clean := strings.TrimPrefix(trimmed, "+")
		user := clean
		if idx := strings.IndexAny(user, ":@."); idx != -1 {
			user = user[:idx]
		}
		add(user)
		if user != "" {
			add("+" + user)
		}
		if !strings.ContainsAny(clean, ":@/\\") {
			add(clean)
		}
	}
	return users
}

// PurgeSessionFiles removes all session-scoped folders and files inside .whatsrook
// (as well as any custom data, database, or plugin directory) for the given session identifiers.
func PurgeSessionFiles(dataDir, database string, noRemainingSessions bool, identifiers ...string) {
	users := extractCleanUsers(identifiers...)

	var baseDirs []string
	seenDirs := make(map[string]bool)
	addDir := func(d string) {
		d = strings.TrimSpace(d)
		if d == "" {
			return
		}
		clean := filepath.Clean(d)
		if clean != "." && clean != "/" && clean != "\\" && !seenDirs[clean] {
			seenDirs[clean] = true
			baseDirs = append(baseDirs, clean)
		}
	}

	addDir(dataDir)
	addDir(DefaultDataDir())
	addDir(os.Getenv("WHATSROOK_DATA_DIR"))
	addDir(os.Getenv("WHATSDATA_DIR"))
	addDir(os.Getenv("WHATSROOK_PLUGIN_DIR"))
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		addDir(filepath.Join(home, ".whatsrook"))
	}
	trimmedDB := strings.TrimSpace(database)
	if trimmedDB != "" && !strings.HasPrefix(trimmedDB, "postgres://") && !strings.HasPrefix(trimmedDB, "postgresql://") {
		dbPath := strings.TrimPrefix(trimmedDB, "file:")
		if idx := strings.IndexByte(dbPath, '?'); idx != -1 {
			dbPath = dbPath[:idx]
		}
		if dir := filepath.Dir(dbPath); dir != "" && dir != "." {
			addDir(dir)
		}
	}

	for _, base := range baseDirs {
		sessionsDir := filepath.Join(base, "sessions")
		for _, u := range users {
			_ = os.RemoveAll(filepath.Join(sessionsDir, u))
		}
		if noRemainingSessions {
			_ = os.RemoveAll(filepath.Join(sessionsDir, "default"))
			if entries, err := os.ReadDir(sessionsDir); err == nil && len(entries) == 0 {
				_ = os.Remove(sessionsDir)
			}
		}

		shellDir := filepath.Join(base, "shell_sessions")
		if noRemainingSessions {
			_ = os.RemoveAll(shellDir)
		} else if entries, err := os.ReadDir(shellDir); err == nil {
			for _, entry := range entries {
				name := entry.Name()
				for _, u := range users {
					if u != "" && strings.Contains(name, u) {
						_ = os.RemoveAll(filepath.Join(shellDir, name))
						break
					}
				}
			}
			if rem, errRem := os.ReadDir(shellDir); errRem == nil && len(rem) == 0 {
				_ = os.Remove(shellDir)
			}
		}
	}

	for _, u := range users {
		_ = os.RemoveAll(filepath.Join("auth", u))
	}
	if noRemainingSessions {
		_ = os.RemoveAll(filepath.Join("auth", "default"))
		if entries, err := os.ReadDir("auth"); err == nil && len(entries) == 0 {
			_ = os.Remove("auth")
		}
	}
}
