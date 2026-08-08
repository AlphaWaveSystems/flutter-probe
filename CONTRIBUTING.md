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
