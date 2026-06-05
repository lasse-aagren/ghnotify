#!/usr/bin/env bash
# Assemble a macOS .app bundle from a pre-built binary.
# Usage: make-app.sh <binary-path> [app-dir]
#   binary-path  path to the compiled binary
#   app-dir      destination .app directory (default: dist/ghnotify.app)
set -euo pipefail

BINARY="${1:?Usage: make-app.sh <binary-path> [app-dir]}"
BINARY_NAME="$(basename "$BINARY")"
APP_DIR="${2:-dist/ghnotify.app}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

mkdir -p "$APP_DIR/Contents/MacOS"
cp "$BINARY" "$APP_DIR/Contents/MacOS/$BINARY_NAME"
cp "$SCRIPT_DIR/../macos/Info.plist" "$APP_DIR/Contents/Info.plist"

echo "make-app.sh: assembled $APP_DIR" >&2
