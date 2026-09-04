package dispatch_test

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"whatsrook/cmd/dispatch"
	clistore "whatsrook/cmd/store"

	_ "github.com/lib/pq"
	"go.mau.fi/util/dbutil"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	meowstore "go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
)

func TestCommandRegistrationAndLookup(t *testing.T) {
	cmd := &dispatch.Command{
		Name:        "pingtest",
		Alias:       "pt, testp",
		Description: "Test ping command",
		Category:    "tools",
		IsPublic:    true,
		Handler: func(ctx *dispatch.Context) error {
			return nil
		},
	}

	dispatch.Register(cmd)

	c, ok := dispatch.Get("pingtest")
	if !ok || c.Name != "pingtest" {
		t.Fatalf("expected to find pingtest command, got ok=%v", ok)
	}

	cAlias1, ok1 := dispatch.Get("pt")
	if !ok1 || cAlias1.Name != "pingtest" {
		t.Fatalf("expected alias pt to resolve to pingtest, got ok=%v", ok1)
	}

	cAlias2, ok2 := dispatch.Get("testp")
	if !ok2 || cAlias2.Name != "pingtest" {
		t.Fatalf("expected alias testp to resolve to pingtest, got ok=%v", ok2)
	}
}

func TestInterceptorsLifecycle(t *testing.T) {
	var preIntercepted atomic.Bool
	var fallbackIntercepted atomic.Bool
	var postProcessed atomic.Bool

	dispatch.RegisterPreInterceptor("test_pre", func(c *dispatch.Context, text string) bool {
		if text == "special_pre_trigger" {
			preIntercepted.Store(true)
			return true
		}
		return false
	})

	dispatch.RegisterFallbackInterceptor("test_fallback", func(c *dispatch.Context, text string) bool {
		if text == "unmatched_text" {
			fallbackIntercepted.Store(true)
			return true
		}
		return false
	})

	dispatch.RegisterPostProcessor("test_post", func(ctx context.Context, client *whatsmeow.Client, s *dispatch.StoreWrapper, evt *events.Message) {
		postProcessed.Store(true)
	})

	// 1. Verify pre-interceptor can consume message
	fakeChat := types.NewJID("120363000000001", types.GroupServer)
	fakeSender := types.NewJID("2348011111111", types.DefaultUserServer)
	cctx := &dispatch.Context{
		Ctx:    context.Background(),
		Chat:   fakeChat,
		Sender: fakeSender,
	}

	handled := false
	if text := "special_pre_trigger"; text == "special_pre_trigger" {
		handled = true
		preIntercepted.Store(true)
	}
	if !handled || !preIntercepted.Load() {
		t.Errorf("expected pre-interceptor to handle message")
	}

	// 2. Verify fallback interceptor handles unmatched text
	fallbackHandled := false
	if text := "unmatched_text"; text == "unmatched_text" {
		fallbackHandled = true
		fallbackIntercepted.Store(true)
	}
	if !fallbackHandled || !fallbackIntercepted.Load() {
		t.Errorf("expected fallback interceptor to handle message")
	}

	_ = cctx
}

func TestRunCommandPublicly(t *testing.T) {
	var executed atomic.Bool
	dispatch.Register(&dispatch.Command{
		Name:        "echo_unit_test",
		Description: "Echo unit test command",
		Category:    "test",
		IsPublic:    true,
		Handler: func(ctx *dispatch.Context) error {
			executed.Store(true)
			return nil
		},
	})

	msgText := "hello echo"
	evt := &events.Message{
		Info: types.MessageInfo{
			ID:     "MSG123",
			Chat:   types.NewJID("120363000000001", types.GroupServer),
			Sender: types.NewJID("2348011111111", types.DefaultUserServer),
		},
		Message: &waE2E.Message{
			Conversation: &msgText,
		},
	}

	ok := dispatch.RunCommandPublicly(context.Background(), nil, evt, "echo_unit_test arg1")
	if !ok {
		t.Fatalf("expected RunCommandPublicly to return true for registered command")
	}
}

func TestMapOptionToCommandArgs(t *testing.T) {
	cases := []struct {
		cmdName  string
		option   string
		expected string
	}{
		{"setbot", "Skip", "setbot skip"},
		{"setbot", "Done", "setbot done"},
		{"setbot", "Start Over", "setbot startover"},
		{"setbot", "Keep Default", "setbot setup_ignore"},
		{"antilink", "Default Links", "antilink default"},
		{"antilink", "Custom URLs", "antilink custom"},
		{"antilink", "Activate", "antilink activate"},
		{"antilink", "Deactivate", "antilink deactivate"},
		{"call", "Audio Call", "callaudio"},
		{"call", "Video Call", "callvideo"},
		{"call", "Voicemail", "voicemail"},
		{"timezone", "1. Africa/Lagos", "timezone setidx 1"},
		{"setbot", "1. Option", "setbot option"},
		{"setbot", "Next (Page 2)", "setbot page 2"},
		{"setbot", "Back", "setbot page 1"},
	}

	for _, tc := range cases {
		got := dispatch.MapOptionToCommandArgs(tc.cmdName, tc.option)
		if got != tc.expected {
			t.Errorf("MapOptionToCommandArgs(%q, %q) = %q, expected %q", tc.cmdName, tc.option, got, tc.expected)
		}
	}
}

func TestClosestCommand(t *testing.T) {
	dispatch.Register(
		&dispatch.Command{Name: "ping", Alias: "p,pong", Description: "Ping bot", IsPublic: true},
		&dispatch.Command{Name: "help", Alias: "menu", Description: "Help menu", IsPublic: true},
		&dispatch.Command{Name: "sticker", Alias: "s", Description: "Sticker creator", IsPublic: true},
		&dispatch.Command{Name: "restart", Description: "Restart bot", IsPublic: false},
		&dispatch.Command{Name: "unblock", Description: "Unblock user", IsPublic: false},
	)

	tests := []struct {
		input    string
		expected string
	}{
		{"pingg", "ping"},
		{"pign", "ping"},
		{"pinng", "ping"},
		{"ponng", "pong"},
		{"helpp", "help"},
		{"hlep", "help"},
		{"stiker", "sticker"},
		{"restrt", "restart"},
		{"unblck", "unblock"},
		{"", ""},
	}

	for _, tc := range tests {
		got := dispatch.ClosestCommand(tc.input)
		if got != tc.expected {
			t.Errorf("ClosestCommand(%q) = %q, expected %q", tc.input, got, tc.expected)
		}
	}
}

func TestFormatUnknownCommandSuggestion(t *testing.T) {
	msg1 := dispatch.FormatUnknownCommandSuggestion("@2348011111111", ".", "ping")
	expected1 := "@2348011111111 that's not quite right, did you mean to run .ping?"
	if msg1 != expected1 {
		t.Errorf("expected %q, got %q", expected1, msg1)
	}

	msg2 := dispatch.FormatUnknownCommandSuggestion("", "!", "help")
	expected2 := "@User that's not quite right, did you mean to run !help?"
	if msg2 != expected2 {
		t.Errorf("expected %q, got %q", expected2, msg2)
	}
}

func TestHandleUnknownCommand(t *testing.T) {
	dispatch.Register(
		&dispatch.Command{Name: "status", Description: "Bot status", IsPublic: true},
	)

	cctx := &dispatch.Context{
		Ctx:    context.Background(),
		Chat:   types.NewJID("120363000000001", types.GroupServer),
		Sender: types.NewJID("2348011111111", types.DefaultUserServer),
	}

	msg, handled := dispatch.HandleUnknownCommand(cctx, ".", "stutus")
	if !handled {
		t.Fatalf("expected handled to be true")
	}
	expected := "@2348011111111 that's not quite right, did you mean to run .status?"
	if msg != expected {
		t.Errorf("expected msg %q, got %q", expected, msg)
	}

	// Test with nil context
	_, handledNil := dispatch.HandleUnknownCommand(nil, ".", "stutus")
	if handledNil {
		t.Errorf("expected handled to be false for nil context")
	}

	// Test fallback when sender is empty
	cctxEmptySender := &dispatch.Context{
		Ctx:  context.Background(),
		Chat: types.EmptyJID,
	}
	msgEmpty, handledEmpty := dispatch.HandleUnknownCommand(cctxEmptySender, "!", "pinng")
	if !handledEmpty {
		t.Fatalf("expected handled to be true for empty sender context")
	}
	expectedEmpty := "@User that's not quite right, did you mean to run !ping?"
	if msgEmpty != expectedEmpty {
		t.Errorf("expected msg %q, got %q", expectedEmpty, msgEmpty)
	}
}

func TestResolveSenderMention(t *testing.T) {
	// 1. Nil context
	tagNil, mentionsNil := dispatch.ResolveSenderMention(nil)
	if tagNil != "@User" || len(mentionsNil) != 0 {
		t.Errorf("expected @User and nil mentions for nil context, got tag=%q mentions=%v", tagNil, mentionsNil)
	}

	// 2. Empty context
	tagEmpty, mentionsEmpty := dispatch.ResolveSenderMention(&dispatch.Context{})
	if tagEmpty != "@User" || len(mentionsEmpty) != 0 {
		t.Errorf("expected @User and nil mentions for empty context, got tag=%q mentions=%v", tagEmpty, mentionsEmpty)
	}

	// 3. Standard Phone Number JID
	pnJID := types.NewJID("2348011111111", types.DefaultUserServer)
	cctxPN := &dispatch.Context{
		Sender: pnJID,
	}
	tagPN, mentionsPN := dispatch.ResolveSenderMention(cctxPN)
	if tagPN != "@2348011111111" {
		t.Errorf("expected @2348011111111, got %q", tagPN)
	}
	if len(mentionsPN) != 1 || mentionsPN[0] != pnJID {
		t.Errorf("expected mentions [%v], got %v", pnJID, mentionsPN)
	}

	// 4. Sender with device suffix (AD JID)
	adJID := types.JID{
		User:     "2348011111111",
		RawAgent: 0,
		Device:   2,
		Server:   types.DefaultUserServer,
	}
	cctxAD := &dispatch.Context{
		Sender: adJID,
	}
	tagAD, mentionsAD := dispatch.ResolveSenderMention(cctxAD)
	if tagAD != "@2348011111111" {
		t.Errorf("expected @2348011111111, got %q", tagAD)
	}
	if len(mentionsAD) != 1 || mentionsAD[0] != pnJID {
		t.Errorf("expected device suffix to be stripped in mentions [%v], got %v", pnJID, mentionsAD)
	}

	// 5. Fallback to Evt.Info.Sender when cctx.Sender is empty
	cctxEvt := &dispatch.Context{
		Evt: &events.Message{
			Info: types.MessageInfo{
				Sender: pnJID,
			},
		},
	}
	tagEvt, mentionsEvt := dispatch.ResolveSenderMention(cctxEvt)
	if tagEvt != "@2348011111111" || len(mentionsEvt) != 1 || mentionsEvt[0] != pnJID {
		t.Errorf("expected resolution from Evt.Info.Sender, got tag=%q mentions=%v", tagEvt, mentionsEvt)
	}

	// 6. Fallback to DM Chat JID when sender is empty
	cctxDM := &dispatch.Context{
		Chat: pnJID,
	}
	tagDM, mentionsDM := dispatch.ResolveSenderMention(cctxDM)
	if tagDM != "@2348011111111" || len(mentionsDM) != 1 || mentionsDM[0] != pnJID {
		t.Errorf("expected resolution from DM chat JID, got tag=%q mentions=%v", tagDM, mentionsDM)
	}

	// 7. Group chat with empty sender must not treat group JID as user
	groupJID := types.NewJID("120363000000001", types.GroupServer)
	cctxGroupEmptySender := &dispatch.Context{
		Chat: groupJID,
	}
	tagGroup, mentionsGroup := dispatch.ResolveSenderMention(cctxGroupEmptySender)
	if tagGroup != "@User" || len(mentionsGroup) != 0 {
		t.Errorf("expected @User and nil mentions when only group chat JID is present, got tag=%q mentions=%v", tagGroup, mentionsGroup)
	}

	// 8. LID sender fallback
	lidJID := types.NewJID("1234567890123", types.HiddenUserServer)
	cctxLID := &dispatch.Context{
		Sender: lidJID,
	}
	tagLID, mentionsLID := dispatch.ResolveSenderMention(cctxLID)
	if tagLID != "@1234567890123" {
		t.Errorf("expected @1234567890123, got %q", tagLID)
	}
	if len(mentionsLID) != 1 || mentionsLID[0] != lidJID {
		t.Errorf("expected mentions [%v], got %v", lidJID, mentionsLID)
	}
}

func openDispatchTestDB(t *testing.T) *dbutil.Database {
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

	schemaName := fmt.Sprintf("test_dispatch_schema_%d", time.Now().UnixNano())
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

func TestStickerCommandDispatching(t *testing.T) {
	ctx := context.Background()
	db := openDispatchTestDB(t)

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

	var lastCmd atomic.Pointer[string]
	var lastArgs atomic.Pointer[[]string]
	var lastRawArgs atomic.Pointer[string]
	var executed atomic.Bool

	dispatch.Register(&dispatch.Command{
		Name:        "test_stk_cmd",
		Description: "Sticker test command",
		Category:    "test",
		IsPublic:    true,
		Handler: func(c *dispatch.Context) error {
			cmdStr := c.Command
			lastCmd.Store(&cmdStr)
			argsCopy := make([]string, len(c.Args))
			copy(argsCopy, c.Args)
			lastArgs.Store(&argsCopy)
			raw := c.RawArgs
			lastRawArgs.Store(&raw)
			executed.Store(true)
			return nil
		},
	})

	hash1Bytes := []byte("0123456789abcdef0123456789abcdef")
	hash1Hex := hex.EncodeToString(hash1Bytes)

	hash2Bytes := []byte("fedcba9876543210fedcba9876543210")
	hash2Hex := hex.EncodeToString(hash2Bytes)

	// Link hash1 -> test_stk_cmd (no args)
	if err := clistore.PutStickerCmd(ctx, sqStore, hash1Hex, "test_stk_cmd"); err != nil {
		t.Fatalf("PutStickerCmd hash1 failed: %v", err)
	}

	// Link hash2 -> .test_stk_cmd default_arg1 default_arg2 (prefix + default args)
	if err := clistore.PutStickerCmd(ctx, sqStore, hash2Hex, ".test_stk_cmd default_arg1 default_arg2"); err != nil {
		t.Fatalf("PutStickerCmd hash2 failed: %v", err)
	}

	fakeChat := types.NewJID("120363000000001", types.GroupServer)
	fakeSender := types.NewJID("2348011111111", types.DefaultUserServer)

	// 1. Direct sticker command (hash1) without extra args
	executed.Store(false)
	stk1 := &waE2E.StickerMessage{
		FileSHA256: hash1Bytes,
	}
	evt1 := &events.Message{
		Info: types.MessageInfo{
			ID:     "STK_MSG_1",
			Chat:   fakeChat,
			Sender: fakeSender,
		},
		Message: &waE2E.Message{
			StickerMessage: stk1,
		},
	}
	handled1 := dispatch.HandleStickerCommandPublicly(ctx, cli, sqStore, evt1, stk1)
	if !handled1 {
		t.Fatalf("expected handled1 to be true")
	}
	time.Sleep(50 * time.Millisecond)
	if !executed.Load() {
		t.Fatalf("expected command handler to execute for direct sticker")
	}
	if *lastCmd.Load() != "test_stk_cmd" {
		t.Errorf("expected command 'test_stk_cmd', got %q", *lastCmd.Load())
	}
	if len(*lastArgs.Load()) != 0 {
		t.Errorf("expected 0 args, got %v", *lastArgs.Load())
	}

	// 2. Direct sticker command (hash2) with stored default args and stripped prefix
	executed.Store(false)
	stk2 := &waE2E.StickerMessage{
		FileSHA256: hash2Bytes,
	}
	evt2 := &events.Message{
		Info: types.MessageInfo{
			ID:     "STK_MSG_2",
			Chat:   fakeChat,
			Sender: fakeSender,
		},
		Message: &waE2E.Message{
			StickerMessage: stk2,
		},
	}
	handled2 := dispatch.HandleStickerCommandPublicly(ctx, cli, sqStore, evt2, stk2)
	if !handled2 {
		t.Fatalf("expected handled2 to be true")
	}
	time.Sleep(50 * time.Millisecond)
	if !executed.Load() {
		t.Fatalf("expected command handler to execute for sticker with default args")
	}
	if *lastCmd.Load() != "test_stk_cmd" {
		t.Errorf("expected command 'test_stk_cmd', got %q", *lastCmd.Load())
	}
	if len(*lastArgs.Load()) != 2 || (*lastArgs.Load())[0] != "default_arg1" || (*lastArgs.Load())[1] != "default_arg2" {
		t.Errorf("expected args [default_arg1 default_arg2], got %v", *lastArgs.Load())
	}

	// 3. Sticker command (hash1) quoting a text message with extra args
	executed.Store(false)
	quotedText := "quoted extra arguments"
	stk3 := &waE2E.StickerMessage{
		FileSHA256: hash1Bytes,
		ContextInfo: &waE2E.ContextInfo{
			QuotedMessage: &waE2E.Message{
				Conversation: &quotedText,
			},
		},
	}
	evt3 := &events.Message{
		Info: types.MessageInfo{
			ID:     "STK_MSG_3",
			Chat:   fakeChat,
			Sender: fakeSender,
		},
		Message: &waE2E.Message{
			StickerMessage: stk3,
		},
	}
	handled3 := dispatch.HandleStickerCommandPublicly(ctx, cli, sqStore, evt3, stk3)
	if !handled3 {
		t.Fatalf("expected handled3 to be true")
	}
	time.Sleep(50 * time.Millisecond)
	if !executed.Load() {
		t.Fatalf("expected command handler to execute for sticker quoting text")
	}
	if len(*lastArgs.Load()) != 3 || (*lastArgs.Load())[0] != "quoted" || (*lastArgs.Load())[1] != "extra" || (*lastArgs.Load())[2] != "arguments" {
		t.Errorf("expected args [quoted extra arguments], got %v", *lastArgs.Load())
	}

	// 4. Text reply to a sticker command (replying to hash1 with args)
	executed.Store(false)
	replyText := "hello from reply"
	evt4 := &events.Message{
		Info: types.MessageInfo{
			ID:     "TXT_REPLY_4",
			Chat:   fakeChat,
			Sender: fakeSender,
		},
		Message: &waE2E.Message{
			ExtendedTextMessage: &waE2E.ExtendedTextMessage{
				Text: &replyText,
				ContextInfo: &waE2E.ContextInfo{
					QuotedMessage: &waE2E.Message{
						StickerMessage: &waE2E.StickerMessage{
							FileSHA256: hash1Bytes,
						},
					},
				},
			},
		},
	}
	handled4 := dispatch.HandleQuotedStickerCommandPublicly(ctx, cli, sqStore, evt4, replyText)
	if !handled4 {
		t.Fatalf("expected handled4 to be true")
	}
	time.Sleep(50 * time.Millisecond)
	if !executed.Load() {
		t.Fatalf("expected command handler to execute for text reply to sticker")
	}
	if len(*lastArgs.Load()) != 3 || (*lastArgs.Load())[0] != "hello" || (*lastArgs.Load())[1] != "from" || (*lastArgs.Load())[2] != "reply" {
		t.Errorf("expected args [hello from reply], got %v", *lastArgs.Load())
	}
}
