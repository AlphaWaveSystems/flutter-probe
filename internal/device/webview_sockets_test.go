package device

import (
	"strconv"
	"strings"
	"testing"
)

const unixHeader = "Num       RefCount Protocol Flags    Type St Inode Path\n"

func unixLine(path string) string {
	return "0000000000000000: 00000002 00000000 00010000 0001 01 12345 " + path + "\n"
}

func TestParseWebViewSockets(t *testing.T) {
	cases := []struct {
		name  string
		table string
		want  []int
	}{
		{"empty", "", nil},
		{"header only", unixHeader, nil},
		{"one socket", unixHeader + unixLine("@webview_devtools_remote_2783"), []int{2783}},
		{"two sockets sorted by pid", unixLine("@webview_devtools_remote_2783") + unixLine("@webview_devtools_remote_1707"), []int{1707, 2783}},
		{"duplicate name collapses", unixLine("@webview_devtools_remote_55") + unixLine("@webview_devtools_remote_55"), []int{55}},
		{"unrelated sockets ignored", unixLine("@chrome_devtools_remote") + unixLine("/dev/socket/zygote") + unixLine("@webview_devtools_remote_9"), []int{9}},
		{"file system path with that name", unixLine("/data/local/tmp/webview_devtools_remote_12"), nil},
		{"no @ prefix", unixLine("webview_devtools_remote_12"), nil},
		{"letters after the digits", unixLine("@webview_devtools_remote_12a"), nil},
		{"letters instead of digits", unixLine("@webview_devtools_remote_abc"), nil},
		{"no digits", unixLine("@webview_devtools_remote_"), nil},
		{"zero pid", unixLine("@webview_devtools_remote_0"), nil},
		{"leading zero", unixLine("@webview_devtools_remote_012"), nil},
		{"pid above the limit", unixLine("@webview_devtools_remote_99999999"), nil},
		{"huge digit string", unixLine("@webview_devtools_remote_" + strings.Repeat("9", 40)), nil},
		{"negative sign", unixLine("@webview_devtools_remote_-5"), nil},
		{"plus sign", unixLine("@webview_devtools_remote_+5"), nil},
		{"unicode digits", unixLine("@webview_devtools_remote_١٢٣"), nil},
		{"extra text after the path", "0000000000000000: 00000002 00000000 00010000 0001 01 12345 @webview_devtools_remote_5 extra\n", nil},
		{"too few fields", "@webview_devtools_remote_5\n", nil},
		{"socket without a path", "0000000000000000: 00000002 00000000 00010000 0001 01 12345\n", nil},
		{"CRLF line ends", strings.ReplaceAll(unixLine("@webview_devtools_remote_77"), "\n", "\r\n"), []int{77}},
		{"path inside another field", "@webview_devtools_remote_5 00000002 00000000 00010000 0001 01 12345 @x\n", nil},
	}
	for _, c := range cases {
		got := ParseWebViewSockets(c.table)
		var pids []int
		for _, s := range got {
			pids = append(pids, s.PID)
			if s.Name != webViewSocketPrefix+itoa(s.PID) {
				t.Errorf("%s: name %q does not match pid %d", c.name, s.Name, s.PID)
			}
		}
		if len(pids) != len(c.want) {
			t.Errorf("%s: got pids %v, want %v", c.name, pids, c.want)
			continue
		}
		for i := range pids {
			if pids[i] != c.want[i] {
				t.Errorf("%s: got pids %v, want %v", c.name, pids, c.want)
				break
			}
		}
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

func TestOwnWebViewSocketNeverReturnsAnotherProcess(t *testing.T) {
	table := unixHeader + unixLine("@webview_devtools_remote_1707") + unixLine("@webview_devtools_remote_2783")
	s, err := OwnWebViewSocket(table, 2783)
	if err != nil || s.PID != 2783 || s.Name != "webview_devtools_remote_2783" {
		t.Fatalf("own socket = %+v, %v", s, err)
	}
	if _, err := OwnWebViewSocket(table, 2784); err == nil || !strings.Contains(err.Error(), "no WebView debugging socket") {
		t.Fatalf("pid without a socket: err = %v", err)
	}
	for _, pid := range []int{0, -1, maxPID + 1} {
		if _, err := OwnWebViewSocket(table, pid); err == nil || !strings.Contains(err.Error(), "not a process id") {
			t.Errorf("pid %d: err = %v, want \"not a process id\"", pid, err)
		}
	}
	// a table with only another process's socket yields none for ours
	if _, err := OwnWebViewSocket(unixLine("@webview_devtools_remote_1707"), 2783); err == nil {
		t.Fatal("another process's socket was returned")
	}
}
