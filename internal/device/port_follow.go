package device

// Following the agent to the port it really listens on (Android).
//
// Two apps on one device that host the probe agent on the same port cannot both bind it: the agent of the second app moves to the next free port (PROBE_PORT_BUSY,
// at most a few above the configured one) and the CLI, which forwards the configured port, reaches the first app's agent with the second app's token (a 401) or
// reaches nothing. See port_owner.go for the diagnostic that names the holder. This file finds the port the app under test really listens on and moves the adb
// forward to it, so the run goes on against the right app.
//
// The rules, so that it cannot connect a run to the wrong app:
//   - It runs only after a dial has failed (a 401, or a connection that does not complete), never on the first dial: on the happy path nothing here makes an adb call.
//   - The port is found by the uid of the listening socket, which must be exactly the uid of the app's package (`pm list packages -U`), and the package must be the
//     only one with that uid (a shared uid proves nothing). The candidates are the ports [base, base+portFollowRange) of the configured port; exactly one must
//     belong to the app (or the configured one is among them, and then nothing moves). None, or several without the configured one, change nothing.
//   - A token that did not come from run-as (a file or a log shared by every app) does not prove the app, so before the first dial the port is checked. When another
//     app holds it, the run does not dial it: the follower looks for the app's own port for the whole dial window (the app may still be starting), and when it finds
//     none the connect is REFUSED with a PortHeldError that names the holder. Failing closed is the point: the token may be the holder's, and dialing it would run the
//     tests against the wrong app without a sign of it.
//   - The same check runs again for a token that was read again after the agent rejected one, when that token is not from run-as (CheckRefreshedToken): on a cold
//     start the run-as read can fail and fall back to a shared source, and the dial loop stops on the refusal (probelink: RefuseDial) instead of dialing the holder.
//   - Nothing is shared between connections: every connection (every lane of a parallel run) has its own PortFollower.
//
// What it does not cover: when /proc/net cannot be read, nobody can say who holds the port, so nothing is refused; with an agent too old to move its port and an app that
// is not debuggable (no run-as), the token and the port can then both belong to another app, and nothing here can tell. Studio, `probe record`, the device view, the
// Wi-Fi target of Studio, the cloud path and the first connect of a composite test do not use the follower (see the CHANGELOG).

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// portFollowRange is how many ports from the configured one are looked at: the agent tries the next ones when its port is taken.
	portFollowRange = 10
	// portFollowInterval is the least time between two lookups of one follower, so a dial that keeps failing does not run adb every second.
	portFollowInterval = 3 * time.Second
	// preflightRetryInterval is how long Preflight waits between two looks for the app's own port while the dial window lasts.
	preflightRetryInterval = time.Second
)

// PortHeldError says that another app holds the agent port and the app under test has opened none (yet), with a token that does not prove which app it belongs to.
// The connect is refused instead of dialed: it could reach the other app.
type PortHeldError struct {
	Port    int
	Holders []string
	App     string
}

// RefuseDial tells the dial loop of probelink that this error is final: when a token refresh returns it, the dial stops instead of trying again.
func (e *PortHeldError) RefuseDial() bool { return true }

func (e *PortHeldError) Error() string {
	return fmt.Sprintf("agent port %d on the device is held by %s, and %s has not opened an agent port; not connecting, because the token read here may belong to %s: stop it, or give the app under test another port (--dart-define=PROBE_PORT=<n>, and agent.port / agent.device_port in probe.yaml to match)",
		e.Port, strings.Join(e.Holders, ", "), e.App, strings.Join(e.Holders, ", "))
}

// listeningPortsOf returns, sorted and without repeats, the ports in [lo, hi) with a socket in LISTEN state, bound to a loopback or the any address, in the text of
// /proc/net/tcp or /proc/net/tcp6, whose owner is uid.
func listeningPortsOf(procNet string, uid, lo, hi int) []int {
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
		addr, hexPort, ok := strings.Cut(f[1], ":")
		if !ok || !blocksLoopbackBind(addr) {
			continue
		}
		p, err := strconv.ParseInt(hexPort, 16, 32)
		if err != nil || int(p) < lo || int(p) >= hi {
			continue
		}
		u, err := strconv.Atoi(f[7])
		if err != nil || u != uid {
			continue
		}
		seen[int(p)] = true
	}
	out := make([]int, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Ints(out)
	return out
}

// appUID returns the uid of the package appID from the parsed `pm list packages -U`, and whether that uid belongs to that package alone. A package that is not
// listed, or one that shares its uid with another package, gives false.
func appUID(pkgs map[int][]string, appID string) (int, bool) {
	uid, found := 0, false
	for u, names := range pkgs {
		for _, n := range names {
			if n != appID {
				continue
			}
			if len(names) != 1 || found {
				return 0, false
			}
			uid, found = u, true
		}
	}
	return uid, found
}

// DiscoverAppPort looks for the port the app appID listens on, among the ports [devicePort, devicePort+portFollowRange) of the device. It returns the port, and
// ok when exactly one port there belongs to the app, or when the app listens on devicePort itself (the answer is then devicePort). It returns ok=false, with the
// reason in why, when the app listens nowhere in the range, on several ports without devicePort, has no usable package uid, or when the files cannot be read.
// Read-only, three adb calls at most, bounded by portOwnerTimeout and ctx.
func DiscoverAppPort(ctx context.Context, runner AdbRunner, serial string, devicePort int, appID string) (port int, ok bool, why string) {
	if runner == nil || ctx.Err() != nil || !forwardSerialRE.MatchString(serial) || !ValidPackageName(appID) {
		return 0, false, "not a device or app this can look at"
	}
	if devicePort < 1 || devicePort >= 65535-portFollowRange {
		return 0, false, "the agent port is out of range"
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
	pkgs := parsePackageUIDs(read("pm", "list", "packages", "-U"))
	uid, single := appUID(pkgs, appID)
	if !single {
		return 0, false, "the app has no package uid of its own"
	}
	lo, hi := devicePort, devicePort+portFollowRange
	ports := listeningPortsOf(read("cat", "/proc/net/tcp6"), uid, lo, hi)
	ports = append(ports, listeningPortsOf(read("cat", "/proc/net/tcp"), uid, lo, hi)...)
	ports = uniqueSorted(ports)
	for _, p := range ports {
		if p == devicePort {
			return devicePort, true, ""
		}
	}
	switch len(ports) {
	case 0:
		return 0, false, "the app listens on none of the ports"
	case 1:
		return ports[0], true, ""
	}
	return 0, false, "the app listens on several of the ports"
}

func uniqueSorted(in []int) []int {
	sort.Ints(in)
	out := in[:0]
	for i, p := range in {
		if i == 0 || p != in[i-1] {
			out = append(out, p)
		}
	}
	return out
}

// PortForwarder moves the adb forward of the agent port. *Manager is one.
type PortForwarder interface {
	ForwardPort(ctx context.Context, serial string, hostPort, devicePort int) error
}

// PortFollower keeps the device port of one connection: the configured one until a failed dial shows that the app listens on another, and then that one. It is for
// one connection (one serial, one host port) and holds no state shared with another.
type PortFollower struct {
	runner   AdbRunner
	fwd      PortForwarder
	serial   string
	appID    string
	hostPort int
	base     int
	notice   func(string)
	onChange func(devicePort int)

	mu         sync.Mutex
	current    int
	source     TokenSource
	lastLookup time.Time
	now        func() time.Time
	sleep      func(ctx context.Context, d time.Duration)
}

// NewPortFollower returns a follower for the app appID on the device serial, whose agent port on the device is devicePort and is forwarded from hostPort. notice, when
// not nil, receives the one line said when the port moves; onChange, when not nil, receives the new device port, so that the caller's own copy of it follows.
func NewPortFollower(runner AdbRunner, fwd PortForwarder, serial, appID string, hostPort, devicePort int, notice func(string), onChange func(int)) *PortFollower {
	return &PortFollower{runner: runner, fwd: fwd, serial: serial, appID: appID, hostPort: hostPort, base: devicePort, current: devicePort, notice: notice, onChange: onChange, now: time.Now, sleep: sleepCtx}
}

// sleepCtx waits for d or for ctx to end.
func sleepCtx(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

// DevicePort is the port on the device the forward should point at now.
func (f *PortFollower) DevicePort() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.current
}

// NoteTokenSource records where the last token came from.
func (f *PortFollower) NoteTokenSource(src TokenSource) {
	f.mu.Lock()
	f.source = src
	f.mu.Unlock()
}

// Preflight runs once before the first dial. A token read through run-as proves the app, so nothing is checked and no adb call is made. Any other source does not,
// and then the run must not dial a port that another app holds: the follower looks for the app's own port, again every second until window has passed (the app may
// still be starting), and moves the forward when it finds it. When it does not, Preflight returns a *PortHeldError and the caller must not dial. It returns true when
// the forward was moved, and (false, nil) when nobody else holds the port or when that cannot be read. A cancelled ctx gives its error.
func (f *PortFollower) Preflight(ctx context.Context, window time.Duration) (bool, error) {
	f.mu.Lock()
	src, cur := f.source, f.current
	f.mu.Unlock()
	if src == TokenSourceRunAs {
		return false, nil
	}
	deadline := f.now().Add(window)
	for {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		holders := PortOwners(ctx, f.runner, f.serial, cur, f.appID)
		if len(holders) == 0 {
			return false, nil
		}
		if f.follow(ctx) {
			return true, nil
		}
		if !f.now().Before(deadline) {
			return false, &PortHeldError{Port: cur, Holders: holders, App: f.appID}
		}
		f.sleep(ctx, preflightRetryInterval)
	}
}

// CheckRefreshedToken is for a token that was read again after the agent rejected one (the RefreshToken of the dial). Right after a cold start the run-as token can be
// rejected and the read falls back to a shared source (a file, the log): that token may be the other app's, so, as before the first dial, the holder check runs again and
// the redial does not happen against a port another app holds. window is how long to look for the app's own port; when ctx has a deadline (the dial's) it is the time
// left in it, less one retry. It returns a *PortHeldError when the redial must not happen. A run-as token needs nothing.
func (f *PortFollower) CheckRefreshedToken(ctx context.Context, src TokenSource, window time.Duration) error {
	f.NoteTokenSource(src)
	if src == TokenSourceRunAs {
		return nil
	}
	if dl, ok := ctx.Deadline(); ok {
		window = time.Until(dl) - preflightRetryInterval
		if window < 0 {
			window = 0
		}
	}
	_, err := f.Preflight(ctx, window)
	return err
}

// Suspect is for a dial that failed: the port may belong to another app. It looks for the app's own port and moves the forward when it finds exactly one other. It
// returns true when the forward was moved. It looks at most once in portFollowInterval.
func (f *PortFollower) Suspect(ctx context.Context) bool {
	f.mu.Lock()
	if !f.lastLookup.IsZero() && f.now().Sub(f.lastLookup) < portFollowInterval {
		f.mu.Unlock()
		return false
	}
	f.lastLookup = f.now()
	f.mu.Unlock()
	return f.follow(ctx)
}

func (f *PortFollower) follow(ctx context.Context) bool {
	f.mu.Lock()
	cur := f.current
	f.mu.Unlock()
	port, ok, _ := DiscoverAppPort(ctx, f.runner, f.serial, f.base, f.appID)
	if !ok || port == cur {
		return false
	}
	var holder string
	if names := PortOwners(ctx, f.runner, f.serial, cur, f.appID); len(names) > 0 {
		holder = " is held by " + strings.Join(names, ", ")
	} else {
		holder = " is not the app's"
	}
	if err := f.fwd.ForwardPort(ctx, f.serial, f.hostPort, port); err != nil {
		return false
	}
	f.mu.Lock()
	f.current = port
	f.mu.Unlock()
	if f.onChange != nil {
		f.onChange(port)
	}
	if f.notice != nil {
		f.notice(fmt.Sprintf("agent port %d on the device%s; using %d for %s", cur, holder, port, f.appID))
	}
	return true
}
