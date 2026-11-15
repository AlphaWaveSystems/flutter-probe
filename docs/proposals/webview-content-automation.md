# Getting probe's hands on WebView content

**Status:** Requested by the owner (2026-11-15); design for review; step 2 (spike) done, step 3 (implementation) in slices
**Origin:** FP-8 (Jira), listed as an open gap in `docs/roadmap/gap-analysis-2026-08.md`. A concrete demand for WebView support now exists (the owner asked for it on 2026-11-15).
**Touches:** `internal/device`, `internal/runner`, ProbeScript grammar, the dictionary surfaces
(docs, MCP, VS Code extension, Studio). The Dart agent is not modified.

The earlier version of this proposal recommended not starting because no flow or adopter had asked
for WebView content. That blocker is gone: a concrete demand now exists. This version records the
plan, the design proposed for review, and the rules it must follow. The original analysis is kept
below under "Findings from the first analysis (2026-08)" because it still explains why Android
comes first.

## Plan

- **Android first**, through the Chrome DevTools Protocol (CDP) of the app's own WebView.
- **iOS later.** It uses a different, undocumented mechanism; see "iOS" below for what is and is
  not known.
- Delivered in three steps, one at a time: (1) this document, (2) a spike with a real WebView
  fixture and a recorded run, (3) implementation in small PRs. Each step starts only after the
  previous one is accepted.

## Android design

### Reaching the WebView

1. Find the app's WebView debug sockets: abstract Unix sockets named
   `webview_devtools_remote_<pid>`, where `<pid>` is the process id of the app under test. List
   them from the device (`/proc/net/unix`) and keep only the ones whose `<pid>` is the app's own
   process. Never use sockets that belong to another process. (Confirmed by the spike: a second
   process on the same device had its own `webview_devtools_remote_<pid>` socket, so this rule is
   needed, not defensive.)
2. Forward that one socket to a free loopback port chosen by adb
   (`adb -s <serial> forward tcp:0 localabstract:webview_devtools_remote_<pid>`; adb prints the
   port). Record the port; it is what we remove later. The spike must confirm that adb prints the
   allocated port for `forward tcp:0 ...` on the adb version in use. If it does not, the fallback is a
   port chosen by probe and checked free first, and that port is still removed individually.
3. List the targets over HTTP (`GET http://127.0.0.1:<port>/json/list`). Each target is one page
   in a WebView. Connect to its `webSocketDebuggerUrl`.
4. Talk CDP over that WebSocket: `Runtime.evaluate` for lookup, `DOM` for structure,
   `Input.dispatchMouseEvent` and `Input.dispatchKeyEvent`/`Input.insertText` for taps and typing.
   The lookup code that `Runtime.evaluate` runs is fixed text written by probe, never text from a
   script (see "Owner question").
5. When the step ends, remove only the forward this run created
   (`adb -s <serial> forward --remove tcp:<port>`).

The app must have WebView debugging switched on
(`WebView.setWebContentsDebuggingEnabled(true)`, which Flutter's `webview_flutter` sets through its
debugging option). Whether a debug build always has it on has to be confirmed in the spike, and the
answer goes into the user docs as a requirement. If debugging is off the step fails with a message
that says so (see "Failure messages").

### Minimal ProbeScript surface

It reuses the selector syntax probe already has (text, `#id`-style, quoted strings) and adds one
suffix, `in webview`. Nothing new to learn for the selector part.

```
see "Getting Started" in webview
tap "Contact support" in webview
type "hello" into "#message" in webview
wait until "Message sent" appears in webview
```

- `see` / `tap` / `type` / `wait until ... appears` keep their meaning. `in webview` sends the step
  to the WebView instead of the Flutter widget tree.
- A text selector matches the visible text of a DOM element (exact, then contains, the same order
  probe uses for widgets). A selector that starts with `#`, `.` or `[` is a CSS selector.
- **Picking a WebView** when the app has more than one: `in webview "<part of the page title or
  URL>"`, for example `tap "Pay" in webview "checkout"`. Without a name, exactly one target must
  exist; if there is more than one, the step fails and lists them (see below).
- The grammar (EBNF), a conformance test and all doc surfaces are part of step 3; this section only
  fixes the shape.

### Failure messages (what a user must be able to read)

| Situation | Message must say |
|---|---|
| No `webview_devtools_remote_<pid>` socket for the app | No WebView debugging socket was found for the app. The app may have no WebView on this screen, or WebView debugging is off in this build (`setWebContentsDebuggingEnabled`). |
| Several targets, no name given | Found N WebView pages (titles/URLs shown, truncated); name one with `in webview "<part of title or url>"`. |
| Selector matches nothing | No element in the WebView page "<title>" matches "<selector>" (same wording as the widget case). |
| Selector matches several | Matches N elements; tap would use the first visible one (or fail, to be decided in step 3 together with the widget behaviour). |
| Element not visible or outside the viewport | The element exists but is not visible. |
| CDP connection lost or page navigated mid-step | The WebView page changed while the step ran; retry rules follow the existing step retry. |
| Forward could not be created or removed | The adb error, plus which port and serial. |

Messages never include page content beyond the matched text, and never include the CDP
debugger URL, which can carry a page identifier.

## iOS (later; honest status)

What is known:
- `WKWebView` content can be inspected through Safari's Web Inspector protocol. It is not CDP and
  Apple does not document it as a public interface.
- The app has to make the page inspectable (`isInspectable = true` on iOS 16.4 and later, or
  debug-build defaults on earlier versions).
- Community tools translate that protocol into a CDP-like interface. None has been tried in this
  project.

What is not known, and is a spike of its own (after Android):
- Whether it works against the Simulator the way probe uses it today, and against a physical
  iPhone.
- Whether any community relay is stable enough to depend on in CI.
- What input events (tap, typing) it can deliver.

We commit to Android only in this plan. iOS gets its own proposal update once the Android work
has shipped and a spike has real results.

## Security rules (binding on the spike and the implementation)

1. **Loopback only.** Every forward and every connection is `127.0.0.1`. Never bind or connect
   to another interface.
2. **Only the app under test's own sockets.** Match `webview_devtools_remote_<pid>` against the
   process id of the app's own process. Never attach to another app's WebView, to Chrome, or to a
   system WebView of another process.
3. **Never `adb forward --remove-all`** (or any command that removes forwards probe did not
   create). Record the exact port that adb allocated and remove only that one, also on failure,
   timeout and Ctrl-C.
4. **Every forward and every removal takes `-s <serial>`.** The spike showed why: with two
   emulators attached, `adb forward --remove tcp:<port>` without `-s` failed (exit 1) and left the
   forward in place.
5. **Do not touch anything that is not this run's:** other devices, other forwards, the agent port
   of an already-running probe session (`48686` by default), or an address that belongs to another
   tool. The implementation takes the device serial from the run and does nothing outside it.
6. **No raw JavaScript from script text.** CDP lookup code is a fixed template inside probe. A
   `.probe` file cannot send its own `Runtime.evaluate` text. See the owner question below.
7. **Redaction:** typed values and page text follow the existing redaction rules for logs and
   reports; the WebSocket debugger URL is not written to logs or reports.
8. **Payment and login pages in a WebView** are the app owner's call; this feature does not add
   anything that tries to bypass page protections (no JS injection beyond the fixed lookup, no
   header or cookie handling).

## Spike findings (step 2, 2026-11-15)

A throwaway script (not part of probe) drove a standalone one-WebView test app on a named Android
emulator (Android 15, system WebView 124, adb 1.0.41). It found the app's own debug socket, made one
forward, listed the targets, selected an element by its text, tapped it with
`Input.dispatchMouseEvent`, read the result back, and removed its own forward.

- `adb forward tcp:0 localabstract:webview_devtools_remote_<pid>` prints the allocated port on
  stdout (exit 0) on adb 1.0.41. Keep the fallback (a port chosen by probe and checked free first)
  for adb versions that do not print it.
- Removing the forward needs `-s <serial>` when more than one device is attached (rule 4).
- The pid rule is necessary: the device had two `webview_devtools_remote_*` sockets, one from
  another process.
- `GET /json/list` returned one target of type `page` with the page title; a target is picked from
  that list. The debugger URL was never printed.
- Coordinates from `getBoundingClientRect()` (CSS pixels) were correct for `Input.dispatchMouseEvent`
  with a device pixel ratio of 1.

Not yet tested, each is an open item for the implementation slices: scrolled pages, iframes, a
device pixel ratio other than 1, typing, and a release build with WebView debugging off.

## Owner question

Do we ever allow raw JavaScript from a ProbeScript file (a verb like `run javascript "..." in
webview`)? **Default and plan: no.** It gives scripts a way to run arbitrary code in the page, can
leak data in logs, and is the main way a test could do something the rules above forbid. If the
owner wants it, it is a separate decision with its own review.

## Steps and gates

1. **This document** (docs only). Merge after an independent verdict.
2. **Spike** (done, see "Spike findings"). A fixture with a real WebView that does not add or
   upgrade packages in the Flutter e2e test app: the native Kotlin twin app in
   `e2e-test-workspace` if it has or can get a WebView without new dependencies, otherwise a tiny
   standalone fixture. Run it on a **named** Android emulator that is not one already in use by
   another test. Evidence: a recorded run that selects and taps inside the WebView, with the
   forward created and removed.
3. **Implementation**, in small PRs after the spike is accepted: a CDP client in `internal/device`
   with a loopback fake CDP server for tests, the runner verbs, the grammar EBNF with a conformance
   test, all doc surfaces, and the MCP, VS Code and Studio dictionaries.

## Findings from the first analysis (2026-08)

The sections below are the original analysis, kept because they explain the platform split. The
"no justifying flow" finding and the recommendation not to start are superseded by the Decision
above.

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
it got built because the social app had a real, code-cited, currently-blocked
flow (`post_form_page.dart:561`/`848`, an `image_picker.pickMultiImage()`
call with no bypass). This proposal looked for the equivalent and didn't
find one:

- `docs/roadmap/gap-analysis-2026-08.md` and
  `docs/roadmap/maestro-competitive-roadmap.md`/`-COMPLETED.md` describe
  the consumer app as a "Firebase/IAP consumer app" and the social app as a
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

## The original Android sketch (superseded syntax)

The sketch below used `see web` / `tap web`. The proposed surface is `... in webview`, defined in "Minimal ProbeScript surface" above; the implementation notes still apply.

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
- iOS gets no implementation in this pass; see "iOS (later; honest status)" above.

