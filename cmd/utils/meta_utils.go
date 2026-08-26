// Shell command execution helpers used internally by Meta AI tool invocations.
package cliutils

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
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
// not limited to `curl`, `rm`, `wget`, `nc`, `dd`, interpreters, etc.
// Any upstream caller, prompt injection, or identity confusion flaw
// that allows an attacker to invoke this function with an arbitrary
// command string achieves FULL PROCESS TAKEOVER on this machine.
//
// All other mechanisms that grant shell access to sudoers (the `.sh`
// command, the interactive shell session, the `device/status`
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
	input = strings.TrimSpace(input)
	if input == "" {
		return "", fmt.Errorf("empty command")
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		psScript := "[Console]::OutputEncoding = [System.Text.Encoding]::UTF8; " + input
		if pwshPath, err := exec.LookPath("pwsh"); err == nil {
			cmd = exec.Command(pwshPath, "-NoProfile", "-NonInteractive", "-Command", psScript)
		} else if psPath, err := exec.LookPath("powershell"); err == nil {
			cmd = exec.Command(psPath, "-NoProfile", "-NonInteractive", "-Command", psScript)
		} else {
			cmd = exec.Command("cmd.exe", "/c", input)
		}
	} else {
		shell := "bash"
		if _, err := exec.LookPath("bash"); err != nil {
			shell = "sh"
		}
		cmd = exec.Command(shell, "-c", input)
	}

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
