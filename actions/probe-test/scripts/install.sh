#!/usr/bin/env bash
# Installs the FlutterProbe CLI from a GitHub release onto the PATH of the current job.
# Inputs (environment): PROBE_VERSION (a version like 0.25.2, or "latest"), PROBE_OS (Linux, macOS or Windows),
# PROBE_ARCH (X64 or ARM64; the values of runner.os and runner.arch), PROBE_INSTALL_DIR (default: $RUNNER_TEMP/probe-bin).
# PROBE_INSTALL_DRY_RUN=1 only prints the download URL.
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

base="https://github.com/AlphaWaveSystems/flutter-probe/releases"
if [ "$version" = "latest" ]; then
  url="${base}/latest/download/${asset}"
else
  version="${version#v}"
  url="${base}/download/v${version}/${asset}"
fi

if [ "${PROBE_INSTALL_DRY_RUN:-}" = "1" ]; then
  echo "$url"
  exit 0
fi

dir="${PROBE_INSTALL_DIR:-${RUNNER_TEMP:-${TMPDIR:-/tmp}}/probe-bin}"
mkdir -p "$dir"
target="$dir/probe"
case "$asset" in *.exe) target="$dir/probe.exe" ;; esac

echo "Downloading $url"
curl --fail --silent --show-error --location --retry 3 --output "$target" "$url"
chmod +x "$target"

if [ -n "${GITHUB_PATH:-}" ]; then
  echo "$dir" >> "$GITHUB_PATH"
fi
export PATH="$dir:$PATH"
"$target" version
