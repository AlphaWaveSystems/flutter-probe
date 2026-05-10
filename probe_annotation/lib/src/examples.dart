/// Data-driven test examples (rows of variable substitutions).
library;

/// A list of named columns and rows. Used by [ProbeTest.examples] to expand
/// a test into one execution per row, with `<column>` references in step
/// arguments substituted to the row's value.
///
/// Emits an `examples:` table after the test body:
///
/// ```
/// examples:
///   | email          | result    |
///   | alice@a.com    | Dashboard |
///   | bob@b.com      | Error     |
/// ```
class Examples {
  final List<String> headers;
  final List<List<String>> rows;
  final String? source;

  /// Inline examples: provide [headers] and [rows] directly.
  const Examples({required this.headers, required this.rows}) : source = null;

  /// CSV-backed examples: emits `examples: from "csvPath"`.
  const Examples.from(String csvPath)
      : headers = const [],
        rows = const [],
        source = csvPath;
}
