package runner

import (
	"context"
	"strings"
	"testing"
)

func TestTypeNativeRefusesUntypableTextBeforeTapping(t *testing.T) {
	// a DeviceContext with no Manager would panic if the tap were attempted, so reaching the text check first is what the test proves
	dc := &DeviceContext{}
	for text, want := range map[string]string{"a\nb": "control character", "wifí": "outside ASCII", "50%": "%", "-x": "starts with"} {
		err := dc.TypeNative(context.Background(), "Search", text)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: err = %v, want it to contain %q", text, err, want)
		}
	}
}
