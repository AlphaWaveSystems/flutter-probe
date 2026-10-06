# Optional local-LLM support (FP-14)

Status: implementing. Builds on `ai-visual-assertions-prd.md` (the `ai:` block and `provider: local`).

## Non-negotiable constraint: AI is optional, tests never depend on it

1. **Off by default.** No `ai:` block, or no `--ai-triage` / `probe ai ...` / `probe triage` invocation, means no AI
   code path runs and no network call is made.
2. **Never in the pass/fail path.** Everything new here runs *after* results are final, or in separate commands.
   The only AI that can influence a result is the pre-existing explicit `see "..." with ai` step, which is
   unchanged.
3. **Fail open.** Any AI failure (endpoint down, timeout, bad JSON, panic) degrades to a one-line note. It never
   changes the exit code, the report, or the number of passed/failed tests.
4. **Bounded.** Hard per-call timeout, a cap on how many failures are triaged, and no retries.
5. **Private.** Same rules as vision: `redact:` applies, `provider: local` sends nothing off the host.

These are enforced by tests (see "Verification").

## Findings that shape the work

- `with ai` assertions already support `provider: local` (OpenAI-compatible endpoint, no key).
- `probe generate` / AI selector suggestions use a *separate* Anthropic-only `Generator`; `provider: local` is
  ignored there.
- "Self-healing" (`selfheal.go`) is deterministic fuzzy matching, not an LLM.
- Many small local models have **no vision**; today `with ai` simply fails for them.

## Work items

| # | Item | Notes |
|---|---|---|
| A | `TextCompleter` + OpenAI-compatible text client | One small interface; Anthropic and OpenAI-compatible implementations. |
| B | `probe generate` / `SuggestSelector` honour `ai.provider` | Anthropic stays the default when no provider is set, so existing setups are unchanged. |
| C | `probe ai doctor` | Checks endpoint, model listed, a text round-trip and a 1x1-image vision probe. Prints what works. Exit code reflects only the doctor, never tests. |
| D | Failure triage | `probe test --ai-triage` (opt-in) and `probe triage --input results.json`. Advisory paragraph per failure from the error + visible texts. Written to stdout and `<reports>/triage.md`. |
| E | Text-only `with ai` | `ai.vision: false` evaluates `see "..." with ai` against the screen's visible texts/keys instead of a screenshot, for non-vision local models. |
| F | MCP `triage_failure` tool | Wraps D. |
| G | Docs | `docs/`, website AI page, MCP docs, CHANGELOG. |

## Verification

- Unit tests: client parsing, provider selection, fail-open behaviour (500s, timeouts, garbage, panics).
- Contract tests: running with a dead `ai.endpoint` and `--ai-triage` produces byte-identical results and exit
  code to running without it, and a run without any `ai:` config never constructs a client.
- Live check against a real local server if one is reachable (LM Studio / Ollama), otherwise stated as unverified.

## Out of scope

Autonomous test repair that edits `.probe` files, any hosted FlutterProbe relay, and making AI a required
dependency of any command that runs tests.
