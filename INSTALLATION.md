# WhatsRook Installation & Setup Guide

This guide provides step-by-step instructions for installing and running WhatsRook on **Linux**, **macOS**, **Android (Termux)**, and **Windows**.

---

## Quick Install Summary

| Platform | Architecture | Quick One-Liner Installer |
| :--- | :--- | :--- |
| **Linux** | `x86_64` / `ARM64` / `ARMv7` | `curl -fsSL https://raw.githubusercontent.com/ThruqeLabs/whatsrook/master/scripts/install.sh \| bash` |
| **macOS** | Apple Silicon (`M1/M2/M3/M4`) / Intel | `curl -fsSL https://raw.githubusercontent.com/ThruqeLabs/whatsrook/master/scripts/install.sh \| bash` |
| **Android (Termux)** | `aarch64` (ARM64) / `arm` | `curl -fsSL https://raw.githubusercontent.com/ThruqeLabs/whatsrook/master/scripts/install.sh \| bash` |
| **Windows** | `x64` (AMD64) | `irm https://raw.githubusercontent.com/ThruqeLabs/whatsrook/master/scripts/install.ps1 \| iex` |

---

## 1. Linux Installation

Supported distributions: Ubuntu, Debian, Fedora, Arch Linux, Alpine, CentOS, Rocky Linux, openSUSE.

### Option A: Automated Installation (Recommended)
Run the automated installation script:
```bash
curl -fsSL https://raw.githubusercontent.com/ThruqeLabs/whatsrook/master/scripts/install.sh | bash
```

### Option B: Manual Installation via `curl`
1. Download the archive matching your CPU architecture:
   ```bash
   # For x86_64 (Intel/AMD)
   curl -LO https://github.com/ThruqeLabs/whatsrook/releases/latest/download/whatsrook-linux-amd64.tar.gz

   # For ARM64 (Raspberry Pi 4/5, AWS Graviton, Ampere)
   curl -LO https://github.com/ThruqeLabs/whatsrook/releases/latest/download/whatsrook-linux-arm64.tar.gz
   ```

2. Extract and install the binary:
   ```bash
   tar -xzf whatsrook-linux-*.tar.gz
   sudo mv whatsrook /usr/local/bin/
   sudo chmod +x /usr/local/bin/whatsrook
   ```

3. Verify installation:
   ```bash
   whatsrook --help
   ```

### Configuring Environment Variables on Linux
You can configure WhatsRook either using environment variables or a local `.env` file.

**Using Shell Profile (`~/.bashrc` or `~/.zshrc`):**
```bash
export SESSION="+2348062795602"
export AUTH="qr"                    # "qr" or "pair"
export CLIENT="default"             # "default", "android", or "ios"
export DATABASE_URL="default"       # "default" (SQLite) or PostgreSQL URI
export VERBOSE="false"
```
Reload your environment:
```bash
source ~/.bashrc
```

**Using `.env` File:**
Create a `.env` file in your working directory:
```env
SESSION=+2348062795602
AUTH=qr
CLIENT=default
DATABASE_URL=default
VERBOSE=false
```

### Running Interactive Standby TUI on Linux
To manage sessions, edit parameters, or create new pairings with the modern Bubble Tea interface:
```bash
whatsrook -i
# or
whatsrook standby
```

---

## 2. macOS Installation

Supported versions: macOS Monterey (12+), macOS Ventura (13+), macOS Sonoma (14+), macOS Sequoia (15+).

### Option A: Automated Installation (Recommended)
Open Terminal and run:
```bash
curl -fsSL https://raw.githubusercontent.com/ThruqeLabs/whatsrook/master/scripts/install.sh | bash
```

### Option B: Manual Installation via `curl`
1. Download the archive for your Mac architecture:
   ```bash
   # For Apple Silicon (M1, M2, M3, M4)
   curl -LO https://github.com/ThruqeLabs/whatsrook/releases/latest/download/whatsrook-darwin-arm64.tar.gz

   # For Intel Macs
   curl -LO https://github.com/ThruqeLabs/whatsrook/releases/latest/download/whatsrook-darwin-amd64.tar.gz
   ```

2. Extract and move to `/usr/local/bin`:
   ```bash
   tar -xzf whatsrook-darwin-*.tar.gz
   sudo mv whatsrook /usr/local/bin/
   sudo chmod +x /usr/local/bin/whatsrook
   ```

3. Remove the macOS Gatekeeper quarantine attribute:
   ```bash
   sudo xattr -d com.apple.quarantine /usr/local/bin/whatsrook 2>/dev/null || true
   ```

### Configuring Environment Variables on macOS
Add configuration to `~/.zshrc`:
```bash
export SESSION="+2348062795602"
export AUTH="qr"
export CLIENT="default"
export VERBOSE="false"
```
Apply changes:
```bash
source ~/.zshrc
```

### Running Interactive Standby TUI on macOS
```bash
whatsrook -i
```

---

## 3. Android Installation (Termux)

Run WhatsRook natively on Android smartphones and tablets using [Termux](https://termux.dev/).

### Step 1: Install Termux Prerequisites
Open Termux and update the package repository:
```bash
pkg update -y && pkg upgrade -y
pkg install -y curl tar git
```

### Step 2: Automated Installation
Run the installer inside Termux:
```bash
curl -fsSL https://raw.githubusercontent.com/ThruqeLabs/whatsrook/master/scripts/install.sh | bash
```

### Step 3: Manual Installation (Alternative)
```bash
# Download Android ARM64 binary
curl -LO https://github.com/ThruqeLabs/whatsrook/releases/latest/download/whatsrook-android-arm64.tar.gz

# Extract and install into Termux binary prefix
tar -xzf whatsrook-android-arm64.tar.gz
mv whatsrook $PREFIX/bin/
chmod +x $PREFIX/bin/whatsrook
```

### Configuring Environment Variables on Termux
Add variables to `~/.bashrc`:
```bash
echo 'export SESSION="+2348062795602"' >> ~/.bashrc
echo 'export AUTH="qr"' >> ~/.bashrc
echo 'export CLIENT="android"' >> ~/.bashrc
source ~/.bashrc
```

### Running the Interactive TUI on Termux
```bash
# Launch the interactive session manager
whatsrook -i
```

### Keeping WhatsRook Running in Background on Android
To prevent Android from putting Termux to sleep:
```bash
# Acquire wake lock
termux-wake-lock

# Run WhatsRook in background
nohup whatsrook > whatsrook.log 2>&1 &
```

---

## 4. Windows Installation

Supported versions: Windows 10, Windows 11, Windows Server 2019/2022.

### Option A: Automated Installation via PowerShell (Recommended)
Open **PowerShell** (as regular user or Administrator) and run:
```powershell
irm https://raw.githubusercontent.com/ThruqeLabs/whatsrook/master/scripts/install.ps1 | iex
```
*This downloads the latest binary, extracts it to `%LOCALAPPDATA%\Programs\whatsrook`, and adds it to your user `PATH`.*

### Option B: Manual Installation via `curl.exe`
1. Open PowerShell and create the installation directory:
   ```powershell
   New-Item -ItemType Directory -Force -Path "$env:LOCALAPPDATA\Programs\whatsrook"
   ```

2. Download the release archive using `curl.exe`:
   ```powershell
   curl.exe -LO "https://github.com/ThruqeLabs/whatsrook/releases/latest/download/whatsrook-windows-amd64.tar.gz"
   ```

3. Extract using built-in `tar.exe`:
   ```powershell
   tar.exe -xzf whatsrook-windows-amd64.tar.gz -C "$env:LOCALAPPDATA\Programs\whatsrook"
   ```

4. Add WhatsRook to your persistent user `PATH`:
   ```powershell
   $currentPath = [Environment]::GetEnvironmentVariable("Path", [EnvironmentVariableTarget]::User)
   [Environment]::SetEnvironmentVariable("Path", "$currentPath;$env:LOCALAPPDATA\Programs\whatsrook", [EnvironmentVariableTarget]::User)
   $env:Path = "$env:Path;$env:LOCALAPPDATA\Programs\whatsrook"
   ```

5. Verify:
   ```powershell
   whatsrook --help
   ```

### Configuring Environment Variables on Windows

**In Current PowerShell Session:**
```powershell
$env:SESSION = "+2348062795602"
$env:AUTH = "qr"
$env:CLIENT = "default"
$env:VERBOSE = "false"
```

**Persistent User Environment Variables:**
```powershell
[Environment]::SetEnvironmentVariable("SESSION", "+2348062795602", [EnvironmentVariableTarget]::User)
[Environment]::SetEnvironmentVariable("CLIENT", "default", [EnvironmentVariableTarget]::User)
[Environment]::SetEnvironmentVariable("AUTH", "qr", [EnvironmentVariableTarget]::User)
```

**Using `.env` File:**
Create a `.env` file in your execution directory:
```env
SESSION=+2348062795602
AUTH=qr
CLIENT=default
VERBOSE=false
```

### Running Interactive Standby TUI on Windows
Launch **Windows Terminal** (or PowerShell / Command Prompt) and run:
```powershell
whatsrook -i
```

---

## 5. Environment Variables Reference

| Variable | Type | Allowed Values | Default | Description |
| :--- | :--- | :--- | :--- | :--- |
| `SESSION` | `string` | Phone number with country code (e.g. `+2348062795602`) | *empty* | Identifier for session store. |
| `AUTH` | `string` | `qr`, `pair` | `qr` | Authentication mechanism when establishing a new link. |
| `CLIENT` | `string` | `default`, `android`, `ios` | `default` | Companion device platform signature emulated during pairing. |
| `DATABASE_URL` | `string` | `default`, PostgreSQL URI (`postgres://user:pass@host:5432/db`) | `default` | Database storage driver backend (`default` uses SQLite). |
| `VERBOSE` | `boolean` | `true`, `false` | `false` | Enable verbose protocol and socket debug logging. |
| `LOGOUT` | `boolean` | `true`, `false` | `false` | Flushes session credentials from database and terminates. |
| `HTTP_PORT` | `integer` | Valid TCP port (e.g. `8080`) | `0` (auto) | Custom port for WebSocket & REST API server. |

---

## 6. Updating WhatsRook

WhatsRook includes a built-in automated updater:

```bash
# Check for available updates
whatsrook update check

# Update to latest stable release
whatsrook update stable

# Update to latest alpha / nightly build
whatsrook update beta
```
