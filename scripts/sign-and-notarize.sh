#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"

APP_NAME="Cubicle"
APP_DIR="$ROOT_DIR/.build/app/$APP_NAME.app"
ZIP_PATH="$ROOT_DIR/.build/app/$APP_NAME-notarize.zip"
INSTALL_APP_DIR="/Applications/$APP_NAME.app"
ENTITLEMENTS_FILE="$ROOT_DIR/scripts/Cubicle.entitlements"

SIGN_IDENTITY="${SIGN_IDENTITY:-Developer ID Application: Prabhat Singh (962J6UC69Z)}"
TEAM_ID="${TEAM_ID:-962J6UC69Z}"
KEYCHAIN_PROFILE="${KEYCHAIN_PROFILE:-}"
APPLE_ID="${APPLE_ID:-}"
APP_PASSWORD="${APP_PASSWORD:-}"

if [[ ! -d "$APP_DIR" ]]; then
  echo "Missing app bundle at $APP_DIR. Build it first with scripts/build-app.sh." >&2
  exit 1
fi

echo "Signing $APP_DIR with identity: $SIGN_IDENTITY"
codesign --force --deep --options runtime --timestamp --entitlements "$ENTITLEMENTS_FILE" --sign "$SIGN_IDENTITY" "$APP_DIR"
codesign --verify --deep --strict --verbose=2 "$APP_DIR"

rm -f "$ZIP_PATH"
/usr/bin/ditto -c -k --keepParent "$APP_DIR" "$ZIP_PATH"
echo "Created notarization archive: $ZIP_PATH"

submit_with_keychain_profile() {
  xcrun notarytool submit "$ZIP_PATH" \
    --keychain-profile "$KEYCHAIN_PROFILE" \
    --wait
}

submit_with_apple_id() {
  xcrun notarytool submit "$ZIP_PATH" \
    --apple-id "$APPLE_ID" \
    --team-id "$TEAM_ID" \
    --password "$APP_PASSWORD" \
    --wait
}

if [[ -n "$KEYCHAIN_PROFILE" ]]; then
  echo "Submitting for notarization using keychain profile: $KEYCHAIN_PROFILE"
  submit_with_keychain_profile
elif [[ -n "$APPLE_ID" && -n "$APP_PASSWORD" ]]; then
  echo "Submitting for notarization using Apple ID credentials."
  submit_with_apple_id
else
  cat >&2 <<'EOF'
Notarization credentials are missing.
Set one of:
  1) KEYCHAIN_PROFILE=<profile>
  2) APPLE_ID=<apple-id> APP_PASSWORD=<app-specific-password> TEAM_ID=<team-id>
Then rerun this script.
EOF
  exit 2
fi

echo "Stapling notarization ticket..."
xcrun stapler staple "$APP_DIR"
xcrun stapler validate "$APP_DIR"
spctl --assess --type execute --verbose=4 "$APP_DIR"

echo "Refreshing installed app at $INSTALL_APP_DIR"
rm -rf "$INSTALL_APP_DIR"
/usr/bin/ditto "$APP_DIR" "$INSTALL_APP_DIR"

echo "Done. Signed, notarized, stapled, and installed: $INSTALL_APP_DIR"
