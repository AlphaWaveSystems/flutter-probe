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
base="https://github.com/AlphaWaveSystems/flutter-probe/releases"
if [ "$version" = "latest" ]; then
  url="$base/latest/download/$asset"
else
  version="${version#v}"
  url="$base/download/v$version/$asset"
fi

bin_dir="$(mktemp -d)"
echo "Downloading $url"
curl --fail --silent --show-error --location --retry 3 --output "$bin_dir/probe" "$url" || exit 1
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
