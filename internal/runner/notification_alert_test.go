package runner

import (
	"context"
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
	{"ru", "Разрешить", "Попросить не отслеживать"},
}

var notificationAlerts = []struct{ locale, allow, deny string }{
	{"en", "Allow", "Don't Allow"},
	{"de", "Erlauben", "Nicht erlauben"},
	{"fr", "Autoriser", "Refuser"},
	{"ja", "許可", "許可しない"},
	{"ar", "السماح", "عدم السماح"},
	{"ru", "Разрешить", "Запретить"},
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

// TestNotificationAlertIsNotTheLocationAlert: the location alert has an allow-once button, so it is not answered
// as the notification alert.
func TestNotificationAlertIsNotTheLocationAlert(t *testing.T) {
	f := &fakeSys{dialog: &sysdialog.Dialog{Buttons: []string{"Allow Once", "Allow While Using App", "Don't Allow"}}}
	dc := &DeviceContext{sysDriver: f}
	if err := dc.answerNotificationAlertWithin(context.Background(), "Don't Allow", 0); err != nil {
		t.Fatal(err)
	}
	if len(f.tapped) != 0 {
		t.Errorf("tapped %q on the location alert", f.tapped)
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
