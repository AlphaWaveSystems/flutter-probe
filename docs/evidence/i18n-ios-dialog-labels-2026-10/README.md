# iOS system alert button labels, measured per language (2026-10-09)

Raw measurements behind `internal/sysdialog/labels_ios.go`.

## Method

- Device: iPhone 17e simulator, iOS 26.3 runtime. Nothing else was touched.
- Per language: boot, `simctl spawn defaults write -g AppleLanguages -array <lang>` and
  `AppleLocale`, shut down, boot again, read both values back (the first lines of
  each file), then trigger alerts.
- Alerts are triggered by a throwaway app (not part of this repo) that requests one
  permission per launch. Each request starts from `simctl privacy reset all` plus an
  uninstall and reinstall of the app, because a previously answered permission is not
  asked again.
- Buttons are read with the repo's driver: `probe system-dialog list`, against one
  `xcodebuild test-without-building` runner kept alive for the whole language
  (port 48991). Labels are recorded exactly as returned, including U+2019.
- One file per language (`<lang>.txt`) with the commands and the driver output.
  Lines starting with `!!` would flag extra stacked dialogs; there are none.

## Measured (27 languages)

en, de, fr, es, it, pt-BR, nl, sv, da, nb, fi, pl, cs, tr, ru, uk, el, he, ar, hi, th,
vi, id, ja, ko, zh-Hans, zh-Hant. AppleLocale used: the matching `xx_XX` (en_US, de_DE,
fr_FR, es_ES, it_IT, pt_BR, nl_NL, sv_SE, da_DK, nb_NO, fi_FI, pl_PL, cs_CZ, tr_TR,
ru_RU, uk_UA, el_GR, he_IL, ar_SA, hi_IN, th_TH, vi_VN, id_ID, ja_JP, ko_KR, zh_CN, zh_TW).

Alerts observed in every language: notification, location (when in use), camera,
microphone, reminders, speech recognition, motion and fitness.

| Role | Observed on |
| --- | --- |
| allow | notification, camera, microphone, reminders, speech, motion (second button) |
| deny | the same alerts (first button) and the location alert (last button) |
| allow_once | location alert, first button |
| allow_while_using | location alert, second button |

Some languages use different spellings for deny between alert types (for example cs
"Zakázat" on notifications vs "Nepovolovat" elsewhere, ru "Запретить" on location vs
"Не разрешать" elsewhere, el "Όχι" on location); all spellings are in the table.
Hebrew renders allow as "אישור" and Greek location "while using" as the shorter
"Κατά τη χρήση της εφαρμογής".

## Not observable (marked missing, not guessed)

- **ok, cancel, open**: none of the alerts above has an OK, Cancel or Open button on
  this simulator. A local-network alert (which has an OK-style button on devices) was
  requested but never appeared on the simulator. The "Open in <app>?" confirmation
  could not be triggered: `simctl openurl` for tel:, mailto:, facetime: and itms-apps:
  failed with LSApplicationWorkspaceErrorDomain 115 (no handler), sms: and maps:
  showed no dialog.
- **Location "Always" upgrade alert**: requesting Always authorization showed the
  same three-button when-in-use alert, so nothing new was recorded.
- **Rich-preview alerts (photos, calendar full access, contacts)**: seen in English
  only during probing ("Limit Access…" / "Allow Full Access" / "Don’t Allow", and
  "Allow Full Access" / "Don’t Allow"). The driver's tap did not dismiss them
  ("tapped ... but the dialog is still showing"), which blocked later requests, so
  they were left out of the per-language runs and are not in the table. That is a
  driver limitation worth a separate look.
- The Maps-launch route for the location alert did not work: Location Services was
  off on this simulator, so a throwaway app request was used instead.
- Only the iOS 26.3 runtime was measured; other iOS versions may word labels
  differently.
