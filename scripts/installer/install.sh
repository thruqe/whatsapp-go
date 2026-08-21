#!/usr/bin/env bash
set -euo pipefail

REPO="Thruqe/whatsrook"
BIN_NAME="whatsrook"
INSTALL_DIR="${WHATROOK_INSTALL_DIR:-$HOME/.whatsrook/bin}"

# 1. Detect OS
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$OS" in
  linux)  TARGET_OS="linux" ;;
  darwin) TARGET_OS="darwin" ;;
  *)
    echo "❌ Unsupported operating system: $OS" >&2
    exit 1
    ;;
esac

# 2. Detect Architecture
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64)   TARGET_ARCH="amd64" ;;
  arm64|aarch64)  TARGET_ARCH="arm64" ;;
  *)
    echo "❌ Unsupported architecture: $ARCH" >&2
    exit 1
    ;;
esac

echo "==> Installing ${BIN_NAME} for ${TARGET_OS}/${TARGET_ARCH}..."

ASSET_NAME="${BIN_NAME}-${TARGET_OS}-${TARGET_ARCH}.tar.gz"
DOWNLOAD_URL="https://github.com/${REPO}/releases/latest/download/${ASSET_NAME}"

TMP_DIR="$(mktemp -d)"
cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

# 3. Download release archive
echo "==> Downloading ${DOWNLOAD_URL}..."
if command -v curl >/dev/null 2>&1; then
  curl -fsSL "$DOWNLOAD_URL" -o "${TMP_DIR}/${ASSET_NAME}"
elif command -v wget >/dev/null 2>&1; then
  wget -qO "${TMP_DIR}/${ASSET_NAME}" "$DOWNLOAD_URL"
else
  echo "❌ Neither curl nor wget was found. Please install curl or wget." >&2
  exit 1
fi

# 4. Extract archive
echo "==> Extracting archive..."
mkdir -p "$INSTALL_DIR"
tar -xzf "${TMP_DIR}/${ASSET_NAME}" -C "$TMP_DIR"

if [ -f "${TMP_DIR}/${BIN_NAME}" ]; then
  mv "${TMP_DIR}/${BIN_NAME}" "${INSTALL_DIR}/${BIN_NAME}"
elif [ -f "${TMP_DIR}/bin/${BIN_NAME}" ]; then
  mv "${TMP_DIR}/bin/${BIN_NAME}" "${INSTALL_DIR}/${BIN_NAME}"
else
  # Find executable if archive structure is nested
  FOUND="$(find "$TMP_DIR" -type f -name "$BIN_NAME" | head -n 1)"
  if [ -n "$FOUND" ]; then
    mv "$FOUND" "${INSTALL_DIR}/${BIN_NAME}"
  else
    echo "❌ Binary ${BIN_NAME} not found in archive." >&2
    exit 1
  fi
fi

chmod +x "${INSTALL_DIR}/${BIN_NAME}"

# 5. Add to PATH in shell profile if not already present
add_to_path() {
  local profile="$1"
  local line="export PATH=\"\$PATH:${INSTALL_DIR}\""
  if [ -f "$profile" ]; then
    if ! grep -qs "${INSTALL_DIR}" "$profile"; then
      echo "" >> "$profile"
      echo "# WhatsRook CLI" >> "$profile"
      echo "$line" >> "$profile"
      echo "✓ Added ${INSTALL_DIR} to ${profile}"
    fi
  fi
}

add_to_path "$HOME/.bashrc"
add_to_path "$HOME/.zshrc"
add_to_path "$HOME/.profile"

echo ""
echo "🎉 WhatsRook installed successfully to ${INSTALL_DIR}/${BIN_NAME}"
echo "Restart your terminal or run:"
echo "  export PATH=\"\$PATH:${INSTALL_DIR}\""
echo ""
echo "Run 'whatsrook -h' to get started."
