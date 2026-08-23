package whatsrook

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCompanionReg"
	"go.mau.fi/whatsmeow/proto/waWa6"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"whatsrook/utils"
	"whatsrook/utils/cache"

	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

// ClientType represents the platform emulated by the WhatsApp client.
type ClientType int

var (
	ErrLoggedOut = errors.New("logged out from WhatsApp")
)

const (
	ClientChrome  ClientType = iota
	ClientAndroid ClientType = iota
	ClientIos     ClientType = iota
)

// ParseClientType converts a platform name string to its ClientType enum.
func ParseClientType(s string) (ClientType, bool) {
	c, ok := map[string]ClientType{
		"chrome":  ClientChrome,
		"android": ClientAndroid,
		"ios":     ClientIos,
	}[strings.ToLower(s)]
	return c, ok
}

// Config holds configuration parameters for a Client instance.
type Config struct {
	Session         string
	DataDir         string // Directory for logs and the shared SQLite database (default: next to the binary)
	Database        string // Database connection URL or "sqlite" (default: "sqlite"). Can also be set via DATABASE_URL_<phone> env var for per-session override.
	ClientType      ClientType
	Verbose         bool
	SkipOldMessages bool
	AsyncMessageAck bool // If true, SendMessage will return immediately after writing to the socket and process server ACKs in the background.
}

// Abstraction over the whatsmeow WhatsApp client and store container.
type Client struct {
	Config Config

	rawClient *whatsmeow.Client
	container *sqlstore.Container
	mu        sync.Mutex
}

// NewClient creates and initializes a new WhatsRook core Client instance.
func NewClient(cfg Config) *Client {
	c := &Client{Config: cfg}
	c.applyDefaults()
	return c
}

// DefaultDataDir returns the directory next to the running binary (or cwd when
// running via `go run`/tests).
func DefaultDataDir() string {
	if exePath, err := os.Executable(); err == nil {
		if !strings.Contains(exePath, "go-build") && !strings.Contains(exePath, "/tmp/") && !strings.Contains(exePath, `\Temp\`) {
			return filepath.Dir(exePath)
		}
	}
	return "."
}

// DefaultAuthDir is kept for backward-compatibility with any external callers;
// it now delegates to DefaultDataDir.
func DefaultAuthDir() string { return DefaultDataDir() }

func (c *Client) applyDefaults() {
	if c.Config.DataDir == "" {
		c.Config.DataDir = DefaultDataDir()
	}
}

// WAClient returns the underlying whatsmeow.Client instance.
func (c *Client) WAClient() *whatsmeow.Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rawClient
}

// Container returns the underlying sqlstore.Container instance.
func (c *Client) Container() *sqlstore.Container {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.container
}

// InitSession initializes the logger, shared database store container, and whatsmeow client.
// All sessions share a single database file (SQLite) or a single Postgres database;
// no per-session directory is created.
func (c *Client) InitSession(ctx context.Context) error {
	c.applyDefaults()

	if c.Config.Session == "" {
		return errors.New("session phone number is required")
	}

	// Logs go into DataDir/logs/ — shared across all sessions.
	if err := utils.InitLogger(c.Config.DataDir, c.Config.Verbose); err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}

	waLevel := "INFO"
	if c.Config.Verbose {
		waLevel = "DEBUG"
	}

	// Shared database path for SQLite: DataDir/whatsrook.db
	dbPath := filepath.Join(c.Config.DataDir, "whatsrook.db")

	container, err := c.initStore(ctx, dbPath, waLevel)
	if err != nil {
		return fmt.Errorf("failed to open db: %w", err)
	}

	// Retrieve existing device for this session phone number, or create a new one.
	deviceStore, err := c.getOrCreateDevice(ctx, container)
	if err != nil {
		_ = container.Close()
		return fmt.Errorf("failed to get device: %w", err)
	}
	deviceStore.ExternalCache = cache.Default()

	clientLog := utils.WhatsmeowStyle("Client", "INFO", true)
	rawClient := whatsmeow.NewClient(deviceStore, clientLog)
	rawClient.AsyncMessageAck = c.Config.AsyncMessageAck

	switch c.Config.ClientType {
	case ClientAndroid:
		store.DeviceProps.PlatformType = waCompanionReg.DeviceProps_ANDROID_PHONE.Enum()
		store.DeviceProps.Os = new("16")
		store.BaseClientPayload.UserAgent.Platform = waWa6.ClientPayload_UserAgent_ANDROID.Enum()
		store.BaseClientPayload.UserAgent.OsVersion = new("16.0.0")
		store.BaseClientPayload.UserAgent.OsBuildNumber = new("16.0.0")
		store.BaseClientPayload.WebInfo = nil
	case ClientIos:
		store.DeviceProps.PlatformType = waCompanionReg.DeviceProps_IOS_PHONE.Enum()
		store.DeviceProps.Os = new("18.0")
		store.BaseClientPayload.UserAgent.Platform = waWa6.ClientPayload_UserAgent_IOS.Enum()
		store.BaseClientPayload.UserAgent.OsVersion = new("18.0")
		store.BaseClientPayload.UserAgent.OsBuildNumber = new("18.0")
		store.BaseClientPayload.WebInfo = nil
	default:
		store.DeviceProps.PlatformType = waCompanionReg.DeviceProps_CHROME.Enum()
		store.DeviceProps.Os = new("Linux")
		store.BaseClientPayload.UserAgent.Platform = waWa6.ClientPayload_UserAgent_WEB.Enum()
	}

	c.mu.Lock()
	c.container = container
	c.rawClient = rawClient
	c.mu.Unlock()

	return nil
}

// getOrCreateDevice finds an existing device matching the session phone number
// in the shared container, or returns a freshly created (unpaired) device.
func (c *Client) getOrCreateDevice(ctx context.Context, container *sqlstore.Container) (*store.Device, error) {
	devices, err := container.GetAllDevices(ctx)
	if err != nil {
		return nil, err
	}

	phone := c.Config.Session
	// Strip leading '+' so we can do a prefix match on JID users like "447911123456.0".
	phone = strings.TrimPrefix(phone, "+")

	for _, dev := range devices {
		if dev.ID != nil && strings.HasPrefix(dev.ID.User, phone) {
			return dev, nil
		}
	}

	// No matching device — return a new unpaired device.
	return container.NewDevice(), nil
}

// Close closes the underlying database store container.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.container != nil {
		err := c.container.Close()
		c.container = nil
		return err
	}
	return nil
}

// AddEventHandler registers an event handler function on the whatsmeow client.
func (c *Client) AddEventHandler(handler func(evt any)) {
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()
	if cli != nil {
		cli.AddEventHandler(handler)
	}
}

// Connect connects the WhatsApp client.
func (c *Client) Connect() error {
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()
	if cli == nil {
		return errors.New("client not initialized")
	}
	return cli.Connect()
}

// Disconnect disconnects the WhatsApp client.
func (c *Client) Disconnect() {
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()
	if cli != nil {
		cli.Disconnect()
	}
}

// Logout logs out the WhatsApp client session from WhatsApp servers.
func (c *Client) Logout(ctx context.Context) error {
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()
	if cli == nil {
		return errors.New("client not initialized")
	}
	return cli.Logout(ctx)
}

// WipeSession is a no-op kept for backward-compatibility.
// Directory-based session wiping is no longer used; use Client.ClearSessionDB
// to remove only the affected device record from the shared database.
func WipeSession(_ string) {}

// ClearSessionDB deletes the current session's device record from the shared
// database. Other sessions stored in the same database are not affected.
func (c *Client) ClearSessionDB(ctx context.Context, _ string) {
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()

	if cli != nil && cli.Store != nil {
		if err := cli.Store.Delete(ctx); err != nil {
			slog.Warn("failed to delete session device store", "err", err)
		}
	}

	_ = c.Close()
}

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

func (c *Client) initStore(ctx context.Context, dbPath, waLevel string) (*sqlstore.Container, error) {
	dbLog := utils.WhatsmeowStyle("Database", waLevel, true)

	dbConn := c.Config.Database
	if dbConn == "" {
		// Per-session override: DATABASE_URL_<phone> takes priority over generic env vars.
		phone := strings.TrimPrefix(c.Config.Session, "+")
		if phone != "" {
			dbConn = os.Getenv("DATABASE_URL_" + phone)
		}
	}
	if dbConn == "" {
		dbConn = os.Getenv("DATABASE_URL")
	}
	if dbConn == "" {
		dbConn = os.Getenv("POSTGRES_URL")
	}
	if dbConn == "" {
		dbConn = os.Getenv("DB_URL")
	}
	if dbConn == "" {
		dbConn = "sqlite"
	}

	if dbConn != "sqlite" && dbConn != "none" && (strings.HasPrefix(dbConn, "postgres://") || strings.HasPrefix(dbConn, "postgresql://")) {
		slog.Info("attempting connection to PostgreSQL database...", "url", sanitizeDBURL(dbConn))
		container, err := sqlstore.New(ctx, "postgres", dbConn, dbLog)
		if err == nil && container != nil {
			slog.Info("successfully connected to PostgreSQL database")
			return container, nil
		}

		if !strings.HasSuffix(dbConn, "?sslmode=disable") {
			disableURL := ensureSSLDisabled(dbConn)
			slog.Warn("PostgreSQL SSL connection failed, attempting reconnection with sslmode=disable...", "err", err, "url", sanitizeDBURL(disableURL))
			container, errDisable := sqlstore.New(ctx, "postgres", disableURL, dbLog)
			if errDisable == nil && container != nil {
				slog.Info("successfully connected to PostgreSQL database with sslmode=disable")
				return container, nil
			}
			slog.Warn("PostgreSQL connection failed with sslmode=disable", "err", errDisable)
		} else {
			slog.Warn("PostgreSQL connection failed", "err", err)
		}
		slog.Warn("falling back to SQLite after PostgreSQL connection failure")
	}

	slog.Info("initializing SQLite database store", "path", dbPath)
	sqliteURI := fmt.Sprintf(
		"file:%s?_pragma=busy_timeout=5000&_pragma=journal_mode=WAL&_pragma=synchronous=NORMAL&_pragma=foreign_keys=on&_pragma=cache_size=-2000",
		dbPath,
	)
	container, err := sqlstore.New(ctx, "sqlite", sqliteURI, dbLog)
	if err != nil {
		return nil, fmt.Errorf("failed to open SQLite database: %w", err)
	}
	return container, nil
}

// GetJoinedGroups returns all WhatsApp groups the user is participating in.
func (c *Client) GetJoinedGroups(ctx context.Context) ([]*types.GroupInfo, error) {
	wa := c.WAClient()
	if wa == nil {
		return nil, errors.New("client not initialized")
	}
	return wa.GetJoinedGroups(ctx)
}

// GetSubscribedNewsletters returns all newsletters/channels the user is subscribed to.
func (c *Client) GetSubscribedNewsletters(ctx context.Context) ([]*types.NewsletterMetadata, error) {
	wa := c.WAClient()
	if wa == nil {
		return nil, errors.New("client not initialized")
	}
	return wa.GetSubscribedNewsletters(ctx)
}

// GetGroupInfo retrieves the full group metadata and participants for a given group JID.
func (c *Client) GetGroupInfo(ctx context.Context, jid types.JID) (*types.GroupInfo, error) {
	wa := c.WAClient()
	if wa == nil {
		return nil, errors.New("client not initialized")
	}
	return wa.GetGroupInfo(ctx, jid)
}

// GetNewsletterInfo retrieves the metadata for a given newsletter/channel JID.
func (c *Client) GetNewsletterInfo(ctx context.Context, jid types.JID) (*types.NewsletterMetadata, error) {
	wa := c.WAClient()
	if wa == nil {
		return nil, errors.New("client not initialized")
	}
	return wa.GetNewsletterInfo(ctx, jid)
}
