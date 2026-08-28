# External Plugins

External plugins allow you to extend whatsrook with independently developed executable programs. A plugin can be written in any language (Rust, Go, Python, C, etc.), compiled into a standalone binary, installed into whatsrook, and used directly as a WhatsApp command.

External plugins run as isolated child processes. They do not need to be compiled together with whatsrook, and they do not require changes to the whatsrook Go source code.

---

## How It Works

The external plugin lifecycle:

1. Create a plugin program.
2. Build an executable for the operating system and CPU architecture running whatsrook.
3. Install the executable with the `.install` command.
4. Run the plugin by sending its installed command name in WhatsApp.
5. The plugin reads a JSON request from standard input and either:
   - **Simple Mode**: Writes a single text reply to standard output.
   - **Live Streaming Mode**: Writes newline-delimited JSON action frames to standard output to send messages, receive message IDs, and perform live in-place edits (e.g. real-time ticker prices, live progress updates).

Example:

```text
.install weather
.weather London
```

The `.install`, `.uninstall`, and `.plist` management commands are restricted to the bot owner and configured sudoers.

---

## Plugin Commands

### Install

Install from official registry (automatically detects host OS & architecture, including Android Termux):

```text
.install <name>
```

Install all official plugins at once:

```text
.install all
```

Install from a custom HTTP/HTTPS URL or clean URL (whatsrook automatically appends host platform suffix if missing):

```text
.install weather https://example.com/releases/latest/download/weather
```

Install a local server binary:

```text
.install weather /opt/plugins/weather
```

Plugin names must:
- Contain between 1 and 64 characters.
- Start with an alphanumeric character.
- Contain only letters, numbers, underscores (`_`), and hyphens (`-`).

### List Installed Plugins

```text
.plist
```

(`pluginlist` is also accepted as an alias).

### Uninstall

```text
.uninstall <name>
.uninstall all
```

---

## Plugin Protocol

### Inbound Request Payload (Standard Input)

When a command is triggered, whatsrook sends a JSON request line on `stdin`:

```json
{
  "command": "btc",
  "args": ["stop"],
  "raw_args": "stop",
  "chat": "1234567890@s.whatsapp.net",
  "sender": "9876543210@s.whatsapp.net",
  "prefix": ".",
  "bot_name": "WhatsRook",
  "push_name": "Alice",
  "is_group": true,
  "is_sudo": true,
  "live_session": false,
  "is_cancel_request": true
}
```

#### Request Fields:

| Field | Type | Description |
|---|---|---|
| `command` | `string` | The installed plugin name. |
| `args` | `[]string` | Arguments split on whitespace. |
| `raw_args` | `string` | The complete unparsed argument string following the command. |
| `chat` | `string` | WhatsApp JID of the chat where the command was executed. |
| `sender` | `string` | WhatsApp JID of the sender. |
| `prefix` | `string` | Active command prefix from bot settings (e.g. `.` or `/`). |
| `bot_name` | `string` | Configured display name of the bot. |
| `push_name` | `string` | WhatsApp push display name of the sender. |
| `is_group` | `bool` | `true` if invoked inside a WhatsApp group. |
| `is_sudo` | `bool` | `true` if sender is bot owner or in `sudoers`. |
| `live_session` | `bool` | `true` if a live streaming session is currently active for this chat & command. |
| `is_cancel_request` | `bool` | `true` if the argument is a stop keyword (`stop`, `cancel`, `end`, `off`). |

---

### Response Modes

#### 1. Simple Mode (Plain Text)

For standard one-shot commands, write the reply directly to `stdout`. whatsrook trims the text and sends it as a WhatsApp reply.

```text
Weather for London: 18°C, Partly Cloudy ⛅
```

#### 2. Live Streaming & In-Place Editing Mode

For live tickers (like `.btc`), countdowns, progress loaders, or multi-step tasks, external plugins can stream newline-delimited JSON action frames to `stdout`.

##### Action Frames (Plugin → WhatsRook via `stdout`):

1. **Send Initial Message & Obtain Message ID:**
   ```json
   {"action":"reply","text":"₿ *Bitcoin Price:* $88,240.50\n\n_Updating every 1.5s..._"}
   ```
   WhatsRook responds on `stdin` with an Acknowledgment frame containing the sent WhatsApp `msg_id`:
   ```json
   {"ok":true,"msg_id":"3EB0ABC12345"}
   ```

2. **Live Edit Message:**
   ```json
   {"action":"edit","msg_id":"3EB0ABC12345","text":"₿ *Bitcoin Price:* $88,295.10\n\n_Updating every 1.5s..._"}
   ```

3. **Conclude Live Session:**
   ```json
   {"action":"done"}
   ```

---

## Live Session Management & Cancellation

When a streaming plugin is running:
- WhatsRook tracks the active session keyed by `chat_jid:command_name`.
- Users in the chat can send `.<command> stop` (e.g. `.btc stop`, `.btc cancel`, `.btc off`) at any time to instantly terminate the live process and clean up resources.
- Live sessions are automatically capped at a 5-minute safety timeout window.

---

## Manifest & Access Permissions

Each installed plugin stores a `<name>.json` manifest file adjacent to its executable. Plugins can configure public access:

```json
{
  "name": "btc",
  "description": "Live Bitcoin ticker",
  "is_public": true
}
```

- When `"is_public": true`, any participant in a chat or group can run the command.
- When omitted or `false`, only the bot owner and configured `sudoers` can invoke the command.

---

## Rust Example with `whatsrook-sdk`

```rust
use std::thread;
use std::time::{Duration, Instant};
use whatsrook_sdk::{create_http_client, send_done, send_edit_live, send_reply_live, Request};

fn main() {
    let req = Request::load_streaming();
    let prefix = req.prefix();

    let initial_msg = format!("⏳ Starting live countdown...\nUse {}countdown stop to cancel.", prefix);
    let msg_id = match send_reply_live(&initial_msg) {
        Some(id) => id,
        None => return,
    };

    for i in (1..=10).rev() {
        thread::sleep(Duration::from_millis(1500));
        send_edit_live(&msg_id, &format!("⏳ T-minus {} seconds...", i));
    }

    send_edit_live(&msg_id, "🚀 Blast off!");
    send_done();
}
```

---

## Complete Go Example

```go
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

type Request struct {
	Command string `json:"command"`
	Prefix  string `json:"prefix"`
	RawArgs string `json:"raw_args"`
}

type Action struct {
	Action string `json:"action"`
	Text   string `json:"text,omitempty"`
	MsgID  string `json:"msg_id,omitempty"`
}

type Ack struct {
	OK    bool   `json:"ok"`
	MsgID string `json:"msg_id"`
}

func main() {
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')

	var req Request
	_ = json.Unmarshal([]byte(strings.TrimSpace(line)), &req)

	// Send initial message
	action, _ := json.Marshal(Action{Action: "reply", Text: "Loading live status..."})
	fmt.Println(string(action))

	// Read Ack
	ackLine, _ := reader.ReadString('\n')
	var ack Ack
	_ = json.Unmarshal([]byte(strings.TrimSpace(ackLine)), &ack)

	// Live edit loop
	for i := 1; i <= 5; i++ {
		time.Sleep(1500 * time.Millisecond)
		edit, _ := json.Marshal(Action{
			Action: "edit",
			MsgID:  ack.MsgID,
			Text:   fmt.Sprintf("Live Status Update #%d/5", i),
		})
		fmt.Println(string(edit))
	}

	done, _ := json.Marshal(Action{Action: "done"})
	fmt.Println(string(done))
}
```

---

## Building for Multiple Platforms

WhatsRook uses static MUSL compilation on Linux to support both standard servers and Android (Termux) environments seamlessly:

- **Linux AMD64 (Static MUSL)**: `x86_64-unknown-linux-musl`
- **Linux ARM64 / Android Termux (Static MUSL)**: `aarch64-unknown-linux-musl`
- **macOS Apple Silicon**: `aarch64-apple-darwin`
- **macOS Intel**: `x86_64-apple-darwin`
- **Windows x64**: `x86_64-pc-windows-msvc`
