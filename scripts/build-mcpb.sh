#!/usr/bin/env bash
#
# Build a Claude Desktop Extension (.mcpb) bundle for probe-mcp.
#
# Usage:
#   scripts/build-mcpb.sh <binary-path> <platform> <version> <output-dir>
#
# Arguments:
#   binary-path   Path to the probe-mcp binary to bundle.
#   platform      Target platform: darwin | linux | win32
#   version       Semver string for the manifest (e.g. 0.9.4).
#   output-dir    Directory the resulting flutter-probe-<platform>-<arch>.mcpb
#                 will be written to.
#
# The script also auto-detects the architecture from the binary's name
# (probe-mcp-darwin-arm64 → arm64) for the output filename.
#
# Requires: jq, npx (for @anthropic-ai/mcpb), zip.

set -euo pipefail

if [[ $# -ne 4 ]]; then
  echo "usage: $0 <binary-path> <platform> <version> <output-dir>" >&2
  exit 1
fi

BINARY="$1"
PLATFORM="$2"
VERSION="$3"
OUT_DIR="$4"

if [[ ! -f "$BINARY" ]]; then
  echo "error: binary not found: $BINARY" >&2
  exit 1
fi

case "$PLATFORM" in
  darwin|linux|win32) ;;
  *) echo "error: platform must be darwin|linux|win32, got: $PLATFORM" >&2; exit 1 ;;
esac

# Derive arch from binary filename suffix.
BINARY_BASENAME="$(basename "$BINARY")"
ARCH="amd64"
case "$BINARY_BASENAME" in
  *arm64*) ARCH="arm64" ;;
  *amd64*) ARCH="amd64" ;;
  *universal*) ARCH="universal" ;;
esac

# Pick the in-bundle binary name. On Windows it must end in .exe so the host
# spawns it correctly; elsewhere the name is plain.
case "$PLATFORM" in
  win32) BUNDLED_NAME="probe-mcp.exe" ;;
  *)     BUNDLED_NAME="probe-mcp" ;;
esac

mkdir -p "$OUT_DIR"
# Convert OUT_DIR (and BINARY) to absolute paths up-front so the subshell
# below — which does `cd "$WORK_DIR"` before invoking mcpb — resolves them
# correctly. Without this, a relative `mcpb-out/...` would land inside the
# tempdir and the upload step couldn't find it.
OUT_DIR="$(cd "$OUT_DIR" && pwd)"
BINARY="$(cd "$(dirname "$BINARY")" && pwd)/$(basename "$BINARY")"

WORK_DIR="$(mktemp -d)"
trap 'rm -rf "$WORK_DIR"' EXIT

# 1. Stage bundle directory.
cp "$BINARY" "$WORK_DIR/$BUNDLED_NAME"
chmod +x "$WORK_DIR/$BUNDLED_NAME"

# 2. Render manifest from template.
TEMPLATE="$(cd "$(dirname "$0")/.." && pwd)/mcpb/template/manifest.json"
if [[ ! -f "$TEMPLATE" ]]; then
  echo "error: manifest template not found: $TEMPLATE" >&2
  exit 1
fi

# Use sed (BSD-compatible) to substitute placeholders. The command path is
# bundle-relative via ${__dirname} so the host can locate the binary after
# unpacking.
sed -e "s|@@VERSION@@|$VERSION|g" \
    -e "s|@@ENTRY_POINT@@|$BUNDLED_NAME|g" \
    -e "s|@@COMMAND@@|\${__dirname}/$BUNDLED_NAME|g" \
    -e "s|@@PLATFORM@@|$PLATFORM|g" \
    "$TEMPLATE" > "$WORK_DIR/manifest.json"

# 3. Validate then pack.
(
  cd "$WORK_DIR"
  npx --yes @anthropic-ai/mcpb validate manifest.json
)

OUTPUT="$OUT_DIR/flutter-probe-$PLATFORM-$ARCH.mcpb"
(
  cd "$WORK_DIR"
  npx --yes @anthropic-ai/mcpb pack . "$OUTPUT"
)

echo "Built $OUTPUT"
ls -lh "$OUTPUT"
