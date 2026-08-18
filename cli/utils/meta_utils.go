// Shell command execution helpers used internally by Meta AI tool invocations.
package cliutils

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"go.mau.fi/whatsmeow/types"
	"golang.org/x/term"
)

// RunCmd runs an arbitrary shell command and returns its combined
// stdout+stderr output.
//
// DANGER — ARBITRARY COMMAND EXECUTION:
// This function has no allowlist, no argument validation, and no
// restriction on which binaries can run. Whatever string is passed in
// is split on whitespace and executed directly via exec.Command — the
// first token becomes the program to run, and everything after it
// becomes its arguments, verbatim.
//
// This means ANY command reachable on PATH inside this process's
// environment can be executed, with arbitrary arguments, including but
// not limited to:
//   - destructive operations (deleting files, wiping data)
//   - reading or exfiltrating any file/secret/credential accessible to
//     this process (env vars, API keys, session/auth tokens, database
//     files, etc.)
//   - outbound network requests (data exfiltration, downloading and
//     running additional payloads)
//   - further compromising the container or, if the container has any
//     mounted volumes, shared credentials, or network reach, pivoting
//     beyond it
//
// Restricting WHO can trigger this (e.g. sudo/owner-only checks) does
// NOT make this function itself safe — it only narrows which accounts,
// if compromised (phished, session-hijacked, device malware, leaked
// session file, etc.), grant an attacker this same arbitrary execution
// capability. The owner's WhatsApp session is a single point of failure
// for the entire container the moment this function is reachable from
// it.
//
// There is no safe way to expose this over a remote, message-based
// interface. If you need remote admin capabilities, prefer a fixed,
// allowlisted set of operations (specific binaries + specific argument
// values only) instead of this function.
func RunCmd(input string) (string, error) {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return "", fmt.Errorf("empty command")
	}

	cmd := exec.Command(parts[0], parts[1:]...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// GetChatType returns the chat type based on the JID's server suffix.
func (d *Data) GetChatType() string {
	jid, err := types.ParseJID(d.ChatID)
	if err != nil {
		return "Unknown"
	}
	switch jid.Server {
	case types.GroupServer:
		return "Group"
	case types.DefaultUserServer, types.LegacyUserServer:
		return "User"
	case types.NewsletterServer:
		return "Newsletter"
	case types.BroadcastServer:
		return "Broadcast"
	default:
		return "Unknown"
	}
}

// GetTerminalType returns a best-effort identifier for the terminal our
// process is currently running under.
//
// Resolution order:
//  1. If stdout is not attached to a terminal at all (e.g. output is piped
//     or redirected to a file), it returns "not a terminal".
//  2. If the TERM_PROGRAM environment variable is set (commonly populated
//     by terminal emulators such as iTerm2, Apple Terminal, VS Code's
//     integrated terminal, etc.), that value is returned as it's usually
//     the most human-readable identifier.
//  3. Otherwise it falls back to the TERM environment variable (e.g.
//     "xterm-256color", "screen", "linux"), which is the POSIX-standard
//     way terminals advertise their capabilities.
//  4. If none of the above are set, it returns "unknown".
//
// Note: this is best-effort. TERM/TERM_PROGRAM are set by the terminal
// emulator or shell and can be absent, spoofed, or inaccurate — this
// function does not attempt to query terminal capabilities directly.
func GetTerminalType() string {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return "not a terminal"
	}

	if termProgram := os.Getenv("TERM_PROGRAM"); termProgram != "" {
		return termProgram
	}

	if termType := os.Getenv("TERM"); termType != "" {
		return termType
	}

	return "unknown"
}
