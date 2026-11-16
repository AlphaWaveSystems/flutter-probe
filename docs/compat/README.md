# Flutter and Dart compatibility

FlutterProbe drives an app through Flutter's widget tree and semantics. A new Flutter or Dart release can change widgets, gestures, text input,
rendering or accessibility, and either break the agent or leave new components unsupported. This folder is how that is kept in view.

| File | What it is | Who writes it |
|---|---|---|
| `catalog.json` | Every public widget of Flutter's `widgets`, `material` and `cupertino` libraries, stamped with the Flutter and Dart versions it was made from | generated, never edited by hand |
| `MATRIX.md` | For each widget family, how FlutterProbe finds, taps, types into, scrolls and reads it, and whether a fixture screen and a real-device test cover it | by hand, only from tests that ran |
| `RELEASES.md` | A log of each stable Flutter and Dart release: what affects FlutterProbe, the catalog changes, what was done | by hand |

## The rules

1. **Catalog.** `catalog.json` is produced by `tools/flutter-catalog` from an installed SDK and records names only: the class name, the library, the
   direct superclass, and whether the class is abstract or marked deprecated. No documentation text is copied from Flutter's sources.
2. **Matrix.** A cell says "not assessed" until a test covers it. A widget FlutterProbe cannot work with is listed as unsupported, never left out. No cell
   claims support that no test backs.
3. **Every change** to widget lookup, tap, type, scroll or read code (the agent, the runner, the device layer) updates the matrix row of what it touches
   and adds or adjusts a fixture test.
4. **Every stable release** of Flutter, the Dart SDK that ships with it, and each tool FlutterProbe depends on: read its release notes and breaking-change
   list, add an entry to `RELEASES.md` (version, date, what changed, what was done or must be done), regenerate `catalog.json` and review the diff
   (added, removed, renamed and deprecated widgets), and run the fixture suite on that version. Dart language changes and deprecations go in the same log.
5. **No silent drift.** `.github/workflows/compat-drift.yml` runs every Monday at 06:00 UTC, and on demand. It reads Flutter's official release manifest for the
   newest stable release and fails when that release is not the newest entry in `RELEASES.md`. It also installs that release and runs
   `tools/flutter-catalog -check` against it. A failure opens or updates one open issue labelled `compat-drift`, and the run fails. The check does not run
   the fixture suite. Whoever touches the repo and sees a newer stable release still records it in `RELEASES.md` before finishing.

## The test app follows the catalog

The Flutter fixture app (`native-test-apps/flutter`) is where the matrix gets its results. A widget family gets (1) a screen, selected by `--dart-define=PROBE_FIXTURE_WIDGETS=<family>` (`lib/widget_families.dart`), whose elements
carry a semantics identifier and a key of the same name and show their state as text; (2) a ProbeScript test in `probe-tests/widgets/`; (3) its rows in `MATRIX.md`, set from a run of that test on a named device and Flutter version.
When a new stable Flutter changes the catalog (an added, removed, renamed or deprecated widget), the families that the change touches are checked against the diff: a new widget of a family gets an element on that family's
screen, a removed or renamed one is taken off it, and the test is run again on the new release. A family with no screen yet stays "not assessed".

## When a new stable Flutter comes out

1. Get that SDK from the official source only, and check it: the release manifest (`https://storage.googleapis.com/flutter_infra_release/releases/releases_macos.json`) names the stable release and its
   revision; a clone of the release tag of `https://github.com/flutter/flutter` must have exactly that commit. Run `flutter --version` once with analytics off, and use a scratch `HOME` and `PUB_CACHE` if
   the machine's own Flutter must stay untouched. Never run `flutter upgrade` on an SDK that is not meant to change.
2. Regenerate the catalog from it (below) and read the diff: `git diff docs/compat/catalog.json` shows added and removed lines (a changed flag is a changed line). Write the added, removed, renamed and
   deprecated widgets in the new `RELEASES.md` entry, with what they mean for a family in `MATRIX.md`.
3. Read the release notes of the release and the Dart changelog, and the tool's requirements (`packages/flutter_tools/lib/src/android/gradle_utils.dart`, `packages/flutter_tools/gradle/.../DependencyVersionChecker.kt`,
   `packages/flutter_tools/lib/src/macos/xcode.dart`). Record what concerns FlutterProbe, with the pull-request numbers; where a page gives no date or no requirement, say so.
4. Run the fixture suite on that release on a named device and record the result; raise the fixtures' Gradle, AGP and Kotlin versions when the tool's new minimums need it.
5. Put the new entry at the top of `RELEASES.md` (the weekly check reads the first `## Flutter <version>` heading).

The catalog stamp is the release (Flutter version, framework revision, Dart version). It does **not** record the channel: that says how an SDK was installed (a clone of a tag reports `[user-branch]`, an installed
stable release `stable`), not which release it is, and two installs of one release must give the same file for `-check` to mean anything.

## Regenerating the catalog

The tool only reads the SDK folder (it never writes there, runs nothing from it and uses no network). Use the SDK you want to record:

```bash
go run ./tools/flutter-catalog -sdk /path/to/flutter            # writes docs/compat/catalog.json
go run ./tools/flutter-catalog -sdk /path/to/flutter -check     # writes nothing; exit 1 when the file differs
```

`-sdk` defaults to `$FLUTTER_ROOT`. The output is sorted and has no timestamps, so a second run on the same SDK gives the same bytes.
Its tests run against a small made-up SDK tree in `tools/flutter-catalog/testdata/sdk` (not Flutter source), so they need no Flutter install;
`PROBE_FLUTTER_SDK=/path/to/flutter go test ./tools/flutter-catalog` also runs a check on a real one.

## What a "widget" is here

A public class (no leading underscore) that is exported by the library's own file under `src/<library>/` (honouring `show` and `hide`) and whose chain
of superclasses reaches `Widget`. A widget belongs to the first of `widgets`, `material`, `cupertino` whose folder holds it. Render objects, states,
controllers and other helper classes are not widgets and are not listed.
