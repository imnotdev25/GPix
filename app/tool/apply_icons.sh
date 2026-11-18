#!/usr/bin/env bash
# Apply assets/icon/gpix.png as the app icon on every generated platform.
#
# flutter_launcher_icons fails outright when its config names a platform whose
# folder doesn't exist (e.g. iOS/Android when only macos/ was generated), so this
# writes a temporary config enabling just the platforms present, then runs it.
# Linux isn't supported by flutter_launcher_icons; tool/setup_linux.sh handles it.
#
# Usage (from anywhere):  bash tool/apply_icons.sh
set -euo pipefail

cd "$(dirname "$0")/.."

has() { [ -d "$1" ] && echo true || echo false; }
android="$(has android)"
ios="$(has ios)"
macos="$(has macos)"
windows="$(has windows)"

if [ "$android$ios$macos$windows" = "falsefalsefalsefalse" ]; then
  echo "skip: no android/ ios/ macos/ windows/ folder — run 'flutter create' first"
  exit 0
fi

mkdir -p .dart_tool
cfg=".dart_tool/gpix_launcher_icons.yaml"
cat > "$cfg" <<EOF
flutter_launcher_icons:
  image_path: "assets/icon/gpix.png"
  # Phones crop icons to their own shape, so they get the edge-to-edge artwork.
  image_path_android: "assets/icon/gpix_full.png"
  image_path_ios: "assets/icon/gpix_full.png"
  android: $android
  ios: $ios
  remove_alpha_ios: true
  macos:
    generate: $macos
    image_path: "assets/icon/gpix.png"
  windows:
    generate: $windows
    image_path: "assets/icon/gpix.png"
    icon_size: 256
EOF

echo "==> Applying gpix icon (android=$android ios=$ios macos=$macos windows=$windows)"
flutter pub get >/dev/null
dart run flutter_launcher_icons -f "$cfg"
