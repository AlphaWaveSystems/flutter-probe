#!/usr/bin/env bash
# Runs the dependency licence check over the Go modules and npm projects of a checkout. Used by the two workflows that check licences
# (dependabot-compat.yml for Dependabot's pull requests, license-check.yml for everyone else's), so that the steps cannot drift apart.
#
#   scripts/licensecheck/check-all.sh REPO_DIR TOOL_DIR TMP_DIR TARGET...
#
#   REPO_DIR  the checkout whose dependencies are checked (the pull request)
#   TOOL_DIR  the checkout whose scripts/licensecheck is run: the same as REPO_DIR, or, for a pull request from outside, the BASE branch, so that a pull
#             request cannot weaken the tool or this script (the disclosure guard works the same way); the workflow file itself comes from the pull request and is
#             caught by review, not by this check. The allowlist is read from REPO_DIR: it can silence notes and the unlicensed-Go-package failure, never a strong
#             copyleft licence.
#   TMP_DIR   where the tools' output goes
#   TARGET    go                  the Go modules: ., tools/probe-convert, studio   (needs go-licenses v1.6.0 in $GOPATH/bin or $GO_LICENSES)
#             npm:<directory>     one npm project, for example npm:website, npm:studio/frontend, npm:vscode   (needs node and npm)
#
# Every target is checked, also after one has failed, so one run shows everything. Exit 0 when all passed, 1 when any failed.
set -uo pipefail

if [ "$#" -lt 4 ]; then
  echo "usage: check-all.sh REPO_DIR TOOL_DIR TMP_DIR go|npm:DIR..." >&2
  exit 2
fi
repo=$(cd "$1" && pwd)
tool=$(cd "$2" && pwd)
tmp=$3
shift 3
mkdir -p "$tmp"
tmp=$(cd "$tmp" && pwd)

licensecheck() { (cd "$tool" && go run ./scripts/licensecheck "$@"); }
failed=0

for target in "$@"; do
  case "$target" in
    go)
      gl=${GO_LICENSES:-$(go env GOPATH)/bin/go-licenses}
      # Studio embeds its built frontend; go-licenses only needs the embed pattern to match
      mkdir -p "$repo/studio/frontend/dist"
      [ -e "$repo/studio/frontend/dist/index.html" ] || echo '<html></html>' > "$repo/studio/frontend/dist/index.html"
      for m in . tools/probe-convert studio; do
        out="$tmp/go-licenses-$(echo "$m" | tr / -).csv"
        echo "=== Go licences: $m ==="
        if ! (cd "$repo/$m" && "$gl" report ./... > "$out" 2> "$tmp/go-licenses.err"); then
          cat "$tmp/go-licenses.err"
          failed=1
          continue
        fi
        licensecheck go "$out" --own github.com/alphawavesystems/ --allow "$repo/scripts/licensecheck/allowlist.txt" || failed=1
      done
      ;;
    npm:*)
      d=${target#npm:}
      out="$tmp/license-checker-$(echo "$d" | tr / -).json"
      echo "=== npm licences: $d ==="
      if ! (cd "$repo/$d" && npm ci --ignore-scripts --no-audit --no-fund && npx --yes license-checker@25.0.1 --excludePrivatePackages --json > "$out"); then
        failed=1
        continue
      fi
      licensecheck npm "$out" --allow "$repo/scripts/licensecheck/allowlist.txt" || failed=1
      ;;
    *)
      echo "check-all.sh: unknown target $target" >&2
      exit 2
      ;;
  esac
done
exit "$failed"
