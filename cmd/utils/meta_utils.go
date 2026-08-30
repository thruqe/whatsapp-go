// Shell command execution helpers used internally by Meta AI tool invocations.
package cliutils

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
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
