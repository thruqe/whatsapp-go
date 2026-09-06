package whatsrook

import (
	"context"
	"testing"

	"go.mau.fi/whatsmeow/types"
)

func TestResolveMentionPN(t *testing.T) {
	ctx := context.Background()
	pnJID := types.NewJID("2348012345678", types.DefaultUserServer)

	resolvedJID, tagUser := ResolveMentionRaw(ctx, nil, pnJID)
	if tagUser != "2348012345678" {
		t.Fatalf("expected tagUser to be '2348012345678', got '%s'", tagUser)
	}
	if resolvedJID.User != "2348012345678" || resolvedJID.Server != types.DefaultUserServer {
		t.Fatalf("expected resolvedJID to be %s, got %s", pnJID.String(), resolvedJID.String())
	}

	jids, tagUser2 := ResolveMentionJIDs(ctx, nil, pnJID)
	if tagUser2 != "2348012345678" {
		t.Fatalf("expected tagUser2 to be '2348012345678', got '%s'", tagUser2)
	}
	if len(jids) != 1 || jids[0].User != "2348012345678" {
		t.Fatalf("expected 1 jid with user '2348012345678', got %+v", jids)
	}
}

func TestResolveMentionLIDFallback(t *testing.T) {
	ctx := context.Background()
	lidJID := types.NewJID("123456789012345", types.HiddenUserServer)

	resolvedJID, tagUser := ResolveMentionRaw(ctx, nil, lidJID)
	if tagUser != "123456789012345" {
		t.Fatalf("expected tagUser to be '123456789012345', got '%s'", tagUser)
	}
	if resolvedJID.User != "123456789012345" || resolvedJID.Server != types.HiddenUserServer {
		t.Fatalf("expected resolvedJID to be %s, got %s", lidJID.String(), resolvedJID.String())
	}
}

func TestFormatMention(t *testing.T) {
	pctx := &PluginContext{
		Chat:   types.NewJID("12345", "g.us"),
		Sender: types.NewJID("2348012345678", types.DefaultUserServer),
	}

	target := types.NewJID("2348099999999", types.DefaultUserServer)
	tag, resolved := pctx.FormatMention(target)

	if tag != "@2348099999999" {
		t.Fatalf("expected tag '@2348099999999', got '%s'", tag)
	}
	if resolved.User != "2348099999999" {
		t.Fatalf("expected resolved.User '2348099999999', got '%s'", resolved.User)
	}

	tagJIDs, jids := pctx.FormatMentionJIDs(target)
	if tagJIDs != "@2348099999999" {
		t.Fatalf("expected tagJIDs '@2348099999999', got '%s'", tagJIDs)
	}
	if len(jids) == 0 || jids[0].User != "2348099999999" {
		t.Fatalf("expected jids containing target, got %+v", jids)
	}
}

func TestResolveContactNameFallback(t *testing.T) {
	ctx := context.Background()
	pnJID := types.NewJID("2348012345678", types.DefaultUserServer)

	name := ResolveContactName(ctx, nil, pnJID)
	if name != "2348012345678" {
		t.Fatalf("expected fallback to user '2348012345678', got '%s'", name)
	}
}
