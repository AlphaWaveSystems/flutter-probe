package cli

import (
	"strings"
	"testing"
)

// --cloud was registered as "upload test results to FlutterProbe Cloud after run", but nothing read it: the upload is
// decided by a configured cloud token. It must refuse before runTests reads a config, a file or a device.
func TestTestCloudFlagRefusesBeforeAnythingRuns(t *testing.T) {
	for _, arg := range []string{"--cloud", "--cloud=true"} {
		t.Run(arg, func(t *testing.T) {
			f := testCmd.Flags()
			t.Cleanup(func() { _ = f.Set("cloud", "false") })
			if err := f.Parse([]string{arg}); err != nil {
				t.Fatalf("parse %s: %v", arg, err)
			}
			// A path that cannot be read: had runTests started, this would fail on the file or the config, not on the flag.
			err := runTests(testCmd, []string{"/nonexistent/probe-cloud-test/x.probe"})
			if err != errCloudNotImplemented {
				t.Fatalf("err = %v: the refusal must be the flag's own, returned before runTests reads anything", err)
			}
			if !strings.Contains(err.Error(), "not implemented") || !strings.Contains(err.Error(), "--cloud-token") {
				t.Errorf("the message does not say what to do: %q", err)
			}
		})
	}
}

func TestTestCloudFlagOffDoesNotRefuse(t *testing.T) {
	if err := testCmd.Flags().Set("cloud", "false"); err != nil {
		t.Fatal(err)
	}
	if err := refuseCloudFlag(testCmd); err != nil {
		t.Fatalf("refuseCloudFlag with the flag off = %v", err)
	}
}

func TestTestCloudFlagIsHiddenAndNotDescribedAsWorking(t *testing.T) {
	fl := testCmd.Flags().Lookup("cloud")
	if fl == nil || !fl.Hidden {
		t.Fatal("--cloud must be hidden from help")
	}
	if strings.Contains(strings.ToLower(fl.Usage), "upload test results to flutterprobe cloud after run") {
		t.Errorf("usage still promises an upload: %q", fl.Usage)
	}
}
