package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// --watch was registered and documented as "re-run tests automatically on file changes", but nothing read it: a
// run with it was a plain single run. It must refuse before anything touches a device or a file.
func TestTestWatchFlagRefuses(t *testing.T) {
	for _, arg := range []string{"--watch", "-w", "--watch=true"} {
		t.Run(arg, func(t *testing.T) {
			f := testCmd.Flags()
			t.Cleanup(func() { _ = f.Set("watch", "false") })
			if err := f.Parse([]string{arg}); err != nil {
				t.Fatalf("parse %s: %v", arg, err)
			}
			var errOut bytes.Buffer
			testCmd.SetErr(&errOut)
			// A path that cannot be read: had runTests started, this would fail on the file (or the config), not on the flag.
			err := runTestsAndReportError(testCmd, []string{"/nonexistent/probe-watch-test/x.probe"})
			if err != errWatchNotImplemented {
				t.Fatalf("err = %v: the refusal must be the flag's own, returned before runTests reads anything", err)
			}
			if !errors.Is(err, errWatchNotImplemented) {
				t.Fatalf("err = %v, want errWatchNotImplemented", err)
			}
			if !strings.Contains(errOut.String(), "not implemented") {
				t.Errorf("the refusal was not printed: %q", errOut.String())
			}
		})
	}
}

func TestTestWatchFlagOffDoesNotRefuse(t *testing.T) {
	if err := testCmd.Flags().Set("watch", "false"); err != nil {
		t.Fatal(err)
	}
	if err := refuseWatch(testCmd); err != nil {
		t.Fatalf("refuseWatch with the flag off = %v", err)
	}
}

func TestTestWatchFlagIsHiddenAndNotDescribedAsWorking(t *testing.T) {
	fl := testCmd.Flags().Lookup("watch")
	if fl == nil || !fl.Hidden {
		t.Fatal("--watch must be hidden from help")
	}
	if strings.Contains(testCmd.Example, "--watch") {
		t.Error("the examples of probe test still show --watch")
	}
	if strings.Contains(strings.ToLower(fl.Usage), "re-run tests automatically") {
		t.Errorf("usage still promises a watch mode: %q", fl.Usage)
	}
}
