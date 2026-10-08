#!/usr/bin/env bash
# Submits one file to Apple's notary service and waits for the verdict. Used by
# build.yml for both submissions a signed macOS build makes (the .app, then the
# .dmg), so the two cannot judge the outcome differently.
#
# The outcome is read from notarytool's JSON status rather than its exit code,
# and anything but Accepted prints the notary log, which names the binary at fault.
#
# Usage: scripts/ci-notarize.sh FILE   (needs APPLE_ID, APPLE_PASSWORD, APPLE_TEAM_ID)
set -euo pipefail

file="$1"
result="$(xcrun notarytool submit "$file" --apple-id "$APPLE_ID" --password "$APPLE_PASSWORD" \
  --team-id "$APPLE_TEAM_ID" --wait --output-format json)" || true
echo "$result"
if [ "$(jq -r .status <<< "$result")" != "Accepted" ]; then
  xcrun notarytool log "$(jq -r '.id // empty' <<< "$result" || true)" --apple-id "$APPLE_ID" \
    --password "$APPLE_PASSWORD" --team-id "$APPLE_TEAM_ID" || true
  echo "::error::notarization of $file was not accepted"
  exit 1
fi
