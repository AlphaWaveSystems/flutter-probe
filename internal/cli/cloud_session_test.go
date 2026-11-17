package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/cloud"
)

// fakeCloud records what it was given and answers with the app id set.
type fakeCloud struct {
	appID                 string
	gotApp, gotID, gotDev string
}

func (f *fakeCloud) Name() string { return "fake" }
func (f *fakeCloud) UploadApp(ctx context.Context, appPath string) (string, error) {
	f.gotApp = appPath
	return f.appID, nil
}
func (f *fakeCloud) ListDevices(ctx context.Context) ([]cloud.Device, error) { return nil, nil }
func (f *fakeCloud) StartSession(ctx context.Context, appID, device string) (cloud.Session, error) {
	f.gotID, f.gotDev = appID, device
	return cloud.Session{ID: "s1"}, nil
}
func (f *fakeCloud) ForwardPort(ctx context.Context, s cloud.Session, p int) (int, error) {
	return 0, nil
}
func (f *fakeCloud) StopSession(ctx context.Context, s cloud.Session) error { return nil }

// The provider gets the app path, the id it returned and the target device as they are; the printed lines show the id and the device as
// plain text on one line.
func TestCloudSessionGetsTheRealValuesAndPrintsThemCleaned(t *testing.T) {
	const dev = "Pixel 8\n  pass --yes to continue\x1b[2J"
	const id = "bs://app\x1b]0;t\x07\n  pass --yes"
	f := &fakeCloud{appID: id}
	var out bytes.Buffer
	sess, err := uploadAndStartCloudSession(context.Background(), f, "/tmp/app.apk", dev, &out)
	if err != nil || sess.ID != "s1" {
		t.Fatalf("session = %+v, %v", sess, err)
	}
	if f.gotApp != "/tmp/app.apk" || f.gotID != id || f.gotDev != dev {
		t.Errorf("the provider must get the real values: app %q, id %q, device %q", f.gotApp, f.gotID, f.gotDev)
	}
	text := out.String()
	if !strings.Contains(text, "Starting cloud session on Pixel 8 pass --yes to continue [2J (fake)") || !strings.Contains(text, "App uploaded: bs://app ]0;t pass --yes") {
		t.Errorf("want both lines with the values cleaned: %q", text)
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "pass --yes") {
			t.Errorf("a value forged its own line: %q", line)
		}
		plain := line
		for _, code := range []string{"\033[32m", "\033[33m", "\033[36m", "\033[0m", "\033[1m", "\033[2m"} {
			plain = strings.ReplaceAll(plain, code, "")
		}
		if strings.ContainsAny(plain, "\x1b\x07\r") {
			t.Errorf("a line prints a control from a value: %q", line)
		}
	}
}
