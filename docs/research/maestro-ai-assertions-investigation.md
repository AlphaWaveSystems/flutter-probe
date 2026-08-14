# Investigation: Maestro's AI Commands (`assertWithAI`, `assertNoDefectsWithAI`, `extractTextWithAI`)

**Status:** Complete
**Date:** 2026-08-13
**Scope:** How Maestro's AI-powered assertion commands actually move data, what account/infra
they depend on, and what that means for teams evaluating them (or evaluating an equivalent
feature in FlutterProbe).
**Method:** Primary-source read of `mobile-dev-inc/maestro` (the OSS CLI implementation) and
`mobile-dev-inc/maestro-docs`, cross-checked against published docs and the PR that introduced
the feature. File:line citations below point at the OSS repo as of this investigation.

---

## 1. Executive summary

Maestro ships three screenshot-based AI commands: `assertWithAI`, `assertNoDefectsWithAI`, and
the less-publicized `extractTextWithAI`. All three are marketed as "just describe what you want
in plain English" — but under the hood they are thin wrappers around a **mandatory HTTP call to
Maestro's own cloud backend** (`api.copilot.mobile.dev`), authenticated with a **Maestro Cloud
account token**. There is no supported local, offline, or bring-your-own-provider path for the
commands as shipped in the CLI. This confirms the premise: it's a vendor-hosted proxy feature
gated behind an account, not a client-side "call OpenAI with your own key" feature, despite
`maestro-ai`'s source tree containing a standalone OpenAI client (used only by an internal demo
app, not by the production command handlers).

Concretely, every `assertWithAI` / `assertNoDefectsWithAI` / `extractTextWithAI` step:

1. Captures a full-resolution screenshot of the device screen.
2. Base64-encodes it into a JSON body, together with your assertion text.
3. POSTs it over HTTPS to `https://api.copilot.mobile.dev/v2/find-defects` (or `/v2/extract-text`).
4. Authenticates with `Bearer <token>`, where the token is either an env var
   (`MAESTRO_CLOUD_API_KEY`) or a cached session created by `maestro login`.
5. Maestro Cloud relays the image to a third-party LLM vendor it selects on your behalf
   ("Maestro automatically manages the underlying third-party AI providers") and returns a
   structured verdict.

No paid plan is required — a free Maestro Cloud account is sufficient — but an **account and a
network round-trip to Maestro's infrastructure are both mandatory**. There is no config flag to
point this at a self-hosted endpoint, no per-field redaction, and no documented data-retention or
model-training policy in the command reference itself.

---

## 2. What each command does

| Command | Purpose | Docs source |
|---|---|---|
| `assertWithAI` | Screenshots the current screen, sends it + a natural-language assertion to an LLM, asks "is this true?" Returns pass/fail. Documented use case: things hard to express as element assertions — e.g. "A two-factor authentication prompt, with space for 6 digits, is visible." | [docs.maestro.dev/reference/commands-available/assertwithai](https://docs.maestro.dev/reference/commands-available/assertwithai) |
| `assertNoDefectsWithAI` | Screenshots the current screen, sends it to an LLM with a fixed prompt asking it to flag visual defects (cut-off text/elements, overlap, mis-centering). Intended as a "does this screen look right" smoke check. | [docs.maestro.dev](https://docs.maestro.dev/api-reference/commands/assertnodefectswithai) |
| `extractTextWithAI` | Screenshots the current screen, asks the LLM to read/extract specific text from it (e.g. an OTP code) into a variable. Not part of the two commands named in the prompt, but shares the exact same transport/auth path — found while tracing the code. | `Orchestra.kt` (see §3) |

Both `assertWithAI` and `assertNoDefectsWithAI` default `optional: true`, i.e. a failed AI
verdict does **not** fail the flow unless you explicitly opt in — an implicit admission by
Maestro's own team that model output isn't reliable enough to gate CI on by default.

## 3. How it actually works (source-verified)

Package `maestro-ai` (in the `mobile-dev-inc/maestro` monorepo) contains two independent
implementations:

- `maestro/ai/openai/Client.kt` — a **direct** OpenAI chat-completions client. Used only by
  `maestro/ai/DemoApp.kt`, a standalone CLI demo tool (`maestro-ai-demo`) for developing/testing
  prompts against fixture screenshots. It is **not** wired into the shipped `probe`/`maestro` test
  runner.
- `maestro/ai/cloud/ApiClient.kt` — the **production path**. This is what `assertWithAI` /
  `assertNoDefectsWithAI` / `extractTextWithAI` actually call.

`ApiClient.kt` (paraphrased from source):

```kotlin
class ApiClient {
    private val baseUrl by lazy {
        System.getenv("MAESTRO_CLOUD_API_URL") ?: "https://api.copilot.mobile.dev"
    }

    suspend fun findDefects(apiKey: String, screen: ByteArray, assertion: String? = null): FindDefectsResponse {
        val url = "$baseUrl/v2/find-defects"
        httpClient.post(url) {
            headers { append(HttpHeaders.Authorization, "Bearer $apiKey") }
            setBody(json.encodeToString(FindDefectsRequest(assertion = assertion, screen = screen)))
        }
        ...
    }
}
```

`screen: ByteArray` is the **raw screenshot**, serialized as base64 inside the JSON body — no
downsampling, cropping, or redaction step in this code path. `assertion` is your literal
ProbeScript-equivalent assertion string.

`maestro-orchestra/.../Orchestra.kt` wires the engine and enforces the account requirement:

```kotlin
private val AIPredictionEngine: AIPredictionEngine? = apiKey?.let { CloudAIPredictionEngine(it) }
...
if (AIPredictionEngine == null) {
    throw MaestroException.CloudApiKeyNotAvailable(
        "`MAESTRO_CLOUD_API_KEY` is not available. Did you export MAESTRO_CLOUD_API_KEY?"
    )
}
```

i.e. **every** `assertWithAI`/`assertNoDefectsWithAI`/`extractTextWithAI` step hard-fails at
runtime if no Maestro Cloud credential is present. There is no local fallback engine.

`maestro-client/.../auth/ApiKey.kt` resolves that credential with this precedence:

1. `MAESTRO_CLOUD_API_KEY` environment variable, or
2. a cached session token at `~/.mobiledev/authtoken`, written by running `maestro login`
   (Maestro Cloud's device-code OAuth-style login flow).

So in practice a team either (a) runs `maestro login` interactively once per machine/CI runner
and stores a long-lived session token, or (b) generates an API key from the Maestro Cloud
dashboard and injects it as a CI secret. **Both paths require creating a Maestro Cloud account**
— there is no anonymous or offline mode for these three commands.

```
┌──────────────┐   screenshot (raw PNG, base64)    ┌────────────────────────┐   image + prompt   ┌───────────────────┐
│  Maestro CLI  │ ─────────────────────────────────▶│ api.copilot.mobile.dev │────────────────────▶│ 3rd-party LLM      │
│  (your device/│   + assertion text                 │  (Maestro Cloud, v2/   │  vendor-selected,   │ vendor (OpenAI /   │
│   CI runner)  │   Bearer <MAESTRO_CLOUD_API_KEY>   │  find-defects,         │  not user-           │ Anthropic per      │
│               │◀─────────────────────────────────  │  extract-text)        │◀─────────────────────│ public statements) │
└──────────────┘        pass/fail + defect list      └────────────────────────┘   verdict            └───────────────────┘
        ▲
        │ credential sourced from either:
        │  - MAESTRO_CLOUD_API_KEY env var, or
        │  - ~/.mobiledev/authtoken (written by `maestro login`)
        │ → both require a Maestro Cloud account
```

Source files (as of investigation date, `mobile-dev-inc/maestro` default branch):
- `maestro-ai/src/main/java/maestro/ai/cloud/ApiClient.kt`
- `maestro-ai/src/main/java/maestro/ai/Prediction.kt`
- `maestro-ai/src/main/java/maestro/ai/CloudPredictionAIEngine.kt`
- `maestro-orchestra/src/main/java/maestro/orchestra/Orchestra.kt` (lines ~150–151, ~538–620 covering AI command handlers)
- `maestro-client/src/main/java/maestro/auth/ApiKey.kt`
- Feature PR: [mobile-dev-inc/maestro#1906](https://github.com/mobile-dev-inc/maestro/pull/1906)

## 4. What is and isn't disclosed

**Confirmed from source/docs:**
- Full, unredacted screenshot bytes leave the device/CI runner on every AI-assertion step.
- Destination is Maestro's own backend (`api.copilot.mobile.dev`), not the LLM vendor directly.
- Maestro Cloud account (free tier acceptable) is a hard requirement — enforced with a thrown
  exception if absent, not a graceful degrade.
- The specific LLM vendor/model is chosen by Maestro, not configurable by the caller in the
  shipped command surface.
- `MAESTRO_CLOUD_API_URL` env var exists in the client and can override the base URL — but it is
  **undocumented** as a supported customer feature (no docs page references it); treat it as an
  internal/staging override, not a sanctioned self-hosting path.

**Not disclosed anywhere in the command reference docs (`assertWithAI`, `assertNoDefectsWithAI`
pages):**
- Data retention period for uploaded screenshots on Maestro's backend.
- Whether uploaded screenshots/assertions are used for model training/fine-tuning (by Maestro or
  by its LLM subprocessors).
- Full list of AI subprocessors (which LLM vendor(s), which region).
- Any DPA / SOC2 / data-residency guarantees specific to the AI endpoints (as opposed to Maestro
  Cloud generally).

These would need to come from Maestro's Privacy Policy / DPA / subprocessor list — not from the
command docs — before any team could sign off on this for screens that show real user data
(auth flows, payment, health, KYC, 2FA — notably the docs' own example use case is a **2FA
prompt**, i.e. the reference example is itself a security-sensitive screen).

A related, separately-gated surface — **"AI test analysis"** (`--analyze` flag / Maestro Cloud
dashboard) — uploads test logs, per-step command metadata, *and* screenshots for an entire flow
run, again exclusively through Maestro Cloud, again requiring login. Same risk profile, larger
blast radius (whole-flow artifact, not one screenshot).

## 5. Why this matters (risk framing)

1. **Supply-chain dependency, not a pure client feature.** Adding these commands to a test suite
   adds Maestro Cloud *and* an undisclosed, Maestro-selected LLM vendor to your software supply
   chain and your data flow diagram — for every CI run that exercises an AI-asserted screen.
2. **PII/screen-content exposure by construction.** The commands only work by uploading exactly
   the pixels a real user would see. Test fixtures with realistic data (which is common practice,
   precisely to catch real bugs) means realistic PII goes with it, off-device, on every run.
3. **New secret to manage.** `MAESTRO_CLOUD_API_KEY` (or the cached authtoken) becomes a CI
   secret with implicit read access to a stream of your app's UI screenshots — a materially
   different blast radius from, say, a device-farm credential.
4. **No opt-out granularity below "don't use the command."** There's no redaction hook, no
   per-run consent gate, no local model fallback — the only way to keep a given screen's pixels
   off Maestro's infrastructure is to not write an AI-assertion step against it.
5. **Non-determinism reaches CI gating.** `optional: true` by default mitigates flakiness, but
   teams that flip it to `false` for meaningful coverage are now gating merges on a third-party,
   versionless, non-reproducible model call.

## 6. Implication for FlutterProbe

FlutterProbe already has a precedent for exactly this class of decision: the `cloud:` block in
`probe.yaml` for device farms is explicitly **bring-your-own-account, no default URL** — the
opposite shape from Maestro's AI commands, which are default-cloud with account creation forced
at first use. That existing convention is the natural constraint for any AI-assertion feature
FlutterProbe considers — see the companion PRD:
[`docs/prd/ai-visual-assertions-prd.md`](../prd/ai-visual-assertions-prd.md).

## Sources

- [assertWithAI — Maestro Docs](https://docs.maestro.dev/reference/commands-available/assertwithai)
- [assertNoDefectsWithAi — Maestro Docs](https://docs.maestro.dev/api-reference/commands/assertnodefectswithai)
- [AI test analysis — Maestro Docs](https://docs.maestro.dev/maestro-flows/workspace-management/ai-test-analysis)
- [What's new in Maestro: 1.39.0](https://maestro.dev/blog/whats-new-in-maestro-1-39-0)
- [mobile-dev-inc/maestro PR #1906 — Feature/maestro ai](https://github.com/mobile-dev-inc/maestro/pull/1906)
- `mobile-dev-inc/maestro` source: `maestro-ai/`, `maestro-orchestra/src/main/java/maestro/orchestra/Orchestra.kt`, `maestro-client/src/main/java/maestro/auth/ApiKey.kt`
