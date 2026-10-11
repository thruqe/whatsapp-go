package whatsrook

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCompanionReg"
	"go.mau.fi/whatsmeow/proto/waWa6"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"
)

// ClientType defines the companion platform personality.
type ClientType int

const (
	ClientChrome ClientType = iota
	ClientAndroid
	ClientIos
)

var (
	ErrLoggedOut       = errors.New("logged out from WhatsApp")
	ErrPairingTimedOut = errors.New("pairing timed out")
)

// ParseClientType parses a string into a ClientType.
func ParseClientType(s string) (ClientType, bool) {
	c, ok := map[string]ClientType{
		"chrome":      ClientChrome,
		"android":     ClientAndroid,
		"ios":         ClientIos,
		"smb_android": ClientAndroid,
		"smba":        ClientAndroid,
		"smb_ios":     ClientIos,
		"smbi":        ClientIos,
	}[strings.ToLower(s)]
	return c, ok
}

// Config holds client initialization parameters.
type Config struct {
	DataDir         string
	Database        string
	Session         string
	ClientType      ClientType
	Business        bool
	Verbose         bool
	AsyncMessageAck bool
	Logger          waLog.Logger
	ExternalCache   store.ByteCache
}

// Client wraps whatsmeow.Client with lightweight session management.
type Client struct {
	Config Config

	mu        sync.Mutex
	rawClient *whatsmeow.Client
	container *sqlstore.Container
}

// NewClient creates a new Client instance.
func NewClient(cfg Config) *Client {
	if cfg.DataDir == "" {
		cfg.DataDir = DefaultDataDir()
	}
	if cfg.Logger == nil {
		cfg.Logger = defaultLogger
	}
	return &Client{
		Config: cfg,
	}
}

// WAClient returns the underlying whatsmeow.Client.
func (c *Client) WAClient() *whatsmeow.Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rawClient
}

// Container returns the underlying sqlstore.Container.
func (c *Client) Container() *sqlstore.Container {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.container
}

// InitSession initializes the database container and WhatsApp client.
func (c *Client) InitSession(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	container, err := OpenStoreContainer(ctx, c.Config.DataDir, c.Config.Database, c.Config.Session)
	if err != nil {
		return fmt.Errorf("failed to initialize store: %w", err)
	}
	c.container = container

	deviceStore, err := c.resolveDeviceStore(ctx, container)
	if err != nil {
		return fmt.Errorf("failed to get device store: %w", err)
	}
	if c.Config.ExternalCache != nil {
		deviceStore.ExternalCache = c.Config.ExternalCache
	}

	waLogger := c.Config.Logger
	if waLogger == nil {
		waLogger = defaultLogger
	}
	cli := whatsmeow.NewClient(deviceStore, waLogger)
	cli.AsyncMessageAck = c.Config.AsyncMessageAck

	isBusiness := c.Config.Business || (deviceStore != nil && (deviceStore.BusinessName != "" || strings.HasPrefix(strings.ToLower(deviceStore.Platform), "smb")))
	ConfigureCompanionPlatform(c.Config.ClientType, isBusiness)

	c.rawClient = cli
	return nil
}

// ConfigureCompanionPlatform sets the companion platform registration payload.
func ConfigureCompanionPlatform(clientType ClientType, isBusiness bool) {
	switch clientType {
	case ClientAndroid:
		store.DeviceProps.PlatformType = waCompanionReg.DeviceProps_ANDROID_PHONE.Enum()
		store.DeviceProps.Os = new("16")
		if isBusiness {
			store.BaseClientPayload.UserAgent.Platform = waWa6.ClientPayload_UserAgent_SMB_ANDROID.Enum()
		} else {
			store.BaseClientPayload.UserAgent.Platform = waWa6.ClientPayload_UserAgent_ANDROID.Enum()
		}
		store.BaseClientPayload.UserAgent.OsVersion = new("16.0.0")
		store.BaseClientPayload.UserAgent.OsBuildNumber = new("16.0.0")
		store.BaseClientPayload.WebInfo = nil
	case ClientIos:
		store.DeviceProps.PlatformType = waCompanionReg.DeviceProps_IOS_PHONE.Enum()
		store.DeviceProps.Os = new("18.0")
		if isBusiness {
			store.BaseClientPayload.UserAgent.Platform = waWa6.ClientPayload_UserAgent_SMB_IOS.Enum()
		} else {
			store.BaseClientPayload.UserAgent.Platform = waWa6.ClientPayload_UserAgent_IOS.Enum()
		}
		store.BaseClientPayload.UserAgent.OsVersion = new("18.0")
		store.BaseClientPayload.UserAgent.OsBuildNumber = new("18.0")
		store.BaseClientPayload.WebInfo = nil
	default:
		store.DeviceProps.PlatformType = waCompanionReg.DeviceProps_CHROME.Enum()
		osName := "Linux"
		switch runtime.GOOS {
		case "windows":
			osName = "Windows"
		case "darwin":
			osName = "macOS"
		}
		store.DeviceProps.Os = &osName
		store.BaseClientPayload.UserAgent.Platform = waWa6.ClientPayload_UserAgent_WEB.Enum()
		store.BaseClientPayload.WebInfo = &waWa6.ClientPayload_WebInfo{
			WebSubPlatform: waWa6.ClientPayload_WebInfo_WEB_BROWSER.Enum(),
		}
	}
}

func (c *Client) resolveDeviceStore(ctx context.Context, container *sqlstore.Container) (*store.Device, error) {
	cleanPhone := strings.TrimPrefix(c.Config.Session, "+")
	cleanPhone = strings.ReplaceAll(cleanPhone, " ", "")
	cleanPhone = strings.ReplaceAll(cleanPhone, "-", "")

	if cleanPhone != "" {
		jid := types.NewJID(cleanPhone, types.DefaultUserServer)
		dev, err := container.GetDevice(ctx, jid)
		if err == nil && dev != nil {
			return dev, nil
		}

		devices, err := container.GetAllDevices(ctx)
		if err == nil {
			for _, d := range devices {
				if d.ID != nil && (d.ID.User == cleanPhone || strings.Contains(d.ID.String(), cleanPhone)) {
					return d, nil
				}
			}
		}

		return container.NewDevice(), nil
	}

	device, err := container.GetFirstDevice(ctx)
	if err == nil && device != nil {
		return device, nil
	}

	return container.NewDevice(), nil
}

// Connect connects to the WhatsApp servers.
func (c *Client) Connect() error {
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()

	if cli == nil {
		return fmt.Errorf("client not initialized: call InitSession first")
	}

	return cli.Connect()
}

// Disconnect disconnects from WhatsApp servers and flushes sessions.
func (c *Client) Disconnect() {
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()

	if cli != nil {
		cli.Disconnect()
		if cli.Store != nil {
			_ = cli.Store.FlushSessions(context.Background())
		}
	}
}

// IsConnected returns whether the client is currently connected.
func (c *Client) IsConnected() bool {
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()

	if cli == nil {
		return false
	}
	return cli.IsConnected()
}

// IsLoggedIn returns whether the client has an active authenticated session.
func (c *Client) IsLoggedIn() bool {
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()

	if cli == nil {
		return false
	}
	return cli.IsLoggedIn()
}

// WaitForConnection blocks until connected or timeout expires.
func (c *Client) WaitForConnection(ctx context.Context, timeout time.Duration) bool {
	if c.IsConnected() {
		return true
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return false
		case <-time.After(100 * time.Millisecond):
			if c.IsConnected() {
				return true
			}
		}
	}
	return c.IsConnected()
}

// SetPresence sends presence state.
func (c *Client) SetPresence(state types.Presence) error {
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()

	if cli == nil {
		return fmt.Errorf("client not initialized")
	}

	return cli.SendPresence(context.Background(), state)
}

// SetOnline marks presence as available.
func (c *Client) SetOnline() error {
	return c.SetPresence(types.PresenceAvailable)
}

// Close closes the client and release database container resources.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.rawClient != nil {
		c.rawClient.Disconnect()
		c.rawClient = nil
	}

	if c.container != nil {
		err := c.container.Close()
		c.container = nil
		return err
	}

	return nil
}

// PairPhone initiates pairing using a phone number code.
func (c *Client) PairPhone(ctx context.Context, phone string) (string, error) {
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()

	if cli == nil {
		return "", fmt.Errorf("client not initialized: call InitSession first")
	}

	cleanPhone := strings.TrimPrefix(phone, "+")
	cleanPhone = strings.ReplaceAll(cleanPhone, " ", "")
	cleanPhone = strings.ReplaceAll(cleanPhone, "-", "")

	if cleanPhone == "" {
		return "", fmt.Errorf("invalid phone number: cannot be empty")
	}

	defaultLogger.Debugf("initiating companion device phone pairing... phone=%s", cleanPhone)
	defaultLogger.Infof("Requesting pairing code from WhatsApp...")

	if !cli.IsConnected() {
		if err := cli.Connect(); err != nil {
			return "", fmt.Errorf("failed to connect for pairing: %w", err)
		}
	}

	code, err := cli.PairPhone(ctx, cleanPhone, true, whatsmeow.PairClientChrome, "Chrome (Linux)")
	if err != nil {
		return "", fmt.Errorf("phone pairing request failed: %w", err)
	}

	defaultLogger.Debugf("pairing code generated successfully phone=%s code=%s", cleanPhone, code)
	defaultLogger.Infof("Pairing code generated successfully")
	return code, nil
}

// PairQR returns the QR code channel for authentication.
func (c *Client) PairQR(ctx context.Context) (<-chan whatsmeow.QRChannelItem, error) {
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()

	if cli == nil {
		return nil, fmt.Errorf("client not initialized: call InitSession first")
	}

	qrChan, err := cli.GetQRChannel(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get QR channel: %w", err)
	}

	if !cli.IsConnected() {
		if err := cli.Connect(); err != nil {
			return nil, fmt.Errorf("failed to connect for QR pairing: %w", err)
		}
	}

	return qrChan, nil
}

// DefaultDataDir returns the default local data directory for whatsrook.
func DefaultDataDir() string {
	if dir := os.Getenv("WHATSROOK_DATA_DIR"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".whatsrook"
	}
	return filepath.Join(home, ".whatsrook")
}
