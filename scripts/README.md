# scripts

Maintainer scripts for the FlutterProbe repository. None of them are needed to *use* FlutterProbe.

| Script | Purpose |
|---|---|
| `release.sh <version>` | Release helper: `./scripts/release.sh 0.15.0`. Only bumps `VERSION`, the agent pubspec and the VS Code package, then commits and tags: the rest of the checklist is in `docs/ops/release-pipeline.md` ("Before a release"). |
| `ci-local.sh [--all] [--job NAME] [--list] [--base REF] [--pr-text FILE] [--allow-skip] [--report FILE] [--release-dry-run]` | Runs the CI checks locally, for example when hosted CI is unavailable: the five checks a pull request needs, each with the commands of its workflow job. `--all` adds the Dart agent, Studio, VS Code extension and website checks. `--pr-text FILE` adds the pull request title or body to the Disclosure guard scan (repeatable). A required check whose tool is missing fails the run; `--allow-skip` ends it as `PARTIAL` instead. Exit status: 0 `PASS` (all five required checks ran and passed on a clean tree), 1 a check failed or was not run, 2 usage error, 3 a required check was skipped, 4 uncommitted changes to tracked files, 5 `PARTIAL` (a `--job` selection or an allowed skip). Also `make ci-local` and `make ci-local-all`. See "Run the CI checks locally" in `CONTRIBUTING.md`. |
| `test-all.sh [--quick]` | Builds `probe` and `probe-convert`, runs the Go unit tests with `-race`, lint, and the `probe-convert` converter tests. `--quick` skips integration tests that need a build. The Dart agent tests run separately: `cd probe_agent && flutter test`. |
| `build-mcpb.sh <binary> <platform> <version> <out-dir>` | Builds the Claude Desktop Extension (`.mcpb`) bundle for `probe-mcp` from `mcpb/template`. Run by CI on release. |
| `setup-wiki.sh` | One-time: initializes the GitHub Wiki from `docs/wiki/`. Needs an authenticated `gh` with write access. |
| `bench/` | Comparison benchmarks (`run-comparison.sh`, `summarize.py`). |
