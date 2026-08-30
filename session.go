package whatsrook

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types/events"
)

// StoredSession describes an existing WhatsApp device session found in the store.
type StoredSession struct {
	JID      string
	User     string // phone number (without plus)
	PushName string
	Platform string
	Business bool
}

// ListStoredSessions queries the database store for all saved device sessions.
func ListStoredSessions(ctx context.Context, dataDir, database string) ([]StoredSession, error) {
	if dataDir == "" {
		dataDir = DefaultDataDir()
	}
	dbPath := filepath.Join(dataDir, "whatsrook.db")

	dummy := &Client{
		Config: Config{
			DataDir:  dataDir,
			Database: database,
		},
	}
	container, err := dummy.initStore(ctx, dbPath, "ERROR")
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

// DeleteStoredSession initiates a server-side logout request to WhatsApp to unpair/revoke the companion device,
// and deletes all local session keys and device records from the database.
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

		if err := cli.Connect(); err == nil {
			select {
			case <-connected:
			case <-time.After(3 * time.Second):
			case <-initCtx.Done():
			}

			logoutCtx, logoutCancel := context.WithTimeout(ctx, 5*time.Second)
			_ = cli.Logout(logoutCtx)
			logoutCancel()
			cli.Disconnect()
		}
	}

	client.ClearSessionDB(ctx, "")
	return nil
}

func fallbackDeleteDevice(ctx context.Context, dataDir, database, phone string) error {
	dbPath := filepath.Join(dataDir, "whatsrook.db")
	dummy := &Client{
		Config: Config{
			DataDir:  dataDir,
			Database: database,
			Session:  phone,
		},
	}
	container, err := dummy.initStore(ctx, dbPath, "ERROR")
	if err != nil {
		return err
	}
	defer func() {
		_ = container.Close()
	}()

	devices, err := container.GetAllDevices(ctx)
	if err != nil {
		return err
	}

	cleanPhone := strings.TrimPrefix(phone, "+")
	for _, dev := range devices {
		if dev.ID != nil && strings.HasPrefix(dev.ID.User, cleanPhone) {
			return container.DeleteDevice(ctx, dev)
		}
	}
	return fmt.Errorf("session %q not found in store", phone)
}
