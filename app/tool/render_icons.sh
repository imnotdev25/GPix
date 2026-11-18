#!/usr/bin/env bash
# Render the icon sources in assets/icon/*.svg to 1024x1024 PNGs with headless
# Chrome (macOS). Re-run after editing an SVG, then tool/apply_icons.sh.
#   gpix.svg      -> gpix.png       desktop (macOS, Windows, Linux)
#   gpix_full.svg -> gpix_full.png  edge-to-edge, for Android and iOS
set -euo pipefail

cd "$(dirname "$0")/.."

CHROME="${CHROME:-/Applications/Google Chrome.app/Contents/MacOS/Google Chrome}"
if [ ! -x "$CHROME" ]; then
  echo "error: Chrome not found; set CHROME=/path/to/chrome" >&2
  exit 1
fi

for name in gpix gpix_full; do
  svg="assets/icon/$name.svg"
  png="assets/icon/$name.png"
  "$CHROME" --headless=new --disable-gpu --hide-scrollbars \
    --force-device-scale-factor=1 --default-background-color=00000000 \
    --window-size=1024,1024 --screenshot="$PWD/$png" "file://$PWD/$svg" >/dev/null 2>&1
  echo "rendered $png"
done
