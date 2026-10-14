#!/usr/bin/env bash
# Checks the asset mapping of install.sh without downloading anything. Run: bash scripts/test-install.sh
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
fail=0
check() { # os arch version want
  got="$(PROBE_INSTALL_DRY_RUN=1 PROBE_OS="$1" PROBE_ARCH="$2" PROBE_VERSION="$3" bash "$here/install.sh" 2>&1 || true)"
  if [ "$got" != "$4" ]; then echo "FAIL $1/$2/$3: got '$got', want '$4'"; fail=1; fi
}
b=https://github.com/AlphaWaveSystems/flutter-probe/releases
check Linux X64 latest "$b/latest/download/probe-linux-amd64"
check macOS ARM64 latest "$b/latest/download/probe-darwin-arm64"
check macOS X64 0.25.2 "$b/download/v0.25.2/probe-darwin-amd64"
check macOS ARM64 v0.25.2 "$b/download/v0.25.2/probe-darwin-arm64"
check Windows X64 0.25.2 "$b/download/v0.25.2/probe-windows-amd64.exe"
out="$(PROBE_INSTALL_DRY_RUN=1 PROBE_OS=Linux PROBE_ARCH=ARM64 bash "$here/install.sh" 2>&1 || true)"
case "$out" in *"No FlutterProbe CLI release asset"*) ;; *) echo "FAIL Linux/ARM64 must be refused: $out"; fail=1 ;; esac
[ "$fail" = 0 ] && echo "install.sh asset mapping: ok"
exit "$fail"
