#!/usr/bin/env sh
# Register the GitHub Actions secrets a macOS runner needs to sign and notarize
# the build. The Apple account values (APPLE_ID / APPLE_PASSWORD /
# APPLE_TEAM_ID) are read from `.env.signing`; the certificate comes from a
# password-protected .p12 exported from Keychain Access, and
# APPLE_SIGNING_IDENTITY is derived from that .p12's certificate common name.
#
# The identity is derived from the .p12 rather than typed into `.env.signing`:
# the release workflow imports APPLE_CERTIFICATE into a temporary keychain and
# passes APPLE_SIGNING_IDENTITY to codesign, so the two must name the same
# certificate. A value copied by hand can drift from the .p12 actually uploaded.
#
# No secret value is printed: each is piped straight into `gh secret set`.
#
# Usage:
#   ./scripts/setup-ci-signing-secrets.sh path/to/DeveloperID.p12
#   ./scripts/setup-ci-signing-secrets.sh --update-key path/to/update-signing.key
#
# The second form registers only UPDATE_SIGNING_KEY, the Ed25519 key that signs
# the update files (create it with `go run ./tools/updatesig keygen`). It is its
# own path because the two rotate independently: replacing the update key must
# not require the certificate and its password, nor rewrite the Apple secrets.
# The key is checked against the public key committed in internal/updatesig
# before it is registered, since a mismatched key signs updates no client accepts
# and the release would not otherwise notice.
#
# Export the certificate first: Keychain Access > login > My Certificates >
# "Developer ID Application: <Name> (<TEAMID>)" > right-click > Export…, save as
# a .p12 and set an export password (you type it below).
#
# Prerequisites: `gh` authenticated for this repository and a filled-in
# `.env.signing` (see `.env.signing.example`).
set -eu

root=$(CDPATH= cd "$(dirname "$0")/.." && pwd)
env_file="$root/.env.signing"

usage() {
  echo "usage: $0 path/to/DeveloperID.p12" >&2
  echo "       $0 --update-key path/to/update-signing.key" >&2
  exit 1
}

update_key=''
p12=''
case "${1:-}" in
  '') usage ;;
  --update-key)
    update_key="${2:-}"
    [ -n "$update_key" ] || usage
    [ -f "$update_key" ] || { echo "error: key file '$update_key' not found." >&2; exit 1; }
    case "$update_key" in /*) ;; *) update_key="$PWD/$update_key" ;; esac
    ;;
  *)
    p12="$1"
    [ -f "$p12" ] || { echo "error: certificate '$p12' not found." >&2; exit 1; }
    # Pin the path now, before the `cd "$root"` below. A relative argument — which is
    # how the usage line above writes it — would otherwise pass this check and then
    # stop resolving, and openssl's discarded stderr turns that into "wrong export
    # password?" for a password that was right.
    case "$p12" in /*) ;; *) p12="$PWD/$p12" ;; esac
    [ -f "$env_file" ] || {
      echo "error: $env_file not found — copy .env.signing.example and fill it in." >&2
      exit 1
    }
    ;;
esac

# Six secrets are written one after another, so an unauthenticated `gh` would
# leave the repository holding a partial set. Check before writing any.
gh auth status >/dev/null 2>&1 || {
  echo "error: gh is not authenticated. Run 'gh auth login' first." >&2
  exit 1
}

# `gh secret set` resolves its repository from the working directory, not from
# $root, so running this by absolute path from another checkout would write a
# certificate and two passwords onto that repository instead. Resolve the target
# from $root, name it before the first write, and pin every call to it.
cd "$root"
repo=$(gh repo view --json nameWithOwner -q .nameWithOwner 2>/dev/null) || {
  echo "error: gh cannot resolve a repository from $root." >&2
  exit 1
}
echo "Target repository: $repo"

if [ -n "$update_key" ]; then
  go run ./tools/updatesig check-key -key-file "$update_key"
  tr -d '\n' < "$update_key" | gh secret set UPDATE_SIGNING_KEY --repo "$repo"
  echo "Registered UPDATE_SIGNING_KEY on $repo."
  echo "Verify with: gh secret list --repo $repo"
  exit 0
fi

# Load APPLE_ID / APPLE_PASSWORD / APPLE_TEAM_ID without echoing them.
# (APPLE_SIGNING_IDENTITY is derived from the .p12 below, not from this file.)
set -a
# shellcheck source=/dev/null
. "$env_file"
set +a

for var in APPLE_ID APPLE_PASSWORD APPLE_TEAM_ID; do
  eval "value=\${$var:-}"
  [ -n "$value" ] || { echo "error: $var is empty in $env_file." >&2; exit 1; }
done

# Prompt for the .p12 export password with echo off. Echo is restored through a
# trap as well as inline: under `set -e` a Ctrl-D makes `read` return non-zero
# and the script exits between the two stty calls, which would leave the
# operator's terminal silent until they think to run `stty sane`.
restore_echo() { stty echo 2>/dev/null || true; }
trap 'restore_echo' EXIT
trap 'restore_echo; trap - EXIT; exit 130' INT
trap 'restore_echo; trap - EXIT; exit 143' TERM
printf 'Export password for %s: ' "$p12"
stty -echo 2>/dev/null || true
IFS= read -r p12_password
restore_echo
trap - EXIT INT TERM
printf '\n'

# Derive the signing identity from the certificate inside the .p12 so it always
# matches APPLE_CERTIFICATE. Try modern openssl first, then -legacy (OpenSSL 3
# needs it to read the ciphers Keychain exports use; LibreSSL ignores the retry).
# The export password is fed on stdin (-passin stdin), never on argv, so it stays
# out of process listings.
signing_identity=''
for legacy in '' '-legacy'; do
  signing_identity=$(
    printf '%s' "$p12_password" \
      | openssl pkcs12 $legacy -in "$p12" -passin stdin -nokeys -clcerts 2>/dev/null \
      | openssl x509 -noout -subject -nameopt multiline 2>/dev/null \
      | sed -n 's/^[[:space:]]*commonName[[:space:]]*=[[:space:]]*//p' \
      | head -1
  )
  [ -n "$signing_identity" ] && break
done
if [ -z "$signing_identity" ]; then
  echo "error: could not read the certificate from $p12 — wrong export password?" >&2
  exit 1
fi
case "$signing_identity" in
  "Developer ID Application:"*) : ;;
  *) echo "warning: the .p12 is not a 'Developer ID Application' certificate;" \
          "notarization will be rejected." >&2 ;;
esac

# APPLE_CERTIFICATE is the base64-encoded .p12; APPLE_SIGNING_IDENTITY is the
# certificate's common name; the rest come from .env.signing. Piping keeps every
# value off the argv list and out of the logs.
base64 < "$p12"                 | gh secret set APPLE_CERTIFICATE           --repo "$repo"
printf '%s' "$p12_password"     | gh secret set APPLE_CERTIFICATE_PASSWORD  --repo "$repo"
printf '%s' "$signing_identity" | gh secret set APPLE_SIGNING_IDENTITY      --repo "$repo"
printf '%s' "$APPLE_ID"         | gh secret set APPLE_ID                    --repo "$repo"
printf '%s' "$APPLE_PASSWORD"   | gh secret set APPLE_PASSWORD              --repo "$repo"
printf '%s' "$APPLE_TEAM_ID"    | gh secret set APPLE_TEAM_ID               --repo "$repo"

echo "Registered signing secrets on $repo."
echo "Verify with: gh secret list --repo $repo"
