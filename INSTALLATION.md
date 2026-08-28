# Installation

Copy and paste the one-liner command for your platform into your terminal. It will automatically download, install, and launch the interactive WhatsRook interface.

### Linux

**x86_64 (Intel / AMD):**
```bash
curl -fsSL https://github.com/ThruqeLabs/whatsrook/releases/latest/download/whatsrook-linux-amd64.tar.gz | sudo tar -xz -C /usr/local/bin && sudo chmod +x /usr/local/bin/whatsrook && whatsrook -i
```

**ARM64 (Raspberry Pi / AWS Graviton / Ampere):**
```bash
curl -fsSL https://github.com/ThruqeLabs/whatsrook/releases/latest/download/whatsrook-linux-arm64.tar.gz | sudo tar -xz -C /usr/local/bin && sudo chmod +x /usr/local/bin/whatsrook && whatsrook -i
```

### macOS

**Apple Silicon (M1 / M2 / M3 / M4):**
```bash
curl -fsSL https://github.com/ThruqeLabs/whatsrook/releases/latest/download/whatsrook-darwin-arm64.tar.gz | sudo tar -xz -C /usr/local/bin && sudo chmod +x /usr/local/bin/whatsrook && sudo xattr -d com.apple.quarantine /usr/local/bin/whatsrook 2>/dev/null; whatsrook -i
```

**Intel:**
```bash
curl -fsSL https://github.com/ThruqeLabs/whatsrook/releases/latest/download/whatsrook-darwin-amd64.tar.gz | sudo tar -xz -C /usr/local/bin && sudo chmod +x /usr/local/bin/whatsrook && sudo xattr -d com.apple.quarantine /usr/local/bin/whatsrook 2>/dev/null; whatsrook -i
```

### Android (Termux)

```bash
pkg install -y curl tar && curl -fsSL https://github.com/ThruqeLabs/whatsrook/releases/latest/download/whatsrook-android-arm64.tar.gz -o wr.tar.gz && tar -xzf wr.tar.gz -C $PREFIX/bin && rm -f wr.tar.gz && chmod +x $PREFIX/bin/whatsrook && whatsrook -i
```

### Windows

Open **PowerShell** and paste:

```powershell
New-Item -ItemType Directory -Force -Path "$env:LOCALAPPDATA\whatsrook" | Out-Null; curl.exe -fsSL "https://github.com/ThruqeLabs/whatsrook/releases/latest/download/whatsrook-windows-amd64.tar.gz" -o "$env:TEMP\wr.tar.gz"; tar.exe -xzf "$env:TEMP\wr.tar.gz" -C "$env:LOCALAPPDATA\whatsrook"; Remove-Item "$env:TEMP\wr.tar.gz"; & "$env:LOCALAPPDATA\whatsrook\whatsrook.exe" -i
```
