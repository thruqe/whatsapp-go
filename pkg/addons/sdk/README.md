![whatsrook](./assets/logo.svg)

_`whatsrook/pkg/sdk` is the Go library for building external plugins that run inside [WhatsRook](https://github.com/thruqe/whatsapp-go) — with full access to the WhatsApp action protocol._

[![Go Reference](https://pkg.go.dev/badge/whatsrook/pkg/sdk.svg)](https://pkg.go.dev/whatsrook/pkg/sdk)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

## Usage

Import `whatsrook/pkg/sdk`:

```go
import "whatsrook/pkg/sdk"
```

External plugins are standalone binaries. WhatsRook spawns them as child processes, writes a JSON request payload to `stdin`, and reads newline-delimited JSON action frames from `stdout`. See [How It Works](#how-it-works) for the full protocol, and the [Plugin Development Guide](../../docs/EXTERNAL_PLUGIN.md) for installation and deployment.

## Features

- **Request Parsing**: `stdin` JSON deserialization with argument indexing (`Arg`, `ArgAsInt`), subcommand routing (`Subcommand`), and flag evaluation (`Flag`, `FlagValue`).
- **Complete Action Protocol**: Text, image, audio, video, document, sticker, poll, reactions, and live in-place edits.
- **Fluent Action Builders**: Construct and chain actions with `.WithCaption()`, `.AsGIF()`, `.WithMimeType()`, and `.Send()`.
- **WhatsApp Message Formatting**: Markdown text decorators (`Bold`, `Italic`, `Quote`, `CodeBlock`) and fluent [`MessageBuilder`](#message-formatting).
- **Media Utilities**: RFC 4648 Base64 encoding/decoding and disk-to-data-URL helpers (`ReadFileAsDataURL`).
- **Live Session Support**: Send messages, receive message IDs via stdin ACK, and update them in real time.
- **Simple-Mode Output**: Single-reply plugins with zero boilerplate (`Respond(...)`).
- **Preconfigured HTTP Client**: Standard browser-like HTTP client (`CreateHTTPClient`).
- **CLI Development Fallback**: Test plugins directly from the terminal without a running WhatsApp connection.

## How It Works

When a WhatsApp user triggers a plugin command, WhatsRook:

1. Spawns the plugin binary.
2. Writes one JSON line to `stdin` — the `Request` payload.
3. Reads newline-delimited JSON action frames from `stdout`.
4. For frames that return a message ID (e.g. `reply`), writes an acknowledgement back to `stdin`.

```text
WhatsRook  ──stdin──▶  plugin (reads Request JSON)
plugin     ──stdout─▶  WhatsRook (reads Action frames)
WhatsRook  ──stdin──▶  plugin (reads Ack, for live actions)
```

## Quick Start

```go
package main

import (
	"fmt"
	"whatsrook/pkg/sdk"
)

func main() {
	req := sdk.Load()
	query := req.Query()

	if query == "" {
		sdk.Respond(fmt.Sprintf("Usage: %shello <name>", req.EffectivePrefix()))
		return
	}

	sdk.Respond(fmt.Sprintf("Hello, %s! 👋", query))
}
```

## Message Formatting

Format text with WhatsApp Markdown decorators or compose complex messages with `MessageBuilder`:

```go
msg := sdk.NewMessageBuilder().
	Header("Server Diagnostics").
	Bullet("CPU: 12% across 8 cores").
	Bullet("RAM: 3.2 GB / 16 GB").
	Newline().
	Quote("Cluster health: optimal").
	Build()

sdk.Respond(msg)
```

## Live Session Example

```go
package main

import (
	"fmt"
	"time"
	"whatsrook/pkg/sdk"
)

func main() {
	req := sdk.Load()

	if req.IsGroup && !req.IsAdmin {
		_ = sdk.SendReact("❌")
		sdk.Respond("This feature is for group admins only.")
		return
	}

	_ = sdk.SendReact("🚀")
	_ = sdk.SendPoll("Which asset to track?", []string{"BTC", "ETH", "Gold"})

	msgID, err := sdk.SendReplyLive("⏳ Initializing live tracker...")
	if err == nil && msgID != "" {
		for i := 1; i <= 5; i++ {
			time.Sleep(1500 * time.Millisecond)
			_ = sdk.SendEditLive(msgID, fmt.Sprintf("📈 Tracker tick #%d...", i))
		}
	}

	_ = sdk.SendDone()
}
```

## Action Reference

| Action | Helper | Description |
| :--- | :--- | :--- |
| `reply` | `SendReplyLive(text)` | Send text, returns `msgID` for edits |
| `edit` | `SendEditLive(id, text)` | In-place message edit |
| `react` | `SendReact(emoji)` / `SendReactTo(id, emoji)` | Emoji reaction |
| `delete` | `SendDelete(id)` | Revoke a message for everyone |
| `send_image` | `SendImage(data, caption)` | Image from URL or base64 |
| `send_audio` | `SendAudio(data, ptt)` | Audio or voice note |
| `send_video` | `SendVideo(data, caption)` | Video; `SendGIF` for looping GIF |
| `send_document` | `SendDocument(data, name, caption)` | File attachment |
| `send_sticker` | `SendSticker(data)` | WebP sticker |
| `poll` | `SendPoll(question, options)` / `SendMultiPoll` | Interactive poll |
| `loader` | `SendLoader(text)` | Typing / processing indicator |
| `done` | `SendDone()` | End the live session |

## Licensing

This project is open source, see the [LICENSE](./LICENSE) file for full details.
