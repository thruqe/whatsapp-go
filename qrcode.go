package whatsrook

import (
	"context"
	"errors"

	"go.mau.fi/whatsmeow"
)

// GetQRChannel returns the QR channel for pairing.
func (c *Client) GetQRChannel(ctx context.Context) (<-chan whatsmeow.QRChannelItem, error) {
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()
	if cli == nil {
		return nil, errors.New("client not initialized")
	}

	qrChan, err := cli.GetQRChannel(ctx)
	if err != nil {
		return nil, err
	}

	if !cli.IsConnected() {
		if err := cli.Connect(); err != nil {
			return nil, err
		}
	}
	return qrChan, nil
}
