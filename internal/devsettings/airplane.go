package devsettings

import (
	"fmt"
	"regexp"
	"strings"
)

// emulatorSerial is the serial an Android emulator has under adb: emulator- and its console port. It is the only serial
// the airplane step accepts. A physical device, a wireless-debugging name and a network address all have other serials,
// and airplane mode can cut the adb connection to any of them.
var emulatorSerial = regexp.MustCompile(`^emulator-[0-9]{1,5}$`)

// PlanAirplaneMode returns the commands that turn the airplane mode of one named Android emulator on or off. It is not a
// Setting: the panel, the CLI and the device_set MCP tool do not offer it, only the `set airplane mode` step does.
//
// The decision is made from the serial alone, so a caller that forgets to say the device is physical cannot change the
// outcome. The state stays until a later command changes it.
func PlanAirplaneMode(p Platform, device string, on bool) ([]Command, error) {
	// A wireless-debugging name has underscores, which deviceRe does not allow, so it is named before that check.
	if p == Android && (strings.Contains(device, "_adb-tls-connect") || strings.HasSuffix(device, "._tcp")) {
		return nil, fmt.Errorf("%q is a wireless-debugging device: airplane mode would cut the adb connection to it, so the step refuses it; only an emulator takes it (serial emulator-NNNN)", device)
	}
	if device == "" || strings.EqualFold(device, "booted") || !deviceRe.MatchString(device) {
		return nil, fmt.Errorf("name one specific device (a serial); %q is not accepted", device)
	}
	switch p {
	case Android:
		if strings.Contains(device, ":") {
			return nil, fmt.Errorf("%q is a network adb address (host:port): airplane mode would cut the adb connection to it, so the step refuses it; only an emulator takes it (serial emulator-NNNN)", device)
		}
		if !emulatorSerial.MatchString(device) {
			return nil, fmt.Errorf("%q is not an emulator serial: airplane mode runs only on an Android emulator (serial emulator-NNNN); a physical device is refused, because turning airplane mode on can cut the adb connection to it", device)
		}
		state := "disable"
		if on {
			state = "enable"
		}
		return []Command{adb(device, "cmd", "connectivity", "airplane-mode", state)}, nil
	case IOS:
		return nil, fmt.Errorf("airplane mode cannot be changed on an iOS simulator: simctl has no command for it, so the step does not fake it")
	}
	return nil, fmt.Errorf("unknown platform %q (use android or ios)", p)
}
