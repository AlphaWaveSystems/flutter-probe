Captured on an Android 15 (API 35) emulator with the fixture app `dev.probe.webviewfixture` (a throwaway app of this project), by
`adb -s <serial> shell dumpsys activity exit-info <package>` and `adb -s <serial> logcat -b crash -d`. Nothing else was edited, except where said here.

- `exit-java.txt`, `crashbuf-java.txt`: an uncaught Java exception thrown in the activity's onCreate (a fake token in the message).
- `exit-segv.txt`, `crashbuf-segv.txt`: SIGSEGV sent to the app's own process (a native crash with the system's tombstone lines).
- `exit-webview-and-stop.txt`: exit-info with other processes of the app's uid (WebView sandbox) and an earlier force-stop.
- `crashbuf-webview-crashpad-noise.txt`: the crash buffer after `am crash` hit the WebView's sandbox process; trimmed to the first 16 lines and the first
  four crashpad lines (cut at 260 characters), because the real buffer was 57 KB of minidump text.
