# Contributing to FlutterProbe

Thank you for your interest in contributing to FlutterProbe! This guide explains how to get involved.

## Reporting Bugs

Open an issue on [GitHub Issues](https://github.com/nicklaus-dev/FlutterProbe/issues) with:

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

## Questions?

If you are unsure about anything, open an issue to discuss before starting work. We are happy to help guide contributions.
