// package whatsrook implements the core session manager, database coordinator, and lifecycle bridge
// for the whatsmeow whatsapp protocol engine.
//
// architectural mechanics:
// this is the central orchestrator coordinating multi-backend persistent storage (shared sqlite / postgresql),
// device registration identity resolution, companion hardware profile emulation (chrome, android, ios),
// and event-driven message dispatching. it encapsulates raw connection primitives inside a thread-safe
// client abstraction, providing structured fallback strategies, integrated caching layers, and high-level messaging helpers.
package whatsrook

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	"whatsrook/builder"
	"whatsrook/cache"
	Logger "whatsrook/logger"
	"whatsrook/qr"
	"whatsrook/system"
	"whatsrook/webp"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	_ "github.com/glebarez/go-sqlite"
	_ "github.com/lib/pq"
)

// clienttype specifies the companion operating system and hardware profile to emulate during registration.
type ClientType int

var (
	// errloggedout is the sentinel error indicating that the active session has been explicitly
	// terminated by the remote whatsapp server or unlinked by the primary device.
	ErrLoggedOut = errors.New("logged out from WhatsApp")

	// ErrPairingTimedOut indicates that the phone pairing handshake failed or timed out.
	ErrPairingTimedOut = errors.New("pairing timed out")
	// ErrPairTimeout is an alias for ErrPairingTimedOut.
	ErrPairTimeout = ErrPairingTimedOut
)

const (
	// clientchrome emulates a desktop web client running on linux.
	ClientChrome ClientType = iota
	// clientandroid emulates an android mobile companion device.
	ClientAndroid
	// clientios emulates an ios mobile companion device.
	ClientIos
)

// parseclienttype parses an arbitrary platform string into its corresponding clienttype enum.
// this performs case-insensitive normalization; returns false if the platform identifier is unknown.
func ParseClientType(s string) (ClientType, bool) {
	c, ok := map[string]ClientType{
		"chrome":  ClientChrome,
		"android": ClientAndroid,
		"ios":     ClientIos,
	}[strings.ToLower(s)]
	return c, ok
}

// config defines the operational parameters, storage directories, and runtime flags for a client instance.
type Config struct {
	// session holds the primary identifier (e.g., phone number or session token) for the device.
	Session string

	// datadir specifies the base filesystem path for logs and local database storage.
	DataDir string

	// database defines the connection uri or storage driver selector (e.g., "sqlite", postgres connection string).
	Database string

	// clienttype defines the companion device platform signature emulated during pairing.
	ClientType ClientType

	// verbose toggles debug-level tracing across whatsmeow protocol logs and internal drivers.
	Verbose bool

	// skipoldmessages instructs the client to ignore backlog history during initial handshake synchronization.
	SkipOldMessages bool

	// asyncmessageack enables non-blocking message dispatch where write operations return immediately
	// upon socket flush without blocking on server-side message receipt acknowledgments.
	AsyncMessageAck bool
}

// client is the primary abstraction encapsulating the whatsmeow core client, database container,
// and concurrency control primitives.
type Client struct {
	Config Config

	rawClient *whatsmeow.Client
	container *sqlstore.Container
	mu        sync.Mutex
}

// newclient constructs an uninitialized client instance and populates baseline configuration defaults.
func NewClient(cfg Config) *Client {
	if cfg.DataDir == "" {
		cfg.DataDir = DefaultDataDir()
	}
	return &Client{
		Config: cfg,
	}
}

// waclient returns the underlying raw whatsmeow client instance.
// caller must ensure that initsession has been executed before consuming this handle.
func (c *Client) WAClient() *whatsmeow.Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rawClient
}

// InitSession initializes persistent storage drivers, retrieves or provisions the companion device record,
// and constructs the active whatsmeow client instance.
//
// it automatically handles directory creation, sqlite / postgresql connection initialization,
// device lookup based on the configured phone session, and binds zap-based structured loggers.
func (c *Client) InitSession(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	container, err := OpenStoreContainer(ctx, c.Config.DataDir, c.Config.Database)
	if err != nil {
		return fmt.Errorf("failed to initialize store: %w", err)
	}
	c.container = container

	deviceStore, err := c.resolveDeviceStore(ctx, container)
	if err != nil {
		return fmt.Errorf("failed to get device store: %w", err)
	}

	waLogger := Logger.NewWaLogger("client")
	cli := whatsmeow.NewClient(deviceStore, waLogger)

	c.rawClient = cli
	return nil
}

// OpenStoreContainer opens and prepares a sqlstore.Container storage backend across sqlite or postgresql.
//
// it automatically creates the base directory if missing, resolves SQLite PRAGMAs for WAL mode,
// connection busy timeouts, and binds structured logging diagnostics.
func OpenStoreContainer(ctx context.Context, dataDir, database string) (*sqlstore.Container, error) {
	if dataDir == "" {
		dataDir = DefaultDataDir()
	}
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create data directory %q: %w", dataDir, err)
	}

	waLogger := Logger.NewWaLogger("database")

	if database != "" && database != "sqlite" {
		driver, dsn, err := ParseDatabaseConfig(database)
		if err != nil {
			return nil, err
		}
		return sqlstore.New(ctx, driver, dsn, waLogger)
	}

	dbPath := filepath.Join(dataDir, "whatsrook.db")
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)", dbPath)
	return sqlstore.New(ctx, "sqlite", dsn, waLogger)
}

// ParseDatabaseConfig parses a database configuration string or URL into driver name and DSN.
func ParseDatabaseConfig(dbConf string) (string, string, error) {
	if strings.HasPrefix(dbConf, "postgres://") || strings.HasPrefix(dbConf, "postgresql://") {
		return "postgres", dbConf, nil
	}
	if u, err := url.Parse(dbConf); err == nil && u.Scheme != "" {
		return u.Scheme, dbConf, nil
	}
	return "sqlite", dbConf, nil
}

// resolveDeviceStore retrieves an existing registered device or creates a fresh companion device identity.
func (c *Client) resolveDeviceStore(ctx context.Context, container *sqlstore.Container) (*store.Device, error) {
	if c.Config.Session != "" {
		phone := strings.TrimPrefix(c.Config.Session, "+")
		jid := types.NewJID(phone, types.DefaultUserServer)

		deviceStore, err := container.GetDevice(ctx, jid)
		if err != nil {
			return nil, err
		}
		if deviceStore != nil {
			return deviceStore, nil
		}
	}

	devices, err := container.GetAllDevices(ctx)
	if err != nil {
		return nil, err
	}

	if len(devices) > 0 && c.Config.Session == "" {
		return devices[0], nil
	}

	return container.NewDevice(), nil
}

// connect establishes the active websocket connection to the remote whatsapp server cluster.
func (c *Client) Connect() error {
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()

	if cli == nil {
		return fmt.Errorf("client not initialized: call InitSession first")
	}

	return cli.Connect()
}

// disconnect cleanly terminates the underlying websocket session.
func (c *Client) Disconnect() {
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()

	if cli != nil {
		cli.Disconnect()
	}
}

// isconnected returns true if the raw websocket transport layer is currently active.
func (c *Client) IsConnected() bool {
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()

	if cli == nil {
		return false
	}
	return cli.IsConnected()
}

// isloggedin returns true if the client holds a valid cryptographic session state recognized by whatsapp.
func (c *Client) IsLoggedIn() bool {
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()

	if cli == nil {
		return false
	}
	return cli.IsLoggedIn()
}

// waitforconnection blocks execution until the websocket connection state transition completes or ctx expires.
func (c *Client) WaitForConnection(ctx context.Context, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return false
		default:
			if c.IsConnected() && c.IsLoggedIn() {
				return true
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	return false
}

// handlesessionreset cleanly wipes local session identity keys and resets store records.
func (c *Client) HandleSessionReset(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.rawClient != nil {
		c.rawClient.Disconnect()
		if c.rawClient.Store != nil && c.rawClient.Store.ID != nil {
			_ = c.rawClient.Store.Delete(ctx)
		}
		c.rawClient = nil
	}

	if c.container != nil {
		_ = c.container.Close()
		c.container = nil
	}

	dbPath := filepath.Join(c.Config.DataDir, "whatsrook.db")
	_ = os.Remove(dbPath)
	_ = os.Remove(dbPath + "-wal")
	_ = os.Remove(dbPath + "-shm")

	return nil
}

// close cleanly tears down client connections, background workers, and persistent store handles.
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

// pairphone initiates companion device linking using an 8-character numeric verification code.
func (c *Client) PairPhone(ctx context.Context, phone string) (string, error) {
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()

	if cli == nil {
		return "", fmt.Errorf("client not initialized")
	}

	phone = strings.TrimPrefix(phone, "+")
	phone = strings.ReplaceAll(phone, " ", "")
	phone = strings.ReplaceAll(phone, "-", "")

	clientType := whatsmeow.PairClientChrome
	displayName := "Chrome (Linux)"
	switch c.Config.ClientType {
	case ClientAndroid:
		displayName = "Android"
	case ClientIos:
		displayName = "iOS"
	}

	code, err := cli.PairPhone(ctx, phone, true, clientType, displayName)
	if err != nil {
		return "", fmt.Errorf("failed to pair phone: %w", err)
	}

	return code, nil
}

// pairqr initiates companion device pairing by returning a stream of qr code data strings.
func (c *Client) PairQR(ctx context.Context) (<-chan whatsmeow.QRChannelItem, error) {
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()

	if cli == nil {
		return nil, fmt.Errorf("client not initialized")
	}

	qrChan, err := cli.GetQRChannel(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get QR channel: %w", err)
	}

	return qrChan, nil
}

//go:fix inline
func protoStr(s string) *string { return new(s) }

//go:fix inline
func protoBool(b bool) *bool { return new(b) }

// GetQRChannel streams QR code events for pairing.
//
//go:fix inline
func (c *Client) GetQRChannel(ctx context.Context) (<-chan whatsmeow.QRChannelItem, error) {
	return c.PairQR(ctx)
}

// QRChannel is an alias for GetQRChannel.
func (c *Client) QRChannel(ctx context.Context) (<-chan whatsmeow.QRChannelItem, error) {
	return c.PairQR(ctx)
}

// GetMediaType returns the human-readable media classification string.
func GetMediaType(msg *waE2E.Message) string {
	if msg == nil {
		return "text"
	}
	if msg.ImageMessage != nil {
		return "image"
	}
	if msg.VideoMessage != nil {
		if msg.VideoMessage.GetGifPlayback() {
			return "gif"
		}
		return "video"
	}
	if msg.AudioMessage != nil {
		if msg.AudioMessage.GetPTT() {
			return "voice"
		}
		return "audio"
	}
	if msg.DocumentMessage != nil {
		return "document"
	}
	if msg.StickerMessage != nil {
		return "sticker"
	}
	return "text"
}

// defaultdatadir returns the default filesystem storage path across target operating systems.
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

// storedsession describes an existing whatsapp device session found in the store.
type StoredSession struct {
	JID      string
	User     string // phone number (without plus)
	PushName string
	Platform string
	Business bool
}

// ListStoredSessions queries the database store for all saved companion device sessions across SQLite or PostgreSQL.
//
// it inspects registered device records without opening active network websockets, returning
// structured metadata such as JID, push name, emulated platform, and business account indicators.
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

// DeleteStoredSession initiates a graceful server-side logout request to unpair the companion device,
// followed by complete purging of local session keys and device records from persistent storage.
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
		return fallbackDeleteDevice(ctx, dataDir, database, phone)
	}

	cli := client.WAClient()
	if cli != nil && cli.Store != nil && cli.Store.ID != nil {
		connected := make(chan struct{}, 1)
		cli.AddEventHandler(func(evt any) {
			if _, ok := evt.(*events.Connected); ok {
				select {
				case connected <- struct{}{}:
				default:
				}
			}
		})

		_ = cli.Connect()

		select {
		case <-connected:
			logoutCtx, logoutCancel := context.WithTimeout(ctx, 5*time.Second)
			_ = cli.Logout(logoutCtx)
			logoutCancel()
		case <-time.After(3 * time.Second):
		}

		_ = cli.Store.Delete(ctx)
		return nil
	}

	return fallbackDeleteDevice(ctx, dataDir, database, phone)
}

// fallbackDeleteDevice performs direct database-level removal of device records when network logout is impossible or failed.
func fallbackDeleteDevice(ctx context.Context, dataDir, database, phone string) error {
	container, err := OpenStoreContainer(ctx, dataDir, database)
	if err != nil {
		return fmt.Errorf("failed to open database container: %w", err)
	}
	defer func() {
		_ = container.Close()
	}()

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

// ClearSessionDB deletes the device record associated with this client or phone number.
func (c *Client) ClearSessionDB(ctx context.Context, phone string) {
	c.mu.Lock()
	raw := c.rawClient
	c.mu.Unlock()
	if raw != nil && raw.Store != nil {
		_ = raw.Store.Delete(ctx)
	}
	if phone != "" {
		_ = fallbackDeleteDevice(ctx, c.Config.DataDir, c.Config.Database, phone)
	}
}

// AttachContextInfo attaches ContextInfo metadata to inner proto payloads.
func AttachContextInfo(msg *waE2E.Message, ci *waE2E.ContextInfo) {
	if msg == nil || ci == nil {
		return
	}
	if msg.ExtendedTextMessage != nil {
		msg.ExtendedTextMessage.ContextInfo = ci
	} else if msg.ImageMessage != nil {
		msg.ImageMessage.ContextInfo = ci
	} else if msg.VideoMessage != nil {
		msg.VideoMessage.ContextInfo = ci
	} else if msg.AudioMessage != nil {
		msg.AudioMessage.ContextInfo = ci
	} else if msg.DocumentMessage != nil {
		msg.DocumentMessage.ContextInfo = ci
	} else if msg.StickerMessage != nil {
		msg.StickerMessage.ContextInfo = ci
	} else if msg.Conversation != nil {
		text := *msg.Conversation
		msg.Conversation = nil
		msg.ExtendedTextMessage = &waE2E.ExtendedTextMessage{
			Text:        &text,
			ContextInfo: ci,
		}
	}
}

// ─── Protocol Helper Utilities ─────────────────────────────────────────────

// UnwrapMessageProto unwraps nested message envelopes (ViewOnce, Ephemeral, Edited, Forwarded).
func UnwrapMessageProto(msg *waE2E.Message) *waE2E.Message {
	if msg == nil {
		return nil
	}
	for {
		if ephem := msg.GetEphemeralMessage(); ephem != nil && ephem.GetMessage() != nil {
			msg = ephem.GetMessage()
			continue
		}
		if vo := msg.GetViewOnceMessage(); vo != nil && vo.GetMessage() != nil {
			msg = vo.GetMessage()
			continue
		}
		if vo2 := msg.GetViewOnceMessageV2(); vo2 != nil && vo2.GetMessage() != nil {
			msg = vo2.GetMessage()
			continue
		}
		if vo2ext := msg.GetViewOnceMessageV2Extension(); vo2ext != nil && vo2ext.GetMessage() != nil {
			msg = vo2ext.GetMessage()
			continue
		}
		if docCap := msg.GetDocumentWithCaptionMessage(); docCap != nil && docCap.GetMessage() != nil {
			msg = docCap.GetMessage()
			continue
		}
		if edited := msg.GetEditedMessage(); edited != nil && edited.GetMessage() != nil {
			msg = edited.GetMessage()
			continue
		}
		if bfm := msg.GetBotForwardedMessage(); bfm != nil && bfm.GetMessage() != nil {
			msg = bfm.GetMessage()
			continue
		}
		if devSent := msg.GetDeviceSentMessage(); devSent != nil && devSent.GetMessage() != nil {
			msg = devSent.GetMessage()
			continue
		}
		break
	}
	return msg
}

// GetContextInfoFromProto retrieves ContextInfo from any supported protobuf message variant.
func GetContextInfoFromProto(msg *waE2E.Message) *waE2E.ContextInfo {
	msg = UnwrapMessageProto(msg)
	if msg == nil {
		return nil
	}
	if ext := msg.GetExtendedTextMessage(); ext != nil && ext.GetContextInfo() != nil {
		return ext.GetContextInfo()
	}
	if img := msg.GetImageMessage(); img != nil && img.GetContextInfo() != nil {
		return img.GetContextInfo()
	}
	if vid := msg.GetVideoMessage(); vid != nil && vid.GetContextInfo() != nil {
		return vid.GetContextInfo()
	}
	if aud := msg.GetAudioMessage(); aud != nil && aud.GetContextInfo() != nil {
		return aud.GetContextInfo()
	}
	if doc := msg.GetDocumentMessage(); doc != nil && doc.GetContextInfo() != nil {
		return doc.GetContextInfo()
	}
	if stk := msg.GetStickerMessage(); stk != nil && stk.GetContextInfo() != nil {
		return stk.GetContextInfo()
	}
	if btn := msg.GetButtonsMessage(); btn != nil && btn.GetContextInfo() != nil {
		return btn.GetContextInfo()
	}
	if btnResp := msg.GetButtonsResponseMessage(); btnResp != nil && btnResp.GetContextInfo() != nil {
		return btnResp.GetContextInfo()
	}
	if list := msg.GetListResponseMessage(); list != nil && list.GetContextInfo() != nil {
		return list.GetContextInfo()
	}
	if poll := msg.GetPollCreationMessage(); poll != nil && poll.GetContextInfo() != nil {
		return poll.GetContextInfo()
	}
	return nil
}

// ExtractMessageText extracts the human-readable text string from any incoming message event.
func ExtractMessageText(evt *events.Message) string {
	if evt == nil || evt.Message == nil {
		return ""
	}
	return ExtractTextFromProto(evt.Message)
}

// ExtractTextFromProto extracts human-readable text from a raw protobuf message.
func ExtractTextFromProto(msg *waE2E.Message) string {
	msg = UnwrapMessageProto(msg)
	if msg == nil {
		return ""
	}
	if c := msg.GetConversation(); c != "" {
		return c
	}
	if ext := msg.GetExtendedTextMessage(); ext != nil && ext.GetText() != "" {
		return ext.GetText()
	}
	if img := msg.GetImageMessage(); img != nil && img.GetCaption() != "" {
		return img.GetCaption()
	}
	if vid := msg.GetVideoMessage(); vid != nil && vid.GetCaption() != "" {
		return vid.GetCaption()
	}
	if doc := msg.GetDocumentMessage(); doc != nil && doc.GetCaption() != "" {
		return doc.GetCaption()
	}
	if poll := msg.GetPollCreationMessage(); poll != nil && poll.GetName() != "" {
		return poll.GetName()
	}
	return ""
}

// IsAdminRaw checks if a participant JID is an admin in groupInfo.
func IsAdminRaw(ctx context.Context, client *whatsmeow.Client, groupInfo *types.GroupInfo, userJID types.JID) bool {
	if groupInfo == nil {
		return false
	}
	target := userJID.ToNonAD()
	for _, p := range groupInfo.Participants {
		if p.JID.ToNonAD().User == target.User {
			return p.IsAdmin || p.IsSuperAdmin
		}
	}
	return false
}

// IsSudoRaw checks if a sender JID has sudo/owner privileges stored in database settings.
func IsSudoRaw(ctx context.Context, client *whatsmeow.Client, sender types.JID) bool {
	if client == nil || client.Store == nil || client.Store.ID == nil {
		return false
	}
	if IsSameUserRaw(ctx, client, sender, *client.Store.ID) {
		return true
	}
	if client.Store.Identities == nil {
		return false
	}
	s, ok := client.Store.Identities.(interface {
		GetSetting(ctx context.Context, key string) (string, error)
	})
	if !ok {
		return false
	}
	val, err := s.GetSetting(ctx, "sudo:"+sender.ToNonAD().String())
	return err == nil && val == "true"
}

// IsSameUserRaw compares two JIDs ignoring device and agent AD suffixes.
func IsSameUserRaw(ctx context.Context, client *whatsmeow.Client, jid1, jid2 types.JID) bool {
	return jid1.ToNonAD().User == jid2.ToNonAD().User
}

// ResolveMentionRaw resolves mention strings and display usernames for a given participant JID.
func ResolveMentionRaw(ctx context.Context, client *whatsmeow.Client, participant types.JID) (types.JID, string) {
	resolved := participant.ToNonAD()
	username := resolved.User
	if client != nil && client.Store != nil && client.Store.Contacts != nil {
		if contact, err := client.Store.Contacts.GetContact(ctx, resolved); err == nil && contact.Found {
			if contact.PushName != "" {
				username = contact.PushName
			} else if contact.FullName != "" {
				username = contact.FullName
			}
		}
	}
	return resolved, username
}

// RemoveEmojis strips emoji characters from text strings.
func RemoveEmojis(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 0x1F000 && r <= 0x1FFFF || r >= 0x2600 && r <= 0x27BF {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// ─── Text & Formatting Facades ─────────────────────────────────────────────

var (
	// Bold returns plain text without WhatsApp bold formatting symbols (*).
	Bold = builder.Bold
	// Boldf formats text according to format specifier without bold formatting symbols.
	Boldf = builder.Boldf
	// Italic returns plain text without WhatsApp italic formatting symbols (_).
	Italic = builder.Italic
	// Italicf formats text according to format specifier without italic formatting symbols.
	Italicf = builder.Italicf
	// Code returns plain text without WhatsApp inline code formatting symbols (`).
	Code = builder.Code
	// Codef formats text according to format specifier without inline code formatting symbols.
	Codef = builder.Codef
	// CodeBlock returns plain text without WhatsApp code block formatting symbols (```).
	CodeBlock = builder.CodeBlock
	// Strike returns plain text without WhatsApp strikethrough formatting symbols (~).
	Strike = builder.Strike
	// Strikef formats text according to format specifier without strikethrough formatting symbols.
	Strikef = builder.Strikef
	// Quote returns plain text without WhatsApp quote formatting symbols (>).
	Quote = builder.Quote
	// Quotef formats text according to format specifier without quote formatting symbols.
	Quotef = builder.Quotef
	// NewText creates an interactive text builder instance.
	NewText = builder.NewText
)

// ─── Interactive Loader Engine ─────────────────────────────────────────────

var (
	loaderFrames    = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	activeLoaders   sync.Map
	loaderIDCounter uint64
	loaderIDMu      sync.Mutex
)

func nextLoaderID() string {
	loaderIDMu.Lock()
	defer loaderIDMu.Unlock()
	loaderIDCounter++
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), loaderIDCounter)
}

// Loader manages a live animating status indicator message in chat.
type Loader struct {
	ctx         *PluginContext
	id          string
	initialText string
	msgID       types.MessageID
	stopChan    chan struct{}
	active      bool
	stopped     bool
	mu          sync.Mutex
}

// StartLoader returns a Loader that sends an animated loading message to the chat,
// continuously editing frame-by-frame until the operation completes, then deleting the message.
func (ctx *PluginContext) StartLoader(initialText ...string) *Loader {
	txt := "Please wait"
	if len(initialText) > 0 && initialText[0] != "" {
		txt = initialText[0]
	}
	loaderID := nextLoaderID()
	l := &Loader{
		ctx:         ctx,
		id:          loaderID,
		initialText: txt,
		stopChan:    make(chan struct{}),
	}

	activeLoaders.Store(loaderID, l)
	l.activate()
	return l
}

func (l *Loader) activate() {
	l.mu.Lock()
	if l.stopped {
		l.mu.Unlock()
		return
	}
	l.active = true
	l.mu.Unlock()

	frame := loaderFrames[0]
	displayText := fmt.Sprintf("%s %s", l.initialText, frame)

	resp, err := l.ctx.Client.SendMessage(l.ctx.GetSendContext(), l.ctx.Chat, &waE2E.Message{
		Conversation: &displayText,
	})
	if err != nil {
		return
	}

	l.mu.Lock()
	l.msgID = resp.ID
	l.mu.Unlock()

	go l.run()
}

func (l *Loader) run() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	frameIdx := 1
	for {
		select {
		case <-l.stopChan:
			return
		case <-l.ctx.Ctx.Done():
			l.Delete()
			return
		case <-ticker.C:
			l.mu.Lock()
			if l.stopped || l.msgID == "" {
				l.mu.Unlock()
				return
			}
			frame := loaderFrames[frameIdx%len(loaderFrames)]
			frameIdx++
			l.mu.Unlock()

			displayText := fmt.Sprintf("%s %s", l.initialText, frame)
			l.editFrame(displayText)
		}
	}
}

func (l *Loader) editFrame(text string) {
	l.mu.Lock()
	if l.stopped || l.msgID == "" {
		l.mu.Unlock()
		return
	}
	msgID := l.msgID
	l.mu.Unlock()

	if l.ctx == nil || l.ctx.Client == nil {
		return
	}

	formatted := l.ctx.formatTextResponse(text)
	msg := &waE2E.Message{
		Conversation: &formatted,
	}
	editMsg := l.ctx.Client.BuildEdit(l.ctx.Chat, msgID, msg)
	_, _ = l.ctx.Client.SendMessage(l.ctx.GetSendContext(), l.ctx.Chat, editMsg)
}

// Cancel cancels the running operation context associated with this loader.
func (l *Loader) Cancel() {
	l.Stop()
	if l.ctx != nil {
		l.ctx.Cancel()
	}
	l.mu.Lock()
	msgID := l.msgID
	l.msgID = ""
	l.mu.Unlock()

	if msgID != "" && l.ctx != nil && l.ctx.Client != nil {
		formatted := l.ctx.formatTextResponse("Operation cancelled.")
		msg := &waE2E.Message{
			Conversation: &formatted,
		}
		editMsg := l.ctx.Client.BuildEdit(l.ctx.Chat, msgID, msg)
		_, _ = l.ctx.Client.SendMessage(l.ctx.GetSendContext(), l.ctx.Chat, editMsg)
	}
}

// CancelLoader looks up active loader by ID and cancels its operation.
func CancelLoader(id string) bool {
	val, ok := activeLoaders.Load(id)
	if !ok {
		return false
	}
	if l, ok := val.(*Loader); ok {
		l.Cancel()
		return true
	}
	return false
}

// MessageID returns the underlying MessageID of the loader message.
func (l *Loader) MessageID() types.MessageID {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.msgID
}

// Stop halts the animation ticker.
func (l *Loader) Stop() {
	l.mu.Lock()
	if !l.stopped {
		l.stopped = true
		close(l.stopChan)
		activeLoaders.Delete(l.id)
	}
	l.mu.Unlock()
}

// Done stops the animation ticker and updates the loader message with final text if active.
func (l *Loader) Done(finalText string) {
	l.Stop()
	l.mu.Lock()
	msgID := l.msgID
	l.msgID = ""
	l.mu.Unlock()

	if msgID != "" && finalText != "" && l.ctx != nil && l.ctx.Client != nil {
		formatted := l.ctx.formatTextResponse(finalText)
		msg := &waE2E.Message{
			Conversation: &formatted,
		}
		editMsg := l.ctx.Client.BuildEdit(l.ctx.Chat, msgID, msg)
		_, _ = l.ctx.Client.SendMessage(l.ctx.GetSendContext(), l.ctx.Chat, editMsg)
	}
}

// Delete stops the animation ticker and deletes the loader message from chat if active.
func (l *Loader) Delete() {
	l.Stop()
	l.mu.Lock()
	msgID := l.msgID
	l.msgID = ""
	l.mu.Unlock()

	if msgID != "" && l.ctx != nil && l.ctx.Client != nil {
		revokeMsg := l.ctx.Client.BuildRevoke(l.ctx.Chat, types.EmptyJID, msgID)
		_, _ = l.ctx.Client.SendMessage(l.ctx.GetSendContext(), l.ctx.Chat, revokeMsg)
	}
}

// ─── Plugin Context Model ──────────────────────────────────────────────────

// PluginContext captures the invocation execution environment for external/native plugin actions.
type PluginContext struct {
	Ctx        context.Context
	CancelFunc context.CancelFunc
	Client     *whatsmeow.Client
	Evt        *events.Message

	Command string
	Args    []string
	RawArgs string

	Chat   types.JID
	Sender types.JID

	autoLoaderMu  sync.Mutex
	autoLoader    *Loader
	loaderTimer   *time.Timer
	loaderStopped bool
}

// Cancel invokes context cancellation if configured.
func (c *PluginContext) Cancel() {
	if c.CancelFunc != nil {
		c.CancelFunc()
	}
}

// GetSendContext returns an active, non-canceled Context suitable for network dispatch.
func (c *PluginContext) GetSendContext() context.Context {
	if c == nil || c.Ctx == nil || c.Ctx.Err() != nil {
		return context.Background()
	}
	return c.Ctx
}

// GetClient returns the underlying whatsmeow client instance.
func (c *PluginContext) GetClient() *whatsmeow.Client {
	if c == nil {
		return nil
	}
	return c.Client
}

// GetChat returns the current chat JID.
func (c *PluginContext) GetChat() types.JID {
	if c == nil {
		return types.EmptyJID
	}
	return c.Chat
}

// GetSender returns the triggering sender JID.
func (c *PluginContext) GetSender() types.JID {
	if c == nil {
		return types.EmptyJID
	}
	return c.Sender
}

// FormatTextResponse formats the text response stripping unwanted symbols.
func (c *PluginContext) FormatTextResponse(text string) string {
	return c.formatTextResponse(text)
}

// ReplyContextInfo returns the ContextInfo configured for quoted reply.
func (c *PluginContext) ReplyContextInfo() *waE2E.ContextInfo {
	return c.replyContextInfo()
}

func (c *PluginContext) formatTextResponse(text string) string {
	text = strings.ReplaceAll(text, "*", "")
	text = RemoveEmojis(text)
	text = strings.ReplaceAll(text, "```", "")
	return text
}

func (c *PluginContext) replyContextInfo() *waE2E.ContextInfo {
	if c.Evt == nil {
		return nil
	}
	ci := GetContextInfoFromProto(c.Evt.Message)
	if ci == nil {
		ci = &waE2E.ContextInfo{}
	}
	ci.StanzaID = &c.Evt.Info.ID
	ci.Participant = protoStr(c.Evt.Info.Sender.ToNonAD().String())
	ci.QuotedMessage = UnwrapMessageProto(c.Evt.Message)
	return ci
}

// SendText sends a plain text message without quoting.
func (c *PluginContext) SendText(text string) error {
	c.StopAutoLoader()
	if c.Client == nil {
		return fmt.Errorf("client unavailable")
	}
	formatted := c.formatTextResponse(text)
	_, err := c.Client.SendMessage(c.GetSendContext(), c.Chat, &waE2E.Message{
		Conversation: &formatted,
	})
	return err
}

// SendTextWithMentions sends a text message with mentioned JIDs without quoting.
func (c *PluginContext) SendTextWithMentions(text string, mentions []types.JID) error {
	c.StopAutoLoader()
	if c.Client == nil {
		return fmt.Errorf("client unavailable")
	}
	formatted := c.formatTextResponse(text)
	var mentionStrs []string
	for _, m := range mentions {
		if !m.IsEmpty() {
			mentionStrs = append(mentionStrs, m.ToNonAD().String())
		}
	}
	_, err := c.Client.SendMessage(c.GetSendContext(), c.Chat, &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: &formatted,
			ContextInfo: &waE2E.ContextInfo{
				MentionedJID: mentionStrs,
			},
		},
	})
	return err
}

// SendImage sends an image without quoting.
func (c *PluginContext) SendImage(data []byte, mimetype, caption string) error {
	return c.SendImageWithMentions(data, mimetype, caption, nil)
}

// SendImageWithMentions sends an image with mentions without quoting.
func (c *PluginContext) SendImageWithMentions(data []byte, mimetype, caption string, mentions []types.JID) error {
	c.StopAutoLoader()
	if c.Client == nil {
		return fmt.Errorf("client unavailable")
	}
	if mimetype == "" {
		mimetype = "image/jpeg"
	}
	uploaded, err := c.Client.Upload(c.GetSendContext(), data, whatsmeow.MediaImage)
	if err != nil {
		return fmt.Errorf("upload image failed: %w", err)
	}

	var ci *waE2E.ContextInfo
	if len(mentions) > 0 {
		ci = &waE2E.ContextInfo{}
		for _, m := range mentions {
			if !m.IsEmpty() {
				ci.MentionedJID = append(ci.MentionedJID, m.ToNonAD().String())
			}
		}
	}

	msg := &waE2E.Message{
		ImageMessage: &waE2E.ImageMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimetype,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    protoUint64(uint64(len(data))),
			Caption:       &caption,
			ContextInfo:   ci,
		},
	}
	_, err = c.Client.SendMessage(c.GetSendContext(), c.Chat, msg)
	return err
}

// SendVideo sends a video without quoting.
func (c *PluginContext) SendVideo(data []byte, mimetype, caption string) error {
	return c.sendVideoInternal(data, mimetype, caption, false)
}

// SendVideoGif sends a looping GIF video without quoting.
func (c *PluginContext) SendVideoGif(data []byte, mimetype, caption string) error {
	return c.sendVideoInternal(data, mimetype, caption, true)
}

// SendVideoWithMentions sends a video without quoting, mentioning specific user JIDs.
func (c *PluginContext) SendVideoWithMentions(data []byte, mimetype, caption string, mentions []types.JID) error {
	c.StopAutoLoader()
	if c.Client == nil {
		return fmt.Errorf("client unavailable")
	}
	if mimetype == "" {
		mimetype = "video/mp4"
	}
	uploaded, err := c.Client.Upload(c.GetSendContext(), data, whatsmeow.MediaVideo)
	if err != nil {
		return fmt.Errorf("upload video failed: %w", err)
	}

	var mentionStrs []string
	for _, m := range mentions {
		if !m.IsEmpty() {
			mentionStrs = append(mentionStrs, m.ToNonAD().String())
		}
	}

	msg := &waE2E.Message{
		VideoMessage: &waE2E.VideoMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimetype,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    protoUint64(uint64(len(data))),
			Caption:       &caption,
			ContextInfo: &waE2E.ContextInfo{
				MentionedJID: mentionStrs,
			},
		},
	}
	_, err = c.Client.SendMessage(c.GetSendContext(), c.Chat, msg)
	return err
}

func (c *PluginContext) sendVideoInternal(data []byte, mimetype, caption string, isGif bool) error {
	c.StopAutoLoader()
	if c.Client == nil {
		return fmt.Errorf("client unavailable")
	}
	if mimetype == "" {
		mimetype = "video/mp4"
	}
	uploaded, err := c.Client.Upload(c.GetSendContext(), data, whatsmeow.MediaVideo)
	if err != nil {
		return fmt.Errorf("upload video failed: %w", err)
	}
	msg := &waE2E.Message{
		VideoMessage: &waE2E.VideoMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimetype,
			GifPlayback:   protoBool(isGif),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    protoUint64(uint64(len(data))),
			Caption:       &caption,
		},
	}
	_, err = c.Client.SendMessage(c.GetSendContext(), c.Chat, msg)
	return err
}

// SendAudio sends an audio file without quoting.
func (c *PluginContext) SendAudio(data []byte, mimetype string) error {
	c.StopAutoLoader()
	if c.Client == nil {
		return fmt.Errorf("client unavailable")
	}
	if mimetype == "" {
		mimetype = "audio/mp4"
	}
	uploaded, err := c.Client.Upload(c.GetSendContext(), data, whatsmeow.MediaAudio)
	if err != nil {
		return fmt.Errorf("upload audio failed: %w", err)
	}
	msg := &waE2E.Message{
		AudioMessage: &waE2E.AudioMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimetype,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    protoUint64(uint64(len(data))),
		},
	}
	_, err = c.Client.SendMessage(c.GetSendContext(), c.Chat, msg)
	return err
}

// SendDocument sends a document without quoting.
func (c *PluginContext) SendDocument(data []byte, mimetype, filename, caption string) error {
	c.StopAutoLoader()
	if c.Client == nil {
		return fmt.Errorf("client unavailable")
	}
	if mimetype == "" {
		mimetype = "application/octet-stream"
	}
	uploaded, err := c.Client.Upload(c.GetSendContext(), data, whatsmeow.MediaDocument)
	if err != nil {
		return fmt.Errorf("upload document failed: %w", err)
	}
	msg := &waE2E.Message{
		DocumentMessage: &waE2E.DocumentMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimetype,
			FileName:      &filename,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    protoUint64(uint64(len(data))),
			Caption:       &caption,
		},
	}
	_, err = c.Client.SendMessage(c.GetSendContext(), c.Chat, msg)
	return err
}

// SendSticker sends a WebP sticker without quoting.
func (c *PluginContext) SendSticker(data []byte) error {
	c.StopAutoLoader()
	if c.Client == nil {
		return fmt.Errorf("client unavailable")
	}
	uploaded, err := c.Client.Upload(c.GetSendContext(), data, whatsmeow.MediaImage)
	if err != nil {
		return fmt.Errorf("upload sticker failed: %w", err)
	}
	msg := &waE2E.Message{
		StickerMessage: &waE2E.StickerMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      protoStr("image/webp"),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    protoUint64(uint64(len(data))),
		},
	}
	_, err = c.Client.SendMessage(c.GetSendContext(), c.Chat, msg)
	return err
}

// SendTextWithGroupMention sends a text message with WhatsApp native group mention.
func (c *PluginContext) SendTextWithGroupMention(text string) error {
	c.StopAutoLoader()
	if c.Client == nil {
		return fmt.Errorf("client unavailable")
	}
	formatted := c.formatTextResponse(text)
	var nonJID uint32 = 1
	msg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: &formatted,
			ContextInfo: &waE2E.ContextInfo{
				NonJIDMentions: &nonJID,
			},
		},
	}
	_, err := c.Client.SendMessage(c.GetSendContext(), c.Chat, msg)
	return err
}

// Reply sends a plain text response quoted to the triggering message.
func (c *PluginContext) Reply(text string) error {
	_, err := c.ReplyWithID(text)
	return err
}

// Replyf formats and sends a text response quoted to the triggering message.
func (c *PluginContext) Replyf(format string, args ...any) error {
	return c.Reply(fmt.Sprintf(format, args...))
}

// ReplyWithMentions sends a text message with mentions quoted to the triggering message.
func (c *PluginContext) ReplyWithMentions(text string, mentions []types.JID) error {
	c.StopAutoLoader()
	if c.Client == nil {
		return fmt.Errorf("client unavailable")
	}
	formatted := c.formatTextResponse(text)
	ci := c.replyContextInfo()
	if ci == nil {
		ci = &waE2E.ContextInfo{}
	}
	for _, m := range mentions {
		if !m.IsEmpty() {
			ci.MentionedJID = append(ci.MentionedJID, m.ToNonAD().String())
		}
	}
	msg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text:        &formatted,
			ContextInfo: ci,
		},
	}
	_, err := c.Client.SendMessage(c.GetSendContext(), c.Chat, msg)
	return err
}

// ReplyWithID sends a text message and returns the sent message ID.
func (c *PluginContext) ReplyWithID(text string) (string, error) {
	c.StopAutoLoader()
	if c.Client == nil {
		return "", fmt.Errorf("client unavailable")
	}
	ctx := c.GetSendContext()
	formatted := c.formatTextResponse(text)
	msg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text:        &formatted,
			ContextInfo: c.replyContextInfo(),
		},
	}
	resp, err := c.Client.SendMessage(ctx, c.Chat, msg)
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

// ReplyWithImage uploads and sends an image quoted to the triggering message.
func (c *PluginContext) ReplyWithImage(data []byte, mimetype, caption string) error {
	return c.ReplyWithImageWithMentions(data, mimetype, caption, nil)
}

// ReplyWithImageWithMentions uploads and sends an image with mentions quoted to the triggering message.
func (c *PluginContext) ReplyWithImageWithMentions(data []byte, mimetype, caption string, mentions []types.JID) error {
	c.StopAutoLoader()
	if c.Client == nil {
		return fmt.Errorf("client unavailable")
	}
	if mimetype == "" {
		mimetype = "image/jpeg"
	}
	uploaded, err := c.Client.Upload(c.GetSendContext(), data, whatsmeow.MediaImage)
	if err != nil {
		return fmt.Errorf("upload image failed: %w", err)
	}

	ci := c.replyContextInfo()
	if ci == nil && len(mentions) > 0 {
		ci = &waE2E.ContextInfo{}
	}
	if ci != nil {
		for _, m := range mentions {
			if !m.IsEmpty() {
				ci.MentionedJID = append(ci.MentionedJID, m.ToNonAD().String())
			}
		}
	}

	msg := &waE2E.Message{
		ImageMessage: &waE2E.ImageMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimetype,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    protoUint64(uint64(len(data))),
			Caption:       &caption,
			ContextInfo:   ci,
		},
	}
	_, err = c.Client.SendMessage(c.GetSendContext(), c.Chat, msg)
	return err
}

// ReplyWithVideo uploads and sends a video quoted to the triggering message.
func (c *PluginContext) ReplyWithVideo(data []byte, mimetype, caption string) error {
	return c.replyVideoInternal(data, mimetype, caption, false)
}

// ReplyWithVideoGif uploads and sends a GIF video quoted to the triggering message.
func (c *PluginContext) ReplyWithVideoGif(data []byte, mimetype, caption string) error {
	return c.replyVideoInternal(data, mimetype, caption, true)
}

func (c *PluginContext) replyVideoInternal(data []byte, mimetype, caption string, isGif bool) error {
	c.StopAutoLoader()
	if c.Client == nil {
		return fmt.Errorf("client unavailable")
	}
	if mimetype == "" {
		mimetype = "video/mp4"
	}
	uploaded, err := c.Client.Upload(c.GetSendContext(), data, whatsmeow.MediaVideo)
	if err != nil {
		return fmt.Errorf("upload video failed: %w", err)
	}

	msg := &waE2E.Message{
		VideoMessage: &waE2E.VideoMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimetype,
			GifPlayback:   protoBool(isGif),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    protoUint64(uint64(len(data))),
			Caption:       &caption,
			ContextInfo:   c.replyContextInfo(),
		},
	}
	_, err = c.Client.SendMessage(c.GetSendContext(), c.Chat, msg)
	return err
}

// ReplyWithVideoWithMentions uploads and sends video quoted with mentioned user JIDs.
func (c *PluginContext) ReplyWithVideoWithMentions(data []byte, mimetype, caption string, mentions []types.JID) error {
	c.StopAutoLoader()
	if c.Client == nil {
		return fmt.Errorf("client unavailable")
	}
	if mimetype == "" {
		mimetype = "video/mp4"
	}
	uploaded, err := c.Client.Upload(c.GetSendContext(), data, whatsmeow.MediaVideo)
	if err != nil {
		return fmt.Errorf("upload video failed: %w", err)
	}

	ci := c.replyContextInfo()
	if ci == nil {
		ci = &waE2E.ContextInfo{}
	}
	var mentionStrs []string
	for _, m := range mentions {
		if !m.IsEmpty() {
			mentionStrs = append(mentionStrs, m.ToNonAD().String())
		}
	}
	ci.MentionedJID = mentionStrs

	msg := &waE2E.Message{
		VideoMessage: &waE2E.VideoMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimetype,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    protoUint64(uint64(len(data))),
			Caption:       &caption,
			ContextInfo:   ci,
		},
	}
	_, err = c.Client.SendMessage(c.GetSendContext(), c.Chat, msg)
	return err
}

// ReplyWithAudio uploads and sends audio quoted to the triggering message.
func (c *PluginContext) ReplyWithAudio(data []byte, mimetype string) error {
	c.StopAutoLoader()
	if c.Client == nil {
		return fmt.Errorf("client unavailable")
	}
	if mimetype == "" {
		mimetype = "audio/mp4"
	}
	uploaded, err := c.Client.Upload(c.GetSendContext(), data, whatsmeow.MediaAudio)
	if err != nil {
		return fmt.Errorf("upload audio failed: %w", err)
	}

	msg := &waE2E.Message{
		AudioMessage: &waE2E.AudioMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimetype,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    protoUint64(uint64(len(data))),
			ContextInfo:   c.replyContextInfo(),
		},
	}
	_, err = c.Client.SendMessage(c.GetSendContext(), c.Chat, msg)
	return err
}

// ReplyWithDocument uploads and sends a file document quoted to the triggering message.
func (c *PluginContext) ReplyWithDocument(data []byte, mimetype, filename, caption string) error {
	c.StopAutoLoader()
	if c.Client == nil {
		return fmt.Errorf("client unavailable")
	}
	if mimetype == "" {
		mimetype = "application/octet-stream"
	}
	uploaded, err := c.Client.Upload(c.GetSendContext(), data, whatsmeow.MediaDocument)
	if err != nil {
		return fmt.Errorf("upload document failed: %w", err)
	}

	msg := &waE2E.Message{
		DocumentMessage: &waE2E.DocumentMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimetype,
			FileName:      &filename,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    protoUint64(uint64(len(data))),
			Caption:       &caption,
			ContextInfo:   c.replyContextInfo(),
		},
	}
	_, err = c.Client.SendMessage(c.GetSendContext(), c.Chat, msg)
	return err
}

// ReplyWithSticker uploads and sends a WebP sticker quoted to the triggering message.
func (c *PluginContext) ReplyWithSticker(data []byte) error {
	c.StopAutoLoader()
	if c.Client == nil {
		return fmt.Errorf("client unavailable")
	}
	uploaded, err := c.Client.Upload(c.GetSendContext(), data, whatsmeow.MediaImage)
	if err != nil {
		return fmt.Errorf("upload sticker failed: %w", err)
	}

	msg := &waE2E.Message{
		StickerMessage: &waE2E.StickerMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      protoStr("image/webp"),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    protoUint64(uint64(len(data))),
			ContextInfo:   c.replyContextInfo(),
		},
	}
	_, err = c.Client.SendMessage(c.GetSendContext(), c.Chat, msg)
	return err
}

// ReplyWithGroupMention sends a text message with group mention quoted to the triggering message.
func (c *PluginContext) ReplyWithGroupMention(text string) error {
	c.StopAutoLoader()
	if c.Client == nil {
		return fmt.Errorf("client unavailable")
	}
	formatted := c.formatTextResponse(text)
	var nonJID uint32 = 1
	ci := c.replyContextInfo()
	if ci == nil {
		ci = &waE2E.ContextInfo{}
	}
	ci.NonJIDMentions = &nonJID

	msg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text:        &formatted,
			ContextInfo: ci,
		},
	}
	_, err := c.Client.SendMessage(c.GetSendContext(), c.Chat, msg)
	return err
}

// Edit edits an existing message.
func (c *PluginContext) Edit(msgID types.MessageID, content any, extra ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
	c.StopAutoLoader()
	if c.Client == nil {
		return whatsmeow.SendResponse{}, fmt.Errorf("client unavailable")
	}
	var msg *waE2E.Message
	switch v := content.(type) {
	case string:
		formatted := c.formatTextResponse(v)
		msg = &waE2E.Message{
			Conversation: &formatted,
		}
	case *waE2E.Message:
		msg = v
	default:
		return whatsmeow.SendResponse{}, fmt.Errorf("unsupported content type: %T", content)
	}
	editMsg := c.Client.BuildEdit(c.Chat, msgID, msg)
	return c.Client.SendMessage(c.GetSendContext(), c.Chat, editMsg)
}

// Delete deletes/revokes a message for everyone.
func (c *PluginContext) Delete(msgID types.MessageID, senderJID ...types.JID) (whatsmeow.SendResponse, error) {
	c.StopAutoLoader()
	if c.Client == nil {
		return whatsmeow.SendResponse{}, fmt.Errorf("client unavailable")
	}
	sJID := types.EmptyJID
	if len(senderJID) > 0 {
		sJID = senderJID[0]
	}
	revokeMsg := c.Client.BuildRevoke(c.Chat, sJID, msgID)
	return c.Client.SendMessage(c.GetSendContext(), c.Chat, revokeMsg)
}

// React sends an emoji reaction to the triggering message.
func (c *PluginContext) React(emoji string) error {
	if c.Client == nil {
		return fmt.Errorf("client unavailable")
	}
	targetID := ""
	if c.Evt != nil {
		targetID = c.Evt.Info.ID
	}
	if targetID == "" {
		return fmt.Errorf("no target message for reaction")
	}
	reactionMsg := c.Client.BuildReaction(c.Chat, types.EmptyJID, targetID, emoji)
	_, err := c.Client.SendMessage(c.GetSendContext(), c.Chat, reactionMsg)
	return err
}

// ReactMessage sends an emoji reaction to a specific target message ID.
func (c *PluginContext) ReactMessage(targetID string, emoji string) error {
	if c.Client == nil {
		return fmt.Errorf("client unavailable")
	}
	reactionMsg := c.Client.BuildReaction(c.Chat, types.EmptyJID, targetID, emoji)
	_, err := c.Client.SendMessage(c.GetSendContext(), c.Chat, reactionMsg)
	return err
}

// Text initializes a new TextBuilder bound to this PluginContext.
func (c *PluginContext) Text(initial ...string) *builder.TextBuilder {
	return builder.NewTextWithSender(c, initial...)
}

// NewText initializes a new TextBuilder bound to this PluginContext.
func (c *PluginContext) NewText(initial ...string) *builder.TextBuilder {
	return builder.NewTextWithSender(c, initial...)
}

// Rook returns a WARook builder engine bound to this PluginContext.
func (c *PluginContext) Rook() *builder.WARook {
	return builder.From(c)
}

// Poll initializes a new PollBuilder bound to this PluginContext.
func (c *PluginContext) Poll(question string) *builder.PollBuilder {
	return c.Rook().NewPoll(question)
}

// NewPoll initializes a new PollBuilder bound to this PluginContext.
func (c *PluginContext) NewPoll(question string) *builder.PollBuilder {
	return c.Rook().NewPoll(question)
}

// StartAutoLoader arms a delayed loader that appears in chat with "Please wait"
// if an operation takes longer than delay. If delay <= 0, 1200ms is used.
func (c *PluginContext) StartAutoLoader(delay ...time.Duration) {
	if c == nil || c.Client == nil {
		return
	}
	c.autoLoaderMu.Lock()
	defer c.autoLoaderMu.Unlock()

	if c.loaderStopped || c.loaderTimer != nil || c.autoLoader != nil {
		return
	}

	d := 1200 * time.Millisecond
	if len(delay) > 0 && delay[0] > 0 {
		d = delay[0]
	}

	c.loaderTimer = time.AfterFunc(d, func() {
		c.autoLoaderMu.Lock()
		if c.loaderStopped || c.loaderTimer == nil {
			c.autoLoaderMu.Unlock()
			return
		}
		c.loaderTimer = nil
		c.autoLoaderMu.Unlock()

		l := c.StartLoader("Please wait")

		c.autoLoaderMu.Lock()
		if c.loaderStopped {
			c.autoLoaderMu.Unlock()
			l.Delete()
			return
		}
		c.autoLoader = l
		c.autoLoaderMu.Unlock()
	})
}

// StopAutoLoader disarms any pending timer or stops and deletes an active loader message.
func (c *PluginContext) StopAutoLoader() {
	if c == nil {
		return
	}
	c.autoLoaderMu.Lock()
	c.loaderStopped = true
	if c.loaderTimer != nil {
		c.loaderTimer.Stop()
		c.loaderTimer = nil
	}
	loader := c.autoLoader
	c.autoLoader = nil
	c.autoLoaderMu.Unlock()

	if loader != nil {
		loader.Delete()
	}
}

// GetPrefix returns the configured command prefix, default ".".
func (c *PluginContext) GetPrefix() string {
	if c != nil && c.Client != nil && c.Client.Store != nil && c.Client.Store.Identities != nil {
		if s, ok := c.Client.Store.Identities.(interface {
			GetSetting(ctx context.Context, key string) (string, error)
		}); ok {
			if val, err := s.GetSetting(c.GetSendContext(), "prefix"); err == nil && val != "" {
				return val
			}
		}
	}
	return "."
}

// GetBotName returns the configured bot display name, default "WhatsRook".
func (c *PluginContext) GetBotName() string {
	if c != nil && c.Client != nil && c.Client.Store != nil && c.Client.Store.Identities != nil {
		if s, ok := c.Client.Store.Identities.(interface {
			GetSetting(ctx context.Context, key string) (string, error)
		}); ok {
			if val, err := s.GetSetting(c.GetSendContext(), "bot_name"); err == nil && val != "" {
				return val
			}
		}
	}
	return "WhatsRook"
}

// IsOwner returns true if the sender is the primary bot owner.
func (c *PluginContext) IsOwner() bool {
	if c == nil || c.Client == nil || c.Client.Store == nil || c.Client.Store.ID == nil {
		return false
	}
	return IsSameUserRaw(c.GetSendContext(), c.Client, c.Sender, *c.Client.Store.ID)
}

// IsSudo returns true if the sender is a sudo user or bot owner.
func (c *PluginContext) IsSudo() bool {
	if c == nil || c.Client == nil {
		return false
	}
	return IsSudoRaw(c.GetSendContext(), c.Client, c.Sender)
}

// GetQuotedMessage returns the quoted message proto if the triggering message is a reply.
func (c *PluginContext) GetQuotedMessage() *waE2E.Message {
	if c == nil || c.Evt == nil || c.Evt.Message == nil {
		return nil
	}
	ci := GetContextInfoFromProto(c.Evt.Message)
	if ci != nil && ci.QuotedMessage != nil {
		return UnwrapMessageProto(ci.QuotedMessage)
	}
	return nil
}

// GetQuotedSender returns the quoted message sender JID.
func (c *PluginContext) GetQuotedSender() (types.JID, bool) {
	if c == nil || c.Evt == nil || c.Evt.Message == nil {
		return types.EmptyJID, false
	}
	ci := GetContextInfoFromProto(c.Evt.Message)
	if ci != nil && ci.Participant != nil && *ci.Participant != "" {
		if parsed, err := types.ParseJID(*ci.Participant); err == nil {
			return parsed, true
		}
	}
	return types.EmptyJID, false
}

// GetContextInfo returns the ContextInfo from the triggering message.
func (c *PluginContext) GetContextInfo() *waE2E.ContextInfo {
	if c == nil || c.Evt == nil || c.Evt.Message == nil {
		return nil
	}
	return GetContextInfoFromProto(c.Evt.Message)
}

// GetMentionedJIDs returns any mentioned JIDs in the triggering message context.
func (c *PluginContext) GetMentionedJIDs() []types.JID {
	ci := c.GetContextInfo()
	if ci == nil || len(ci.MentionedJID) == 0 {
		return nil
	}
	var res []types.JID
	for _, m := range ci.MentionedJID {
		if parsed, err := types.ParseJID(m); err == nil {
			res = append(res, parsed)
		}
	}
	return res
}

// IsSenderAdmin checks if the sender has admin privileges in the provided groupInfo.
func (c *PluginContext) IsSenderAdmin(groupInfo *types.GroupInfo) bool {
	if c == nil || c.Client == nil || groupInfo == nil {
		return false
	}
	return IsAdminRaw(c.GetSendContext(), c.Client, groupInfo, c.Sender)
}

// ResolveMention resolves a JID to its normalized non-AD JID and username.
func (c *PluginContext) ResolveMention(jid types.JID) (types.JID, string) {
	return ResolveMentionRaw(c.GetSendContext(), c.Client, jid)
}

// FormatMention returns "@username" string and the resolved JID.
func (c *PluginContext) FormatMention(jid types.JID) (string, types.JID) {
	resolved, username := c.ResolveMention(jid)
	return "@" + username, resolved
}

// ResolvePN returns the normalized non-AD phone number JID.
func (c *PluginContext) ResolvePN(jid types.JID) types.JID {
	return jid.ToNonAD()
}

// IsSameUser compares two JIDs ignoring device suffixes.
func (c *PluginContext) IsSameUser(a, b types.JID) bool {
	return IsSameUserRaw(c.GetSendContext(), c.Client, a, b)
}

// IsTargetSudo checks if a target JID is a sudo user or owner.
func (c *PluginContext) IsTargetSudo(target types.JID) bool {
	if c == nil || c.Client == nil {
		return false
	}
	return IsSudoRaw(c.GetSendContext(), c.Client, target)
}

// IsTargetOwner checks if a target JID is the bot owner.
func (c *PluginContext) IsTargetOwner(target types.JID) bool {
	if c == nil || c.Client == nil || c.Client.Store == nil || c.Client.Store.ID == nil {
		return false
	}
	return c.IsSameUser(target, *c.Client.Store.ID)
}

// GetTargets resolves target user JIDs from quoted reply, mentions, or arguments.
func (c *PluginContext) GetTargets() []types.JID {
	if c == nil {
		return nil
	}
	if q, ok := c.GetQuotedSender(); ok && !q.IsEmpty() {
		return []types.JID{q.ToNonAD()}
	}
	if m := c.GetMentionedJIDs(); len(m) > 0 {
		var resolved []types.JID
		for _, j := range m {
			if !j.IsEmpty() {
				resolved = append(resolved, j.ToNonAD())
			}
		}
		if len(resolved) > 0 {
			return resolved
		}
	}
	if len(c.Args) > 0 {
		var resolved []types.JID
		for _, arg := range c.Args {
			clean := strings.TrimLeft(arg, "@+")
			if len(clean) >= 5 {
				resolved = append(resolved, types.NewJID(clean, types.DefaultUserServer))
			}
		}
		if len(resolved) > 0 {
			return resolved
		}
	}
	if !c.Chat.IsEmpty() && c.Chat.Server != "g.us" {
		if c.Client != nil && c.Client.Store != nil && c.Client.Store.ID != nil {
			if !c.IsSameUser(c.Chat, *c.Client.Store.ID) {
				return []types.JID{c.Chat.ToNonAD()}
			}
		} else {
			return []types.JID{c.Chat.ToNonAD()}
		}
	}
	return nil
}

// GetMedia downloads media bytes and mimetype from the triggering message or quoted message.
func (c *PluginContext) GetMedia() ([]byte, string, error) {
	if c.Client == nil {
		return nil, "", fmt.Errorf("client unavailable")
	}
	extract := func(msg *waE2E.Message) ([]byte, string, bool) {
		msg = UnwrapMessageProto(msg)
		if msg == nil {
			return nil, "", false
		}
		var downloadable whatsmeow.DownloadableMessage
		var mime string
		if img := msg.GetImageMessage(); img != nil {
			downloadable = img
			mime = img.GetMimetype()
		} else if vid := msg.GetVideoMessage(); vid != nil {
			downloadable = vid
			mime = vid.GetMimetype()
		} else if aud := msg.GetAudioMessage(); aud != nil {
			downloadable = aud
			mime = aud.GetMimetype()
		} else if doc := msg.GetDocumentMessage(); doc != nil {
			downloadable = doc
			mime = doc.GetMimetype()
		} else if stk := msg.GetStickerMessage(); stk != nil {
			downloadable = stk
			mime = stk.GetMimetype()
		}
		if downloadable == nil {
			return nil, "", false
		}
		data, err := c.Client.Download(c.GetSendContext(), downloadable)
		if err != nil {
			return nil, "", false
		}
		return data, mime, true
	}

	if c.Evt != nil && c.Evt.Message != nil {
		if data, mime, ok := extract(c.Evt.Message); ok {
			return data, mime, nil
		}
	}
	if quoted := c.GetQuotedMessage(); quoted != nil {
		if data, mime, ok := extract(quoted); ok {
			return data, mime, nil
		}
	}
	return nil, "", fmt.Errorf("no media found")
}

func protoUint64(u uint64) *uint64 { return &u }

// IsAdmin checks if a specific JID is a group admin.
func (c *PluginContext) IsAdmin(info *types.GroupInfo, jid types.JID) bool {
	if info == nil {
		return false
	}
	target := jid.ToNonAD()
	for _, p := range info.Participants {
		if p.JID.ToNonAD() == target {
			return p.IsAdmin || p.IsSuperAdmin
		}
	}
	return false
}

// AmIAdmin checks if the bot itself is an admin in the group.
func (c *PluginContext) AmIAdmin(info *types.GroupInfo) bool {
	if c.Client == nil || c.Client.Store == nil || c.Client.Store.ID == nil {
		return false
	}
	return c.IsAdmin(info, *c.Client.Store.ID)
}

// Sprintf formats text according to format specifier.
func Sprintf(format string, args ...any) string {
	return fmt.Sprintf(format, args...)
}

// IsViewOnceMessage returns true if the message is a ViewOnce container or contains a ViewOnce media attachment.
func IsViewOnceMessage(msg *waE2E.Message) bool {
	if msg == nil {
		return false
	}
	if msg.EphemeralMessage != nil && msg.EphemeralMessage.Message != nil {
		return IsViewOnceMessage(msg.EphemeralMessage.Message)
	}
	if msg.ViewOnceMessage != nil || msg.ViewOnceMessageV2 != nil || msg.ViewOnceMessageV2Extension != nil {
		return true
	}
	if img := msg.GetImageMessage(); img != nil && img.GetViewOnce() {
		return true
	}
	if vid := msg.GetVideoMessage(); vid != nil && vid.GetViewOnce() {
		return true
	}
	if aud := msg.GetAudioMessage(); aud != nil && aud.GetViewOnce() {
		return true
	}
	return false
}

// ExtractViewOnceMessage extracts and returns the inner media message from any ViewOnce wrapper.
func ExtractViewOnceMessage(msg *waE2E.Message) *waE2E.Message {
	msg = UnwrapMessageProto(msg)
	if msg == nil {
		return nil
	}
	res := &waE2E.Message{}
	if img := msg.GetImageMessage(); img != nil {
		cloned := proto.Clone(img).(*waE2E.ImageMessage)
		cloned.ViewOnce = protoBool(false)
		res.ImageMessage = cloned
		return res
	}
	if vid := msg.GetVideoMessage(); vid != nil {
		cloned := proto.Clone(vid).(*waE2E.VideoMessage)
		cloned.ViewOnce = protoBool(false)
		res.VideoMessage = cloned
		return res
	}
	if aud := msg.GetAudioMessage(); aud != nil {
		cloned := proto.Clone(aud).(*waE2E.AudioMessage)
		cloned.ViewOnce = protoBool(false)
		res.AudioMessage = cloned
		return res
	}
	return msg
}

// UnwrapAndSendViewOnceMessage downloads encrypted ViewOnce media, re-uploads it with fresh keys, and sends the clean unwrapped message to target JID.
func UnwrapAndSendViewOnceMessage(ctx context.Context, client *whatsmeow.Client, msg *waE2E.Message, senderJID types.JID, pushName string, targetJID types.JID, quoteID string, sourceChat ...types.JID) error {
	if msg == nil || client == nil {
		return fmt.Errorf("invalid arguments")
	}

	unwrapped := ExtractViewOnceMessage(msg)
	if unwrapped == nil {
		return fmt.Errorf("failed to extract inner ViewOnce message")
	}

	if img := unwrapped.GetImageMessage(); img != nil {
		data, err := client.Download(ctx, img)
		if err != nil {
			return fmt.Errorf("download image: %w", err)
		}
		uploaded, err := client.Upload(ctx, data, whatsmeow.MediaImage)
		if err != nil {
			return fmt.Errorf("upload image: %w", err)
		}
		img.URL = &uploaded.URL
		img.DirectPath = &uploaded.DirectPath
		img.MediaKey = uploaded.MediaKey
		img.FileEncSHA256 = uploaded.FileEncSHA256
		img.FileSHA256 = uploaded.FileSHA256
		img.FileLength = protoUint64(uint64(len(data)))
		img.ViewOnce = protoBool(false)
	} else if vid := unwrapped.GetVideoMessage(); vid != nil {
		data, err := client.Download(ctx, vid)
		if err != nil {
			return fmt.Errorf("download video: %w", err)
		}
		uploaded, err := client.Upload(ctx, data, whatsmeow.MediaVideo)
		if err != nil {
			return fmt.Errorf("upload video: %w", err)
		}
		vid.URL = &uploaded.URL
		vid.DirectPath = &uploaded.DirectPath
		vid.MediaKey = uploaded.MediaKey
		vid.FileEncSHA256 = uploaded.FileEncSHA256
		vid.FileSHA256 = uploaded.FileSHA256
		vid.FileLength = protoUint64(uint64(len(data)))
		vid.ViewOnce = protoBool(false)
	} else if aud := unwrapped.GetAudioMessage(); aud != nil {
		data, err := client.Download(ctx, aud)
		if err != nil {
			return fmt.Errorf("download audio: %w", err)
		}
		uploaded, err := client.Upload(ctx, data, whatsmeow.MediaAudio)
		if err != nil {
			return fmt.Errorf("upload audio: %w", err)
		}
		aud.URL = &uploaded.URL
		aud.DirectPath = &uploaded.DirectPath
		aud.MediaKey = uploaded.MediaKey
		aud.FileEncSHA256 = uploaded.FileEncSHA256
		aud.FileSHA256 = uploaded.FileSHA256
		aud.FileLength = protoUint64(uint64(len(data)))
		aud.ViewOnce = protoBool(false)
	}

	_, err := client.SendMessage(ctx, targetJID, unwrapped)
	return err
}

// FormatTextResponseRaw applies optional global font styles or formatting transformations.
func FormatTextResponseRaw(text string) string {
	return text
}

// EncodeProtoMessage serializes a protobuf message into a hex-encoded string.
func EncodeProtoMessage(msg proto.Message) (string, error) {
	data, err := proto.Marshal(msg)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}

// DecodeProtoMessage decodes a hex-encoded or base64-encoded protobuf message.
func DecodeProtoMessage(encoded string) (*waE2E.Message, error) {
	data, err := hex.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		var b64Err error
		data, b64Err = base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
		if b64Err != nil {
			return nil, fmt.Errorf("decode proto hex: %v, b64: %v", err, b64Err)
		}
	}
	msg := &waE2E.Message{}
	if err := proto.Unmarshal(data, msg); err != nil {
		return nil, err
	}
	return msg, nil
}

// DispatchListSelection resolves an interactive list button response.
func DispatchListSelection(ctx any, text, displayText string) bool {
	return false
}

// DispatchPollVoteEvent routes incoming poll votes to reactive action handlers.
func DispatchPollVoteEvent(ctx any, evt *events.Message) bool {
	return false
}

// ─── Convenience Subpackage Re-exports ─────────────────────────────────────

var (
	// NewMemoryStore initializes a memory cache store.
	NewMemoryStore = cache.NewMemoryStore
	// InitCache initializes global caching.
	InitCache = cache.Init
	// NewWaLogger constructs a Zap protocol logger adapter.
	NewWaLogger = Logger.NewWaLogger
	// GetSystemStats retrieves host hardware metrics.
	GetSystemStats = system.GetStats
	// FormatBytes formats byte counts.
	FormatBytes = system.FormatBytes
	// AddStickerMetadata injects EXIF metadata into WebP stickers.
	AddStickerMetadata = webp.AddStickerMetadata
	// WriteStickerMetadata writes EXIF metadata to a WebP file.
	WriteStickerMetadata = webp.WriteStickerMetadata
	// EncodePNG generates a QR code PNG image.
	EncodePNG = qr.EncodePNG
	// StartQRServer starts the local QR web pairing server.
	StartQRServer = qr.StartServer
)
