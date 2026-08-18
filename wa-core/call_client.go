package whatsmeow

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/rs/zerolog"
	"go.mau.fi/whatsmeow/diag"
	"go.mau.fi/whatsmeow/types"
)

// CallOptions controls media negotiated for an outbound call.
type CallOptions struct {
	// Video advertises a WhatsApp video call. The caller must provide encoded H.264
	// access units with Call.SendVideo after media is active.
	Video bool
}

// GroupCallOptions controls media and optional chat binding for an outbound group call.
type GroupCallOptions struct {
	// GroupJID binds the call to a WhatsApp group. Leave empty for an ad-hoc call.
	GroupJID string
	// Video advertises an H.264 group video call.
	Video bool
}

// Call places a 1:1 call to target (a phone number, a phone JID, or an @lid JID),
// returning the live Call once the offer is on the wire. Attach a Player and listeners
// to the returned Call; media starts automatically once the peer answers and the relay
// endpoint arrives.
func (cli *Client) Call(ctx context.Context, target string) (*Call, error) {
	return cli.CallWithOptions(ctx, target, CallOptions{})
}

// CallWithOptions places a 1:1 call with explicit media options.
func (cli *Client) CallWithOptions(ctx context.Context, target string, opts CallOptions) (*Call, error) {
	return cli.callEngine().placeCall(ctx, target, opts)
}

// GroupCall places an audio group call to at least two remote targets.
func (cli *Client) GroupCall(ctx context.Context, targets ...string) (*Call, error) {
	return cli.GroupCallWithOptions(ctx, targets, GroupCallOptions{})
}

// GroupCallWithOptions places a group call with explicit media and group binding.
func (cli *Client) GroupCallWithOptions(
	ctx context.Context,
	targets []string,
	opts GroupCallOptions,
) (*Call, error) {
	return cli.callEngine().placeGroupCall(ctx, targets, opts)
}

// GroupCallByID places an audio call to every remote member of a WhatsApp group.
// The groupID may be a bare numeric ID or a canonical @g.us JID.
func (cli *Client) GroupCallByID(ctx context.Context, groupID string) (*Call, error) {
	return cli.GroupCallByIDWithOptions(ctx, groupID, GroupCallOptions{})
}

// GroupCallByIDWithOptions resolves a WhatsApp group roster and places one bound
// group call to all remote members with explicit media options.
func (cli *Client) GroupCallByIDWithOptions(
	ctx context.Context,
	groupID string,
	opts GroupCallOptions,
) (*Call, error) {
	groupJID, err := parseGroupCallID(groupID)
	if err != nil {
		return nil, err
	}
	info, err := cli.GetGroupInfo(ctx, groupJID)
	if err != nil {
		return nil, fmt.Errorf("whatsmeow: get group info: %w", err)
	}
	if info == nil {
		return nil, errors.New("whatsmeow: group roster lookup returned no group")
	}
	targets := remoteGroupCallTargets(info.Participants, cli.groupSelfJIDs())
	if len(targets) < 2 {
		return nil, errors.New("whatsmeow: group call by ID requires at least two remote members")
	}
	if len(targets) > 31 {
		return nil, fmt.Errorf(
			"whatsmeow: group call by ID has %d remote members; at most 31 can be called",
			len(targets),
		)
	}
	opts.GroupJID = groupJID.String()
	return cli.GroupCallWithOptions(ctx, targets, opts)
}

func parseGroupCallID(raw string) (types.JID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return types.EmptyJID, errors.New("whatsmeow: group ID is required")
	}
	if !strings.ContainsRune(raw, '@') {
		raw += "@" + types.GroupServer
	}
	jid, err := types.ParseJID(raw)
	if err != nil || jid.Server != types.GroupServer || jid.User == "" {
		return types.EmptyJID, errors.New("whatsmeow: invalid group JID")
	}
	return jid.ToNonAD(), nil
}

func (cli *Client) groupSelfJIDs() []types.JID {
	if cli.Store == nil {
		return nil
	}
	return []types.JID{cli.Store.GetJID(), cli.Store.GetLID()}
}

func remoteGroupCallTargets(
	participants []types.GroupParticipant,
	self []types.JID,
) []string {
	seen := make(map[types.JID]struct{}, len(participants)*3+len(self))
	for _, jid := range self {
		if jid = jid.ToNonAD(); !jid.IsEmpty() {
			seen[jid] = struct{}{}
		}
	}
	targets := make([]string, 0, len(participants))
	for _, participant := range participants {
		identities := []types.JID{
			participant.JID.ToNonAD(),
			participant.PhoneNumber.ToNonAD(),
			participant.LID.ToNonAD(),
		}
		duplicate := false
		for _, jid := range identities {
			if jid.IsEmpty() {
				continue
			}
			if _, exists := seen[jid]; exists {
				duplicate = true
				break
			}
		}
		for _, jid := range identities {
			if !jid.IsEmpty() {
				seen[jid] = struct{}{}
			}
		}
		if duplicate {
			continue
		}
		target := types.EmptyJID
		for _, jid := range identities {
			if !jid.IsEmpty() {
				target = jid
				break
			}
		}
		if target.IsEmpty() {
			continue
		}
		targets = append(targets, target.String())
	}
	return targets
}

// OnIncomingCall registers the listener fired for each inbound call offer. The handler
// receives a Call that has not been answered yet; call Answer or Reject on it. Only the
// most recently registered listener is used.
func (cli *Client) OnIncomingCall(fn func(*Call)) {
	cli.callMu.Lock()
	cli.onIncomingCall = fn
	cli.callMu.Unlock()
	_ = cli.callEngine()
}

// incomingCallHandler returns the registered inbound-call listener, or nil.
func (cli *Client) incomingCallHandler() func(*Call) {
	cli.callMu.Lock()
	defer cli.callMu.Unlock()
	return cli.onIncomingCall
}

// SetCallLogger sets the zerolog logger for VoIP debugging.
func (cli *Client) SetCallLogger(l zerolog.Logger) {
	cli.callMu.Lock()
	cli.callLogger = l
	cli.callMu.Unlock()
	_ = cli.callEngine()
}

// SetCallDiag attaches a diagnostic recorder for VoIP.
func (cli *Client) SetCallDiag(d *diag.Recorder) {
	cli.callMu.Lock()
	cli.callDiag = d
	cli.callMu.Unlock()
	_ = cli.callEngine()
}

// InitCallEngine explicitly initializes and wires the VoIP engine.
func (cli *Client) InitCallEngine() {
	_ = cli.callEngine()
}

// callEngine returns the active VoIP engine, lazily initializing it.
func (cli *Client) callEngine() *engine {
	cli.callMu.Lock()
	defer cli.callMu.Unlock()
	if cli.callEng == nil {
		cli.callEng = newEngine(cli)
		cli.callEng.install()
	}
	return cli.callEng
}
