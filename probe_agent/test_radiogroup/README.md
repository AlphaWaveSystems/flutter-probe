# test_radiogroup

Tests that need Flutter's real `RadioGroup` (Flutter 3.35 or newer). They are not under `test/` because `flutter test` there also runs on the older Flutter that CI pins, where `RadioGroup` does not exist and
the file would not compile. The same behaviour with a stand-in group is in `test/checked_state_radio_test.dart`, which runs everywhere.

Run them by hand, on a Flutter that has `RadioGroup`:

```bash
cd probe_agent
flutter test test_radiogroup
```

Run them whenever the agent's radio reading (`_radioChecked` in `lib/src/executor.dart`) changes and when a new stable Flutter comes out (`docs/compat/README.md`).
