package main

import (
	"context"
	"os"
	"strings"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"

	"github.com/thruqe/whatsrook"
)

// SettingGetter retrieves a setting value for a client and key from the database
type SettingGetter func(ctx context.Context, client *whatsmeow.Client, key string) (string, error)

var (
	GlobalSettingGetter SettingGetter
)

// GetClientSetting retrieves a configuration value from the global setting getter or identity store fallback.
func GetClientSetting(ctx context.Context, client *whatsmeow.Client, key string) (string, error) {
	if GlobalSettingGetter != nil {
		if val, err := GlobalSettingGetter(ctx, client, key); err == nil && val != "" {
			return val, nil
		}
	}
	if client != nil && client.Store != nil && client.Store.Identities != nil {
		if s, ok := client.Store.Identities.(interface {
			GetSetting(ctx context.Context, key string) (string, error)
		}); ok {
			return s.GetSetting(ctx, key)
		}
	}
	return "", nil
}

// IsOwner returns true if the sender is the primary bot owner.
func (c *PluginContext) IsOwner() bool {
	if c == nil || c.Client == nil {
		return false
	}
	c.isOwnerOnce.Do(func() {
		c.isOwnerVal = c.calculateIsOwner()
	})
	return c.isOwnerVal
}

func (c *PluginContext) calculateIsOwner() bool {
	if c.Evt != nil && c.Evt.Info.IsFromMe {
		return true
	}
	if c.Client.Store == nil {
		return false
	}
	if c.Client.Store.ID != nil && !c.Client.Store.ID.IsEmpty() && IsSameUserRaw(c.GetSendContext(), c.Client, c.Sender, *c.Client.Store.ID) {
		return true
	}
	if !c.Client.Store.LID.IsEmpty() && IsSameUserRaw(c.GetSendContext(), c.Client, c.Sender, c.Client.Store.LID) {
		return true
	}
	if c.Evt != nil && !c.Evt.Info.SenderAlt.IsEmpty() {
		if c.Client.Store.ID != nil && !c.Client.Store.ID.IsEmpty() && IsSameUserRaw(c.GetSendContext(), c.Client, c.Evt.Info.SenderAlt, *c.Client.Store.ID) {
			return true
		}
		if !c.Client.Store.LID.IsEmpty() && IsSameUserRaw(c.GetSendContext(), c.Client, c.Evt.Info.SenderAlt, c.Client.Store.LID) {
			return true
		}
	}
	return false
}

// IsSudo returns true if the sender is a sudo user or bot owner.
func (c *PluginContext) IsSudo() bool {
	if c == nil || c.Client == nil {
		return false
	}
	c.isSudoOnce.Do(func() {
		c.isSudoVal = c.calculateIsSudo()
	})
	return c.isSudoVal
}

func (c *PluginContext) calculateIsSudo() bool {
	if c.IsOwner() {
		return true
	}
	if IsSudoRaw(c.GetSendContext(), c.Client, c.Sender) {
		return true
	}
	if c.Evt != nil {
		if !c.Evt.Info.SenderAlt.IsEmpty() {
			if IsSudoRaw(c.GetSendContext(), c.Client, c.Evt.Info.SenderAlt) {
				c.Client.StoreLIDPNMapping(c.GetSendContext(), c.Evt.Info.SenderAlt, c.Sender)
				return true
			}
		}
		if c.Evt.Info.PushName != "" {
			if raw, err := GetClientSetting(c.GetSendContext(), c.Client, "sudoers"); err == nil && raw != "" {
				pushLower := strings.ToLower(strings.TrimSpace(c.Evt.Info.PushName))
				for sudoerStr := range strings.FieldsSeq(raw) {
					if strings.EqualFold(sudoerStr, pushLower) {
						return true
					}
				}
			}
		}
	}
	return false
}

// IsSudoRaw checks if a sender JID has sudo/owner privileges stored in database settings or environment.
func IsSudoRaw(ctx context.Context, client *whatsmeow.Client, sender types.JID) bool {
	if client == nil || sender.IsEmpty() {
		return false
	}

	// 1. Check if sender is the bot owner (Store.ID or Store.LID)
	if client.Store != nil {
		if client.Store.ID != nil && !client.Store.ID.IsEmpty() && IsSameUserRaw(ctx, client, sender, *client.Store.ID) {
			return true
		}
		if !client.Store.LID.IsEmpty() && IsSameUserRaw(ctx, client, sender, client.Store.LID) {
			return true
		}
	}

	// 2. Check environment variables (SUDOERS, SUDO, OWNER)
	for _, envKey := range []string{"SUDOERS", "SUDO", "OWNER"} {
		if envVal := strings.TrimSpace(os.Getenv(envKey)); envVal != "" {
			for entry := range strings.FieldsSeq(envVal) {
				cleanEntry := strings.TrimPrefix(entry, "+")
				if parsed, err := types.ParseJID(entry); err == nil {
					if IsSameUserRaw(ctx, client, sender, parsed) {
						return true
					}
				} else if cleanEntry != "" && (sender.ToNonAD().User == cleanEntry || strings.TrimPrefix(sender.ToNonAD().User, "+") == cleanEntry) {
					return true
				}
			}
		}
	}

	// 3. Check database settings (database "sudoers" list)
	if raw, err := GetClientSetting(ctx, client, "sudoers"); err == nil && raw != "" {
		var senderPushName string
		if client.Store != nil && client.Store.Contacts != nil {
			lookupJID := sender.ToNonAD()
			if lookupJID.Server == types.HiddenUserServer && client.Store.LIDs != nil {
				if pn, pnErr := client.Store.LIDs.GetPNForLID(ctx, lookupJID); pnErr == nil && !pn.IsEmpty() {
					lookupJID = pn.ToNonAD()
				}
			}
			if contact, cErr := client.Store.Contacts.GetContact(ctx, lookupJID); cErr == nil && contact.Found {
				if contact.Username != "" {
					senderPushName = strings.ToLower(contact.Username)
				} else if contact.PushName != "" {
					senderPushName = strings.ToLower(contact.PushName)
				} else if contact.FullName != "" {
					senderPushName = strings.ToLower(contact.FullName)
				}
			}
		}
		if senderPushName == "" {
			if recent := whatsrook.GetRecentMessageForJID(sender); recent != nil && recent.Info.PushName != "" {
				senderPushName = strings.ToLower(recent.Info.PushName)
			}
		}

		for sudoerStr := range strings.FieldsSeq(raw) {
			cleanSudoer := strings.TrimPrefix(sudoerStr, "+")
			if strings.Contains(sudoerStr, "@") {
				if sudoerJID, err := types.ParseJID(sudoerStr); err == nil {
					if IsSameUserRaw(ctx, client, sender, sudoerJID) {
						return true
					}
				}
			} else if strings.HasPrefix(cleanSudoer, "user:") || strings.HasPrefix(cleanSudoer, "username:") {
				targetName := strings.ToLower(strings.TrimPrefix(strings.TrimPrefix(cleanSudoer, "user:"), "username:"))
				if senderPushName != "" && senderPushName == targetName {
					return true
				}
			} else if sender.ToNonAD().User == cleanSudoer || strings.TrimPrefix(sender.ToNonAD().User, "+") == cleanSudoer {
				return true
			}
		}
	}
	return false
}

// IsSameUserRaw compares two JIDs ignoring device suffixes and matching PN/LID mappings.
func IsSameUserRaw(ctx context.Context, client *whatsmeow.Client, a, b types.JID) bool {
	if a.IsEmpty() || b.IsEmpty() {
		return false
	}
	normA := a.ToNonAD()
	normB := b.ToNonAD()

	if normA == normB {
		return true
	}
	if normA.User == normB.User && normA.Server == normB.Server {
		return true
	}

	if client != nil && client.Store != nil && client.Store.LIDs != nil {
		if normA.Server == types.HiddenUserServer && normB.Server == types.DefaultUserServer {
			if pn, err := client.Store.LIDs.GetPNForLID(ctx, normA); err == nil && !pn.IsEmpty() {
				if pn.ToNonAD() == normB {
					return true
				}
			}
		} else if normA.Server == types.DefaultUserServer && normB.Server == types.HiddenUserServer {
			if pn, err := client.Store.LIDs.GetPNForLID(ctx, normB); err == nil && !pn.IsEmpty() {
				if pn.ToNonAD() == normA {
					return true
				}
			}
		}
	}
	return false
}

// ParticipantMatchesUser checks if a group participant matches the target JID.
func ParticipantMatchesUser(ctx context.Context, client *whatsmeow.Client, p types.GroupParticipant, target types.JID) bool {
	if IsSameUserRaw(ctx, client, p.JID, target) {
		return true
	}
	if !p.LID.IsEmpty() && IsSameUserRaw(ctx, client, p.LID, target) {
		return true
	}
	if !p.PhoneNumber.IsEmpty() && IsSameUserRaw(ctx, client, p.PhoneNumber, target) {
		return true
	}
	return false
}

// IsAdminRaw checks if a user is an admin or superadmin in groupInfo.
func IsAdminRaw(ctx context.Context, client *whatsmeow.Client, groupInfo *types.GroupInfo, userJID types.JID) bool {
	if groupInfo == nil || userJID.IsEmpty() {
		return false
	}
	for _, p := range groupInfo.Participants {
		if ParticipantMatchesUser(ctx, client, p, userJID) {
			return p.IsAdmin || p.IsSuperAdmin
		}
	}
	return false
}

// IsBotAdminRaw checks if the bot itself has admin rights in the group.
func IsBotAdminRaw(ctx context.Context, client *whatsmeow.Client, groupInfo *types.GroupInfo) bool {
	if client == nil || client.Store == nil || groupInfo == nil {
		return false
	}
	if client.Store.ID != nil && !client.Store.ID.IsEmpty() {
		if IsAdminRaw(ctx, client, groupInfo, *client.Store.ID) {
			return true
		}
	}
	if !client.Store.LID.IsEmpty() {
		if IsAdminRaw(ctx, client, groupInfo, client.Store.LID) {
			return true
		}
	}
	return false
}

// IsSenderAdmin checks if the sender has admin privileges in groupInfo.
func (c *PluginContext) IsSenderAdmin(groupInfo *types.GroupInfo) bool {
	if c == nil || c.Client == nil || groupInfo == nil {
		return false
	}
	return IsAdminRaw(c.GetSendContext(), c.Client, groupInfo, c.Sender)
}

// IsAdmin checks if a specific JID is a group admin.
func (c *PluginContext) IsAdmin(info *types.GroupInfo, jid types.JID) bool {
	if c == nil || c.Client == nil || info == nil {
		return false
	}
	return IsAdminRaw(c.GetSendContext(), c.Client, info, jid)
}

// AmIAdmin checks if the bot itself is an admin in the group.
func (c *PluginContext) AmIAdmin(info *types.GroupInfo) bool {
	if c == nil || c.Client == nil || info == nil {
		return false
	}
	return IsBotAdminRaw(c.GetSendContext(), c.Client, info)
}

// IsTargetSudo checks if a target JID is a sudo user or owner.
func (c *PluginContext) IsTargetSudo(target types.JID) bool {
	if c == nil || c.Client == nil {
		return false
	}
	return IsSudoRaw(c.GetSendContext(), c.Client, target)
}

// IsTargetOwner checks if a target JID is the bot owner.
func (c *PluginContext) IsTargetOwner(target types.JID) bool {
	if c == nil || c.Client == nil || c.Client.Store == nil {
		return false
	}
	if c.Client.Store.ID != nil && !c.Client.Store.ID.IsEmpty() && c.IsSameUser(target, *c.Client.Store.ID) {
		return true
	}
	if !c.Client.Store.LID.IsEmpty() && c.IsSameUser(target, c.Client.Store.LID) {
		return true
	}
	return false
}

// IsSameUser compares two JIDs ignoring device suffixes.
func (c *PluginContext) IsSameUser(a, b types.JID) bool {
	return IsSameUserRaw(c.GetSendContext(), c.Client, a, b)
}

// GetTargets resolves target user JIDs from quoted reply, mentions, or arguments.
func (c *PluginContext) GetTargets() []types.JID {
	if c == nil {
		return nil
	}
	if q, ok := c.GetQuotedSender(); ok && !q.IsEmpty() {
		return []types.JID{q.ToNonAD()}
	}
	if m := c.GetMentionedJIDs(); len(m) > 0 {
		var resolved []types.JID
		for _, j := range m {
			if !j.IsEmpty() {
				resolved = append(resolved, j.ToNonAD())
			}
		}
		if len(resolved) > 0 {
			return resolved
		}
	}
	if len(c.Args) > 0 {
		var resolved []types.JID
		for _, arg := range c.Args {
			if strings.Contains(arg, "@") {
				if parsed, err := types.ParseJID(arg); err == nil && !parsed.IsEmpty() {
					resolved = append(resolved, parsed.ToNonAD())
					continue
				}
			}
			clean := strings.TrimLeft(arg, "@+")
			if strings.Contains(clean, "@") {
				if parsed, err := types.ParseJID(clean); err == nil && !parsed.IsEmpty() {
					resolved = append(resolved, parsed.ToNonAD())
					continue
				}
			}
			if len(clean) >= 5 {
				resolved = append(resolved, types.NewJID(clean, types.DefaultUserServer))
			}
		}
		if len(resolved) > 0 {
			return resolved
		}
	}
	if !c.Chat.IsEmpty() && c.Chat.Server != "g.us" && c.Chat.Server != "broadcast" {
		if c.Client != nil && c.Client.Store != nil && c.Client.Store.ID != nil {
			if !c.IsSameUser(c.Chat, *c.Client.Store.ID) {
				if len(c.Args) == 0 {
					c.Args = []string{c.Chat.ToNonAD().String()}
				}
				return []types.JID{c.Chat.ToNonAD()}
			}
		} else {
			if len(c.Args) == 0 {
				c.Args = []string{c.Chat.ToNonAD().String()}
			}
			return []types.JID{c.Chat.ToNonAD()}
		}
	}
	return nil
}
