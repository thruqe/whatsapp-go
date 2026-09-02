package utils

import (
	"context"
	"fmt"

	"whatsrook"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

func ResolveBusinessTarget(ctx context.Context, client *whatsmeow.Client, targets []types.JID, chat types.JID, prefix, cmdName string) (types.JID, types.JID, error) {
	var rawTarget types.JID
	if len(targets) > 0 {
		rawTarget = targets[0]
	} else if chat.Server != types.GroupServer {
		rawTarget = chat
	} else {
		return types.JID{}, types.JID{}, fmt.Errorf("usage:\n- %s%s @user\n- %s%s 1234567890\n- Reply to a business user's message with %s%s", prefix, cmdName, prefix, cmdName, prefix, cmdName)
	}

	queryJID := rawTarget
	if rawTarget.Server == types.HiddenUserServer {
		if pnJID := whatsrook.ResolvePN(ctx, client, rawTarget); !pnJID.IsEmpty() {
			queryJID = pnJID
		}
	} else {
		queryJID = rawTarget.ToNonAD()
	}

	return rawTarget, queryJID, nil
}

func FetchBusinessProfileAndValidate(ctx context.Context, client *whatsmeow.Client, rawTarget, queryJID types.JID) (*types.BusinessProfile, error) {
	profile, err := client.GetBusinessProfile(ctx, queryJID)
	if (err != nil || profile == nil) && rawTarget != queryJID {
		profile, err = client.GetBusinessProfile(ctx, rawTarget)
	}

	if err != nil || profile == nil {
		return nil, fmt.Errorf("not a business user")
	}

	return profile, nil
}
