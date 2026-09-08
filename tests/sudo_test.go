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

func TestPluginContext_LID_And_SenderAlt_IsSudo(t *testing.T) {
	ctx := context.Background()

	botPN := types.NewJID("2348011112222", types.DefaultUserServer)
	botLID := types.NewJID("1000000000001", types.HiddenUserServer)
	sudoPN := types.NewJID("2348000000001", types.DefaultUserServer)
	sudoLID := types.NewJID("123456789012345", types.HiddenUserServer)

	mockStore := &mockIdentityStore{
		settings: map[string]string{
			"sudoers": "2348000000001@s.whatsapp.net testuser",
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

	// Message arriving from an LID sender where SenderAlt is the sudo PN
	pctx := &whatsrook.PluginContext{
		Ctx:    ctx,
		Client: cli,
		Sender: sudoLID,
		Chat:   sudoLID,
		Evt: &events.Message{
			Info: types.MessageInfo{
				Chat:      sudoLID,
				Sender:    sudoLID,
				SenderAlt: sudoPN,
				PushName:  "testuser",
			},
			Message: &waE2E.Message{},
		},
	}

	if !pctx.IsSudo() {
		t.Errorf("expected IsSudo() = true for LID sender when SenderAlt matches sudoer")
	}

	// Message arriving where Sender is LID and only PushName matches sudoer
	unknownLID := types.NewJID("9999999999999", types.HiddenUserServer)
	pctxPushName := &whatsrook.PluginContext{
		Ctx:    ctx,
		Client: cli,
		Sender: unknownLID,
		Chat:   unknownLID,
		Evt: &events.Message{
			Info: types.MessageInfo{
				Chat:     unknownLID,
				Sender:   unknownLID,
				PushName: "testuser",
			},
			Message: &waE2E.Message{},
		},
	}

	if !pctxPushName.IsSudo() {
		t.Errorf("expected IsSudo() = true for user when PushName matches sudoer")
	}
}

func TestPluginContext_GetTargets_DM_Fallback(t *testing.T) {
	botPN := types.NewJID("2348011112222", types.DefaultUserServer)
	botLID := types.NewJID("1000000000001", types.HiddenUserServer)
	targetUser := types.NewJID("2348000000001", types.DefaultUserServer)

	devStore := &store.Device{
		ID:  &botPN,
		LID: botLID,
	}
	cli := &whatsmeow.Client{
		Store: devStore,
	}

	// 1. In a DM chat without args or quote, GetTargets resolves to Chat and populates Args
	pctxDM := &whatsrook.PluginContext{
		Client: cli,
		Chat:   targetUser,
		Sender: targetUser,
		Args:   []string{},
	}
	targets := pctxDM.GetTargets()
	if len(targets) != 1 || targets[0] != targetUser {
		t.Fatalf("expected GetTargets to resolve to %v in DM, got %v", targetUser, targets)
	}
	if len(pctxDM.Args) != 1 || pctxDM.Args[0] != targetUser.String() {
		t.Errorf("expected Args to be populated with %q, got %v", targetUser.String(), pctxDM.Args)
	}

	// 2. In a group chat without args or quote, GetTargets returns nil
	groupChat := types.NewJID("120363000000001", types.GroupServer)
	pctxGroup := &whatsrook.PluginContext{
		Client: cli,
		Chat:   groupChat,
		Sender: targetUser,
		Args:   []string{},
	}
	if targetsGroup := pctxGroup.GetTargets(); len(targetsGroup) != 0 {
		t.Errorf("expected GetTargets to return nil in group chat, got %v", targetsGroup)
	}

	// 3. With explicit args, GetTargets resolves the arg JID
	explicitUser := types.NewJID("2348099991111", types.DefaultUserServer)
	pctxArgs := &whatsrook.PluginContext{
		Client: cli,
		Chat:   groupChat,
		Sender: targetUser,
		Args:   []string{"2348099991111"},
	}
	if targetsArgs := pctxArgs.GetTargets(); len(targetsArgs) != 1 || targetsArgs[0] != explicitUser {
		t.Errorf("expected GetTargets to resolve explicit arg %v, got %v", explicitUser, targetsArgs)
	}
}

func TestGlobalSettingGetter_IsSudoRaw(t *testing.T) {
	ctx := context.Background()

	botPN := types.NewJID("2348011112222", types.DefaultUserServer)
	sudoLID := types.NewJID("123456789012346", types.HiddenUserServer)
	sudoPN := types.NewJID("2348000000002", types.DefaultUserServer)

	// Device store with NO IdentityStore implementing GetSetting
	devStore := &store.Device{
		ID: &botPN,
	}
	cli := &whatsmeow.Client{
		Store: devStore,
	}

	// Register GlobalSettingGetter
	origGetter := whatsrook.GlobalSettingGetter
	defer func() { whatsrook.GlobalSettingGetter = origGetter }()

	whatsrook.GlobalSettingGetter = func(ctx context.Context, client *whatsmeow.Client, key string) (string, error) {
		if key == "sudoers" {
			return "2348000000002@s.whatsapp.net 123456789012346@lid testuser", nil
		}
		return "", nil
	}

	// Test 1: sender is LID in sudoers
	if !whatsrook.IsSudoRaw(ctx, cli, sudoLID) {
		t.Errorf("expected IsSudoRaw = true for sudoLID via GlobalSettingGetter")
	}

	// Test 2: sender is PN in sudoers
	if !whatsrook.IsSudoRaw(ctx, cli, sudoPN) {
		t.Errorf("expected IsSudoRaw = true for sudoPN via GlobalSettingGetter")
	}

	// Test 3: PluginContext IsSudo with SenderAlt
	pctx := &whatsrook.PluginContext{
		Ctx:    ctx,
		Client: cli,
		Sender: sudoLID,
		Chat:   sudoLID,
		Evt: &events.Message{
			Info: types.MessageInfo{
				Chat:      sudoLID,
				Sender:    sudoLID,
				SenderAlt: sudoPN,
				PushName:  "Whatsrook",
			},
			Message: &waE2E.Message{},
		},
	}
	if !pctx.IsSudo() {
		t.Errorf("expected pctx.IsSudo() = true for sudo user via GlobalSettingGetter")
	}
}
