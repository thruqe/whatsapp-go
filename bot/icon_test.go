package main

import (
	"bytes"
	"encoding/binary"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEmbeddedLogoPNG(t *testing.T) {
	data := EmbeddedLogoPNG()
	if len(data) == 0 {
		t.Fatal("expected embedded logo.png to be non-empty")
	}

	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("failed to decode embedded logo.png: %v", err)
	}
	if cfg.Width != 512 || cfg.Height != 512 {
		t.Fatalf("expected 512x512 logo.png, got %dx%d", cfg.Width, cfg.Height)
	}
}

func TestBuildLinuxDesktopEntry(t *testing.T) {
	entry := BuildLinuxDesktopEntry("/opt/whatsrook/bin/whatsrook", "/home/user/.local/share/icons/hicolor/512x512/apps/whatsrook.png")
	required := []string{
		"[Desktop Entry]",
		"Type=Application",
		"Name=WhatsRook",
		`Exec="/opt/whatsrook/bin/whatsrook"`,
		"Icon=/home/user/.local/share/icons/hicolor/512x512/apps/whatsrook.png",
		"Terminal=true",
	}
	for _, req := range required {
		if !strings.Contains(entry, req) {
			t.Errorf("desktop entry missing %q:\n%s", req, entry)
		}
	}
}

func TestBuildMacOSICNSAndResourceFork(t *testing.T) {
	pngData := EmbeddedLogoPNG()
	icnsData, err := BuildMacOSICNS(pngData)
	if err != nil {
		t.Fatalf("BuildMacOSICNS failed: %v", err)
	}

	if string(icnsData[0:4]) != "icns" {
		t.Fatalf("expected 'icns' magic, got %q", string(icnsData[0:4]))
	}
	totalLen := binary.BigEndian.Uint32(icnsData[4:8])
	if int(totalLen) != len(icnsData) {
		t.Fatalf("expected icns header length %d to match slice length %d", totalLen, len(icnsData))
	}
	if string(icnsData[8:12]) != "ic09" {
		t.Fatalf("expected 512x512 PNG block type 'ic09', got %q", string(icnsData[8:12]))
	}
	elemLen := binary.BigEndian.Uint32(icnsData[12:16])
	if int(elemLen) != 8+len(pngData) {
		t.Fatalf("expected ic09 element length %d, got %d", 8+len(pngData), elemLen)
	}
	if !bytes.Equal(icnsData[16:], pngData) {
		t.Fatal("icns payload does not match embedded PNG bytes")
	}

	rsrc := BuildMacOSResourceFork(icnsData)
	dataOffset := binary.BigEndian.Uint32(rsrc[0:4])
	mapOffset := binary.BigEndian.Uint32(rsrc[4:8])
	dataLen := binary.BigEndian.Uint32(rsrc[8:12])
	mapLen := binary.BigEndian.Uint32(rsrc[12:16])

	if dataOffset != 256 {
		t.Fatalf("expected ResourceFork dataOffset 256, got %d", dataOffset)
	}
	if int(dataLen) != 4+len(icnsData) {
		t.Fatalf("expected ResourceFork dataLen %d, got %d", 4+len(icnsData), dataLen)
	}
	if int(mapOffset)+int(mapLen) != len(rsrc) {
		t.Fatalf("expected mapOffset+mapLen=%d, got %d", len(rsrc), int(mapOffset)+int(mapLen))
	}
	resPayloadLen := binary.BigEndian.Uint32(rsrc[dataOffset : dataOffset+4])
	if int(resPayloadLen) != len(icnsData) {
		t.Fatalf("expected resource payload length %d, got %d", len(icnsData), resPayloadLen)
	}
	rm := rsrc[mapOffset:]
	if string(rm[30:34]) != "icns" {
		t.Fatalf("expected resource type 'icns' in resource map, got %q", string(rm[30:34]))
	}
	resID := int16(binary.BigEndian.Uint16(rm[38:40]))
	if resID != -16455 {
		t.Fatalf("expected kCustomIconResource ID -16455, got %d", resID)
	}

	finderInfo := BuildMacOSFinderInfoWithCustomIcon()
	flags := binary.BigEndian.Uint16(finderInfo[8:10])
	if flags&0x0400 == 0 {
		t.Fatalf("expected kHasCustomIcon (0x0400) bit set in FinderInfo flags, got 0x%04x", flags)
	}
}

func TestVerifyLinuxDesktopIconIntegration(t *testing.T) {
	exePath, err := os.Executable()
	if err != nil {
		t.Skipf("cannot resolve test executable: %v", err)
	}
	tmpDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmpDir, "share"))

	report, err := VerifyBinaryIcon(exePath)
	if err != nil {
		t.Fatalf("VerifyBinaryIcon failed on test binary: %v", err)
	}
	if !report.Supported {
		t.Fatalf("expected icon support on %s", report.OS)
	}
	if len(report.Details) == 0 {
		t.Fatal("expected non-empty verification details")
	}
}
