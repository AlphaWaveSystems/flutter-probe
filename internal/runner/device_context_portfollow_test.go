package runner

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/device"
)

// procAdb answers the three read-only commands of the port lookup, and nothing else.
type procAdb struct{ tcp6, pm string }

func (a procAdb) RunArgs(ctx context.Context, args ...string) ([]byte, error) {
	line := strings.Join(args, " ")
	switch {
	case strings.HasSuffix(line, "cat /proc/net/tcp6"):
		return []byte(a.tcp6), nil
	case strings.HasSuffix(line, "cat /proc/net/tcp"):
		return []byte("  sl  local_address rem_address st\n"), nil
	case strings.HasSuffix(line, "pm list packages -U"):
		return []byte(a.pm), nil
	}
	return nil, fmt.Errorf("unexpected command: %s", line)
}

type recordingForwarder struct{ calls [][3]int }

func (f *recordingForwarder) ForwardPort(ctx context.Context, serial string, hostPort, devicePort int) error {
	f.calls = append(f.calls, [3]int{hostPort, devicePort, 0})
	return nil
}

// bareContext is a DeviceContext without the device lookup NewDeviceContext makes (it runs adb): these tests must pass on a machine that has none.
func bareContext(m *device.Manager, platform device.Platform, serial string) *DeviceContext {
	return &DeviceContext{Manager: m, Platform: platform, Serial: serial}
}

func listenRow(port, uid int) string {
	return fmt.Sprintf("   0: 0000000000000000FFFF00000100007F:%04X 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000 %5d        0 1 1 0000000000000000 100 0 0 10 0\n", port, uid)
}

func TestDeviceContextHasAFollowerOnlyOnAndroidWithAnAdbForward(t *testing.T) {
	m := device.NewManager()
	android := bareContext(m, device.PlatformAndroid, "emulator-5554")
	android.AppID, android.Port, android.DevicePort = "com.example.app", 48686, 48911
	f := android.portFollower()
	if f == nil || f.DevicePort() != 48911 {
		t.Fatalf("an Android context needs a follower on its configured device port 48911, got %v", f)
	}
	if android.portFollower() != f {
		t.Errorf("the follower must be made once and reused")
	}
	if android.reconnectDialOptions("tok").OnPortSuspect == nil {
		t.Errorf("an Android reconnect must carry the port hook")
	}

	noDevicePort := bareContext(m, device.PlatformAndroid, "emulator-5554")
	noDevicePort.Port = 48686
	if f := noDevicePort.portFollower(); f == nil || f.DevicePort() != 48686 {
		t.Errorf("without a device port the follower starts at the host port 48686, got %v", f)
	}

	ios := bareContext(m, device.PlatformIOS, "UDID")
	if ios.portFollower() != nil || ios.reconnectDialOptions("tok").OnPortSuspect != nil {
		t.Errorf("an iOS context must not have a follower or a port hook")
	}
	http := bareContext(m, device.PlatformAndroid, "emulator-5554")
	http.UseHTTP = true
	if http.portFollower() != nil {
		t.Errorf("an agent reached over HTTP has no adb forward to move")
	}
	noManager := bareContext(nil, device.PlatformAndroid, "emulator-5554")
	if noManager.portFollower() != nil {
		t.Errorf("without a device manager there is no follower")
	}
}

// The device port every later step uses (re-forward, reconnect, hints) is the one the follower found, not the configured one.
func TestDeviceContextDevicePortFollowsTheFollower(t *testing.T) {
	dc := bareContext(device.NewManager(), device.PlatformAndroid, "emulator-5554")
	dc.AppID, dc.Port, dc.DevicePort = "com.second", 48686, 48911
	if dc.devicePort() != 48911 {
		t.Fatalf("devicePort = %d before any follower, want 48911", dc.devicePort())
	}
	adb := procAdb{
		tcp6: "  sl local_address rem_address st\n" + listenRow(48911, 10100) + listenRow(48912, 10200),
		pm:   "package:com.first uid:10100\npackage:com.second uid:10200\n",
	}
	fwd := &recordingForwarder{}
	var notices []string
	dc.Follower = device.NewPortFollower(adb, fwd, "emulator-5554", "com.second", 48686, 48911, func(s string) { notices = append(notices, s) }, nil)
	if dc.devicePort() != 48911 {
		t.Errorf("devicePort = %d before the follower moved, want 48911", dc.devicePort())
	}
	if !dc.Follower.Suspect(context.Background()) {
		t.Fatalf("the follower did not find the app's port")
	}
	if dc.devicePort() != 48912 {
		t.Errorf("devicePort = %d after the follower moved, want 48912", dc.devicePort())
	}
	if len(fwd.calls) != 1 || fwd.calls[0][0] != 48686 || fwd.calls[0][1] != 48912 {
		t.Errorf("forward calls = %v, want one: host 48686 -> device 48912", fwd.calls)
	}
}

// Two lanes of a parallel run have their own followers: one finding a port says nothing about the other.
func TestLanesHaveSeparateFollowers(t *testing.T) {
	m := device.NewManager()
	a := bareContext(m, device.PlatformAndroid, "emulator-5554")
	b := bareContext(m, device.PlatformAndroid, "emulator-5556")
	for _, dc := range []*DeviceContext{a, b} {
		dc.AppID, dc.Port, dc.DevicePort = "com.example.app", 48686, 48911
	}
	if a.portFollower() == b.portFollower() {
		t.Errorf("two lanes share one follower")
	}
}
