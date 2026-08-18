package whatsrook

import (
	"context"
	"errors"
	"fmt"

	"go.mau.fi/whatsmeow"
)

// ErrPairTimeout is returned when Whatsmeow fails to complete the pairing
// handshake within the designated deadline.
var ErrPairTimeout = errors.New("pairing timed out")

// PairPhone requests a pair code for the specified phone number.
func (c *Client) PairPhone(ctx context.Context, phone string) (string, error) {
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()
	if cli == nil {
		return "", errors.New("client not initialized")
	}

	if !cli.IsConnected() {
		if err := cli.Connect(); err != nil {
			return "", err
		}
	}

	var pairType whatsmeow.PairClientType
	var clientDisplay string

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

	code, err := cli.PairPhone(ctx, phone, true, pairType, clientDisplay)
	if err != nil {
		return "", fmt.Errorf("pair code failed: %w", err)
	}
	return code, nil
}
