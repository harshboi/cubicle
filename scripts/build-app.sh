#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"

mkdir -p "$ROOT_DIR/.build/clang-module-cache" "$ROOT_DIR/.build/swiftpm-cache"
export CLANG_MODULE_CACHE_PATH="$ROOT_DIR/.build/clang-module-cache"

swift build -c release --cache-path "$ROOT_DIR/.build/swiftpm-cache"

APP_NAME="Cubicle"
EXECUTABLE_NAME="Cubicle"
SWIFT_EXECUTABLE_NAME="Cubicle"
APP_DIR="$ROOT_DIR/.build/app/$APP_NAME.app"
MACOS_DIR="$APP_DIR/Contents/MacOS"
RESOURCES_DIR="$APP_DIR/Contents/Resources"
ICON_FILE="$RESOURCES_DIR/Cubicle.icns"
INSTALL_APP_DIR="/Applications/$APP_NAME.app"
INSTALLED_ICON_FILE="$INSTALL_APP_DIR/Contents/Resources/Cubicle.icns"
LEGACY_APP_DIR="$ROOT_DIR/.build/app/Mandrake.app"
LEGACY_INSTALL_APP_DIR="/Applications/Mandrake.app"
ENTITLEMENTS_FILE="$ROOT_DIR/scripts/Cubicle.entitlements"
DEFAULT_SIGN_IDENTITY="Developer ID Application: Prabhat Singh (962J6UC69Z)"
SIGN_IDENTITY="${SIGN_IDENTITY:-}"

if [[ -z "$SIGN_IDENTITY" ]] && security find-identity -v -p codesigning 2>/dev/null | grep -Fq "$DEFAULT_SIGN_IDENTITY"; then
  SIGN_IDENTITY="$DEFAULT_SIGN_IDENTITY"
fi

if [[ -n "$SIGN_IDENTITY" ]]; then
  SIGN_ARGS=(--options runtime --timestamp --sign "$SIGN_IDENTITY")
else
  SIGN_ARGS=(--sign -)
fi

rm -rf "$APP_DIR" "$LEGACY_APP_DIR"
mkdir -p "$MACOS_DIR" "$RESOURCES_DIR"

cat > "$APP_DIR/Contents/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleExecutable</key>
  <string>Cubicle</string>
  <key>CFBundleIdentifier</key>
  <string>local.cubicle.mac</string>
  <key>CFBundleDisplayName</key>
  <string>Cubicle</string>
  <key>CFBundleName</key>
  <string>Cubicle</string>
  <key>CFBundleIconFile</key>
  <string>Cubicle.icns</string>
  <key>CFBundlePackageType</key>
  <string>APPL</string>
  <key>CFBundleShortVersionString</key>
  <string>0.1.0</string>
  <key>CFBundleVersion</key>
  <string>1</string>
  <key>LSMinimumSystemVersion</key>
  <string>13.0</string>
  <key>NSHighResolutionCapable</key>
  <true/>
  <key>NSMicrophoneUsageDescription</key>
  <string>Cubicle uses microphone audio only when you start a live transcription session.</string>
  <key>NSDesktopFolderUsageDescription</key>
  <string>Cubicle reads and writes its local runtime data in getwebexspace-data when that folder is on your Desktop.</string>
  <key>NSDocumentsFolderUsageDescription</key>
  <string>Cubicle can read and write a user-selected runtime data folder if you keep it in Documents.</string>
</dict>
</plist>
PLIST

if python3 -c 'import PIL' >/dev/null 2>&1; then
  python3 "$ROOT_DIR/scripts/generate-app-icon.py" "$ICON_FILE"
elif [[ -f "$INSTALLED_ICON_FILE" ]]; then
  cp "$INSTALLED_ICON_FILE" "$ICON_FILE"
else
  echo "Pillow is not installed; continuing without regenerating Cubicle.icns" >&2
fi
cp "$ROOT_DIR/.build/release/$SWIFT_EXECUTABLE_NAME" "$MACOS_DIR/$EXECUTABLE_NAME"
chmod +x "$MACOS_DIR/$EXECUTABLE_NAME"
codesign --force --deep --entitlements "$ENTITLEMENTS_FILE" "${SIGN_ARGS[@]}" "$APP_DIR"

rm -rf "$INSTALL_APP_DIR" "$LEGACY_INSTALL_APP_DIR"
/usr/bin/ditto "$APP_DIR" "$INSTALL_APP_DIR"
codesign --verify --deep --strict --verbose=2 "$INSTALL_APP_DIR"

echo "Built $APP_DIR"
echo "Installed $INSTALL_APP_DIR"
if [[ -n "$SIGN_IDENTITY" ]]; then
  echo "Signed with $SIGN_IDENTITY"
else
  echo "Signed ad-hoc because no SIGN_IDENTITY was configured or discoverable"
fi
