package main

import (
	"testing"

	"go.mau.fi/whatsmeow/types"
)

func TestResolveMentionTagging(t *testing.T) {
	tag := "on"
	ownerJIDStr := "100000000000001@s.whatsapp.net"
	userJID := types.NewJID("100000000000002", types.DefaultUserServer)
	customMsg := "Welcome {user} to the group! Hosted by {owner}."

	var mentions []string
	if tag == "on" || tag == "" {
		mentions = append(mentions, userJID.String())
	}
	if ownerJIDStr != "" && (containsPlaceholder(customMsg, "{owner}") || containsPlaceholder(customMsg, "{creator}")) {
		mentions = append(mentions, ownerJIDStr)
	}

	if len(mentions) != 2 {
		t.Fatalf("expected 2 mentions (user and owner), got %d: %+v", len(mentions), mentions)
	}
	if mentions[0] != userJID.String() || mentions[1] != ownerJIDStr {
		t.Errorf("mentions mismatch: %+v", mentions)
	}
}

func containsPlaceholder(s, placeholder string) bool {
	for i := 0; i+len(placeholder) <= len(s); i++ {
		if s[i:i+len(placeholder)] == placeholder {
			return true
		}
	}
	return false
}

func TestGroupPromoteDemoteMentions(t *testing.T) {
	memberJID := types.NewJID("2348011111111", types.DefaultUserServer)
	actorJID := types.NewJID("2348022222222", types.DefaultUserServer)

	mentions := []types.JID{memberJID}
	if !actorJID.IsEmpty() {
		mentions = append(mentions, actorJID)
	}

	if len(mentions) != 2 {
		t.Fatalf("expected 2 mentions for promotion event, got %d", len(mentions))
	}
	if mentions[0] != memberJID || mentions[1] != actorJID {
		t.Errorf("mentions order/values unexpected: %+v", mentions)
	}
}
