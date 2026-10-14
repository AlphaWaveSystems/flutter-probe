#!/usr/bin/env bash
# Checks install.sh without the network: the asset mapping, and the checksum path with a good, a tampered and an
# unpublished download (served from file:// URLs). Run: bash scripts/test-install.sh
set -uo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
fail=0
note() { echo "FAIL $*"; fail=1; }

# ---- asset mapping (dry run) ----
check() { # os arch version want
  got="$(PROBE_INSTALL_DRY_RUN=1 PROBE_OS="$1" PROBE_ARCH="$2" PROBE_VERSION="$3" bash "$here/install.sh" 2>&1 || true)"
  [ "$got" = "$4" ] || note "mapping $1/$2/$3: got '$got', want '$4'"
}
b=https://github.com/AlphaWaveSystems/flutter-probe/releases
check Linux X64 latest "$b/latest/download/probe-linux-amd64"
check macOS ARM64 latest "$b/latest/download/probe-darwin-arm64"
check macOS X64 0.25.2 "$b/download/v0.25.2/probe-darwin-amd64"
check macOS ARM64 v0.25.2 "$b/download/v0.25.2/probe-darwin-arm64"
check Windows X64 0.25.2 "$b/download/v0.25.2/probe-windows-amd64.exe"
out="$(PROBE_INSTALL_DRY_RUN=1 PROBE_OS=Linux PROBE_ARCH=ARM64 bash "$here/install.sh" 2>&1 || true)"
case "$out" in *"No FlutterProbe CLI release asset"*) ;; *) note "Linux/ARM64 must be refused: $out" ;; esac

# ---- checksum path ----
sha() { if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | cut -d' ' -f1; else shasum -a 256 "$1" | cut -d' ' -f1; fi; }
T="$(mktemp -d)"; trap 'rm -rf "$T"' EXIT
mkdir -p "$T/rel/download/v9.9.9" "$T/api/tags"
asset="$T/rel/download/v9.9.9/probe-linux-amd64"
printf '#!/bin/sh\necho "probe version 9.9.9"\n' > "$asset"
good="$(sha "$asset")"
write_api() { # digest-json-value
  printf '{"tag_name":"v9.9.9","assets":[{"name":"probe-darwin-arm64","digest":"sha256:%s"},{"name":"probe-linux-amd64","digest":%s}]}' "$(printf 'f%.0s' $(seq 64))" "$1" > "$T/api/tags/v9.9.9"
}
run() { # extra env...
  env PROBE_VERSION=9.9.9 PROBE_OS=Linux PROBE_ARCH=X64 PROBE_INSTALL_DIR="$T/bin" PROBE_RELEASES_URL="file://$T/rel" PROBE_API_URL="file://$T/api" "$@" bash "$here/install.sh" 2>&1
}

write_api "\"sha256:$good\""
rm -rf "$T/bin"; out="$(run)"; rc=$?
{ [ $rc -eq 0 ] && [ -x "$T/bin/probe" ] && case "$out" in *"Verified probe-linux-amd64"*) true ;; *) false ;; esac; } || note "a matching digest must install: rc=$rc $out"

printf '#!/bin/sh\necho "tampered"\n' > "$asset"      # the digest is for the original file
rm -rf "$T/bin"; out="$(run)"; rc=$?
{ [ $rc -ne 0 ] && [ ! -e "$T/bin/probe" ] && case "$out" in *"SHA-256 mismatch"*) true ;; *) false ;; esac; } || note "a tampered download must fail and install nothing: rc=$rc $out"

rm -rf "$T/bin"; out="$(run PROBE_SKIP_VERIFY=1)"; rc=$?
{ [ $rc -eq 0 ] && case "$out" in *"NOT verified"*) true ;; *) false ;; esac; } || note "PROBE_SKIP_VERIFY=1 must install and say so: rc=$rc $out"

printf '#!/bin/sh\necho "probe version 9.9.9"\n' > "$asset"
write_api "null"
rm -rf "$T/bin"; out="$(run)"; rc=$?
{ [ $rc -ne 0 ] && [ ! -e "$T/bin/probe" ] && case "$out" in *"does not publish a SHA-256 digest"*) true ;; *) false ;; esac; } || note "a release without a digest must fail: rc=$rc $out"

printf '{"tag_name":"v9.9.9","assets":[]}' > "$T/api/tags/v9.9.9"
rm -rf "$T/bin"; out="$(run)"; rc=$?
[ $rc -ne 0 ] || note "an asset missing from the release JSON must fail: $out"

[ "$fail" = 0 ] && echo "install.sh: asset mapping and checksum path ok"
exit "$fail"
