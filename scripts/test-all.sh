#!/usr/bin/env bash
#
# test-all.sh — Run every test suite in the FlutterProbe repository.
#
# Usage:
#   ./scripts/test-all.sh           # run everything
#   ./scripts/test-all.sh --quick   # skip integration tests (no build required)
#
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

# ── Colours (disabled when not a TTY or NO_COLOR is set) ─────────────────────
if [[ -t 1 ]] && [[ -z "${NO_COLOR:-}" ]]; then
  GREEN='\033[32m' RED='\033[31m' YELLOW='\033[33m'
  BOLD='\033[1m' DIM='\033[2m' RESET='\033[0m'
else
  GREEN='' RED='' YELLOW='' BOLD='' DIM='' RESET=''
fi

PASS=0
FAIL=0
SKIP=0
FAILURES=()
QUICK=false
[[ "${1:-}" == "--quick" ]] && QUICK=true

# ── Helpers ──────────────────────────────────────────────────────────────────
section() { printf "\n${BOLD}══ %s${RESET}\n" "$1"; }
pass()    { ((PASS++)); printf "  ${GREEN}✓${RESET} %s\n" "$1"; }
fail()    { ((FAIL++)); FAILURES+=("$1"); printf "  ${RED}✗${RESET} %s\n" "$1"; }
skip()    { ((SKIP++)); printf "  ${YELLOW}–${RESET} %s ${DIM}(skipped)${RESET}\n" "$1"; }

run_test() {
  local label="$1"; shift
  if "$@" > /tmp/probe-test-output.log 2>&1; then
    pass "$label"
  else
    fail "$label"
    # Show last 20 lines on failure for debugging
    printf "    ${DIM}── last 20 lines ──${RESET}\n"
    tail -20 /tmp/probe-test-output.log | sed 's/^/    /'
    printf "    ${DIM}───────────────────${RESET}\n"
  fi
}

# ══════════════════════════════════════════════════════════════════════════════
# 1. BUILD
# ══════════════════════════════════════════════════════════════════════════════
section "Build"

run_test "go mod tidy"              go mod tidy
run_test "build probe CLI"          go build -ldflags="-s -w" -o bin/probe ./cmd/probe
run_test "build probe-convert"      bash -c "cd tools/probe-convert && go build -o ../../bin/probe-convert ."

# ══════════════════════════════════════════════════════════════════════════════
# 2. PROBE CLI — UNIT TESTS
# ══════════════════════════════════════════════════════════════════════════════
section "Probe CLI — Unit Tests"

run_test "internal/parser"          go test -race ./internal/parser/
run_test "internal/runner"          go test -race ./internal/runner/
run_test "internal/ai"              go test -race ./internal/ai/
run_test "internal/migrate"         go test -race ./internal/migrate/

# ══════════════════════════════════════════════════════════════════════════════
# 3. PROBE CLI — LINT
# ══════════════════════════════════════════════════════════════════════════════
section "Probe CLI — Lint"

if [[ -d tests/ ]]; then
  run_test "probe lint tests/"      bin/probe lint tests/
else
  skip "probe lint tests/ (no tests/ directory)"
fi

# ══════════════════════════════════════════════════════════════════════════════
# 4. PROBE-CONVERT — UNIT TESTS (per converter package)
# ══════════════════════════════════════════════════════════════════════════════
section "probe-convert — Unit Tests"

for pkg in maestro gherkin robot detox appium; do
  run_test "convert/$pkg" bash -c "cd tools/probe-convert && go test -race ./convert/$pkg/"
done
run_test "catalog"        bash -c "cd tools/probe-convert && go test -race ./catalog/"

# ══════════════════════════════════════════════════════════════════════════════
# 5. PROBE-CONVERT — INTEGRATION TESTS (golden + lint + verify)
# ══════════════════════════════════════════════════════════════════════════════
section "probe-convert — Integration Tests"

if $QUICK; then
  skip "TestGoldenFiles (--quick)"
  skip "TestLintGeneratedOutput (--quick)"
  skip "TestVerifyDryRun (--quick)"
else
  run_test "TestGoldenFiles" \
    bash -c "cd tools/probe-convert && go test -v -run TestGoldenFiles ."
  run_test "TestLintGeneratedOutput" \
    bash -c "cd tools/probe-convert && go test -v -run TestLintGeneratedOutput ."
  run_test "TestVerifyDryRun" \
    bash -c "cd tools/probe-convert && go test -v -run TestVerifyDryRun ."
fi

# ══════════════════════════════════════════════════════════════════════════════
# 6. PROBE-CONVERT — CLI DRY-RUN (every example file)
# ══════════════════════════════════════════════════════════════════════════════
section "probe-convert — CLI Dry-Run (all 15 examples)"

if $QUICK; then
  skip "CLI dry-run (--quick)"
else
  for dir in maestro gherkin robot detox appium; do
    for f in tools/probe-convert/examples/"$dir"/*; do
      [[ -f "$f" ]] || continue
      name="$(basename "$f")"
      run_test "dry-run $dir/$name" bin/probe-convert --dry-run "$f"
    done
  done
fi

# ══════════════════════════════════════════════════════════════════════════════
# 7. PROBE-CONVERT — CLI LINT + VERIFY (all examples, real file output)
# ══════════════════════════════════════════════════════════════════════════════
section "probe-convert — CLI Lint + Verify (all examples)"

if $QUICK; then
  skip "CLI lint+verify (--quick)"
else
  TMPDIR_OUT="$(mktemp -d)"
  trap "rm -rf '$TMPDIR_OUT'" EXIT

  run_test "convert --lint --verify -r (all 15 examples)" \
    bin/probe-convert --lint --verify -r -o "$TMPDIR_OUT" tools/probe-convert/examples/

  # Also verify the output files pass probe lint directly
  run_test "probe lint on converted output" \
    bin/probe lint "$TMPDIR_OUT"/
fi

# ══════════════════════════════════════════════════════════════════════════════
# 8. PROBE-CONVERT — CLI FLAGS
# ══════════════════════════════════════════════════════════════════════════════
section "probe-convert — CLI Flags"

if $QUICK; then
  skip "CLI flag tests (--quick)"
else
  EX="tools/probe-convert/examples"

  run_test "--from maestro"    bin/probe-convert --dry-run --from maestro "$EX/maestro/login.yaml"
  run_test "--from gherkin"    bin/probe-convert --dry-run --from gherkin "$EX/gherkin/login.feature"
  run_test "--from robot"      bin/probe-convert --dry-run --from robot   "$EX/robot/login.robot"
  run_test "--from detox"      bin/probe-convert --dry-run --from detox   "$EX/detox/login.spec.js"
  run_test "--from appium"     bin/probe-convert --dry-run --from appium  "$EX/appium/test_login.py"
  run_test "--no-color"        bin/probe-convert --dry-run --no-color     "$EX/maestro/login.yaml"
  run_test "-v (verbose)"      bin/probe-convert --dry-run -v             "$EX/maestro/login.yaml"
  run_test "-r (recursive)"    bin/probe-convert --dry-run -r             "$EX/maestro/"
fi

# ══════════════════════════════════════════════════════════════════════════════
# 9. PROBE-CONVERT — CATALOG
# ══════════════════════════════════════════════════════════════════════════════
section "probe-convert — Catalog"

run_test "catalog (summary)"       bin/probe-convert catalog
run_test "catalog maestro"         bin/probe-convert catalog maestro
run_test "catalog gherkin"         bin/probe-convert catalog gherkin
run_test "catalog robot"           bin/probe-convert catalog robot
run_test "catalog detox"           bin/probe-convert catalog detox
run_test "catalog appium_python"   bin/probe-convert catalog appium_python
run_test "catalog appium_java"     bin/probe-convert catalog appium_java
run_test "catalog appium_js"       bin/probe-convert catalog appium_js
run_test "catalog --markdown"      bin/probe-convert catalog --markdown

# aliases
run_test "catalog py (alias)"      bin/probe-convert catalog py
run_test "catalog java (alias)"    bin/probe-convert catalog java
run_test "catalog js (alias)"      bin/probe-convert catalog js
run_test "catalog wdio (alias)"    bin/probe-convert catalog wdio

# ══════════════════════════════════════════════════════════════════════════════
# 10. PROBE-CONVERT — FORMAT HELP
# ══════════════════════════════════════════════════════════════════════════════
section "probe-convert — Format Help"

for fmt in maestro gherkin robot detox appium; do
  run_test "formats $fmt"          bin/probe-convert formats "$fmt"
done

# ══════════════════════════════════════════════════════════════════════════════
# SUMMARY
# ══════════════════════════════════════════════════════════════════════════════
TOTAL=$((PASS + FAIL + SKIP))

printf "\n${BOLD}══ Summary${RESET}\n"
printf "  Total:   %d\n" "$TOTAL"
printf "  ${GREEN}Passed:  %d${RESET}\n" "$PASS"
if [[ $FAIL -gt 0 ]]; then
  printf "  ${RED}Failed:  %d${RESET}\n" "$FAIL"
fi
if [[ $SKIP -gt 0 ]]; then
  printf "  ${YELLOW}Skipped: %d${RESET}\n" "$SKIP"
fi

if [[ $FAIL -gt 0 ]]; then
  printf "\n  ${RED}Failed tests:${RESET}\n"
  for f in "${FAILURES[@]}"; do
    printf "    ${RED}✗${RESET} %s\n" "$f"
  done
  printf "\n"
  exit 1
fi

printf "\n  ${GREEN}All tests passed.${RESET}\n\n"
