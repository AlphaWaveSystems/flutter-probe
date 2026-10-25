package device

import "testing"

// `pm list packages` filters by substring, so only an exact name counts as installed.
func TestListsPackage(t *testing.T) {
	tests := []struct {
		name string
		out  string
		pkg  string
		want bool
	}{
		{"an exact match is listed", "package:com.example.my_app\n", "com.example.my_app", true},
		{"a longer package that starts with the name is not", "package:com.example.my_app.debug\n", "com.example.my_app", false},
		{"a shorter name that the listing contains is not", "package:com.example.my_app\n", "com.example.my", false},
		{"other lines and CRLF endings are handled", "WARNING: something\npackage:com.example.my_app\r\n", "com.example.my_app", true},
		{"no output lists nothing", "", "com.example.my_app", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := listsPackage(tt.out, tt.pkg); got != tt.want {
				t.Fatalf("listsPackage(%q, %q) = %v, want %v", tt.out, tt.pkg, got, tt.want)
			}
		})
	}
}
