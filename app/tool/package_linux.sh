#!/usr/bin/env bash
# Package a built Linux desktop app (`flutter build linux --release`) as:
#   out/gpix_<ver>_linux-<arch>.tar.gz   bundle + .desktop + icon + install.sh
#   out/gpix_<ver>_<debarch>.deb         installs to /opt/gpix, `gpix-app` command
#   out/gpix_<ver>_linux-<arch>.AppImage single-file, runs anywhere with GTK 3
#
# Usage (from anywhere):  bash tool/package_linux.sh [version]
# Env:  FORMATS="tar deb appimage"  (default: all three)
#       APPIMAGETOOL=/path/to/appimagetool  (otherwise downloaded for x86_64)
# Needs: dpkg-deb for .deb; ImageMagick (convert/magick) for sized deb icons
# (optional); network access for appimagetool if it isn't provided.
set -euo pipefail

cd "$(dirname "$0")/.."

VERSION="${1:-${GITHUB_REF_NAME:-dev}}"
FORMATS="${FORMATS:-tar deb appimage}"
want() { case " $FORMATS " in *" $1 "*) return 0;; esac; return 1; }

BUNDLE="$(ls -d build/linux/*/release/bundle 2>/dev/null | head -n1 || true)"
if [ -z "$BUNDLE" ] || [ ! -x "$BUNDLE/gpix_app" ]; then
  echo "error: no release bundle under build/linux/*/release/bundle — run 'flutter build linux --release' first" >&2
  exit 1
fi
case "$BUNDLE" in
  build/linux/arm64/*) ARCH=arm64; DEB_ARCH=arm64; AI_ARCH=aarch64 ;;
  *)                   ARCH=x64;   DEB_ARCH=amd64; AI_ARCH=x86_64 ;;
esac

ICON=assets/icon/gpix.png
DESKTOP=packaging/linux/gpix.desktop
OUT=out
mkdir -p "$OUT"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

resize() { # resize <src> <px> <dst>
  if command -v magick >/dev/null; then magick "$1" -resize "${2}x${2}" "$3"
  elif command -v convert >/dev/null; then convert "$1" -resize "${2}x${2}" "$3"
  else return 1; fi
}

# ---- tar.gz --------------------------------------------------------------
if want tar; then
  T="$WORK/gpix"
  mkdir -p "$T"
  cp -R "$BUNDLE" "$T/bundle"
  cp "$ICON" "$T/gpix.png"
  cp "$DESKTOP" "$T/gpix.desktop"
  cp packaging/linux/install.sh "$T/install.sh"
  chmod +x "$T/install.sh"
  tar -C "$WORK" -czf "$OUT/gpix_${VERSION}_linux-${ARCH}.tar.gz" gpix
  echo "✅ $OUT/gpix_${VERSION}_linux-${ARCH}.tar.gz"
fi

# ---- .deb ----------------------------------------------------------------
if want deb; then
  DEB_VERSION="${VERSION#v}"
  case "$DEB_VERSION" in [0-9]*) ;; *) DEB_VERSION="0.0.0~dev" ;; esac
  D="$WORK/deb"
  mkdir -p "$D/DEBIAN" "$D/opt/gpix" "$D/usr/bin" "$D/usr/share/applications" "$D/usr/share/pixmaps"
  cp -R "$BUNDLE/." "$D/opt/gpix/"
  ln -s /opt/gpix/gpix_app "$D/usr/bin/gpix-app"
  sed "s|@EXEC@|/opt/gpix/gpix_app|" "$DESKTOP" > "$D/usr/share/applications/com.gpix.gpix_app.desktop"
  cp "$ICON" "$D/usr/share/pixmaps/gpix.png"
  for px in 48 64 128 256 512; do
    dir="$D/usr/share/icons/hicolor/${px}x${px}/apps"
    mkdir -p "$dir"
    if ! resize "$ICON" "$px" "$dir/gpix.png"; then
      rm -rf "$D/usr/share/icons"
      echo "note: ImageMagick not found; the .deb ships only /usr/share/pixmaps/gpix.png"
      break
    fi
  done
  SIZE_KB="$(du -sk "$D/opt" "$D/usr" | awk '{s+=$1} END {print s}')"
  cat > "$D/DEBIAN/control" <<EOF
Package: gpix-app
Version: $DEB_VERSION
Architecture: $DEB_ARCH
Maintainer: gpix <noreply@localhost>
Installed-Size: $SIZE_KB
Depends: libgtk-3-0 | libgtk-3-0t64
Section: graphics
Priority: optional
Description: Desktop client for a self-hosted gpix media server
 Browse, upload and share your gpix library from the desktop.
EOF
  dpkg-deb --build --root-owner-group "$D" "$OUT/gpix_${VERSION}_${DEB_ARCH}.deb" >/dev/null
  echo "✅ $OUT/gpix_${VERSION}_${DEB_ARCH}.deb"
fi

# ---- AppImage ------------------------------------------------------------
if want appimage; then
  A="$WORK/AppDir"
  mkdir -p "$A/usr/lib/gpix"
  cp -R "$BUNDLE/." "$A/usr/lib/gpix/"
  cp "$ICON" "$A/gpix.png"
  ln -s gpix.png "$A/.DirIcon"
  sed "s|@EXEC@|gpix_app|" "$DESKTOP" > "$A/gpix.desktop"
  cat > "$A/AppRun" <<'EOF'
#!/bin/sh
HERE="$(dirname "$(readlink -f "$0")")"
exec "$HERE/usr/lib/gpix/gpix_app" "$@"
EOF
  chmod +x "$A/AppRun"

  TOOL="${APPIMAGETOOL:-}"
  if [ -z "$TOOL" ]; then
    TOOL="$WORK/appimagetool"
    curl -fsSL -o "$TOOL" \
      "https://github.com/AppImage/appimagetool/releases/download/continuous/appimagetool-${AI_ARCH}.AppImage"
    chmod +x "$TOOL"
  fi
  # CI runners have no FUSE; run the tool from its extracted payload instead.
  APPIMAGE_EXTRACT_AND_RUN=1 ARCH="$AI_ARCH" "$TOOL" --no-appstream "$A" \
    "$OUT/gpix_${VERSION}_linux-${ARCH}.AppImage" >/dev/null
  echo "✅ $OUT/gpix_${VERSION}_linux-${ARCH}.AppImage"
fi
