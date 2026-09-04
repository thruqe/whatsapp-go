package tests

import (
	"context"
	"testing"
	"whatsrook"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
)

func TestParticipantMatchesUser(t *testing.T) {
	ctx := context.Background()

	botPN := types.NewJID("2348011112222", types.DefaultUserServer)
	botLID := types.NewJID("1000000000001", types.HiddenUserServer)
	client := &whatsmeow.Client{
		Store: &store.Device{
			ID:  &botPN,
			LID: botLID,
		},
	}

	userPN := types.NewJID("2348099998888", types.DefaultUserServer)
	userLID := types.NewJID("1000000000099", types.HiddenUserServer)

	// Participant has LID as JID, and PN in PhoneNumber
	pLIDPrimary := types.GroupParticipant{
		JID:         userLID,
		LID:         userLID,
		PhoneNumber: userPN,
		IsAdmin:     true,
	}

	// 1. Should match userLID
	if !whatsrook.ParticipantMatchesUser(ctx, client, pLIDPrimary, userLID) {
		t.Errorf("expected pLIDPrimary to match userLID")
	}

	// 2. Should match userPN via p.PhoneNumber
	if !whatsrook.ParticipantMatchesUser(ctx, client, pLIDPrimary, userPN) {
		t.Errorf("expected pLIDPrimary to match userPN via p.PhoneNumber")
	}

	// 3. Should match userPN with AD device suffix
	userPNWithDevice := types.NewADJID("2348099998888", 0, 1)
	if !whatsrook.ParticipantMatchesUser(ctx, client, pLIDPrimary, userPNWithDevice) {
		t.Errorf("expected pLIDPrimary to match userPN with device suffix")
	}

	// 4. Bot matching when participant is listed as bot's LID
	pBotLID := types.GroupParticipant{
		JID:          botLID,
		LID:          botLID,
		PhoneNumber:  botPN,
		IsAdmin:      false,
		IsSuperAdmin: true,
	}

	// Should match bot by botPN (Store.ID)
	if !whatsrook.ParticipantMatchesUser(ctx, client, pBotLID, botPN) {
		t.Errorf("expected pBotLID to match botPN via client store")
	}
	// Should match bot by botLID (Store.LID)
	if !whatsrook.ParticipantMatchesUser(ctx, client, pBotLID, botLID) {
		t.Errorf("expected pBotLID to match botLID")
	}

	// 5. Non-matching user
	stranger := types.NewJID("2348077776666", types.DefaultUserServer)
	if whatsrook.ParticipantMatchesUser(ctx, client, pLIDPrimary, stranger) {
		t.Errorf("stranger should not match participant")
	}
}

func TestIsAdminRaw_And_IsBotAdminRaw(t *testing.T) {
	ctx := context.Background()

	botPN := types.NewJID("2348011112222", types.DefaultUserServer)
	botLID := types.NewJID("1000000000001", types.HiddenUserServer)
	client := &whatsmeow.Client{
		Store: &store.Device{
			ID:  &botPN,
			LID: botLID,
		},
	}

	adminPN := types.NewJID("2348022223333", types.DefaultUserServer)
	adminLID := types.NewJID("1000000000002", types.HiddenUserServer)

	memberPN := types.NewJID("2348033334444", types.DefaultUserServer)
	memberLID := types.NewJID("1000000000003", types.HiddenUserServer)

	groupInfo := &types.GroupInfo{
		JID: types.NewJID("123456789-group", types.GroupServer),
		Participants: []types.GroupParticipant{
			{
				JID:          botLID,
				LID:          botLID,
				PhoneNumber:  botPN,
				IsAdmin:      false,
				IsSuperAdmin: true, // bot is superadmin
			},
			{
				JID:         adminLID,
				LID:         adminLID,
				PhoneNumber: adminPN,
				IsAdmin:     true, // admin
			},
			{
				JID:         memberLID,
				LID:         memberLID,
				PhoneNumber: memberPN,
				IsAdmin:     false,
			},
		},
	}

	// Test IsBotAdminRaw
	if !whatsrook.IsBotAdminRaw(ctx, client, groupInfo) {
		t.Errorf("expected IsBotAdminRaw = true for superadmin bot")
	}

	// Test IsAdminRaw for bot with botPN
	if !whatsrook.IsAdminRaw(ctx, client, groupInfo, botPN) {
		t.Errorf("expected IsAdminRaw = true for botPN")
	}

	// Test IsAdminRaw for bot with botLID
	if !whatsrook.IsAdminRaw(ctx, client, groupInfo, botLID) {
		t.Errorf("expected IsAdminRaw = true for botLID")
	}

	// Test IsAdminRaw for regular admin with PN
	if !whatsrook.IsAdminRaw(ctx, client, groupInfo, adminPN) {
		t.Errorf("expected IsAdminRaw = true for adminPN")
	}

	// Test IsAdminRaw for regular admin with LID
	if !whatsrook.IsAdminRaw(ctx, client, groupInfo, adminLID) {
		t.Errorf("expected IsAdminRaw = true for adminLID")
	}

	// Test IsAdminRaw for regular member (should be false)
	if whatsrook.IsAdminRaw(ctx, client, groupInfo, memberPN) {
		t.Errorf("expected IsAdminRaw = false for memberPN")
	}
	if whatsrook.IsAdminRaw(ctx, client, groupInfo, memberLID) {
		t.Errorf("expected IsAdminRaw = false for memberLID")
	}

	// Test PluginContext
	pctx := &whatsrook.PluginContext{
		Ctx:    ctx,
		Client: client,
		Sender: adminPN,
	}

	if !pctx.AmIAdmin(groupInfo) {
		t.Errorf("expected pctx.AmIAdmin = true")
	}
	if !pctx.IsSenderAdmin(groupInfo) {
		t.Errorf("expected pctx.IsSenderAdmin = true for adminPN sender")
	}

	// Switch sender to regular member
	pctx.Sender = memberLID
	if pctx.IsSenderAdmin(groupInfo) {
		t.Errorf("expected pctx.IsSenderAdmin = false for memberLID sender")
	}
}
