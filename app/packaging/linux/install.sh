#!/usr/bin/env bash
# Install the gpix desktop app for the current user (no root needed).
# Run from the extracted tarball:   ./install.sh
# Remove again with:                ./install.sh --uninstall
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
data="${XDG_DATA_HOME:-$HOME/.local/share}"
dest="$data/gpix-app"
bin="$HOME/.local/bin/gpix-app"
desktop="$data/applications/com.gpix.gpix_app.desktop"
icon="$data/icons/hicolor/256x256/apps/gpix.png"

if [ "${1:-}" = "--uninstall" ]; then
  rm -rf "$dest" "$bin" "$desktop" "$icon"
  echo "gpix removed."
  exit 0
fi

rm -rf "$dest"
mkdir -p "$dest" "$(dirname "$bin")" "$(dirname "$desktop")" "$(dirname "$icon")"
cp -R "$here/bundle/." "$dest/"
ln -sf "$dest/gpix_app" "$bin"
cp "$here/gpix.png" "$icon"
sed "s|@EXEC@|$dest/gpix_app|" "$here/gpix.desktop" > "$desktop"
command -v update-desktop-database >/dev/null && update-desktop-database "$data/applications" >/dev/null 2>&1 || true
command -v gtk-update-icon-cache >/dev/null && gtk-update-icon-cache -q "$data/icons/hicolor" >/dev/null 2>&1 || true

echo "gpix installed. Launch it from your app menu, or run: gpix-app"
case ":$PATH:" in *":$HOME/.local/bin:"*) ;; *) echo "(add ~/.local/bin to PATH to use the gpix-app command)";; esac
