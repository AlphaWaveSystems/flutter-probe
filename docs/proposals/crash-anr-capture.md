# Crash and ANR capture in a run (scope only)

**Status:** Proposal, scope only. No code, no promise to build.
**Touches (if built):** `internal/device`, `internal/runner`, `internal/runner/reporter.go`, `internal/report`, the run's output folder.
**Related:** `docs/prd/run-recording.md` (step events, artifacts, redaction), `docs/proposals/webview-content-automation.md` (the rules for adb and untrusted text).

## The ask

When the app under test crashes or stops responding during a test, the run should keep the evidence of why, next to the test result, so a failed CI run can be understood without reproducing it on a device.

## What exists today (checked on `origin/main`)

- Nothing collects a crash or an ANR. If the app dies mid-test, the step fails with a lost connection, the runner tries to reconnect (`maxReconnectAttempts`, `internal/runner/executor.go`), and the test ends with a connection error. The only sign of a death is a message when the agent token cannot be read because the process is not running (`internal/device/manager.go`, "the app process ... is not running (it exited or crashed ...)"), which points the user at `adb logcat`.
- `logcat` is used only to find the agent token. `save logs` asks the in-app agent to save the app's own log; a native crash or an ANR is outside what the agent can write.
- A test result carries `Artifacts` (screenshots, video), written into the JSON, JUnit and HTML reports (`internal/runner/reporter.go`, `internal/report/html.go`). There is no field for a crash.
- Redaction exists for printed lines, reports and the hosted upload (`sysdialog.ScrubKnown`, `internal/hosted/scrub.go`).

## What would be collected

Android (emulator and device):
1. **Why the process ended**: `dumpsys activity exit-info <package>` lists the last exits of the app with a reason (crash, ANR, low memory, signal, user request) and a timestamp. Available on Android 11 and later; older versions have no equivalent and the proposal does not try one.
2. **The crash text**: `logcat -b crash -d` for the app's process (Java exception or native signal with its stack), bounded by the time the test ran.
3. **The ANR signal**: an "isn't responding" system dialog found by the existing uiautomator dump, plus the exit-info reason.
4. **A screenshot** at the time of the failure, as the run already takes one.

iOS simulator: the crash report files the system writes for the process (`.ips` in the user's diagnostic reports folder) and the simulator's log for the process over the test's time window. Physical iOS devices: not in scope until there is a documented, tested way to read their crash reports without a developer account step; the proposal states this plainly instead of guessing.

Not collected: symbolication, memory dumps, any data from outside the app under test, anything that needs root.

## How it would work

1. **Detect.** When a step fails with a lost connection, the runner asks whether the app process is gone (Android `pidof`, simulator process list) instead of only retrying. A process that is gone, or whose pid changed, is a death; a process that is alive but not answering for longer than a limit is a hang and gets the ANR check.
2. **Collect once**, after the failure and before the app is relaunched, through the same adb runner interface and quoting rules the WebView code uses: every command takes `-s <serial>`, every word that is not a constant goes through the shell-quoting helper, nothing removes or changes anything on the device.
3. **Store** in the run's own output folder, one small folder per failed test (`crash/<test>/`): a summary file (reason, time, process id, app version), the crash text and the exit-info lines. Size-capped (for example 256 KB per file) and cut at a whole line.
4. **Report.** A new optional `crash` object on the test result: reason, time, a one-line summary (the exception class or the signal name) and the relative paths. JSON gets the object, JUnit gets the summary in the failure message and the files as attachments in the same way artifacts are attached, HTML gets a section. A run with no crash has none of this.

## Rules

- **The text is untrusted.** Log and crash text comes from the device and can contain anything the app printed. It is shown as text only: never put in a ProbeScript, a shell command or a URL, and never interpreted.
- **Redaction.** All of it passes the existing masking (known secrets, credential patterns) before it is written, and the summary line is cut to a fixed length. Whether the files go into the hosted upload follows the upload's existing allow-list; they are not added to it by this proposal.
- **No effect on a passing run.** Nothing is collected, nothing is written and no extra adb call is made unless a step failed with a lost connection or a hang.
- **Bounded.** One collection per test, a time limit on it, and a fixed ceiling on the bytes read from the device.

## Slices, if the scope is accepted (each a separate PR, docs and tests included)

- S1. A collector in `internal/device` for Android: pure parsers for the exit-info text and the crash buffer, a recording-adb test harness with real captured examples, hostile-text tests.
- S2. Detection in the runner: telling a dead app from a slow one, one collection per test, no change for passing runs.
- S3. The `crash` object in the JSON, JUnit and HTML reports and the files in the output folder.
- S4. The iOS simulator collector.
- S5. The hang case: the ANR dialog check and its report wording.

## What is not known and needs a spike first

Results of the first spike (Android emulator and iOS simulator, one measurement each) are in `docs/proposals/crash-anr-capture-spike.md`; the open items below say what it did not cover.

- Whether `exit-info` is complete on the emulator images the project uses and on physical devices, and how long entries stay.
- Whether the crash buffer still holds the crash by the time the failure is noticed on a busy device, and what window to read.
- Whether a hang can be told from a slow screen without false alarms.
- Where the simulator writes the report for a debug build run from Flutter, and how soon.
- What a physical iOS device offers, if anything, that does not need extra setup.

## Not in scope

Symbolication, memory or heap dumps, a crash database or trends, uploading to a third-party service, collecting for apps that are not the one under test, and any change to how a passing run behaves.

## Questions for the owner

- Is collection on by default for a failed test, or opt-in with a flag?
- May the crash files be part of the hosted upload?
- Is a failed iOS physical-device run acceptable to leave without crash evidence for now?
