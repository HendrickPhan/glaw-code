# Prebuilt Binaries

This directory contains prebuilt binaries for `glaw`, organized by platform.

## Naming Convention

Binaries are named using the pattern:

```
glaw-{os}-{arch}
```

For example:
- `glaw-darwin-amd64`
- `glaw-darwin-arm64`
- `glaw-linux-amd64`
- `glaw-linux-arm64`
- `glaw-windows-amd64.exe`
- `glaw-windows-arm64.exe`

## How to Build

Run the build script from the project root to compile all platform binaries into this directory:

```bash
bash build.sh
```

## GitHub Releases

These binaries are attached to GitHub Releases. The `install.sh` script downloads
the correct binary for the user's platform directly from the latest release — no
Go or Node.js toolchain required on the user's machine.
