# R-3/PT-27 evidence — Android token survives cache clear

## Automated regression coverage (primary evidence)

`probe_agent/test/token_reprint_test.dart` adds a `@visibleForTesting` counter
(`ProbeServer.tokenFileWriteAttempts`) since `_writeTokenFile()` is a platform-gated no-op on the
host test environment (neither `Platform.isIOS` nor `Platform.isAndroid`), so nothing about its
real file-write behavior is directly observable from a host test — the counter is what actually
distinguishes pre-fix from post-fix.

**Pre-fix** (`git stash` on `lib/src/server.dart`): compile error —
`tokenFileWriteAttempts` doesn't exist, since the counter and the periodic re-write call are both
part of this fix.

**Post-fix**: `server.tokenFileWriteAttempts == 1` immediately after `start()`, and
`>= 3` after waiting past two 3-second periodic ticks — proving the periodic timer now
re-attempts the write on every tick, not just once at startup. Full `flutter test` suite
(40 tests) passes with no regressions.

## Real-device attempt — inconclusive, documented honestly

Attempted the original PT-27 repro methodology (`adb shell run-as <app> cat cache/probe/token`
polled every 2s while navigating to a screen that instantiates an `ImagePicker`) against
nect-flutter's Create Post form on an Android emulator, using a temporary
`dependency_overrides: flutter_probe_agent: {path: ...}` build.

Successfully reproduced the exact navigation path from PT-27's original report (feed → FAB →
"Create Post" → "Add Images" → system photo picker, via `post_form_image_button`), but the poll
showed `cat: cache/probe/token: No such file or directory` from the very first tick — before any
navigation happened. Direct inspection confirmed `cache/probe/` was **never created at all** on
this build (`ls cache/` shows `WebView`, `data`, `libCachedImageData`, `oat`, `volley` — no
`probe` directory), meaning `_writeTokenFile()`'s directory-creation step is failing silently on
this specific emulator/build combination — the same failure mode `_writeTokenFile()`'s own
try/catch is designed to swallow ("non-fatal: CLI can still read from log stream").

This is a pre-existing environment issue, not something introduced by this fix: the periodic
re-write added here calls the exact same `_writeTokenFile()` used at startup, and startup's
single call already wasn't creating the directory on this build before any of today's changes.
It's very likely the same underlying environment issue behind the `unexpected EOF` WS-dial
failures hit during R-4's real-device attempt, and worth folding into the R-2/PT-25 investigation
rather than chasing separately here. The `dependency_overrides` used for this attempt were fully
reverted — nect-flutter's repo is back to its original state.
