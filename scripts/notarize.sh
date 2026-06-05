#!/usr/bin/env bash
# Notarize a zip archive with Apple's notarytool.
# Requires APPLE_ID, APP_PASSWORD (app-specific password), and TEAM_ID env vars.
# No-ops gracefully when any of these are absent.
set -euo pipefail

ARCHIVE="${1:?Usage: notarize.sh <path-to-zip>}"

if [[ -z "${APPLE_ID:-}" || -z "${APP_PASSWORD:-}" || -z "${TEAM_ID:-}" ]]; then
    echo "notarize.sh: APPLE_ID / APP_PASSWORD / TEAM_ID not set — skipping notarization" >&2
    exit 0
fi

echo "notarize.sh: submitting '$ARCHIVE' to Apple notarytool..." >&2
xcrun notarytool submit "$ARCHIVE" \
    --apple-id  "$APPLE_ID" \
    --password  "$APP_PASSWORD" \
    --team-id   "$TEAM_ID" \
    --wait

# Staple the binary inside the archive so it can be verified offline.
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

unzip -q "$ARCHIVE" -d "$TMPDIR"
BINARY="$TMPDIR/ghnotify"
if [[ -f "$BINARY" ]]; then
    echo "notarize.sh: stapling '$BINARY'..." >&2
    xcrun stapler staple "$BINARY"
    # Replace the unstabled binary in the archive with the stapled one.
    (cd "$TMPDIR" && zip -j - ghnotify) > "${ARCHIVE}.tmp"
    mv "${ARCHIVE}.tmp" "$ARCHIVE"
fi

echo "notarize.sh: done." >&2
