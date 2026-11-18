#!/usr/bin/env bash
# Build the gpix macOS app in release mode and package it as a .dmg.
#
# Usage (from the app/ directory):
#   bash tool/build_macos_dmg.sh
#
# Output: build/gpix.dmg  (drag the app onto Applications to install)
#
# The build is UNSIGNED. Gatekeeper will warn on first launch — recipients can
# right-click the app → Open, or run:
#   xattr -dr com.apple.quarantine /Applications/gpix_app.app
# For distribution without warnings you need an Apple Developer ID certificate
# plus notarization (see the notes at the bottom of app/README.md).
set -euo pipefail

cd "$(dirname "$0")/.."

VOL_NAME="gpix"
OUT_DMG="build/${VOL_NAME}.dmg"

# 1. Ensure the macOS platform exists (it's generated / git-ignored).
if [ ! -d macos ]; then
  echo "==> Generating macOS platform files"
  flutter create --platforms=macos --org com.gpix --project-name gpix_app .
fi

# 2. Grant outbound network access (sandboxed apps are blocked by default).
echo "==> Patching entitlements (network client)"
bash tool/setup_macos.sh >/dev/null

# 3. Apply the gpix app icon. Without this step the app ships with Flutter's
#    default icon.
bash tool/apply_icons.sh

# 4. Build the release app.
echo "==> flutter build macos --release"
flutter build macos --release

APP_PATH="$(ls -d build/macos/Build/Products/Release/*.app 2>/dev/null | head -n1 || true)"
if [ -z "${APP_PATH:-}" ] || [ ! -d "$APP_PATH" ]; then
  echo "error: built .app not found under build/macos/Build/Products/Release/" >&2
  exit 1
fi
echo "==> Built: $APP_PATH"

# 5. Stage the app + an Applications symlink, then create a compressed DMG.
echo "==> Packaging $OUT_DMG"
STAGE="$(mktemp -d)"
RW_DMG="$(mktemp -u).dmg"
MNT=""
cleanup() {
  [ -n "$MNT" ] && hdiutil detach -quiet "$MNT" 2>/dev/null || true
  rm -rf "$STAGE" "$RW_DMG"
}
trap cleanup EXIT
cp -R "$APP_PATH" "$STAGE/"
ln -s /Applications "$STAGE/Applications"

# Give the mounted disk image the gpix icon too (Finder reads .VolumeIcon.icns
# when the volume carries the "custom icon" flag, which SetFile sets below).
ICNS="$APP_PATH/Contents/Resources/AppIcon.icns"
if [ -f "$ICNS" ]; then
  cp "$ICNS" "$STAGE/.VolumeIcon.icns"
fi

mkdir -p "$(dirname "$OUT_DMG")"
rm -f "$OUT_DMG"
hdiutil create -quiet -volname "$VOL_NAME" -srcfolder "$STAGE" -fs HFS+ -format UDRW -ov "$RW_DMG"
if [ -f "$STAGE/.VolumeIcon.icns" ] && command -v SetFile >/dev/null; then
  MNT="$(mktemp -d)"
  hdiutil attach -quiet -nobrowse -noautoopen -mountpoint "$MNT" "$RW_DMG"
  SetFile -a C "$MNT"
  hdiutil detach -quiet "$MNT"
  MNT=""
fi
hdiutil convert -quiet "$RW_DMG" -format UDZO -o "$OUT_DMG"

echo ""
echo "✅ Created $OUT_DMG"
echo "   Unsigned build — first launch: right-click the app → Open."
