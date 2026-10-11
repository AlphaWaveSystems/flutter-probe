#!/usr/bin/env bash
# Scan a git range for text that must not be published, using maintainer-defined
# patterns kept OUTSIDE this repository.
#
#   scripts/disclosure-scan.sh <repo-dir> <git-range> <patterns-file> [--extra FILE]...
#
# patterns-file: one extended regex per line; blank lines and lines starting
# with '#' are ignored. Matching is case-insensitive.
#
# What is scanned: lines added in the range, the names of changed files, the
# commit messages in the range, and any --extra text files (for example a pull
# request title and body).
#
# Output never contains the matched text or the pattern, only the location and
# the rule number: CI logs of a public repository are public. Exit status 1 if
# anything matched, 2 on a usage error.
set -euo pipefail

if [ "$#" -lt 3 ]; then
  echo "usage: $0 <repo-dir> <git-range> <patterns-file> [--extra FILE]..." >&2
  exit 2
fi
repo=$1; range=$2; patterns=$3; shift 3
extras=()
while [ "$#" -gt 0 ]; do
  case "$1" in
    --extra) [ "$#" -ge 2 ] || { echo "--extra needs a file" >&2; exit 2; }; extras+=("$2"); shift 2 ;;
    *) echo "unknown argument" >&2; exit 2 ;;
  esac
done
[ -s "$patterns" ] || { echo "patterns file is missing or empty" >&2; exit 2; }

tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT

# Added lines as file:line:text (-U0 hunks, so only additions advance the counter).
git -C "$repo" diff -U0 --no-color --no-ext-diff "$range" | awk '
  /^\+\+\+ b\// { f = substr($0, 7); next }
  /^\+\+\+ /    { f = ""; next }
  /^@@/         { match($0, /\+[0-9]+/); ln = substr($0, RSTART + 1, RLENGTH - 1) + 0; next }
  /^\+/ && f != "" { print f ":" ln ":" substr($0, 2); ln++ }
' > "$tmp/added.txt"
git -C "$repo" diff --name-only --no-ext-diff "$range" > "$tmp/paths.txt"
git -C "$repo" log --format='commit %h:%n%B' "$range" > "$tmp/commits.txt"
: > "$tmp/extra.txt"
for f in "${extras[@]+"${extras[@]}"}"; do cat "$f" >> "$tmp/extra.txt"; echo >> "$tmp/extra.txt"; done

hits=0; rule=0
while IFS= read -r pat || [ -n "$pat" ]; do
  case "$pat" in ''|'#'*) continue ;; esac
  rule=$((rule + 1))
  # An invalid pattern must not silently disable the guard.
  if printf '' | grep -E -- "$pat" >/dev/null 2>&1; [ "$?" -eq 2 ]; then
    echo "rule $rule: invalid pattern" >&2; exit 2
  fi
  while IFS= read -r loc; do echo "rule $rule: added line $loc"; hits=$((hits + 1)); done \
    < <(grep -iE -- "$pat" "$tmp/added.txt" | cut -d: -f1,2 | sort -u || true)
  while IFS= read -r p; do echo "rule $rule: file name $p"; hits=$((hits + 1)); done \
    < <(grep -iE -- "$pat" "$tmp/paths.txt" | sort -u || true)
  if grep -iEq -- "$pat" "$tmp/commits.txt"; then echo "rule $rule: a commit message in the range"; hits=$((hits + 1)); fi
  if grep -iEq -- "$pat" "$tmp/extra.txt"; then echo "rule $rule: the extra text (for example the pull request title or body)"; hits=$((hits + 1)); fi
done < "$patterns"

[ "$rule" -gt 0 ] || { echo "patterns file has no rules" >&2; exit 2; }
if [ "$hits" -gt 0 ]; then
  echo "disclosure scan: $hits match(es) across $rule rule(s). The matched text is not shown on purpose."
  exit 1
fi
echo "disclosure scan: clean ($rule rules)"
