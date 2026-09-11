package whatsmeow

import (
	"encoding/json"
	"strings"
	"testing"

	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/types"
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
		t.Errorf("expected evt.AllowNonAdminSubGroupCreation true, got %+v", evt.AllowNonAdminSubGroupCreation)
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
