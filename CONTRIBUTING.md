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

- Commit first. The checks read the working tree, so the tree is read with `git status` before the first check, and the run is marked `DIRTY TREE` and fails when the tree is not the commit: a staged or unstaged change to a tracked file, an untracked file that git does not ignore, or a file hidden with `--assume-unchanged` or `--skip-worktree`. The tree is read again after the last check: a check that changed a tracked file also makes the run a `DIRTY TREE`. `git status` runs with `--untracked-files=normal` and `--ignore-submodules=none`, so your git configuration cannot hide a file. Files that git ignores (`.gitignore`, `.git/info/exclude`, `core.excludesFile`) do not count, by design. Untracked files the run itself creates do not count; they are named in a note. The Go tools write `go.work.sum` during a build; when the file was not there before the run and git does not track it, the script removes it again.
- `--job NAME` runs one check (repeatable) and `--list` shows the names. A run that leaves out a required check ends with `PARTIAL`, not `PASS`.
- A check whose tool is missing is `SKIPPED (tool X not installed)`. A skipped required check fails the run; with `--allow-skip` the run ends with `PARTIAL` instead.
- For a run of the checks, the last line says `PASS`, and the exit status is 0, only when all five required checks ran and passed on a clean tree. Exit status 1 is a failed check or one that was not run, 2 a usage error or a run that could not start (`git status` failed, the `--report` file cannot be written), 3 a skipped required check, 4 a dirty tree, 5 `PARTIAL` (a `--job` selection, an allowed skip, a requested optional check that was skipped because its tool is missing, or `GOFLAGS` set), and 130 an interrupted run, which ends with `FAIL (interrupted)`. A run with `GOFLAGS` set ends with `PARTIAL`, because `GOFLAGS` changes what the Go commands do. With `--all`, an optional check that is skipped because its tool is missing also ends the run with `PARTIAL`, naming the skipped checks. The script removes the exported shell functions of your shell before it runs, so that a function named like a tool cannot stand in for it, and says how many it removed; the `GIT_*` variables that redirect git (`GIT_DIR`, `GIT_WORK_TREE`, `GIT_INDEX_FILE`, `GIT_CONFIG_*`, ...) are removed the same way. A hostile environment is out of scope: overriding a shell builtin (`compgen`, `unset`), aliases set through `BASH_ENV`, and a changed `PATH` are not defended against; the final block shows `GOFLAGS` and `GOTOOLCHAIN` when they are set. `--help` lists the exit statuses.
- The Disclosure guard check needs the maintainers' pattern file, which is not in this repository. Maintainers set `PROBE_DISCLOSURE_DENYLIST` to its path. Without it the check reports `NOT RUN (no denylist)` and the run does not pass, as the hosted check does not pass without its patterns. `--base REF` sets the start of the scanned range (default `origin/main`, as last fetched; a ref with a character other than letters, digits and `. _ / ~ ^ @ { } : + , -` is refused, because the ref is written on the last line); another base is named in the last line. `--report FILE` refuses, with exit status 2 and before anything is written, a file that git tracks, any path inside a `.git` directory, `go.work.sum`, a symbolic link, and an existing file with more than one hard link (it may be a tracked file under another name). Other paths are not checked: the script overwrites the file you name. When the base is `HEAD` or already contains it, there is nothing to scan: the check reports `NOT RUN (empty range)` and the run does not pass. That is also the result on `main` right after a merge. `--pr-text FILE` adds the pull request title or body to the scan. The hosted check always scans the title and body; a local run without `--pr-text` says in a note that it did not.
- `--release-dry-run` builds the `probe` and `probe-mcp` release binaries for the four release targets into a temporary directory and prints their sizes and checksums. It publishes nothing and creates no tag. It runs no checks and never prints `PASS`: its last line is `release-dry-run: BUILT 8 binaries at <sha> (no checks run)`. It cannot be combined with `--all`, `--job`, `--allow-skip`, `--base`, `--report` or `--pr-text`. Its exit status is 0 when the eight binaries were built from a clean tree, 1 when a build failed, 2 for a usage error and 4 for a dirty tree.

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
