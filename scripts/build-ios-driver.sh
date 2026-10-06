#!/usr/bin/env bash
#
# Build the FlutterProbe iOS system-dialog driver (the XCUITest runner in
# ios-driver/) for the iOS Simulator and package it as probe-ios-driver.zip.
#
# Usage:
#   scripts/build-ios-driver.sh [output-dir]      # default: bin/
#
# Needs macOS, Xcode and xcodegen (brew install xcodegen). The zip holds the
# .xctestrun file plus the built apps; `probe ios-driver install` downloads it
# from the GitHub release and `xcodebuild test-without-building` runs it, so
# end users need Xcode but never build it themselves.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="${1:-$ROOT/bin}"
VERSION="$(tr -d '[:space:]' < "$ROOT/VERSION")"
mkdir -p "$OUT"
OUT="$(cd "$OUT" && pwd)"

command -v xcodegen >/dev/null || { echo "xcodegen not found (brew install xcodegen)" >&2; exit 1; }
command -v xcodebuild >/dev/null || { echo "xcodebuild not found (install Xcode)" >&2; exit 1; }

cd "$ROOT/ios-driver"
# The driver reports the CLI version it was built for.
sed -i.bak "s/static let version = \".*\"/static let version = \"$VERSION\"/" UITests/Driver.swift
rm -f UITests/Driver.swift.bak

xcodegen generate --quiet
rm -rf build
xcodebuild build-for-testing \
  -project ProbeDriver.xcodeproj -scheme ProbeDriver \
  -destination 'generic/platform=iOS Simulator' \
  -derivedDataPath build \
  CODE_SIGNING_ALLOWED=NO | tail -3

STAGE="$(mktemp -d)"
trap 'rm -rf "$STAGE"' EXIT
cp build/Build/Products/*.xctestrun "$STAGE/"
cp -R build/Build/Products/Debug-iphonesimulator "$STAGE/"
# Drop debug-only artifacts the runner does not need.
find "$STAGE" -name '*.swiftmodule' -prune -exec rm -rf {} + 2>/dev/null || true
find "$STAGE" -name '*.dSYM' -prune -exec rm -rf {} + 2>/dev/null || true

rm -f "$OUT/probe-ios-driver.zip"
(cd "$STAGE" && zip -qry "$OUT/probe-ios-driver.zip" .)
echo "built $OUT/probe-ios-driver.zip (version $VERSION)"
