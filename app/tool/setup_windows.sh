#!/usr/bin/env bash
# Patch the generated windows/ runner (git-ignored, created by `flutter create`)
# so the window title and the .exe's file properties say "gpix" rather than
# "gpix_app". The icon itself comes from tool/apply_icons.sh.
#
# Runs in Git Bash on Windows (and anywhere with perl). Safe to run repeatedly.
# Usage (from anywhere):  bash tool/setup_windows.sh
set -euo pipefail

cd "$(dirname "$0")/.."

main=windows/runner/main.cpp
rc=windows/runner/Runner.rc
if [ ! -f "$main" ] || [ ! -f "$rc" ]; then
  echo "error: windows/ not found — run 'flutter create --platforms=windows --org com.gpix --project-name gpix_app .' first" >&2
  exit 1
fi

perl -pi -e 's/L"gpix_app"/L"gpix"/' "$main"
perl -pi -e 's/(VALUE "(?:FileDescription|ProductName)", )"gpix_app"/$1"gpix"/' "$rc"

echo "patched: $main, $rc"
