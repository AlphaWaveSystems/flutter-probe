# Getting probe's hands on WebView content

**Status:** Proposal — not yet implemented
**Origin:** FP-8 (Jira). Flagged as an open gap in
`docs/roadmap/gap-analysis-2026-08.md` ("WebView content (Chrome DevTools
hierarchy mode) — no FlutterProbe equivalent yet"), but unlike every other
item that gap analysis produced (native UI → PT-13/N-1/N-2), no design work
had been done on this one before this proposal.
**Touches:** `internal/device`, `internal/runner`, ProbeScript grammar,
`probe_agent/lib/src/executor.dart` (read for context, not modified by this
proposal)

Maestro's `androidWebViewHierarchy: devtools` flag attaches to a WebView's
Chrome DevTools Protocol endpoint and folds its DOM into the same element
tree Maestro already inspects, so a flow can select and tap into
web-rendered content the same way it selects native views. FlutterProbe has
no equivalent today. This proposes whether, and how, to build one.

## TL;DR

- **Ask:** Let ProbeScript select and interact with content rendered inside
  an in-app WebView (`webview_flutter`, or any platform view backed by a
  system WebView), matching what Maestro's devtools hierarchy mode does.
- **Finding:** Two findings, and both cut against starting now. First,
  probe's Dart agent has no WebView code path to extend — the only
  WebView-related line in `executor.dart` is `_openLink`'s
  `'useWebView': false` (line 974), which forces every `open link` call out
  to the system browser instead of an in-app WebView. Second, and more
  important: unlike N-2 (which opened with nect-flutter's real, blocking
  `image_picker` → `PHPickerViewController` flow), neither of this
  project's reference apps has any documented in-app WebView content
  anywhere in this repo's roadmap, gap-analysis, or evidence corpus. There
  is no concrete flow to justify or size this work against.
- **Recommendation:** Don't start implementation. The problem is fully
  split by platform — Android has a real, standard debugging protocol; iOS
  does not — so there isn't one project to scope, only two independent
  ones, and neither has an adopter demand signal today. Revisit only once a
  concrete flow surfaces, the same bar every other Phase 2/3 item in this
  roadmap was held to.

## What "no WebView content" means today, concretely

`probe_agent/lib/src/executor.dart`'s `_openLink` (lines 968–984) is the
only place the word "WebView" appears in the Dart agent:

```dart
Future<void> _openLink(String url) async {
  const channel = MethodChannel('plugins.flutter.io/url_launcher');
  try {
    await channel.invokeMethod<bool>('launch', {
      'url': url,
      'useSafariVC': false,
      'useWebView': false,
      'enableJavaScript': false,
      'enableDomStorage': false,
      'universalLinksOnly': false,
      'headers': <String, String>{},
    });
  } catch (_) {
    // Fallback: record the launch intent so verify_browser can confirm it
    _externalUrlLaunches.add(url);
  }
}
```

This backs ProbeScript's `open link "…"` verb, and it explicitly opts out
of `url_launcher`'s in-app-WebView mode (`useWebView: false`), forcing
every such call to the system browser. That's a deliberate, narrow choice
for that one verb — it says nothing about whether an adopter's own app
embeds a `WebView` widget elsewhere in its own code (a help page, a
checkout iframe, an SSO redirect) — but it does mean the agent has
literally zero existing plumbing that talks to a WebView today, in
contrast to PT-13's native-UI proposal, which at least had permission
dialogs' OS-level bypass to build alongside. Here there's nothing to
extend; a WebView verb would be new code end to end.

## No concrete justifying flow found

N-2 didn't get built on the strength of "Maestro has this and we don't" —
it got built because nect-flutter had a real, code-cited, currently-blocked
flow (`post_form_page.dart:561`/`848`, an `image_picker.pickMultiImage()`
call with no bypass). This proposal looked for the equivalent and didn't
find one:

- `docs/roadmap/gap-analysis-2026-08.md` and
  `docs/roadmap/maestro-competitive-roadmap.md`/`-COMPLETED.md` describe
  water-sip as a "Firebase/IAP consumer app" and nect-flutter as a
  "Firebase/Firestore social app" with a 66+/76-flow Maestro suite — neither
  description, nor any of the individual task write-ups (`B-1`, `E-3`,
  `N-1`, `N-2`, or the `docs/evidence/**` folders they link to), mentions
  an embedded WebView, an in-app browser, an iframe, or any web-rendered
  screen in either app.
- The PRD for AI visual assertions (`docs/prd/ai-visual-assertions-prd.md`)
  lists "third-party webview content that isn't in the Flutter widget tree"
  as one motivating *use case* for AI-based screenshot assertions — a
  fallback for content probe can't structurally select — but that's a
  hypothetical example in a different feature's PRD, not a documented
  occurrence in either reference app.

That absence is itself the headline finding of this proposal. Without a
real flow, there's nothing to size the work against, and picking an
implementation shape is guesswork: a text-content help page needs
DOM-selector matching; a payment iframe needs cross-origin frame handling
and probably shouldn't be automated at all for PCI-scope reasons; a
CMS-rendered onboarding screen needs neither. N-2 could commit to a shape
because it had one real flow pinning the requirements down. This proposal
can't yet.

## The technical split, and why it's worse than N-2's

Android and iOS diverge here even more sharply than they did for native-UI
bridging (PT-13/N-2), because the underlying debugging mechanisms aren't
just differently-costly versions of the same thing — one is a standard
protocol and the other is an unofficial workaround for a private one.

| | Android WebView | iOS WKWebView |
|---|---|---|
| Debug endpoint | Chrome DevTools Protocol (CDP), the same protocol Chrome's own `chrome://inspect` and remote-debugging tooling speak | Safari's remote Web Inspector protocol — a separate, largely undocumented binary protocol |
| How it's exposed | A Unix domain socket (`@webview_devtools_remote_<pid>`) once `WebView.setWebContentsDebuggingEnabled(true)` is set on the app; reachable via `adb forward` exactly like Chrome's own inspector does it | Not exposed as CDP at all. The only public path is Safari's Develop menu; non-Apple tooling has historically only gotten at it via reverse-engineered relays (`ios-webkit-debug-proxy`) that translate the private protocol into a CDP-shaped surface |
| Standardization | Documented, stable, Google-maintained | Unofficial, no documented public API, no stability guarantee from Apple |
| Maturity vs. this repo's N-2 precedent | Comparable to N-1's `uiautomator` — a standing, documented OS mechanism | Meaningfully **less** mature than N-2's WDA. WDA is an official XCTest-based server Appium has run in production since ~2016; the WebView-inspection equivalent here is a community shim around a protocol Apple doesn't document and can change without notice |
| What Maestro itself ships | `androidWebViewHierarchy: devtools` — documented, supported | Nothing. Maestro's own docs (`docs.maestro.dev`, checked 2026-08-16) have no iOS equivalent flag or section for WebView content at all |

That last row matters: Maestro is the tool this entire roadmap effort
benchmarks against, and the one whose devtools flag is the direct
inspiration for this ticket. If a mobile-testing vendor with years of
investment in exactly this space hasn't shipped an iOS WebView-inspection
story, that's independent evidence the iOS side is harder than "just build
the iOS half too" — not only this proposal's own assessment of it.

## What Android-only support would look like, if a flow justified it

Sketched for completeness, not as a commitment — this shape should be
re-derived against whatever real flow eventually justifies building this,
the same way N-1's `uiautomator` design and N-2's `WDAClient` design were
each derived from (or at least checked against) a real flow.

```
test "read a WebView-rendered help article"
  tap "Help"
  wait until "Getting Started" appears
  see web "Getting Started"
  tap web "Contact support"
  see "Message sent"
```

**Implementation shape:**
- A `CDPClient` (new, `internal/device/cdp.go` or similar), parallel to
  N-2's proposed `WDAClient`: `adb forward` onto the app's
  `@webview_devtools_remote_<pid>` socket, then plain JSON-over-WebSocket
  CDP calls — `DOM.getDocument`/`DOM.querySelector` to find an element by
  text or CSS selector, `Input.dispatchMouseEvent` to tap it.
- Unlike N-1/N-2's native-UI verbs, this can't be a pure `DeviceContext`
  operation dispatched outside the Dart agent the way `AllowPermission` or
  `TapNative` are — CDP needs to know a WebView exists and its debug port
  is open, which in turn needs `WebView.setWebContentsDebuggingEnabled(true)`
  to be set on the app (typically already true in debug builds, but a new
  adopter-facing requirement to document, since release builds commonly
  disable it — the same class of "app-side opt-in" this repo already
  documents for the iOS notification-prompt workaround in PT-13).
- Whether `see web`/`tap web` should be `DeviceContext`-dispatched (like
  N-1/N-2, treating WebView content as an isolated block) or routed
  through the Dart agent (so WebView and Flutter-tree actions can
  interleave in one step sequence) depends entirely on what the
  justifying flow actually needs — that question shouldn't be answered
  speculatively here.
- iOS gets no implementation in this pass; see Options below.

## Options considered

### A — Build both Android CDP and iOS remote-inspector support now, regardless of demand

**Not now.** No justifying flow on either platform, and the iOS mechanism
is meaningfully less mature than WDA was for N-2 — this is a harder reject
than N-2's own "Option A" (compiling XCTest into every adopter's project),
because this proposal doesn't have N-2's offsetting factors (a real flow,
a mature backend) to make the up-front cost worth paying.

### B — Scope Android CDP only now, hold iOS for later

**Plausible middle path, not recommended yet.** Mirrors PT-13/N-1's
precedent of shipping the cheap platform first. But "cheap to build" isn't
sufficient justification on its own — every other item this roadmap has
shipped (N-1, N-2, E-1 through E-3) started from a concrete, documented
flow, not a capability gap alone. Revisit this option specifically once
one exists.

### C — Do nothing further until a real flow appears

**Recommended.** Matches this proposal's own key finding: unlike every
other tracked gap in this roadmap, WebView content has zero grounding in
either reference app today. The right next step is finding — or
deliberately going and looking for, e.g. with a design partner or a third
reference app — a concrete flow, not writing code against a hypothetical
one.

### D — Wait for the ecosystem to mature (Apple ships an official inspector, or `ios-webkit-debug-proxy` stabilizes)

**Not now**, for the same reason N-2 rejected the equivalent option: no
public indication this is coming, no timeline to plan against. Worth
naming as a real alternative to "build the unofficial shim ourselves,"
though, given how much less mature that shim is than WDA was — this
proposal isn't confident enough in the iOS mechanism to recommend building
against it even if a flow appeared tomorrow, without first spiking whether
`ios-webkit-debug-proxy` (or an equivalent) is stable enough to run in CI.

## Recommended next step

Don't write any code yet. Before this proposal's status changes:

1. Actively look for a concrete flow — check whether any current or
   prospective adopter's app embeds WebView content in a screen that needs
   test coverage (a help/FAQ page, a checkout or SSO redirect, a
   CMS-driven onboarding screen). This is the same bar N-2 met with
   nect-flutter's photo picker; nothing here should be built without it.
2. If and when one appears, scope Android-only first per Option C/B,
   sized against that real flow rather than the hypothetical sketch above
   — and separately validate whether an iOS mechanism is even viable
   before promising iOS parity.
3. Keep `docs/roadmap/gap-analysis-2026-08.md`'s existing WebView bullet
   pointed at this proposal (done as part of this change) so this gap
   stays visible and doesn't get silently rediscovered the next time a
   Maestro-parity sweep happens.
