# Follow-up: host adb server forensics + a live guest-adbd wedge reproduction

Continuation of this folder's investigation, prompted by reviewing the
[SDK platform-tools release notes](https://developer.android.com/tools/releases/platform-tools)
for anything usable against #237. Two concrete findings, one environmental recommendation.

## 1. The host adb server closes forwarded sockets on a flush timeout — with timestamps matching every observed probe WS death

`adb-server-host.log` (this folder — the host machine's own adb server log,
`$TMPDIR/adb.501.log`, preserved verbatim) shows repeated:

```
W adb : sockets.cpp:310 timeout expired while flushing socket, closing
```

The adb server buffers data for each forwarded socket; when it can't flush to the
slower side within its internal timeout, it **closes the socket itself** — the
tunneled peer sees exactly `use of closed network connection` / WS `close 1006`,
while both endpoint processes stay healthy. This is the missing middle-man
mechanism the earlier investigation's loopback experiments could never trigger:
**without adb in the path, a stalled reader just causes TCP backpressure; with adb
in the path, a stalled reader gets the connection killed.**

Timestamp correlation (same host, 2026-08-14): the log has single flush-timeout
closes at **07:10, 07:12, 07:13, and 07:24** — each matching a probe WS
`unexpected EOF` / dead-forward failure hit live during that morning's benchmark
and evidence sessions, plus a ~200-event burst at 08:09–08:10 during the
heaviest screenshot/Maestro-driver churn. Every observed connection death has a
matching adb-server-side close event; none required either endpoint to fail.

## 2. Controlled experiment: a stalled forwarded stream can wedge the emulator's guest adbd into a permanent `offline` — reproducing B-3's finding with zero Maestro involvement

Attempting to measure the flush timeout directly (device-side `nc` listener with
a deliberately stalled consumer, host-side sustained writer through
`adb forward tcp:9999 tcp:9999`) produced something more interesting than a
number: after a ~2 GB blast through the forward plus a second stalled-listener
setup, the emulator's transport wedged entirely —

- every `adb shell` invocation hung (while `adb devices` still answered),
- `adb reconnect` moved the device to `offline` and it **stayed** offline,
- `adb kill-server` + restart did **not** recover it,
- only killing the emulator process itself and cold-booting recovered.

Caveat, stated honestly: the wedge followed a sequence of three socket
manipulations, so this doesn't isolate the single triggering action. What it
does establish: **guest-side adbd can be wedged into the exact
device-offline-and-stays-offline state B-3's benchmark observed, by transport
abuse alone, with no Maestro involved** — further confirming G-1's decision to
retire "Maestro destabilizes the device" as a claim. Tools that hammer adb
harder (Maestro's driver install/uninstall cycle, screenshot streams) simply
have more opportunities to hit this than tools that don't.

## 3. Platform-tools review (the actual prompt for this follow-up)

Installed here: **37.0.0** (Feb 2026). Latest: **37.0.1** (July 2026).

- Directly relevant history: 28.0.1 fixed "a file descriptor double-close ...
  resulting in connections being closed when an `adb connect` happens
  simultaneously" and added 60s TCP reconnection; 28.0.2 fixed "flakiness of
  `adb shell` port forwarding that leads to 'Connection reset by peer'";
  35.0.2 fixed an mDNS backend bug "bringing down server on truncated query".
  Precedent is clear that this class of bug lives in adb and gets fixed there.
- 37.0.0 (the version in use) switched the default mDNS backend to the new
  `libadbmdns` — a fresh backend in the same component that historically took
  the whole server down. No smoking gun in our log tying mDNS to the closes,
  but worth knowing which new subsystem shipped in the exact version in use.
- 37.0.1's changes are USB/Windows-focused — nothing that targets forwarded-TCP
  flush behavior. Upgrading is cheap hygiene, not a fix.

## What this means for #237

- The most likely drop mechanism at NECT's sign-in-churn → `goto feed`
  transition: device-side load stalls the forwarded WS stream long enough for
  the host adb server's flush timeout to close it (§1), with §2's guest-adbd
  wedge as the escalated form when it happens harder.
- **The decisive next check is nearly free**: during the next live NECT repro,
  tail `$TMPDIR/adb.<uid>.log` alongside — if `sockets.cpp:310 timeout expired
  while flushing socket, closing` lands at the moment of the drop, the
  mechanism is confirmed end-to-end.
- Probe-side, the right posture is already in place as of v0.12.0: treat the
  adb transport as a thing that kills healthy connections under load, and make
  recovery actually work (#238's retry-dispatch + conditional fixes, plus
  `Reconnect`'s existing re-forward). A drop should now cost one reconnect
  cycle. What probe cannot fix from its side is §2's guest-adbd wedge — if the
  transport reaches that state, no amount of CLI-side reconnecting helps, and
  the honest failure mode is a clear error, not a retry loop.
