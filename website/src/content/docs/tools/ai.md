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
- **Fails open.** If the model is down, slow or answers garbage, triage, selector proposals and doctor print a one-line note.
  Results, report files and the exit code are untouched.
- **Bounded.** Every call has a timeout, there are no retries, at most 5 failures are triaged, and triage
  stops after the first provider error instead of calling a dead endpoint once per failure. Selector
  proposals make at most 3 requests per run.
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

## In Studio

Studio's AI chat reads the same `ai:` block as the CLI, and these rules apply to it:

- **Keys.** Save the key in the keychain from the AI panel, or write `api_key: ${NAME}` and set that variable in the environment that starts Studio. A plain key in `probe.yaml` is not used, and Studio says so. A saved keychain key goes only to the Anthropic provider, never to a local endpoint.
- **Local endpoints.** `provider: local` must name `localhost` or a loopback address. Any other address is refused unless `studio.ai_allow_remote_endpoint: true` is set. An address with a user name or password is refused, and only `http` and `https` are accepted.
- **Off.** With no `ai:` block and no saved key, AI is off, and the chat sends nothing. `provider: none` turns AI off even when a key is saved.
- **Redirects.** Provider requests do not follow a redirect, so a prompt is never sent to a second host.
- **Redaction.** Every system prompt and chat message is redacted before it leaves. The families are API keys of common providers (OpenAI and Anthropic style `sk-`, GitHub, AWS, Slack, Google and Stripe), PEM private keys, `Authorization: Basic` values, assignments whose name says secret, token, password or key, and secret query parameters such as `?token=`. Emails and file paths are not redacted.

## What you can do with it

| Feature | Command | Needs vision |
|---|---|---|
| Explain failures | `probe test --ai-triage`, `probe triage --input reports/results.json`, MCP `triage_failure` | no |
| Propose a selector for a failed step | `probe test --ai-heal --format json` | no |
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

### Selector proposals for failed steps

```bash
probe test tests/ --ai-heal --format json -o reports/results.json
```

When a step runs into its timeout on a text or key selector (`tap "Save"`, `see #total`), `probe` records
the texts and widget keys that were visible on screen at that moment. The model is asked for one replacement
from them, and the failed result in the JSON report gets a `selector_proposal`:

```json
"selector_proposal": {
  "advisory": "Proposed by a language model from the failure text. Not applied to the test file and not verified against the app. It does not change this result.",
  "line": 9,
  "failed_selector": "\"Save\"",
  "proposed_selector": "\"Save changes\"",
  "applied": false,
  "verified": false,
  "model": "qwen2.5-coder:7b",
  "provider": "local",
  "location": "local"
}
```

What it does and does not do:

- **The step stays failed.** The results, the summary and the exit code are the same when the model
  answers, fails or times out. Nothing is written to the `.probe` file, and the proposal is not tried
  against the app. You decide whether to edit the test.
- **Only what was recorded as on screen.** When a step runs into its timeout, `probe` keeps the visible
  texts and widget keys as two lists of data. A proposal is kept only when it is exactly one of those texts,
  written as `"text"`, or exactly one of those keys, written as `#key`. A part of a listed text, a text
  offered as a key, a key offered as a text, or any other word of the failure text is not kept, and the
  entry gets an `unavailable` reason with no `proposed_selector`.
- **Only step timeouts, and no list is read from the failure text.** The lists are recorded when the step
  runs into its timeout. They are never read back out of the failure text, because the text on screen is
  part of it and could be written to look like a list. The app's own "Widget not found" message also lists
  the visible texts, but it is plain text that a widget can shape, so it is not used: a failure that is not
  a step timeout is not sent to the model, and its entry says so in `unavailable`. The failure text keeps at
  most 40 characters of each listed text, and a text over that length is never proposed, even though the
  full text was on screen. When every listed item is that long, the `unavailable` reason says so.
- **Bounded.** At most one request per failed step, at most 3 requests per run, each limited by
  `ai.timeout`, no retries, none after a failed request, and two minutes for all of them together. Model
  text is cut to 200 bytes, the API key is removed from it, and control characters, line and paragraph
  separators and bidirectional controls in it are replaced by spaces before it is printed or written to
  the report. The file name and the selector in the terminal line get the same treatment.
- **What is sent.** The failed step, its selector, the failure text and the two lists, with resolved
  environment values masked. The failure text is what `--ai-triage` sends. No screenshot is sent.
- **Where it shows.** The JSON report and one line per proposal in the terminal. `location` is `local` for
  `provider: local` on this machine, `network` for `provider: local` on another host, and `hosted` for
  `openai` and `anthropic`. Proposals are left out of an uploaded report. JUnit and HTML reports do not
  show them.
- **Off by default.** Without `--ai-heal`, or without an `ai:` block, no request is made.

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
use `provider: local` if that matters. In Studio's chat, `provider: openai` sends the open file with each message, as `provider: anthropic` already does.

See [Configuration](/advanced/configuration/#ai) for every `ai:` key.
