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
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"

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

// DeleteStoredSession logs out and purges an account's device records from the store.
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
	defer func() {
		_ = client.Close()
	}()

	initCtx, initCancel := context.WithTimeout(ctx, 10*time.Second)
	defer initCancel()

	if err := client.InitSession(initCtx); err != nil {
		return FallbackDeleteDevice(ctx, dataDir, database, phone)
	}

	cli := client.WAClient()
	if cli != nil && cli.Store != nil && cli.Store.ID != nil {
		if GlobalSessionPurger != nil && client.container != nil && phone != "" {
			_ = GlobalSessionPurger(ctx, client.container.GetDB(), phone)
		}

		_ = cli.Store.Delete(ctx)
		_ = FallbackDeleteDevice(ctx, dataDir, database, phone)
		return nil
	}

	return FallbackDeleteDevice(ctx, dataDir, database, phone)
}

// FallbackDeleteDevice deletes a device record by phone number directly from the container.
func FallbackDeleteDevice(ctx context.Context, dataDir, database, phone string) error {
	container, err := OpenStoreContainer(ctx, dataDir, database)
	if err != nil {
		return fmt.Errorf("failed to open database container: %w", err)
	}
	defer func() {
		_ = container.Close()
	}()

	if GlobalSessionPurger != nil && phone != "" {
		_ = GlobalSessionPurger(ctx, container.GetDB(), phone)
	}

	cleanPhone := strings.TrimPrefix(phone, "+")
	jid := types.NewJID(cleanPhone, types.DefaultUserServer)

	dev, err := container.GetDevice(ctx, jid)
	if err != nil {
		return fmt.Errorf("failed to lookup device in store: %w", err)
	}
	if dev != nil {
		if err := dev.Delete(ctx); err != nil {
			return fmt.Errorf("failed to delete device record: %w", err)
		}
		return nil
	}

	devices, err := container.GetAllDevices(ctx)
	if err != nil {
		return fmt.Errorf("failed to scan devices: %w", err)
	}
	for _, d := range devices {
		if d.ID != nil && (d.ID.User == cleanPhone || strings.Contains(d.ID.String(), cleanPhone)) {
			_ = d.Delete(ctx)
			return nil
		}
	}

	return nil
}

// ClearSessionDB clears device and custom database records.
func (c *Client) ClearSessionDB(ctx context.Context, phone string) {
	c.mu.Lock()
	raw := c.rawClient
	container := c.container
	c.mu.Unlock()

	if phone == "" && raw != nil && raw.Store != nil && raw.Store.ID != nil {
		phone = raw.Store.ID.User
	}
	if phone == "" {
		phone = c.Config.Session
	}

	if GlobalSessionPurger != nil && phone != "" {
		if container != nil && container.GetDB() != nil {
			_ = GlobalSessionPurger(ctx, container.GetDB(), phone)
		} else if raw != nil && raw.Store != nil {
			if s, ok := raw.Store.Identities.(*sqlstore.SQLStore); ok && s != nil {
				_ = GlobalSessionPurger(ctx, s.GetDB(), phone)
			}
		}
	}

	if raw != nil && raw.Store != nil {
		_ = raw.Store.Delete(ctx)
	}
	if phone != "" {
		_ = FallbackDeleteDevice(ctx, c.Config.DataDir, c.Config.Database, phone)
	}
}
