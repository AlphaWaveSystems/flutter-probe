# scripts

Maintainer scripts for the FlutterProbe repository. None of them are needed to *use* FlutterProbe.

| Script | Purpose |
|---|---|
| `release.sh <version>` | Release helper: `./scripts/release.sh 0.15.0`. See the release checklist in `CONTRIBUTING.md` and `CHANGELOG.md`. |
| `test-all.sh [--quick]` | Builds `probe` and `probe-convert`, runs the Go unit tests with `-race`, lint, and the `probe-convert` converter tests. `--quick` skips integration tests that need a build. The Dart agent tests run separately: `cd probe_agent && flutter test`. |
| `build-mcpb.sh <binary> <platform> <version> <out-dir>` | Builds the Claude Desktop Extension (`.mcpb`) bundle for `probe-mcp` from `mcpb/template`. Run by CI on release. |
| `setup-wiki.sh` | One-time: initializes the GitHub Wiki from `docs/wiki/`. Needs an authenticated `gh` with write access. |
| `bench/` | Comparison benchmarks (`run-comparison.sh`, `summarize.py`). |
