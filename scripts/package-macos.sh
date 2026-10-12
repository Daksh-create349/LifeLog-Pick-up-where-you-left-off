#!/usr/bin/env bash
set -euo pipefail

# LIFELOG V2-M6 — Reproducible macOS Packaging Script
# Architecture: Apple Silicon (darwin/arm64)

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "=========================================="
echo "LIFELOG V2 Production Packaging (macOS arm64)"
echo "=========================================="

cd "$REPO_ROOT"

# 1. Compile pure-Go zero-CGO binary
echo "==> 1. Compiling pure-Go daemon binary (CGO_ENABLED=0)..."
CGO_ENABLED=0 go build -ldflags="-s -w" -o lifelog .
echo "    Built: lifelog ($(du -h lifelog | awk '{print $1}'))"

# 2. Compile native Swift AppKit observer helper
echo "==> 2. Compiling native Swift observer helper..."
mkdir -p bin
swiftc -O cmd/app-observer/main.swift -o bin/lifelog-app-observer
echo "    Built: bin/lifelog-app-observer ($(du -h bin/lifelog-app-observer | awk '{print $1}'))"

# 3. Build Electron production bundles (TypeScript main + Vite renderer)
echo "==> 3. Building Electron main & renderer bundles..."
cd "$REPO_ROOT/desktop"
npm run build

# 4. Package Electron app bundle with @electron/packager
echo "==> 4. Packaging macOS application with @electron/packager..."
rm -rf dist-package
npx @electron/packager . "LifeLog" \
  --platform=darwin \
  --arch=arm64 \
  --out=dist-package \
  --overwrite \
  --app-bundle-id="com.lifelog.desktop" \
  --app-version="0.2.0" \
  --build-version="0.2.0" \
  --app-category-type="public.app-category.productivity" \
  --prune=true \
  --ignore="^/(src|tests|screenshots|tsconfig.*|\.vite)"

APP_BUNDLE="$REPO_ROOT/desktop/dist-package/LifeLog-darwin-arm64/LifeLog.app"

if [ ! -d "$APP_BUNDLE" ]; then
  echo "Error: Application bundle not found at $APP_BUNDLE" >&2
  exit 1
fi

# 5. Embed runtime binaries into Contents/Resources/bin
echo "==> 5. Embedding Go daemon and Swift helper into app resources..."
RESOURCES_BIN="$APP_BUNDLE/Contents/Resources/bin"
mkdir -p "$RESOURCES_BIN"
cp "$REPO_ROOT/lifelog" "$RESOURCES_BIN/lifelog"
cp "$REPO_ROOT/bin/lifelog-app-observer" "$RESOURCES_BIN/lifelog-app-observer"
chmod 755 "$RESOURCES_BIN/lifelog" "$RESOURCES_BIN/lifelog-app-observer"

# 6. Verify packaged bundle contents
echo "==> 6. Verifying packaged application bundle..."
test -f "$APP_BUNDLE/Contents/MacOS/LifeLog"
test -f "$APP_BUNDLE/Contents/Resources/bin/lifelog"
test -f "$APP_BUNDLE/Contents/Resources/bin/lifelog-app-observer"
test -f "$APP_BUNDLE/Contents/Resources/app.asar"
test -f "$APP_BUNDLE/Contents/Info.plist"
echo "    Bundle verification passed!"

# 7. Create DMG distributable
echo "==> 7. Creating standalone DMG installer..."
DMG_PATH="$REPO_ROOT/desktop/dist-package/LifeLog-0.2.0-arm64.dmg"
rm -f "$DMG_PATH"
hdiutil create -volname "LifeLog" -srcfolder "$APP_BUNDLE" -ov -format UDZO "$DMG_PATH"
echo "    Created DMG: $DMG_PATH ($(du -h "$DMG_PATH" | awk '{print $1}'))"

# 8. Create portable ZIP distributable
echo "==> 8. Creating standalone portable ZIP distributable..."
ZIP_PATH="$REPO_ROOT/desktop/dist-package/LifeLog-0.2.0-macOS-arm64.zip"
rm -f "$ZIP_PATH"
(cd "$REPO_ROOT/desktop/dist-package" && zip -r -q -y "LifeLog-0.2.0-macOS-arm64.zip" "LifeLog-darwin-arm64/LifeLog.app")
echo "    Created ZIP: $ZIP_PATH ($(du -h "$ZIP_PATH" | awk '{print $1}'))"

echo "=========================================="
echo "Packaging complete!"
echo "App Bundle: $APP_BUNDLE"
echo "DMG File:   $DMG_PATH"
echo "ZIP File:   $ZIP_PATH"
echo "=========================================="
