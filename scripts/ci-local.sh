#!/usr/bin/env bash
# Run the CI checks locally, for example when hosted CI is unavailable.
#
#   scripts/ci-local.sh                 the five checks a pull request needs
#   scripts/ci-local.sh --all           also the other checks that can run locally
#   scripts/ci-local.sh --job NAME      one check, by id or by check name (repeatable)
#   scripts/ci-local.sh --list          the checks this script knows
#   scripts/ci-local.sh --release-dry-run
#                                       build the release binaries into a temporary
#                                       directory; nothing is published or tagged
#
# Options:
#   --base REF        base of the disclosure scan range REF...HEAD (default origin/main,
#                     as last fetched; this script does not fetch)
#   --pr-text FILE    a text file the disclosure scan also reads, for example the pull
#                     request title or body (repeatable)
#   --allow-skip      a required check that is skipped because a tool is missing ends
#                     the run as PARTIAL instead of FAIL
#   --report FILE     also write the final block to FILE as Markdown
#
# Environment:
#   PROBE_DISCLOSURE_DENYLIST   path of the pattern file for the "Disclosure guard"
#                               check. The hosted check fails when it has no patterns;
#                               here the check reports NOT RUN and does not count as
#                               passed. The file is passed to the scanner by path and
#                               its contents are never printed or copied.
#
# The last line is `ci-local: PASS (5 of 5 required checks) at <sha>` only when all
# five required checks ran and passed on a clean tree. Every other run of the checks
# ends with FAIL or PARTIAL. A base other than origin/main is named after the <sha>.
#
# Exit status of a run of the checks:
#     0  PASS: all five required checks ran and passed on a clean tree
#     1  FAIL: a check failed or was not run
#     2  usage error, or the run could not start (no git repository, `git status`
#        failed, the --report file cannot be written)
#     3  FAIL: a required check was skipped because a tool is missing
#     4  FAIL: nothing failed, but the tree was not the commit (see DIRTY TREE below)
#     5  PARTIAL: nothing failed, but the run does not show that the commit passes the
#        five checks (a --job selection, a skip allowed with --allow-skip, a requested
#        optional check that was skipped because its tool is missing, or GOFLAGS is set)
#   130  FAIL (interrupted)
# --list and --help run nothing and exit 0.
#
# Exit status of --release-dry-run. It runs no checks, never prints PASS, and cannot
# be combined with --all, --job, --allow-skip, --base, --report or --pr-text:
#     0  BUILT: the eight binaries were built from a clean tree
#     1  a build failed
#     2  usage error, or the run could not start
#     4  the binaries were built, but the tree was not the commit
#   130  interrupted
# end of help
#
# Each check runs the commands of the workflow job with the same check name. The
# commands are declared in declare_jobs below, and scripts/ci_local_drift_test.go
# fails when they no longer match the workflow files.
#
# DIRTY TREE. A result is tied to a commit, and the checks read the working tree. So
# the tree is read with `git status` before the first check, and the run is marked
# DIRTY TREE and fails when the tree is not the commit:
#   - a tracked file has a staged or unstaged change;
#   - there is an untracked file that git does not ignore (a tracked file can use it,
#     and a clean clone of the commit would not have it);
#   - a file is hidden from `git status` with --assume-unchanged or --skip-worktree;
#   - a check changed a tracked file (the tree is read again after the last check).
# `git status` is run with --untracked-files=normal and --ignore-submodules=none, so
# the user's git configuration cannot hide an untracked file. Untracked files the run
# itself creates do not count; they are named in a note.
#
# Environment. A step does not read BASH_ENV or ENV, and the exported shell functions
# of the caller are removed before anything runs, so that a function named like a tool
# cannot stand in for it; the final block says how many were removed. A hostile
# environment (a function named like a shell builtin, a changed PATH) is out of scope.
#
# What a run leaves behind: the per-check logs in a temporary directory (its path is
# printed), and what the commands themselves write inside the repository (bin/,
# coverage.out, node_modules and build output, all ignored by git). `make build` runs
# `go mod tidy`, as it does in the workflow. With --all, `npm ci` reinstalls
# node_modules in studio/frontend, vscode and website.
#
# What this script deletes: the linter it installed into its own temporary directory,
# and go.work.sum in the repository root when the file was not there before the run
# and git does not track it. The Go tools write that file during a build of the
# workspace; left behind, it would make the next run a DIRTY TREE.
set -uo pipefail

# Exported shell functions of the caller could stand in for a tool (a function named
# go that returns 0). Remove them before anything runs, and count them for the report.
EXPORTED_FUNCS=0
for fn in $(compgen -A function); do
  unset -f "$fn"
  EXPORTED_FUNCS=$((EXPORTED_FUNCS + 1))
done

ROOT=$(cd "$(dirname "$0")/.." && pwd)

# ---- Job table --------------------------------------------------------------------
# One entry per check. Arrays with the same index, so the script runs on bash 3.2.
JOB_ID=(); JOB_KIND=(); JOB_NAME=(); JOB_TOOLS=(); JOB_PINS=(); JOB_SRC=()
STEP_JOB=(); STEP_DIR=(); STEP_CMD=(); STEP_KIND=()

# job ID required|optional "Check name" "tools it needs" "tool=version the workflow pins" "workflow file:job key"
job() {
  JOB_ID+=("$1"); JOB_KIND+=("$2"); JOB_NAME+=("$3"); JOB_TOOLS+=("$4"); JOB_PINS+=("$5"); JOB_SRC+=("$6")
}
# step ID DIR COMMAND: a `run:` command of the workflow job, word for word. DIR is the
# step's working-directory ("." when it has none).
step() { STEP_JOB+=("$1"); STEP_DIR+=("$2"); STEP_CMD+=("$3"); STEP_KIND+=("workflow"); }
# local_step ID DIR COMMAND: a command that stands in for a workflow step that only
# works on a hosted runner. The drift test lists each of these with its reason.
local_step() { STEP_JOB+=("$1"); STEP_DIR+=("$2"); STEP_CMD+=("$3"); STEP_KIND+=("local"); }

# The commands are data: they are expanded by the shell that runs each step.
# shellcheck disable=SC2016
declare_jobs() {
  # ---- required: .github/workflows/ci.yml ----
  job build required "Build" "go make" "go=1.26.10" "ci.yml:build"
  step build . 'make build'
  step build . 'make build-convert'
  step build . 'test -x bin/probe
test -x bin/probe-convert'
  step build . 'for target in linux/amd64 darwin/amd64 darwin/arm64 windows/amd64; do
  for pkg in ./cmd/probe ./cmd/probe-mcp; do
    echo "building $pkg for $target"
    GOOS="${target%/*}" GOARCH="${target#*/}" go build -o /dev/null "$pkg"
  done
done'

  job unit-tests required "Go unit tests" "go" "go=1.26.10" "ci.yml:unit-tests"
  step unit-tests . 'go test -v -race -coverprofile=coverage.out ./...'

  job convert-tests required "probe-convert tests" "go make" "go=1.26.10" "ci.yml:convert-tests"
  step convert-tests . 'make build'
  step convert-tests . 'make build-convert'
  step convert-tests . 'make test-convert'
  step convert-tests . 'make test-convert-integration'

  # staticcheck is installed into this run's temporary directory (GOBIN), not into
  # the developer's own GOBIN.
  job lint required "Lint" "go" "go=1.26.10" "ci.yml:lint"
  step lint . 'go vet ./...'
  step lint . 'cd tools/probe-convert && go vet ./...'
  step lint . 'go install honnef.co/go/tools/cmd/staticcheck@latest'
  step lint . 'staticcheck ./...'
  step lint . 'cd tools/probe-convert && staticcheck ./...'

  # ---- required: .github/workflows/disclosure-guard.yml ----
  # Not a list of shell commands: see run_disclosure_guard.
  job guard required "Disclosure guard" "git" "" "disclosure-guard.yml:guard"

  # ---- optional (--all) ----
  job dart-agent optional "Dart agent" "flutter dart" "flutter=3.29.x" "ci.yml:dart-agent"
  step dart-agent . 'cd probe_agent && flutter pub get'
  step dart-agent . 'cd probe_agent && dart analyze lib/ --fatal-infos'
  step dart-agent . 'cd probe_agent && flutter test'
  step dart-agent . 'cd probe_agent && dart pub publish --dry-run'
  local_step dart-agent . 'BASE=$CI_LOCAL_BASE_SHA
HEAD=$CI_LOCAL_HEAD_SHA
if [ -z "$BASE" ]; then echo "ERROR: the base commit is unknown"; exit 1; fi
LIB_CHANGED=$(git diff --name-only "$BASE" "$HEAD" -- probe_agent/lib/ | wc -l | tr -d " ")
CHANGELOG_CHANGED=$(git diff --name-only "$BASE" "$HEAD" -- probe_agent/CHANGELOG.md | wc -l | tr -d " ")
if [ "$LIB_CHANGED" -gt 0 ] && [ "$CHANGELOG_CHANGED" -eq 0 ]; then
  echo "ERROR: probe_agent/lib/ changed but probe_agent/CHANGELOG.md was not updated."
  echo "Add a changelog entry under the [Unreleased] section before merging."
  exit 1
fi'

  job frontend optional "Studio frontend" "node npm" "node=26" "studio-ci.yml:frontend"
  step frontend studio/frontend 'npm ci --no-audit --no-fund'
  step frontend studio/frontend 'npm run build'
  step frontend studio/frontend 'npm run typecheck:device-view'
  step frontend studio/frontend 'npm run test:device-view'
  step frontend studio/frontend 'npm run test:probescript'
  step frontend studio/frontend 'npm run check:probescript'

  job studio-go optional "Studio Go tests" "node npm go" "node=26 go=1.26.10" "studio-ci.yml:studio-go"
  step studio-go studio/frontend 'npm ci --no-audit --no-fund
npm run build'
  step studio-go studio 'go test ./...'

  job vscode optional "VS Code extension tests" "node npm" "node=22" "studio-ci.yml:vscode"
  step vscode vscode 'npm ci --no-audit --no-fund'
  step vscode vscode 'npm test'

  # The workflow job has no name and also deploys the site; only its build runs here.
  job website optional "Website build" "node npm" "node=22.23.4" "docs.yml:deploy"
  step website website 'npm ci'
  step website website 'npm run build'
}
declare_jobs

# ---- Helpers ----------------------------------------------------------------------
usage() { sed -n '2,/^# end of help$/p' "$0" | sed -e '$d' -e 's/^# \{0,1\}//'; }
die_usage() { echo "ci-local: $1" >&2; echo "try: scripts/ci-local.sh --help" >&2; exit 2; }

# One line, single spaces: the form the drift test compares.
normalise() { printf '%s' "$1" | tr '\n\t' '  ' | sed -e 's/  */ /g' -e 's/^ //' -e 's/ $//'; }

job_index() { # id or check name -> index, or nothing
  local i
  for i in "${!JOB_ID[@]}"; do
    if [ "${JOB_ID[$i]}" = "$1" ] || [ "${JOB_NAME[$i]}" = "$1" ]; then echo "$i"; return 0; fi
  done
  return 1
}

list_jobs() {
  local i
  for i in "${!JOB_ID[@]}"; do
    printf '%-9s %-14s %s\n' "${JOB_KIND[$i]}" "${JOB_ID[$i]}" "${JOB_NAME[$i]}"
  done
}

# Machine-readable job table for scripts/ci_local_drift_test.go. Tab-separated.
describe_jobs() {
  local i s pin
  for i in "${!JOB_ID[@]}"; do
    printf 'job\t%s\t%s\t%s\t%s\t%s\n' "${JOB_ID[$i]}" "${JOB_KIND[$i]}" "${JOB_NAME[$i]}" "${JOB_SRC[$i]%%:*}" "${JOB_SRC[$i]#*:}"
    for pin in ${JOB_PINS[$i]}; do
      printf 'pin\t%s\t%s\t%s\n' "${JOB_ID[$i]}" "${pin%%=*}" "${pin#*=}"
    done
  done
  for s in "${!STEP_JOB[@]}"; do
    printf '%s\t%s\t%s\t%s\n' "${STEP_KIND[$s]}" "${STEP_JOB[$s]}" "${STEP_DIR[$s]}" "$(normalise "${STEP_CMD[$s]}")"
  done
}

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}'
  else shasum -a 256 "$1" | awk '{print $1}'; fi
}

# version_matches LOCAL PIN: 26.10.0 matches 26; 3.29.4 matches 3.29.x; go1.26.10 matches 1.26.10.
version_matches() {
  local have=${1#v} want=${2%.x}
  have=${have#go}
  [ "$have" = "$want" ] && return 0
  case "$have" in "$want".*) return 0 ;; esac
  return 1
}

# Read once: `flutter --version` is slow.
V_GO=""; V_NODE=""; V_FLUTTER=""
read_tool_versions() {
  if command -v go >/dev/null 2>&1; then V_GO=$(cd "$ROOT" && go env GOVERSION 2>/dev/null); fi
  if command -v node >/dev/null 2>&1; then V_NODE=$(node --version 2>/dev/null); fi
  if command -v flutter >/dev/null 2>&1; then V_FLUTTER=$(flutter --version 2>/dev/null | awk 'NR==1 {print $2}'); fi
}
tool_version() {
  case "$1" in
    go) echo "$V_GO" ;;
    node) echo "$V_NODE" ;;
    flutter) echo "$V_FLUTTER" ;;
    *) echo "" ;;
  esac
}

# ---- Arguments --------------------------------------------------------------------
ALL=0; ALLOW_SKIP=0; RELEASE=0; BASE_REF="origin/main"; BASE_SET=0; REPORT=""
WANTED=(); PR_TEXT=()
while [ "$#" -gt 0 ]; do
  case "$1" in
    --all) ALL=1; shift ;;
    --allow-skip) ALLOW_SKIP=1; shift ;;
    --release-dry-run) RELEASE=1; shift ;;
    --list) list_jobs; exit 0 ;;
    --describe) describe_jobs; exit 0 ;;
    --job) [ "$#" -ge 2 ] || die_usage "--job needs a name"; WANTED+=("$2"); shift 2 ;;
    --base) [ "$#" -ge 2 ] || die_usage "--base needs a ref"
      case $2 in *[[:cntrl:]]*) die_usage "--base holds a control character" ;; esac; BASE_REF=$2; BASE_SET=1; shift 2 ;;
    --pr-text) [ "$#" -ge 2 ] || die_usage "--pr-text needs a file"; PR_TEXT+=("$2"); shift 2 ;;
    --report) [ "$#" -ge 2 ] || die_usage "--report needs a file"; REPORT=$2; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) die_usage "unknown argument: $1" ;;
  esac
done

if [ "$RELEASE" -eq 1 ]; then
  if [ "$ALL" -eq 1 ] || [ "$ALLOW_SKIP" -eq 1 ] || [ "${#WANTED[@]}" -gt 0 ] || [ "$BASE_SET" -eq 1 ] || [ -n "$REPORT" ] || [ "${#PR_TEXT[@]}" -gt 0 ]; then
    die_usage "--release-dry-run runs no checks; it cannot be combined with --all, --job, --allow-skip, --base, --report or --pr-text"
  fi
fi

# A check runs once, even when --job names it twice.
SELECTED=()
if [ "${#WANTED[@]}" -gt 0 ]; then
  picked=" "
  for w in "${WANTED[@]}"; do
    idx=$(job_index "$w") || die_usage "unknown check: $w (see --list)"
    case "$picked" in *" $idx "*) continue ;; esac
    picked="$picked$idx "
    SELECTED+=("$idx")
  done
else
  for i in "${!JOB_ID[@]}"; do
    if [ "${JOB_KIND[$i]}" = "required" ] || [ "$ALL" -eq 1 ]; then SELECTED+=("$i"); fi
  done
fi

# Paths the caller gave are relative to the caller's directory, not to the repository
# root this script changes to.
CALLER_DIR=$PWD
abs_path() { case "$1" in /*) printf '%s\n' "$1" ;; *) printf '%s\n' "$CALLER_DIR/$1" ;; esac; }
for n in ${PR_TEXT[@]+"${!PR_TEXT[@]}"}; do
  PR_TEXT[n]=$(abs_path "${PR_TEXT[$n]}")
  [ -f "${PR_TEXT[$n]}" ] || die_usage "--pr-text file not found: ${PR_TEXT[$n]}"
done
if [ -n "$REPORT" ]; then REPORT=$(abs_path "$REPORT"); fi
DENYLIST=${PROBE_DISCLOSURE_DENYLIST:-}
if [ -n "$DENYLIST" ]; then DENYLIST=$(abs_path "$DENYLIST"); fi

command -v git >/dev/null 2>&1 || { echo "ci-local: git is not installed" >&2; exit 2; }
cd "$ROOT" || exit 2

# A step must not read a start-up file: it could change what the commands do.
unset BASH_ENV ENV

# ---- Run state --------------------------------------------------------------------
HEAD_SHA=$(git rev-parse HEAD 2>/dev/null) || { echo "ci-local: not a git repository" >&2; exit 2; }
BASE_SHA=$(git rev-parse --verify -q "${BASE_REF}^{commit}" 2>/dev/null || true)
export CI_LOCAL_BASE_SHA="$BASE_SHA" CI_LOCAL_HEAD_SHA="$HEAD_SHA"
# The last line names a base that is not the default.
BASE_NOTE=""
if [ "$BASE_REF" != "origin/main" ]; then BASE_NOTE=" (base $BASE_REF)"; fi
SELF="ci-local"
if [ "$RELEASE" -eq 1 ]; then SELF="release-dry-run"; fi

# The tree, read before anything runs. A failing `git status` is not a clean tree.
STATUS_BEFORE=$(git status --porcelain --untracked-files=normal --ignore-submodules=none 2>/dev/null) || { echo "ci-local: git status failed; cannot tell whether the tree is the commit" >&2; exit 2; }
FLAGGED=$(git ls-files -v 2>/dev/null) || { echo "ci-local: git ls-files failed; cannot tell whether the tree is the commit" >&2; exit 2; }
N_CHANGED=$(printf '%s\n' "$STATUS_BEFORE" | grep -c '^[^?]' || true)
N_UNTRACKED=$(printf '%s\n' "$STATUS_BEFORE" | grep -c '^??' || true)
# Lower case: --assume-unchanged. S: --skip-worktree. git status does not show changes to these.
N_HIDDEN=$(printf '%s\n' "$FLAGGED" | LC_ALL=C grep -c '^[a-zS] ' || true)
WORKSUM_BEFORE=""
if [ -e go.work.sum ]; then WORKSUM_BEFORE=yes; fi

# The report file is created now: a path that cannot be written stops the run before
# any check, and an old report cannot be mistaken for the result of this run.
if [ -n "$REPORT" ]; then
  # Not a file the repository tracks, not go.work.sum (which the script may remove),
  # and not a link: the truncation below must not destroy anything.
  report_dir=$(cd "$(dirname "$REPORT")" 2>/dev/null && pwd -P) || report_dir=""
  if [ -L "$REPORT" ]; then
    echo "ci-local: the report file $REPORT is a symbolic link; choose a plain file" >&2; exit 2
  fi
  if [ -n "$report_dir" ]; then
    root_phys=$(pwd -P)
    case "$report_dir/$(basename "$REPORT")" in
      "$root_phys"/*)
        report_rel=${report_dir#"$root_phys"}; report_rel=${report_rel#/}
        report_rel=${report_rel:+$report_rel/}$(basename "$REPORT")
        if [ "$(printf '%s' "$report_rel" | tr '[:upper:]' '[:lower:]')" = "go.work.sum" ] || git ls-files --error-unmatch -- "$report_rel" >/dev/null 2>&1; then
          echo "ci-local: the report file $REPORT is a file git tracks, or go.work.sum; choose another path" >&2; exit 2
        fi ;;
    esac
  fi
  { : > "$REPORT"; } 2>/dev/null || { echo "ci-local: cannot write the report file $REPORT" >&2; exit 2; }
fi

TMP_BASE=${TMPDIR:-/tmp}
LOG_DIR=$(mktemp -d "${TMP_BASE%/}/ci-local.XXXXXX") || { echo "ci-local: cannot create a temporary directory" >&2; exit 2; }
TOOLS_BIN="$LOG_DIR/tools/bin"
mkdir -p "$TOOLS_BIN"

# go.work.sum, when this run's Go commands wrote it: not there before, not tracked.
WORKSUM_REMOVED=""
remove_new_worksum() {
  if [ -z "$WORKSUM_BEFORE" ] && [ -f "$ROOT/go.work.sum" ] && ! git -C "$ROOT" ls-files --error-unmatch go.work.sum >/dev/null 2>&1; then
    rm -f "$ROOT/go.work.sum" && WORKSUM_REMOVED=yes
  fi
}
# The deletions: the tools this run installed into its own temporary directory, and
# the go.work.sum this run wrote.
# shellcheck disable=SC2329  # called by the EXIT trap
cleanup() {
  if [ -n "${LOG_DIR:-}" ] && [ -d "$LOG_DIR/tools" ]; then rm -rf "$LOG_DIR/tools"; fi
  remove_new_worksum
}
trap cleanup EXIT
# An interrupted run is a failed run. While a command of a check runs, bash handles
# the signal when that command ends (Ctrl-C ends it at once).
# shellcheck disable=SC2329  # called by the INT and TERM traps
interrupted() {
  echo
  echo "$SELF: FAIL (interrupted) at $HEAD_SHA$BASE_NOTE"
  if [ -n "$REPORT" ]; then echo "$SELF: FAIL (interrupted) at $HEAD_SHA$BASE_NOTE" > "$REPORT"; fi
  exit 130
}
trap interrupted INT TERM

TREE="clean"
if [ "$N_CHANGED" -gt 0 ] || [ "$N_UNTRACKED" -gt 0 ] || [ "$N_HIDDEN" -gt 0 ]; then
  TREE="DIRTY TREE ($N_CHANGED changed, $N_UNTRACKED untracked, $N_HIDDEN hidden from git status)"
  {
    echo "=================================================================="
    echo "WARNING: DIRTY TREE. The working tree is not commit"
    echo "$HEAD_SHA:"
    echo "  $N_CHANGED tracked file(s) with staged or unstaged changes"
    echo "  $N_UNTRACKED untracked file(s) that git does not ignore"
    echo "  $N_HIDDEN file(s) hidden with --assume-unchanged or --skip-worktree"
    echo "The checks read the working tree, so this result does not describe"
    echo "the commit. See \`git status\` and \`git ls-files -v\`; commit, remove"
    echo "or ignore the files and run again."
    echo "=================================================================="
  } >&2
fi

# Runs one command the way a workflow `run:` step does (bash -e), from DIR.
run_step() { # dir, command, log
  {
    echo
    echo "+ (in $1) $2"
  } >> "$3"
  (
    cd "$ROOT/$1" || exit 1
    CI=true GOBIN="$TOOLS_BIN" PATH="$TOOLS_BIN:$PATH" bash --noprofile --norc -e -c "$2"
  ) >> "$3" 2>&1
}

# The "Disclosure guard" check, as the workflow runs it for a pull request: the scanner
# of the base commit (so a change cannot alter the scanner that checks it) reads the
# range BASE...HEAD, any --pr-text files, and docs/evidence when that scanner knows
# --tree. Sets STATUS and DETAIL.
run_disclosure_guard() { # log
  local log=$1 deny=$DENYLIST scanner="$LOG_DIR/base-scanner/disclosure-scan.sh" args=() f
  if [ -z "$deny" ]; then
    echo "PROBE_DISCLOSURE_DENYLIST is not set. The hosted check fails without its patterns; this check was not run." >> "$log"
    STATUS="NOT RUN"; DETAIL="no denylist"; return
  fi
  if [ ! -s "$deny" ]; then
    echo "The file PROBE_DISCLOSURE_DENYLIST names is missing or empty; this check was not run." >> "$log"
    STATUS="NOT RUN"; DETAIL="denylist file missing or empty"; return
  fi
  if [ -z "$BASE_SHA" ]; then
    echo "unknown base: $BASE_REF" >> "$log"
    STATUS="FAIL"; DETAIL="unknown base $BASE_REF"; return
  fi
  # A base that is HEAD, or that already contains HEAD, leaves nothing to scan. That is
  # also so on the main branch right after a merge: there is no change to check.
  if [ "$BASE_SHA" = "$HEAD_SHA" ] || git merge-base --is-ancestor "$HEAD_SHA" "$BASE_SHA" 2>/dev/null; then
    echo "empty range: $BASE_REF ($BASE_SHA) contains HEAD; nothing was scanned" >> "$log"
    STATUS="NOT RUN"; DETAIL="empty range: $BASE_REF contains HEAD"; return
  fi
  mkdir -p "$LOG_DIR/base-scanner"
  if ! git show "$BASE_SHA:scripts/disclosure-scan.sh" > "$scanner" 2>/dev/null; then
    echo "scripts/disclosure-scan.sh is missing on the base" >> "$log"
    STATUS="FAIL"; DETAIL="no scanner on the base"; return
  fi
  for f in ${PR_TEXT[@]+"${PR_TEXT[@]}"}; do args+=(--extra "$f"); done
  if grep -q -- '--tree' "$scanner"; then args+=(--tree docs/evidence); fi
  {
    echo "scanner: scripts/disclosure-scan.sh at $BASE_SHA"
    echo "range: $BASE_REF...HEAD ($BASE_SHA...$HEAD_SHA)"
    echo "pull request text files: ${#PR_TEXT[@]}"
  } >> "$log"
  GUARD_SCANNED=yes
  # The scanner reports rule numbers and positions only, never the matched text.
  if (cd "$ROOT" && bash "$scanner" . "$BASE_REF...HEAD" "$deny" ${args[@]+"${args[@]}"}) >> "$log" 2>&1; then
    STATUS="PASS"; DETAIL=""
  else
    STATUS="FAIL"; DETAIL=""
  fi
}

# ---- Release dry run --------------------------------------------------------------
# The `build` job of .github/workflows/release.yml: the probe and probe-mcp binaries
# for the four release targets. Nothing is uploaded, no tag is made.
release_dry_run() {
  local out="$LOG_DIR/release" version target goos goarch ext failed=0 f built=0
  command -v go >/dev/null 2>&1 || { echo "ci-local: go is not installed" >&2; return 1; }
  version=$(cat "$ROOT/VERSION" 2>/dev/null || echo "0.0.0")
  mkdir -p "$out"
  echo "Release dry run: version $version (from VERSION), commit $HEAD_SHA, tree: $TREE"
  for target in linux/amd64 darwin/amd64 darwin/arm64 windows/amd64; do
    goos=${target%/*}; goarch=${target#*/}; ext=""
    [ "$goos" = "windows" ] && ext=".exe"
    echo "building $target"
    GOOS=$goos GOARCH=$goarch go build -ldflags "-s -w -X github.com/alphawavesystems/flutter-probe/internal/cli.Version=${version}" -o "$out/probe-${goos}-${goarch}${ext}" ./cmd/probe/ >> "$LOG_DIR/release.log" 2>&1 || failed=1
    GOOS=$goos GOARCH=$goarch go build -ldflags "-s -w -X main.Version=${version}" -o "$out/probe-mcp-${goos}-${goarch}${ext}" ./cmd/probe-mcp/ >> "$LOG_DIR/release.log" 2>&1 || failed=1
  done
  echo
  printf '%10s  %-64s  %s\n' "bytes" "sha256" "file"
  for f in "$out"/*; do
    [ -f "$f" ] || continue
    built=$((built + 1))
    printf '%10s  %s  %s\n' "$(wc -c < "$f" | tr -d ' ')" "$(sha256_of "$f")" "$(basename "$f")"
  done
  echo
  echo "Files: $out"
  echo "Not built here: the Studio apps, the .mcpb bundles and the iOS driver."
  echo "The checksums describe these local files. A hosted release build has other checksums."
  echo "Nothing was published and no tag was created."
  echo "No checks were run: this is not a result for a pull request."
  if [ "$failed" -ne 0 ] || [ "$built" -ne 8 ]; then
    echo "release-dry-run: FAIL ($built of 8 binaries built; see $LOG_DIR/release.log) at $HEAD_SHA"
    return 1
  fi
  if [ "$TREE" != "clean" ]; then
    echo "release-dry-run: FAIL (DIRTY TREE; the files were built from a tree that is not the commit) at $HEAD_SHA"
    return 4
  fi
  echo "release-dry-run: BUILT 8 binaries at $HEAD_SHA (no checks run)"
}

if [ "$RELEASE" -eq 1 ]; then
  release_dry_run
  exit $?
fi

# ---- Run the checks ---------------------------------------------------------------
R_STATUS=(); R_DETAIL=(); R_SECS=()
GUARD_SCANNED=""
echo "ci-local: $HEAD_SHA, ${#SELECTED[@]} check(s), logs in $LOG_DIR"
for i in "${SELECTED[@]}"; do
  id=${JOB_ID[$i]}; name=${JOB_NAME[$i]}; log="$LOG_DIR/$id.log"
  STATUS="PASS"; DETAIL=""
  : > "$log"
  start=$(date +%s)
  for tool in ${JOB_TOOLS[$i]}; do
    if ! command -v "$tool" >/dev/null 2>&1; then
      STATUS="SKIPPED"; DETAIL="tool $tool not installed"; break
    fi
  done
  if [ "$STATUS" = "PASS" ]; then
    echo "RUN      $name"
    if [ "$id" = "guard" ]; then
      run_disclosure_guard "$log"
    else
      for s in "${!STEP_JOB[@]}"; do
        [ "${STEP_JOB[$s]}" = "$id" ] || continue
        if ! run_step "${STEP_DIR[$s]}" "${STEP_CMD[$s]}" "$log"; then
          STATUS="FAIL"; DETAIL="step: $(normalise "${STEP_CMD[$s]}" | cut -c1-60)"; break
        fi
      done
    fi
  fi
  secs=$(( $(date +%s) - start ))
  R_STATUS+=("$STATUS"); R_DETAIL+=("$DETAIL"); R_SECS+=("$secs")
  label=$STATUS; [ -n "$DETAIL" ] && label="$STATUS ($DETAIL)"
  printf '%s  %s  %ss\n' "$label" "$name" "$secs"
  if [ "$STATUS" = "FAIL" ]; then echo "         log: $log"; fi
done

# The linter this run installed, read before the temporary tools are removed.
LINTER="not run in this selection"
if [ -x "$TOOLS_BIN/staticcheck" ]; then LINTER=$("$TOOLS_BIN/staticcheck" -version 2>/dev/null || echo "staticcheck, version unknown"); fi

# What the run itself changed or created, compared with the tree before the first check.
remove_new_worksum
STATUS_AFTER=$(git status --porcelain --untracked-files=normal --ignore-submodules=none 2>/dev/null) || STATUS_AFTER="git status failed"
CHANGED_BY_RUN=0; TRACKED_BY_RUN=0
if [ "$STATUS_AFTER" != "$STATUS_BEFORE" ]; then
  while IFS= read -r status_line; do
    [ -n "$status_line" ] || continue
    case $'\n'"$STATUS_BEFORE"$'\n' in
      *$'\n'"$status_line"$'\n'*) ;;
      *)
        CHANGED_BY_RUN=$((CHANGED_BY_RUN + 1))
        case $status_line in "??"*) ;; *) TRACKED_BY_RUN=$((TRACKED_BY_RUN + 1)) ;; esac ;;
    esac
  done <<< "$STATUS_AFTER"
fi
# A check that changed a tracked file (go mod tidy, a generator) ran on one tree and
# left another: the result is not about the commit.
if [ "$TRACKED_BY_RUN" -gt 0 ] && [ "$TREE" = "clean" ]; then
  TREE="DIRTY TREE (a check changed $TRACKED_BY_RUN tracked file(s))"
fi

# Go settings that change what the commands do. GOFLAGS can make a check pass without
# running anything (-run, -tags, -mod), so a run with GOFLAGS is not a PASS.
GO_FLAGS=${GOFLAGS:-}; GO_TOOLCHAIN=${GOTOOLCHAIN:-}
if command -v go >/dev/null 2>&1; then
  [ -n "$GO_FLAGS" ] || GO_FLAGS=$(go env GOFLAGS 2>/dev/null || true)
  [ -n "$GO_TOOLCHAIN" ] || GO_TOOLCHAIN=$(go env GOTOOLCHAIN 2>/dev/null || true)
fi

# ---- Final block ------------------------------------------------------------------
req_total=0
for i in "${!JOB_ID[@]}"; do [ "${JOB_KIND[$i]}" = "required" ] && req_total=$((req_total + 1)); done
n_pass=0; n_fail=0; n_skip=0; n_notrun=0
req_pass=0; req_skip=0; bad=""; opt_skip=0; opt_skipped=""
for n in "${!SELECTED[@]}"; do
  i=${SELECTED[$n]}; st=${R_STATUS[$n]}
  case "$st" in
    PASS) n_pass=$((n_pass + 1)) ;;
    FAIL) n_fail=$((n_fail + 1)); bad="$bad, ${JOB_NAME[$i]}" ;;
    SKIPPED) n_skip=$((n_skip + 1)) ;;
    "NOT RUN") n_notrun=$((n_notrun + 1)); bad="$bad, ${JOB_NAME[$i]} (not run)" ;;
  esac
  if [ "${JOB_KIND[$i]}" = "required" ]; then
    [ "$st" = "PASS" ] && req_pass=$((req_pass + 1))
    [ "$st" = "SKIPPED" ] && req_skip=$((req_skip + 1))
  elif [ "$st" = "SKIPPED" ]; then
    opt_skip=$((opt_skip + 1)); opt_skipped="$opt_skipped, ${JOB_NAME[$i]}"
  fi
done
bad=${bad#, }

# PASS is only ever printed when every required check ran and passed on a clean tree.
# SELECTED holds each check once, so req_pass counts distinct checks.
req_missing=$((req_total - req_pass - req_skip))
why=""
[ "$req_skip" -gt 0 ] && why="$why; $req_skip skipped with --allow-skip"
[ "$req_missing" -gt 0 ] && why="$why; $req_missing not selected"
[ -n "$GO_FLAGS" ] && why="$why; GOFLAGS is set"
[ "$opt_skip" -gt 0 ] && why="$why; requested optional checks skipped: ${opt_skipped#, }"
if [ $((n_fail + n_notrun)) -gt 0 ]; then
  VERDICT="FAIL ($req_pass of $req_total required checks passed; not passed: $bad)"; CODE=1
elif [ "$req_skip" -gt 0 ] && [ "$ALLOW_SKIP" -eq 0 ]; then
  VERDICT="FAIL ($req_pass of $req_total required checks passed; $req_skip skipped)"; CODE=3
elif [ "$TREE" != "clean" ]; then
  VERDICT="FAIL (DIRTY TREE; $req_pass of $req_total required checks passed on a tree that is not the commit)"; CODE=4
elif [ "$req_pass" -eq "$req_total" ] && [ -z "$GO_FLAGS" ] && [ "$opt_skip" -eq 0 ]; then
  VERDICT="PASS ($req_pass of $req_total required checks)"; CODE=0
else
  VERDICT="PARTIAL ($req_pass of $req_total required checks passed$why)"; CODE=5
fi

final_block() {
  local n i label tool pin have notes="" seen=""
  echo "### ci-local result"
  echo
  echo "- Commit: \`$HEAD_SHA\`"
  echo "- Base: \`$BASE_REF\` at \`${BASE_SHA:-unknown}\`"
  echo "- Tree: $TREE"
  echo "- go: ${V_GO:-not installed} (workflows: 1.26.10)"
  echo "- flutter: ${V_FLUTTER:-not installed} (workflows: 3.29.x)"
  echo "- node: ${V_NODE:-not installed} (workflows: 26 for Studio, 22 for the VS Code extension and the website)"
  echo "- linter: $LINTER (workflows: staticcheck@latest, the newest release on the day of the run)"
  if [ -n "$GO_FLAGS" ]; then echo "- GOFLAGS: \`$GO_FLAGS\` (workflows: not set)"; fi
  if [ -n "$GO_TOOLCHAIN" ]; then echo "- GOTOOLCHAIN: \`$GO_TOOLCHAIN\`"; fi
  echo
  echo "| Result | Check | Required | Time |"
  echo "|---|---|---|---|"
  for n in "${!SELECTED[@]}"; do
    i=${SELECTED[$n]}
    label=${R_STATUS[$n]}; [ -n "${R_DETAIL[$n]}" ] && label="$label (${R_DETAIL[$n]})"
    echo "| $label | ${JOB_NAME[$i]} | $([ "${JOB_KIND[$i]}" = "required" ] && echo yes || echo no) | ${R_SECS[$n]}s |"
    [ "${R_STATUS[$n]}" = "SKIPPED" ] && continue
    for pin in ${JOB_PINS[$i]}; do
      tool=${pin%%=*}
      have=$(tool_version "$tool")
      if [ -n "$have" ] && ! version_matches "$have" "${pin#*=}"; then
        case "$seen" in *"|$tool=${pin#*=}|"*) continue ;; esac
        seen="$seen|$tool=${pin#*=}|"
        notes="$notes- Note: this machine has $tool $have; the workflow uses ${pin#*=} for \"${JOB_NAME[$i]}\".
"
      fi
    done
  done
  echo
  if [ -n "$notes" ]; then printf '%s\n' "$notes"; fi
  if [ "$CHANGED_BY_RUN" -gt 0 ]; then
    echo "- Note: the run itself changed or created $CHANGED_BY_RUN file(s) that git does not ignore (see \`git status\`). They were not there when the checks started."
    echo
  fi
  if [ -n "$WORKSUM_REMOVED" ]; then
    echo "- Note: the Go tools wrote go.work.sum during the run. It was not there before and git does not track it, so it was removed."
    echo
  fi
  if [ "$EXPORTED_FUNCS" -gt 0 ]; then
    echo "- Note: $EXPORTED_FUNCS exported shell function(s) were in the environment and were removed before the checks ran, so that none stands in for a tool."
    echo
  fi
  if [ -n "$GO_FLAGS" ]; then
    echo "- Note: GOFLAGS is set. It changes what the Go commands do, so this run is not a PASS."
    echo
  fi
  if [ -n "$GUARD_SCANNED" ] && [ "${#PR_TEXT[@]}" -eq 0 ]; then
    echo "- Note: no --pr-text file was given, so the pull request title and body were not scanned. The hosted check always scans them."
    echo
  fi
  echo "Counts: $n_pass passed, $n_fail failed, $n_skip skipped, $n_notrun not run (${#SELECTED[@]} selected, $req_total required)"
  echo
  echo "ci-local: $VERDICT at $HEAD_SHA$BASE_NOTE"
}

read_tool_versions
BLOCK=$(final_block)
echo
echo "Logs: $LOG_DIR"
echo
printf '%s\n' "$BLOCK"
if [ -n "$REPORT" ]; then
  printf '%s\n' "$BLOCK" > "$REPORT" || { echo "ci-local: cannot write the report file $REPORT" >&2; exit 2; }
fi
exit "$CODE"
