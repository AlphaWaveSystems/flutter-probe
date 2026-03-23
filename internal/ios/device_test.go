package ios

import "testing"

func TestIsSimulatorUDID(t *testing.T) {
	tests := []struct {
		udid string
		want bool
	}{
		// Simulator UDIDs (standard UUID format)
		{"909F49AD-EE6A-4263-AFED-BAC0FC5C8B40", true},
		{"F7038CCA-300E-4529-AB95-F691EB5E6052", true},
		{"4CC9ECA0-8C9D-4D48-BECB-F9C40151703D", true},
		{"ED4DDB5D-A947-4842-8CDB-BCB0D474E70B", true},

		// Physical device UDIDs (hex serial format)
		{"00008120-0011790211A2201E", false},
		{"00008030-001A39E83CC2802E", false},
		{"00008101-000E25391A20001E", false},

		// Edge cases
		{"", false},
		{"emulator-5554", false},
		{"localhost:5555", false},
	}

	for _, tt := range tests {
		t.Run(tt.udid, func(t *testing.T) {
			got := IsSimulatorUDID(tt.udid)
			if got != tt.want {
				t.Errorf("IsSimulatorUDID(%q) = %v, want %v", tt.udid, got, tt.want)
			}
		})
	}
}
