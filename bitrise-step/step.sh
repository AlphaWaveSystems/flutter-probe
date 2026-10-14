#!/bin/bash
# FlutterProbe tests: install the CLI from a GitHub release, run `probe test`, fail the step if the tests failed.
set -uo pipefail

version="${probe_version:-latest}"
case "$(uname -s)/$(uname -m)" in
  Darwin/arm64)       asset="probe-darwin-arm64" ;;
  Darwin/x86_64)      asset="probe-darwin-amd64" ;;
  Linux/x86_64)       asset="probe-linux-amd64" ;;
  *) echo "No FlutterProbe CLI release asset for $(uname -s)/$(uname -m). See https://github.com/AlphaWaveSystems/flutter-probe/releases" >&2; exit 1 ;;
esac
base="${PROBE_RELEASES_URL:-https://github.com/AlphaWaveSystems/flutter-probe/releases}"
if [ "$version" = "latest" ]; then
  url="$base/latest/download/$asset"
else
  version="${version#v}"
  url="$base/download/v$version/$asset"
fi

bin_dir="$(mktemp -d)"
work="$(mktemp -d)"
echo "Downloading $url"
curl --fail --silent --show-error --location --retry 3 --output "$work/asset" "$url" || exit 1

# Check the download against the SHA-256 digest that GitHub records for the release asset (the "digest" field of the
# Releases API). Nothing is installed on a mismatch or when the release publishes no digest. skip_verify=yes turns it off.
file_sha256() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | cut -d' ' -f1; else shasum -a 256 "$1" | cut -d' ' -f1; fi
}
expected_digest() {
  local d=""
  if command -v jq >/dev/null 2>&1; then
    d="$(jq -r --arg n "$asset" '[.assets[]? | select(.name == $n) | .digest // empty] | first // empty' "$1" 2>/dev/null || true)"
  elif command -v python3 >/dev/null 2>&1; then
    d="$(python3 - "$1" "$asset" <<'PY' 2>/dev/null || true
import json, sys
for a in json.load(open(sys.argv[1])).get("assets", []):
    if a.get("name") == sys.argv[2] and a.get("digest"):
        print(a["digest"]); break
PY
)"
  fi
  case "$d" in sha256:*) echo "${d#sha256:}" ;; esac
}
if [ "${skip_verify:-no}" = "yes" ]; then
  echo "skip_verify=yes: the download is NOT verified." >&2
else
  api="${PROBE_API_URL:-https://api.github.com/repos/AlphaWaveSystems/flutter-probe/releases}"
  if [ "$version" = "latest" ]; then api_url="$api/latest"; else api_url="$api/tags/v$version"; fi
  curl --fail --silent --show-error --location --retry 3 --header "Accept: application/vnd.github+json" \
    --output "$work/release.json" "$api_url" || exit 1
  want="$(expected_digest "$work/release.json")"
  if [ -z "$want" ]; then
    echo "The release does not publish a SHA-256 digest for $asset, so the download cannot be verified. Set skip_verify to yes to install anyway." >&2
    exit 1
  fi
  got="$(file_sha256 "$work/asset")"
  if [ "$got" != "$want" ]; then
    echo "SHA-256 mismatch for $asset: expected $want, got $got. The download was not installed." >&2
    exit 1
  fi
  echo "Verified $asset (sha256 $got)"
fi

mv "$work/asset" "$bin_dir/probe"
chmod +x "$bin_dir/probe"
export PATH="$bin_dir:$PATH"
probe version

args=(test)
# shellcheck disable=SC2206
tests=(${tests:-tests/})
args+=("${tests[@]}")
[ -n "${device:-}" ] && args+=(--device "$device")
[ -n "${timeout:-}" ] && args+=(--timeout "$timeout")
if [ -n "${junit_path:-}" ]; then
  mkdir -p "$(dirname "$junit_path")"
  args+=(--format junit -o "$junit_path")
fi
[ "${upload_report:-no}" = "yes" ] && args+=(--upload-report)
args+=(-y)
# shellcheck disable=SC2206
[ -n "${extra_args:-}" ] && args+=(${extra_args})

echo "probe ${args[*]}"
probe "${args[@]}"
code=$?
envman add --key PROBE_EXIT_CODE --value "$code"
exit "$code"
