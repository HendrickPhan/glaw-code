#!/usr/bin/env bash
#
# install.sh — Install glaw-code CLI
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/HendrickPhan/glaw-code/main/install.sh | bash
#   or
#   git clone git@github.com:HendrickPhan/glaw-code.git && cd glaw-code && bash install.sh
#
set -euo pipefail

REPO_URL="git@github.com:HendrickPhan/glaw-code.git"
HTTPS_URL="https://github.com/HendrickPhan/glaw-code.git"
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

info()  { echo -e "${CYAN}[INFO]${RESET} $*"; }
ok()    { echo -e "${GREEN}[OK]${RESET} $*"; }
warn()  { echo -e "${YELLOW}[WARN]${RESET} $*"; }
err()   { echo -e "${RED}[ERROR]${RESET} $*" >&2; }

# -------------------------------------------------------
# Detect OS and Architecture
# -------------------------------------------------------
detect_platform() {
    OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
    ARCH="$(uname -m)"

    case "$OS" in
        darwin)  OS="darwin" ;;
        linux)   OS="linux" ;;
        mingw*|msys*|cygwin*|windows_nt) OS="windows" ;;
        *)
            err "Unsupported operating system: $OS"
            exit 1
            ;;
    esac

    case "$ARCH" in
        x86_64|amd64)   ARCH="amd64" ;;
        arm64|aarch64)  ARCH="arm64" ;;
        *)
            err "Unsupported architecture: $ARCH"
            exit 1
            ;;
    esac

    info "Detected platform: ${OS}/${ARCH}"
}

# -------------------------------------------------------
# Check prerequisites
# -------------------------------------------------------
check_prerequisites() {
    local missing=()

    if ! command -v go &>/dev/null; then
        missing+=("go (Go 1.22+)")
    fi

    if ! command -v git &>/dev/null; then
        missing+=("git")
    fi

    if [ ${#missing[@]} -ne 0 ]; then
        err "Missing prerequisites:"
        for m in "${missing[@]}"; do
            echo "  - $m"
        done
        echo ""
        echo "Install Go: https://go.dev/dl/"
        echo "Install Git: https://git-scm.com/"
        exit 1
    fi

    # Check Go version (need 1.22+)
    GO_VERSION=$(go version | grep -oP 'go\K[0-9]+\.[0-9]+' || echo "0.0")
    GO_MAJOR=$(echo "$GO_VERSION" | cut -d. -f1)
    GO_MINOR=$(echo "$GO_VERSION" | cut -d. -f2)

    if [ "$GO_MAJOR" -lt 1 ] || { [ "$GO_MAJOR" -eq 1 ] && [ "$GO_MINOR" -lt 22 ]; }; then
        err "Go 1.22+ is required (found Go ${GO_VERSION})"
        exit 1
    fi

    # Check if Node.js is available (optional, for web UI build)
    if command -v node &>/dev/null; then
        HAS_NODE=true
    else
        HAS_NODE=false
        warn "Node.js not found. Web UI will not be built (REPL mode still works)."
    fi
}

# -------------------------------------------------------
# Clone or update the repository
# -------------------------------------------------------
clone_repo() {
    BUILD_DIR="${TMPDIR_BASE}/glaw-code-build-$$"

    # If running from within the repo, use current directory
    if [ -f "./cmd/glaw/main.go" ] && [ -f "./go.mod" ]; then
        BUILD_DIR="$(pwd)"
        info "Running from repo directory: ${BUILD_DIR}"
        return
    fi

    info "Cloning repository..."
    rm -rf "$BUILD_DIR"

    # Try SSH first, fall back to HTTPS
    if git clone --depth 1 "$REPO_URL" "$BUILD_DIR" 2>/dev/null; then
        :
    elif git clone --depth 1 "$HTTPS_URL" "$BUILD_DIR" 2>/dev/null; then
        :
    else
        err "Failed to clone repository. Check your network connection and git access."
        exit 1
    fi

    ok "Repository cloned to ${BUILD_DIR}"
}

# -------------------------------------------------------
# Build the binary
# -------------------------------------------------------
build_glaw() {
    cd "$BUILD_DIR"

    info "Downloading Go dependencies..."
    go mod download

    # Build web UI if Node.js is available
    if [ "$HAS_NODE" = true ] && [ -d "web" ] && [ -f "web/package.json" ]; then
        info "Building web UI..."
        cd web
        npm install --silent 2>/dev/null || npm install
        npm run build
        cd ..
        rm -rf internal/web/static
        cp -r web/out internal/web/static
        ok "Web UI built"
    else
        warn "Skipping web UI build (Node.js not available or web/ not found)"
    fi

    info "Building ${BINARY_NAME}..."
    local ldflags="-s -w"
    CGO_ENABLED=0 go build -ldflags "$ldflags" -o "$BINARY_NAME" ./cmd/glaw/

    ok "Build complete: ./${BINARY_NAME}"
}

# -------------------------------------------------------
# Install the binary
# -------------------------------------------------------
install_glaw() {
    local src="${BUILD_DIR}/${BINARY_NAME}"
    local dest="${INSTALL_DIR}/${BINARY_NAME}"

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
# Cleanup
# -------------------------------------------------------
cleanup() {
    if [ -n "${BUILD_DIR:-}" ] && [ "${BUILD_DIR}" != "$(pwd)" ] && [ -d "${BUILD_DIR}" ]; then
        rm -rf "${BUILD_DIR}"
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
    check_prerequisites
    clone_repo
    build_glaw
    install_glaw

    echo ""
    ok "Done!"
}

main "$@"
