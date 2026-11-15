package devsettings

import (
	"fmt"
	"strings"
)

// PlanAirplaneMode returns the commands that turn the airplane mode of one named Android emulator on or off. It is not a
// Setting: the panel, the CLI and the device_set MCP tool do not offer it, only the `set airplane mode` step does.
//
// The state stays until a later command changes it. A device reached over the network (a host:port serial) is refused:
// turning the radio off would cut the connection adb uses, and the run could not reach the device again.
func PlanAirplaneMode(p Platform, device string, on bool) ([]Command, error) {
	if device == "" || strings.EqualFold(device, "booted") || !deviceRe.MatchString(device) {
		return nil, fmt.Errorf("name one specific device (a serial); %q is not accepted", device)
	}
	switch p {
	case Android:
		if strings.Contains(device, ":") {
			return nil, fmt.Errorf("%q is a network adb address (host:port): airplane mode would cut the adb connection to it, so the step refuses it; use an emulator serial such as emulator-5554", device)
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
