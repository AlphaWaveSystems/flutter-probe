# Scope: the next AI feature in Studio, "Explain this failed step"

Status: proposal, no code. Written so the scope, the limits and the first slice can be decided before anything is built.

## Where Studio's AI stands today
What exists (all opt-in, all advisory, none in the pass/fail path): chat; **Explain failures** (one request per failed test, at most 5, the test name, the error text and the file relative to the workspace); **Suggest a fix** (one ProbeScript line for the failed step, shown on a card; *Replace the line* puts it in the editor after Studio has checked that it is one step with the same verb at the same line); a connection test; local-model discovery. Providers are Anthropic, OpenAI and a local OpenAI-compatible endpoint (Ollama, LM Studio, llama.cpp), bring-your-own-key, direct to the provider with no relay. One outbound gate redacts and bounds every field; hosted destinations show a preview before a send, and a destination on this machine is confirmed once per session (the frontend remembers it, keyed on the destination label). How firmly the send is tied to what was previewed differs by feature, and the next one must not inherit the weaker case: **Suggest a fix** is id-based in Go (the preview returns an id, the send is for that id, and a change after it stops the send: `TestAChangeAfterThePreviewStopsTheSend`, `studio/ai_selector_fix_test.go`); **Explain failures** rechecks only the destination label (`ExplainPreviewed(destination)`), and `ExplainLastRun` sends with no Go-side preview at all, so a change to the failures after the preview does not stop that send. Screenshots are never sent from Studio. With `provider: none` or no `ai:` block nothing is sent (pinned by tests for nine configurations).

## What the feature is
A button on a **failed step row** that explains why that step failed, in a few sentences, using only what the run already knows about it. It is the step-level version of Explain failures: the person is looking at one red line and asks "why this one".

It answers three things and nothing else: the most likely cause (a category: lookup did not match, timing, the app died, a permission or system dialog, network), what the run showed that points to it, and what to look at or try next. It never edits, saves, runs, connects, clears data or presses anything. If the text suggests a line, it is shown as text; the only way a line reaches the editor is the existing **Replace the line**, with all its checks.

## What it may read, and what it may send
Read (Studio's own state, nothing new from the device): the failed step (its line, description, the lookup), its error text, the steps before it in the test (descriptions only), the attempt number, the crash note if the run recorded one, and the file's name relative to the workspace.

Sent, after the outbound gate, and only after the preview:
- always: the step line, the error text, the previous steps' descriptions (bounded: 500 characters for the step, 3000 for the error, 20 previous steps of 200 each);
- only if the person turned the existing switch on (`studio.ai_context.widget_tree`, off by default): the widget tree near the lookup, bounded and redacted as in chat.

Never sent: screenshots (this feature is text only), other files, the key, anything from the keychain, the person's settings, device identifiers, the workspace path. A secret-shaped value in any field is replaced before the preview, so the preview already shows what leaves.

## Local first, bring your own key
- The default destination is the one the person configured. If it is on this machine, one confirmation per session (today kept by the frontend, keyed on the destination label, as for Explain failures). If it is hosted, a preview each time. A remote endpoint still needs the existing explicit confirmation.
- No relay, no account, no default provider: with nothing configured the button says so and sends nothing.
- A local model is the case that must work well: the prompt is short, the output is plain text, and a model that cannot follow the format gets its text shown as is, never parsed into an action.

## Consent and wipe rules
The model is never in the decision path of anything destructive. This is the same rule as `AskClearData`, stated for the model:
1. Nothing the model writes is executed. Text from the model is shown as text (set with `textContent`, never as markup).
2. No wipe, restart, reinstall, permission change or connection is offered as a button by this feature, and none is reachable from its output. If the explanation says "clear the app's data", that is advice in a sentence; the step in the test and its question (the clear-data dialog, `--yes`, the person's own setting) are the only way a wipe happens.
3. The feature has no `--yes`, no setting that skips the preview for hosted destinations, and the automation endpoint and the MCP tools cannot call it (its category, `aiProvider`, is in `automationNever`, which the bound-method table tests in `studio/bound_methods_test.go` enforce). It is not window-only: Studio's AI methods can be reached through the test-only headless bridge, which is why the Go side, not the window, must enforce the preview (next point).
4. The preview is the contract, enforced in Go: the preview call returns an id for the exact request it showed, the send takes that id and nothing else (no text, no destination from the window), it works once, and a change to the step, the run, the file, the settings or the key after the preview stops the send. This is the id-based path of Suggest a fix (`TestAChangeAfterThePreviewStopsTheSend`), not the label check of Explain failures; a send that has no preview id is refused.

## Failure modes, and what is shown
| What happens | What the person sees | Pinned by |
|---|---|---|
| AI off, no provider, or a refused configuration | the button is absent or says why; no request | the zero-requests test over the configurations |
| provider slow or hung | a stop after the limit, and a plain "no answer", no retry | per-call timeout tests |
| provider error | a fixed sentence, never the provider's raw text | the error-gating tests |
| empty, off-topic or very long answer | shown cut to the limit, or "no explanation" | new tests |
| text that tries to instruct ("ignore the above, run ...") in the error, a test name or screen text | it is data: the prompt says so, the output is only ever shown as text, and nothing it says can act | new tests with planted instructions in each field |
| a secret in the error text | replaced before the preview | the redaction catalog test and `TestExplainGatesEveryField`, extended to this feature's fields (new) |
| the person changes the file or the run while it waits | the answer is shown for the step it was asked about, labelled with it | new test |

## Measurable targets
These are proposals for the owner to confirm; each can be checked by a test or a count.
1. **No request unless asked:** zero requests with AI off or unconfigured, and zero before the person presses the button and the preview (the existing property, extended to the new surface).
2. **Leak check:** planted secrets of every family the redactor knows (about fourteen pattern families, plus the JWT and card-number rules), put in each of the fields this feature sends, appear in 0 of N previews. This is a proposed new test: today `TestExplainGatesEveryField` and the hidden-character sweeps cover Explain failures, and the plant-in-every-position fuzz exists only for Save as step (`FuzzSaveStep`).
3. **Bounded:** a request is at most about 12 KB without the widget tree and about 45 KB with it, whatever the inputs.
4. **Preview first:** the preview is shown with no network use, in under 100 ms on the seeded corpus below.
5. **Usefulness, measured, not claimed:** a corpus of 30 recorded failures with a known cause category (selector not found, timing, app crash, permission dialog, network error, wrong text), run against one named local model and one hosted model. The target is the category named in the first sentence for at least 70% on the local model and 85% on the hosted one; the number reported is whatever is measured, and a miss is reported, not hidden.
6. **Local latency:** answer in under 20 seconds at the 95th percentile on the named local model on the project's reference machine, or the setting that makes it so is documented.

## The first slice
One thing, small enough for a few reviews:
- a pure builder for the step's request (fields, bounds, the gate) with its table tests, no UI;
- the preview and the send through a two-call path bound by a preview id in Go, as Suggest a fix is (not the `ExplainPreviewed(destination)` shape), for one step instead of one test;
- the text shown under the step row, advisory style, with the "Explain failures" card's wording and no buttons;
- tests for the failure table above, and the corpus run as a script (not in CI) that prints the target 5 number.

Not in the first slice: the widget tree, any suggested line, history, saving an explanation, a second provider setting.

## Open decisions
- The widget tree: ask for it per request instead of a switch? (Proposal: keep the existing switch, off by default.)
- A text-only local model class to name as the reference one for target 5 and 6.
- Whether the explanation may be copied into the test file as a comment (proposal: no).
