# R-2/PT-25 evidence — Android WebSocket drop investigation (negative result)

## Status: attempted, not reproduced. Root cause still open.

This is a research/repro task, not a code-fix task (per the roadmap's own framing) — `DONE.md`
explicitly says reproducing PT-25's core symptom "needs the specific background-network-load
conditions the report describes... which this project's own generic test app doesn't produce
regardless of run length." water-sip is a real Firebase-heavy app (Crashlytics, Analytics,
Remote Config, Performance, In-App Messaging) plus real IAP/billing SDK calls, so it was used as
a more realistic stand-in.

## Attempt 1 — with deliberate app backgrounding (`r2_stress_repro.probe`)

Combined navigation churn, IAP restore-purchase, and two deliberate `HOME`-key background/
foreground cycles (8s and 15s backgrounded) via concurrent `adb shell input keyevent
KEYCODE_HOME` + `am start`, with `adb logcat` capturing in the background throughout.

**Result: inconclusive, not a real repro.** The test failed at ~13.5s with `Widget not found:
id("#today_quick_add_250")` — the app had just been backgrounded via the HOME key at that exact
moment, so the widget genuinely wasn't on screen. The logcat shows a clean `ProbeAgent: CLI
disconnected` correlated with the deliberate backgrounding's task CLOSE transition — expected
lifecycle behavior, not a spontaneous drop. This attempt conflated "the app is deliberately
backgrounded so nothing is visible" with PT-25's actual symptom (a drop *during* normal,
continuous foreground operation), so it doesn't test the real hypothesis.

## Attempt 2 — continuous foreground, no backgrounding (`r2_stress_repro2.probe`)

15 iterations of: quick-add → tab switch (History/Settings/Today) → open paywall → back, each
step separated by real waits — 97 seconds of continuous foreground operation under real Firebase
background load (Crashlytics/Analytics/RemoteConfig/Performance/InAppMessaging all active) plus
repeated IAP paywall opens. `adb logcat` captured concurrently throughout, filtered for
WebSocket/socket/exception signals.

**Result: passed cleanly. No WebSocket drop.** The only two `ProbeAgent: CLI disconnected` log
lines correlate exactly with the test's start (the prior test's `restart the app` force-stop) and
end (the CLI's own deliberate disconnect after the test completed) — both expected, not a mid-session
drop. `Socket closed`/`ClassCastException` noise in the logcat is system-level (Conscrypt TLS,
`GeneratedPluginRegistrant`) and occurs only during the app's cold-start window, unrelated to the
ProbeAgent's own WebSocket server.

## What this rules out (useful for whoever continues this)

~100 seconds of continuous Firebase background chatter (Crashlytics/Analytics/RemoteConfig/
Performance/InAppMessaging) plus IAP billing SDK calls, on their own, are **not sufficient** to
trigger the drop — at least not reliably within that window on this Android emulator. Plausible
next steps, not attempted here due to time:
- Much longer duration (10+ minutes) — Doze mode / background network restrictions may need more
  elapsed time to kick in even with the app foregrounded.
- Genuine network condition changes (WiFi↔cellular handoff, airplane-mode toggle) rather than just
  background SDK chatter — `probe` has no `toggle airplane mode` verb yet (a Maestro-parity gap
  noted in the competitive roadmap's Phase 1, item E-6) which would help test this directly.
- A physical device rather than an emulator — Doze/App Standby behavior differs meaningfully from
  emulator networking, and PT-25's original report may have been against real hardware.
- The original reporting project's own app/backend, if access can be arranged, since its exact
  Firebase/Firestore usage pattern is what triggered the original observation.

## Repro scripts

Both included in this folder for reuse in future attempts:
`r2_stress_repro.probe` (with backgrounding — kept for reference, not recommended as-is since it
conflates two different failure modes) and `r2_stress_repro2.probe` (clean continuous-foreground
version, the one to extend/lengthen for the next attempt).
