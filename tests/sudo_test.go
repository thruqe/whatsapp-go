package tests

import (
	"context"
	"os"
	"testing"
	"whatsrook"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

type mockIdentityStore struct {
	store.IdentityStore
	settings map[string]string
}

func (m *mockIdentityStore) GetSetting(ctx context.Context, key string) (string, error) {
	if val, ok := m.settings[key]; ok {
		return val, nil
	}
	return "", nil
}

func TestIsSameUserRaw(t *testing.T) {
	ctx := context.Background()

	botPN := types.NewJID("2348011112222", types.DefaultUserServer)
	botLID := types.NewJID("1000000000001", types.HiddenUserServer)

	devStore := &store.Device{
		ID:  &botPN,
		LID: botLID,
	}
	cli := &whatsmeow.Client{
		Store: devStore,
	}

	// 1. Same PN with different AD device suffixes
	senderAD := types.NewADJID("2348011112222", 0, 88)
	if !whatsrook.IsSameUserRaw(ctx, cli, senderAD, botPN) {
		t.Errorf("expected senderAD and botPN to match as same user")
	}

	// 2. Direct match between bot Store.ID and bot Store.LID
	if !whatsrook.IsSameUserRaw(ctx, cli, botLID, botPN) {
		t.Errorf("expected botLID and botPN to match as same user via store")
	}
	if !whatsrook.IsSameUserRaw(ctx, cli, botPN, botLID) {
		t.Errorf("expected botPN and botLID to match as same user via store")
	}

	// 3. Different user should not match
	otherUser := types.NewJID("2348099998888", types.DefaultUserServer)
	if whatsrook.IsSameUserRaw(ctx, cli, otherUser, botPN) {
		t.Errorf("expected different users not to match")
	}
}

func TestPluginContext_IsOwner_And_IsSudo(t *testing.T) {
	ctx := context.Background()

	botPN := types.NewJID("2348011112222", types.DefaultUserServer)
	botLID := types.NewJID("1000000000001", types.HiddenUserServer)
	otherUserPN := types.NewJID("2348055556666", types.DefaultUserServer)

	mockStore := &mockIdentityStore{
		settings: map[string]string{
			"sudoers": "2348055556666@s.whatsapp.net 1000000000009@lid",
		},
	}

	devStore := &store.Device{
		ID:         &botPN,
		LID:        botLID,
		Identities: mockStore,
	}
	cli := &whatsmeow.Client{
		Store: devStore,
	}

	// Case 1: Message sent from bot owner's primary device (IsFromMe = true)
	pctxFromMe := &whatsrook.PluginContext{
		Ctx:    ctx,
		Client: cli,
		Sender: botLID,
		Evt: &events.Message{
			Info: types.MessageInfo{
				IsFromMe: true,
				Sender:   botLID,
			},
			Message: &waE2E.Message{},
		},
	}
	if !pctxFromMe.IsOwner() {
		t.Errorf("expected IsOwner() = true when IsFromMe = true")
	}
	if !pctxFromMe.IsSudo() {
		t.Errorf("expected IsSudo() = true when IsOwner() = true")
	}

	// Case 2: Message sent with owner's LID in a group (IsFromMe = false, Sender = botLID)
	pctxLID := &whatsrook.PluginContext{
		Ctx:    ctx,
		Client: cli,
		Sender: botLID,
		Evt: &events.Message{
			Info: types.MessageInfo{
				IsFromMe: false,
				Sender:   botLID,
			},
			Message: &waE2E.Message{},
		},
	}
	if !pctxLID.IsOwner() {
		t.Errorf("expected IsOwner() = true when Sender matches bot Store.LID")
	}
	if !pctxLID.IsSudo() {
		t.Errorf("expected IsSudo() = true when Sender matches bot Store.LID")
	}

	// Case 3: Message sent with owner's PN
	pctxPN := &whatsrook.PluginContext{
		Ctx:    ctx,
		Client: cli,
		Sender: botPN,
		Evt: &events.Message{
			Info: types.MessageInfo{
				IsFromMe: false,
				Sender:   botPN,
			},
			Message: &waE2E.Message{},
		},
	}
	if !pctxPN.IsOwner() {
		t.Errorf("expected IsOwner() = true when Sender matches bot Store.ID")
	}
	if !pctxPN.IsSudo() {
		t.Errorf("expected IsSudo() = true when Sender matches bot Store.ID")
	}

	// Case 4: Secondary user in database sudoers list
	pctxSudoer := &whatsrook.PluginContext{
		Ctx:    ctx,
		Client: cli,
		Sender: otherUserPN,
		Evt: &events.Message{
			Info: types.MessageInfo{
				IsFromMe: false,
				Sender:   otherUserPN,
			},
			Message: &waE2E.Message{},
		},
	}
	if pctxSudoer.IsOwner() {
		t.Errorf("expected IsOwner() = false for sudoer who is not owner")
	}
	if !pctxSudoer.IsSudo() {
		t.Errorf("expected IsSudo() = true for user present in database sudoers list")
	}

	// Case 5: Secondary user in environment variable SUDOERS
	envUserPN := types.NewJID("2348077778888", types.DefaultUserServer)
	os.Setenv("SUDOERS", "2348077778888@s.whatsapp.net")
	defer os.Unsetenv("SUDOERS")

	pctxEnv := &whatsrook.PluginContext{
		Ctx:    ctx,
		Client: cli,
		Sender: envUserPN,
		Evt: &events.Message{
			Info: types.MessageInfo{
				IsFromMe: false,
				Sender:   envUserPN,
			},
			Message: &waE2E.Message{},
		},
	}
	if !pctxEnv.IsSudo() {
		t.Errorf("expected IsSudo() = true for user specified in SUDOERS env var")
	}

	// Case 6: Random unauthorized user
	randomUser := types.NewJID("1234500000", types.DefaultUserServer)
	pctxRandom := &whatsrook.PluginContext{
		Ctx:    ctx,
		Client: cli,
		Sender: randomUser,
		Evt: &events.Message{
			Info: types.MessageInfo{
				IsFromMe: false,
				Sender:   randomUser,
			},
			Message: &waE2E.Message{},
		},
	}
	if pctxRandom.IsOwner() {
		t.Errorf("expected IsOwner() = false for random user")
	}
	if pctxRandom.IsSudo() {
		t.Errorf("expected IsSudo() = false for random user")
	}
}

func TestDispatchPollVoteEvent_Wiring(t *testing.T) {
	pollID := "POLL_TEST_ID_123"
	pctx := &whatsrook.PluginContext{
		Chat:   types.NewJID("120363000000001", types.GroupServer),
		Sender: types.NewJID("2348011111111", types.DefaultUserServer),
	}

	// Message with PollUpdateMessage targeting nonexistent route should return false
	msgKeyID := pollID
	evt := &events.Message{
		Info: types.MessageInfo{
			Chat:   pctx.Chat,
			Sender: pctx.Sender,
			ID:     "VOTE_MSG_ID_1",
		},
		Message: &waE2E.Message{
			PollUpdateMessage: &waE2E.PollUpdateMessage{
				PollCreationMessageKey: &waCommon.MessageKey{
					ID: &msgKeyID,
				},
			},
		},
	}

	// No route registered: should not panic, returns false
	if whatsrook.DispatchPollVoteEvent(pctx, evt) {
		t.Errorf("expected false for unregistered poll route")
	}

	// Non-poll message should return false
	emptyEvt := &events.Message{
		Info: types.MessageInfo{
			Chat:   pctx.Chat,
			Sender: pctx.Sender,
		},
		Message: &waE2E.Message{},
	}
	if whatsrook.DispatchPollVoteEvent(pctx, emptyEvt) {
		t.Errorf("expected false for non-poll message")
	}
}
