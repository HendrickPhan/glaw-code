#!/bin/bash
set -e

# ─────────────────────────────────────────────────────────
# build.sh — Build glaw binaries for all supported platforms
#
# Outputs binaries into the prebuild/ directory using the
# naming convention:  glaw-{os}-{arch}[.exe]
#
# Usage:
#   bash build.sh           # build all platforms
#   bash build.sh darwin    # build only for macOS (darwin)
#   bash build.sh current   # build only for the current OS/arch
# ─────────────────────────────────────────────────────────

BINARY_NAME="glaw"
PREBUILD_DIR="$(cd "$(dirname "$0")" && pwd)/prebuild"
MAIN_PACKAGE="./cmd/glaw"
LDFLAGS="-s -w"

# Colors
GREEN='\033[0;32m'
CYAN='\033[0;36m'
BOLD='\033[1m'
RESET='\033[0m'

info()  { echo -e "${CYAN}[BUILD]${RESET} $*"; }
ok()    { echo -e "${GREEN}[DONE]${RESET} $*"; }

# -------------------------------------------------------
# Detect the current platform
# -------------------------------------------------------
current_os() {
    local os="$(uname -s | tr '[:upper:]' '[:lower:]')"
    case "$os" in
        darwin)  echo "darwin" ;;
        linux)   echo "linux" ;;
        mingw*|msys*|cygwin*|windows_nt) echo "windows" ;;
        *) echo "$os" ;;
    esac
}

current_arch() {
    local arch="$(uname -m)"
    case "$arch" in
        x86_64|amd64)  echo "amd64" ;;
        arm64|aarch64) echo "arm64" ;;
        *) echo "$arch" ;;
    esac
}

# -------------------------------------------------------
# Build the web UI (optional, requires Node.js)
# -------------------------------------------------------
build_web_ui() {
    if [ ! -d "web" ] || [ ! -f "web/package.json" ]; then
        echo -e "${CYAN}[SKIP]${RESET} No web/ directory found, skipping web UI build."
        return
    fi

    if ! command -v node &>/dev/null; then
        echo -e "${CYAN}[SKIP]${RESET} Node.js not found, skipping web UI build."
        return
    fi

    info "Building Next.js web UI..."
    cd web
    npm install --silent 2>/dev/null || npm install
    npm run build
    cd ..
    rm -rf internal/web/static
    cp -r web/out internal/web/static
    ok "Web UI built and copied to internal/web/static"
}

# -------------------------------------------------------
# Build a single platform binary
# -------------------------------------------------------
build_binary() {
    local goos="$1"
    local goarch="$2"
    local output_name="${BINARY_NAME}-${goos}-${goarch}"

    if [ "$goos" = "windows" ]; then
        output_name="${output_name}.exe"
    fi

    local output_path="${PREBUILD_DIR}/${output_name}"

    info "Building ${output_name}..."
    CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
        go build -ldflags "$LDFLAGS" -o "$output_path" "$MAIN_PACKAGE"

    local size
    size=$(du -h "$output_path" | cut -f1)
    ok "${output_name} (${size})"
}

# -------------------------------------------------------
# Main
# -------------------------------------------------------
main() {
    echo -e "${BOLD}glaw-code builder${RESET}"
    echo ""

    # Ensure prebuild directory exists
    mkdir -p "$PREBUILD_DIR"

    # Build web UI first (embedded into the binary)
    build_web_ui

    # Download Go dependencies
    info "Downloading Go dependencies..."
    go mod download

    echo ""

    # Determine which platforms to build
    TARGET="${1:-all}"

    if [ "$TARGET" = "current" ]; then
        local os arch
        os="$(current_os)"
        arch="$(current_arch)"
        build_binary "$os" "$arch"
    elif [ "$TARGET" = "darwin" ]; then
        build_binary "darwin" "amd64"
        build_binary "darwin" "arm64"
    elif [ "$TARGET" = "linux" ]; then
        build_binary "linux" "amd64"
        build_binary "linux" "arm64"
    elif [ "$TARGET" = "windows" ]; then
        build_binary "windows" "amd64"
        build_binary "windows" "arm64"
    else
        # Build all supported platforms
        build_binary "darwin" "amd64"
        build_binary "darwin" "arm64"
        build_binary "linux" "amd64"
        build_binary "linux" "arm64"
        build_binary "windows" "amd64"
        build_binary "windows" "arm64"
    fi

    echo ""
    ok "All binaries written to prebuild/"
    ls -lh "${PREBUILD_DIR}/"
}

main "$@"
