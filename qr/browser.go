package qr

import (
	"fmt"
	"os/exec"
	"runtime"
)

// OpenBrowser attempts to open the specified URL in the default system web browser.
// It executes the platform-specific launcher without blocking.
func OpenBrowser(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux", "freebsd", "openbsd", "netbsd":
		if _, err := exec.LookPath("xdg-open"); err == nil {
			cmd = exec.Command("xdg-open", url)
		} else if _, err := exec.LookPath("x-www-browser"); err == nil {
			cmd = exec.Command("x-www-browser", url)
		} else if _, err := exec.LookPath("www-browser"); err == nil {
			cmd = exec.Command("www-browser", url)
		} else if _, err := exec.LookPath("gio"); err == nil {
			cmd = exec.Command("gio", "open", url)
		} else {
			return fmt.Errorf("no supported browser opener found (xdg-open, x-www-browser, www-browser, gio)")
		}
	case "android":
		if _, err := exec.LookPath("termux-open-url"); err == nil {
			cmd = exec.Command("termux-open-url", url)
		} else if _, err := exec.LookPath("xdg-open"); err == nil {
			cmd = exec.Command("xdg-open", url)
		} else {
			cmd = exec.Command("am", "start", "-a", "android.intent.action.VIEW", "-d", url)
		}
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	return cmd.Start()
}
