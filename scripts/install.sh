#!/usr/bin/env bash
set -e

REPO="ThruqeLabs/whatsrook"
BIN_NAME="whatsrook"

# Detect OS
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

IS_TERMUX=false
if [ -n "$PREFIX" ] && [ -d "$PREFIX/bin" ] && [[ "$PREFIX" == *"com.termux"* ]]; then
  IS_TERMUX=true
fi

case "$OS" in
  linux*)
    if [ "$IS_TERMUX" = true ]; then
      TARGET_OS="android"
    else
      TARGET_OS="linux"
    fi
    ;;
  darwin*)
    TARGET_OS="darwin"
    ;;
  *)
    echo "Error: Unsupported operating system: $OS"
    exit 1
    ;;
esac

# Detect Architecture
case "$ARCH" in
  x86_64|amd64)
    TARGET_ARCH="amd64"
    ;;
  aarch64|arm64)
    TARGET_ARCH="arm64"
    ;;
  armv7l|armv8l|arm)
    TARGET_ARCH="arm"
    ;;
  *)
    echo "Error: Unsupported CPU architecture: $ARCH"
    exit 1
    ;;
esac

ASSET_NAME="whatsrook-${TARGET_OS}-${TARGET_ARCH}.tar.gz"

echo "============================================================"
echo "Installing WhatsRook ($TARGET_OS / $TARGET_ARCH)"
echo "============================================================"

# Resolve Target Installation Directory
if [ "$IS_TERMUX" = true ]; then
  INSTALL_DIR="$PREFIX/bin"
elif [ -w "/usr/local/bin" ]; then
  INSTALL_DIR="/usr/local/bin"
elif [ "$EUID" -eq 0 ]; then
  INSTALL_DIR="/usr/local/bin"
else
  INSTALL_DIR="$HOME/.local/bin"
  mkdir -p "$INSTALL_DIR"
fi

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

DOWNLOAD_URL="https://github.com/${REPO}/releases/latest/download/${ASSET_NAME}"
ALPHA_FALLBACK_URL="https://github.com/${REPO}/releases/download/alpha/${ASSET_NAME}"

echo "Downloading ${ASSET_NAME}..."
if ! curl -fsSL -o "${TMP_DIR}/${ASSET_NAME}" "$DOWNLOAD_URL"; then
  echo "Latest release download failed. Attempting alpha release..."
  if ! curl -fsSL -o "${TMP_DIR}/${ASSET_NAME}" "$ALPHA_FALLBACK_URL"; then
    echo "Error: Failed to download release asset from GitHub."
    exit 1
  fi
fi

echo "Extracting binary..."
tar -xzf "${TMP_DIR}/${ASSET_NAME}" -C "${TMP_DIR}"

if [ ! -f "${TMP_DIR}/${BIN_NAME}" ]; then
  echo "Error: Extracted archive did not contain '${BIN_NAME}' binary."
  exit 1
fi

chmod +x "${TMP_DIR}/${BIN_NAME}"

echo "Installing to ${INSTALL_DIR}/${BIN_NAME}..."
if [ -w "$INSTALL_DIR" ]; then
  mv "${TMP_DIR}/${BIN_NAME}" "${INSTALL_DIR}/${BIN_NAME}"
else
  echo "Elevated permissions required to write to ${INSTALL_DIR}."
  sudo mv "${TMP_DIR}/${BIN_NAME}" "${INSTALL_DIR}/${BIN_NAME}"
fi

# Gatekeeper quarantine removal for macOS
if [ "$TARGET_OS" = "darwin" ]; then
  xattr -d com.apple.quarantine "${INSTALL_DIR}/${BIN_NAME}" 2>/dev/null || true
fi

# PATH Check
case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *)
    echo "Notice: ${INSTALL_DIR} is not in your PATH."
    echo "Add it to your shell configuration:"
    echo "  export PATH=\"\$PATH:${INSTALL_DIR}\""
    ;;
esac

echo "============================================================"
echo "WhatsRook installed successfully!"
echo "Version: $(${INSTALL_DIR}/${BIN_NAME} --version 2>/dev/null || echo 'latest')"
echo ""
echo "Quick Start:"
echo "  Run interactive TUI setup:  whatsrook -i"
echo "  Start configured session:   whatsrook"
echo "  Show CLI options:           whatsrook --help"
echo "============================================================"
