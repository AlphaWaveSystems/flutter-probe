package device_test

import (
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/device"
)

func TestResolveAndroidPermissions_Valid(t *testing.T) {
	tests := []struct {
		name     string
		minPerms int
	}{
		{"camera", 1},
		{"location", 2},
		{"storage", 3},
		{"notifications", 1},
		{"microphone", 1},
		{"contacts", 2},
		{"phone", 2},
		{"calendar", 2},
		{"sms", 2},
		{"bluetooth", 2},
	}

	for _, tt := range tests {
		perms, err := device.ResolveAndroidPermissions(tt.name)
		if err != nil {
			t.Errorf("ResolveAndroidPermissions(%q): %v", tt.name, err)
			continue
		}
		if len(perms) < tt.minPerms {
			t.Errorf("ResolveAndroidPermissions(%q): got %d perms, want >= %d", tt.name, len(perms), tt.minPerms)
		}
		for _, p := range perms {
			if p == "" {
				t.Errorf("ResolveAndroidPermissions(%q): empty permission string", tt.name)
			}
		}
	}
}

func TestResolveAndroidPermissions_Invalid(t *testing.T) {
	_, err := device.ResolveAndroidPermissions("nonexistent")
	if err == nil {
		t.Error("expected error for unknown permission")
	}
}

func TestResolveIOSService_Valid(t *testing.T) {
	tests := []struct {
		name string
		svc  string
	}{
		{"camera", "camera"},
		{"location", "location"},
		{"microphone", "microphone"},
		{"photos", "photos"},
		{"contacts", "contacts-limited"},
		{"calendar", "calendar"},
		{"sms", "sms"},
	}

	for _, tt := range tests {
		svc, err := device.ResolveIOSService(tt.name)
		if err != nil {
			t.Errorf("ResolveIOSService(%q): %v", tt.name, err)
			continue
		}
		if svc != tt.svc {
			t.Errorf("ResolveIOSService(%q): got %q, want %q", tt.name, svc, tt.svc)
		}
	}
}

func TestResolveIOSService_Invalid(t *testing.T) {
	_, err := device.ResolveIOSService("nonexistent")
	if err == nil {
		t.Error("expected error for unknown permission")
	}
}

func TestResolveIOSService_NotificationsUnsupported(t *testing.T) {
	// Notifications are intentionally excluded from iOS privacy services
	_, err := device.ResolveIOSService("notifications")
	if err == nil {
		t.Error("expected error: notifications not supported via simctl")
	}
}
