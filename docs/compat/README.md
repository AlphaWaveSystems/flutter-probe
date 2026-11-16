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
