#!/usr/bin/env bash
# Checks the download verification of step.sh without the network (file:// URLs): a matching digest installs and runs,
# a tampered download and a release without a digest stop the step. Run: bash test-step.sh
set -uo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
fail=0
note() { echo "FAIL $*"; fail=1; }
case "$(uname -s)/$(uname -m)" in
  Darwin/arm64) asset=probe-darwin-arm64 ;; Darwin/x86_64) asset=probe-darwin-amd64 ;; Linux/x86_64) asset=probe-linux-amd64 ;;
  *) echo "unsupported host for this test"; exit 0 ;;
esac
sha() { if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | cut -d' ' -f1; else shasum -a 256 "$1" | cut -d' ' -f1; fi; }
T="$(mktemp -d)"; trap 'rm -rf "$T"' EXIT
mkdir -p "$T/rel/download/v9.9.9" "$T/api/tags" "$T/bin"
f="$T/rel/download/v9.9.9/$asset"
write_stub() { printf '#!/bin/sh\necho "STUB probe $*"\nexit 0\n' > "$f"; }
write_stub
good="$(sha "$f")"
printf '#!/bin/sh\necho "envman $*"\n' > "$T/bin/envman"; chmod +x "$T/bin/envman"
write_api() { printf '{"assets":[{"name":"%s","digest":%s}]}' "$asset" "$1" > "$T/api/tags/v9.9.9"; }
run() { env PATH="$T/bin:$PATH" PROBE_RELEASES_URL="file://$T/rel" PROBE_API_URL="file://$T/api" probe_version=9.9.9 tests=tests/ "$@" bash "$here/step.sh" 2>&1; }

write_api "\"sha256:$good\""
out="$(run)"; rc=$?
{ [ $rc -eq 0 ] && case "$out" in *"Verified $asset"*) true ;; *) false ;; esac; } || note "a matching digest must install and run: rc=$rc $out"

printf '#!/bin/sh\necho tampered\n' > "$f"
out="$(run)"; rc=$?
{ [ $rc -ne 0 ] && case "$out" in *"SHA-256 mismatch"*) true ;; *) false ;; esac && case "$out" in *"STUB probe"*) false ;; *) true ;; esac; } || note "a tampered download must stop the step before probe runs: rc=$rc $out"

out="$(run skip_verify=yes)"; rc=$?
{ [ $rc -eq 0 ] && case "$out" in *"NOT verified"*) true ;; *) false ;; esac; } || note "skip_verify=yes must run and say so: rc=$rc $out"

write_stub; write_api "null"
out="$(run)"; rc=$?
{ [ $rc -ne 0 ] && case "$out" in *"does not publish a SHA-256 digest"*) true ;; *) false ;; esac; } || note "a release without a digest must stop the step: rc=$rc $out"

[ "$fail" = 0 ] && echo "step.sh: checksum path ok"
exit "$fail"
