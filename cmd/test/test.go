// Package test hosts owner-only experimental commands used to probe how
// WhatsApp clients render interactive message types and in-app webviews.
package test

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"whatsrook/cmd/dispatch"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

func init() {
	dispatch.Register(&dispatch.Command{
		Name:         "test",
		Description:  "Send working Test D in-app webview button pointing to local hello-world or custom URL",
		Category:     "owner",
		HideFromMenu: true,
		Handler:      handleTest,
	})
}

// handleTest sends WhatsApp Native Flow interactive buttons using the verified
// Test D implementation with WebView hints.
//
// When invoked without arguments, it serves an embedded hello.html via a local
// HTTP server exposed over a secure cloudflared quick tunnel.
//
// Implementation Details (Test D):
//   - Action: "cta_url"
//   - WebView hints:
//     webview_presentation: "full"
//     webview_interaction: true
//     payment_link_preview: false
//   - Wrapped in viewOnceMessage container
//   - Stanza wrapped with biz/native_flow transport data
func handleTest(ctx *dispatch.Context) error {
	target := strings.TrimSpace(ctx.RawArgs)
	if target == "" {
		_ = ctx.Reply("🚀 Starting local hello.html server and cloudflared tunnel...")
		var err error
		if target, err = publicPageURL(ctx.Ctx); err != nil {
			return fmt.Errorf("serve hello page: %w", err)
		}
		_ = ctx.Replyf("🌐 Serving local hello.html at:\n%s\n\nSending Test D webview buttons...", target)
	}

	if u, err := url.Parse(target); err != nil || u.Scheme != "https" || u.Host == "" {
		return ctx.Replyf("Invalid URL. Must be https://. Usage: %stest [optional-url]", ctx.GetPrefix())
	}

	// Variant 1: Exact Test D implementation (wrapped in viewOnceMessage)
	if err := sendTestDViewOnceVariant(ctx, target); err != nil {
		return fmt.Errorf("send viewOnce Test D variant: %w", err)
	}

	// Variant 2: Same Test D parameters without viewOnce wrapper (direct interactive)
	if err := sendTestDDirectVariant(ctx, target); err != nil {
		return fmt.Errorf("send direct Test D variant: %w", err)
	}

	return nil
}

// buildTestDPayload constructs the button JSON payload with all required webview hints.
func buildTestDPayload(target string) (string, error) {
	params, err := json.Marshal(struct {
		DisplayText         string `json:"display_text"`
		URL                 string `json:"url"`
		MerchantURL         string `json:"merchant_url"`
		WebviewPresentation string `json:"webview_presentation"`
		WebviewInteraction  bool   `json:"webview_interaction"`
		PaymentLinkPreview  bool   `json:"payment_link_preview"`
	}{
		DisplayText:         "Open Webview",
		URL:                 target,
		MerchantURL:         target,
		WebviewPresentation: "full",
		WebviewInteraction:  true,
		PaymentLinkPreview:  false,
	})
	if err != nil {
		return "", fmt.Errorf("marshal Test D params: %w", err)
	}
	return string(params), nil
}

// sendTestDViewOnceVariant sends the interactive message wrapped inside a viewOnceMessage
// container, which enables interactive button rendering on WhatsApp.
func sendTestDViewOnceVariant(ctx *dispatch.Context, target string) error {
	buttonJSON, err := buildTestDPayload(target)
	if err != nil {
		return err
	}

	interactiveMsg := &waE2E.InteractiveMessage{
		Header: &waE2E.InteractiveMessage_Header{
			Title:              new("WhatsApp Webview (Test D - ViewOnce)"),
			HasMediaAttachment: new(false),
		},
		Body: &waE2E.InteractiveMessage_Body{
			Text: new("Tap below to open Hello World. (Wrapped in viewOnceMessage + webview_interaction: true)"),
		},
		Footer: &waE2E.InteractiveMessage_Footer{
			Text: new("whatsrook • in-app webview"),
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

	innerMsg := &waE2E.Message{
		MessageContextInfo: &waE2E.MessageContextInfo{
			WeblinkRenderConfig: waE2E.WebLinkRenderConfig_WEBVIEW.Enum(),
		},
		InteractiveMessage: interactiveMsg,
	}

	msg := &waE2E.Message{
		MessageContextInfo: &waE2E.MessageContextInfo{
			WeblinkRenderConfig: waE2E.WebLinkRenderConfig_WEBVIEW.Enum(),
		},
		ViewOnceMessage: &waE2E.FutureProofMessage{
			Message: innerMsg,
		},
	}

	_, err = ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, msg)
	return err
}

// sendTestDDirectVariant sends the interactive message directly without the viewOnceMessage
// wrapper, but with all webview hints intact.
func sendTestDDirectVariant(ctx *dispatch.Context, target string) error {
	buttonJSON, err := buildTestDPayload(target)
	if err != nil {
		return err
	}

	msg := &waE2E.Message{
		MessageContextInfo: &waE2E.MessageContextInfo{
			WeblinkRenderConfig: waE2E.WebLinkRenderConfig_WEBVIEW.Enum(),
		},
		InteractiveMessage: &waE2E.InteractiveMessage{
			Header: &waE2E.InteractiveMessage_Header{
				Title:              new("WhatsApp Webview (Test D - Direct)"),
				HasMediaAttachment: new(false),
			},
			Body: &waE2E.InteractiveMessage_Body{
				Text: new("Tap below to open Hello World. (Direct interactive + webview_interaction: true)"),
			},
			Footer: &waE2E.InteractiveMessage_Footer{
				Text: new("whatsrook • in-app webview"),
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
		},
	}

	_, err = ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, msg)
	return err
}
