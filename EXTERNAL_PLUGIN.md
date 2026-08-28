# External Plugins

External plugins allow you to extend whatsrook with independently developed executable programs. A plugin can be written in any language, compiled into a binary, installed into whatsrook, and used as a WhatsApp command.

External plugins run as separate processes. They do not need to be compiled together with whatsrook, and they do not require a Go import or a change to the whatsrook source code.

## How It Works

The external plugin flow is:

1. Create a plugin program.
2. Build an executable for the operating system and CPU architecture running whatsrook.
3. Install the executable with the `install` command.
4. Run the plugin by sending its installed name as a command.
5. The plugin reads a JSON request from standard input and writes its WhatsApp reply to standard output.

For example:

```text
.install weather /home/user/weather-plugin
.weather London
```

The `install`, `uninstall`, and `plist` commands are in the `Plugins` command category. They are restricted to the bot owner and configured sudoers.

## Plugin Commands

### Install

Install a local executable:

```text
.install <name> <path>
```

Example:

```text
.install weather /home/user/weather-plugin
```

Install an executable from an HTTP or HTTPS URL:

```text
.install weather https://example.com/releases/weather-plugin-linux-amd64
```

The source must be a local file or an HTTP(S) URL. The downloaded or copied file is stored in the managed plugin directory, made executable, and registered under the supplied name.

Plugin names must:

- Contain between 1 and 64 characters.
- Start with a letter or number.
- Contain only letters, numbers, underscores (`_`), and hyphens (`-`).

Names are normalized to lowercase during installation and removal.

### List Installed Plugins

List all installed external plugins:

```text
.plist
```

`pluginlist` is also accepted as an alias:

```text
.pluginlist
```

The list contains the installed plugin names in alphabetical order.

### Uninstall

Remove an installed plugin:

```text
.uninstall <name>
```

Example:

```text
.uninstall weather
```

Uninstall only removes the executable and metadata managed by whatsrook. It does not remove the original local source file or any remote release.

## Plugin Protocol

The executable is started with the command arguments passed after the plugin name. For this message:

```text
.weather London tomorrow
```

whatsrook starts the executable approximately as:

```text
weather London tomorrow
```

The plugin also receives a JSON request on standard input:

```json
{
  "command": "weather",
  "args": ["London", "tomorrow"],
  "raw_args": "London tomorrow",
  "chat": "1234567890@s.whatsapp.net",
  "sender": "1234567890@s.whatsapp.net"
}
```

The request fields are:

| Field | Description |
| --- | --- |
| `command` | The installed plugin name. |
| `args` | Arguments split on whitespace. |
| `raw_args` | The complete argument text after the command name. |
| `chat` | JID of the WhatsApp chat where the command was sent. |
| `sender` | JID of the person who sent the command. |

The plugin must write the reply to standard output. whatsrook trims the output and sends it as a text reply to the current WhatsApp chat.

For example:

```text
Weather for London: 18°C, cloudy.
```

Only standard output is used as the reply. Diagnostic messages should be written to standard error. Standard error is logged by whatsrook and is not sent to the chat.

## Complete Go Example

Create a directory for the plugin:

```bash
mkdir weather-plugin
cd weather-plugin
```

Create `main.go`:

```go
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type Request struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
	RawArgs string   `json:"raw_args"`
	Chat    string   `json:"chat"`
	Sender  string   `json:"sender"`
}

func main() {
	var request Request
	if err := json.NewDecoder(os.Stdin).Decode(&request); err != nil {
		fmt.Fprintln(os.Stderr, "invalid whatsrook request:", err)
		os.Exit(1)
	}

	city := strings.TrimSpace(request.RawArgs)
	if city == "" {
		fmt.Fprintln(os.Stdout, "Usage: .weather <city>")
		return
	}

	// Replace this with a real weather API request.
	fmt.Fprintf(os.Stdout, "Weather lookup requested for %s.", city)
}
```

Build it:

```bash
go build -o weather-plugin .
```

Install it from WhatsApp:

```text
.install weather /absolute/path/to/weather-plugin
```

Use it:

```text
.weather London
```

The example replies:

```text
Weather lookup requested for London.
```

## Building for Another Platform

Build the plugin for the platform where whatsrook runs. A binary built for the wrong operating system or architecture cannot be executed.

Linux AMD64:

```bash
GOOS=linux GOARCH=amd64 go build -o weather-plugin .
```

Linux ARM64:

```bash
GOOS=linux GOARCH=arm64 go build -o weather-plugin .
```

macOS Apple Silicon:

```bash
GOOS=darwin GOARCH=arm64 go build -o weather-plugin .
```

Windows AMD64:

```bash
GOOS=windows GOARCH=amd64 go build -o weather-plugin.exe .
```

## Storage Location

By default, plugins are stored in a `plugins` directory inside the whatsrook data directory:

```text
<whatsrook-data-directory>/plugins/
```

Set `WHATSROOK_PLUGIN_DIR` to use a different directory:

```bash
WHATSROOK_PLUGIN_DIR=/opt/whatsrook/plugins whatsrook
```

The directory is created with restricted permissions when the first plugin is installed. Each installed plugin has a corresponding metadata file with a `.json` suffix.

## Runtime Limits and Security

External plugins run with the same operating-system permissions as the whatsrook process. Only install binaries you trust.

The plugin system applies these protections and limits:

- Installation and execution are restricted to the bot owner and sudoers.
- Plugin names are validated and cannot contain path separators.
- Remote downloads support only HTTP and HTTPS.
- A plugin is limited to 64 MiB during installation.
- A plugin process is stopped after 30 seconds.
- Existing built-in commands take precedence over external plugins with the same name.
- External plugins are started as separate processes and do not receive direct access to the WhatsApp client or whatsrook internals.

External plugins can execute arbitrary code available to the operating-system user running whatsrook. Review source code and release artifacts before installing them.

## Limitations

The current protocol is intentionally small. An external plugin can:

- Receive command arguments and basic chat metadata.
- Read a JSON request from standard input.
- Return a text reply through standard output.

An external plugin cannot currently:

- Register multiple native whatsrook commands.
- Receive arbitrary WhatsApp events.
- Use the native `Context` or `WARook` APIs.
- Send images, audio, documents, polls, reactions, or other rich messages directly.
- Persist settings through the whatsrook database.

Use an in-tree Go plugin when deeper integration with whatsrook APIs is required.
