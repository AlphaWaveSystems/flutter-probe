# Issue #237 investigation — Android WS drop cascade: two real bugs found and fixed, drop trigger itself still open

Issue #237 reports a deterministic Android WS connection death at a specific file-to-file
transition (7 real Firebase sign-in/sign-out cycles → next file's `goto feed`), after which
**every remaining test fails** — 76/79 failures in one run from a single transport event, with
auto-reconnect never recovering.

This investigation did not (yet) reproduce the underlying drop itself — that genuinely needs the
reporting project's Firebase-loaded app running live (see "Still open" below). What it did find,
by tracing the exact reported failure signature (`probelink: write: ... use of closed network
connection`, `Connection lost — attempt 1` repeating) through the CLI source, is that **the
cascade after the drop — the part that turns one transport blip into 76 failures — was caused by
two real, confirmed CLI bugs**, both now fixed.

## Bug 1 — the post-reconnect retry switch had drifted out of sync with the dispatch switch

`internal/runner/executor.go`'s `runStep` had two hand-maintained copies of the step-dispatch
switch: the initial attempt (11 step kinds) and the post-reconnect retry (6 step kinds). The
retry copy was missing `parser.RecipeCall`, `ConditionalStep`, `LoopStep`, `RetryStep`, and
`HTTPCallStep`.

Consequence, matching #237's report exactly: a connection error surfacing from a recipe call
(`goto feed` is one) enters the retry loop, `tryReconnect` succeeds — **and then the retry switch
matches nothing**. `err` silently keeps its stale pre-reconnect value, `isConnectionError(err)`
stays true, and the loop calls `tryReconnect` again — whose first action is `e.client.Close()`,
killing the connection it *just* established the attempt before. The entire retry budget burns
without the step ever being re-run, and the connection is left in whatever state the final
attempt happened to produce.

**Fix**: both paths now call a single shared `dispatchStep` helper — the two switches are
structurally the same switch and cannot drift apart again. Locked in by
`internal/runner/dispatch_step_test.go`'s `TestDispatchStep_*` (each previously-missing step kind
asserted to produce its observable side effect through the shared dispatcher, not just a nil
error).

## Bug 2 — conditionals misread a dead connection as "condition not visible"

`runConditional` (the `if X appears` executor) treated **any** `See` failure as "not visible" and
took the else branch — including connection errors. With a dead connection mid-recipe, every
`if X appears` block silently misroutes: `goto feed`'s `otherwise: go back / go back` navigates
*away* from a perfectly healthy screen because the visibility check couldn't be asked, not
because the answer was no. Nothing ever reached the reconnect machinery from the check itself.

`runAction`'s `if visible` suffix pre-check already handled this exact case correctly
("propagate connection errors for auto-reconnect") — the ConditionalStep path just never got the
same treatment.

**Fix**: connection errors from the visibility check now propagate (so runStep's reconnect fires
and the condition is re-evaluated against a live connection); ordinary not-found still takes the
else branch. Locked in by `TestRunConditional_ConnectionErrorPropagates` and
`TestRunConditional_OrdinaryNotFoundStillTakesElse`.

## Ruled out along the way (negative results, kept honestly)

The first hypothesis was the Dart agent's `ws.pingInterval = 5s` (server.dart:249 — added for
iOS/iproxy idle-drop prevention, applied unconditionally to all connections): if the app's
isolate is too busy (Firebase auth/listener churn) to service ping/pong for >5s, does the agent
kill a healthy connection? **Empirically no**, across four load shapes (`ping_repro.dart`,
`ping_repro2.dart`, both in this folder — runnable with plain `dart run`):

- one continuous synchronous CPU block 4× the ping interval — connection survived;
- a dense microtask flood (195M chained microtasks over 4× the interval) — survived;
- repeated 200ms sync bursts with 50ms yields — survived;
- and separately, the **real Go client** (`probelink.DialWithOptions` + its actual
  pingLoop/read-deadline code) against a server that refused to service its read loop for 20s
  (4× the CLI's ping interval, exceeding its 15s pong-refresh deadline) — the client kept the
  connection open and `Connected()` true for 25+ s. (Temporary in-repo test, deleted after the
  result was recorded — the code path it exercised is unchanged.)

So neither side's keepalive machinery kills a busy-but-alive connection on a healthy transport.
This makes an emulator/adb-forward transport-layer event (or something Android-specific not
reproducible on a macOS host loopback) the stronger suspect for the original drop.

## Still open — the drop trigger itself

What actually kills the socket at that specific file transition remains unconfirmed. It needs a
live instrumented run against the reporting project's app (real Firebase Auth + Firestore churn —
the trigger conditions this repo's own test apps can't produce, per PT-25's earlier resolution
note). With these two fixes, the *impact* of the next occurrence changes shape entirely: the
first reconnect that succeeds now actually re-runs the failed step (recipe calls included), and
conditionals no longer misroute on a dead connection — so a drop should now cost one visible
reconnect cycle, not the rest of the run. That also makes the next live repro far more
informative: whatever still fails after these fixes is the drop itself, not the cascade.
