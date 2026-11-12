# Contributing to FlutterProbe

Thank you for your interest in contributing to FlutterProbe! This guide explains how to get involved.

## Reporting Bugs

Open an issue on [GitHub Issues](https://github.com/AlphaWaveSystems/flutter-probe/issues) with:

- A clear, descriptive title
- Steps to reproduce the problem
- Expected vs actual behavior
- FlutterProbe version (`probe --version`), OS, Flutter version
- Relevant `.probe` file snippets or error output

## Pull Request Flow

1. **Fork** the repository on GitHub
2. **Branch** from `main` — use a descriptive name (e.g., `fix/token-timeout`, `feat/swipe-direction`)
3. **Develop** your changes following the code style guidelines below
4. **Test** your changes (see Testing section)
5. **Submit a PR** against `main` with a clear description of what and why

Keep PRs focused on a single concern. If you have multiple unrelated changes, submit separate PRs.

## Code Style

- **Go**: Run `gofmt` (or `goimports`) on all `.go` files before committing. Follow standard Go conventions.
- **Dart**: Run `dart format .` in the `probe_agent/` directory. Follow Effective Dart guidelines.
- **ProbeScript**: Use consistent 2-space indentation in `.probe` files.

## DCO Sign-Off

All commits must include a Developer Certificate of Origin sign-off line:

```
Signed-off-by: Your Name <your@email.com>
```

Add this automatically with:

```bash
git commit -s -m "Your commit message"
```

This certifies that you wrote or have the right to submit the code under the project's license.

## Testing

Before submitting a PR, run the full test suite:

```bash
make test                    # Go unit tests
cd probe_agent && flutter test  # Dart agent tests
```

For converter changes:

```bash
make test-convert            # Converter unit tests
make test-convert-integration  # Golden files + lint + dry-run
```

Ensure all tests pass and no regressions are introduced.

### Run the CI checks locally

You can run the CI checks locally, for example when hosted CI is unavailable:

```bash
make ci-local        # the five checks a pull request needs
make ci-local-all    # also the Dart agent, Studio, VS Code extension and website checks
```

`scripts/ci-local.sh` runs the commands of the workflow job behind each check: Build, Go unit tests, probe-convert tests, Lint and Disclosure guard. It prints one line per check, the path of the logs, and a final block with the commit, the tool versions and a last line such as `ci-local: PASS (5 of 5 required checks) at <sha>`. `--report FILE` writes that block as Markdown, ready to paste into a pull request.

- Commit first. The checks read the working tree, so a run with uncommitted changes to tracked files is marked `DIRTY TREE` and fails. Untracked files are counted in a note.
- `--job NAME` runs one check (repeatable) and `--list` shows the names. A run that leaves out a required check ends with `PARTIAL`, not `PASS`.
- A check whose tool is missing is `SKIPPED (tool X not installed)`. A skipped required check fails the run unless you pass `--allow-skip`.
- The Disclosure guard check needs the maintainers' pattern file, which is not in this repository. Maintainers set `PROBE_DISCLOSURE_DENYLIST` to its path. Without it the check reports `NOT RUN (no denylist)` and the run does not pass, as the hosted check does not pass without its patterns. `--base REF` sets the start of the scanned range (default `origin/main`, as last fetched), and `--pr-text FILE` adds the pull request title or body to the scan.
- `--release-dry-run` builds the `probe` and `probe-mcp` release binaries for the four release targets into a temporary directory and prints their sizes and checksums. It publishes nothing and creates no tag.

The script uses the tools on your machine, so the final block shows their versions next to the ones the workflows use. Lint installs the current `staticcheck` into the run's temporary directory, as the workflow installs it on the runner. A test (`scripts/ci_local_drift_test.go`, part of `make test`) fails when a workflow command and the script no longer match.

## ProbeScript Language Changes

If your change affects the ProbeScript language (new syntax, modified parsing):

1. Update the parser in `internal/parser/`
2. Add or update parser tests covering the new syntax
3. Add `.probe` test files in `tests/` demonstrating the new feature
4. Update golden files if converter output changes (`make update-golden`)
5. Run `make lint` to verify all `.probe` files still parse correctly

## API Stability & Deprecation Policy

`flutter_probe_agent`'s public Dart API (anything exported from
`package:flutter_probe_agent/flutter_probe_agent.dart`) is used directly by
downstream apps' test code, so removing or changing it breaks real projects
without warning if done carelessly. This happened once already: a minor
version bump reduced the public API surface down to just `ProbeAgent` and
`isProbeEnabled`, silently deleting a plugin-registration API
(`ProbePlugin`/`ProbePluginRegistry`) that at least one downstream project
had a load-bearing test-automation feature built on top of, with no
deprecation cycle and no CHANGELOG migration note beyond "reduced public
API."

To prevent a repeat, any removal or breaking change to the public API must
follow this sequence:

1. **Deprecate first.** Mark the old API `@Deprecated('...')` with a message
   naming the replacement, for at least one minor version before removal.
2. **Document the migration.** The CHANGELOG entry for the deprecation must
   name the replacement pattern explicitly — not just "deprecated X" or
   "reduced public API."
3. **Remove in a later minor version**, with its own CHANGELOG entry
   referencing the original deprecation.

If a use case the current public API supports (e.g. extending/customizing
agent behavior from the app side) is intentionally being dropped rather than
replaced, say so explicitly in the CHANGELOG and in the PR description —
don't let it read as an oversight.

`ProbePlugin`/`ProbePluginRegistry` specifically: whether that capability
should be reintroduced (under a deprecation-safe path this time) or the
removal should stand as a permanent, intentional scope decision is a product
call for the maintainer, not something this policy resolves on its own —
it's tracked as an open question in `IMPROVEMENT_TASKS.md` (PT-08) pending
that decision.

## Questions?

If you are unsure about anything, open an issue to discuss before starting work. We are happy to help guide contributions.
