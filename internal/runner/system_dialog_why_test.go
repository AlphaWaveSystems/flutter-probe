package runner

import (
	"context"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/sysdialog"
)

type diagDriver struct {
	sysdialog.Driver // unused methods stay nil
	got              [2]string
}

func (d *diagDriver) Diagnose(_ context.Context, appID, title string) string {
	d.got = [2]string{appID, title}
	return "focused window: dev.example.app/.MainActivity; POST_NOTIFICATIONS: denied"
}

type plainDriver struct{ sysdialog.Driver }

func TestWhyNoDialogAddsTheDeviceFindingsToATimedOutWait(t *testing.T) {
	e := &Executor{deviceCtx: &DeviceContext{AppID: "dev.example.app"}}
	d := &diagDriver{}
	got := e.whyNoDialog(context.Background(), d, parser.SystemDialogStep{Appear: true, Title: "notifications"})
	if !strings.Contains(got, "POST_NOTIFICATIONS: denied") || !strings.HasPrefix(got, " [") || d.got != [2]string{"dev.example.app", "notifications"} {
		t.Fatalf("got %q, asked %v", got, d.got)
	}
	// Waiting for a dialog to disappear, or a driver that cannot diagnose (iOS), adds nothing.
	if got := e.whyNoDialog(context.Background(), d, parser.SystemDialogStep{Appear: false}); got != "" {
		t.Fatalf("disappear wait: %q", got)
	}
	if got := e.whyNoDialog(context.Background(), plainDriver{}, parser.SystemDialogStep{Appear: true}); got != "" {
		t.Fatalf("driver without Diagnose: %q", got)
	}
}
