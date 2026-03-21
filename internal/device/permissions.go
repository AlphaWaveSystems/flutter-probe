package device

import "fmt"

// AndroidPermissions maps human-readable names to Android manifest permission strings.
var AndroidPermissions = map[string][]string{
	"notifications": {"android.permission.POST_NOTIFICATIONS"},
	"camera":        {"android.permission.CAMERA"},
	"location":      {"android.permission.ACCESS_FINE_LOCATION", "android.permission.ACCESS_COARSE_LOCATION"},
	"microphone":    {"android.permission.RECORD_AUDIO"},
	"storage":       {"android.permission.READ_EXTERNAL_STORAGE", "android.permission.WRITE_EXTERNAL_STORAGE", "android.permission.READ_MEDIA_IMAGES", "android.permission.READ_MEDIA_VIDEO"},
	"contacts":      {"android.permission.READ_CONTACTS", "android.permission.WRITE_CONTACTS"},
	"phone":         {"android.permission.READ_PHONE_STATE", "android.permission.CALL_PHONE"},
	"calendar":      {"android.permission.READ_CALENDAR", "android.permission.WRITE_CALENDAR"},
	"sms":           {"android.permission.READ_SMS", "android.permission.SEND_SMS"},
	"bluetooth":     {"android.permission.BLUETOOTH_CONNECT", "android.permission.BLUETOOTH_SCAN"},
}

// IOSPrivacyServices maps human-readable names to simctl privacy service names.
// NOTE: "notifications" is intentionally excluded — xcrun simctl privacy does
// not support granting notification permission (it requires the native
// UNUserNotificationCenter prompt). Apps should check
// bool.fromEnvironment('PROBE_AGENT') and skip the notification request when
// running under FlutterProbe.
var IOSPrivacyServices = map[string]string{
	"camera":    "camera",
	"location":  "location",
	"microphone": "microphone",
	"photos":    "photos",
	"contacts":  "contacts-limited",
	"calendar":  "calendar",
	"sms":       "sms",
}

// ResolveAndroidPermissions returns the Android permission strings for a human-readable name.
func ResolveAndroidPermissions(name string) ([]string, error) {
	perms, ok := AndroidPermissions[name]
	if !ok {
		return nil, fmt.Errorf("unknown permission %q — available: notifications, camera, location, microphone, storage, contacts, phone, calendar, sms, bluetooth", name)
	}
	return perms, nil
}

// ResolveIOSService returns the simctl privacy service for a human-readable name.
func ResolveIOSService(name string) (string, error) {
	svc, ok := IOSPrivacyServices[name]
	if !ok {
		return "", fmt.Errorf("unknown permission %q — available: notifications, camera, location, microphone, photos, contacts, calendar", name)
	}
	return svc, nil
}
