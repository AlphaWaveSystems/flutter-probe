#!/usr/bin/env bash
# Scan a git range for text that must not be published, using maintainer-defined
# patterns kept OUTSIDE this repository.
#
#   scripts/disclosure-scan.sh <repo-dir> <git-range> <patterns-file> [--extra FILE]... [--tree PATH]...
#
# patterns-file: one extended regex per line; blank lines and lines starting
# with '#' are ignored. Matching is case-insensitive.
#
# What is scanned: lines added in the range, the names of changed files, the
# commit messages in the range, any --extra text files (for example a pull
# request title and body), and, for each --tree PATH, the names and text of every
# file tracked under PATH at the end of the range. The tree check catches a match
# that is already in the tree even when the range does not touch it.
#
# Output never contains the matched text, a file path or a pattern, only a rule
# number and a position, because CI logs of a public repository are public.
# Changed files are numbered in `git diff --name-only <range>` order and tree
# files in sorted `git ls-tree -r --name-only <end> -- <PATH>` order, so the same
# listing with `nl` finds the file. Exit status 1 if anything matched, 2 on a
# usage error or a failed search.
set -euo pipefail

if [ "$#" -lt 3 ]; then
  echo "usage: $0 <repo-dir> <git-range> <patterns-file> [--extra FILE]... [--tree PATH]..." >&2
  exit 2
fi
repo=$1; range=$2; patterns=$3; shift 3
extras=()
trees=()
while [ "$#" -gt 0 ]; do
  case "$1" in
    --extra) [ "$#" -ge 2 ] || { echo "--extra needs a file" >&2; exit 2; }; extras+=("$2"); shift 2 ;;
    --tree) [ "$#" -ge 2 ] || { echo "--tree needs a path" >&2; exit 2; }; trees+=("$2"); shift 2 ;;
    *) echo "unknown argument" >&2; exit 2 ;;
  esac
done
[ -s "$patterns" ] || { echo "patterns file is missing or empty" >&2; exit 2; }

tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT

# Tree files at the end of the range, numbered in sorted order.
end=${range##*...}
: > "$tmp/tree-list.txt"
if [ "${#trees[@]}" -gt 0 ]; then
  git -C "$repo" rev-parse --verify -q "$end^{commit}" >/dev/null || { echo "unknown revision at the end of the range" >&2; exit 2; }
  git -C "$repo" ls-tree -r --name-only "$end" -- "${trees[@]}" | LC_ALL=C sort -u > "$tmp/tree-list.txt"
fi

# Changed file names, numbered in this order.
git -C "$repo" diff --name-only --no-ext-diff "$range" > "$tmp/paths.txt"
# Added lines as changed-file-number:line:text (-U0 hunks, so only additions advance the counter).
git -C "$repo" diff -U0 --no-color --no-ext-diff "$range" | awk -v names="$tmp/paths.txt" '
  BEGIN { n = 0; while ((getline p < names) > 0) idx[p] = ++n }
  /^\+\+\+ b\// { f = substr($0, 7); next }
  /^\+\+\+ /    { f = ""; next }
  /^@@/         { match($0, /\+[0-9]+/); ln = substr($0, RSTART + 1, RLENGTH - 1) + 0; next }
  /^\+/ && f != "" { k = (f in idx) ? idx[f] : 0; print k ":" ln ":" substr($0, 2); ln++ }
' > "$tmp/added.txt"
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
  while IFS=: read -r i l; do echo "rule $rule: changed file #$i, added line $l"; hits=$((hits + 1)); done \
    < <(grep -iE -- "$pat" "$tmp/added.txt" | cut -d: -f1,2 | sort -u || true)
  while IFS= read -r i; do echo "rule $rule: changed file #$i has a matching name"; hits=$((hits + 1)); done \
    < <(grep -n -iE -- "$pat" "$tmp/paths.txt" | cut -d: -f1 || true)
  if grep -iEq -- "$pat" "$tmp/commits.txt"; then echo "rule $rule: a commit message in the range"; hits=$((hits + 1)); fi
  if grep -iEq -- "$pat" "$tmp/extra.txt"; then echo "rule $rule: the extra text (for example the pull request title or body)"; hits=$((hits + 1)); fi
  if [ "${#trees[@]}" -gt 0 ]; then
    while IFS= read -r n; do echo "rule $rule: tree file #$n has a matching name"; hits=$((hits + 1)); done \
      < <(grep -n -iE -- "$pat" "$tmp/tree-list.txt" | cut -d: -f1 || true)
    # git grep exits 1 when nothing matches; 2 or more is an error, and an error must fail the guard.
    set +e
    git -C "$repo" grep -I -l -i -E -e "$pat" "$end" -- "${trees[@]}" > "$tmp/tree-raw.txt" 2>/dev/null
    status=$?
    set -e
    [ "$status" -le 1 ] || { echo "tree search failed" >&2; exit 2; }
    cut -d: -f2- "$tmp/tree-raw.txt" | LC_ALL=C sort -u > "$tmp/tree-hits.txt"
    while IFS= read -r n; do echo "rule $rule: tree file #$n has a match in its text"; hits=$((hits + 1)); done \
      < <(grep -n -x -F -f "$tmp/tree-hits.txt" "$tmp/tree-list.txt" | cut -d: -f1 || true)
  fi
done < "$patterns"

[ "$rule" -gt 0 ] || { echo "patterns file has no rules" >&2; exit 2; }
if [ "$hits" -gt 0 ]; then
  echo "disclosure scan: $hits match(es) across $rule rule(s). The matched text is not shown on purpose."
  exit 1
fi
echo "disclosure scan: clean ($rule rules)"
