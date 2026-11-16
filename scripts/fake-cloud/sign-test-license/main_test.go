package main

import (
	"bytes"
	"encoding/base64"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/license"
)

// The license the tool prints verifies with the public key it prints, carries the plan an upload needs, and expires after a day.
func TestTheTestLicenseVerifiesWithItsOwnKey(t *testing.T) {
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	pubEnv, key := sign(now)
	kid, pub, ok := strings.Cut(pubEnv, "=")
	if !ok || kid != "localtest" {
		t.Fatalf("pubEnv = %q", pubEnv)
	}
	ks := license.KeySet{Keys: []license.TrustedKey{{KID: kid, PublicKey: pub}}}
	c, err := license.VerifyWith(key, ks, now.Add(time.Hour))
	if err != nil || !c.Has(license.Plan("probe_cloud")) || c.Seats != 1 {
		t.Fatalf("claims %+v, err %v", c, err)
	}
	if _, err := license.VerifyWith(key, ks, now.Add(25*time.Hour)); err != license.ErrExpired {
		t.Errorf("after a day: %v, want expired", err)
	}
	// a key the tool signed is not trusted by anything else: another run's public key does not verify it
	otherPub, _ := sign(now)
	_, other, _ := strings.Cut(otherPub, "=")
	if _, err := license.VerifyWith(key, license.KeySet{Keys: []license.TrustedKey{{KID: "localtest", PublicKey: other}}}, now); err == nil {
		t.Error("a different key pair verified the license")
	}
	// a released CLI trusts no such key: with no key set at all, nothing verifies
	if _, err := license.VerifyWith(key, license.KeySet{}, now); err != license.ErrNoPublicKey {
		t.Errorf("with no trusted key: %v", err)
	}
}

// The tool makes nothing when the service address is not this machine, and says why without printing a key.
func TestItRefusesAServiceThatIsNotThisMachine(t *testing.T) {
	for _, ok := range []string{"", "http://127.0.0.1:48899", "http://localhost:1", "https://127.0.0.1", "http://[::1]:8080"} {
		if err := checkLoopback(ok); err != nil {
			t.Errorf("%q refused: %v", ok, err)
		}
	}
	for _, bad := range []string{"https://api.flutterprobe.dev", "http://example.com", "http://10.0.0.5:48899", "http://127.0.0.1.example.com", "not a url", "http://",
		"http://localhost.evil.com", "http://localhost.", "http://LOCALHOST:1", "http://Localhost", "http://127.0.0.1@evil.com", "http://localhost@evil.com:1", "http://evil.com#@127.0.0.1", "http://127.0.0.1.nip.io", "http://0.0.0.0:1", "http://[::ffff:10.0.0.1]:1"} {
		if err := checkLoopback(bad); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}

func TestTheLicenseIsMarkedAsATestOne(t *testing.T) {
	_, key := sign(time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC))
	payload, _ := base64.RawURLEncoding.DecodeString(strings.Split(key, ".")[2])
	if !strings.Contains(string(payload), "LOCAL-TEST-ONLY-NOT-A-REAL-LICENSE") || !strings.Contains(string(payload), `"kid":"localtest"`) {
		t.Errorf("payload = %s", payload)
	}
}

// The built tool, run as a program: a remote service makes it print nothing on stdout and exit 1; a local one gets a license, with the TEST ONLY line on stderr.
func TestTheBuiltToolRefusesARemoteServiceAndMarksItsOutput(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go is not on the path")
	}
	bin := filepath.Join(t.TempDir(), "sign-test-license")
	if out, err := exec.Command(goBin, "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	run := func(url string) (stdout, stderr string, code int) {
		cmd := exec.Command(bin)
		cmd.Env = append(os.Environ(), "PROBE_CLOUD_URL="+url)
		var o, e bytes.Buffer
		cmd.Stdout, cmd.Stderr = &o, &e
		err := cmd.Run()
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		} else if err != nil {
			t.Fatal(err)
		}
		return o.String(), e.String(), code
	}
	out, errOut, code := run("https://api.flutterprobe.dev")
	if out != "" || code != 1 || !strings.Contains(errOut, "refusing to make a test license") || strings.Contains(errOut, "TEST ONLY") {
		t.Errorf("remote: stdout %q, stderr %q, exit %d", out, errOut, code)
	}
	out, errOut, code = run("http://127.0.0.1:48899")
	if code != 0 || !strings.Contains(errOut, "TEST ONLY") || !strings.Contains(out, "export PROBE_LICENSE_PUBKEY=localtest=") || !strings.Contains(out, "export PROBE_LICENSE_KEY=FP2.localtest.") {
		t.Errorf("local: stdout %q, stderr %q, exit %d", out, errOut, code)
	}
	if strings.Contains(errOut, "FP2.") {
		t.Errorf("the license was printed to stderr: %q", errOut)
	}
	// no service named at all is fine too: the fake server's default is used
	if out, _, code := run(""); code != 0 || !strings.Contains(out, "PROBE_LICENSE_KEY") {
		t.Errorf("no URL: %q %d", out, code)
	}
}
