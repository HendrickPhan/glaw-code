#!/usr/bin/env bash
#
# install.sh — Install glaw-code CLI from prebuilt binary
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/HendrickPhan/glaw-code/main/install.sh | bash
#   or
#   git clone git@github.com:HendrickPhan/glaw-code.git && cd glaw-code && bash install.sh
#
# This script downloads a prebuilt binary from GitHub Releases — no Go or
# Node.js toolchain is required on the user's machine.
#
set -euo pipefail

GITHUB_REPO="HendrickPhan/glaw-code"
BINARY_NAME="glaw"
INSTALL_DIR="/usr/local/bin"
TMPDIR_BASE="${TMPDIR:-/tmp}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
RESET='\033[0m'

info() { echo -e "${CYAN}[INFO]${RESET} $*"; }
ok() { echo -e "${GREEN}[OK]${RESET} $*"; }
warn() { echo -e "${YELLOW}[WARN]${RESET} $*"; }
err() { echo -e "${RED}[ERROR]${RESET} $*" >&2; }

# -------------------------------------------------------
# Detect OS and Architecture
# -------------------------------------------------------
detect_platform() {
  OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
  ARCH="$(uname -m)"

  case "$OS" in
  darwin) OS="darwin" ;;
  linux) OS="linux" ;;
  mingw* | msys* | cygwin* | windows_nt) OS="windows" ;;
  *)
    err "Unsupported operating system: $OS"
    exit 1
    ;;
  esac

  case "$ARCH" in
  x86_64 | amd64) ARCH="amd64" ;;
  arm64 | aarch64) ARCH="arm64" ;;
  *)
    err "Unsupported architecture: $ARCH"
    exit 1
    ;;
  esac

  info "Detected platform: ${OS}/${ARCH}"
}

# -------------------------------------------------------
# Determine the binary file name for the current platform
# -------------------------------------------------------
binary_filename() {
  local name="${BINARY_NAME}-${OS}-${ARCH}"
  if [ "$OS" = "windows" ]; then
    name="${name}.exe"
  fi
  echo "$name"
}

# -------------------------------------------------------
# Get the latest release tag from GitHub
# -------------------------------------------------------
get_latest_version() {
  local api_url="https://api.github.com/repos/${GITHUB_REPO}/releases/latest"

  # Try to fetch the latest release tag.
  # Use a subshell to avoid pipefail killing the script on 404.
  RELEASE_VERSION=""
  local response
  response=$(curl -fsSL "$api_url" 2>/dev/null || true)
  if [ -n "$response" ]; then
    RELEASE_VERSION=$(echo "$response" | grep '"tag_name"' | head -1 | sed -E 's/.*"tag_name"\s*:\s*"([^"]+)".*/\1/' || true)
  fi

  if [ -z "${RELEASE_VERSION:-}" ]; then
    # No release found — will fall back to local prebuild/ directory
    warn "No GitHub release found. Will try local prebuild/ directory."
  else
    info "Latest release: ${RELEASE_VERSION}"
  fi
}

# -------------------------------------------------------
# Download the prebuilt binary
# -------------------------------------------------------
download_binary() {
  local filename
  filename="$(binary_filename)"
  DOWNLOAD_DIR="${TMPDIR_BASE}/glaw-code-install-$$"
  mkdir -p "$DOWNLOAD_DIR"

  if [ -n "$RELEASE_VERSION" ]; then
    # ── Download from GitHub Release ──────────────────────
    local download_url="https://github.com/${GITHUB_REPO}/releases/download/${RELEASE_VERSION}/${filename}"
    info "Downloading ${filename} from ${download_url}..."

    if curl -fsSL --progress-bar -o "${DOWNLOAD_DIR}/${BINARY_NAME}" "$download_url"; then
      ok "Downloaded ${filename}"
    else
      err "Failed to download ${filename} from GitHub Releases."
      err "Make sure a release with this binary exists at:"
      err "  https://github.com/${GITHUB_REPO}/releases/tag/${RELEASE_VERSION}"
      exit 1
    fi
  else
    # ── Fallback: copy from local prebuild/ directory ─────
    # This is useful when running install.sh from a cloned repo
    # and no GitHub Release has been published yet.
    local script_dir
    script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    local prebuild_path="${script_dir}/prebuild/${filename}"

    if [ -f "$prebuild_path" ]; then
      info "Installing from local prebuild/${filename}..."
      cp "$prebuild_path" "${DOWNLOAD_DIR}/${BINARY_NAME}"
      ok "Copied from ${prebuild_path}"
    else
      err "No prebuilt binary found for your platform."
      err "Expected one of:"
      err "  - GitHub Release: https://github.com/${GITHUB_REPO}/releases"
      err "  - Local file:     prebuild/${filename}"
      err ""
      err "To create a prebuilt binary, run:  bash build.sh"
      exit 1
    fi
  fi

  chmod +x "${DOWNLOAD_DIR}/${BINARY_NAME}"
}

# -------------------------------------------------------
# Install the binary to a location on PATH
# -------------------------------------------------------
install_glaw() {
  local src="${DOWNLOAD_DIR}/${BINARY_NAME}"

  if [ ! -f "$src" ]; then
    err "Binary not found at ${src}"
    exit 1
  fi

  # On Windows (Git Bash / MSYS2), copy to a location on PATH
  if [ "$OS" = "windows" ]; then
    local win_dest="${HOME}/bin/${BINARY_NAME}.exe"
    mkdir -p "${HOME}/bin"
    cp "$src" "$win_dest"
    ok "Installed to ${win_dest}"
    warn "Make sure ${HOME}/bin is in your PATH"
    return
  fi

  # Try direct copy first
  local dest="${INSTALL_DIR}/${BINARY_NAME}"
  if cp "$src" "$dest" 2>/dev/null; then
    chmod +x "$dest"
    ok "Installed to ${dest}"
  elif [ "$(id -u)" -ne 0 ]; then
    # Need sudo
    info "Requesting sudo to install to ${dest}..."
    sudo cp "$src" "$dest"
    sudo chmod +x "$dest"
    ok "Installed to ${dest}"
  else
    # Fallback: install to user's local bin
    local user_bin="${HOME}/.local/bin"
    mkdir -p "$user_bin"
    cp "$src" "${user_bin}/${BINARY_NAME}"
    chmod +x "${user_bin}/${BINARY_NAME}"
    ok "Installed to ${user_bin}/${BINARY_NAME}"
    warn "Add ${user_bin} to your PATH if not already there:"
    echo '  export PATH="${HOME}/.local/bin:${PATH}"'
  fi

  # Verify installation
  if command -v "$BINARY_NAME" &>/dev/null; then
    local version
    version="$("$BINARY_NAME" --version 2>/dev/null || echo 'unknown')"
    ok "${BINARY_NAME} installed successfully! Version: ${version}"
    echo ""
    echo -e "${BOLD}Quick start:${RESET}"
    echo "  ${BINARY_NAME}              # Start interactive REPL"
    echo "  ${BINARY_NAME} serve        # Start web UI on :8080"
    echo "  ${BINARY_NAME} \"fix the bug\" # One-shot mode"
    echo ""
    echo -e "Set ${CYAN}ANTHROPIC_API_KEY${RESET} or ${CYAN}XAI_API_KEY${RESET} to enable AI features."
  else
    warn "Binary installed but not found in PATH. You may need to restart your shell."
  fi
}

# -------------------------------------------------------
# Cleanup temporary files
# -------------------------------------------------------
cleanup() {
  if [ -n "${DOWNLOAD_DIR:-}" ] && [ -d "${DOWNLOAD_DIR}" ]; then
    rm -rf "${DOWNLOAD_DIR}"
  fi
}
trap cleanup EXIT

# -------------------------------------------------------
# Main
# -------------------------------------------------------
main() {
  echo -e "${BOLD}glaw-code installer${RESET}"
  echo ""

  detect_platform
  get_latest_version
  download_binary
  install_glaw

  echo ""
  ok "Done!"
}

main "$@"
