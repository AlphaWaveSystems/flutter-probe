package cli

import (
	"context"
	"encoding/xml"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/probelink"
	"github.com/spf13/cobra"
)

// A JUnit run whose agent refuses the connection at startup writes its file: a closed port makes the real
// dial error, and the file parses with one <error> testcase per planned file.
func TestConnectFailureOnClosedPortWritesJUnitFile(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close() // nothing listens on this port now

	_, dialErr := probelink.DialWithOptions(context.Background(), probelink.DialOptions{
		Host: "127.0.0.1", Port: port, DialTimeout: 2 * time.Second,
	})
	if dialErr == nil {
		t.Fatal("expected the closed port to refuse the connection")
	}
	cause := fmt.Errorf("connecting to ProbeAgent: %w", dialErr)

	out := filepath.Join(t.TempDir(), "reports", "results.xml")
	files := []string{"tests/a.probe", "tests/b.probe"}
	if err := writeConnectFailureReport(&cobra.Command{}, files, out, false, cause); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("junit file was not written: %v", err)
	}
	var doc struct {
		Tests  int `xml:"tests,attr"`
		Errors int `xml:"errors,attr"`
		Suites []struct {
			Cases []struct {
				Error *struct {
					Message string `xml:"message,attr"`
				} `xml:"error"`
			} `xml:"testcase"`
		} `xml:"testsuite"`
	}
	if err := xml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("junit file does not parse: %v\n%s", err, raw)
	}
	if doc.Tests != 2 || doc.Errors != 2 || len(doc.Suites) != 2 {
		t.Fatalf("want 2 files with 2 errors, got tests=%d errors=%d suites=%d\n%s", doc.Tests, doc.Errors, len(doc.Suites), raw)
	}
	for i, s := range doc.Suites {
		if len(s.Cases) != 1 || s.Cases[0].Error == nil || !strings.Contains(s.Cases[0].Error.Message, "connecting to ProbeAgent") {
			t.Errorf("suite %d: want one <error> carrying the connect error\n%s", i, raw)
		}
	}
}
