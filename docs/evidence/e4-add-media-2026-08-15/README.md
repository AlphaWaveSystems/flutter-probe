# E-4 evidence — `add media "path"`

`add media "path/to/file.jpg"` seeds a local file into the device's camera roll/gallery —
`adb push` + `MEDIA_SCANNER_SCAN_FILE` broadcast on Android, `xcrun simctl addmedia` on iOS.
Unblocks image-picker-adjacent flows that need an existing photo to select. CLI-side only, no
Dart RPC. Matches Maestro's `addMedia`. Competitive roadmap Phase 1, E-4.

## Automated regression coverage

`internal/parser/parser_test.go`: `TestParser_AddMedia` (path capture), and
`TestParser_RecipeCallStartingWithAdd` — the same PT-23-style collision guard `open` already
needed: a recipe whose name happens to start with "add" (but isn't followed by "media") falls
through to a normal recipe call instead of being swallowed by the built-in verb. This works for
free here because `add`/`media` are matched as an exact two-word compound token in the lexer, not
via a keyword-then-lookahead like `open` needed — if the second word isn't "media", the lexer
never produces the compound token at all, so the parser's default case picks it up as a recipe
call without any special-casing.

`internal/runner/retry_optional_test.go`: `TestRunStep_AddMedia_CloudModeSkips` — cloud mode
(no `DeviceContext`, since this is CLI-side only) skips gracefully with a warning instead of
panicking on a nil `deviceCtx`.

Full `go test ./...` passes with no regressions.

## Real-device evidence (water-sip)

`e4_add_media_evidence.probe`, 1 test, run identically against both platforms. The test itself
only proves the command executes without error — the actual seeding is verified independently,
outside of probe, against real OS-level media stores, so the evidence isn't circular.

### Android emulator

```
✓  add media seeds a file into the gallery (5.65s)
```

Independently verified afterward (not through probe):

```
$ adb shell ls -la /sdcard/Pictures/ | grep e4_test_photo
-rw-rw---- 1 u0_a195 media_rw 321822 2026-08-15 06:37 e4_test_photo.jpg

$ adb shell content query --uri content://media/external/images/media --projection _data | grep e4_test_photo
Row: 49 _data=/storage/emulated/0/Pictures/e4_test_photo.jpg
```

The file is not just present on disk but **indexed in MediaStore** — the query any image
picker/gallery app uses — confirming it's genuinely visible to the app, not just sitting
unindexed in a directory.

### iOS simulator

```
✓  add media seeds a file into the gallery (5.419s)
```

Independently verified afterward (not through probe) by launching the built-in Photos app
(`com.apple.mobileslideshow`) and screenshotting it: the seeded file appears as the newest photo
in the Library (7 photos total, last one is the seeded file — visibly the same "Open in WaterSip"
dialog screenshot used as test input, converted to JPEG). See
`e4_ios_photos_app_verification.png`.

## Known limitation (not part of this feature, flagging for Phase 2)

`add media` only seeds the file into the OS media store — it does not, and currently *cannot*,
drive the app's own native image-picker UI to actually select that photo. Both water-sip
(no `image_picker` usage found) and nect-flutter (`image_picker` used in several screens) present
a native, OS-level picker sheet when an in-app "choose photo" button is tapped, and probe has no
way to interact with that native UI yet — driving it is exactly the gap Phase 2's N-1
(`tap native` / `see native` via `uiautomator`, Android) and N-2 (iOS native bridging proposal)
are scoped to close. Until then, `add media` is useful to pre-populate the gallery so a photo is
*available* to pick, but the actual pick-this-photo step in a test still needs to be worked
around (e.g. an app-specific test-only bypass) rather than driven through probe.
