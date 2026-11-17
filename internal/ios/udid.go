package ios

import "regexp"

// The ids a simulator or a device can have. A UDID ends up in file paths (a simulator's data folder, a temporary file for devicectl's
// output), so only these exact forms are used there: hex digits and dashes, never a path separator, "..", a glob character or a NUL.
var (
	// simulatorUDIDRE is a simulator's UDID: a UUID, 8-4-4-4-12 hex digits.
	simulatorUDIDRE = regexp.MustCompile(`^[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}$`)
	// deviceUDIDRE is a physical device's UDID: 8 hex digits, a dash and 16 hex digits (iPhone XS and later), or 40 hex digits (earlier
	// devices). devicectl also takes a CoreDevice identifier, which is a UUID (simulatorUDIDRE).
	deviceUDIDRE = regexp.MustCompile(`^([0-9A-Fa-f]{8}-[0-9A-Fa-f]{16}|[0-9A-Fa-f]{40})$`)
)

// ValidSimulatorUDID reports whether s is a simulator UDID.
func ValidSimulatorUDID(s string) bool { return simulatorUDIDRE.MatchString(s) }

// ValidDeviceUDID reports whether s is a physical device's UDID or a CoreDevice identifier.
func ValidDeviceUDID(s string) bool { return deviceUDIDRE.MatchString(s) || simulatorUDIDRE.MatchString(s) }
