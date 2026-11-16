//go:build !darwin && !linux

package cli

// fdIsTerminal on other systems trusts the character-device check in stdinIsTerminal, which already excludes the null device.
func fdIsTerminal(uintptr) bool {
	return true
}
