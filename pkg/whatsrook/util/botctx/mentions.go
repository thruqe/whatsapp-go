package botctx

import (
	"context"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

// ResolveMentionJIDs resolves a participant JID to the full set of JIDs that must be
// included in ContextInfo.MentionedJID, and the parsed user identifier (phone number or
// LID user) to be used as "@" + tagUser in message text.
func ResolveMentionJIDs(ctx context.Context, client *whatsmeow.Client, participant types.JID) ([]types.JID, string) {
	resolved := participant.ToNonAD()

	var pnJID, lidJID types.JID
	switch resolved.Server {
	case types.HiddenUserServer:
		lidJID = resolved
		if client != nil && client.Store != nil && client.Store.LIDs != nil {
			if pn, err := client.Store.LIDs.GetPNForLID(ctx, resolved); err == nil && !pn.IsEmpty() {
				pnJID = pn.ToNonAD()
			}
		}
	default:
		pnJID = resolved
		if client != nil && client.Store != nil && client.Store.LIDs != nil {
			if lid, err := client.Store.LIDs.GetLIDForPN(ctx, resolved); err == nil && !lid.IsEmpty() {
				lidJID = lid.ToNonAD()
			}
		}
	}

	seen := make(map[string]bool)
	var jids []types.JID
	for _, j := range []types.JID{pnJID, lidJID} {
		if !j.IsEmpty() {
			key := j.String()
			if !seen[key] {
				seen[key] = true
				jids = append(jids, j)
			}
		}
	}
	if len(jids) == 0 {
		jids = append(jids, resolved)
	}

	tagUser := pnJID.User
	if tagUser == "" {
		tagUser = resolved.User
	}
	if tagUser == "" {
		tagUser = resolved.String()
	}
	if tagUser == "" {
		tagUser = "User"
	}
	return jids, tagUser
}

// ResolveMentionRaw resolves a participant JID to its primary non-AD JID and user string (without @).
func ResolveMentionRaw(ctx context.Context, client *whatsmeow.Client, participant types.JID) (types.JID, string) {
	jids, tagUser := ResolveMentionJIDs(ctx, client, participant)
	if len(jids) > 0 {
		return jids[0], tagUser
	}
	return participant.ToNonAD(), tagUser
}

// ResolveContactName returns the push name or full name of a contact, or falls back to phone number / user ID.
func ResolveContactName(ctx context.Context, client *whatsmeow.Client, participant types.JID) string {
	resolved := participant.ToNonAD()
	if client != nil && client.Store != nil && client.Store.Contacts != nil {
		if contact, err := client.Store.Contacts.GetContact(ctx, resolved); err == nil && contact.Found {
			if contact.PushName != "" {
				return contact.PushName
			} else if contact.FullName != "" {
				return contact.FullName
			}
		}
	}
	return resolved.User
}

// ResolveMentionJIDs resolves a JID on *PluginContext.
func (c *PluginContext) ResolveMentionJIDs(jid types.JID) ([]types.JID, string) {
	return ResolveMentionJIDs(c.GetSendContext(), c.Client, jid)
}

// ResolveMention resolves a JID on *PluginContext.
func (c *PluginContext) ResolveMention(jid types.JID) (types.JID, string) {
	return ResolveMentionRaw(c.GetSendContext(), c.Client, jid)
}

// FormatMention returns "@user" string and the resolved primary JID.
func (c *PluginContext) FormatMention(jid types.JID) (string, types.JID) {
	jids, tagUser := c.ResolveMentionJIDs(jid)
	resolved := jid.ToNonAD()
	if len(jids) > 0 {
		resolved = jids[0]
	}
	return "@" + tagUser, resolved
}

// FormatMentionJIDs returns "@user" string and all associated JIDs (PN + LID).
func (c *PluginContext) FormatMentionJIDs(jid types.JID) (string, []types.JID) {
	jids, tagUser := c.ResolveMentionJIDs(jid)
	return "@" + tagUser, jids
}

// GetContactName returns the display push name or full name of a contact.
func (c *PluginContext) GetContactName(jid types.JID) string {
	return ResolveContactName(c.GetSendContext(), c.Client, jid)
}

// ResolvePN returns the normalized non-AD phone number JID.
func (c *PluginContext) ResolvePN(jid types.JID) types.JID {
	return jid.ToNonAD()
}
