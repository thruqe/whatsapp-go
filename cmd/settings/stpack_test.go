package settings_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"whatsrook/cmd/dispatch"
	_ "whatsrook/cmd/owner"
	"whatsrook/cmd/settings"
	clistore "whatsrook/cmd/store"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	meowstore "go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
)

func TestCommandsRegistration(t *testing.T) {
	// 1. stpack must be registered
	cmd, ok := dispatch.Get("stpack")
	if !ok || cmd == nil {
		t.Fatalf("expected 'stpack' command to be registered in dispatch")
	}

	// 2. update, upgrade, setpack, setauthor, channels, newsletters must NOT be registered
	for _, removed := range []string{"update", "upgrade", "setpack", "setauthor", "channels", "newsletters"} {
		if _, exists := dispatch.Get(removed); exists {
			t.Fatalf("expected command %q to be removed from dispatch, but it was found", removed)
		}
	}
}

func TestStpack_InteractiveFlow(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	if err := clistore.RunMigrations(ctx, db); err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}

	testPhone := fmt.Sprintf("1555%07d", time.Now().UnixNano()%10000000)
	ownerPN := types.NewJID(testPhone, types.DefaultUserServer)

	container := sqlstore.NewWithDB(db.RawDB, "postgres", nil)
	sqStore := sqlstore.NewSQLStore(container, ownerPN)

	devStore := &meowstore.Device{
		ID: &ownerPN,
	}

	cli := &whatsmeow.Client{
		Store: devStore, Log: waLog.Noop,
	}
	cli.Store.Identities = sqStore

	// Set OS environment owner
	os.Setenv("SESSION", testPhone)

	stpackCmd, ok := dispatch.Get("stpack")
	if !ok {
		t.Fatalf("expected stpack command to be registered")
	}

	makeMsgCtx := func(text string) *dispatch.Context {
		return &dispatch.Context{
			Ctx:     ctx,
			Client:  cli,
			Chat:    ownerPN,
			Sender:  ownerPN,
			Command: "stpack",
			Evt: &events.Message{
				Info: types.MessageInfo{
					Chat:     ownerPN,
					Sender:   ownerPN,
					IsFromMe: true,
					ID:       "EVT_STPACK_" + text,
				},
				Message: &waE2E.Message{
					Conversation: &text,
				},
			},
		}
	}

	// 1. Execute stpack command
	initCtx := makeMsgCtx(".stpack")
	if err := stpackCmd.Handler(initCtx); err != nil {
		t.Fatalf("stpack handler failed: %v", err)
	}

	// 2. Step 1: User sends author name
	msgCtxAuthor := makeMsgCtx("Alice")
	handledStep1 := settings.HandlePendingStpackInput(msgCtxAuthor, "Alice")
	if !handledStep1 {
		t.Fatalf("expected HandlePendingStpackInput to handle step 1 (author input)")
	}

	// 3. Step 2: User sends pack name
	msgCtxPack := makeMsgCtx("AlicePack")
	handledStep2 := settings.HandlePendingStpackInput(msgCtxPack, "AlicePack")
	if !handledStep2 {
		t.Fatalf("expected HandlePendingStpackInput to handle step 2 (pack input)")
	}

	// 4. Verify settings saved in database
	author, err := clistore.GetSetting(ctx, sqStore, "sticker_author")
	if err != nil || author != "Alice" {
		t.Fatalf("expected sticker_author='Alice', got %q (err: %v)", author, err)
	}

	pack, err := clistore.GetSetting(ctx, sqStore, "sticker_pack")
	if err != nil || pack != "AlicePack" {
		t.Fatalf("expected sticker_pack='AlicePack', got %q (err: %v)", pack, err)
	}

	// 5. Verify session is cleared after completion
	msgCtxAfterDone := makeMsgCtx("Hello")
	handledAfterDone := settings.HandlePendingStpackInput(msgCtxAfterDone, "Hello")
	if handledAfterDone {
		t.Fatalf("expected HandlePendingStpackInput to return false after session completed")
	}
}

func TestStpack_CancelFlow(t *testing.T) {
	ctx := context.Background()
	testPhone := fmt.Sprintf("1555%07d", time.Now().UnixNano()%10000000)
	ownerPN := types.NewJID(testPhone, types.DefaultUserServer)

	devStore := &meowstore.Device{
		ID: &ownerPN,
	}
	cli := &whatsmeow.Client{
		Store: devStore, Log: waLog.Noop,
	}

	os.Setenv("SESSION", testPhone)

	makeMsgCtx := func(text string) *dispatch.Context {
		return &dispatch.Context{
			Ctx:     ctx,
			Client:  cli,
			Chat:    ownerPN,
			Sender:  ownerPN,
			Command: "stpack",
			Evt: &events.Message{
				Info: types.MessageInfo{
					Chat:     ownerPN,
					Sender:   ownerPN,
					IsFromMe: true,
					ID:       "EVT_STPACK_CANCEL_" + text,
				},
				Message: &waE2E.Message{
					Conversation: &text,
				},
			},
		}
	}

	key := ownerPN.ToNonAD().String() + ":" + ownerPN.ToNonAD().String()
	settings.StpackSessionMu.Lock()
	settings.PendingStpackSessions[key] = settings.StpackSession{
		Step:      1,
		UpdatedAt: time.Now(),
	}
	settings.StpackSessionMu.Unlock()

	// Send cancel
	cancelCtx := makeMsgCtx("cancel")
	handled := settings.HandlePendingStpackInput(cancelCtx, "cancel")
	if !handled {
		t.Fatalf("expected HandlePendingStpackInput to intercept and cancel")
	}

	// Verify session is deleted
	settings.StpackSessionMu.RLock()
	_, exists := settings.PendingStpackSessions[key]
	settings.StpackSessionMu.RUnlock()
	if exists {
		t.Fatalf("expected session to be deleted after cancel")
	}
}

func TestStpack_StateTransitions(t *testing.T) {
	ctx := context.Background()
	testPhone := fmt.Sprintf("1555%07d", time.Now().UnixNano()%10000000)
	ownerPN := types.NewJID(testPhone, types.DefaultUserServer)

	devStore := &meowstore.Device{
		ID: &ownerPN,
	}
	cli := &whatsmeow.Client{
		Store: devStore, Log: waLog.Noop,
	}

	os.Setenv("SESSION", testPhone)

	makeMsgCtx := func(text string) *dispatch.Context {
		return &dispatch.Context{
			Ctx:     ctx,
			Client:  cli,
			Chat:    ownerPN,
			Sender:  ownerPN,
			Command: "stpack",
			Evt: &events.Message{
				Info: types.MessageInfo{
					Chat:     ownerPN,
					Sender:   ownerPN,
					IsFromMe: true,
					ID:       "EVT_STPACK_TRANS_" + text,
				},
				Message: &waE2E.Message{
					Conversation: &text,
				},
			},
		}
	}

	key := ownerPN.ToNonAD().String() + ":" + ownerPN.ToNonAD().String()

	// 1. Initialize session in Step 1 (Awaiting Author)
	settings.StpackSessionMu.Lock()
	settings.PendingStpackSessions[key] = settings.StpackSession{
		Step:      1,
		UpdatedAt: time.Now(),
	}
	settings.StpackSessionMu.Unlock()

	// 2. User sends author name
	authorCtx := makeMsgCtx("TestAuthor")
	handled1 := settings.HandlePendingStpackInput(authorCtx, "TestAuthor")
	if !handled1 {
		t.Fatalf("expected step 1 to be handled")
	}

	// Verify session transitioned to Step 2 with Author recorded
	settings.StpackSessionMu.RLock()
	session, exists := settings.PendingStpackSessions[key]
	settings.StpackSessionMu.RUnlock()
	if !exists {
		t.Fatalf("expected session to exist after step 1")
	}
	if session.Step != 2 {
		t.Fatalf("expected session.Step == 2, got %d", session.Step)
	}
	if session.Author != "TestAuthor" {
		t.Fatalf("expected session.Author == 'TestAuthor', got %q", session.Author)
	}

	// 3. User sends pack name
	packCtx := makeMsgCtx("TestPack")
	handled2 := settings.HandlePendingStpackInput(packCtx, "TestPack")
	if !handled2 {
		t.Fatalf("expected step 2 to be handled")
	}

	// Verify session is cleared after step 2
	settings.StpackSessionMu.RLock()
	_, exists = settings.PendingStpackSessions[key]
	settings.StpackSessionMu.RUnlock()
	if exists {
		t.Fatalf("expected session to be cleared after step 2 completion")
	}
}

func TestStpack_CommandInterruption(t *testing.T) {
	ctx := context.Background()
	testPhone := fmt.Sprintf("1555%07d", time.Now().UnixNano()%10000000)
	ownerPN := types.NewJID(testPhone, types.DefaultUserServer)

	devStore := &meowstore.Device{
		ID: &ownerPN,
	}
	cli := &whatsmeow.Client{
		Store: devStore, Log: waLog.Noop,
	}

	os.Setenv("SESSION", testPhone)

	makeMsgCtx := func(text string) *dispatch.Context {
		return &dispatch.Context{
			Ctx:     ctx,
			Client:  cli,
			Chat:    ownerPN,
			Sender:  ownerPN,
			Command: "stpack",
			Evt: &events.Message{
				Info: types.MessageInfo{
					Chat:     ownerPN,
					Sender:   ownerPN,
					IsFromMe: true,
					ID:       "EVT_STPACK_CMD_" + text,
				},
				Message: &waE2E.Message{
					Conversation: &text,
				},
			},
		}
	}

	key := ownerPN.ToNonAD().String() + ":" + ownerPN.ToNonAD().String()

	// Initialize session
	settings.StpackSessionMu.Lock()
	settings.PendingStpackSessions[key] = settings.StpackSession{
		Step:      1,
		UpdatedAt: time.Now(),
	}
	settings.StpackSessionMu.Unlock()

	// User types a command starting with prefix (e.g. ".ping") -> should clear session and return false
	cmdCtx := makeMsgCtx(".ping")
	handled := settings.HandlePendingStpackInput(cmdCtx, ".ping")
	if handled {
		t.Fatalf("expected HandlePendingStpackInput to return false for command with prefix")
	}

	// Verify session was cleared
	settings.StpackSessionMu.RLock()
	_, exists := settings.PendingStpackSessions[key]
	settings.StpackSessionMu.RUnlock()
	if exists {
		t.Fatalf("expected session to be cleared when command is sent")
	}
}
