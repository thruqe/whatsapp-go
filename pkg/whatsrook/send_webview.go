package whatsrook

import (
	"context"
	"encoding/json"
	"fmt"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

// WebviewButtonParams defines parameters for a CTA URL button configured for WhatsApp's in-app browser.
type WebviewButtonParams struct {
	DisplayText         string `json:"display_text"`
	URL                 string `json:"url"`
	MerchantURL         string `json:"merchant_url"`
	WebviewPresentation string `json:"webview_presentation"` // "full", "tall", or "compact"
	WebviewInteraction  bool   `json:"webview_interaction"`  // true keeps navigation inside WhatsApp
	PaymentLinkPreview  bool   `json:"payment_link_preview"`
}

// WebviewMessageOpts defines options for constructing and sending an in-app WebView button message.
type WebviewMessageOpts struct {
	Title               string             // Header title
	Body                string             // Body text content
	Footer              string             // Footer text (optional)
	ButtonText          string             // Button display label (e.g. "Open In-App Webview")
	URL                 string             // Destination URL (must be valid https://)
	MerchantURL         string             // Optional; defaults to URL
	WebviewPresentation string             // Optional; "full" (default), "tall", or "compact"
	Quoted              *waE2E.ContextInfo // Optional quoted message ContextInfo
}

// BuildWebviewPayload serializes the button parameters JSON required by WhatsApp Native Flow for webview interaction.
func BuildWebviewPayload(displayText, targetURL string, presentation ...string) (string, error) {
	pres := "full"
	if len(presentation) > 0 && presentation[0] != "" {
		pres = presentation[0]
	}

	params, err := json.Marshal(WebviewButtonParams{
		DisplayText:         displayText,
		URL:                 targetURL,
		MerchantURL:         targetURL,
		WebviewPresentation: pres,
		WebviewInteraction:  true,
		PaymentLinkPreview:  false,
	})
	if err != nil {
		return "", fmt.Errorf("marshal webview payload: %w", err)
	}
	return string(params), nil
}

// BuildWebviewMessage constructs a direct Native Flow InteractiveMessage configured with WeblinkRenderConfig_WEBVIEW.
func BuildWebviewMessage(opts WebviewMessageOpts) (*waE2E.Message, error) {
	pres := opts.WebviewPresentation
	if pres == "" {
		pres = "full"
	}
	btnText := opts.ButtonText
	if btnText == "" {
		btnText = "Open In-App Webview"
	}

	targetURL := opts.URL
	merchantURL := opts.MerchantURL
	if merchantURL == "" {
		merchantURL = targetURL
	}

	params, err := json.Marshal(WebviewButtonParams{
		DisplayText:         btnText,
		URL:                 targetURL,
		MerchantURL:         merchantURL,
		WebviewPresentation: pres,
		WebviewInteraction:  true,
		PaymentLinkPreview:  false,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal webview payload: %w", err)
	}
	buttonJSON := string(params)

	interactiveMsg := &waE2E.InteractiveMessage{
		Body: &waE2E.InteractiveMessage_Body{
			Text: new(opts.Body),
		},
		InteractiveMessage: &waE2E.InteractiveMessage_NativeFlowMessage_{
			NativeFlowMessage: &waE2E.InteractiveMessage_NativeFlowMessage{
				Buttons: []*waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton{{
					Name:             new("cta_url"),
					ButtonParamsJSON: new(buttonJSON),
				}},
				MessageVersion: proto.Int32(1),
			},
		},
	}

	if opts.Title != "" {
		interactiveMsg.Header = &waE2E.InteractiveMessage_Header{
			Title:              new(opts.Title),
			HasMediaAttachment: new(false),
		}
	}

	if opts.Footer != "" {
		interactiveMsg.Footer = &waE2E.InteractiveMessage_Footer{
			Text: new(opts.Footer),
		}
	}

	if opts.Quoted != nil {
		interactiveMsg.ContextInfo = opts.Quoted
	}

	return &waE2E.Message{
		MessageContextInfo: &waE2E.MessageContextInfo{
			WeblinkRenderConfig: waE2E.WebLinkRenderConfig_WEBVIEW.Enum(),
		},
		InteractiveMessage: interactiveMsg,
	}, nil
}

// SendWebview sends a Native Flow in-app WebView button message.
func SendWebview(ctx context.Context, client *whatsmeow.Client, chat types.JID, opts WebviewMessageOpts) error {
	_, err := SendWebviewWithID(ctx, client, chat, opts)
	return err
}

// SendWebviewWithID sends a Native Flow in-app WebView button message and returns the assigned message ID.
func SendWebviewWithID(ctx context.Context, client *whatsmeow.Client, chat types.JID, opts WebviewMessageOpts) (string, error) {
	if client == nil {
		return "", fmt.Errorf("client unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	msg, err := BuildWebviewMessage(opts)
	if err != nil {
		return "", err
	}

	resp, err := client.SendMessage(ctx, chat, msg)
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

// Webview sends an in-app WebView button message via the Sender instance.
func (s *Sender) Webview(opts WebviewMessageOpts) error {
	return SendWebview(s.getContext(), s.Client, s.Chat, opts)
}

// WebviewWithID sends an in-app WebView button message via the Sender instance and returns the assigned message ID.
func (s *Sender) WebviewWithID(opts WebviewMessageOpts) (string, error) {
	return SendWebviewWithID(s.getContext(), s.Client, s.Chat, opts)
}
