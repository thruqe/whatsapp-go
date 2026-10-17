package whatsmeow

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/polymorfa/libsignal-protocol-go/groups"
	"github.com/polymorfa/libsignal-protocol-go/protocol"

	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"go.mau.fi/whatsmeow/util/log"
)

func TestParseGroupNode_MemberLinkAndShareHistoryMode(t *testing.T) {
	cli := &Client{
		Log: waLog.Stdout("Test", "DEBUG", true),
	}

	groupNode := &waBinary.Node{
		Tag: "group",
		Attrs: waBinary.Attrs{
			"id":       "120363428685490321",
			"creation": "1700000000",
			"subject":  "Test Group",
		},
		Content: []waBinary.Node{
			{
				Tag:     "member_share_group_history_mode",
				Content: []byte("all_member_share"),
			},
			{
				Tag:     "member_link_mode",
				Content: []byte("admin_link"),
			},
			{
				Tag:     "member_add_mode",
				Content: []byte("admin_add"),
			},
			{
				Tag: "allow_non_admin_sub_group_creation",
			},
		},
	}

	info, err := cli.parseGroupNode(groupNode)
	if err != nil {
		t.Fatalf("parseGroupNode failed: %v", err)
	}

	if info.MemberShareHistoryMode != types.GroupMemberShareHistoryModeAllMember {
		t.Errorf("expected MemberShareHistoryMode %s, got %s", types.GroupMemberShareHistoryModeAllMember, info.MemberShareHistoryMode)
	}
	if info.MemberLinkMode != types.GroupMemberLinkModeAdmin {
		t.Errorf("expected MemberLinkMode %s, got %s", types.GroupMemberLinkModeAdmin, info.MemberLinkMode)
	}
	if info.MemberAddMode != types.GroupMemberAddModeAdmin {
		t.Errorf("expected MemberAddMode %s, got %s", types.GroupMemberAddModeAdmin, info.MemberAddMode)
	}
	if !info.AllowNonAdminSubGroupCreation {
		t.Errorf("expected AllowNonAdminSubGroupCreation true, got false")
	}
}

func TestParseGroupNode_GeneralChat(t *testing.T) {
	cli := &Client{
		Log: waLog.Stdout("Test", "DEBUG", true),
	}

	groupNode := &waBinary.Node{
		Tag: "group",
		Attrs: waBinary.Attrs{
			"id":       "120363423354434197",
			"creation": "1700000000",
			"subject":  "Community General Chat",
		},
		Content: []waBinary.Node{
			{
				Tag: "general_chat",
			},
		},
	}

	info, err := cli.parseGroupNode(groupNode)
	if err != nil {
		t.Fatalf("parseGroupNode failed: %v", err)
	}

	if !info.IsGeneralChat {
		t.Errorf("expected IsGeneralChat true, got false")
	}

	// Also verify parseGroupLinkTargetNode with general_chat
	linkTarget, err := parseGroupLinkTargetNode(groupNode)
	if err != nil {
		t.Fatalf("parseGroupLinkTargetNode failed: %v", err)
	}
	if !linkTarget.IsGeneralChat {
		t.Errorf("expected linkTarget.IsGeneralChat true, got false")
	}

	// Verify group change with general_chat
	changeNode := &waBinary.Node{
		Tag: "notification",
		Attrs: waBinary.Attrs{
			"from": types.NewJID("120363423354434197", types.GroupServer),
			"t":    "1700000000",
		},
		Content: []waBinary.Node{
			{
				Tag: "general_chat",
			},
		},
	}
	evt, _, _, err := cli.parseGroupChangeWithUsernames(changeNode)
	if err != nil {
		t.Fatalf("parseGroupChangeWithUsernames failed: %v", err)
	}
	if evt.GeneralChat == nil || !*evt.GeneralChat {
		t.Errorf("expected evt.GeneralChat true, got %+v", evt.GeneralChat)
	}
}

func TestParseGroupChange_MemberLinkAndShareHistoryMode(t *testing.T) {
	cli := &Client{
		Log: waLog.Stdout("Test", "DEBUG", true),
	}

	changeNode := &waBinary.Node{
		Tag: "notification",
		Attrs: waBinary.Attrs{
			"from": types.NewJID("120363428685490321", types.GroupServer),
			"t":    "1700000000",
		},
		Content: []waBinary.Node{
			{
				Tag:     "member_link_mode",
				Content: []byte("all_member_link"),
			},
			{
				Tag:     "member_share_group_history_mode",
				Content: []byte("all_member_share"),
			},
			{
				Tag: "allow_non_admin_sub_group_creation",
			},
		},
	}

	evt, _, _, err := cli.parseGroupChangeWithUsernames(changeNode)
	if err != nil {
		t.Fatalf("parseGroupChangeWithUsernames failed: %v", err)
	}

	if evt.MemberLinkMode == nil || *evt.MemberLinkMode != types.GroupMemberLinkModeAllMember {
		t.Errorf("expected evt.MemberLinkMode %s, got %+v", types.GroupMemberLinkModeAllMember, evt.MemberLinkMode)
	}
	if evt.MemberShareHistoryMode == nil || *evt.MemberShareHistoryMode != types.GroupMemberShareHistoryModeAllMember {
		t.Errorf("expected evt.MemberShareHistoryMode %s, got %+v", types.GroupMemberShareHistoryModeAllMember, evt.MemberShareHistoryMode)
	}
	if evt.AllowNonAdminSubGroupCreation == nil || !*evt.AllowNonAdminSubGroupCreation {
		t.Errorf("expected AllowNonAdminSubGroupCreation true, got %+v", evt.AllowNonAdminSubGroupCreation)
	}
}

func TestSetStatusInput_JSONMarshal(t *testing.T) {
	text := "Status Message"
	input := types.SetStatusInput{
		Text: &text,
	}
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if strings.Contains(string(data), "ephemeral_duration_sec") {
		t.Errorf("expected ephemeral_duration_sec to be omitted when 0, got: %s", string(data))
	}
}

func TestGroupMetaCache_CanSend(t *testing.T) {
	adminJID := types.NewJID("123456", types.DefaultUserServer)
	nonAdminJID := types.NewJID("654321", types.DefaultUserServer)

	cache := &groupMetaCache{
		ReadOnly: true,
		Members:  []types.JID{adminJID, nonAdminJID},
		Admins:   []types.JID{adminJID},
	}

	msg := &waE2E.Message{
		Conversation: new("hello"),
	}

	if !cache.CanSend(adminJID, msg) {
		t.Errorf("expected admin to be able to send in read-only group")
	}
	adminADJID := adminJID
	adminADJID.Device = 1
	if !cache.CanSend(adminADJID, msg) {
		t.Errorf("expected admin companion device to be able to send in read-only group")
	}
	if cache.CanSend(nonAdminJID, msg) {
		t.Errorf("expected non-admin to NOT be able to send ordinary message in read-only group")
	}

	reactionMsg := &waE2E.Message{
		ReactionMessage: &waE2E.ReactionMessage{},
	}
	if !cache.CanSend(nonAdminJID, reactionMsg) {
		t.Errorf("expected non-admin to be able to send reaction message in read-only group")
	}

	pollUpdateMsg := &waE2E.Message{
		PollUpdateMessage: &waE2E.PollUpdateMessage{},
	}
	if !cache.CanSend(nonAdminJID, pollUpdateMsg) {
		t.Errorf("expected non-admin to be able to send poll update message in read-only group")
	}
}

func TestUpdateGroupParticipantCache(t *testing.T) {
	groupJID := types.NewJID("120363000000000000", types.GroupServer)
	adminJID := types.NewJID("111111", types.DefaultUserServer)
	memberJID := types.NewJID("222222", types.DefaultUserServer)
	newMemberJID := types.NewJID("333333", types.DefaultUserServer)

	cli := &Client{
		groupCache: map[types.JID]*groupMetaCache{
			groupJID: {
				ReadOnly: false,
				Members:  []types.JID{adminJID, memberJID},
				Admins:   []types.JID{adminJID},
			},
		},
	}

	// 1. Promote member to admin
	cli.updateGroupParticipantCache(&events.GroupInfo{
		JID:     groupJID,
		Promote: []types.JID{memberJID},
	})
	cached := cli.groupCache[groupJID]
	if !slices.Contains(cached.Admins, memberJID) {
		t.Errorf("expected memberJID to be promoted to admin")
	}

	// 2. Announce mode change
	cli.updateGroupParticipantCache(&events.GroupInfo{
		JID: groupJID,
		Announce: &types.GroupAnnounce{
			IsAnnounce: true,
		},
	})
	if !cached.ReadOnly {
		t.Errorf("expected group cache ReadOnly to be true")
	}

	// 3. Member join
	cli.updateGroupParticipantCache(&events.GroupInfo{
		JID:  groupJID,
		Join: []types.JID{newMemberJID},
	})
	if !slices.Contains(cached.Members, newMemberJID) {
		t.Errorf("expected newMemberJID to be in Members")
	}

	// 4. Demote member
	cli.updateGroupParticipantCache(&events.GroupInfo{
		JID:    groupJID,
		Demote: []types.JID{memberJID},
	})
	if slices.Contains(cached.Admins, memberJID) {
		t.Errorf("expected memberJID to be demoted from Admins")
	}

	// 5. Member leave
	cli.updateGroupParticipantCache(&events.GroupInfo{
		JID:   groupJID,
		Leave: []types.JID{newMemberJID},
	})
	if slices.Contains(cached.Members, newMemberJID) {
		t.Errorf("expected newMemberJID to be removed from Members")
	}
}

type mockLIDStore struct {
	store.LIDStore
	pnToLID map[types.JID]types.JID
	lidToPN map[types.JID]types.JID
}

func (m *mockLIDStore) GetLIDForPN(_ context.Context, pn types.JID) (types.JID, error) {
	return m.pnToLID[pn], nil
}

func (m *mockLIDStore) GetPNForLID(_ context.Context, lid types.JID) (types.JID, error) {
	return m.lidToPN[lid], nil
}

func TestGhostSend_ParticipantsAndPHashSuppression(t *testing.T) {
	pn1 := types.NewJID("111111", types.DefaultUserServer)
	lid1 := types.NewJID("999111", types.HiddenUserServer)
	pn2 := types.NewJID("222222", types.DefaultUserServer)
	pn1Companion := pn1
	pn1Companion.Device = 2

	cli := &Client{
		Store: &store.Device{
			LIDs: &mockLIDStore{
				pnToLID: map[types.JID]types.JID{pn1: lid1},
				lidToPN: map[types.JID]types.JID{lid1: pn1},
			},
		},
	}

	if (SendRequestExtra{}).isGhost() {
		t.Fatalf("expected empty SendRequestExtra.isGhost() to be false")
	}
	if !(SendRequestExtra{Ghost: true}).isGhost() {
		t.Fatalf("expected SendRequestExtra{Ghost: true}.isGhost() to be true")
	}
	if !(SendRequestExtra{Participants: []types.JID{pn1}}).isGhost() {
		t.Fatalf("expected SendRequestExtra with Participants to have isGhost() == true")
	}

	// Resolve PN -> LID for LID-addressed groups and deduplicate companion devices
	resolvedLID := resolveTargetParticipants(context.Background(), cli, []types.JID{pn1, pn1Companion, pn2}, types.AddressingModeLID)
	expectedLID := []types.JID{lid1, pn2}
	if !slices.Equal(resolvedLID, expectedLID) {
		t.Fatalf("expected resolved LID participants %v, got %v", expectedLID, resolvedLID)
	}

	// Resolve LID -> PN for PN-addressed groups
	resolvedPN := resolveTargetParticipants(context.Background(), cli, []types.JID{lid1, pn1Companion}, types.AddressingModePN)
	expectedPN := []types.JID{pn1}
	if !slices.Equal(resolvedPN, expectedPN) {
		t.Fatalf("expected resolved PN participants %v, got %v", expectedPN, resolvedPN)
	}

	// Verify ephemeralSenderKeyStore creates and encrypts without touching persistent store
	skStore := &ephemeralSenderKeyStore{}
	builder := groups.NewGroupSessionBuilder(skStore, pbSerializer)
	skName := protocol.NewSenderKeyName("120363000000000000@g.us", lid1.SignalAddress())
	skdMsg, err := builder.Create(context.Background(), skName)
	if err != nil || skdMsg == nil {
		t.Fatalf("expected ephemeralSenderKeyStore builder.Create to succeed, got err=%v", err)
	}
	cipher := groups.NewGroupCipher(builder, skName, skStore)
	if _, err := cipher.Encrypt(context.Background(), []byte("ghost payload")); err != nil {
		t.Fatalf("expected ephemeralSenderKeyStore cipher.Encrypt to succeed, got err=%v", err)
	}
}
