package record

import "testing"

// A card number or CVV sent as a bare JSON number is a credential too; other numbers stay.
func TestRedactBodyRedactsNumericCredentialFields(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"card number sent as a number", `{"card_number": 4111111111111111, "amount": 20}`, `{"card_number": "<redacted>", "amount": 20}`},
		{"cvv sent as a string", `{"cvv": "123"}`, `{"cvv": "<redacted>"}`},
		{"cvv sent as a number", `{"cvv":123}`, `{"cvv":"<redacted>"}`},
		{"non credential number kept", `{"count": 4111111111111111}`, `{"count": 4111111111111111}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RedactBody(tt.in); got != tt.want {
				t.Fatalf("RedactBody(%s) = %s, want %s", tt.in, got, tt.want)
			}
		})
	}
}
