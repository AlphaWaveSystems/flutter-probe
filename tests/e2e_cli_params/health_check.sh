#!/bin/bash
# ============================================================================
# FlutterProbe CLI E2E Health Check & Self-Test
# ============================================================================
# Auto-detects available devices, runs applicable test phases, and generates
# a standalone HTML report with results.
#
# Usage:
#   ./tests/e2e_cli_params/health_check.sh [phase|all]
#   ./tests/e2e_cli_params/health_check.sh --report-only
#
#   Phases: 1 (offline), 2 (android), 3 (ios), 4 (combos), 5 (edge), 6 (subcmds), all
#
# Prerequisites:
#   - bin/probe built (or will be built automatically)
#   - Android emulator and/or iOS simulator (auto-detected)
# ============================================================================

set -euo pipefail

# ---- Resolve paths ----
REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
PROBE="$REPO_ROOT/bin/probe"
APP_DIR="/Users/patrickbertsch/dev/flutter-projects/Digacel-Flutter"
IOS_CONFIG="probe_ios.yaml"
RESULTS_DIR="/tmp/probe_e2e_results"
REPORT_FILE="$REPO_ROOT/tests/e2e_cli_params/results.html"
SMOKE_TEST="probe_tests/smoke/00_smoke_happy_path.probe"
LOGIN_TEST="probe_tests/login/01_login_happy_path.probe"
FAIL_TEST="probe_tests/fail/09_intentional_fail.probe"
LIFECYCLE_TEST="probe_tests/lifecycle/40_clear_data_shows_login.probe"
METADATA_FILE="$RESULTS_DIR/.metadata"

# ---- Device detection results (populated later) ----
ANDROID_AVAILABLE=false
ANDROID_DEVICE=""
IOS_AVAILABLE=false
IOS_DEVICE=""
IOS_TOKEN_AVAILABLE=false

# ---- Counters ----
TOTAL=0
PASSED=0
FAILED=0
SKIPPED=0
FAILURES=()

# ---- Per-test result log for HTML report ----
# Format: "test_id|description|params|status|exit_code|notes|phase"
TEST_RESULTS=()

# ---- Colors ----
GREEN='\033[32m'
RED='\033[31m'
YELLOW='\033[33m'
CYAN='\033[36m'
BOLD='\033[1m'
DIM='\033[2m'
RESET='\033[0m'

# ---- Parse arguments ----
PHASE="all"
REPORT_ONLY=false

for arg in "$@"; do
  case "$arg" in
    --report-only)
      REPORT_ONLY=true
      ;;
    *)
      PHASE="$arg"
      ;;
  esac
done

mkdir -p "$RESULTS_DIR"

# ============================================================================
# HELPERS
# ============================================================================

run_test() {
  local test_id="$1"
  local description="$2"
  local expect_exit="$3"  # 0 = expect success, 1 = expect failure, any = either
  local params="$4"
  local phase_num="$5"
  shift 5
  local cmd=("$@")

  TOTAL=$((TOTAL + 1))
  printf "${CYAN}[%03d]${RESET} %-65s " "$test_id" "$description"

  local output_file="$RESULTS_DIR/test_${test_id}.log"
  local start_time
  start_time=$(date +%s)

  set +e
  "${cmd[@]}" > "$output_file" 2>&1
  local actual_exit=$?
  set -e

  local end_time
  end_time=$(date +%s)
  local duration=$((end_time - start_time))

  local verdict=""
  local notes=""
  if [ "$expect_exit" = "any" ]; then
    verdict="PASS"
    notes="exit=$actual_exit (any accepted), ${duration}s"
  elif [ "$expect_exit" -eq 0 ] && [ "$actual_exit" -eq 0 ]; then
    verdict="PASS"
    notes="${duration}s"
  elif [ "$expect_exit" -eq 1 ] && [ "$actual_exit" -ne 0 ]; then
    verdict="PASS"
    notes="Expected error, exit=$actual_exit, ${duration}s"
  else
    verdict="FAIL"
    notes="expected exit=$expect_exit, got=$actual_exit, ${duration}s"
  fi

  if [ "$verdict" = "PASS" ]; then
    PASSED=$((PASSED + 1))
    printf "${GREEN}PASS${RESET} (exit=%d, %ds)\n" "$actual_exit" "$duration"
  else
    FAILED=$((FAILED + 1))
    printf "${RED}FAIL${RESET} (expected exit=%s, got=%d, %ds)\n" "$expect_exit" "$actual_exit" "$duration"
    FAILURES+=("[$test_id] $description (expected=$expect_exit, got=$actual_exit)")
  fi

  # Store result for HTML report
  TEST_RESULTS+=("${test_id}|${description}|${params}|${verdict}|${actual_exit}|${notes}|${phase_num}")
}

skip_test() {
  local test_id="$1"
  local description="$2"
  local reason="$3"
  local params="${4:-}"
  local phase_num="${5:-0}"

  TOTAL=$((TOTAL + 1))
  SKIPPED=$((SKIPPED + 1))
  printf "${CYAN}[%03d]${RESET} %-65s ${YELLOW}SKIP${RESET} (%s)\n" "$test_id" "$description" "$reason"

  TEST_RESULTS+=("${test_id}|${description}|${params}|SKIP|-|${reason}|${phase_num}")
}

check_file() {
  local file="$1"
  local label="$2"
  if [ -f "$file" ] && [ -s "$file" ]; then
    printf "      -> %s: ${GREEN}exists${RESET} (%s bytes)\n" "$label" "$(wc -c < "$file" | tr -d ' ')"
  else
    printf "      -> %s: ${RED}missing or empty${RESET}\n" "$label"
  fi
  return 0
}

check_output_contains() {
  local test_id="$1"
  local pattern="$2"
  local label="$3"
  local log="$RESULTS_DIR/test_${test_id}.log"
  if grep -q "$pattern" "$log" 2>/dev/null; then
    printf "      -> %s: ${GREEN}found${RESET}\n" "$label"
  else
    printf "      -> %s: ${RED}not found${RESET} (pattern: %s)\n" "$label" "$pattern"
  fi
  return 0
}

print_header() {
  echo ""
  echo "------------------------------------------------------------------------"
  printf "  ${BOLD}%s${RESET}\n" "$1"
  echo "------------------------------------------------------------------------"
}

print_summary() {
  echo ""
  echo "========================================================================"
  printf "  ${BOLD}RESULTS${RESET}: "
  printf "${GREEN}%d passed${RESET}, " "$PASSED"
  if [ "$FAILED" -gt 0 ]; then
    printf "${RED}%d failed${RESET}, " "$FAILED"
  else
    printf "0 failed, "
  fi
  printf "${YELLOW}%d skipped${RESET} / %d total\n" "$SKIPPED" "$TOTAL"
  echo "========================================================================"

  if [ ${#FAILURES[@]} -gt 0 ]; then
    echo ""
    printf "  ${RED}${BOLD}Failures:${RESET}\n"
    for f in "${FAILURES[@]}"; do
      printf "    ${RED}x${RESET} %s\n" "$f"
    done
  fi
  echo ""
  echo "  Logs:   $RESULTS_DIR/"
  echo "  Report: $REPORT_FILE"
  echo ""
}

# ============================================================================
# HTML REPORT GENERATION
# ============================================================================

generate_html_report() {
  local timestamp
  timestamp="$(date '+%Y-%m-%d %H:%M:%S')"
  local os_version
  os_version="$(uname -rs)"
  local probe_version="unknown"
  if [ -f "$PROBE" ]; then
    probe_version="$("$PROBE" version 2>/dev/null | head -1 || echo "unknown")"
  fi

  # Build device info string
  local device_info=""
  if [ "$ANDROID_AVAILABLE" = true ]; then
    device_info="Android: $ANDROID_DEVICE"
  else
    device_info="Android: not detected"
  fi
  if [ "$IOS_AVAILABLE" = true ]; then
    device_info="$device_info / iOS Simulator: ${IOS_DEVICE:0:8}..."
    if [ "$IOS_TOKEN_AVAILABLE" = true ]; then
      device_info="$device_info (token available)"
    else
      device_info="$device_info (no token)"
    fi
  else
    device_info="$device_info / iOS: not detected"
  fi

  # Phase name lookup (bash 3.2 compatible, no associative arrays)
  get_phase_name() {
    case "$1" in
      1) echo "Phase 1: Offline Tests (no device needed)" ;;
      2) echo "Phase 2: Android Emulator &mdash; Individual Parameters" ;;
      3) echo "Phase 3: iOS Simulator" ;;
      4) echo "Phase 4: Meaningful Parameter Combinations" ;;
      5) echo "Phase 5: Edge Cases and Error Conditions" ;;
      6) echo "Phase 6: Subcommand Coverage" ;;
      *) echo "Phase $1" ;;
    esac
  }

  # Start HTML
  cat > "$REPORT_FILE" <<'HTMLHEAD'
<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>FlutterProbe CLI E2E Parameter Test Results</title>
<style>
  :root {
    --bg: #0d1117;
    --surface: #161b22;
    --border: #30363d;
    --text: #e6edf3;
    --text-muted: #8b949e;
    --green: #3fb950;
    --red: #f85149;
    --yellow: #d29922;
    --blue: #58a6ff;
    --cyan: #39d2c0;
  }
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body {
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Helvetica, Arial, sans-serif;
    background: var(--bg);
    color: var(--text);
    padding: 2rem;
    line-height: 1.5;
  }
  h1 {
    font-size: 1.75rem;
    margin-bottom: 0.5rem;
    color: var(--text);
  }
  .subtitle {
    color: var(--text-muted);
    margin-bottom: 2rem;
    font-size: 0.95rem;
  }
  .device-info {
    color: var(--text-muted);
    margin-bottom: 1.5rem;
    font-size: 0.85rem;
    padding: 0.75rem 1rem;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: 8px;
  }
  .device-info strong { color: var(--text); }
  .summary-bar {
    display: flex;
    gap: 1.5rem;
    margin-bottom: 2rem;
    flex-wrap: wrap;
  }
  .summary-card {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 1rem 1.5rem;
    min-width: 140px;
  }
  .summary-card .label {
    font-size: 0.8rem;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }
  .summary-card .value {
    font-size: 2rem;
    font-weight: 700;
  }
  .summary-card .value.pass { color: var(--green); }
  .summary-card .value.fail { color: var(--red); }
  .summary-card .value.skip { color: var(--yellow); }
  .summary-card .value.total { color: var(--blue); }

  .phase {
    margin-bottom: 2rem;
  }
  .phase-header {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    padding: 0.75rem 1rem;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: 8px 8px 0 0;
    cursor: pointer;
  }
  .phase-header h2 {
    font-size: 1.1rem;
    flex: 1;
  }
  .phase-badge {
    font-size: 0.75rem;
    padding: 0.2rem 0.6rem;
    border-radius: 12px;
    font-weight: 600;
  }
  .phase-badge.all-pass { background: rgba(63,185,80,0.15); color: var(--green); }
  .phase-badge.has-fail { background: rgba(248,81,73,0.15); color: var(--red); }
  .phase-badge.has-skip { background: rgba(210,153,34,0.15); color: var(--yellow); }

  table {
    width: 100%;
    border-collapse: collapse;
    font-size: 0.9rem;
    background: var(--surface);
    border: 1px solid var(--border);
    border-top: none;
    border-radius: 0 0 8px 8px;
    overflow: hidden;
  }
  th {
    text-align: left;
    padding: 0.6rem 1rem;
    background: rgba(110,118,129,0.1);
    color: var(--text-muted);
    font-weight: 600;
    font-size: 0.8rem;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    border-bottom: 1px solid var(--border);
  }
  td {
    padding: 0.5rem 1rem;
    border-bottom: 1px solid var(--border);
    vertical-align: top;
  }
  tr:last-child td { border-bottom: none; }
  tr:hover td { background: rgba(110,118,129,0.05); }

  .id { color: var(--cyan); font-weight: 600; font-variant-numeric: tabular-nums; }
  .param { font-family: 'SF Mono', SFMono-Regular, Consolas, monospace; font-size: 0.82rem; color: var(--blue); }
  .status {
    display: inline-block;
    padding: 0.15rem 0.5rem;
    border-radius: 4px;
    font-weight: 600;
    font-size: 0.8rem;
  }
  .status.pass { background: rgba(63,185,80,0.15); color: var(--green); }
  .status.fail { background: rgba(248,81,73,0.15); color: var(--red); }
  .status.skip { background: rgba(210,153,34,0.15); color: var(--yellow); }
  .note { color: var(--text-muted); font-size: 0.82rem; }

  .footer {
    margin-top: 2rem;
    padding-top: 1rem;
    border-top: 1px solid var(--border);
    color: var(--text-muted);
    font-size: 0.8rem;
    display: flex;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: 0.5rem;
  }
</style>
</head>
<body>
HTMLHEAD

  # Title and subtitle
  cat >> "$REPORT_FILE" <<EOF

<h1>FlutterProbe CLI E2E Parameter Test Results</h1>
<p class="subtitle">Comprehensive test of every CLI parameter and meaningful combinations &mdash; ${timestamp}</p>

<div class="device-info">
  <strong>Devices:</strong> ${device_info}
</div>

<div class="summary-bar">
  <div class="summary-card"><div class="label">Total</div><div class="value total">${TOTAL}</div></div>
  <div class="summary-card"><div class="label">Passed</div><div class="value pass">${PASSED}</div></div>
  <div class="summary-card"><div class="label">Failed</div><div class="value fail">${FAILED}</div></div>
  <div class="summary-card"><div class="label">Skipped</div><div class="value skip">${SKIPPED}</div></div>
</div>
EOF

  # Generate phase-by-phase tables
  local current_phase=""
  local phase_pass=0
  local phase_fail=0
  local phase_skip=0
  local phase_total=0
  local phase_rows=""

  close_phase() {
    if [ -n "$current_phase" ]; then
      # Determine badge
      local badge_class="all-pass"
      local badge_text="${phase_pass}/${phase_total} PASS"
      if [ "$phase_fail" -gt 0 ]; then
        badge_class="has-fail"
        badge_text="${phase_pass}/${phase_total} PASS, ${phase_fail} fail"
      fi
      if [ "$phase_skip" -gt 0 ]; then
        if [ "$phase_fail" -gt 0 ]; then
          badge_text="${badge_text}, ${phase_skip} skip"
        else
          badge_class="has-skip"
          badge_text="${badge_text}, ${phase_skip} skipped"
        fi
      fi

      local phase_name
      phase_name="$(get_phase_name "$current_phase")"
      cat >> "$REPORT_FILE" <<EOF

<div class="phase">
  <div class="phase-header">
    <h2>${phase_name}</h2>
    <span class="phase-badge ${badge_class}">${badge_text}</span>
  </div>
  <table>
    <tr><th width="50">#</th><th>Test</th><th width="250">Parameters Tested</th><th width="70">Status</th><th width="40">Exit</th><th>Notes</th></tr>
${phase_rows}
  </table>
</div>
EOF
    fi
  }

  for result in "${TEST_RESULTS[@]}"; do
    IFS='|' read -r t_id t_desc t_params t_status t_exit t_notes t_phase <<< "$result"

    if [ "$t_phase" != "$current_phase" ]; then
      close_phase
      current_phase="$t_phase"
      phase_pass=0
      phase_fail=0
      phase_skip=0
      phase_total=0
      phase_rows=""
    fi

    phase_total=$((phase_total + 1))

    local status_class="pass"
    local exit_display="$t_exit"
    case "$t_status" in
      PASS)
        status_class="pass"
        phase_pass=$((phase_pass + 1))
        ;;
      FAIL)
        status_class="fail"
        phase_fail=$((phase_fail + 1))
        ;;
      SKIP)
        status_class="skip"
        phase_skip=$((phase_skip + 1))
        exit_display="-"
        ;;
    esac

    # Escape HTML in description and notes
    local safe_desc
    safe_desc="$(echo "$t_desc" | sed 's/&/\&amp;/g; s/</\&lt;/g; s/>/\&gt;/g')"
    local safe_params
    safe_params="$(echo "$t_params" | sed 's/&/\&amp;/g; s/</\&lt;/g; s/>/\&gt;/g')"
    local safe_notes
    safe_notes="$(echo "$t_notes" | sed 's/&/\&amp;/g; s/</\&lt;/g; s/>/\&gt;/g')"

    phase_rows="${phase_rows}    <tr><td class=\"id\">$(printf '%03d' "$t_id")</td><td>${safe_desc}</td><td class=\"param\">${safe_params}</td><td><span class=\"status ${status_class}\">${t_status}</span></td><td>${exit_display}</td><td class=\"note\">${safe_notes}</td></tr>
"
  done
  # Close last phase
  close_phase

  # Footer
  cat >> "$REPORT_FILE" <<EOF

<div class="footer">
  <span>${probe_version} &mdash; CLI E2E Parameter Coverage</span>
  <span>Generated ${timestamp} &mdash; ${os_version} / ${device_info}</span>
</div>

</body>
</html>
EOF

  echo ""
  printf "  ${GREEN}HTML report generated:${RESET} %s\n" "$REPORT_FILE"
}

# ============================================================================
# DEVICE DETECTION PHASE
# ============================================================================

detect_devices() {
  print_header "Device Detection"

  # Check probe binary
  printf "  %-40s " "bin/probe binary"
  if [ -f "$PROBE" ] && [ -x "$PROBE" ]; then
    printf "${GREEN}found${RESET}\n"
  else
    printf "${YELLOW}not found${RESET} - building...\n"
    (cd "$REPO_ROOT" && make build)
    if [ -f "$PROBE" ] && [ -x "$PROBE" ]; then
      printf "  %-40s ${GREEN}build successful${RESET}\n" ""
    else
      printf "  %-40s ${RED}build failed${RESET}\n" ""
      echo ""
      echo "  ERROR: Could not build bin/probe. Run 'make build' manually."
      exit 1
    fi
  fi

  # Check Android emulator
  printf "  %-40s " "Android emulator (adb)"
  if command -v adb &>/dev/null; then
    local adb_output
    adb_output="$(adb devices 2>/dev/null || true)"
    local emu_line
    emu_line="$(echo "$adb_output" | grep 'emulator-' | grep 'device$' | head -1 || true)"
    if [ -n "$emu_line" ]; then
      ANDROID_DEVICE="$(echo "$emu_line" | awk '{print $1}')"
      ANDROID_AVAILABLE=true
      printf "${GREEN}%s${RESET}\n" "$ANDROID_DEVICE"
    else
      printf "${YELLOW}no emulator running${RESET}\n"
    fi
  else
    printf "${YELLOW}adb not found${RESET}\n"
  fi

  # Check iOS simulator
  printf "  %-40s " "iOS simulator (xcrun simctl)"
  if command -v xcrun &>/dev/null; then
    local sim_output
    sim_output="$(xcrun simctl list devices booted 2>/dev/null || true)"
    local sim_line
    sim_line="$(echo "$sim_output" | grep 'Booted' | head -1 || true)"
    if [ -n "$sim_line" ]; then
      # Extract UDID from parenthesized string
      IOS_DEVICE="$(echo "$sim_line" | sed -n 's/.*(\([A-F0-9-]*\)).*/\1/p')"
      if [ -n "$IOS_DEVICE" ]; then
        IOS_AVAILABLE=true
        printf "${GREEN}%s${RESET}\n" "$IOS_DEVICE"

        # Check for token
        local token_file="$HOME/Library/Developer/CoreSimulator/Devices/$IOS_DEVICE/data/tmp/probe/token"
        printf "  %-40s " "iOS agent token"
        if [ -f "$token_file" ]; then
          IOS_TOKEN_AVAILABLE=true
          printf "${GREEN}available${RESET}\n"
        else
          printf "${YELLOW}not available${RESET} (app may not be running)\n"
        fi
      else
        printf "${YELLOW}could not parse UDID${RESET}\n"
      fi
    else
      printf "${YELLOW}no simulator booted${RESET}\n"
    fi
  else
    printf "${YELLOW}xcrun not found${RESET}\n"
  fi

  # Print status table
  echo ""
  echo "  +-------------------+------------+"
  echo "  | Device            | Status     |"
  echo "  +-------------------+------------+"
  if [ "$ANDROID_AVAILABLE" = true ]; then
    printf "  | Android emulator  | ${GREEN}%-10s${RESET} |\n" "Ready"
  else
    printf "  | Android emulator  | ${RED}%-10s${RESET} |\n" "Missing"
  fi
  if [ "$IOS_AVAILABLE" = true ] && [ "$IOS_TOKEN_AVAILABLE" = true ]; then
    printf "  | iOS simulator     | ${GREEN}%-10s${RESET} |\n" "Ready"
  elif [ "$IOS_AVAILABLE" = true ]; then
    printf "  | iOS simulator     | ${YELLOW}%-10s${RESET} |\n" "No token"
  else
    printf "  | iOS simulator     | ${RED}%-10s${RESET} |\n" "Missing"
  fi
  echo "  +-------------------+------------+"

  # Check if we have anything to work with
  if [ "$ANDROID_AVAILABLE" = false ] && [ "$IOS_AVAILABLE" = false ]; then
    # Phase 1, 5, 6 can still run (offline). Only exit if user wants device phases.
    if [ "$PHASE" = "2" ] || [ "$PHASE" = "3" ] || [ "$PHASE" = "4" ]; then
      echo ""
      echo "  ERROR: No devices available for the requested phase."
      echo ""
      echo "  To start an Android emulator:"
      echo "    emulator -avd <avd_name>"
      echo "    # or via Android Studio > Device Manager"
      echo ""
      echo "  To boot an iOS simulator:"
      echo "    xcrun simctl boot <UDID>"
      echo "    # or open Simulator.app and select a device"
      echo ""
      exit 1
    fi
  fi

  # Save metadata for --report-only
  cat > "$METADATA_FILE" <<EOF
ANDROID_AVAILABLE=$ANDROID_AVAILABLE
ANDROID_DEVICE=$ANDROID_DEVICE
IOS_AVAILABLE=$IOS_AVAILABLE
IOS_DEVICE=$IOS_DEVICE
IOS_TOKEN_AVAILABLE=$IOS_TOKEN_AVAILABLE
EOF
}

# ============================================================================
# REPORT-ONLY MODE
# ============================================================================

if [ "$REPORT_ONLY" = true ]; then
  echo "Report-only mode: generating HTML from existing metadata..."
  if [ -f "$METADATA_FILE" ]; then
    # shellcheck disable=SC1090
    source "$METADATA_FILE"
  fi
  # No test results to write - just inform
  echo "No test data to generate report from (run tests first without --report-only)."
  exit 0
fi

# ============================================================================
# MAIN EXECUTION
# ============================================================================

detect_devices

cd "$APP_DIR"

# ============================================================================
# PHASE 1: Offline / Fast Tests (no device needed)
# ============================================================================
if [ "$PHASE" = "1" ] || [ "$PHASE" = "all" ]; then
  print_header "PHASE 1: Offline Tests (no device needed)"

  run_test 1 "probe version" 0 \
    "version" 1 \
    "$PROBE" version

  run_test 2 "probe lint probe_tests/login/" 0 \
    "lint <dir>" 1 \
    "$PROBE" lint probe_tests/login/

  run_test 3 "probe lint single file" 0 \
    "lint <file>" 1 \
    "$PROBE" lint "$SMOKE_TEST"

  run_test 4 "probe lint all test dirs" 0 \
    "lint <dir>" 1 \
    "$PROBE" lint probe_tests/

  run_test 5 "probe lint nonexistent path (expect error)" 1 \
    "lint <bad-path>" 1 \
    "$PROBE" lint nonexistent/path/

  run_test 6 "probe test --dry-run single smoke file" 0 \
    "--dry-run" 1 \
    "$PROBE" test "$SMOKE_TEST" --dry-run

  run_test 7 "probe test --dry-run entire test suite" 0 \
    "--dry-run <dir>" 1 \
    "$PROBE" test probe_tests/ --dry-run

  run_test 8 "probe test --dry-run --tag smoke" 0 \
    "--dry-run -t smoke" 1 \
    "$PROBE" test probe_tests/ --dry-run -t smoke
  check_output_contains 8 "3 passed" "3 smoke tests found"

  run_test 9 "probe test --dry-run --tag labels" 0 \
    "--dry-run -t labels" 1 \
    "$PROBE" test probe_tests/ --dry-run -t labels
  check_output_contains 9 "4 passed" "4 labels tests found"

  run_test 10 "probe test --dry-run --tag regression" 0 \
    "--dry-run -t regression" 1 \
    "$PROBE" test probe_tests/ --dry-run -t regression
  check_output_contains 10 "9 passed" "9 regression tests found"

  run_test 11 "probe test --dry-run --tag nonexistent (0 tests)" 0 \
    "--dry-run -t <unknown>" 1 \
    "$PROBE" test probe_tests/ --dry-run -t nonexistent_tag
  check_output_contains 11 "0 passed" "0 tests matched"

  run_test 12 "probe test --dry-run --format json" 0 \
    "--dry-run --format json" 1 \
    "$PROBE" test "$SMOKE_TEST" --dry-run --format json

  run_test 13 "probe test --dry-run --format junit" 0 \
    "--dry-run --format junit" 1 \
    "$PROBE" test "$SMOKE_TEST" --dry-run --format junit

  run_test 14 "probe device list" 0 \
    "device list" 1 \
    "$PROBE" device list
  if [ "$ANDROID_AVAILABLE" = true ]; then
    check_output_contains 14 "$ANDROID_DEVICE" "Android emulator listed"
  fi

  run_test 15 "probe report --input nonexistent (expect error)" 1 \
    "report --input <bad>" 1 \
    "$PROBE" report --input /tmp/nonexistent_probe_results.json

  run_test 16 "probe test --dry-run from /tmp (no tests found)" any \
    "--dry-run (no tests)" 1 \
    bash -c "cd /tmp && $PROBE test --dry-run"

  run_test 17 "probe --help" 0 \
    "--help" 1 \
    "$PROBE" --help
  check_output_contains 17 "Available Commands" "help text shows commands"

  run_test 18 "probe test --help" 0 \
    "test --help" 1 \
    "$PROBE" test --help
  check_output_contains 18 "\-\-tag" "help shows --tag flag"
  check_output_contains 18 "\-\-video" "help shows --video flag"
  check_output_contains 18 "\-\-format" "help shows --format flag"

  run_test 19 "probe test --dry-run ignores --device" 0 \
    "--dry-run --device <x>" 1 \
    "$PROBE" test "$SMOKE_TEST" --dry-run --device nonexistent-device

  run_test 20 "probe test --dry-run --format json -o file" 0 \
    "--dry-run --format json -o" 1 \
    "$PROBE" test "$SMOKE_TEST" --dry-run --format json -o "$RESULTS_DIR/dryrun_output.json"
  check_file "$RESULTS_DIR/dryrun_output.json" "JSON output file"
fi

# ============================================================================
# PHASE 2: Android Emulator -- Individual Parameters
# ============================================================================
if [ "$PHASE" = "2" ] || [ "$PHASE" = "all" ]; then
  print_header "PHASE 2: Android Emulator -- Individual Parameters"

  if [ "$ANDROID_AVAILABLE" = false ]; then
    echo "  No Android emulator detected. Skipping phase 2."
    for tid in 21 22 23 24 25 26 27 28 29 30 31 32 33 34 35 36 37 38 39 40 41 42 43 44 45 46 47 48 49 50 51; do
      skip_test "$tid" "Android test $tid" "no emulator" "--device (Android)" 2
    done
  else
    run_test 21 "Android baseline: smoke test (auto-detect device)" 0 \
      "-y -v (auto-detect)" 2 \
      "$PROBE" test "$SMOKE_TEST" -y -v

    run_test 22 "Android --device emulator explicit" 0 \
      "--device $ANDROID_DEVICE" 2 \
      "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" -y -v

    run_test 23 "--device invalid;serial (expect rejection)" 1 \
      "--device <injection>" 2 \
      "$PROBE" test "$SMOKE_TEST" --device "invalid;serial" -y

    run_test 24 "--device nonexistent-device (expect timeout)" 1 \
      "--device <bad> --token-timeout 3s" 2 \
      "$PROBE" test "$SMOKE_TEST" --device "nonexistent-1234" --token-timeout 3s -y

    run_test 25 "--port 48687 wrong port (expect fail)" 1 \
      "--port 48687 --dial-timeout 3s" 2 \
      "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" --port 48687 --dial-timeout 3s -y

    run_test 26 "--dial-timeout 2s with wrong port (fast fail)" 1 \
      "--dial-timeout 2s --port 48687" 2 \
      "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" --port 48687 --dial-timeout 2s -y

    run_test 27 "--token-timeout 5s (fast token)" 0 \
      "--token-timeout 5s" 2 \
      "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" --token-timeout 5s -y -v

    run_test 28 "--tag smoke (3 tests)" 0 \
      "-t smoke" 2 \
      "$PROBE" test probe_tests/ --device "$ANDROID_DEVICE" -t smoke -y -v

    run_test 29 "--format json -o results.json" 0 \
      "--format json -o <file>" 2 \
      "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" --format json -o "$RESULTS_DIR/android_json.json" -y -v
    check_file "$RESULTS_DIR/android_json.json" "JSON results file"

    run_test 30 "probe report from JSON -> HTML" 0 \
      "report --input -o" 2 \
      "$PROBE" report --input "$RESULTS_DIR/android_json.json" -o "$RESULTS_DIR/android_report.html"
    check_file "$RESULTS_DIR/android_report.html" "HTML report file"

    run_test 31 "--format junit -o results.xml" 0 \
      "--format junit -o <file>" 2 \
      "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" --format junit -o "$RESULTS_DIR/android_junit.xml" -y -v
    check_file "$RESULTS_DIR/android_junit.xml" "JUnit XML file"
    check_output_contains 31 "testsuites\|testsuite\|testcase" "JUnit XML structure"

    run_test 32 "--format terminal (explicit)" 0 \
      "--format terminal" 2 \
      "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" --format terminal -y -v

    run_test 33 "verbose output (-v)" 0 \
      "-v" 2 \
      "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" -y -v

    run_test 34 "non-verbose output (no -v)" 0 \
      "(no -v)" 2 \
      "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" -y

    run_test 35 "--video enable recording" 0 \
      "--video" 2 \
      "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" --video -y -v

    run_test 36 "--no-video disable recording" 0 \
      "--no-video" 2 \
      "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" --no-video -y -v

    run_test 37 "--video --video-resolution 480x854" 0 \
      "--video --video-resolution 480x854" 2 \
      "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" --video --video-resolution "480x854" -y -v

    run_test 38 "--video --video-framerate 4" 0 \
      "--video --video-framerate 4" 2 \
      "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" --video --video-framerate 4 -y -v

    run_test 39 "--timeout 45s (generous)" 0 \
      "--timeout 45s" 2 \
      "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" --timeout 45s -y -v

    run_test 40 "--timeout 100ms (too short, expect fail)" 1 \
      "--timeout 100ms" 2 \
      "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" --timeout 100ms -y -v

    run_test 41 "-y auto-confirm lifecycle (clear data)" 0 \
      "-y (clear app data)" 2 \
      "$PROBE" test "$LIFECYCLE_TEST" --device "$ANDROID_DEVICE" -y -v

    run_test 42 "--config probe.yaml (explicit)" 0 \
      "--config probe.yaml" 2 \
      "$PROBE" test "$SMOKE_TEST" --config probe.yaml --device "$ANDROID_DEVICE" -y -v

    run_test 43 "--config nonexistent.yaml (expect error)" 1 \
      "--config <bad>" 2 \
      "$PROBE" test "$SMOKE_TEST" --config /tmp/nonexistent_probe.yaml

    run_test 44 "--adb explicit path" 0 \
      "--adb \$(which adb)" 2 \
      "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" --adb "$(which adb)" -y -v

    run_test 45 "--adb /nonexistent/adb (expect error)" 1 \
      "--adb /nonexistent/adb" 2 \
      "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" --adb /nonexistent/adb -y

    run_test 46 "intentional failure + screenshot capture" 1 \
      "(failure path)" 2 \
      "$PROBE" test "$FAIL_TEST" --device "$ANDROID_DEVICE" -y -v

    run_test 47 "--reconnect-delay 3s with lifecycle" 0 \
      "--reconnect-delay 3s" 2 \
      "$PROBE" test "$LIFECYCLE_TEST" --device "$ANDROID_DEVICE" --reconnect-delay 3s -y -v

    run_test 48 "--format json to stdout (no -o)" 0 \
      "--format json (stdout)" 2 \
      "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" --format json -y

    run_test 49 "--visual-threshold 1.0 --visual-pixel-delta 16" 0 \
      "--visual-threshold 1.0 --visual-pixel-delta 16" 2 \
      "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" --visual-threshold 1.0 --visual-pixel-delta 16 -y -v

    run_test 50 "multiple file args (2 test files)" 0 \
      "<file1> <file2>" 2 \
      "$PROBE" test "$SMOKE_TEST" "$LOGIN_TEST" --device "$ANDROID_DEVICE" -y -v

    run_test 51 "login/ dir + --tag smoke (intersection)" 0 \
      "<dir> -t smoke" 2 \
      "$PROBE" test probe_tests/login/ --device "$ANDROID_DEVICE" -t smoke -y -v
  fi
fi

# ============================================================================
# PHASE 3: iOS Simulator
# ============================================================================
if [ "$PHASE" = "3" ] || [ "$PHASE" = "all" ]; then
  print_header "PHASE 3: iOS Simulator"

  if [ "$IOS_AVAILABLE" = false ]; then
    echo "  No iOS simulator detected. Skipping phase 3."
    for tid in 52 53 54 55 56 57 58; do
      skip_test "$tid" "iOS test $tid" "no simulator booted" "--config ios --device <UDID>" 3
    done
  elif [ "$IOS_TOKEN_AVAILABLE" = false ]; then
    echo "  iOS simulator booted but no token file. App may not be running. Skipping iOS tests."
    skip_test 52 "iOS baseline: smoke test" "app not running" "--config ios --device <UDID>" 3
    skip_test 53 "iOS --format json -o" "app not running" "--format json -o (iOS)" 3
    skip_test 54 "iOS --tag smoke" "app not running" "-t smoke (iOS)" 3
    skip_test 55 "iOS intentional failure" "app not running" "(failure path, iOS)" 3
    skip_test 56 "iOS --video" "app not running" "--video (iOS, h264)" 3
    skip_test 57 "iOS lifecycle test" "app not running" "-y clear data (iOS)" 3
    skip_test 58 "iOS --token-timeout 5s" "app not running" "--token-timeout 5s (iOS)" 3
  else
    run_test 52 "iOS baseline: smoke test" 0 \
      "--config ios --device <UDID>" 3 \
      "$PROBE" test "$SMOKE_TEST" --config "$IOS_CONFIG" --device "$IOS_DEVICE" -y -v

    run_test 53 "iOS --format json -o" 0 \
      "--format json -o (iOS)" 3 \
      "$PROBE" test "$SMOKE_TEST" --config "$IOS_CONFIG" --device "$IOS_DEVICE" --format json -o "$RESULTS_DIR/ios_json.json" -y -v
    check_file "$RESULTS_DIR/ios_json.json" "iOS JSON results"

    run_test 54 "iOS --tag smoke (3 tests)" 0 \
      "-t smoke (iOS)" 3 \
      "$PROBE" test probe_tests/ --config "$IOS_CONFIG" --device "$IOS_DEVICE" -t smoke -y -v

    run_test 55 "iOS intentional failure" 1 \
      "(failure path, iOS)" 3 \
      "$PROBE" test "$FAIL_TEST" --config "$IOS_CONFIG" --device "$IOS_DEVICE" -y -v

    run_test 56 "iOS --video recording (h264)" 0 \
      "--video (iOS, h264)" 3 \
      "$PROBE" test "$SMOKE_TEST" --config "$IOS_CONFIG" --device "$IOS_DEVICE" --video -y -v

    run_test 57 "iOS lifecycle: clear app data" 0 \
      "-y clear data (iOS)" 3 \
      "$PROBE" test "$LIFECYCLE_TEST" --config "$IOS_CONFIG" --device "$IOS_DEVICE" -y -v

    run_test 58 "iOS --token-timeout 5s" 0 \
      "--token-timeout 5s (iOS)" 3 \
      "$PROBE" test "$SMOKE_TEST" --config "$IOS_CONFIG" --device "$IOS_DEVICE" --token-timeout 5s -y -v

    # Terminate iOS app to free port 48686 for subsequent Android tests
    if [ "$PHASE" = "all" ] && [ "$ANDROID_AVAILABLE" = true ]; then
      echo ""
      printf "  ${DIM}Terminating iOS app to free port 48686 for Android tests...${RESET}\n"
      set +e
      xcrun simctl terminate "$IOS_DEVICE" com.digacel.app.dev 2>/dev/null
      set -e
      sleep 1
    fi
  fi
fi

# ============================================================================
# PHASE 4: Meaningful Parameter Combinations
# ============================================================================
if [ "$PHASE" = "4" ] || [ "$PHASE" = "all" ]; then
  print_header "PHASE 4: Meaningful Parameter Combinations"

  if [ "$ANDROID_AVAILABLE" = false ]; then
    echo "  No Android emulator detected. Skipping phase 4."
    for tid in 59 60 61 62 63 64 65 66 67; do
      skip_test "$tid" "Combo test $tid" "no emulator" "(combination)" 4
    done
  else
    run_test 59 "CI sim: --tag smoke + json + video" 0 \
      "-t smoke --format json -o --video --video-resolution --video-framerate" 4 \
      "$PROBE" test probe_tests/ --device "$ANDROID_DEVICE" -t smoke \
        --format json -o "$RESULTS_DIR/ci_results.json" \
        --video --video-resolution "720x1280" --video-framerate 2 \
        -y -v
    check_file "$RESULTS_DIR/ci_results.json" "CI JSON results"

    run_test 60 "Report from CI JSON -> HTML" 0 \
      "report --input -o" 4 \
      "$PROBE" report --input "$RESULTS_DIR/ci_results.json" -o "$RESULTS_DIR/ci_report.html"
    check_file "$RESULTS_DIR/ci_report.html" "CI HTML report"

    run_test 61 "all custom timeouts combined" 0 \
      "--timeout --dial-timeout --token-timeout --reconnect-delay" 4 \
      "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" \
        --timeout 45s --dial-timeout 15s --token-timeout 15s --reconnect-delay 1s \
        -y -v

    run_test 62 "multi-path + tag smoke" 0 \
      "<dir1> <dir2> -t smoke" 4 \
      "$PROBE" test probe_tests/login/ probe_tests/main_menu/ \
        --device "$ANDROID_DEVICE" -t smoke -y -v

    run_test 63 "fail test -> JSON (failure metadata)" 1 \
      "--format json -o (failure)" 4 \
      "$PROBE" test "$FAIL_TEST" --device "$ANDROID_DEVICE" \
        --format json -o "$RESULTS_DIR/fail_results.json" -y -v
    check_file "$RESULTS_DIR/fail_results.json" "Failure JSON results"

    run_test 64 "junit + verbose + --config" 0 \
      "--config --format junit -o -v" 4 \
      "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" \
        --config probe.yaml --format junit -o "$RESULTS_DIR/combo_junit.xml" -y -v
    check_file "$RESULTS_DIR/combo_junit.xml" "Combo JUnit XML"

    run_test 65 "video(480x854,4fps) + json" 0 \
      "--video --video-resolution --video-framerate --format json -o" 4 \
      "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" \
        --video --video-resolution "480x854" --video-framerate 4 \
        --format json -o "$RESULTS_DIR/video_combo.json" -y -v

    run_test 66 "visual regression custom thresholds" 0 \
      "--visual-threshold 2.0 --visual-pixel-delta 32" 4 \
      "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" \
        --visual-threshold 2.0 --visual-pixel-delta 32 -y -v

    # iOS combo test (only if iOS token was available)
    if [ "$IOS_TOKEN_AVAILABLE" = true ]; then
      run_test 67 "iOS CI sim: tag+json+video" 0 \
        "-t smoke --format json --video (iOS)" 4 \
        "$PROBE" test probe_tests/ --config "$IOS_CONFIG" --device "$IOS_DEVICE" \
          -t smoke --format json -o "$RESULTS_DIR/ios_ci.json" --video -y -v
    else
      skip_test 67 "iOS CI sim: tag+json+video" "app not running" "-t smoke --format json --video (iOS)" 4
    fi
  fi
fi

# ============================================================================
# PHASE 5: Edge Cases and Error Conditions
# ============================================================================
if [ "$PHASE" = "5" ] || [ "$PHASE" = "all" ]; then
  print_header "PHASE 5: Edge Cases and Error Conditions"

  if [ "$ANDROID_AVAILABLE" = true ]; then
    run_test 68 "--video + --no-video (conflicting flags)" 0 \
      "--video --no-video" 5 \
      "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" --video --no-video -y -v

    run_test 69 "-o deep/nested/dir (auto-create?)" any \
      "-o <nested>" 5 \
      "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" \
        --format json -o "$RESULTS_DIR/deep/nested/dir/results.json" -y

    run_test 70 "--format json stdout | jq validate" 0 \
      "--format json (pipe)" 5 \
      bash -c "$PROBE test $SMOKE_TEST --device $ANDROID_DEVICE --format json -y 2>/dev/null | python3 -m json.tool > /dev/null"

    run_test 71 "recipes-only dir (0 tests)" any \
      "<recipes-dir>" 5 \
      "$PROBE" test probe_tests/recipes/ --device "$ANDROID_DEVICE" -y

    run_test 72 "--timeout 0 (use default)" 0 \
      "--timeout 0" 5 \
      "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" --timeout 0 -y -v

    run_test 73 "nonexistent .probe file" 1 \
      "<bad-path.probe>" 5 \
      "$PROBE" test probe_tests/nonexistent_test.probe --device "$ANDROID_DEVICE" -y

    run_test 74 "--format xml (invalid format)" 1 \
      "--format xml" 5 \
      "$PROBE" test "$SMOKE_TEST" --format xml --dry-run

    run_test 75 "--port 0 (invalid)" 1 \
      "--port 0" 5 \
      "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" --port 0 --dial-timeout 2s -y

    run_test 76 "--dial-timeout 120s (generous, fast connect)" 0 \
      "--dial-timeout 120s" 5 \
      "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" --dial-timeout 120s -y -v
  else
    # Edge case tests that need a device -- run the offline ones, skip device ones
    skip_test 68 "--video + --no-video (conflicting flags)" "no emulator" "--video --no-video" 5
    skip_test 69 "-o deep/nested/dir (auto-create?)" "no emulator" "-o <nested>" 5
    skip_test 70 "--format json stdout | jq validate" "no emulator" "--format json (pipe)" 5
    skip_test 71 "recipes-only dir (0 tests)" "no emulator" "<recipes-dir>" 5
    skip_test 72 "--timeout 0 (use default)" "no emulator" "--timeout 0" 5

    run_test 73 "nonexistent .probe file" 1 \
      "<bad-path.probe>" 5 \
      "$PROBE" test probe_tests/nonexistent_test.probe -y

    run_test 74 "--format xml (invalid format)" 1 \
      "--format xml" 5 \
      "$PROBE" test "$SMOKE_TEST" --format xml --dry-run

    skip_test 75 "--port 0 (invalid)" "no emulator" "--port 0" 5
    skip_test 76 "--dial-timeout 120s (generous, fast connect)" "no emulator" "--dial-timeout 120s" 5
  fi
fi

# ============================================================================
# PHASE 6: Subcommand Coverage
# ============================================================================
if [ "$PHASE" = "6" ] || [ "$PHASE" = "all" ]; then
  print_header "PHASE 6: Subcommand Coverage"

  run_test 77 "probe device list" 0 \
    "device list" 6 \
    "$PROBE" device list
  check_output_contains 77 "emulator-5554\|SERIAL\|simulator\|Booted" "device table output"

  run_test 78 "probe device start android (already running)" any \
    "device start --platform android" 6 \
    "$PROBE" device start --platform android

  if [ "$IOS_AVAILABLE" = true ]; then
    run_test 79 "probe device start ios" any \
      "device start --platform ios --udid" 6 \
      "$PROBE" device start --platform ios --udid "$IOS_DEVICE"
  else
    run_test 79 "probe device start ios (no sim)" any \
      "device start --platform ios" 6 \
      "$PROBE" device start --platform ios
  fi

  run_test 80 "probe device start --platform windows (invalid)" 1 \
    "device start --platform <invalid>" 6 \
    "$PROBE" device start --platform windows

  if [ -f "$RESULTS_DIR/android_json.json" ]; then
    run_test 81 "probe report --format html (explicit)" 0 \
      "report --format html --input -o" 6 \
      "$PROBE" report --input "$RESULTS_DIR/android_json.json" \
        -o "$RESULTS_DIR/explicit_html_report.html" --format html
    check_file "$RESULTS_DIR/explicit_html_report.html" "Explicit HTML report"
  else
    skip_test 81 "probe report --format html" "no JSON input from phase 2" "report --format html --input -o" 6
  fi

  run_test 82 "probe version" 0 \
    "version" 6 \
    "$PROBE" version
  check_output_contains 82 "probe version" "version string"

  run_test 83 "probe lint -v verbose" 0 \
    "lint -v" 6 \
    "$PROBE" lint probe_tests/ -v

  run_test 84 "probe init scaffold" any \
    "init" 6 \
    bash -c "mkdir -p /tmp/probe_init_test_$$ && cd /tmp/probe_init_test_$$ && $PROBE init && ls probe.yaml tests/"
fi

# ============================================================================
# SUMMARY & REPORT
# ============================================================================
print_summary
generate_html_report

# Exit with appropriate code
if [ "$FAILED" -gt 0 ]; then
  exit 1
fi
exit 0
