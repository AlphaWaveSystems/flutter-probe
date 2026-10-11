package device_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/device"
)

func TestResolveIOSService_NotificationsIsExplicitlyUnsupported(t *testing.T) {
	_, err := device.ResolveIOSService("notifications")
	if !errors.Is(err, device.ErrIOSNotificationsUnsupported) {
		t.Fatalf("want ErrIOSNotificationsUnsupported, got %v", err)
	}
}

func TestResolveIOSService_UnknownDoesNotAdvertiseNotifications(t *testing.T) {
	_, err := device.ResolveIOSService("telepathy")
	if err == nil {
		t.Fatal("want an error for an unknown permission")
	}
	if errors.Is(err, device.ErrIOSNotificationsUnsupported) {
		t.Error("an unknown permission must not be reported as the notifications limitation")
	}
	if strings.Contains(err.Error(), "notifications") {
		t.Errorf("the iOS list must not advertise notifications (unsupported): %q", err.Error())
	}
}

func TestValidPermissionName(t *testing.T) {
	for _, ok := range []string{"notifications", "camera", "location", "photos", "bluetooth", "storage"} {
		if !device.ValidPermissionName(ok) {
			t.Errorf("%q should be a valid permission name", ok)
		}
	}
	for _, bad := range []string{"", "Notifications", "telepathy"} {
		if device.ValidPermissionName(bad) {
			t.Errorf("%q should not be valid", bad)
		}
	}
}
