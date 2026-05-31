#!/usr/bin/env bash
# Build, Developer ID-sign, notarize, and staple a distributable macOS .dmg
# locally. Requires a paid Apple Developer Program membership with:
#   1. a "Developer ID Application" certificate installed in the login keychain
#      (check: security find-identity -v -p codesigning), and
#   2. a stored notarytool credential profile (create once with, e.g.:
#        xcrun notarytool store-credentials snzstudio \
#          --apple-id you@example.com --team-id ABCDE12345 \
#          --password <app-specific-password>)
#
# The free "Personal Team" (free provisioning) tier CANNOT do either — Developer
# ID signing and notarization are paid-program features.
#
# Usage:
#   scripts/build-mac-signed.sh
# Env overrides:
#   DEVELOPER_ID    signing identity. Default: the first "Developer ID
#                   Application" identity found in the keychain.
#   NOTARY_PROFILE  notarytool keychain profile name. Default: snzstudio.
#   PLATFORM        wails build target. Default: darwin/universal.
set -euo pipefail

cd "$(dirname "$0")/.."

PLATFORM="${PLATFORM:-darwin/universal}"
NOTARY_PROFILE="${NOTARY_PROFILE:-snzstudio}"
APP="build/bin/snz-studio.app"
DMG="build/bin/SNZ-Studio.dmg"
ENTITLEMENTS="build/darwin/entitlements.plist"
WAILS="${WAILS:-$HOME/go/bin/wails}"
# Bundled embedding sidecar (llama.cpp). The official prebuilt llama-server runs the
# ruri GGUF unmodified, so no self-build is needed; pin a release that contains the
# ModernBERT graph (>= b9437). SIDECAR_ARCH defaults to arm64 (Apple Silicon); a
# universal sidecar would require lipo-ing arm64 + x64 binaries and dylibs (TODO).
LLAMA_RELEASE="${LLAMA_RELEASE:-b9437}"
SIDECAR_ARCH="${SIDECAR_ARCH:-arm64}" # arm64 | x64

# Resolve the signing identity (env override, else first Developer ID Application).
if [[ -z "${DEVELOPER_ID:-}" ]]; then
  DEVELOPER_ID="$(security find-identity -v -p codesigning \
    | sed -n 's/.*"\(Developer ID Application:[^"]*\)".*/\1/p' | head -1)"
fi

echo "==> Pre-flight checks"
if [[ -z "${DEVELOPER_ID:-}" ]]; then
  echo "ERROR: no 'Developer ID Application' identity found." >&2
  echo "       Create one at developer.apple.com (Certificates -> + -> Developer ID" >&2
  echo "       Application) or via Xcode > Settings > Accounts > Manage Certificates," >&2
  echo "       then re-run. Available identities:" >&2
  security find-identity -v -p codesigning >&2 || true
  exit 1
fi
echo "    Signing identity: $DEVELOPER_ID"

if ! xcrun notarytool history --keychain-profile "$NOTARY_PROFILE" >/dev/null 2>&1; then
  echo "ERROR: notarytool profile '$NOTARY_PROFILE' not found." >&2
  echo "       Create it once with:" >&2
  echo "         xcrun notarytool store-credentials \"$NOTARY_PROFILE\" \\" >&2
  echo "           --apple-id <apple-id> --team-id <team-id> --password <app-specific-pw>" >&2
  exit 1
fi
echo "    Notary profile:   $NOTARY_PROFILE"

echo "==> Building app ($PLATFORM)"
"$WAILS" build -platform "$PLATFORM" -clean

ENT_ARGS=()
[[ -f "$ENTITLEMENTS" ]] && ENT_ARGS=(--entitlements "$ENTITLEMENTS") && echo "    entitlements: $ENTITLEMENTS"
# Expand the (possibly empty) array in a way that is safe under `set -u` on
# macOS's stock bash 3.2, where a bare "${arr[@]}" on an empty array errors.

echo "==> Staging the embedding sidecar (llama-server $LLAMA_RELEASE, macos-$SIDECAR_ARCH)"
RES="$APP/Contents/Resources"
STAGE="$(mktemp -d)"
TARBALL="llama-${LLAMA_RELEASE}-bin-macos-${SIDECAR_ARCH}.tar.gz"
curl -fsSL "https://github.com/ggml-org/llama.cpp/releases/download/${LLAMA_RELEASE}/${TARBALL}" -o "$STAGE/sidecar.tar.gz"
tar xzf "$STAGE/sidecar.tar.gz" -C "$STAGE"
SRCDIR="$(dirname "$(find "$STAGE" -name llama-server -type f | head -1)")"
# Keep only llama-server among the executables: the official tarball ships ~25 tools,
# and any unsigned extra Mach-O executable fails notarization (verified). Dylibs stay.
for f in "$SRCDIR"/*; do
  base="$(basename "$f")"
  if [[ -f "$f" && "$base" != "llama-server" && "$base" != *.dylib ]] && file "$f" | grep -q "Mach-O.*executable"; then
    rm -f "$f"
  fi
done
cp -R "$SRCDIR"/. "$RES"/
echo "    staged sidecar into $RES"

echo "==> Codesigning the sidecar first (inner-most), then the app (do NOT rely on --deep)"
# Sign every sidecar dylib with a hardened runtime + secure timestamp, then the
# llama-server executable with the JIT entitlements. Signing inner code before the
# outer bundle is the canonical order; the outer .app signing below seals these.
find "$RES" -type f -name "*.dylib" -print0 | while IFS= read -r -d '' lib; do
  codesign --force --options runtime --timestamp --sign "$DEVELOPER_ID" "$lib"
done
codesign --force --options runtime --timestamp \
  "${ENT_ARGS[@]+"${ENT_ARGS[@]}"}" --sign "$DEVELOPER_ID" "$RES/llama-server"

echo "==> Codesigning .app (hardened runtime + secure timestamp)"
codesign --force --options runtime --timestamp \
  "${ENT_ARGS[@]+"${ENT_ARGS[@]}"}" --sign "$DEVELOPER_ID" "$APP"
codesign --verify --strict --verbose=2 "$APP"

echo "==> Creating .dmg"
rm -f "$DMG"
hdiutil create -volname "SNZ Studio" -srcfolder "$APP" -ov -format UDZO "$DMG"

echo "==> Codesigning .dmg"
codesign --force --timestamp --sign "$DEVELOPER_ID" "$DMG"

echo "==> Submitting for notarization (this blocks until Apple responds)"
xcrun notarytool submit "$DMG" --keychain-profile "$NOTARY_PROFILE" --wait

echo "==> Stapling the notarization ticket"
xcrun stapler staple "$DMG"

echo "==> Final verification"
xcrun stapler validate "$DMG"
spctl -a -t open --context context:primary-signature -v "$DMG" || true

echo "==> Done: $DMG (signed + notarized + stapled)"
