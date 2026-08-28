package whatsrook

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
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

// DeleteStoredSession removes a specific device session from the database store.
func DeleteStoredSession(ctx context.Context, dataDir, database, phone string) error {
	if dataDir == "" {
		dataDir = DefaultDataDir()
	}
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
