# Device view protocol (v1)

`probe device view` serves a live, interactive view of a Flutter app running on
an emulator, simulator or physical device. Studio and the VS Code extension use
it, and any tool can connect to it. It is a WebSocket server on the loopback
interface that streams screen frames and relays pointer and keyboard input to
the FlutterProbe agent in the app.

## Starting the server

```sh
probe device view --device <id>          # id from `probe device list`
probe device view --device <id> --port 48800
```

| Flag | Default | Meaning |
|------|---------|---------|
| `--device` | the only ready device | Device id. Several ready devices without `--device` is an error. |
| `--port` | `0` (free port) | Loopback port to listen on. |
| `--json` | `true` | Print the startup line as JSON on stdout. With `--json=false` the URL with the token is printed on stderr instead. |

The command connects to the agent the same way `probe test` does (adb forward or
iproxy, token read from the device), prints one line on stdout and runs until
you press Ctrl+C or send SIGTERM:

```json
{"url":"ws://127.0.0.1:51234/device/v1","token":"<random>","device":"emulator-5554"}
```

Connect to `<url>?token=<token>`. The token is random for each run (256 bits)
and appears only in that startup line. The command writes no files on the host.
The app must run with the agent enabled (`PROBE_AGENT=true`); input, hit testing
and the widget tree need an agent version that implements the RPCs listed under
"Agent requirements".

## Security model

* The listener binds `127.0.0.1` only.
* A missing or wrong token is answered with HTTP 401 before any upgrade.
  Tokens are compared in constant time.
* The `Host` header must be `127.0.0.1:<port>` or `localhost:<port>`, and a
  present `Origin` header must be one of `http://127.0.0.1:<port>`,
  `http://localhost:<port>`, `vscode-webview://<id>`, `wails://wails` or
  `http://wails.localhost`. Anything else gets HTTP 403. This stops a web page
  from reaching the server through DNS rebinding. Requests without an `Origin`
  header (not sent by browsers on cross-origin WebSocket connections) still need
  the token.
* Only one client is served at a time. A new connection replaces the current one,
  which is closed with code `4001`.
* Client messages are limited to 4 KiB and 50 per second.

Treat the token like a password for the device: anyone holding it can see the
screen and tap in the app while the command runs.

## Server to client

The first message is always the `hello`. It waits up to 3 seconds for the first
frame so it can carry the screen size and the detected capabilities. A `hello`
is sent again when either changes.

Text messages (JSON):

| Message | Fields |
|---------|--------|
| `{"t":"hello","v":1,"device":{"id","platform","name"},"screen":{"w","h","dpr"},"caps":{"input","hit","bounds","jpeg"}}` | `screen` is the Flutter view size in logical pixels. |
| `{"t":"status","state":"connected\|reconnecting\|error\|paused","msg":"...","warn":true}` | `warn` is present for problems with a client message (unknown type, rate limit, bad value); `state` then repeats the current state. |
| `{"t":"ack","id":<id>,"ok":bool,"err":"..."}` | Reply to an input message that carried an `id`. |
| `{"t":"hit","id":<id>,"node":Node\|null}` | Reply to a `hit` request. `err` is set when the request failed. |
| `{"t":"tree","seq":n,"nodes":[Node...]}` | Only with `caps.bounds` and after `config` `tree:true`. At most 2 per second, only when the tree changed. |

A frame is a binary message: byte 0 is `0x01`, bytes 1 to 4 are a big-endian
`uint32` N, the next N bytes are a UTF-8 JSON header
`{"seq":int,"ts":<ms since epoch>,"w":int,"h":int,"fmt":"jpeg"|"png"}`, and the
rest is the image. `w` and `h` are the pixel size of the image.

A `Node` is `{"i","p","type","key","text","label","x","y","w","h","interactive"}`:
`i` is the index in the list, `p` the parent index (`-1` for the root), and
`x,y,w,h` are logical pixels with the origin at the top-left of the Flutter view.

## Client to server

All messages are text JSON. `id` is optional on input messages, may be a number
or a short string, and is echoed in the reply.

| Message | Notes |
|---------|-------|
| `{"t":"tap","x","y"}` | |
| `{"t":"long_press","x","y","ms"}` | `ms` defaults to 600. |
| `{"t":"swipe","x1","y1","x2","y2","ms"}` | `ms` defaults to 300. |
| `{"t":"text","s":"..."}` | Types into the focused text field. At most 1000 characters. |
| `{"t":"hit","id":n,"x","y"}` | Replies with the deepest interactive or keyed widget at the point. |
| `{"t":"pause"}` / `{"t":"resume"}` | While paused (or with no client) the server makes no screenshot requests. |
| `{"t":"config","fps","quality","scale","tree"}` | `fps` 1 to 30 (default 10), `quality` 1 to 100 (default 60), `scale` 0.1 to 1.0 (default 1.0), `tree` boolean. Any field may be omitted; out-of-range values are ignored with a warning. |

Coordinates are Flutter logical pixels and must be finite numbers in `[0, 20000]`.
Invalid input is answered with a failed `ack` (or a `status` warning without an
`id`). Unknown message types are ignored with a warning. A message above 4 KiB
closes the connection with code 1009. Messages over the rate limit are dropped
and reported.

## Flow control

The server keeps only the latest frame for the client. If the client reads
slowly it receives fewer frames, never a growing backlog. Frames are captured by
calling the agent's screenshot RPC at most `fps` times per second, as JPEG when
the agent supports it and PNG otherwise (`caps.jpeg`).

## Agent requirements and degradation

The server uses these agent RPCs; coordinates are logical pixels.

| RPC | Purpose |
|-----|---------|
| `probe.screenshot` with `{"format","quality","scale","inline":true}` | Returns `{"image":"<base64>","format","width","height","logicalWidth","logicalHeight","dpr"}`. |
| `probe.tap_at`, `probe.long_press_at`, `probe.swipe_at`, `probe.type_text` | Input. |
| `probe.hit_test` | Hover and selection. |
| `probe.dump_tree` with `{"bounds":true}` | Widget bounds for the overlay. |

On connect the server checks which of these exist using calls without side
effects (an empty text entry, a hit test at the origin, a bounds dump). A
"method not found" answer switches the matching feature off in `caps`: without
the input RPCs the view is read-only and input messages are answered with a
failed `ack`. An agent that ignores the `format` option is detected from the
reply and PNG frames are used. An agent that does not support `inline` cannot
stream; the server reports an `error` status asking for a `probe_agent` update.

If the connection to the agent drops, the server sends `reconnecting` status
messages, re-reads the token and reconnects with a growing delay, then sends
`connected` and resumes the stream.
