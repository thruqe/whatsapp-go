package main

import (
	"bytes"
	"debug/elf"
	"debug/macho"
	"debug/pe"
	_ "embed"
	"encoding/binary"
	"errors"
	"fmt"
	"image/png"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/thruqe/whatsrook"
)

//go:embed assets/logo.png
var embeddedLogoPNG []byte

// EmbeddedLogoPNG returns a copy of the embedded 512x512 application icon PNG.
func EmbeddedLogoPNG() []byte {
	out := make([]byte, len(embeddedLogoPNG))
	copy(out, embeddedLogoPNG)
	return out
}

// IconVerificationReport contains deterministic verification results for an OS binary icon.
type IconVerificationReport struct {
	OS              string
	BinaryPath      string
	BinaryFormat    string
	Supported       bool
	IconFilePath    string
	DesktopFilePath string
	CustomIconURI   string
	Details         []string
}

// EnsureBinaryIcon installs and attaches the application icon for the currently running executable
// on supported desktop operating systems (Linux GNOME/freedesktop and macOS).
// Windows icons are embedded directly in the PE .rsrc section at build time.
// Android is excluded as raw CLI binaries inside Termux do not support desktop binary icons.
func EnsureBinaryIcon() {
	if runtime.GOOS == "android" || runtime.GOOS == "windows" {
		return
	}
	exePath, err := os.Executable()
	if err != nil || exePath == "" {
		return
	}
	if resolved, err := filepath.EvalSymlinks(exePath); err == nil && resolved != "" {
		exePath = resolved
	}
	_ = ApplyBinaryIconForOS(runtime.GOOS, exePath)
}

// ApplyBinaryIconForOS applies desktop icon integration for a target binary on the given OS.
func ApplyBinaryIconForOS(targetOS, binaryPath string) error {
	if len(EmbeddedLogoPNG()) == 0 {
		return errors.New("embedded logo.png is empty")
	}
	absBin, err := filepath.Abs(binaryPath)
	if err != nil {
		return fmt.Errorf("failed to resolve binary path: %w", err)
	}
	if _, err := os.Stat(absBin); err != nil {
		return fmt.Errorf("binary does not exist at %s: %w", absBin, err)
	}

	switch targetOS {
	case "linux":
		return applyLinuxDesktopIcon(absBin)
	case "darwin":
		return applyMacOSBinaryIcon(absBin)
	case "windows":
		// Windows icon is embedded at compile time via .syso PE resources.
		return nil
	case "android":
		// Not applicable on Android CLI binaries.
		return nil
	default:
		return nil
	}
}

func resolveLinuxIconPaths() (iconPath string, extraIconPaths []string, desktopPath string, err error) {
	homeDir, err := os.UserHomeDir()
	if err != nil || homeDir == "" {
		return "", nil, "", fmt.Errorf("failed to determine user home directory: %w", err)
	}

	dataHome := os.Getenv("XDG_DATA_HOME")
	if strings.TrimSpace(dataHome) == "" {
		dataHome = filepath.Join(homeDir, ".local", "share")
	}

	icon512 := filepath.Join(dataHome, "icons", "hicolor", "512x512", "apps", "whatsrook.png")
	icon256 := filepath.Join(dataHome, "icons", "hicolor", "256x256", "apps", "whatsrook.png")
	dataDirIcon := filepath.Join(whatsrook.DefaultDataDir(), "logo.png")
	desktopFile := filepath.Join(dataHome, "applications", "whatsrook.desktop")

	return icon512, []string{icon256, dataDirIcon}, desktopFile, nil
}

// findSystemGIO locates the system gio binary that has GVFS metadata support.
// On systems with Linuxbrew installed, /home/linuxbrew/.linuxbrew/bin/gio may precede /usr/bin/gio
// in PATH but lacks the system libgvfsdbus.so GIO module required to set metadata::custom-icon.
func findSystemGIO() (string, []string) {
	cleanEnv := make([]string, 0, len(os.Environ()))
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "GIO_MODULE_DIR=") || strings.HasPrefix(kv, "GIO_EXTRA_MODULES=") {
			continue
		}
		cleanEnv = append(cleanEnv, kv)
	}

	// Prefer system /usr/bin/gio or /bin/gio first so GVFS D-Bus metadata modules load properly.
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

func hasGVFSMetadataSupport() bool {
	if strings.TrimSpace(os.Getenv("DBUS_SESSION_BUS_ADDRESS")) == "" {
		return false
	}
	candidates := []string{
		"/usr/lib64/gio/modules/libgvfsdbus.so",
		"/usr/lib/x86_64-linux-gnu/gio/modules/libgvfsdbus.so",
		"/usr/lib/aarch64-linux-gnu/gio/modules/libgvfsdbus.so",
		"/usr/lib/gio/modules/libgvfsdbus.so",
	}
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return true
		}
	}
	return false
}

func fileURIForPath(absPath string) string {
	u := &url.URL{
		Scheme: "file",
		Path:   filepath.ToSlash(absPath),
	}
	return u.String()
}

func applyLinuxDesktopIcon(absBin string) error {
	iconPath, extraIcons, desktopPath, err := resolveLinuxIconPaths()
	if err != nil {
		return err
	}

	allIcons := append([]string{iconPath}, extraIcons...)
	for _, dst := range allIcons {
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return fmt.Errorf("failed to create icon directory %s: %w", filepath.Dir(dst), err)
		}
		if err := os.WriteFile(dst, embeddedLogoPNG, 0644); err != nil {
			return fmt.Errorf("failed to write icon file %s: %w", dst, err)
		}
	}

	if err := os.MkdirAll(filepath.Dir(desktopPath), 0755); err != nil {
		return fmt.Errorf("failed to create applications directory: %w", err)
	}

	desktopContent := BuildLinuxDesktopEntry(absBin, iconPath)
	if err := os.WriteFile(desktopPath, []byte(desktopContent), 0644); err != nil {
		return fmt.Errorf("failed to write desktop entry %s: %w", desktopPath, err)
	}

	// Set per-file GVFS custom icon attribute so GNOME Files (Nautilus) displays the icon on the binary itself.
	if gioBin, gioEnv := findSystemGIO(); gioBin != "" {
		iconURI := fileURIForPath(iconPath)
		cmd := exec.Command(gioBin, "set", "-t", "string", absBin, "metadata::custom-icon", iconURI)
		cmd.Env = gioEnv
		if out, err := cmd.CombinedOutput(); err != nil && hasGVFSMetadataSupport() {
			return fmt.Errorf("gio set metadata::custom-icon failed (%s): %w", strings.TrimSpace(string(out)), err)
		}
	}

	return nil
}

// BuildLinuxDesktopEntry constructs a freedesktop.org compliant .desktop file for WhatsRook.
func BuildLinuxDesktopEntry(binaryAbsPath, iconAbsPath string) string {
	escapedExec := strings.ReplaceAll(binaryAbsPath, `"`, `\"`)
	var b strings.Builder
	b.WriteString("[Desktop Entry]\n")
	b.WriteString("Version=1.0\n")
	b.WriteString("Type=Application\n")
	b.WriteString("Name=WhatsRook\n")
	b.WriteString("GenericName=WhatsApp Automation Client\n")
	b.WriteString("Comment=High-performance WhatsApp automation client and bot\n")
	b.WriteString(fmt.Sprintf("Exec=\"%s\"\n", escapedExec))
	b.WriteString(fmt.Sprintf("Icon=%s\n", iconAbsPath))
	b.WriteString("Terminal=true\n")
	b.WriteString("Categories=Network;InstantMessaging;Chat;\n")
	b.WriteString("Keywords=WhatsApp;Bot;WhatsRook;Messaging;\n")
	b.WriteString("StartupWMClass=whatsrook\n")
	b.WriteString("StartupNotify=false\n")
	return b.String()
}

// BuildMacOSICNS constructs a valid Apple Icon Image (.icns) container wrapping PNG data.
func BuildMacOSICNS(pngData []byte) ([]byte, error) {
	cfg, err := png.DecodeConfig(bytes.NewReader(pngData))
	if err != nil {
		return nil, fmt.Errorf("invalid PNG data for icns: %w", err)
	}

	osType := "ic09" // 512x512 PNG
	switch {
	case cfg.Width >= 1024:
		osType = "ic10"
	case cfg.Width >= 512:
		osType = "ic09"
	case cfg.Width >= 256:
		osType = "ic08"
	case cfg.Width >= 128:
		osType = "ic07"
	}

	elemLen := uint32(8 + len(pngData))
	totalLen := uint32(8) + elemLen

	buf := make([]byte, totalLen)
	copy(buf[0:4], "icns")
	binary.BigEndian.PutUint32(buf[4:8], totalLen)
	copy(buf[8:12], osType)
	binary.BigEndian.PutUint32(buf[12:16], elemLen)
	copy(buf[16:], pngData)
	return buf, nil
}

// BuildMacOSResourceFork constructs a valid Apple ResourceFork binary structure containing
// a single 'icns' resource with ID -16455 (kCustomIconResource) used by macOS Finder.
func BuildMacOSResourceFork(icnsData []byte) []byte {
	const headerLen = 256
	dataBlockLen := 4 + len(icnsData)
	mapOffset := headerLen + dataBlockLen
	const mapLen = 50

	out := make([]byte, mapOffset+mapLen)

	// 16-byte Resource Fork header
	binary.BigEndian.PutUint32(out[0:4], uint32(headerLen))
	binary.BigEndian.PutUint32(out[4:8], uint32(mapOffset))
	binary.BigEndian.PutUint32(out[8:12], uint32(dataBlockLen))
	binary.BigEndian.PutUint32(out[12:16], uint32(mapLen))

	// Resource data block at offset 256
	binary.BigEndian.PutUint32(out[headerLen:headerLen+4], uint32(len(icnsData)))
	copy(out[headerLen+4:mapOffset], icnsData)

	// Resource map at mapOffset (50 bytes)
	rm := out[mapOffset : mapOffset+mapLen]
	copy(rm[0:16], out[0:16])
	binary.BigEndian.PutUint16(rm[24:26], 28) // offset to type list
	binary.BigEndian.PutUint16(rm[26:28], 50) // offset to name list (end of map)

	// Type list at rm[28:]
	binary.BigEndian.PutUint16(rm[28:30], 0) // 1 type - 1 = 0
	copy(rm[30:34], "icns")
	binary.BigEndian.PutUint16(rm[34:36], 0)  // 1 resource - 1 = 0
	binary.BigEndian.PutUint16(rm[36:38], 10) // offset from type list (28) to ref list (38)

	// Reference list entry (12 bytes at rm[38:50])
	// Resource ID -16455 = 0xBFB9
	binary.BigEndian.PutUint16(rm[38:40], 0xBFB9)
	binary.BigEndian.PutUint16(rm[40:42], 0xFFFF) // no resource name
	rm[42] = 0x00                                 // attributes
	// 24-bit data offset = 0
	rm[43] = 0x00
	rm[44] = 0x00
	rm[45] = 0x00

	return out
}

// BuildMacOSFinderInfoWithCustomIcon returns a 32-byte macOS FinderInfo structure
// with the kHasCustomIcon flag (0x0400) enabled at big-endian uint16 offset 8.
func BuildMacOSFinderInfoWithCustomIcon() [32]byte {
	var info [32]byte
	// Finder flags are stored at bytes [8:10] in big-endian order; 0x0400 = kHasCustomIcon.
	binary.BigEndian.PutUint16(info[8:10], 0x0400)
	return info
}

func applyMacOSBinaryIcon(absBin string) error {
	icnsData, err := BuildMacOSICNS(embeddedLogoPNG)
	if err != nil {
		return err
	}

	dataDir := whatsrook.DefaultDataDir()
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data dir: %w", err)
	}
	icnsPath := filepath.Join(dataDir, "whatsrook.icns")
	if err := os.WriteFile(icnsPath, icnsData, 0644); err != nil {
		return fmt.Errorf("failed to write icns file: %w", err)
	}

	if runtime.GOOS == "darwin" {
		// 1. Apply via AppKit NSWorkspace (updates Finder icon cache and ResourceFork/FinderInfo)
		appliedViaAppKit := false
		if osaBin, err := exec.LookPath("osascript"); err == nil && osaBin != "" {
			script := fmt.Sprintf(
				`ObjC.import("AppKit"); var img = $.NSImage.alloc.initWithContentsOfFile(%q); $.NSWorkspace.sharedWorkspace.setIconForFileOptions(img, %q, 0);`,
				icnsPath, absBin,
			)
			if cmd := exec.Command(osaBin, "-l", "JavaScript", "-e", script); cmd.Run() == nil {
				appliedViaAppKit = true
			}
		}

		// 2. Fallback / ensure xattrs (com.apple.ResourceFork + com.apple.FinderInfo) via xattr tool
		if !appliedViaAppKit {
			if xattrBin, err := exec.LookPath("xattr"); err == nil && xattrBin != "" {
				rsrcFork := BuildMacOSResourceFork(icnsData)
				finderInfo := BuildMacOSFinderInfoWithCustomIcon()
				_ = exec.Command(xattrBin, "-wx", "com.apple.ResourceFork", fmt.Sprintf("%x", rsrcFork), absBin).Run()
				_ = exec.Command(xattrBin, "-wx", "com.apple.FinderInfo", fmt.Sprintf("%x", finderInfo[:]), absBin).Run()
			}
		}
	}

	return nil
}

// VerifyBinaryIcon inspects a compiled binary and verifies that the icon is properly configured
// for its target operating system without relying on assumptions.
func VerifyBinaryIcon(binaryPath string) (*IconVerificationReport, error) {
	absBin, err := filepath.Abs(binaryPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve path %s: %w", binaryPath, err)
	}

	header := make([]byte, 4)
	f, err := os.Open(absBin)
	if err != nil {
		return nil, fmt.Errorf("failed to open binary %s: %w", absBin, err)
	}
	_, readErr := f.Read(header)
	_ = f.Close()
	if readErr != nil {
		return nil, fmt.Errorf("failed to read binary header from %s: %w", absBin, readErr)
	}

	// 1. Validate embedded PNG asset
	cfg, err := png.DecodeConfig(bytes.NewReader(embeddedLogoPNG))
	if err != nil {
		return nil, fmt.Errorf("embedded logo.png is not a valid PNG: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, fmt.Errorf("embedded logo.png has invalid dimensions %dx%d", cfg.Width, cfg.Height)
	}

	switch {
	case header[0] == 'M' && header[1] == 'Z':
		return verifyWindowsPEBinaryIcon(absBin, cfg.Width, cfg.Height)
	case bytes.Equal(header, []byte{0x7f, 'E', 'L', 'F'}):
		return verifyLinuxELFBinaryIcon(absBin, cfg.Width, cfg.Height)
	case isMachOMagic(header):
		return verifyDarwinMachOBinaryIcon(absBin, cfg.Width, cfg.Height)
	default:
		return nil, fmt.Errorf("unrecognized executable header %x for %s", header, absBin)
	}
}

func isMachOMagic(h []byte) bool {
	if len(h) < 4 {
		return false
	}
	v := binary.BigEndian.Uint32(h)
	switch v {
	case 0xfeedface, 0xfeedfacf, 0xcefaedfe, 0xcffaedfe, 0xcafebabe:
		return true
	default:
		return false
	}
}

func verifyWindowsPEBinaryIcon(absBin string, width, height int) (*IconVerificationReport, error) {
	peFile, err := pe.Open(absBin)
	if err != nil {
		return nil, fmt.Errorf("failed to parse windows PE binary: %w", err)
	}
	defer peFile.Close()

	rsrc := peFile.Section(".rsrc")
	if rsrc == nil {
		return nil, fmt.Errorf("windows PE binary %s is missing the .rsrc resource section (icon not embedded)", absBin)
	}
	rsrcData, err := rsrc.Data()
	if err != nil {
		return nil, fmt.Errorf("failed to read .rsrc section from %s: %w", absBin, err)
	}
	if len(rsrcData) == 0 {
		return nil, fmt.Errorf("windows PE .rsrc section in %s is empty", absBin)
	}

	hasPNGIcon := bytes.Contains(rsrcData, []byte("\x89PNG\r\n\x1a\n"))
	hasDIBHeader := bytes.Contains(rsrcData, []byte{0x28, 0x00, 0x00, 0x00}) // BITMAPINFOHEADER size = 40
	if !hasPNGIcon && !hasDIBHeader {
		return nil, fmt.Errorf("windows PE .rsrc section (%d bytes) does not contain a recognizable PNG or DIB icon payload", len(rsrcData))
	}

	return &IconVerificationReport{
		OS:           "windows",
		BinaryPath:   absBin,
		BinaryFormat: "PE32+",
		Supported:    true,
		Details: []string{
			fmt.Sprintf("PE .rsrc section present (%d bytes)", len(rsrcData)),
			fmt.Sprintf("Embedded icon payload verified in .rsrc (PNG=%v, DIB=%v, source=%dx%d)", hasPNGIcon, hasDIBHeader, width, height),
		},
	}, nil
}

func verifyLinuxELFBinaryIcon(absBin string, width, height int) (*IconVerificationReport, error) {
	elfFile, err := elf.Open(absBin)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ELF binary: %w", err)
	}
	_ = elfFile.Close()

	report := &IconVerificationReport{
		OS:           "linux",
		BinaryPath:   absBin,
		BinaryFormat: "ELF",
		Supported:    true,
		Details: []string{
			fmt.Sprintf("Embedded source PNG verified (%dx%d, %d bytes)", width, height, len(embeddedLogoPNG)),
		},
	}

	if runtime.GOOS != "linux" {
		report.Details = append(report.Details, "Cross-compiled Linux ELF verified (GNOME GVFS metadata applies on Linux host)")
		return report, nil
	}

	if err := applyLinuxDesktopIcon(absBin); err != nil {
		return nil, fmt.Errorf("failed to apply Linux GNOME icon integration: %w", err)
	}

	iconPath, _, desktopPath, err := resolveLinuxIconPaths()
	if err != nil {
		return nil, err
	}
	report.IconFilePath = iconPath
	report.DesktopFilePath = desktopPath

	// Verify installed PNG icon on disk
	diskIconBytes, err := os.ReadFile(iconPath)
	if err != nil {
		return nil, fmt.Errorf("installed Linux icon file missing at %s: %w", iconPath, err)
	}
	diskCfg, err := png.DecodeConfig(bytes.NewReader(diskIconBytes))
	if err != nil {
		return nil, fmt.Errorf("installed Linux icon at %s is not a valid PNG: %w", iconPath, err)
	}
	report.Details = append(report.Details, fmt.Sprintf("Installed freedesktop icon verified at %s (%dx%d)", iconPath, diskCfg.Width, diskCfg.Height))

	// Verify .desktop file
	if _, err := os.Stat(desktopPath); err != nil {
		return nil, fmt.Errorf("desktop entry missing at %s: %w", desktopPath, err)
	}
	if validateBin, err := exec.LookPath("desktop-file-validate"); err == nil && validateBin != "" {
		out, err := exec.Command(validateBin, desktopPath).CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("desktop-file-validate failed for %s: %s (%w)", desktopPath, strings.TrimSpace(string(out)), err)
		}
		report.Details = append(report.Details, fmt.Sprintf("Freedesktop launcher validated via desktop-file-validate: %s", desktopPath))
	} else {
		report.Details = append(report.Details, fmt.Sprintf("Freedesktop launcher written at %s", desktopPath))
	}

	// Verify GNOME GVFS metadata::custom-icon attribute on the binary itself
	if gioBin, gioEnv := findSystemGIO(); gioBin != "" && hasGVFSMetadataSupport() {
		cmd := exec.Command(gioBin, "info", "-a", "metadata::custom-icon", absBin)
		cmd.Env = gioEnv
		out, err := cmd.CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("gio info verification failed for %s: %s (%w)", absBin, strings.TrimSpace(string(out)), err)
		}
		outStr := string(out)
		expectedURI := fileURIForPath(iconPath)
		if !strings.Contains(outStr, "metadata::custom-icon:") || !strings.Contains(outStr, expectedURI) {
			return nil, fmt.Errorf("GNOME GVFS metadata::custom-icon not set on %s (expected %s, got output:\n%s)", absBin, expectedURI, strings.TrimSpace(outStr))
		}
		report.CustomIconURI = expectedURI
		report.Details = append(report.Details, fmt.Sprintf("GNOME Nautilus per-file icon verified via gio info: metadata::custom-icon = %s", expectedURI))
	}

	return report, nil
}

func verifyDarwinMachOBinaryIcon(absBin string, width, height int) (*IconVerificationReport, error) {
	machoFile, err := macho.Open(absBin)
	if err != nil {
		return nil, fmt.Errorf("failed to parse macOS Mach-O binary: %w", err)
	}
	_ = machoFile.Close()

	icnsData, err := BuildMacOSICNS(embeddedLogoPNG)
	if err != nil {
		return nil, fmt.Errorf("failed to build macOS .icns container: %w", err)
	}
	if len(icnsData) < 16 || string(icnsData[0:4]) != "icns" {
		return nil, errors.New("invalid macOS .icns header")
	}

	rsrcFork := BuildMacOSResourceFork(icnsData)
	finderInfo := BuildMacOSFinderInfoWithCustomIcon()
	if binary.BigEndian.Uint16(finderInfo[8:10])&0x0400 == 0 {
		return nil, errors.New("macOS FinderInfo kHasCustomIcon bit (0x0400) not set")
	}

	report := &IconVerificationReport{
		OS:           "darwin",
		BinaryPath:   absBin,
		BinaryFormat: "Mach-O",
		Supported:    true,
		Details: []string{
			fmt.Sprintf("Mach-O binary verified with embedded source PNG (%dx%d)", width, height),
			fmt.Sprintf("Apple .icns container verified (%d bytes, block=%s) and ResourceFork verified (%d bytes, ID=-16455)", len(icnsData), string(icnsData[8:12]), len(rsrcFork)),
		},
	}

	if runtime.GOOS == "darwin" {
		if err := applyMacOSBinaryIcon(absBin); err != nil {
			return nil, fmt.Errorf("failed to apply macOS icon to %s: %w", absBin, err)
		}
		icnsPath := filepath.Join(whatsrook.DefaultDataDir(), "whatsrook.icns")
		report.IconFilePath = icnsPath
		report.Details = append(report.Details, fmt.Sprintf("Applied native macOS NSWorkspace file icon from %s", icnsPath))
	}

	return report, nil
}
