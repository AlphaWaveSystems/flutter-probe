/// Folds text for loose, language-tolerant comparison: case, accents, typographic apostrophes/quotes/
/// dashes, full-width forms, invisible characters and whitespace. Mirror of the Go package
/// internal/textfold; both are tested against internal/textfold/testdata/cases.json.
library;

const _replacements = <String, String>{
  '’': "'", '‘': "'", 'ʼ': "'", '′': "'", '`': "'",
  '“': '"', '”': '"', '„': '"', '«': '"', '»': '"',
  '‐': '-', '‑': '-', '‒': '-', '–': '-', '—': '-', '−': '-',
  ' ': ' ', ' ': ' ', ' ': ' ', '　': ' ',
  '​': '', '‌': '', '‍': '', '﻿': '', '­': '', '‎': '', '‏': '',
  'ß': 'ss', 'ẞ': 'ss', 'æ': 'ae', 'Æ': 'ae', 'œ': 'oe', 'Œ': 'oe',
  'ø': 'o', 'Ø': 'o', 'ł': 'l', 'Ł': 'l', 'đ': 'd', 'Đ': 'd', 'ð': 'd', 'Ð': 'd', 'þ': 'th', 'Þ': 'th',
  'ı': 'i', 'İ': 'i',
};

const _accentGroups = <String, String>{
  'a': 'àáâãäåāăąǎǟǡǻȁȃȧ', 'c': 'çćĉċč', 'd': 'ď', 'e': 'èéêëēĕėęěȅȇȩ',
  'g': 'ĝğġģǧǵ', 'h': 'ĥ', 'i': 'ìíîïĩīĭįǐȉȋ', 'j': 'ĵ', 'k': 'ķǩ', 'l': 'ĺļľ',
  'n': 'ñńņňǹ', 'o': 'òóôõöōŏőǒǫȍȏȫȭȯȱ', 'r': 'ŕŗřȑȓ', 's': 'śŝşšș',
  't': 'ţťț', 'u': 'ùúûüũūŭůűųǔǖǘǚǜȕȗ', 'w': 'ŵ', 'y': 'ýÿŷȳ', 'z': 'źżž',
};

final Map<int, String> _accents = () {
  final m = <int, String>{};
  _accentGroups.forEach((base, letters) {
    for (final r in letters.runes) {
      final ch = String.fromCharCode(r);
      m[r] = base;
      m[ch.toUpperCase().runes.first] = base;
    }
  });
  return m;
}();

bool _isAccentMark(int r) =>
    (r >= 0x0300 && r <= 0x036F) ||
    (r >= 0x1AB0 && r <= 0x1AFF) ||
    (r >= 0x1DC0 && r <= 0x1DFF) ||
    (r >= 0x20D0 && r <= 0x20FF) ||
    (r >= 0xFE20 && r <= 0xFE2F) ||
    (r >= 0x064B && r <= 0x065F) ||
    r == 0x0670 ||
    (r >= 0x05B0 && r <= 0x05C7);

String foldText(String input) {
  var s = input;
  _replacements.forEach((from, to) => s = s.replaceAll(from, to));
  final out = StringBuffer();
  for (var r in s.runes) {
    if (r >= 0xFF01 && r <= 0xFF5E) r -= 0xFEE0; // full-width ASCII
    final base = _accents[r];
    if (base != null) {
      out.write(base);
      continue;
    }
    if (_isAccentMark(r)) continue;
    out.write(String.fromCharCode(r).toLowerCase());
  }
  return out.toString().split(RegExp(r'\s+')).where((p) => p.isNotEmpty).join(' ');
}

bool foldedEquals(String a, String b) => foldText(a) == foldText(b);
bool foldedContains(String s, String sub) => foldText(s).contains(foldText(sub));
