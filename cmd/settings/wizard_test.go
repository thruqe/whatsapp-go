package settings_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"whatsrook/cmd/dispatch"
	"whatsrook/cmd/settings"
	clistore "whatsrook/cmd/store"

	_ "github.com/lib/pq"
	"go.mau.fi/util/dbutil"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
)

func openTestDB(t *testing.T) *dbutil.Database {
	t.Helper()
	pgURL := os.Getenv("DATABASE_URL")
	if pgURL == "" {
		pgURL = os.Getenv("POSTGRES_TEST_URL")
	}
	if pgURL == "" {
		t.Skip("skipping PostgreSQL store test: DATABASE_URL not configured")
		return nil
	}
	if !strings.Contains(pgURL, "sslmode=") {
		if strings.Contains(pgURL, "?") {
			pgURL += "&sslmode=disable"
		} else {
			pgURL += "?sslmode=disable"
		}
	}
	rawDB, err := sql.Open("postgres", pgURL)
	if err != nil {
		t.Fatalf("failed opening test PostgreSQL DB: %v", err)
	}
	if err := rawDB.Ping(); err != nil {
		t.Fatalf("failed pinging test PostgreSQL DB: %v", err)
	}

	schemaName := fmt.Sprintf("test_wizard_schema_%d", time.Now().UnixNano())
	if _, err := rawDB.Exec("CREATE SCHEMA " + schemaName); err != nil {
		t.Fatalf("failed creating test schema %s: %v", schemaName, err)
	}
	if _, err := rawDB.Exec("SET search_path TO " + schemaName); err != nil {
		t.Fatalf("failed setting search_path to %s: %v", schemaName, err)
	}

	db, err := dbutil.NewWithDB(rawDB, "postgres")
	if err != nil {
		t.Fatalf("failed wrapping db with dbutil: %v", err)
	}
	t.Cleanup(func() {
		_, _ = rawDB.Exec("SET search_path TO public")
		_, _ = rawDB.Exec("DROP SCHEMA " + schemaName + " CASCADE")
		_ = rawDB.Close()
	})
	return db
}

func TestWizard_ConfigLifecycle(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	if err := clistore.RunMigrations(ctx, db); err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}

	testPhone := fmt.Sprintf("1555%07d", time.Now().UnixNano()%10000000)
	ownerPN := types.NewJID(testPhone, types.DefaultUserServer)

	container := sqlstore.NewWithDB(db.RawDB, "postgres", nil)
	sqStore := sqlstore.NewSQLStore(container, ownerPN)

	devStore := &store.Device{
		ID: &ownerPN,
	}

	cli := &whatsmeow.Client{
		Store: devStore, Log: waLog.Noop,
	}
	cli.Store.Identities = sqStore

	// Set OS environment owner
	os.Setenv("SESSION", testPhone)

	makeMsg := func(text string) *events.Message {
		return &events.Message{
			Info: types.MessageInfo{
				Chat:     ownerPN,
				Sender:   ownerPN,
				IsFromMe: true,
				ID:       "MSG_" + text,
			},
			Message: &waE2E.Message{
				Conversation: &text,
			},
		}
	}

	// 1. Initially, wizard_config is empty (!= "done").
	// Sending any message (e.g. ".ping") should intercept and return true (showing Step 1).
	intercepted := settings.HandlePendingBotCustomizationReply(ctx, cli, makeMsg(".ping"))
	if !intercepted {
		t.Fatalf("expected HandlePendingBotCustomizationReply to intercept when wizard_config != 'done'")
	}

	// Check wizard_config in database
	wc, err := clistore.GetSetting(ctx, sqStore, "wizard_config")
	if err != nil || wc != "name" {
		t.Fatalf("expected wizard_config to be 'name', got %q (err: %v)", wc, err)
	}

	// 2. User sends bot name "WhatsRook"
	interceptedName := settings.HandlePendingBotCustomizationReply(ctx, cli, makeMsg("WhatsRook"))
	if !interceptedName {
		t.Fatalf("expected HandlePendingBotCustomizationReply to intercept step 1 name input")
	}

	// Name should be saved and step advanced to thumb
	botName, err := clistore.GetSetting(ctx, sqStore, settings.BotNameSettingKey)
	if err != nil || botName != "WhatsRook" {
		t.Fatalf("expected bot_name to be 'WhatsRook', got %q", botName)
	}
	wc, _ = clistore.GetSetting(ctx, sqStore, "wizard_config")
	if wc != "thumb" {
		t.Fatalf("expected wizard_config to be 'thumb', got %q", wc)
	}

	// 3. User skips thumbnail step (setbot skip 2)
	skipEvt := makeMsg(".setbot skip 2")
	okSkip := dispatch.RunCommandPublicly(ctx, cli, skipEvt, "setbot skip 2")
	if !okSkip {
		t.Fatalf("expected setbot skip 2 to execute successfully")
	}
	var wcPrefix string
	for range 20 {
		wcPrefix, _ = clistore.GetSetting(ctx, sqStore, "wizard_config")
		if wcPrefix == "prefix" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if wcPrefix != "prefix" {
		t.Fatalf("expected wizard_config to be 'prefix' after skip, got %q", wcPrefix)
	}

	// 4. User provides prefix "!"
	interceptedPrefix := settings.HandlePendingBotCustomizationReply(ctx, cli, makeMsg("!"))
	if !interceptedPrefix {
		t.Fatalf("expected HandlePendingBotCustomizationReply to intercept step 3 prefix input")
	}
	pref, _ := clistore.GetSetting(ctx, sqStore, settings.PrefixSettingKey)
	if pref != "!" {
		t.Fatalf("expected prefix to be '!', got %q", pref)
	}
	wc, _ = clistore.GetSetting(ctx, sqStore, "wizard_config")
	if wc != "bio" {
		t.Fatalf("expected wizard_config to be 'bio', got %q", wc)
	}

	// 5. User provides bio "Hello World"
	interceptedBio := settings.HandlePendingBotCustomizationReply(ctx, cli, makeMsg("Hello World"))
	if !interceptedBio {
		t.Fatalf("expected HandlePendingBotCustomizationReply to intercept step 4 bio input")
	}

	// Wizard should now be marked "done"
	wc, _ = clistore.GetSetting(ctx, sqStore, "wizard_config")
	if wc != "done" {
		t.Fatalf("expected wizard_config to be 'done' after completion, got %q", wc)
	}

	// 6. Now that wizard_config == "done", sending a message should NOT intercept (return false)
	interceptedAfterDone := settings.HandlePendingBotCustomizationReply(ctx, cli, makeMsg(".ping"))
	if interceptedAfterDone {
		t.Fatalf("expected HandlePendingBotCustomizationReply to return false when wizard_config == 'done'")
	}
}
