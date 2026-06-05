#!/usr/bin/env bash
# Sign a binary with a Developer ID Application certificate.
# No-ops gracefully when CODESIGN_IDENTITY is unset and no cert is found locally.
set -euo pipefail

BINARY="${1:?Usage: codesign.sh <path-to-binary>}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Prefer explicit identity; fall back to auto-detecting one from the keychain.
IDENTITY="${CODESIGN_IDENTITY:-}"
if [[ -z "$IDENTITY" ]]; then
    IDENTITY=$(security find-identity -v -p codesigning 2>/dev/null \
        | grep "Developer ID Application" \
        | head -1 \
        | sed 's/.*"\(.*\)"/\1/' || true)
fi

if [[ -z "$IDENTITY" ]]; then
    echo "codesign.sh: no Developer ID Application certificate found — skipping" >&2
    exit 0
fi

echo "codesign.sh: signing '$BINARY' with '$IDENTITY'" >&2
codesign \
    --deep \
    --force \
    --verify \
    --verbose \
    --sign "$IDENTITY" \
    --options runtime \
    --entitlements "$SCRIPT_DIR/entitlements.plist" \
    "$BINARY"

echo "codesign.sh: verification:" >&2
codesign --verify --verbose=2 "$BINARY"
