package whatsrook

import (
	"context"
	"errors"
	"fmt"

	"go.mau.fi/whatsmeow"
)

// errpairtimeout is the sentinel error returned when the companion linking handshake
// fails to achieve cryptographic verification within the allocated temporal window.
// todo: integrate adaptive pairing timeout policies depending on network latency profiles.
// i don't really have an idea on how long it actually takes to make a pair code request invaild
// looks okay, for now
var ErrPairTimeout = errors.New("pairing timed out")

// pairphone requests an 8-character alphanumeric pairing code from the whatsapp signaling servers.
//
// operational protocol:
// 1. verifies client instantiation and establishes an active websocket connection if currently disconnected.
// 2. maps the configured companion platform profile (clienttype) to the corresponding whatsmeow pairing signatures.
// 3. dispatches the companion pairing payload with display metadata for manual confirmation on the primary device.
// todo: evaluate implementing exponential backoff when signaling servers reject rapid sequential pairing attempts.
func (c *Client) PairPhone(ctx context.Context, phone string) (string, error) {
	// retrieve raw client reference under mutex synchronization
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()
	if cli == nil {
		return "", errors.New("client not initialized")
	}

	// ensure active websocket transport before attempting pairing handshake
	if !cli.IsConnected() {
		if err := cli.Connect(); err != nil {
			return "", err
		}
	}

	var pairType whatsmeow.PairClientType
	var clientDisplay string

	// low key i hate how this is
	// wasn't the platform agent and os enough?
	// screw you WhatsApp!!!
	switch c.Config.ClientType {
	case ClientAndroid:
		pairType = whatsmeow.PairClientAndroid
		clientDisplay = "Chrome (Android)"
	case ClientIos:
		pairType = whatsmeow.PairClientChrome
		clientDisplay = "Chrome (iOS)"
	default:
		pairType = whatsmeow.PairClientChrome
		clientDisplay = "Chrome (Linux)"
	}

	// request companion linking code with showPushNotification enabled (true)
	// you may not receive this PushNotification for web client authentication (i don't know why)
	code, err := cli.PairPhone(ctx, phone, true, pairType, clientDisplay)
	if err != nil {
		return "", fmt.Errorf("pair code failed: %w", err)
	}
	return code, nil
}
