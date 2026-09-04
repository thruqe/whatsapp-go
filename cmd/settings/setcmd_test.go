package settings_test

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"testing"
	"time"

	"whatsrook/cmd/dispatch"
	_ "whatsrook/cmd/group"
	_ "whatsrook/cmd/info"
	clistore "whatsrook/cmd/store"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	meowstore "go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
)

func TestSetCmdAndDelCmdLifecycle(t *testing.T) {
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

	setCmd, ok := dispatch.Get("setcmd")
	if !ok {
		t.Fatalf("expected setcmd command to be registered")
	}

	delCmd, ok := dispatch.Get("delcmd")
	if !ok {
		t.Fatalf("expected delcmd command to be registered")
	}

	stkBytes1 := []byte("11112222333344445555666677778888")
	stkHex1 := hex.EncodeToString(stkBytes1)

	stkBytes2 := []byte("88887777666655554444333322221111")
	stkHex2 := hex.EncodeToString(stkBytes2)

	// 1. setcmd with prefix stripping: ".ping"
	cctx1 := &dispatch.Context{
		Ctx:     ctx,
		Client:  cli,
		Chat:    ownerPN,
		Sender:  ownerPN,
		Command: "setcmd",
		Args:    []string{".ping"},
		RawArgs: ".ping",
		Evt: &events.Message{
			Info: types.MessageInfo{
				Chat:     ownerPN,
				Sender:   ownerPN,
				IsFromMe: true,
				ID:       "EVT_SETCMD_1",
			},
			Message: &waE2E.Message{
				ExtendedTextMessage: &waE2E.ExtendedTextMessage{
					ContextInfo: &waE2E.ContextInfo{
						QuotedMessage: &waE2E.Message{
							StickerMessage: &waE2E.StickerMessage{
								FileSHA256: stkBytes1,
							},
						},
					},
				},
			},
		},
	}

	_ = setCmd.Handler(cctx1)

	// Verify in DB that it stripped the leading dot and stored "ping"
	mapped1, err := clistore.GetStickerCmd(ctx, sqStore, stkHex1)
	if err != nil {
		t.Fatalf("GetStickerCmd stk1 failed: %v", err)
	}
	if mapped1 != "ping" {
		t.Fatalf("expected mapped command 'ping', got %q", mapped1)
	}

	// 2. setcmd with arguments: ".tagall Wake up everyone!"
	cctx2 := &dispatch.Context{
		Ctx:     ctx,
		Client:  cli,
		Chat:    ownerPN,
		Sender:  ownerPN,
		Command: "setcmd",
		Args:    []string{".tagall", "Wake", "up", "everyone!"},
		RawArgs: ".tagall Wake up everyone!",
		Evt: &events.Message{
			Info: types.MessageInfo{
				Chat:     ownerPN,
				Sender:   ownerPN,
				IsFromMe: true,
				ID:       "EVT_SETCMD_2",
			},
			Message: &waE2E.Message{
				ExtendedTextMessage: &waE2E.ExtendedTextMessage{
					ContextInfo: &waE2E.ContextInfo{
						QuotedMessage: &waE2E.Message{
							StickerMessage: &waE2E.StickerMessage{
								FileSHA256: stkBytes2,
							},
						},
					},
				},
			},
		},
	}

	_ = setCmd.Handler(cctx2)

	// Verify in DB that it stripped the leading dot and stored "tagall Wake up everyone!"
	mapped2, err := clistore.GetStickerCmd(ctx, sqStore, stkHex2)
	if err != nil {
		t.Fatalf("GetStickerCmd stk2 failed: %v", err)
	}
	if mapped2 != "tagall Wake up everyone!" {
		t.Fatalf("expected mapped command 'tagall Wake up everyone!', got %q", mapped2)
	}

	// 3. delcmd by replying to sticker 1
	cctx3 := &dispatch.Context{
		Ctx:     ctx,
		Client:  cli,
		Chat:    ownerPN,
		Sender:  ownerPN,
		Command: "delcmd",
		Evt: &events.Message{
			Info: types.MessageInfo{
				Chat:     ownerPN,
				Sender:   ownerPN,
				IsFromMe: true,
				ID:       "EVT_DELCMD_1",
			},
			Message: &waE2E.Message{
				ExtendedTextMessage: &waE2E.ExtendedTextMessage{
					ContextInfo: &waE2E.ContextInfo{
						QuotedMessage: &waE2E.Message{
							StickerMessage: &waE2E.StickerMessage{
								FileSHA256: stkBytes1,
							},
						},
					},
				},
			},
		},
	}

	_ = delCmd.Handler(cctx3)

	delCheck1, _ := clistore.GetStickerCmd(ctx, sqStore, stkHex1)
	if delCheck1 != "" {
		t.Fatalf("expected stk1 to be deleted, got %q", delCheck1)
	}

	// 4. delcmd by base command name for sticker 2: "delcmd tagall"
	cctx4 := &dispatch.Context{
		Ctx:     ctx,
		Client:  cli,
		Chat:    ownerPN,
		Sender:  ownerPN,
		Command: "delcmd",
		Args:    []string{"tagall"},
		RawArgs: "tagall",
		Evt: &events.Message{
			Info: types.MessageInfo{
				Chat:     ownerPN,
				Sender:   ownerPN,
				IsFromMe: true,
				ID:       "EVT_DELCMD_2",
			},
		},
	}

	_ = delCmd.Handler(cctx4)

	delCheck2, _ := clistore.GetStickerCmd(ctx, sqStore, stkHex2)
	if delCheck2 != "" {
		t.Fatalf("expected stk2 to be deleted by base name 'tagall', got %q", delCheck2)
	}
}
