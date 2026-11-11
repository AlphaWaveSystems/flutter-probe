package runner

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/sysdialog"
)

// The App Tracking Transparency alert has an Allow button and a decline button with the deny role, so role matching
// alone reads it as the notification alert. The labels are the buttons in six locales, as TCC.framework of the iOS 26.5
// runtime has them: the ATT alert (allow and its decline) and the notification alert (allow and Don't Allow).
var attAlerts = []struct{ locale, allow, decline string }{
	{"en", "Allow", "Ask App Not to Track"},
	{"de", "Erlauben", "App-Tracking ablehnen"},
	{"fr", "Autoriser", "Demander à l’app de ne pas me suivre"},
	{"ja", "許可", "アプリにトラッキングしないように要求"},
	{"ar", "السماح", "مطالبة التطبيق بعدم التتبع"},
	{"ru", "Разрешить", "Попросить не\u00a0отслеживать"},
}

var notificationAlerts = []struct{ locale, allow, deny string }{
	{"en", "Allow", "Don't Allow"},
	{"de", "Erlauben", "Nicht erlauben"},
	{"fr", "Autoriser", "Refuser"},
	{"ja", "許可", "許可しない"},
	{"ar", "السماح", "عدم السماح"},
	{"ru", "Разрешить", "Запретить"},
}

// trackingWarning is the end of the line printed when the wait ends with an ATT alert still on screen.
const trackingWarning = "an App Tracking Transparency alert is showing, so the notification alert was not answered"

// multiSys is a fake driver that shows several dialogs at once, which fakeSys does not.
type multiSys struct {
	fakeSys
	dialogs []sysdialog.Dialog
}

func (m *multiSys) Dialogs(context.Context) ([]sysdialog.Dialog, error) {
	return m.dialogs, nil
}

// captureStdout returns what run writes to standard output.
func captureStdout(t *testing.T, run func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stdout
	os.Stdout = w
	got := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		got <- string(b)
	}()
	func() {
		defer func() {
			os.Stdout = saved
			w.Close()
		}()
		run()
	}()
	return <-got
}

// TestNotificationAlertIgnoresTheATTPrompt: with an ATT alert on screen, neither Allow nor Don't Allow for the
// notification permission taps anything, whatever the order of its buttons.
func TestNotificationAlertIgnoresTheATTPrompt(t *testing.T) {
	ctx := context.Background()
	for _, a := range attAlerts {
		for _, buttons := range [][]string{{a.decline, a.allow}, {a.allow, a.decline}} {
			for _, button := range []string{"Allow", "Don't Allow"} {
				t.Run(a.locale+" "+button, func(t *testing.T) {
					f := &fakeSys{dialog: &sysdialog.Dialog{Buttons: buttons}}
					dc := &DeviceContext{sysDriver: f}
					if err := dc.answerNotificationAlertWithin(ctx, button, 0); err != nil {
						t.Fatal(err)
					}
					if len(f.tapped) != 0 {
						t.Errorf("tapped %q on the ATT alert %q", f.tapped, buttons)
					}
				})
			}
		}
	}
}

// TestNotificationAlertIsStillAnswered: a real notification alert is answered with the button asked for, in both
// directions, in each of the locales.
func TestNotificationAlertIsStillAnswered(t *testing.T) {
	ctx := context.Background()
	for _, n := range notificationAlerts {
		for _, button := range []string{"Allow", "Don't Allow"} {
			t.Run(n.locale+" "+button, func(t *testing.T) {
				f := &fakeSys{dialog: &sysdialog.Dialog{Buttons: []string{n.deny, n.allow}}}
				dc := &DeviceContext{sysDriver: f}
				if err := dc.answerNotificationAlertWithin(ctx, button, 0); err != nil {
					t.Fatal(err)
				}
				if len(f.tapped) != 1 || f.tapped[0] != button {
					t.Errorf("tapped = %q, want [%s]", f.tapped, button)
				}
			})
		}
	}
}

// TestNotificationAlertIsNotALocationAlert: the allow-once rule keeps a dialog with Allow and Don't Allow from being
// taken for the notification alert. The real location alert has no plain Allow button, so it passes without that rule
// and is kept as a sanity case. The second dialog has a plain Allow beside Allow Once, and fails without the rule.
func TestNotificationAlertIsNotALocationAlert(t *testing.T) {
	cases := []struct {
		name    string
		buttons []string
	}{
		{"location alert", []string{"Allow Once", "Allow While Using App", "Don't Allow"}},
		{"plain Allow beside Allow Once", []string{"Allow", "Allow Once", "Don't Allow"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if isNotificationAlert([]sysdialog.Dialog{{Buttons: tc.buttons}}) {
				t.Errorf("%q was taken for the notification alert", tc.buttons)
			}
			f := &fakeSys{dialog: &sysdialog.Dialog{Buttons: tc.buttons}}
			dc := &DeviceContext{sysDriver: f}
			if err := dc.answerNotificationAlertWithin(context.Background(), "Don't Allow", 0); err != nil {
				t.Fatal(err)
			}
			if len(f.tapped) != 0 {
				t.Errorf("tapped %q on %q", f.tapped, tc.buttons)
			}
		})
	}
}

// TestNotificationAlertWaitsWhileATTIsShowing: when both alerts are on screen, the notification alert is not picked
// until the tracking prompt is gone, because a tap by label could land on the tracking alert's Allow.
func TestNotificationAlertWaitsWhileATTIsShowing(t *testing.T) {
	att := sysdialog.Dialog{Buttons: []string{"Ask App Not to Track", "Allow"}}
	notification := sysdialog.Dialog{Buttons: []string{"Don't Allow", "Allow"}}
	if isNotificationAlert([]sysdialog.Dialog{att, notification}) {
		t.Error("the notification alert was picked while the ATT alert was showing")
	}
	if !isNotificationAlert([]sysdialog.Dialog{notification}) {
		t.Error("a notification alert on its own was not picked")
	}
}

// TestNotificationAlertWithATTAlertInEitherOrder: the notification alert and the ATT alert on screen together are not
// answered, whichever is listed first, and the path says why it did not answer.
func TestNotificationAlertWithATTAlertInEitherOrder(t *testing.T) {
	att := sysdialog.Dialog{Buttons: []string{"Ask App Not to Track", "Allow"}}
	notification := sysdialog.Dialog{Buttons: []string{"Don't Allow", "Allow"}}
	orders := []struct {
		name    string
		dialogs []sysdialog.Dialog
	}{
		{"notification first", []sysdialog.Dialog{notification, att}},
		{"ATT first", []sysdialog.Dialog{att, notification}},
	}
	for _, order := range orders {
		t.Run(order.name, func(t *testing.T) {
			for _, button := range []string{"Allow", "Don't Allow"} {
				f := &multiSys{fakeSys: fakeSys{dialog: &order.dialogs[0]}, dialogs: order.dialogs}
				dc := &DeviceContext{sysDriver: f}
				var err error
				out := captureStdout(t, func() {
					err = dc.answerNotificationAlertWithin(context.Background(), button, 0)
				})
				if err != nil {
					t.Fatal(err)
				}
				if len(f.tapped) != 0 {
					t.Errorf("%s: tapped %q", button, f.tapped)
				}
				if !strings.Contains(out, trackingWarning) {
					t.Errorf("%s: the warning is missing, output = %q", button, out)
				}
			}
		})
	}
}

// TestNotificationAlertWarnsOnlyWhileATTIsShowing: no alert at all is silent, and an ATT alert still up when the wait
// ends gets the warning.
func TestNotificationAlertWarnsOnlyWhileATTIsShowing(t *testing.T) {
	cases := []struct {
		name   string
		dialog *sysdialog.Dialog
		warn   bool
	}{
		{"no alert", nil, false},
		{"ATT alert", &sysdialog.Dialog{Buttons: []string{"Ask App Not to Track", "Allow"}}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeSys{dialog: tc.dialog}
			dc := &DeviceContext{sysDriver: f}
			var err error
			out := captureStdout(t, func() {
				err = dc.answerNotificationAlertWithin(context.Background(), "Allow", 0)
			})
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Contains(out, trackingWarning); got != tc.warn {
				t.Errorf("warning printed = %v, want %v (output %q)", got, tc.warn, out)
			}
		})
	}
}
