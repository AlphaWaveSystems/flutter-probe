#!/usr/bin/env bash
# Installs the FlutterProbe CLI from a GitHub release onto the PATH of the current job, after checking the download
# against the SHA-256 digest that GitHub publishes for the release asset (the "digest" field of the Releases API).
#
# Inputs (environment): PROBE_VERSION (a version like 0.26.0, or "latest"), PROBE_OS (Linux, macOS or Windows),
# PROBE_ARCH (X64 or ARM64; the values of runner.os and runner.arch), PROBE_INSTALL_DIR (default: $RUNNER_TEMP/probe-bin),
# GITHUB_TOKEN (optional; raises the API rate limit).
# PROBE_SKIP_VERIFY=1 skips the check (off by default; do not use it in CI you rely on).
# PROBE_INSTALL_DRY_RUN=1 only prints the download URL. PROBE_RELEASES_URL and PROBE_API_URL replace the GitHub URLs (tests).
set -euo pipefail

version="${PROBE_VERSION:-latest}"
os="${PROBE_OS:-$(uname -s)}"
arch="${PROBE_ARCH:-$(uname -m)}"

case "${os}/${arch}" in
  Linux/X64 | Linux/x86_64)   asset="probe-linux-amd64" ;;
  macOS/ARM64 | Darwin/arm64) asset="probe-darwin-arm64" ;;
  macOS/X64 | Darwin/x86_64)  asset="probe-darwin-amd64" ;;
  Windows/X64)                asset="probe-windows-amd64.exe" ;;
  *)
    echo "No FlutterProbe CLI release asset for ${os}/${arch}. Releases: https://github.com/AlphaWaveSystems/flutter-probe/releases" >&2
    exit 1
    ;;
esac

base="${PROBE_RELEASES_URL:-https://github.com/AlphaWaveSystems/flutter-probe/releases}"
api="${PROBE_API_URL:-https://api.github.com/repos/AlphaWaveSystems/flutter-probe/releases}"
if [ "$version" = "latest" ]; then
  url="${base}/latest/download/${asset}"
  api_url="${api}/latest"
else
  version="${version#v}"
  url="${base}/download/v${version}/${asset}"
  api_url="${api}/tags/v${version}"
fi

if [ "${PROBE_INSTALL_DRY_RUN:-}" = "1" ]; then
  echo "$url"
  exit 0
fi

# expected_digest prints the sha256 hex that the release publishes for $asset, or nothing.
expected_digest() {
  local json="$1" d=""
  if command -v jq >/dev/null 2>&1; then
    d="$(jq -r --arg n "$asset" '[.assets[]? | select(.name == $n) | .digest // empty] | first // empty' "$json" 2>/dev/null || true)"
  elif command -v python3 >/dev/null 2>&1; then
    d="$(python3 - "$json" "$asset" <<'PY' 2>/dev/null || true
import json, sys
for a in json.load(open(sys.argv[1])).get("assets", []):
    if a.get("name") == sys.argv[2] and a.get("digest"):
        print(a["digest"]); break
PY
)"
  fi
  case "$d" in sha256:*) echo "${d#sha256:}" ;; esac
}

file_sha256() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | cut -d' ' -f1
  else shasum -a 256 "$1" | cut -d' ' -f1; fi
}

# verify_download FILE JSON: fails unless FILE has the digest the release publishes.
verify_download() {
  local file="$1" json="$2" want got
  want="$(expected_digest "$json")"
  if [ -z "$want" ]; then
    echo "The release does not publish a SHA-256 digest for ${asset}, so the download cannot be verified. Set PROBE_SKIP_VERIFY=1 to install anyway." >&2
    return 1
  fi
  got="$(file_sha256 "$file")"
  if [ "$got" != "$want" ]; then
    echo "SHA-256 mismatch for ${asset}: expected ${want}, got ${got}. The download was not installed." >&2
    return 1
  fi
  echo "Verified ${asset} (sha256 ${got})"
}

dir="${PROBE_INSTALL_DIR:-${RUNNER_TEMP:-${TMPDIR:-/tmp}}/probe-bin}"
mkdir -p "$dir"
target="$dir/probe"
case "$asset" in *.exe) target="$dir/probe.exe" ;; esac
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

echo "Downloading $url"
curl --fail --silent --show-error --location --retry 3 --output "$work/asset" "$url"

if [ "${PROBE_SKIP_VERIFY:-}" = "1" ]; then
  echo "PROBE_SKIP_VERIFY=1: the download is NOT verified." >&2
else
  auth=()
  if [ -n "${GITHUB_TOKEN:-}" ]; then auth=(--header "Authorization: Bearer ${GITHUB_TOKEN}"); fi
  curl --fail --silent --show-error --location --retry 3 ${auth[@]+"${auth[@]}"} --header "Accept: application/vnd.github+json" \
    --output "$work/release.json" "$api_url"
  verify_download "$work/asset" "$work/release.json"
fi

mv "$work/asset" "$target"
chmod +x "$target"

if [ -n "${GITHUB_PATH:-}" ]; then
  echo "$dir" >> "$GITHUB_PATH"
fi
export PATH="$dir:$PATH"
"$target" version
