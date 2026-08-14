# PRD: AI-Powered Visual Assertions for FlutterProbe

**Status:** Draft — for discussion
**Author:** Patrick Bertsch (drafted with Claude Code)
**Date:** 2026-08-13
**Related:** [Investigation — Maestro's AI Commands](../research/maestro-ai-assertions-investigation.md)

---

## 1. Problem statement

Maestro's `assertWithAI` / `assertNoDefectsWithAI` cover a real gap: some things are genuinely
hard to assert with element-based selectors — "does this screen look visually broken," "is a
6-digit OTP prompt showing," "does this chart look roughly right." Teams evaluating FlutterProbe
against Maestro will ask whether we have an equivalent.

We do not, today. But per the investigation above, Maestro's implementation is not a neutral
baseline to copy: it **mandates a Maestro Cloud account and unconditionally uploads full,
unredacted screenshots to Maestro's infrastructure**, which then forwards them to a
Maestro-selected third-party LLM vendor, with no documented retention or training-use policy at
the command level. For any test suite that exercises real-looking user data (which is most of
them, by design), that's an undisclosed PII pipeline riding on a test assertion. It is also a
supply-chain dependency: a new vendor, a new secret, a new non-deterministic input to CI gating.

FlutterProbe's existing positioning is direct-widget-tree access with **no external automation
layer in the hot path**, and its one existing "leaves your machine" surface (`cloud:` device
farms in `probe.yaml`) is deliberately bring-your-own-account with no default endpoint. An
AI-assertion feature that silently reproduces Maestro's default-cloud, vendor-opaque model would
contradict that positioning and this project's own local-preferred harness philosophy.

## 2. Goals

- Let users write assertions that are hard to express structurally (visual correctness, "does
  this look like a defect," reading dynamic/rendered text) using natural-language, LLM-backed
  checks — consistent with ProbeScript's existing natural-English style.
- Make the **data flow legible and opt-in at every step**: no screenshot leaves the device/CI
  runner unless the user has explicitly configured a provider and the specific command is used.
- Support genuinely local/offline execution as a first-class option, not an afterthought — not
  just "bring your own cloud key."
- Match FlutterProbe's existing `cloud:` convention: no default backend, no forced account
  creation, credentials supplied by the user for a provider the user chose.

## 3. Non-goals

- Building or hosting our own always-on inference proxy (the `api.copilot.mobile.dev` pattern).
  If we ever offer a hosted convenience option, it must be a clearly-labeled opt-in add-on, not
  the only path, and it must publish its own retention/training-use policy before ship.
- Matching every Maestro AI capability 1:1 on day one. Ship the highest-value subset first.
- Making AI assertions a default/implicit part of `wait until` / `see` semantics. They must be
  distinct, explicitly-named commands so their cost/privacy profile is visible in the test source.

## 4. Users & use cases

- **Visual smoke check**: "does this screen look broken" after a refactor, without hand-writing
  layout assertions for every element.
- **Dynamic/unstructured content**: OTP/2FA prompts, chat bubbles, generated charts, third-party
  webview content that isn't in the Flutter widget tree and so isn't reachable by FlutterProbe's
  normal widget-tree selectors.
- **Regulated/enterprise teams** who currently rule out Maestro's AI commands outright because
  screens contain real customer data and legal won't sign off on an undisclosed subprocessor —
  this is the differentiation opportunity, not just parity.

## 5. Proposed design

### 5.1 Provider model: local-first, explicit BYO for cloud

Three provider tiers, all opt-in via `probe.yaml`, none enabled by default:

| Tier | How it works | Data leaves device? |
|---|---|---|
| **`local`** (preferred default when configured) | Runs a small on-device/on-host vision-capable model (e.g. via platform APIs — Apple Intelligence on iOS/macOS hosts, Gemini Nano on Android — or a user-supplied local endpoint such as Ollama/LM Studio) | No |
| **`byo-cloud`** | User supplies their own API key for a provider **they** choose (OpenAI, Anthropic, Google) and FlutterProbe's CLI calls that provider **directly** — no FlutterProbe-operated relay in between | Yes, to the vendor the user picked, never to us |
| *(explicitly not offered)* | A FlutterProbe-hosted proxy/relay that picks the model on the user's behalf | — |

This mirrors the existing `cloud:` device-farm block exactly: "bring your own account, no default
URL." No AI command should ever work out of the box without the user first choosing and
configuring a provider — silence/failure-to-configure must be the default, matching Maestro's own
`CloudApiKeyNotAvailable` fail-closed behavior, but for the *opposite* reason (ours fails closed
because nothing is opt-in; theirs fails closed because everything routes through their account).

### 5.2 Config surface (`probe.yaml`)

```yaml
# AI-assisted assertions (optional — no default provider, nothing is sent anywhere unless
# configured here AND used explicitly in a test with `... with ai`).
ai:
  provider: local            # local | openai | anthropic | google
  # local:
  #   endpoint: http://localhost:11434   # e.g. Ollama; falls back to platform on-device model if unset
  # openai / anthropic / google:
  #   api_key: ${OPENAI_API_KEY}         # user's own key, called directly — never proxied through us
  #   model: gpt-4o-mini
  redact:
    - selector: "#credit_card_field"     # blur/mask matched regions before any AI call
    - pattern: "email"                   # built-in PII pattern redaction (email, phone, etc.)
  send_view_hierarchy: false             # opt-in; off by default, screenshot-only otherwise
```

### 5.3 ProbeScript syntax

Consistent with existing natural-English commands (`see`, `wait until`, `if ... is visible`):

```
see "checkout total looks correct" with ai
assert no visual defects with ai
read "OTP code" with ai into <otp>
```

Each of these:
- Only executes if `ai:` is configured in `probe.yaml`; otherwise fails fast at parse/plan time
  with a clear message ("`with ai` used but no `ai:` provider configured in probe.yaml") — never
  a silent no-op and never an implicit cloud call.
- Runs the configured `redact:` rules against the captured screenshot **before** it's handed to
  any provider (local or cloud).
- Logs, in the test report, which provider handled the call and that a screenshot was sent to it
  — so "was AI used, and where did the pixels go" is answerable from the report, not just the
  config file.
- Defaults to non-blocking (`optional`-equivalent) for the same reason Maestro defaults it: model
  output isn't deterministic enough to gate CI by default. Explicit opt-in to hard-fail, per step.

### 5.4 Redaction pass

Non-negotiable for the cloud tier, recommended even for local: before any image bytes reach an
`AIPredictionEngine` (local or remote), apply `redact:` rules from config. At minimum:
- Selector-based masking (black out a specific widget's bounding box — reuse FlutterProbe's
  existing selector/widget-tree access, which Maestro cannot do since it has no tree access).
- A small built-in library of common PII patterns for local OCR-based blur (email, phone, card
  number shapes) as a defense-in-depth layer, not a substitute for selector-based masking.

This is a capability Maestro structurally cannot offer as cleanly — it has no widget-tree access,
only pixels — while FlutterProbe already walks the live tree for every command. Redaction-by-
selector is a genuine differentiator, not just a privacy checkbox.

## 6. Success metrics

- Zero screenshot bytes transmitted to any third party for a project that has no `ai:` block
  configured (verifiable by network policy / CI egress rules — a testable claim, not just a
  promise).
- Time-to-first-AI-assertion for a `local` provider user: works with zero API keys, zero account
  creation, on first `probe test` run after adding `ai: { provider: local }`.
- At least one enterprise/regulated-industry user cites the redaction + local-provider model as
  the reason they chose FlutterProbe's AI assertions over Maestro's, within two release cycles of
  shipping.

## 7. Risks & open questions

- **Local-model quality.** On-device/local vision models may be materially weaker than GPT-4o-
  class cloud models for subtle "does this look broken" judgments. Needs a quality bar before
  claiming parity — may need to ship cloud-tier first with local-tier as fast-follow, but the
  *config shape* (no default provider, no relay) must be locked from day one so we don't back
  into Maestro's model under deadline pressure.
- **`send_view_hierarchy` temptation.** Sending the widget tree alongside the screenshot would
  likely improve accuracy a lot (structured data > pixel guessing) but expands the data surface
  significantly (tree can contain more than what's visually rendered, e.g. off-screen route state
  — see existing project learning that Navigator keeps off-screen routes mounted). Must stay
  opt-in and off by default.
- **Redaction is best-effort, not a guarantee.** Selector-based masking only catches PII the test
  author remembered to list. Docs must say this plainly — no "PII-safe" marketing claim, only
  "PII-reducing, opt-in, and stricter than the alternative we're compared against."
- **License/tier placement.** FlutterProbe is BSL-CLI / MIT-agent
  (see project memory `project_licensing_strategy`) — confirm which side AI-assertion code lives
  on before scoping, since it affects whether community contributors can extend provider support.
- **Support cost of "any local endpoint."** Allowing an arbitrary local endpoint (Ollama, LM
  Studio, etc.) is flexible but widens the support matrix. Consider shipping with one blessed
  local integration first and documenting "OpenAI-compatible endpoint" as the general escape
  hatch rather than officially supporting each tool by name.

## 8. Rollout plan (proposed)

1. **Phase 0 — spec lock.** Finalize `ai:` config shape and `with ai` ProbeScript grammar per
   §5.2–5.3. No provider implementation yet. Get this reviewed against the licensing question in
   §7 before writing code.
2. **Phase 1 — `byo-cloud` tier, `assert ... with ai` only.** Ship the narrowest useful command
   first (equivalent to `assertWithAI`) against user-supplied OpenAI/Anthropic keys, direct
   client→vendor calls, no FlutterProbe relay. Selector-based redaction shipped in this phase,
   not deferred.
2a. Ship `docs/research/maestro-ai-assertions-investigation.md`'s comparison publicly (website
    docs) alongside this — the differentiation only matters if prospective users can see it.
3. **Phase 2 — `assert no visual defects with ai`.** Shipped as pure reuse of Phase 1's
   provider/redaction/config plumbing — a fixed prompt through the same `VisionProvider`, no
   new RPC, no new config surface.
4. **Phase 3 — `local` provider tier — shipped, scoped down from the original sketch.** §5.1
   originally named two things under "local": platform on-device models (Apple Intelligence,
   Gemini Nano) *or* a user-supplied OpenAI-compatible local endpoint (Ollama, LM Studio). Only
   the local-endpoint path shipped. Platform on-device model bridging was evaluated and
   explicitly deferred, not silently dropped: FlutterProbe's CLI is a Go binary with no route to
   in-app, entitlement-gated platform SDKs like Apple's on-device Foundation Models framework or
   Android's Gemini Nano — supporting those would mean shipping native companion apps/shims per
   platform, a distinct and much larger effort than this feature, not something to fold in under
   a "Phase 3" label. If it's ever pursued, it should be its own scoped initiative with its own
   PRD, not an extension of this one. The on-device-model quality question §7 originally flagged
   is therefore moot for what actually shipped — a local Ollama/LM Studio server's model quality
   is the operator's own choice, not something this project evaluates or certifies.
5. **Phase 4 — `read ... with ai into <var>` (OTP/dynamic text extraction).**

Each phase ships behind the same non-default `ai:` config gate — there is no phase where AI
assertions become reachable without explicit user configuration.
