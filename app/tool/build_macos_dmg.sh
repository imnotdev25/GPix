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

# 3. Build the release app.
echo "==> flutter build macos --release"
flutter build macos --release

APP_PATH="$(ls -d build/macos/Build/Products/Release/*.app 2>/dev/null | head -n1 || true)"
if [ -z "${APP_PATH:-}" ] || [ ! -d "$APP_PATH" ]; then
  echo "error: built .app not found under build/macos/Build/Products/Release/" >&2
  exit 1
fi
echo "==> Built: $APP_PATH"

# 4. Stage the app + an Applications symlink, then create a compressed DMG.
echo "==> Packaging $OUT_DMG"
STAGE="$(mktemp -d)"
trap 'rm -rf "$STAGE"' EXIT
cp -R "$APP_PATH" "$STAGE/"
ln -s /Applications "$STAGE/Applications"

mkdir -p "$(dirname "$OUT_DMG")"
rm -f "$OUT_DMG"
hdiutil create \
  -volname "$VOL_NAME" \
  -srcfolder "$STAGE" \
  -fs HFS+ \
  -format UDZO \
  -ov \
  "$OUT_DMG"

echo ""
echo "✅ Created $OUT_DMG"
echo "   Unsigned build — first launch: right-click the app → Open."
