package device

// Which other app holds the agent's port on an Android device (diagnostic for a failed connect). Every app on a device shares one loopback, so two apps that host
// the probe agent on the same port cannot both bind it: the second moves to the next port or fails, and the CLI, which talks to the port it forwards, reaches the
// first app with the second app's token. The owner is found without root and read-only: the shell user can read /proc/net/tcp6 and /proc/net/tcp (measured on
// Android 14 and 15 emulators, see testdata/portowner), which list every listening socket with the uid that owns it, and `pm list packages -U` maps a uid to its
// packages. run-as is not used, and nothing on the device is changed. When any of it cannot be read or does not look as expected the result is empty and the caller
// says nothing (or, in one case, a hedged line without names).

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	// portOwnerTimeout bounds the whole lookup (up to three adb calls).
	portOwnerTimeout = 5 * time.Second
	// procNetMaxBytes caps what is read from one file or command: a device with thousands of sockets or packages still fits, a runaway output does not.
	procNetMaxBytes = 1 << 20
	// procNetMaxLines caps the lines looked at in one output.
	procNetMaxLines = 20000
	// portOwnerMaxNames is how many package names one message names.
	portOwnerMaxNames = 3
	// firstAppUID is the first uid of an installed app (AID_APP_START); lower uids are the system, root and the shell, never "another app".
	firstAppUID = 10000
	// userUIDRange is how many uids one Android user spans: the uid of an app of user N is N*userUIDRange + appId.
	userUIDRange = 100000
)

// listeningUIDs returns, sorted and without repeats, the uids of the sockets in LISTEN state on port in the text of /proc/net/tcp or /proc/net/tcp6, bound to a
// loopback or the any address (the ones that block a bind to 127.0.0.1). Rows that do not look as expected are skipped.
func listeningUIDs(procNet string, port int) []int {
	if port < 1 || port > 65535 {
		return nil
	}
	want := fmt.Sprintf("%04X", port)
	seen := map[int]bool{}
	for n, line := range strings.Split(procNet, "\n") {
		if n >= procNetMaxLines {
			break
		}
		f := strings.Fields(line)
		// sl local_address rem_address st tx_queue:rx_queue tr:tm->when retrnsmt uid timeout inode ...
		if len(f) < 8 || f[3] != "0A" {
			continue
		}
		addr, p, ok := strings.Cut(f[1], ":")
		if !ok || !strings.EqualFold(p, want) || !blocksLoopbackBind(addr) {
			continue
		}
		uid, err := strconv.Atoi(f[7])
		if err != nil || uid < 0 || uid > 1<<30 {
			continue
		}
		seen[uid] = true
	}
	out := make([]int, 0, len(seen))
	for u := range seen {
		out = append(out, u)
	}
	sort.Ints(out)
	return out
}

// blocksLoopbackBind reports whether a listening address (as /proc/net prints it: hex, each 32-bit word in the machine's byte order, so little endian) stops
// another socket from binding 127.0.0.1 on the same port: the any address (0.0.0.0, ::), IPv4 loopback (127.x.x.x), IPv6 loopback (::1) and the IPv4-mapped forms
// of the any and the loopback address (what a Java or Dart socket bound to 127.0.0.1 shows as in tcp6).
func blocksLoopbackBind(addr string) bool {
	addr = strings.ToUpper(addr)
	switch len(addr) {
	case 8: // IPv4: one word
		return addr == "00000000" || strings.HasSuffix(addr, "7F")
	case 32: // IPv6: four words
		if addr == strings.Repeat("0", 32) || addr == "00000000000000000000000001000000" {
			return true
		}
		const mapped = "0000000000000000FFFF0000"
		return strings.HasPrefix(addr, mapped) && blocksLoopbackBind(addr[len(mapped):])
	}
	return false
}

// parsePackageUIDs reads the output of `pm list packages -U` ("package:<name> uid:<uid>", one per line) into a map from uid to the package names that share it.
// A name that is not a package name, a uid that is not a number and any other line are skipped.
func parsePackageUIDs(out string) map[int][]string {
	m := map[int][]string{}
	for n, line := range strings.Split(out, "\n") {
		if n >= procNetMaxLines {
			break
		}
		name, rest, ok := strings.Cut(strings.TrimSpace(line), " uid:")
		name = strings.TrimPrefix(name, "package:")
		if !ok || !ValidPackageName(name) {
			continue
		}
		// a shared uid can list several users (`uid:10207,10307`): the first number is the one of the current user, the others are not looked at
		first, _, _ := strings.Cut(strings.TrimSpace(rest), ",")
		uid, err := strconv.Atoi(first)
		if err != nil || uid < 0 || uid > 1<<30 {
			continue
		}
		m[uid] = append(m[uid], name)
	}
	return m
}

// PortOwners returns the package names of the OTHER apps (not ownApp) that listen on port on the device, at most portOwnerMaxNames, sorted. It returns nothing when
// no other app listens there, when the lookup cannot be made (the files are unreadable, the adb fails, the context is cancelled) or when the owner is not an app
// (the system or the shell). Read-only, three adb calls at most, bounded by portOwnerTimeout and by ctx.
func PortOwners(ctx context.Context, runner AdbRunner, serial string, port int, ownApp string) []string {
	if runner == nil || ctx.Err() != nil || !forwardSerialRE.MatchString(serial) || !ValidPackageName(ownApp) {
		return nil
	}
	cctx, cancel := context.WithTimeout(ctx, portOwnerTimeout)
	defer cancel()
	read := func(args ...string) string {
		out, err := runner.RunArgs(cctx, append([]string{"-s", serial, "shell"}, args...)...)
		if err != nil || len(out) > procNetMaxBytes {
			return ""
		}
		return string(out)
	}
	uids := listeningUIDs(read("cat", "/proc/net/tcp6"), port)
	uids = append(uids, listeningUIDs(read("cat", "/proc/net/tcp"), port)...)
	if len(uids) == 0 {
		return nil
	}
	pkgs := parsePackageUIDs(read("pm", "list", "packages", "-U"))
	own := map[int]bool{}
	for uid, names := range pkgs {
		for _, n := range names {
			if n == ownApp {
				own[uid] = true
			}
		}
	}
	seen := map[string]bool{}
	var names []string
	for _, uid := range uids {
		if uid%userUIDRange < firstAppUID || own[uid] {
			continue
		}
		for _, n := range pkgs[uid] {
			if n != ownApp && !seen[n] {
				seen[n] = true
				names = append(names, n)
			}
		}
	}
	sort.Strings(names)
	if len(names) > portOwnerMaxNames {
		names = names[:portOwnerMaxNames]
	}
	return names
}

// PortOwnerHint says which other app holds the agent's port on the device, or "" when that is not known (see PortOwners). The result starts with " — " to be
// appended to an error. devicePort is the port on the device, not the forwarded host port.
func PortOwnerHint(ctx context.Context, runner AdbRunner, serial string, devicePort int, ownApp string) string {
	names := PortOwners(ctx, runner, serial, devicePort, ownApp)
	switch len(names) {
	case 0:
		return ""
	case 1:
		return fmt.Sprintf(" — agent port %d on the device is held by %s (another app on this device): stop it, or give the app under test another port (--dart-define=PROBE_PORT=<n>, and agent.port / agent.device_port in probe.yaml to match)", devicePort, names[0])
	}
	return fmt.Sprintf(" — agent port %d on the device is held by one of %s (other apps on this device): stop it, or give the app under test another port (--dart-define=PROBE_PORT=<n>, and agent.port / agent.device_port in probe.yaml to match)", devicePort, strings.Join(names, ", "))
}

// NeutralPortHint is the line for a failed token read when the owner of the port could not be named: no names, only the possibility. It is used where the failure
// can have other causes too, so it says "if".
const NeutralPortHint = " — the agent did not answer: if another app on this device uses the same agent port (PROBE_PORT), stop it or set a different port"

// ConnectHint is what is appended to a failed connect on Android once the app is known to be running: the agent port forwarded to another device (ForwardHint), or,
// when that is not the case, another app holding the port on the device (PortOwnerHint). At most one of them, the first that can name something.
func ConnectHint(ctx context.Context, runner AdbRunner, serial string, hostPort, devicePort int, ownApp string) string {
	if h := ForwardHint(ctx, runner, hostPort, serial); h != "" {
		return h
	}
	return PortOwnerHint(ctx, runner, serial, devicePort, ownApp)
}
