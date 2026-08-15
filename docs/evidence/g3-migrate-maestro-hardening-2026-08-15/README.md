# G-3 evidence — hardening `probe migrate maestro` against nect-flutter's real 76-flow suite

## The roadmap's four named commands didn't actually appear in the corpus

The roadmap named `setPermissions`, `relativePoint`, `retry`, and `assertScreenshot` as the 2.x
syntax to harden against. Running a command-frequency audit against nect-flutter's real
`.maestro/flows/` tree first — before writing any conversion code — found **zero occurrences of
any of the four** anywhere in the real suite. What the corpus actually contained, by frequency:

```
448 tapOn                 157 assertVisible          14 assertNotVisible
341 extendedWaitUntil     148 runFlow                 6 pressKey
200 takeScreenshot        119 waitForAnimationToEnd   1 launchApp
 73 scrollUntilVisible     64 inputText               1 evalScript
 27 eraseText
```

`extendedWaitUntil`, `scrollUntilVisible`, and `eraseText` — none previously supported — account
for **441 of the 1,608 total step invocations across the suite (27%)**. This is reported honestly
rather than silently substituted: both sets of commands are now supported (see below), because the
roadmap's four are real, documented Maestro 2.x features worth supporting regardless of whether
this one corpus happens to use them, and the corpus's own actual gaps were the higher-value find.

## What's now supported that wasn't before

**The roadmap's four**, plus two real bugs the previous handling would have hit on real input:
- `setPermissions` — one `allow permission "x"` / `deny permission "x"` line per entry, sorted for
  reproducible output. Unknown values (e.g. Maestro's `unset`) get a `# TODO` and a warning rather
  than being silently dropped.
- `retry` — maps directly onto ProbeScript's own `retry N times` block (added earlier this cycle,
  E-1). Nested `commands` are now recursively converted into a properly indented body, not left as
  a "manual migration required" placeholder.
- `assertScreenshot` — maps onto `compare screenshot "name"` (E-2, this cycle). Handles the bare
  string form, the map form with an explicit name, and warns that a per-assertion `threshold` isn't
  preserved (set `visual.threshold` in `probe.yaml` instead).
- `relativePoint` (`tapOn`/`longPressOn`/`doubleTapOn` with a `point: "x%,y%"` selector instead of
  `id`/`text`) — **a real bug, not just a missing feature**: the old code fell through to
  `quoteVal`'s generic map fallback and would have silently emitted `tap on "map[point:50%,10%]"`
  — a token that isn't a real selector and would fail confusingly at runtime, not at migration
  time. Confirmed against a real occurrence in the corpus
  (`posts/posts-list-page-elements.yaml`) — see below. Now emits a clear `# TODO` explaining
  ProbeScript is selector-only with no coordinate-tap equivalent, and doesn't try to guess.
- `repeat`'s nested `commands` — same bug class as `retry`: previously silently dropped with a
  "requires manual migration" warning and nothing else. Now uses the same recursive conversion as
  `retry`.
- `setLocation` — the existing code had a stale `# not supported in ProbeScript P0` comment;
  `set location <lat>, <lng>` has been a real, working ProbeScript verb since before this cycle.
  Fixed to actually emit it.

**The corpus's own real gaps:**
- `extendedWaitUntil` → `wait until "text" appears`. A dropped custom `timeout` or `optional: true`
  gets a warning (`wait` has no per-step timeout or optional variant in ProbeScript).
- `scrollUntilVisible` → `scroll <direction>`, with a warning that it's an approximation (a single
  scroll, not "keep scrolling until visible") — `scroll <direction> <selector>` in ProbeScript picks
  *which scrollable to act on*, not a target to scroll toward, so passing the target through would
  have silently changed the step's meaning rather than approximating it honestly.
- `eraseText: N` → `clear`, with a warning that it's an approximation (empties the whole field
  rather than backspacing exactly N characters from the cursor) — close enough for the corpus's
  actual, real usage pattern (see below) but flagged for a manual check.

## A structural bug found independent of any single command: non-recursive directory discovery

`probe migrate maestro <dir>` used a single-level `os.ReadDir`, not a recursive walk. Maestro
suites organized into feature subdirectories — nect-flutter's own real suite is laid out exactly
this way (`flows/auth/`, `flows/settings/`, `flows/posts/`, ...) — got **zero files found and a
silent "No Maestro YAML files found." with no error**, not a partial conversion. Running the tool
against `nect-flutter/.maestro/flows/` before this fix reproduced exactly that. Fixed via
`filepath.WalkDir`, and output paths now mirror the source's subdirectory structure (previously
flattened to one directory, which would silently overwrite same-named files from different
subdirectories on collision — no collision existed in this corpus, but the risk was real and
free to close while already in this code).

## Full-corpus run

```
probe migrate maestro nect-flutter/.maestro/flows/ --output <dir>
```

**76/76 files converted, 0 hard errors, 331 warnings** (mostly `extendedWaitUntil`'s dropped
custom-timeout note — expected and correct, not a fault). Full log: `migrate_run.log`. Every
converted file: `nect-flutter-migrated-suite/`.

**Migration-fidelity check**: every one of the 76 generated `.probe` files was then run through
`probe test --dry-run` against this branch's own parser — **76/76 parsed as valid ProbeScript**,
zero syntax errors. This is the real bar for "hardened": not just that the converter emits *some*
output for every input, but that the output is actually valid ProbeScript a test author could open
and run.

Spot-checked outputs:
- `home/first-open-profile-prompt.probe` — three `tapOn` + `eraseText: 60` pairs (the corpus's own
  documented "guard against stale pre-fill" pattern) converted cleanly to three `tap on #x` /
  `clear` pairs.
- `settings/settings-about.probe` — `extendedWaitUntil: {visible: "ACCOUNT", timeout: 10000}`
  converted to `wait until "ACCOUNT" appears`, with a warning about the dropped timeout.
- `posts/posts-list-page-elements.probe` — the real `point: "50%,10%"` occurrence converted to a
  clear `# TODO` instead of the old code's silent `tap on "map[point:50%,10%]"` corruption.

## Not fixed / known remaining gaps

This pass targeted the roadmap's four named commands plus whatever the real corpus's own audit
surfaced — it is not a claim of 100% Maestro syntax coverage. `evalScript` (1 occurrence) still
requires manual Dart conversion, unchanged from before. Commands not present anywhere in this
corpus and not named in the roadmap were not audited.
