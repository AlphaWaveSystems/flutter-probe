---
title: Testing localized apps
description: Write one FlutterProbe suite that runs in every app language and on devices set to any language - selectors, loose text matching, alternatives and system dialogs.
---

FlutterProbe tests are written once and have to survive translation. This page is the playbook, in the order of what to
reach for first.

## 1. Select by key, not by label

`#key` selectors (a `ValueKey`, a `Semantics(identifier:)`) and widget types do not change with the language:

```probe
test "save"
  tap #save_button
  see #saved_banner
```

Use visible text only when you really want to check what the user reads.

## 2. Loose text matching

Text selectors are a case-sensitive substring by default. Run with `--match-loose` (or `defaults.match: loose` in
`probe.yaml`) to compare after folding **case, accents and other diacritics, typographic apostrophes and dashes,
full-width forms, invisible characters and whitespace**, on both sides:

| Script says | Screen shows | Match |
|---|---|---|
| `anderungen SPEICHERN` | `Änderungen speichern` | yes (loose) |
| `don't allow` | `Don’t Allow` | yes (loose) |
| `cafe creme` | `Café Crème` | yes (loose) |
| `Save` | `save` | yes (loose), no (exact) |

Marks that carry meaning (Indic vowel signs, Thai tone marks) are kept, so words in those scripts are not merged.
`#key` selectors are never folded.

## 3. Alternatives

When the label differs per language, list them: `see any of "Save", "Speichern", "Guardar"`,
`wait until any of "Welcome", "Willkommen"`. For exact text use `see "0 ml" matching "^0 ml$"`.

## 4. System dialogs in any device language

Permission alerts and other OS dialogs are drawn in the **device** language. A button is resolved by its **role**
(allow, deny, allow once, allow while using, OK, cancel, not now, open, close), so the script you write in English works on a
German or Japanese device:

```probe
tap "Don't Allow" in system dialog      # finds "Nicht erlauben", "許可しない", ...
deny permission "notifications"
dismiss system dialog                    # the cancel-like button in the device's language
```

On Android the roles come from the permission dialog's language-independent resource ids; on iOS from a table of labels measured
on real simulators for each language (`docs/evidence/i18n-ios-dialog-labels-*`). Measured for allow, deny, allow once and allow while using in 27 languages: English, German, French, Spanish, Italian, Portuguese (BR), Dutch, Swedish, Danish, Norwegian (Bokmål), Finnish, Polish, Czech, Turkish, Russian, Ukrainian, Greek, Hebrew, Arabic, Hindi, Thai, Vietnamese, Indonesian, Japanese, Korean, Chinese (Simplified and Traditional). `ok`, `cancel` and `open` labels outside English are not measured yet (use the exact label, or `dismiss system dialog`). An exact label of the device always wins over a role.

## Changing the app language

```
set language "de"        # relaunch the app in German
see "Einstellungen"
set language "ar"        # right-to-left: the layout flips with the locale
set language "system"    # remove the override again
```

or for a whole run:

```bash
probe test tests/ --locale de-DE            # one language
probe test tests/ --locales de,ja,ar -o reports/r.json   # the suite once per language
```

`--locales` starts one `probe test` per language (fresh app launch and agent connection each), prints a pass/fail table and writes
`reports/r.de.json`, `reports/r.ja.json`, ... — each report records its language. A failing language fails the command.

How it works: Android 13+ (API 33) uses per-app locales (`cmd locale set-app-locales`); an iOS simulator gets
`-AppleLanguages`/`-AppleLocale` launch arguments on every launch of the app. Only the **app** language changes: the device language
stays, so system dialogs keep their labels — which is why dialog buttons are matched by role. Physical iOS devices are not supported
(change the language in Settings > Apps > your app).

## What is not covered yet

See the plan in `docs/proposals/i18n-multi-language.md`: resolving labels from your ARB files (`l10n "key"`) and testing against
backend responses are on the roadmap.
