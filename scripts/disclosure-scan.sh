#!/usr/bin/env bash
# Scan a git range for text that must not be published, using maintainer-defined
# patterns kept OUTSIDE this repository.
#
#   scripts/disclosure-scan.sh <repo-dir> <git-range> <patterns-file> [--extra FILE]... [--tree PATH]...
#
# patterns-file: one extended regex per line; blank lines and lines starting with
# '#' are ignored. Matching is case-insensitive. Binary files are searched as text,
# and a NUL byte is removed before a line is matched.
#
# What is scanned:
#   - the lines the range adds to a file, text or binary;
#   - the names of the files the range adds or modifies (a deleted file is not matched);
#   - the commit messages in the range;
#   - any --extra text files (for example a pull request title and body);
#   - for each --tree PATH, the names and the text of every file tracked under PATH in
#     the tree at the end of the range.
# The caller chooses the commit this script runs from and the range it checks. The
# workflow's header says what each event checks.
#
# Output never contains the matched text, a file path, or a pattern, because CI logs
# of a public repository are public. A finding is a rule number and a position: a
# changed file's number, a line number within the added text, or a tree file's number.
# Changed files are numbered in the order of
# `git diff -z --name-only --diff-filter=d --no-renames <range>`. Tree files are
# numbered in byte order: the names of `git ls-tree -r -z --name-only <end> -- <PATH>`,
# sorted with `LC_ALL=C sort -u`, where <end> is the last commit of the range. A tree
# file whose name holds a line break cannot be matched by name and is reported without
# a position. Exit status 1 if anything matched, 2 on a usage error or a failed search.
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

# Every git call reads literal paths with explicit diff prefixes, so a user's git
# configuration (diff.noprefix, diff.mnemonicPrefix, core.quotePath) cannot change
# what is read.
g() { git --literal-pathspecs -C "$repo" "$@"; }
diff_opts=(--no-color --no-ext-diff --no-renames --src-prefix=a/ --dst-prefix=b/)

tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
hits=0; rule=0

set +e; g diff --quiet "${diff_opts[@]}" "$range" >/dev/null 2>&1; st=$?; set -e
[ "$st" -le 1 ] || { echo "unknown revision in the range" >&2; exit 2; }

# The range's last commit: the tree that the tree check reads.
end=${range##*..}
if [ "${#trees[@]}" -gt 0 ]; then
  g rev-parse --verify -q "$end^{commit}" >/dev/null || { echo "unknown revision at the end of the range" >&2; exit 2; }
  g ls-tree -r -z --name-only "$end" -- "${trees[@]}" > "$tmp/tree-raw.z"
  : > "$tmp/tree-names.txt"
  breaks=0
  while IFS= read -r -d '' name; do
    case "$name" in
      *$'\n'*) breaks=$((breaks + 1)) ;;
      *) printf '%s\n' "$name" >> "$tmp/tree-names.txt" ;;
    esac
  done < "$tmp/tree-raw.z"
  if [ "$breaks" -gt 0 ]; then
    echo "$breaks tree file name(s) hold a line break and cannot be checked by name"
    hits=$((hits + breaks))
  fi
  LC_ALL=C sort -u "$tmp/tree-names.txt" > "$tmp/tree-list.txt"
else
  : > "$tmp/tree-list.txt"
fi

# Names of the files the range adds or modifies, in diff order. Deleted files are
# skipped: removing a file adds no text. A name with a line break keeps its number,
# and its line is a placeholder that no pattern can match.
g diff -z --name-only --diff-filter=d "${diff_opts[@]}" "$range" > "$tmp/paths.z"
: > "$tmp/paths.txt"
n=0
while IFS= read -r -d '' p; do
  n=$((n + 1))
  case "$p" in
    *$'\n'*)
      echo "changed file #$n: its name holds a line break and cannot be checked by name"
      hits=$((hits + 1))
      printf '\001\n' >> "$tmp/paths.txt" ;;
    *) printf '%s\n' "$p" >> "$tmp/paths.txt" ;;
  esac
done < "$tmp/paths.z"

# Added lines as changed-file-number:line:text, one file at a time. Each file's diff is
# read with --text, so a binary file yields its lines. The file's number comes from the
# list above, so no name is parsed from a diff header (git quotes some names there).
: > "$tmp/added.txt"
n=0
while IFS= read -r -d '' p; do
  n=$((n + 1))
  g diff -U0 --text "${diff_opts[@]}" "$range" -- "$p" \
    | tr -d '\000' \
    | awk -v k="$n" '
        /^@@/ { match($0, /\+[0-9]+/); ln = substr($0, RSTART + 1, RLENGTH - 1) + 0; inhunk = 1; next }
        inhunk && /^\+/ { print k ":" ln ":" substr($0, 2); ln++ }
      ' >> "$tmp/added.txt"
done < "$tmp/paths.z"

g log --format='commit %h:%n%B' "$range" > "$tmp/commits.txt"

: > "$tmp/extra.txt"
for f in "${extras[@]+"${extras[@]}"}"; do
  # The error names no path: an unreadable file is reported as such, and nothing else.
  { tr -d '\000' < "$f" >> "$tmp/extra.txt"; } 2>/dev/null || { echo "extra file unreadable" >&2; exit 2; }
  echo >> "$tmp/extra.txt"
done

while IFS= read -r pat || [ -n "$pat" ]; do
  case "$pat" in ''|'#'*) continue ;; esac
  rule=$((rule + 1))
  # An invalid pattern must not silently disable the guard. The message names the rule, not the pattern.
  if printf '' | grep -E -- "$pat" >/dev/null 2>&1; [ "$?" -eq 2 ]; then
    echo "rule $rule: invalid pattern" >&2; exit 2
  fi
  while IFS=: read -r i l; do echo "rule $rule: changed file #$i, added line $l"; hits=$((hits + 1)); done \
    < <(grep -a -iE -- "$pat" "$tmp/added.txt" | cut -d: -f1,2 | sort -u || true)
  while IFS= read -r i; do echo "rule $rule: changed file #$i has a matching name"; hits=$((hits + 1)); done \
    < <(grep -a -n -iE -- "$pat" "$tmp/paths.txt" | cut -d: -f1 || true)
  if grep -a -iEq -- "$pat" "$tmp/commits.txt"; then echo "rule $rule: a commit message in the range"; hits=$((hits + 1)); fi
  if grep -a -iEq -- "$pat" "$tmp/extra.txt"; then echo "rule $rule: the extra text (for example the pull request title or body)"; hits=$((hits + 1)); fi
  if [ "${#trees[@]}" -gt 0 ]; then
    while IFS= read -r n; do echo "rule $rule: tree file #$n has a matching name"; hits=$((hits + 1)); done \
      < <(grep -a -n -iE -- "$pat" "$tmp/tree-list.txt" | cut -d: -f1 || true)
    # The text of the tree's files. git grep reads binary files as text, and -z keeps
    # each name unquoted. Each record is "<end>:<path>" and ends with a NUL.
    set +e
    g grep -a -z -l -i -E -e "$pat" "$end" -- "${trees[@]}" > "$tmp/tree-raw-hits.z" 2>/dev/null
    status=$?
    set -e
    [ "$status" -le 1 ] || { echo "tree search failed" >&2; exit 2; }
    : > "$tmp/tree-hits.txt"
    while IFS= read -r -d '' rec; do printf '%s\n' "${rec#*:}" >> "$tmp/tree-hits.txt"; done < "$tmp/tree-raw-hits.z"
    LC_ALL=C sort -u -o "$tmp/tree-hits.txt" "$tmp/tree-hits.txt"
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
