#!/usr/bin/env bash
# Patch the generated linux/ runner (git-ignored, created by `flutter create`) so
# the desktop app looks like gpix instead of a Flutter template:
#   * window title "gpix" instead of "gpix_app"
#   * window icon loaded from data/gpix.png next to the executable
#   * assets/icon/gpix.png installed into the bundle's data/ directory
#
# Safe to run more than once. Usage (from anywhere):  bash tool/setup_linux.sh
set -euo pipefail

cd "$(dirname "$0")/.."

cc=linux/runner/my_application.cc
cm=linux/CMakeLists.txt
if [ ! -f "$cc" ] || [ ! -f "$cm" ]; then
  echo "error: linux/ not found — run 'flutter create --platforms=linux --org com.gpix --project-name gpix_app .' first" >&2
  exit 1
fi

perl -pi -e 's/"gpix_app"/"gpix"/g' "$cc"

if ! grep -q 'gpix: window icon' "$cc"; then
  GPIX_SNIPPET='
  // gpix: window icon (data/gpix.png next to the executable).
  {
    g_autofree gchar* exe = g_file_read_link("/proc/self/exe", nullptr);
    if (exe != nullptr) {
      g_autofree gchar* dir = g_path_get_dirname(exe);
      g_autofree gchar* icon =
          g_build_filename(dir, "data", "gpix.png", nullptr);
      gtk_window_set_icon_from_file(window, icon, nullptr);
    }
  }
' perl -0pi -e 's/(  gtk_window_set_default_size\(window, \d+, \d+\);\n)/$1$ENV{GPIX_SNIPPET}/' "$cc"
  if ! grep -q 'gpix: window icon' "$cc"; then
    echo "error: couldn't find gtk_window_set_default_size in $cc (Flutter template changed?)" >&2
    exit 1
  fi
fi

if ! grep -q 'gpix: window icon' "$cm"; then
  cat >> "$cm" <<'EOF'

# gpix: window icon, loaded by runner/my_application.cc from the bundle.
install(FILES "${CMAKE_CURRENT_SOURCE_DIR}/../assets/icon/gpix.png"
  DESTINATION "${INSTALL_BUNDLE_DATA_DIR}" COMPONENT Runtime)
EOF
fi

echo "patched: $cc, $cm"
