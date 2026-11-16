# Crash and ANR capture: spike results

**Status:** Evidence note for `docs/proposals/crash-anr-capture.md` ("What is not known and needs a spike first"). Docs only. Every number below was measured once on the named devices; where a statement was not tested it says so.

## Devices and method

| | Android | iOS |
|---|---|---|
| Device | emulator `emulator-5600` (AVD `ProbeWebViewSpike`, `sdk_gphone64_arm64`, Android 15, API 35, build `google/sdk_gphone64_arm64/emu64a:15/AE3A.240806.043/12960925:userdebug/dev-keys`) | simulator `ProbeCrashSpike-iPhone17`, UDID `7CBE6910-6286-44FC-BC18-97A7EDF3A1DF` (iPhone 17 Pro, iOS 26.5), created for this spike |
| App | a throwaway fixture app (`dev.probe.webviewfixture`) that can throw an uncaught exception or send itself SIGSEGV on a launch flag | a throwaway Flutter fixture (`dev.probe.pkfix`) |
| Commands | every adb call names the device: `adb -s emulator-5600 ...`; no forward, no `--remove-all` | `xcrun simctl ... 7CBE6910-...`; the app's pid from `simctl launch` |

Only these two devices were used; both were shut down afterwards.

## Android

| Question | Result |
|---|---|
| Does `dumpsys activity exit-info <package>` list the death, with a reason? | Yes, for all three kinds tried: an uncaught Java exception (`reason=4`, status 0), a native signal (`reason=5`, status 11 for SIGSEGV), a force-stop (`reason=10`, description names the caller). The app's entries are mixed with entries of other processes under the same uid (the WebView sandbox) and of other uids: the parser keeps only processes named exactly as the package or `package:service`. |
| How soon is the entry there after the death? | The first poll that saw it came about **276 ms** after the launch that crashed (one sample, polls cost about 100 to 200 ms each). No waiting loop of seconds is needed; a short bounded retry (a few polls) is enough. |
| How many entries stay? | The list for the app is **capped at 16**. With a crash and a force-stop per cycle, the first crash was still listed after 10 further cycles and gone after 20. So the entry of the death that matters must be read promptly, and one run's several deaths can push earlier ones out. |
| Does it survive a reboot? | **Mostly not.** The system persists the list on a timer (the dump says "Last Timestamp of Persistence"); after an emulator reboot the app had 4 entries and the newest was 41 minutes older than the newest entry before the reboot (19:03 against 19:44). Entries of the last minutes are lost if the device restarts. |
| Is the crash buffer (`logcat -b crash -d`) still holding the crash when the failure is noticed late? | Yes. The ring is **256 KiB** and compresses well: after 110 further crashes it held all 111 `FATAL EXCEPTION` blocks (68 KiB consumed). Flooding the **main** buffer with 3000 lines did not change the crash buffer (2665 lines before and after; the main buffer is a separate 256 KiB ring). A reboot empties it (0 lines). |
| What does the crash buffer hold for each kind? | Java: `AndroidRuntime` lines of the app's own pid (the exception and the stack). Native: a `libc` "Fatal signal" line of the app's pid, then 115 lines written by the **tombstone writer** (tag `DEBUG`, a different pid) that name the crashed pid and `>>> <package> <<<`. A crash of a **WebView sandbox process** writes about 57 KB of `crashpad` minidump text into the same buffer; it must be dropped. |
| Is the app's pid known after it died? | `pidof <package>` prints nothing and exits 1 for a dead app. The pid of the death is in the newest exit entry, so a changed or gone pid is the "dead, not slow" test. |

What this means for the slices: S1 (#632) already filters the tombstone lines and the crashpad noise because of these captures. S2 should read the exit entry **right after** the failure, with a short bounded retry, and should not rely on the entry being there after a restart of the device. The 16-entry cap argues for collecting once per failed test before the app is relaunched (as the proposal says), not at the end of a run.

## iOS simulator

| Question | Result |
|---|---|
| Is a crash report written for a crash of a debug Flutter build on a simulator? | Yes. Sending SIGSEGV to the fixture's process left `~/Library/Logs/DiagnosticReports/Runner-2026-11-15-194603.ips` (**on the host**, not in the simulator's data folder), written about 5 s after the signal (`captureTime` 19:45:58, file time 19:46). |
| What is in it? | Two JSON documents in one file (a header line, then the body): `bundleID`, `pid`, `procName`, `exception` (`EXC_CRASH`, `SIGSEGV`), `termination` (code 11, "Segmentation fault: 11", by which process), `faultingThread`, 21 threads with frames, `captureTime`. 44 KB. |
| Can a report be tied to one simulator and one app? | Yes. `procPath` contains the simulator's UDID (`.../CoreSimulator/Devices/<UDID>/data/Containers/...`) and `coalitionName` is `com.apple.CoreSimulator.SimDevice.<UDID>`, and `bundleID` and `pid` are fields. **The folder is shared by every app on the host**: other Flutter apps are also called `Runner`, so a match must use the UDID, the bundle id and the pid, never the file name. In `procPath` the OS already prints the home folder as `/Users/USER/`; the other fields (the images of the 21 threads) were not checked for paths, so the file must be masked before it is stored or shown. |

## Not tested here

- **ANR and hang detection** (S5): no hang was provoked. It waits for its own spike.
- **Physical Android devices and other API levels**: only API 35 on an emulator. `exit-info` needs a recent Android version; the cut-off was not checked here.
- **Physical iOS devices** (out of scope by decision) and what the simulator's own log (`simctl spawn <UDID> log ...`) offers for a crash.
- Whether a Dart exception that does not end the process leaves any report (it should not; not run).
- How reports are named and kept when the same app crashes many times on a simulator.

## What changes in the proposal

- S2: read `exit-info` immediately and retry briefly; do not assume it survives a restart of the device; tell a dead app from a slow one by pid (gone or changed).
- S4: the iOS collector reads `~/Library/Logs/DiagnosticReports/*.ips` newer than the failure, and keeps only files whose `bundleID`, `pid` and UDID-in-`procPath` match; it waits a bounded few seconds for the file; it masks home paths.
- A decision for the owner stays open: none of this changes the defaults already applied (opt-in, no hosted upload, no physical iOS).
