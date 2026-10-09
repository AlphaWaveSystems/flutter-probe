---
title: Testing against backend data
description: React to what your backend or BFF returns - wait for responses, assert on status and JSON, branch on server data, count requests and mock slow, failing or different answers.
---

Some behaviour depends on data the screen never shows: a plan flag, an order status, a feature toggle your BFF sends. FlutterProbe
records the app's HTTP traffic so a test can read it, and can answer chosen requests itself.

## What is recorded

The agent installs an `HttpOverrides` wrapper when `ProbeAgent.start()` runs, so every `dart:io` `HttpClient` created afterwards is
recorded: the `http` package, dio's default adapter and most Dart clients. Not recorded: WebViews, `dart:html`, and native SDK
calls (a native analytics or payment SDK). Start the agent before the app creates its HTTP clients (the usual `main` does).

Each exchange keeps the method, URL, status, duration, headers (`Authorization`, `Cookie`, `Set-Cookie` and `x-api-key` are
replaced by `<redacted>`) and up to 64 KB of request and response body. The last 300 exchanges are kept. Capture exists in debug and
profile builds only, and `--dart-define=PROBE_HTTP_CAPTURE=false` turns it off. The log starts empty in every test.

## Naming a request

A reference is an optional method and a path:

```
GET "/api/orders"        # exactly this path
"/api/orders/*"          # * matches any run of characters; the whole path must match
"https://api.example.com/v2/me"   # a full URL when the text contains ://
```

`"/api/orders"` does **not** match `/api/orders/42` (use `"/api/orders/*"`). The query string is ignored unless the pattern has a `?`.

## Waiting for and reading responses

```
tap "Refresh"
wait for response GET "/api/orders" status 200      # waits for a new matching response
see response "/api/orders" json "data.count" equals "3"
see response "/api/me" contains "premium"
see response "/api/me" json "data.plan" exists
store response "/api/me" json "data.plan" as plan  # then use <plan> in later steps
```

- `wait for response` takes each exchange once: two waits for the same endpoint need two responses, and a response that arrived
  before the wait started still counts (so `tap` then `wait for response` is safe even when the answer is instant). The step timeout applies.
- `see response`, `store response` and `if response` look at the **newest** matching response and do not wait. With
  `--implicit-wait` a missing response is retried for that long.
- JSON paths are dotted with `[n]` indexes: `data.plan`, `items[0].id`, `items.0.id`. `equals` compares the value as text
  (`42`, `true`, `null`; strings without quotes).

## Branching on server data

```
if response "/api/me" json "data.plan" equals "pro"
  see "Premium"
otherwise
  see "Upgrade"
```

The condition is false when no such response was recorded (it does not wait).

## Counting requests

```
see exactly 1 request GET "/api/me"
see no requests "/api/analytics/*"
clear recorded requests
```

When a count, a wait or a `see response` fails, the message lists the last requests the app made, so a wrong path or a call that
never happened is obvious.

## Mocking

```
when the app calls GET "/api/orders"
  respond with 200 and body "[]" after 3 seconds       # a slow, empty list
when the app calls POST "/api/pay"
  respond with 503 and body "{ \"error\": \"down\" }"
when the app calls PATCH "/api/profile"
  respond with network failure                          # the connection is dropped
```

Mocks are real: the app's client gets the status, headers and body (or a socket error) as if the server sent them, and the log
shows the URL the app asked for. They apply to matching requests from then on, last for the test, and are re-applied when the app
is restarted by `restart the app`. Register them before the step that triggers the request. Use `before each test` for mocks every
test needs.

## Putting it together

```
test "slow orders show a spinner, then an empty state"
  when the app calls GET "/api/orders"
    respond with 200 and body "[]" after 3 seconds
  open the app
  tap "Orders"
  see "Loading"
  wait for response GET "/api/orders" status 200
  see "No orders yet"
  see exactly 1 request GET "/api/orders"
```

Statements are specified in the [grammar](/probescript/grammar/#backend-responses). Because reading the recorded traffic needs agent
0.22+, upgrade `flutter_probe_agent` together with the CLI.
