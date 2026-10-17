package main

import (
	"bytes"
	"debug/elf"
	"debug/macho"
	"encoding/binary"
	"errors"
	"fmt"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const r2LogoURL = "https://pub-687f7ee583cf4f339cab34d2b3a3871d.r2.dev/logo.png"

func ensureLogoPNG(rootDir string) (string, []byte, error) {
	embeddedPath := filepath.Join(rootDir, "bot", "assets", "logo.png")
	if data, err := os.ReadFile(embeddedPath); err == nil && len(data) > 0 {
		if _, err := png.DecodeConfig(bytes.NewReader(data)); err == nil {
			return embeddedPath, data, nil
		}
	}

	tmpIconPath := filepath.Join(rootDir, "tmp", "logo.png")
	if data, err := os.ReadFile(tmpIconPath); err == nil && len(data) > 0 {
		if _, err := png.DecodeConfig(bytes.NewReader(data)); err == nil {
			_ = os.MkdirAll(filepath.Dir(embeddedPath), 0755)
			_ = os.WriteFile(embeddedPath, data, 0644)
			return embeddedPath, data, nil
		}
	}

	fmt.Printf("Fetching application icon from %s...\n", r2LogoURL)
	resp, err := http.Get(r2LogoURL)
	if err != nil {
		return "", nil, fmt.Errorf("failed to fetch icon from R2: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("failed to fetch icon from R2: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, fmt.Errorf("failed to read icon data: %w", err)
	}
	if _, err := png.DecodeConfig(bytes.NewReader(data)); err != nil {
		return "", nil, fmt.Errorf("downloaded icon is not a valid PNG: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(embeddedPath), 0755); err != nil {
		return "", nil, fmt.Errorf("failed to create bot/assets dir: %w", err)
	}
	if err := os.WriteFile(embeddedPath, data, 0644); err != nil {
		return "", nil, fmt.Errorf("failed to write icon to %s: %w", embeddedPath, err)
	}
	return embeddedPath, data, nil
}

func runRes(args []string) error {
	rootDir, err := findRepoRoot()
	if err != nil {
		return fmt.Errorf("failed to locate repo root: %w", err)
	}

	now := time.Now()
	productVersion := ""
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		productVersion = strings.TrimPrefix(strings.TrimSpace(args[0]), "v")
	}
	if productVersion == "" {
		yy := now.Year() % 100
		mm := int(now.Month())
		sha := getGitShortSHA(rootDir)
		productVersion = fmt.Sprintf("%02d.%02d.%s", yy, mm, sha)
	}

	// Windows PE --file-version must be 4 numeric components (e.g. 26.9.0.0)
	parts := strings.Split(productVersion, ".")
	var yr, mo int
	if len(parts) >= 2 {
		yr, _ = strconv.Atoi(parts[0])
		mo, _ = strconv.Atoi(parts[1])
	}
	if yr == 0 {
		yr = now.Year() % 100
	}
	if mo == 0 {
		mo = int(now.Month())
	}
	fileVersion := fmt.Sprintf("%d.%d.0.0", yr, mo)

	iconPath, _, err := ensureLogoPNG(rootDir)
	if err != nil {
		return err
	}

	cmdDir := filepath.Join(rootDir, "bot")
	year := time.Now().Year()

	fmt.Printf("Generating binary resources (Product Version: %s, File Version: %s, Icon: %s)...\n", productVersion, fileVersion, filepath.Base(iconPath))

	cmd := exec.Command("go", "run", "github.com/tc-hib/go-winres@latest", "simply",
		"--icon", iconPath,
		"--manifest", "cli",
		"--product-name", "WhatsRook",
		"--file-description", "WhatsApp Automation Client",
		"--copyright", fmt.Sprintf("Copyright © %d Thruqe", year),
		"--original-filename", "whatsrook.exe",
		"--file-version", fileVersion,
		"--product-version", productVersion,
		"--arch", "amd64,arm64,386,arm",
	)
	// Ensure the go-winres invocation uses the native host toolchain
	var hostEnv []string
	for _, env := range os.Environ() {
		if strings.HasPrefix(env, "GOOS=") || strings.HasPrefix(env, "GOARCH=") {
			continue
		}
		hostEnv = append(hostEnv, env)
	}
	hostEnv = append(hostEnv, "GOOS=", "GOARCH=")
	cmd.Env = hostEnv
	cmd.Dir = cmdDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go-winres failed: %w", err)
	}

	fmt.Println("✓ Windows PE resources generated successfully.")
	return nil
}

// runIcon applies OS-specific icon integration to a compiled binary and deterministically verifies it.
func runIcon(args []string) error {
	rootDir, err := findRepoRoot()
	if err != nil {
		return fmt.Errorf("failed to locate repo root: %w", err)
	}

	_, logoPNG, err := ensureLogoPNG(rootDir)
	if err != nil {
		return err
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(logoPNG))
	if err != nil {
		return fmt.Errorf("invalid logo.png: %w", err)
	}

	targetBin := filepath.Join(rootDir, "bin", "whatsrook")
	if runtime.GOOS == "windows" {
		targetBin += ".exe"
	}
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		targetBin = strings.TrimSpace(args[0])
		if !filepath.IsAbs(targetBin) {
			if _, err := os.Stat(targetBin); err != nil {
				candidate := filepath.Join(rootDir, targetBin)
				if _, err2 := os.Stat(candidate); err2 == nil {
					targetBin = candidate
				}
			}
		}
	}

	absBin, err := filepath.Abs(targetBin)
	if err != nil {
		return fmt.Errorf("failed to resolve binary path %s: %w", targetBin, err)
	}

	f, err := os.Open(absBin)
	if err != nil {
		return fmt.Errorf("failed to open binary %s: %w", absBin, err)
	}
	header := make([]byte, 4)
	_, readErr := f.Read(header)
	_ = f.Close()
	if readErr != nil {
		return fmt.Errorf("failed to read header from %s: %w", absBin, readErr)
	}

	switch {
	case header[0] == 'M' && header[1] == 'Z':
		return verifyWindowsBinaryIcon(absBin, cfg.Width, cfg.Height)
	case bytes.Equal(header, []byte{0x7f, 'E', 'L', 'F'}):
		return applyAndVerifyLinuxBinaryIcon(absBin, logoPNG, cfg.Width, cfg.Height)
	case isMachOHeader(header):
		return applyAndVerifyDarwinBinaryIcon(absBin, logoPNG, cfg.Width, cfg.Height)
	default:
		return fmt.Errorf("unsupported binary header %x in %s", header, absBin)
	}
}

func isMachOHeader(h []byte) bool {
	if len(h) < 4 {
		return false
	}
	switch binary.BigEndian.Uint32(h) {
	case 0xfeedface, 0xfeedfacf, 0xcefaedfe, 0xcffaedfe, 0xcafebabe:
		return true
	default:
		return false
	}
}

func extractPERsrcSection(data []byte) ([]byte, error) {
	if len(data) < 0x40 || data[0] != 'M' || data[1] != 'Z' {
		return nil, errors.New("invalid DOS MZ header")
	}
	peOffset := int(binary.LittleEndian.Uint32(data[0x3c:0x40]))
	if peOffset <= 0 || peOffset+24 > len(data) {
		return nil, errors.New("invalid PE header offset")
	}
	if string(data[peOffset:peOffset+4]) != "PE\x00\x00" {
		return nil, errors.New("missing PE signature")
	}
	coffOffset := peOffset + 4
	numSections := int(binary.LittleEndian.Uint16(data[coffOffset+2 : coffOffset+4]))
	optHeaderSize := int(binary.LittleEndian.Uint16(data[coffOffset+16 : coffOffset+18]))
	secTableOffset := coffOffset + 20 + optHeaderSize

	for i := range numSections {
		secOffset := secTableOffset + i*40
		if secOffset+40 > len(data) {
			break
		}
		name := strings.TrimRight(string(data[secOffset:secOffset+8]), "\x00")
		rawSize := int(binary.LittleEndian.Uint32(data[secOffset+16 : secOffset+20]))
		rawPtr := int(binary.LittleEndian.Uint32(data[secOffset+20 : secOffset+24]))
		if name == ".rsrc" {
			if rawPtr <= 0 || rawSize <= 0 || rawPtr >= len(data) {
				return nil, errors.New("invalid .rsrc section bounds")
			}
			end := min(rawPtr+rawSize, len(data))
			return data[rawPtr:end], nil
		}
	}
	return nil, errors.New("missing .rsrc resource section")
}

func hasPEIconResourceDir(rsrcData []byte) bool {
	if len(rsrcData) < 16 {
		return false
	}
	namedEntries := int(binary.LittleEndian.Uint16(rsrcData[12:14]))
	idEntries := int(binary.LittleEndian.Uint16(rsrcData[14:16]))
	total := namedEntries + idEntries
	if total <= 0 || total > 64 || 16+total*8 > len(rsrcData) {
		return false
	}
	var hasIcon, hasGroupIcon bool
	for i := range total {
		entryOffset := 16 + i*8
		id := binary.LittleEndian.Uint32(rsrcData[entryOffset : entryOffset+4])
		if id == 3 {
			hasIcon = true
		} else if id == 14 {
			hasGroupIcon = true
		}
	}
	return hasIcon && hasGroupIcon
}

func verifyWindowsBinaryIcon(absBin string, width, height int) error {
	binData, err := os.ReadFile(absBin)
	if err != nil {
		return fmt.Errorf("failed to read windows PE binary %s: %w", absBin, err)
	}
	rsrcData, err := extractPERsrcSection(binData)
	if err != nil {
		return fmt.Errorf("windows PE binary %s icon verification failed: %w", absBin, err)
	}

	hasPNG := bytes.Contains(rsrcData, []byte("\x89PNG\r\n\x1a\n"))
	hasDIB := bytes.Contains(rsrcData, []byte{0x28, 0x00, 0x00, 0x00})
	hasIconDir := hasPEIconResourceDir(rsrcData)
	if !hasPNG && !hasDIB && !hasIconDir {
		return fmt.Errorf("windows PE .rsrc section (%d bytes) in %s does not contain an embedded icon payload", len(rsrcData), absBin)
	}

	fmt.Printf("✓ Verified Windows PE icon on %s (.rsrc=%d bytes, RT_ICON=%v, PNG=%v, DIB=%v, source=%dx%d)\n",
		absBin, len(rsrcData), hasIconDir, hasPNG, hasDIB, width, height)
	return nil
}

func findWorkingGIO() (string, []string) {
	cleanEnv := make([]string, 0, len(os.Environ()))
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "GIO_MODULE_DIR=") || strings.HasPrefix(kv, "GIO_EXTRA_MODULES=") {
			continue
		}
		cleanEnv = append(cleanEnv, kv)
	}
	for _, candidate := range []string{"/usr/bin/gio", "/bin/gio"} {
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			return candidate, cleanEnv
		}
	}
	if p, err := exec.LookPath("gio"); err == nil {
		return p, cleanEnv
	}
	return "", cleanEnv
}

func hasActiveGVFS() bool {
	if strings.TrimSpace(os.Getenv("DBUS_SESSION_BUS_ADDRESS")) == "" {
		return false
	}
	for _, p := range []string{
		"/usr/lib64/gio/modules/libgvfsdbus.so",
		"/usr/lib/x86_64-linux-gnu/gio/modules/libgvfsdbus.so",
		"/usr/lib/aarch64-linux-gnu/gio/modules/libgvfsdbus.so",
		"/usr/lib/gio/modules/libgvfsdbus.so",
	} {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return true
		}
	}
	return false
}

func applyAndVerifyLinuxBinaryIcon(absBin string, logoPNG []byte, width, height int) error {
	elfFile, err := elf.Open(absBin)
	if err != nil {
		return fmt.Errorf("failed to parse ELF binary %s: %w", absBin, err)
	}
	_ = elfFile.Close()

	// Verify embedded PNG bytes exist inside the compiled ELF binary itself
	binBytes, err := os.ReadFile(absBin)
	if err != nil {
		return fmt.Errorf("failed to read ELF binary %s: %w", absBin, err)
	}
	if !bytes.Contains(binBytes, logoPNG[:32]) {
		// Note: UPX-compressed binaries compress the rodata section; check UPX magic if not found raw
		if !bytes.Contains(binBytes, []byte("UPX!")) {
			return fmt.Errorf("ELF binary %s does not contain embedded logo.png payload", absBin)
		}
	}

	if runtime.GOOS != "linux" {
		fmt.Printf("✓ Verified Linux ELF binary icon payload in %s (%dx%d)\n", absBin, width, height)
		return nil
	}

	// If targeting Android, desktop icons are not applicable
	if os.Getenv("GOOS") == "android" {
		fmt.Printf("ℹ Skipping desktop icon on Android CLI binary %s\n", absBin)
		return nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil || homeDir == "" {
		return fmt.Errorf("failed to determine user home directory: %w", err)
	}
	dataHome := os.Getenv("XDG_DATA_HOME")
	if strings.TrimSpace(dataHome) == "" {
		dataHome = filepath.Join(homeDir, ".local", "share")
	}

	icon512 := filepath.Join(dataHome, "icons", "hicolor", "512x512", "apps", "whatsrook.png")
	icon256 := filepath.Join(dataHome, "icons", "hicolor", "256x256", "apps", "whatsrook.png")
	dotIcon := filepath.Join(homeDir, ".whatsrook", "logo.png")
	desktopPath := filepath.Join(dataHome, "applications", "whatsrook.desktop")

	for _, dst := range []string{icon512, icon256, dotIcon} {
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return fmt.Errorf("failed to create icon dir %s: %w", filepath.Dir(dst), err)
		}
		if err := os.WriteFile(dst, logoPNG, 0644); err != nil {
			return fmt.Errorf("failed to write icon file %s: %w", dst, err)
		}
	}

	if err := os.MkdirAll(filepath.Dir(desktopPath), 0755); err != nil {
		return fmt.Errorf("failed to create applications dir: %w", err)
	}
	escapedExec := strings.ReplaceAll(absBin, `"`, `\"`)
	desktopContent := fmt.Sprintf("[Desktop Entry]\nVersion=1.0\nType=Application\nName=WhatsRook\nGenericName=WhatsApp Automation Client\nComment=High-performance WhatsApp automation client and bot\nExec=\"%s\"\nIcon=%s\nTerminal=true\nCategories=Network;InstantMessaging;Chat;\nKeywords=WhatsApp;Bot;WhatsRook;Messaging;\nStartupWMClass=whatsrook\nStartupNotify=false\n",
		escapedExec, icon512)
	if err := os.WriteFile(desktopPath, []byte(desktopContent), 0644); err != nil {
		return fmt.Errorf("failed to write desktop file %s: %w", desktopPath, err)
	}

	if validateBin, err := exec.LookPath("desktop-file-validate"); err == nil && validateBin != "" {
		if out, err := exec.Command(validateBin, desktopPath).CombinedOutput(); err != nil {
			return fmt.Errorf("desktop-file-validate failed for %s: %s (%w)", desktopPath, strings.TrimSpace(string(out)), err)
		}
	}

	iconURI := (&url.URL{Scheme: "file", Path: filepath.ToSlash(icon512)}).String()
	gioBin, gioEnv := findWorkingGIO()
	if gioBin != "" && hasActiveGVFS() {
		setCmd := exec.Command(gioBin, "set", "-t", "string", absBin, "metadata::custom-icon", iconURI)
		setCmd.Env = gioEnv
		if out, err := setCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("gio set metadata::custom-icon failed on %s: %s (%w)", absBin, strings.TrimSpace(string(out)), err)
		}

		infoCmd := exec.Command(gioBin, "info", "-a", "metadata::custom-icon", absBin)
		infoCmd.Env = gioEnv
		out, err := infoCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("gio info verification failed on %s: %s (%w)", absBin, strings.TrimSpace(string(out)), err)
		}
		outStr := string(out)
		if !strings.Contains(outStr, "metadata::custom-icon:") || !strings.Contains(outStr, iconURI) {
			return fmt.Errorf("GNOME GVFS metadata::custom-icon verification failed on %s (expected %s, got:\n%s)", absBin, iconURI, strings.TrimSpace(outStr))
		}
		fmt.Printf("✓ Verified Linux GNOME binary icon on %s\n  • metadata::custom-icon = %s\n  • freedesktop launcher  = %s\n", absBin, iconURI, desktopPath)
		return nil
	}

	fmt.Printf("✓ Verified Linux ELF binary & freedesktop launcher on %s (icon=%s, desktop=%s)\n", absBin, icon512, desktopPath)
	return nil
}

func buildICNS(pngData []byte) ([]byte, error) {
	if len(pngData) == 0 {
		return nil, errors.New("empty PNG data")
	}
	elemLen := uint32(8 + len(pngData))
	totalLen := uint32(8) + elemLen
	buf := make([]byte, totalLen)
	copy(buf[0:4], "icns")
	binary.BigEndian.PutUint32(buf[4:8], totalLen)
	copy(buf[8:12], "ic09")
	binary.BigEndian.PutUint32(buf[12:16], elemLen)
	copy(buf[16:], pngData)
	return buf, nil
}

func applyAndVerifyDarwinBinaryIcon(absBin string, logoPNG []byte, width, height int) error {
	machoFile, err := macho.Open(absBin)
	if err != nil {
		return fmt.Errorf("failed to parse macOS Mach-O binary %s: %w", absBin, err)
	}
	_ = machoFile.Close()

	binBytes, err := os.ReadFile(absBin)
	if err != nil {
		return fmt.Errorf("failed to read Mach-O binary %s: %w", absBin, err)
	}
	if !bytes.Contains(binBytes, logoPNG[:32]) {
		return fmt.Errorf("macOS Mach-O binary %s does not contain embedded logo.png payload", absBin)
	}

	icnsData, err := buildICNS(logoPNG)
	if err != nil {
		return err
	}

	if runtime.GOOS == "darwin" {
		homeDir, _ := os.UserHomeDir()
		icnsPath := filepath.Join(homeDir, ".whatsrook", "whatsrook.icns")
		_ = os.MkdirAll(filepath.Dir(icnsPath), 0755)
		if err := os.WriteFile(icnsPath, icnsData, 0644); err != nil {
			return fmt.Errorf("failed to write macOS icns file: %w", err)
		}
		if osaBin, err := exec.LookPath("osascript"); err == nil && osaBin != "" {
			script := fmt.Sprintf(
				`ObjC.import("AppKit"); var img = $.NSImage.alloc.initWithContentsOfFile(%q); $.NSWorkspace.sharedWorkspace.setIconForFileOptions(img, %q, 0);`,
				icnsPath, absBin,
			)
			if out, err := exec.Command(osaBin, "-l", "JavaScript", "-e", script).CombinedOutput(); err != nil {
				return fmt.Errorf("macOS NSWorkspace setIcon failed: %s (%w)", strings.TrimSpace(string(out)), err)
			}
		}
	}

	fmt.Printf("✓ Verified macOS Mach-O binary icon on %s (embedded PNG=%dx%d, icns=%d bytes)\n",
		absBin, width, height, len(icnsData))
	return nil
}
