#!/usr/bin/env bash
#
# install.sh — Install glaw-code CLI from prebuilt binary
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/HendrickPhan/glaw-code/main/install.sh | bash
#   or
#   git clone git@github.com:HendrickPhan/glaw-code.git && cd glaw-code && bash install.sh
#   bash install.sh --local     # Force using local prebuild/ directory
#
# This script installs a prebuilt binary — either from the local prebuild/
# directory (when run from a cloned repo) or by downloading from GitHub
# Releases (when piped via curl).
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

  RELEASE_VERSION=""
  RELEASE_VERSION=$(curl -fsSL "$api_url" 2>/dev/null \
    | grep '"tag_name"' \
    | head -1 \
    | cut -d'"' -f4 || true)

  if [ -z "${RELEASE_VERSION:-}" ]; then
    warn "No GitHub release found. Will try local prebuild/ directory."
  else
    info "Latest release: ${RELEASE_VERSION}"
  fi
}

# -------------------------------------------------------
# Check if local prebuild/ directory is available
# -------------------------------------------------------
has_local_prebuild() {
  local script_dir
  script_dir="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
  local filename
  filename="$(binary_filename)"
  local prebuild_path="${script_dir}/prebuild/${filename}"

  [ -f "$prebuild_path" ]
}

# -------------------------------------------------------
# Download or copy the prebuilt binary
# -------------------------------------------------------
download_binary() {
  local filename
  filename="$(binary_filename)"
  DOWNLOAD_DIR="${TMPDIR_BASE}/glaw-code-install-$$"
  mkdir -p "$DOWNLOAD_DIR"

  # ── Strategy: local prebuild first, then GitHub Release ──
  #
  # When install.sh is run from a cloned repo (bash install.sh),
  # we prefer the local prebuild/ directory because:
  #   1. The developer just built it — it's the freshest binary.
  #   2. Avoids downloading from GitHub, which may have an older release.
  #   3. Works offline.
  #
  # When piped via curl, there's no local prebuild/, so we download
  # from GitHub Releases.

  local script_dir
  script_dir="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
  local prebuild_path="${script_dir}/prebuild/${filename}"

  if [ "${FORCE_LOCAL:-}" = "1" ] || [ -f "$prebuild_path" ]; then
    # ── Use local prebuild/ directory ────────────────────
    if [ -f "$prebuild_path" ]; then
      info "Installing from local prebuild/${filename}..."
      cp "$prebuild_path" "${DOWNLOAD_DIR}/${BINARY_NAME}"
      ok "Copied from ${prebuild_path}"
    else
      err "No local prebuilt binary found at prebuild/${filename}"
      err "Run 'bash build.sh' first to create it."
      exit 1
    fi
  elif [ -n "${RELEASE_VERSION:-}" ]; then
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
    # ── No binary available anywhere ──────────────────────
    err "No prebuilt binary found for your platform."
    err "Expected one of:"
    err "  - Local file:     prebuild/${filename}"
    err "  - GitHub Release: https://github.com/${GITHUB_REPO}/releases"
    err ""
    err "To create a prebuilt binary, run:  bash build.sh"
    exit 1
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

  # Remove old binary first (needed on macOS when overwriting with sudo)
  local dest="${INSTALL_DIR}/${BINARY_NAME}"

  if [ -w "${INSTALL_DIR}" ] && [ ! -f "$dest" -o -w "$dest" ]; then
    # Direct copy (user has write permission)
    cp "$src" "$dest"
    chmod +x "$dest"
    ok "Installed to ${dest}"
  elif [ "$(id -u)" -ne 0 ]; then
    # Need sudo
    info "Requesting sudo to install to ${dest}..."
    sudo rm -f "$dest"
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

  # Parse flags
  case "${1:-}" in
    --local|-l)
      FORCE_LOCAL=1
      info "Forcing local prebuild/ directory"
      ;;
  esac

  detect_platform
  get_latest_version
  download_binary
  install_glaw

  echo ""
  ok "Done!"
}

main "$@"
