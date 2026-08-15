# R-4 evidence — `_textOf` reads `EditableText` (TextField/TextFormField)

## Automated regression coverage (primary evidence)

`probe_agent/test/see_contains_editabletext_test.dart`, 3 widget tests:

- `passes when a TextField pre-filled via a controller contains the checked value`
- `passes when a TextFormField pre-filled via a controller contains the checked value`
- `fails with the actual field content in the message, not a blank string`

**Pre-fix** (`git stash` on `lib/src/executor.dart`, i.e. the `main`-branch `_textOf` with only
`Text`/`RichText` handling):

```
00:00 +0 -3: ... [E]
Some tests failed.
Failing tests:
  ... passes when a TextField pre-filled via a controller contains the checked value
  ... passes when a TextFormField pre-filled via a controller contains the checked value
  ... fails with the actual field content in the message, not a blank string
```

All 3 fail — the third fails because `_stateCheckFailureReason` reports `contains "", not
"expected value"` (the empty string proving `_textOf` returned nothing for the TextField).

**Post-fix**: all 3 pass. Full `flutter test` suite (39 tests) passes with no regressions.

## Real-device attempt — inconclusive, documented honestly

Attempted against nect-flutter (Android emulator, `com.alphawavesystems.nect.dev`, dev flavor)
via a temporary `dependency_overrides: flutter_probe_agent: {path: ...}` pointing at this
worktree, `flutter build apk --flavor development --debug`, and a manual install. The app
launched correctly and printed a stable `PROBE_TOKEN` to logcat, but every WebSocket dial attempt
(`probe test ... --host 127.0.0.1 --port 48688 --token <token>`) failed with `unexpected EOF`,
consistently, over a 20-attempt/20s retry window — a connection-level failure, not a token
rejection (which per PT-25's v0.11.0 fix would fail fast with 401/403 instead of retrying).

This is very unlikely to be caused by this change: the fix touches only `_textOf` and reuses the
existing `_findTextController` helper, both used exclusively *after* an RPC has already been
dispatched over an established connection — nothing in the diff is anywhere near
`server.dart`'s WebSocket upgrade/token-validation path. Given the time cost of debugging what
looks like an unrelated emulator/dev-flavor/port-forwarding issue (`nect-flutter` runs its agent
on a non-default port, 48688), this was not pursued further for R-4; it may be the same class of
issue R-2/PT-25 is tracking and is worth revisiting there. The `dependency_overrides` used for
this attempt were fully reverted — `nect-flutter`'s repo is back to its original state.

Real-device verification for this fix currently rests on the pre-fix/post-fix widget-test
delta above, which is the same evidentiary bar the project's own `DONE.md` accepts for several
prior Dart-agent fixes (e.g. PT-26) when paired with source-level reasoning about blast radius.
