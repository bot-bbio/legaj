#!/usr/bin/env bash
#
# Builds LeGaJ for macOS: freezes the Python helpers, packages the Fyne GUI into
# a .app bundle, and embeds the frozen tools inside it.
#
# Option B distribution — no Python required on the target Mac.
#
# Pipeline:
#   1. PyInstaller freezes scripts/legaj_tools.py -> dist/legaj-tools
#   2. fyne packages the Go GUI                   -> LeGaJ.app
#   3. Copy legaj-tools into LeGaJ.app/Contents/MacOS/tools/
#   4. (Optional) create a .dmg for distribution
#
# IMPORTANT: Fyne uses cgo, so this MUST be run ON a Mac (you cannot cross-compile
# the macOS GUI from Windows). Run PyInstaller on the same Mac so the frozen tool
# matches the target architecture (arm64 or x86_64).
#
# Usage (from the project root, on macOS):
#   ./build_macos.sh
#
# See PACKAGING.md for the macOS working-directory caveat and code signing.

set -euo pipefail
cd "$(dirname "$0")"

PYTHON="${LEGAJ_PYTHON:-python3}"
GO="${LEGAJ_GO:-go}"

echo "==> Installing Python build/runtime dependencies"
"$PYTHON" -m pip install --upgrade pip
"$PYTHON" -m pip install -r requirements.txt
"$PYTHON" -m pip install pyinstaller

echo "==> Freezing Python helpers with PyInstaller"
"$PYTHON" -m PyInstaller --noconfirm --clean legaj-tools.spec
test -f dist/legaj-tools || { echo "PyInstaller did not produce dist/legaj-tools"; exit 1; }

echo "==> Building the Fyne GUI (.app bundle)"
if ! command -v fyne >/dev/null 2>&1; then
  echo "    fyne CLI not found; installing..."
  "$GO" install fyne.io/fyne/v2/cmd/fyne@latest
  export PATH="$("$GO" env GOPATH)/bin:$PATH"
fi

if [ -f Icon.png ]; then
  fyne package --os darwin --icon Icon.png --release
else
  echo "    Icon.png not found — packaging with the default Fyne icon."
  fyne package --os darwin
fi
test -d LeGaJ.app || { echo "fyne did not produce LeGaJ.app"; exit 1; }

echo "==> Embedding frozen tools in the .app bundle"
mkdir -p "LeGaJ.app/Contents/MacOS/tools"
cp dist/legaj-tools "LeGaJ.app/Contents/MacOS/tools/legaj-tools"
chmod +x "LeGaJ.app/Contents/MacOS/tools/legaj-tools"

echo "==> Creating a distributable .dmg (optional)"
if command -v hdiutil >/dev/null 2>&1; then
  rm -f dist/LeGaJ.dmg
  mkdir -p dist
  hdiutil create -volname "LeGaJ" -srcfolder "LeGaJ.app" -ov -format UDZO dist/LeGaJ.dmg
  echo "    dist/LeGaJ.dmg created"
fi

echo "==> Done. App bundle: LeGaJ.app"
echo "    NOTE: see PACKAGING.md — the app uses relative data paths; review the"
echo "    macOS working-directory caveat before distributing widely."
