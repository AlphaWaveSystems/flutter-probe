#!/usr/bin/env bash
set -euo pipefail

# FlutterProbe CI entrypoint
# Starts the Android emulator, waits for boot, then runs probe.

EMULATOR_NAME="${PROBE_EMULATOR:-probe_ci}"
DEVICE_SERIAL="${PROBE_DEVICE:-emulator-5554}"

echo "⬡ FlutterProbe CI"
echo "  Emulator : ${EMULATOR_NAME}"
echo "  App      : /app"
echo ""

# Start emulator in background
echo "  Starting Android emulator..."
emulator -avd "${EMULATOR_NAME}" \
  -no-audio -no-window -no-boot-anim \
  -gpu swiftshader_indirect \
  -memory 2048 &

EMULATOR_PID=$!

# Wait for emulator to boot
echo "  Waiting for emulator to boot..."
adb wait-for-device
adb shell 'while [[ -z $(getprop sys.boot_completed) ]]; do sleep 1; done'
echo "  Emulator ready ✓"

# Build Flutter app with ProbeAgent
cd /app
echo "  Building Flutter app with PROBE_AGENT=true..."
flutter build apk --debug --dart-define=PROBE_AGENT=true

# Install APK
adb install -r build/app/outputs/flutter-apk/app-debug.apk

# Launch app
APP_ID=$(grep '^  applicationId' android/app/build.gradle | awk '{print $2}' | tr -d '"')
adb shell am start -n "${APP_ID}/.MainActivity"

# Run probe tests
echo ""
echo "  Running tests..."
exec "$@"

# Cleanup on exit
trap "kill ${EMULATOR_PID} 2>/dev/null || true" EXIT
