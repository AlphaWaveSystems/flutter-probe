package sysdialog

import "testing"

// Each case notes one environment value and checks whether ScrubKnown then masks it in printed text. The values are
// distinct, so one case cannot change the result of another.
func TestNoteEnvValueMasksOnlyCredentialShapedValues(t *testing.T) {
	cases := []struct {
		name, value, printed string
		masked               bool
	}{
		{"COUNT", "1", "1 of 10", false},                     // one character from an ordinary name: digits in output stay readable
		{"HOST_PORT", "8080", "listening on 8080", false},    // four characters from an ordinary name
		{"GREETING", "hello there", "say hello there", true}, // eleven characters from an ordinary name
		{"FEATURE_FLAG", "false", "the false branch", false}, // a plain word from an ordinary name
		{"AUTH_PIN", "7731", "pin 7731", true},               // four characters from a secret-like name
		{"API_KEY", "rk9", "key rk9 accepted", true},         // three characters from a secret-like name
		{"SHORT_PIN", "42", "code 42", false},                // two characters: below the secret-like minimum
	}
	for _, tc := range cases {
		NoteEnvValue(tc.name, tc.value)
		got := ScrubKnown(tc.printed)
		if masked := got != tc.printed; masked != tc.masked {
			t.Errorf("NoteEnvValue(%q, %q): ScrubKnown(%q) = %q, masked = %v, want %v", tc.name, tc.value, tc.printed, got, masked, tc.masked)
		}
	}
}

// A secret word matches a name only as a whole token of it. A name that holds the word as a token is noted from three
// characters, and a name that holds it only inside a longer token is not.
func TestNoteEnvValueMatchesSecretWordsAsWholeTokens(t *testing.T) {
	cases := []struct {
		name, value, printed string
		masked               bool
	}{
		{"PIN", "pn42", "pin pn42", true},             // pin is a whole token
		{"API_KEY", "ak7q", "key ak7q", true},         // api and key: key is a whole token
		{"DB_PASSWORD", "pw7q", "login pw7q", true},   // password is a whole token
		{"OTP_CODE", "o7q", "otp o7q", true},          // otp is a whole token
		{"CRED_FILE", "cf9z", "file cf9z", true},      // cred is a whole token
		{"PASSPHRASE", "pp8x", "phrase pp8x", true},   // passphrase is in the list
		{"SID", "sd5q", "sid sd5q", true},             // sid is in the list
		{"ENCRYPTION_KEY", "en3q", "enc en3q", true},  // encryption and key are whole tokens
		{"SPINNER", "sp7q", "spinner sp7q", false},    // contains pin, but its only token is spinner
		{"MAPPING", "mp7q", "map mp7q", false},        // contains pin, but its only token is mapping
		{"BYPASS_MODE", "bm7q", "mode bm7q", false},   // contains pass, which is not a whole token
		{"MONKEY_COUNT", "mk7q", "count mk7q", false}, // contains key, which is not a whole token
		{"SIDEBAR", "sb7q", "bar sb7q", false},        // contains sid, which is not a whole token
		{"PASSWORDLESS", "pl7q", "less pl7q", false},  // contains password, which is not a whole token
		{"PIN_X", "true", "x is true", false},         // a plain word is never noted under eight characters
		{"PIN", "1", "pin 1", false},                  // one character is below the minimum, even for a secret word
	}
	for _, tc := range cases {
		NoteEnvValue(tc.name, tc.value)
		got := ScrubKnown(tc.printed)
		if masked := got != tc.printed; masked != tc.masked {
			t.Errorf("NoteEnvValue(%q, %q): ScrubKnown(%q) = %q, masked = %v, want %v", tc.name, tc.value, tc.printed, got, masked, tc.masked)
		}
	}
}

// A secret-like name marks its value as a credential, so the length rule for ordinary names does not apply to it.
func TestSecretLikeNameNotesValuesOfThreeCharacters(t *testing.T) {
	NoteEnvValue("DB_PASSWD", "p9x")
	if got, want := ScrubKnown("login p9x failed"), "login **** failed"; got != want {
		t.Fatalf("ScrubKnown = %q, want %q", got, want)
	}
}
