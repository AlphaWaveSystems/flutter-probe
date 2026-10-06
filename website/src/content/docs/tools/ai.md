---
title: AI & Local Models
description: Optional AI helpers for FlutterProbe — assertions, failure triage and test generation with a cloud model or a small local one. Never required to run tests.
---

FlutterProbe can use a language model for a few things. **All of it is optional and none of it is on the
path that decides whether a test passes.** With no `ai:` block in `probe.yaml`, no AI code runs and nothing
is sent anywhere.

## The guarantees

- **Off by default.** No `ai:` block, no flag, no AI.
- **Tests never depend on it.** `probe test` runs, reports and exits the same with or without a model. The
  only AI that can affect a result is a step *you* wrote: `see "..." with ai`.
- **Fails open.** If the model is down, slow or answers garbage, triage and doctor print a one-line note.
  Results, report files and the exit code are untouched.
- **Bounded.** Every call has a timeout, there are no retries, at most 5 failures are triaged, and triage
  stops after the first provider error instead of calling a dead endpoint once per failure.
- **Private by choice.** With `provider: local`, nothing leaves your machine. `redact:` rules black out
  screenshot regions before any provider sees them.

## Use a local model

Any OpenAI-compatible server works: Ollama, LM Studio, llama.cpp's server.

```yaml
ai:
  provider: local
  endpoint: http://localhost:11434/v1   # Ollama; LM Studio is http://localhost:1234/v1
  model: gemma4:31b
  timeout: 120s                         # local models can be slow
  # vision: false                       # set for a text-only model, see below
```

No API key is needed. Check it:

```bash
probe ai doctor
```

```
✓ config: provider local, model gemma4:31b
✓ endpoint: reachable, model "gemma4:31b" is available
✓ text: answered in 9.6s: "pong"
✓ vision: the model accepts images — `with ai` screenshot assertions will work
```

The doctor reports whether the model accepts images. A text-only model is a supported setup.

## What you can do with it

| Feature | Command | Needs vision |
|---|---|---|
| Explain failures | `probe test --ai-triage`, `probe triage --input reports/results.json`, MCP `triage_failure` | no |
| Generate tests | `probe generate prompt -p "..."`, `probe generate from-recording ...` | no |
| Screen assertions | `see "the total looks correct" with ai` | yes, or text mode |
| Visual smoke check | `assert no visual defects with ai` | yes |
| Read text off screen | `read "the OTP" with ai into otp` | yes |

### Failure triage

```bash
probe test tests/ --ai-triage
```

After the results are final, the model gets each failure's error text and writes a short probable cause and
a next step, printed and saved to `<reports>/triage.md`. Since 0.15.0 failure text names the line, the step
and the visible texts/keys on screen, which is what the model reasons over. Triage is advice. It does not
re-run anything or change a result.

Quality depends on the model: a 0.5B model will guess wrong; a 7B-30B model is useful. Treat the output as
a hint.

When failures come from a CI run, triage the saved report later:

```bash
probe test tests/ --format json -o reports/results.json
probe triage --input reports/results.json -o reports/triage.md
```

### Text-only models (`vision: false`)

Most small local models cannot see images. With `ai.vision: false`, `see "..." with ai` is judged from the
visible texts and widget keys the agent reports instead of a screenshot. Pixel-based steps
(`assert no visual defects`, `read ... with ai`) fail with a clear message rather than guessing.

Text mode is refused when `ai.redact` rules exist, because redaction works on screenshot regions and cannot
be applied to a list of texts. It needs `flutter_probe_agent` 0.15.0 or newer.

### Test generation

`probe generate` uses the `ai:` provider when one is set (including `local`). With no `ai.provider` it keeps
its original behavior: Claude with `--api-key` / `ai.api_key`.

## Cloud providers and privacy

`provider: openai` and `provider: anthropic` send the request to that vendor directly, never through a
FlutterProbe service. Triage and text mode send error text and on-screen text, which can contain user data;
use `provider: local` if that matters.

See [Configuration](/advanced/configuration/#ai) for every `ai:` key.
