#!/bin/bash
# ============================================================================
# FlutterProbe CLI E2E Parameter Test Suite
# ============================================================================
# Comprehensive test of every CLI parameter and meaningful combinations.
# Runs locally against Android emulator and iOS simulator.
#
# Usage:
#   ./tests/e2e_cli_params/run_all.sh [phase]
#   Phases: 1 (offline), 2 (android), 3 (ios), 4 (combos), 5 (edge), 6 (subcmds), all
#
# Prerequisites:
#   - bin/probe built (make build)
#   - Android emulator running with the Flutter app under test (emulator-5554)
#   - iOS simulator booted with the Flutter app under test (for phase 3)
# ============================================================================

set -euo pipefail

PROBE="$(cd "$(dirname "$0")/../.." && pwd)/bin/probe"
APP_DIR="${APP_DIR:-}"
if [ -z "$APP_DIR" ]; then
  echo "ERROR: APP_DIR is not set. Export APP_DIR to point to your Flutter project directory."
  echo "  export APP_DIR=/path/to/your/flutter/project"
  exit 1
fi
ANDROID_DEVICE="emulator-5554"
# Auto-detect the first booted iOS simulator, or override via IOS_DEVICE env var
IOS_DEVICE="${IOS_DEVICE:-$(xcrun simctl list devices booted -j 2>/dev/null | python3 -c "import sys,json; devs=[d['udid'] for r in json.load(sys.stdin)['devices'].values() for d in r if d['state']=='Booted']; print(devs[0] if devs else '')" 2>/dev/null || echo "")}"
IOS_CONFIG="probe_ios.yaml"
RESULTS_DIR="/tmp/probe_e2e_results"
SMOKE_TEST="probe_tests/smoke/00_smoke_happy_path.probe"
LOGIN_TEST="probe_tests/login/01_login_happy_path.probe"
FAIL_TEST="probe_tests/fail/09_intentional_fail.probe"
LIFECYCLE_TEST="probe_tests/lifecycle/40_clear_data_shows_login.probe"

# Counters
TOTAL=0
PASSED=0
FAILED=0
SKIPPED=0
FAILURES=()

# Colors
GREEN='\033[32m'
RED='\033[31m'
YELLOW='\033[33m'
CYAN='\033[36m'
BOLD='\033[1m'
RESET='\033[0m'

mkdir -p "$RESULTS_DIR"

# ---- Helpers ----

run_test() {
  local test_id="$1"
  local description="$2"
  local expect_exit="$3"  # 0 = expect success, 1 = expect failure, any = either
  shift 3
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
  if [ "$expect_exit" = "any" ]; then
    verdict="PASS"
  elif [ "$expect_exit" -eq 0 ] && [ "$actual_exit" -eq 0 ]; then
    verdict="PASS"
  elif [ "$expect_exit" -eq 1 ] && [ "$actual_exit" -ne 0 ]; then
    verdict="PASS"
  else
    verdict="FAIL"
  fi

  if [ "$verdict" = "PASS" ]; then
    PASSED=$((PASSED + 1))
    printf "${GREEN}PASS${RESET} (exit=%d, %ds)\n" "$actual_exit" "$duration"
  else
    FAILED=$((FAILED + 1))
    printf "${RED}FAIL${RESET} (expected exit=%s, got=%d, %ds)\n" "$expect_exit" "$actual_exit" "$duration"
    FAILURES+=("[$test_id] $description (expected=$expect_exit, got=$actual_exit)")
  fi
}

skip_test() {
  local test_id="$1"
  local description="$2"
  local reason="$3"
  TOTAL=$((TOTAL + 1))
  SKIPPED=$((SKIPPED + 1))
  printf "${CYAN}[%03d]${RESET} %-65s ${YELLOW}SKIP${RESET} (%s)\n" "$test_id" "$description" "$reason"
}

# Check a file exists and is non-empty after a test (informational only, never exits)
check_file() {
  local file="$1"
  local label="$2"
  if [ -f "$file" ] && [ -s "$file" ]; then
    printf "      → %s: ${GREEN}exists${RESET} (%s bytes)\n" "$label" "$(wc -c < "$file" | tr -d ' ')"
  else
    printf "      → %s: ${RED}missing or empty${RESET}\n" "$label"
  fi
  return 0
}

# Check output log contains a string (informational only, never exits)
check_output_contains() {
  local test_id="$1"
  local pattern="$2"
  local label="$3"
  local log="$RESULTS_DIR/test_${test_id}.log"
  if grep -q "$pattern" "$log" 2>/dev/null; then
    printf "      → %s: ${GREEN}found${RESET}\n" "$label"
  else
    printf "      → %s: ${RED}not found${RESET} (pattern: %s)\n" "$label" "$pattern"
  fi
  return 0
}

print_header() {
  echo ""
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  printf "  ${BOLD}%s${RESET}\n" "$1"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
}

print_summary() {
  echo ""
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  printf "  ${BOLD}RESULTS${RESET}: "
  printf "${GREEN}%d passed${RESET}, " "$PASSED"
  if [ "$FAILED" -gt 0 ]; then
    printf "${RED}%d failed${RESET}, " "$FAILED"
  else
    printf "0 failed, "
  fi
  printf "${YELLOW}%d skipped${RESET} / %d total\n" "$SKIPPED" "$TOTAL"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

  if [ ${#FAILURES[@]} -gt 0 ]; then
    echo ""
    printf "  ${RED}${BOLD}Failures:${RESET}\n"
    for f in "${FAILURES[@]}"; do
      printf "    ${RED}✗${RESET} %s\n" "$f"
    done
  fi
  echo ""
  echo "  Logs: $RESULTS_DIR/"
  echo ""
}

# ---- Determine which phases to run ----
PHASE="${1:-all}"

cd "$APP_DIR"

# ============================================================================
# PHASE 1: Offline / Fast Tests (no device needed)
# ============================================================================
if [ "$PHASE" = "1" ] || [ "$PHASE" = "all" ]; then
  print_header "PHASE 1: Offline Tests (no device needed)"

  # 1. probe version
  run_test 1 "probe version" 0 \
    "$PROBE" version

  # 2. probe lint on login directory
  run_test 2 "probe lint probe_tests/login/" 0 \
    "$PROBE" lint probe_tests/login/

  # 3. probe lint on single file
  run_test 3 "probe lint single file" 0 \
    "$PROBE" lint "$SMOKE_TEST"

  # 4. probe lint on all test directories
  run_test 4 "probe lint all test dirs" 0 \
    "$PROBE" lint probe_tests/

  # 5. probe lint on nonexistent path
  run_test 5 "probe lint nonexistent path (expect error)" 1 \
    "$PROBE" lint nonexistent/path/

  # 6. probe test --dry-run single file
  run_test 6 "probe test --dry-run single smoke file" 0 \
    "$PROBE" test "$SMOKE_TEST" --dry-run

  # 7. probe test --dry-run entire suite
  run_test 7 "probe test --dry-run entire test suite" 0 \
    "$PROBE" test probe_tests/ --dry-run

  # 8. probe test --dry-run --tag smoke
  run_test 8 "probe test --dry-run --tag smoke" 0 \
    "$PROBE" test probe_tests/ --dry-run -t smoke
  check_output_contains 8 "3 passed" "3 smoke tests found"

  # 9. probe test --dry-run --tag labels
  run_test 9 "probe test --dry-run --tag labels" 0 \
    "$PROBE" test probe_tests/ --dry-run -t labels
  check_output_contains 9 "4 passed" "4 labels tests found"

  # 10. probe test --dry-run --tag regression
  run_test 10 "probe test --dry-run --tag regression" 0 \
    "$PROBE" test probe_tests/ --dry-run -t regression
  check_output_contains 10 "9 passed" "9 regression tests found"

  # 11. probe test --dry-run --tag nonexistent
  run_test 11 "probe test --dry-run --tag nonexistent (0 tests)" 0 \
    "$PROBE" test probe_tests/ --dry-run -t nonexistent_tag
  check_output_contains 11 "0 passed" "0 tests matched"

  # 12. probe test --dry-run --format json
  run_test 12 "probe test --dry-run --format json" 0 \
    "$PROBE" test "$SMOKE_TEST" --dry-run --format json

  # 13. probe test --dry-run --format junit
  run_test 13 "probe test --dry-run --format junit" 0 \
    "$PROBE" test "$SMOKE_TEST" --dry-run --format junit

  # 14. probe device list
  run_test 14 "probe device list" 0 \
    "$PROBE" device list
  check_output_contains 14 "emulator-5554" "Android emulator listed"

  # 15. probe report with missing input file
  run_test 15 "probe report --input nonexistent (expect error)" 1 \
    "$PROBE" report --input /tmp/nonexistent_probe_results.json

  # 16. probe test --dry-run from wrong directory (no tests/ default)
  run_test 16 "probe test --dry-run from /tmp (no tests found)" any \
    bash -c "cd /tmp && $PROBE test --dry-run"

  # 17. probe --help
  run_test 17 "probe --help" 0 \
    "$PROBE" --help
  check_output_contains 17 "Available Commands" "help text shows commands"

  # 18. probe test --help
  run_test 18 "probe test --help" 0 \
    "$PROBE" test --help
  check_output_contains 18 "\-\-tag" "help shows --tag flag"
  check_output_contains 18 "\-\-video" "help shows --video flag"
  check_output_contains 18 "\-\-format" "help shows --format flag"

  # 19. probe test --dry-run with --device (device should be ignored)
  run_test 19 "probe test --dry-run ignores --device" 0 \
    "$PROBE" test "$SMOKE_TEST" --dry-run --device nonexistent-device

  # 20. probe test --dry-run --format json -o file
  run_test 20 "probe test --dry-run --format json -o file" 0 \
    "$PROBE" test "$SMOKE_TEST" --dry-run --format json -o "$RESULTS_DIR/dryrun_output.json"
  check_file "$RESULTS_DIR/dryrun_output.json" "JSON output file"
fi

# ============================================================================
# PHASE 2: Android Emulator — Individual Parameters
# ============================================================================
if [ "$PHASE" = "2" ] || [ "$PHASE" = "all" ]; then
  print_header "PHASE 2: Android Emulator — Individual Parameters"

  # 21. Baseline: minimal Android test (auto-detect device)
  run_test 21 "Android baseline: smoke test (auto-detect device)" 0 \
    "$PROBE" test "$SMOKE_TEST" -y -v

  # 22. Explicit --device
  run_test 22 "Android --device emulator-5554 explicit" 0 \
    "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" -y -v

  # 23. --device with invalid serial (shell injection attempt)
  run_test 23 "--device invalid;serial (expect rejection)" 1 \
    "$PROBE" test "$SMOKE_TEST" --device "invalid;serial" -y

  # 24. --device nonexistent but valid format
  run_test 24 "--device nonexistent-device (expect timeout)" 1 \
    "$PROBE" test "$SMOKE_TEST" --device "nonexistent-1234" --token-timeout 3s -y

  # 25. --port wrong port (agent not listening)
  run_test 25 "--port 48687 wrong port (expect fail)" 1 \
    "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" --port 48687 --dial-timeout 3s -y

  # 26. --dial-timeout short with wrong port (fast fail)
  run_test 26 "--dial-timeout 2s with wrong port (fast fail)" 1 \
    "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" --port 48687 --dial-timeout 2s -y

  # 27. --token-timeout short (should succeed — token available quickly)
  run_test 27 "--token-timeout 5s (fast token)" 0 \
    "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" --token-timeout 5s -y -v

  # 28. --tag smoke (real execution)
  run_test 28 "--tag smoke (3 tests)" 0 \
    "$PROBE" test probe_tests/ --device "$ANDROID_DEVICE" -t smoke -y -v

  # 29. --format json -o file
  run_test 29 "--format json -o results.json" 0 \
    "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" --format json -o "$RESULTS_DIR/android_json.json" -y -v
  check_file "$RESULTS_DIR/android_json.json" "JSON results file"

  # 30. probe report from JSON (depends on #29)
  run_test 30 "probe report from JSON → HTML" 0 \
    "$PROBE" report --input "$RESULTS_DIR/android_json.json" -o "$RESULTS_DIR/android_report.html"
  check_file "$RESULTS_DIR/android_report.html" "HTML report file"

  # 31. --format junit -o file
  run_test 31 "--format junit -o results.xml" 0 \
    "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" --format junit -o "$RESULTS_DIR/android_junit.xml" -y -v
  check_file "$RESULTS_DIR/android_junit.xml" "JUnit XML file"
  check_output_contains 31 "testsuites\|testsuite\|testcase" "JUnit XML structure"

  # 32. --format terminal (explicit, same as default)
  run_test 32 "--format terminal (explicit)" 0 \
    "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" --format terminal -y -v

  # 33. Verbose vs non-verbose
  run_test 33 "verbose output (-v)" 0 \
    "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" -y -v

  run_test 34 "non-verbose output (no -v)" 0 \
    "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" -y

  # 35. --video enable
  run_test 35 "--video enable recording" 0 \
    "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" --video -y -v

  # 36. --no-video disable
  run_test 36 "--no-video disable recording" 0 \
    "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" --no-video -y -v

  # 37. --video with custom resolution
  run_test 37 "--video --video-resolution 480x854" 0 \
    "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" --video --video-resolution "480x854" -y -v

  # 38. --video with custom framerate
  run_test 38 "--video --video-framerate 4" 0 \
    "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" --video --video-framerate 4 -y -v

  # 39. --timeout 45s (generous, should pass)
  run_test 39 "--timeout 45s (generous)" 0 \
    "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" --timeout 45s -y -v

  # 40. --timeout 100ms (too short, expect failure)
  run_test 40 "--timeout 100ms (too short, expect fail)" 1 \
    "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" --timeout 100ms -y -v

  # 41. -y with lifecycle test (clear app data)
  run_test 41 "-y auto-confirm lifecycle (clear data)" 0 \
    "$PROBE" test "$LIFECYCLE_TEST" --device "$ANDROID_DEVICE" -y -v

  # 42. --config explicit probe.yaml
  run_test 42 "--config probe.yaml (explicit)" 0 \
    "$PROBE" test "$SMOKE_TEST" --config probe.yaml --device "$ANDROID_DEVICE" -y -v

  # 43. --config nonexistent file
  run_test 43 "--config nonexistent.yaml (expect error)" 1 \
    "$PROBE" test "$SMOKE_TEST" --config /tmp/nonexistent_probe.yaml

  # 44. --adb explicit path (which adb)
  run_test 44 "--adb $(which adb) explicit path" 0 \
    "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" --adb "$(which adb)" -y -v

  # 45. --adb invalid path
  run_test 45 "--adb /nonexistent/adb (expect error)" 1 \
    "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" --adb /nonexistent/adb -y

  # 46. Intentional failure test (screenshot on failure)
  run_test 46 "intentional failure + screenshot capture" 1 \
    "$PROBE" test "$FAIL_TEST" --device "$ANDROID_DEVICE" -y -v

  # 47. --reconnect-delay override
  run_test 47 "--reconnect-delay 3s with lifecycle" 0 \
    "$PROBE" test "$LIFECYCLE_TEST" --device "$ANDROID_DEVICE" --reconnect-delay 3s -y -v

  # 48. --format json to stdout (no -o)
  run_test 48 "--format json to stdout (no -o)" 0 \
    "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" --format json -y

  # 49. --visual-threshold and --visual-pixel-delta
  run_test 49 "--visual-threshold 1.0 --visual-pixel-delta 16" 0 \
    "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" --visual-threshold 1.0 --visual-pixel-delta 16 -y -v

  # 50. Multiple file arguments
  run_test 50 "multiple file args (2 test files)" 0 \
    "$PROBE" test "$SMOKE_TEST" "$LOGIN_TEST" --device "$ANDROID_DEVICE" -y -v

  # 51. Directory + tag filter intersection
  run_test 51 "login/ dir + --tag smoke (intersection)" 0 \
    "$PROBE" test probe_tests/login/ --device "$ANDROID_DEVICE" -t smoke -y -v
fi

# ============================================================================
# PHASE 3: iOS Simulator
# ============================================================================
if [ "$PHASE" = "3" ] || [ "$PHASE" = "all" ]; then
  print_header "PHASE 3: iOS Simulator"

  # Check if iOS app is running
  IOS_TOKEN_FILE="$HOME/Library/Developer/CoreSimulator/Devices/$IOS_DEVICE/data/tmp/probe/token"
  if [ ! -f "$IOS_TOKEN_FILE" ]; then
    echo "  ⚠  iOS app not running (no token file). Skipping iOS tests."
    skip_test 52 "iOS baseline: smoke test" "app not running"
    skip_test 53 "iOS --format json -o" "app not running"
    skip_test 54 "iOS --tag smoke" "app not running"
    skip_test 55 "iOS intentional failure" "app not running"
    skip_test 56 "iOS --video" "app not running"
    skip_test 57 "iOS lifecycle test" "app not running"
    skip_test 58 "iOS --token-timeout 5s" "app not running"
  else
    # 52. iOS baseline
    run_test 52 "iOS baseline: smoke test" 0 \
      "$PROBE" test "$SMOKE_TEST" --config "$IOS_CONFIG" --device "$IOS_DEVICE" -y -v

    # 53. iOS --format json -o
    run_test 53 "iOS --format json -o" 0 \
      "$PROBE" test "$SMOKE_TEST" --config "$IOS_CONFIG" --device "$IOS_DEVICE" --format json -o "$RESULTS_DIR/ios_json.json" -y -v
    check_file "$RESULTS_DIR/ios_json.json" "iOS JSON results"

    # 54. iOS --tag smoke
    run_test 54 "iOS --tag smoke (3 tests)" 0 \
      "$PROBE" test probe_tests/ --config "$IOS_CONFIG" --device "$IOS_DEVICE" -t smoke -y -v

    # 55. iOS intentional failure
    run_test 55 "iOS intentional failure" 1 \
      "$PROBE" test "$FAIL_TEST" --config "$IOS_CONFIG" --device "$IOS_DEVICE" -y -v

    # 56. iOS --video
    run_test 56 "iOS --video recording (h264)" 0 \
      "$PROBE" test "$SMOKE_TEST" --config "$IOS_CONFIG" --device "$IOS_DEVICE" --video -y -v

    # 57. iOS lifecycle (clear app data)
    run_test 57 "iOS lifecycle: clear app data" 0 \
      "$PROBE" test "$LIFECYCLE_TEST" --config "$IOS_CONFIG" --device "$IOS_DEVICE" -y -v

    # 58. iOS --token-timeout
    run_test 58 "iOS --token-timeout 5s" 0 \
      "$PROBE" test "$SMOKE_TEST" --config "$IOS_CONFIG" --device "$IOS_DEVICE" --token-timeout 5s -y -v
  fi
fi

# ============================================================================
# PHASE 4: Meaningful Parameter Combinations
# ============================================================================
if [ "$PHASE" = "4" ] || [ "$PHASE" = "all" ]; then
  print_header "PHASE 4: Meaningful Parameter Combinations"

  # 59. Full CI simulation: JSON + video + tag + output
  run_test 59 "CI sim: --tag smoke + json + video" 0 \
    "$PROBE" test probe_tests/ --device "$ANDROID_DEVICE" -t smoke \
      --format json -o "$RESULTS_DIR/ci_results.json" \
      --video --video-resolution "720x1280" --video-framerate 2 \
      -y -v
  check_file "$RESULTS_DIR/ci_results.json" "CI JSON results"

  # 60. Report from CI JSON
  run_test 60 "Report from CI JSON → HTML" 0 \
    "$PROBE" report --input "$RESULTS_DIR/ci_results.json" -o "$RESULTS_DIR/ci_report.html"
  check_file "$RESULTS_DIR/ci_report.html" "CI HTML report"

  # 61. Custom timeouts combo
  run_test 61 "all custom timeouts combined" 0 \
    "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" \
      --timeout 45s --dial-timeout 15s --token-timeout 15s --reconnect-delay 1s \
      -y -v

  # 62. Multiple paths + tag filter
  run_test 62 "multi-path + tag smoke" 0 \
    "$PROBE" test probe_tests/login/ probe_tests/main_menu/ \
      --device "$ANDROID_DEVICE" -t smoke -y -v

  # 63. Fail test with JSON output (verify failure metadata)
  run_test 63 "fail test → JSON (failure metadata)" 1 \
    "$PROBE" test "$FAIL_TEST" --device "$ANDROID_DEVICE" \
      --format json -o "$RESULTS_DIR/fail_results.json" -y -v
  check_file "$RESULTS_DIR/fail_results.json" "Failure JSON results"

  # 64. JUnit + verbose + explicit config
  run_test 64 "junit + verbose + --config" 0 \
    "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" \
      --config probe.yaml --format junit -o "$RESULTS_DIR/combo_junit.xml" -y -v
  check_file "$RESULTS_DIR/combo_junit.xml" "Combo JUnit XML"

  # 65. Video + custom resolution + framerate + JSON
  run_test 65 "video(480x854,4fps) + json" 0 \
    "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" \
      --video --video-resolution "480x854" --video-framerate 4 \
      --format json -o "$RESULTS_DIR/video_combo.json" -y -v

  # 66. Visual regression + custom thresholds + verbose
  run_test 66 "visual regression custom thresholds" 0 \
    "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" \
      --visual-threshold 2.0 --visual-pixel-delta 32 -y -v

  # 67. iOS full CI sim (if available)
  IOS_TOKEN_FILE="$HOME/Library/Developer/CoreSimulator/Devices/$IOS_DEVICE/data/tmp/probe/token"
  if [ -f "$IOS_TOKEN_FILE" ]; then
    run_test 67 "iOS CI sim: tag+json+video" 0 \
      "$PROBE" test probe_tests/ --config "$IOS_CONFIG" --device "$IOS_DEVICE" \
        -t smoke --format json -o "$RESULTS_DIR/ios_ci.json" --video -y -v
  else
    skip_test 67 "iOS CI sim: tag+json+video" "app not running"
  fi
fi

# ============================================================================
# PHASE 5: Edge Cases and Error Conditions
# ============================================================================
if [ "$PHASE" = "5" ] || [ "$PHASE" = "all" ]; then
  print_header "PHASE 5: Edge Cases and Error Conditions"

  # 68. --video and --no-video together (last wins)
  run_test 68 "--video + --no-video (conflicting flags)" 0 \
    "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" --video --no-video -y -v

  # 69. -o to deep nonexistent directory
  run_test 69 "-o deep/nested/dir (auto-create?)" any \
    "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" \
      --format json -o "$RESULTS_DIR/deep/nested/dir/results.json" -y

  # 70. --format json to stdout (pipe-friendly)
  run_test 70 "--format json stdout | jq validate" 0 \
    bash -c "$PROBE test $SMOKE_TEST --device $ANDROID_DEVICE --format json -y 2>/dev/null | python3 -m json.tool > /dev/null"

  # 71. Empty recipe-only directory
  run_test 71 "recipes-only dir (0 tests)" any \
    "$PROBE" test probe_tests/recipes/ --device "$ANDROID_DEVICE" -y

  # 72. --timeout 0 (use config default)
  run_test 72 "--timeout 0 (use default)" 0 \
    "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" --timeout 0 -y -v

  # 73. Single file that doesn't exist
  run_test 73 "nonexistent .probe file" 1 \
    "$PROBE" test probe_tests/nonexistent_test.probe --device "$ANDROID_DEVICE" -y

  # 74. --format invalid
  run_test 74 "--format xml (invalid format)" 1 \
    "$PROBE" test "$SMOKE_TEST" --format xml --dry-run

  # 75. --port 0 (invalid port)
  run_test 75 "--port 0 (invalid)" 1 \
    "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" --port 0 --dial-timeout 2s -y

  # 76. Extremely long --dial-timeout (but correct port, should connect fast)
  run_test 76 "--dial-timeout 120s (generous, fast connect)" 0 \
    "$PROBE" test "$SMOKE_TEST" --device "$ANDROID_DEVICE" --dial-timeout 120s -y -v
fi

# ============================================================================
# PHASE 6: Subcommand Coverage
# ============================================================================
if [ "$PHASE" = "6" ] || [ "$PHASE" = "all" ]; then
  print_header "PHASE 6: Subcommand Coverage"

  # 77. probe device list (already tested, but explicit)
  run_test 77 "probe device list" 0 \
    "$PROBE" device list
  check_output_contains 77 "emulator-5554\|SERIAL" "device table output"

  # 78. probe device start android (already running)
  run_test 78 "probe device start android (already running)" any \
    "$PROBE" device start --platform android

  # 79. probe device start ios (already booted)
  run_test 79 "probe device start ios" any \
    "$PROBE" device start --platform ios --udid "$IOS_DEVICE"

  # 80. probe device start invalid platform
  run_test 80 "probe device start --platform windows (invalid)" 1 \
    "$PROBE" device start --platform windows

  # 81. probe report --format html (explicit)
  if [ -f "$RESULTS_DIR/android_json.json" ]; then
    run_test 81 "probe report --format html (explicit)" 0 \
      "$PROBE" report --input "$RESULTS_DIR/android_json.json" \
        -o "$RESULTS_DIR/explicit_html_report.html" --format html
    check_file "$RESULTS_DIR/explicit_html_report.html" "Explicit HTML report"
  else
    skip_test 81 "probe report --format html" "no JSON input from phase 2"
  fi

  # 82. probe version
  run_test 82 "probe version" 0 \
    "$PROBE" version
  check_output_contains 82 "probe version" "version string"

  # 83. probe lint --verbose
  run_test 83 "probe lint -v verbose" 0 \
    "$PROBE" lint probe_tests/ -v

  # 84. probe init in temp dir
  run_test 84 "probe init scaffold" any \
    bash -c "cd /tmp/probe_init_test_$$ && mkdir -p /tmp/probe_init_test_$$ && $PROBE init && ls probe.yaml tests/"
fi

# ============================================================================
# SUMMARY
# ============================================================================
print_summary

# Exit with appropriate code
if [ "$FAILED" -gt 0 ]; then
  exit 1
fi
exit 0
