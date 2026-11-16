# Real captures for the port-owner lookup

Made on 2026-11-16 on two emulators, as the shell user (`adb shell cat /proc/net/tcp6`, `cat /proc/net/tcp`, `pm list packages -U`), with two throwaway apps
(`dev.probe.portspikea` and `dev.probe.portspikeb`, not in this repository) that both try to bind 127.0.0.1:48911 (0xBF0F): `a` started first and holds it, `b`
fails with EADDRINUSE.

| Files | Device | Result |
|---|---|---|
| `*-api35.txt` | Android 15 (API 35) phone image (google_apis, arm64) | port 48911 listening in tcp6 as the IPv4-mapped loopback, owned by uid 10207 (`a`); `tcp` has only its header |
| `*-api34.txt` | Android 14 (API 34) TV image (arm64) | the same, owned by uid 10122 (`a`) |

`pm-list-U-*.txt` keeps only the system packages that matter to the tests and the two throwaway apps; the rest of the list was dropped. The socket tables are
complete (addresses, states and uids only).
