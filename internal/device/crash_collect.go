package device

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/alphawavesystems/flutter-probe/internal/shellquote"
)

// Crash evidence for Android (docs/proposals/crash-anr-capture.md, slice S1). Two parsers for the text the device prints, and one collector that runs two
// read-only adb commands and returns bounded, cleaned and masked evidence. Nothing calls it yet.
//
// Everything the device prints is untrusted: an app can put any text in its exception message, its log, and its exit description. So the text is only ever
// shown as text; it is cleaned (control, bidi and other invisible characters removed), masked by the caller's function, and cut to fixed sizes, in that
// order (a cut before the masking could leave the readable start of a secret). No error returned here contains device text.
//
// Limits of the masking: it works on one line at a time, as the log prints them. A secret split over two lines is not seen by a function that matches one
// value, and a block of several lines is not recognised by one that matches a line. The one multi-line form that is handled here is a PEM block (-----BEGIN
// ... -----END): ParseCrashBuffer replaces it by one marker line. Anything else that spans lines depends on what the caller's Redact can match.

// Limits of one collection. Zero fields of CrashOptions mean these.
const (
	defaultCrashTimeout   = 10 * time.Second
	maxCrashInputBytes    = 512 << 10      // read from either command; the rest is dropped before parsing
	defaultCrashTextBytes = 32 << 10       // kept in CrashText
	defaultCrashLines     = 200            // kept in CrashText
	maxCrashLineRunes     = 400            // one kept line is cut here
	maxExitRecords        = 8              // exit-info entries kept
	maxExitDescription    = 120            // runes of the description of one exit
	maxCrashSummary       = 160            // runes of the summary
	maxProcessRunes       = 80             // runes of a process name
	maxPEMLines           = 200            // lines a PEM block hides at most, so an unterminated BEGIN cannot hide the rest of the buffer
	futureSlack           = 48 * time.Hour // an exit stamped later than this after now is not believed: the device prints no zone, which can be a day away
)

// ExitKind says what an exit-info entry's reason number means, for the report's wording. The number is the authority; the words the device prints next to it are ignored.
type ExitKind string

const (
	ExitUnknown       ExitKind = "unknown"
	ExitSelf          ExitKind = "exit"           // the app called exit
	ExitSignaled      ExitKind = "signaled"       // killed by a signal (not a crash handler)
	ExitLowMemory     ExitKind = "low_memory"     // the system killed it for memory
	ExitCrash         ExitKind = "crash"          // an uncaught Java or Kotlin exception
	ExitNativeCrash   ExitKind = "native_crash"   // a native crash with a signal
	ExitANR           ExitKind = "anr"            // the app did not respond
	ExitUserRequested ExitKind = "user_requested" // force-stop or swipe away
	ExitOther         ExitKind = "other"
)

// exitKinds maps android.app.ApplicationExitInfo REASON_* numbers.
var exitKinds = map[int]ExitKind{
	0: ExitUnknown, 1: ExitSelf, 2: ExitSignaled, 3: ExitLowMemory, 4: ExitCrash, 5: ExitNativeCrash, 6: ExitANR,
	7: ExitOther, 8: ExitOther, 9: ExitOther, 10: ExitUserRequested, 11: ExitUserRequested, 12: ExitOther, 13: ExitOther, 14: ExitOther, 15: ExitOther, 16: ExitOther,
}

// ExitRecord is one entry of `dumpsys activity exit-info <package>` that belongs to the app's own processes.
type ExitRecord struct {
	// WallClock is the device's wall-clock time of the exit as printed. The device prints no zone, so it carries the UTC location only as a label: compare it with other
	// device wall-clock times, never with this machine's clock.
	WallClock   time.Time
	PID         int
	Reason      int      // the ApplicationExitInfo reason number
	Kind        ExitKind // what the number means
	Status      int      // the exit status or the signal number, as printed
	Process     string   // the process name: the package, or the package, a colon and a service name of 1 to 64 letters, digits, _ . -
	User        int      // the Android user the process ran as; -1 when the dump does not print it
	Description string   // what the system printed, cleaned; may be empty. ParseExitInfo does not cut it: CollectCrashEvidence masks it, then cuts it
}

var (
	exitHeaderRE = regexp.MustCompile(`^\s*ApplicationExitInfo #\d+:\s*$`)
	exitTimeRE   = regexp.MustCompile(`timestamp=(\d{4}-\d\d-\d\d \d\d:\d\d:\d\d\.\d{3}) pid=(\d{1,10})\b`)
	exitProcRE   = regexp.MustCompile(`\bprocess=(\S+) reason=(\d{1,3})\b.*?\bstatus=(-?\d{1,10})`)
	exitDescRE   = regexp.MustCompile(`\bdescription=(.*?) state=`)
	exitUserRE   = regexp.MustCompile(`\buser=(\d{1,9})\b`)
	packageRE    = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*(\.[A-Za-z][A-Za-z0-9_]*)+$`)
	processTail  = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)
)

// crashNow is the clock ParseExitInfo checks entries against; tests replace it.
var crashNow = time.Now

// validPackage reports whether s can be an Android package name. It is checked before the name reaches a command line, in addition to quoting.
func validPackage(s string) bool { return len(s) <= 255 && packageRE.MatchString(s) }

// ValidPackageName is validPackage for a caller that must refuse a bad name before any adb call of its own.
func ValidPackageName(s string) bool { return validPackage(s) }

// ExitFilter says which entries ParseExitInfoFiltered keeps, before the cap on their number is applied. The zero value keeps every user's entries and judges
// future stamps by this machine's clock with a slack of futureSlack.
type ExitFilter struct {
	// OnlyUser keeps only the entries of User; an entry whose dump does not print a user is kept.
	OnlyUser bool
	User     int
	// Now is the device's own wall clock (from `date` on the device), in the same zone-less form as the entries; zero means this machine's clock is used instead.
	Now time.Time
	// Slack is how far after the clock an entry may be stamped and still be believed. Zero means futureSlack when Now is zero, and two minutes when it is given.
	Slack time.Duration
}

// maxExitCandidates bounds how many entries are looked at before the newest are chosen.
const maxExitCandidates = 64

// ParseExitInfo is ParseExitInfoFiltered with the zero filter.
func ParseExitInfo(text, pkg string) []ExitRecord {
	return ParseExitInfoFiltered(text, pkg, ExitFilter{})
}

// ParseExitInfoFiltered reads the text of `dumpsys activity exit-info <package>` and returns the entries of the package's own processes (the package name, or the package
// name, a colon and a service name) that pass the filter, newest first by their time (ties keep the order the device printed them), at most maxExitRecords. The filter
// and the sort come before the cap, so entries of other users or stamped in the future cannot push real ones out. Entries of other processes under the same user id, such
// as a WebView sandbox, are left out. A line it cannot read is skipped; it never fails. The text is cut at maxCrashInputBytes first, and at most maxExitCandidates
// entries are looked at.
func ParseExitInfoFiltered(text, pkg string, f ExitFilter) []ExitRecord {
	if !validPackage(pkg) {
		return nil
	}
	text = capBytes(text, maxCrashInputBytes)
	var out []ExitRecord
	var block []string
	// the device prints no zone, so without its own clock "now" is this machine's wall clock read as if it were the device's, with a day or two of slack for the difference
	now, slack := f.Now, f.Slack
	if now.IsZero() {
		n := crashNow()
		now = time.Date(n.Year(), n.Month(), n.Day(), n.Hour(), n.Minute(), n.Second(), 0, time.UTC)
		if slack == 0 {
			slack = futureSlack
		}
	} else if slack == 0 {
		slack = 2 * time.Minute
	}
	flush := func() {
		if rec, ok := exitRecordOf(block, pkg); ok && len(out) < maxExitCandidates &&
			!rec.WallClock.After(now.Add(slack)) && (!f.OnlyUser || rec.User == -1 || rec.User == f.User) {
			out = append(out, rec)
		}
		block = block[:0]
	}
	for _, line := range strings.Split(text, "\n") {
		if exitHeaderRE.MatchString(strings.TrimRight(line, "\r")) {
			flush()
			block = append(block, "")
			continue
		}
		if len(block) > 0 {
			if strings.Contains(line, "Historical Process Exit for") || strings.Contains(line, "package:") && !strings.Contains(line, "=") {
				flush()
				continue
			}
			block = append(block, line)
		}
	}
	flush()
	// newest first, but an entry stamped after the clock (inside the slack) ranks behind every entry that is not: it is the doubtful one, so the cap drops it first
	sort.SliceStable(out, func(i, j int) bool {
		fi, fj := out[i].WallClock.After(now), out[j].WallClock.After(now)
		if fi != fj {
			return !fi
		}
		return out[i].WallClock.After(out[j].WallClock)
	})
	if len(out) > maxExitRecords {
		out = out[:maxExitRecords]
	}
	return out
}

// ownProcess reports whether name is the app's process: the package name, or the package name, a colon and a short service name of plain characters.
func ownProcess(name, pkg string) bool {
	if name == pkg {
		return true
	}
	tail, ok := strings.CutPrefix(name, pkg+":")
	return ok && processTail.MatchString(tail)
}

func exitRecordOf(block []string, pkg string) (ExitRecord, bool) {
	if len(block) == 0 {
		return ExitRecord{}, false
	}
	var rec ExitRecord
	rec.User = -1
	var haveTime, haveProc bool
	for _, line := range block {
		if m := exitTimeRE.FindStringSubmatch(line); m != nil && !haveTime {
			t, err := time.ParseInLocation("2006-01-02 15:04:05.000", m[1], time.UTC)
			pid, perr := strconv.Atoi(m[2])
			if err == nil && perr == nil {
				rec.WallClock, rec.PID, haveTime = t, pid, true
				if u := exitUserRE.FindStringSubmatch(line); u != nil {
					rec.User, _ = strconv.Atoi(u[1])
				}
			}
		}
		if m := exitProcRE.FindStringSubmatch(line); m != nil && !haveProc {
			reason, _ := strconv.Atoi(m[2])
			status, _ := strconv.Atoi(m[3])
			rec.Process, rec.Reason, rec.Status, haveProc = m[1], reason, status, true
		}
		if m := exitDescRE.FindStringSubmatch(line); m != nil && rec.Description == "" {
			rec.Description = cleanDeviceText(m[1])
		}
	}
	if !haveTime || !haveProc || !ownProcess(rec.Process, pkg) {
		return ExitRecord{}, false
	}
	kind, known := exitKinds[rec.Reason]
	if !known {
		kind = ExitOther
	}
	rec.Kind = kind
	return rec, true
}

// CrashText is what ParseCrashBuffer keeps of the crash buffer for one process.
type CrashText struct {
	// Lines are the kept lines, in order, cleaned but neither masked nor cut: the collector masks each one and then cuts it at maxCrashLineRunes, so a cut can never
	// leave the readable start of a secret. The byte limit counts a line as its cut length.
	Lines []string
	// Summary is one line that names the failure: the first exception line of a Java crash, or the "Fatal signal" line of a native one. Empty when there is none.
	Summary string
	// Truncated is true when lines or bytes were dropped to stay inside the limits.
	Truncated bool
}

var logLineRE = regexp.MustCompile(`^\d\d-\d\d \d\d:\d\d:\d\d\.\d{3}\s+(\d+)\s+\d+\s+[VDIWEFA]\s+(.*?)\s*: ?(.*)$`)

// ParseCrashBuffer reads the text of `logcat -b crash -d` (the threadtime format) and keeps the lines about one process: the lines the process itself wrote (the
// logger's process id is pid), and the system's tombstone lines (tag DEBUG), when the tombstone names that process id or the package. Lines of the crashpad handler
// (minidump text, kilobytes of noise) and of every other process are dropped. At most maxLines lines and maxBytes bytes are kept.
func ParseCrashBuffer(text string, pid int, pkg string, maxLines, maxBytes int) CrashText {
	if maxLines <= 0 {
		maxLines = defaultCrashLines
	}
	if maxBytes <= 0 {
		maxBytes = defaultCrashTextBytes
	}
	type entry struct {
		pid  int
		tag  string
		line string
	}
	var groups [][]entry
	var lastPID int
	var lastTag string
	for _, raw := range strings.Split(capBytes(text, maxCrashInputBytes), "\n") {
		m := logLineRE.FindStringSubmatch(strings.TrimRight(raw, "\r"))
		if m == nil {
			continue // a header such as "--------- beginning of crash", or text that is not a log line
		}
		lp, _ := strconv.Atoi(m[1])
		tag := m[2]
		if tag == "crashpad" {
			continue
		}
		if len(groups) == 0 || lp != lastPID || tag != lastTag {
			groups = append(groups, nil)
		}
		lastPID, lastTag = lp, tag
		groups[len(groups)-1] = append(groups[len(groups)-1], entry{lp, tag, m[3]})
	}
	var out CrashText
	bytes := 0
	pemLeft := 0
	pemOpen := false // a block began and its end has not been seen
	for _, g := range groups {
		keep := g[0].pid == pid
		if !keep && g[0].tag == "DEBUG" {
			for _, e := range g {
				if strings.Contains(e.line, "pid: "+strconv.Itoa(pid)+",") || (pkg != "" && strings.Contains(e.line, ">>> "+pkg+" <<<")) {
					keep = true
					break
				}
			}
		}
		if !keep {
			continue
		}
		for _, e := range g {
			line := cleanDeviceText(e.line)
			if pemLeft > 0 {
				pemLeft--
				if strings.Contains(line, "-----END ") {
					pemLeft, pemOpen = 0, false
				} else if pemLeft == 0 {
					out.Truncated, pemOpen = true, false // the block never ended within the limit: what follows may be its body
				}
				continue
			}
			if strings.Contains(line, "-----BEGIN ") {
				if !strings.Contains(line, "-----END ") {
					pemLeft, pemOpen = maxPEMLines, true
				}
				line = "[key block removed]"
			}
			if out.Summary == "" {
				out.Summary = summaryOf(e.tag, line)
			}
			size := len(cutRunes(line, maxCrashLineRunes)) + 1
			if len(out.Lines) >= maxLines || bytes+size > maxBytes {
				out.Truncated = true
				continue
			}
			out.Lines = append(out.Lines, line)
			bytes += size
		}
	}
	if pemOpen {
		out.Truncated = true // the buffer ended inside a key block
	}
	return out
}

var (
	javaThrowableRE = regexp.MustCompile(`^(?:[A-Za-z_$][\w$]*\.)+[\w$]*(?:Exception|Error|Throwable)\b`)
	fatalSignalRE   = regexp.MustCompile(`^Fatal signal \d+ \([A-Z0-9_]+\)`)
)

// summaryOf returns the line when it is the one that names the failure, or "".
func summaryOf(tag, line string) string {
	switch {
	case tag == "AndroidRuntime" && javaThrowableRE.MatchString(line):
		return line
	case tag == "libc":
		return fatalSignalRE.FindString(line)
	}
	return ""
}

// CrashOptions bound and mask one collection.
type CrashOptions struct {
	// Redact masks known secrets and credential patterns in text. It is required: the evidence carries text of the app under test, which can hold a token, and
	// nothing is returned without it. It must not return text longer than the text it is given plus a fixed marker per match.
	Redact func(string) string
	// PID selects the exit entry (and the crash-buffer lines) of one process, normally the pid the app had before it died. Zero means the newest entry of the app.
	PID int
	// User is the Android user whose entries are kept (default 0, the main user); entries of other users are dropped. An entry whose dump does not print a user is kept.
	User int
	// DeviceNow is the device's wall clock (zone-less, as the exit entries print it), when the caller has read it; it makes the check against future stamps exact. Zero uses this machine's clock with two days of slack.
	DeviceNow time.Time
	// Timeout bounds the whole collection (default 10 seconds). MaxLines and MaxBytes bound the kept crash text (defaults 200 lines, 32 KiB).
	Timeout  time.Duration
	MaxLines int
	MaxBytes int
}

// CrashEvidence is what one collection found. Every string in it has been cleaned and masked.
type CrashEvidence struct {
	Package string
	// Exit is the entry that was chosen (the one with opts.PID, or the newest); nil when the device listed none for the app.
	Exit *ExitRecord
	// Exits are all the entries kept for the app, newest first.
	Exits []ExitRecord
	// Summary names the failure in one line (see CrashText.Summary), masked; it can be empty.
	Summary string
	// CrashLines are the crash-buffer lines of the chosen process, masked.
	CrashLines []string
	Truncated  bool
}

// CrashCollectError is the only error type the collector returns. Its text is fixed, it never contains text from the device or from adb, and it unwraps to the
// context error when the collection ran out of time or was cancelled, so a caller can tell the two apart.
type CrashCollectError struct {
	Step  string // "exit-info", "crash buffer" or "options"
	cause error  // only a context error is kept
}

func (e *CrashCollectError) Error() string {
	return "crash evidence: could not collect (" + e.Step + ")"
}
func (e *CrashCollectError) Unwrap() error { return e.cause }

// contextCause returns the context's own error when the collection's context is over (a deadline that passed or a cancel), whatever error adb gave back, and
// nil otherwise. It never returns the adb error.
func contextCause(ctx context.Context, err error) error {
	if c := ctx.Err(); c != nil {
		return c
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	return nil
}

// redactor calls the caller's masking function and survives a panic in it: a masking function that panics must not take the caller's run down, and nothing
// is returned from a collection in which it failed.
type redactor struct {
	f      func(string) string
	failed bool
}

func (r *redactor) apply(s string) (out string) {
	if r.failed {
		return ""
	}
	defer func() {
		if recover() != nil {
			r.failed, out = true, ""
		}
	}()
	return r.f(s)
}

// CollectCrashEvidence runs two read-only commands on one device, `dumpsys activity exit-info <package>` and `logcat -b crash -d`, each named with -s <serial>,
// and returns what they say about the app. It changes nothing on the device: no clear, no forward, no shell command beyond the two. Every string in the result,
// the process names included, has been cleaned, masked and cut.
func CollectCrashEvidence(ctx context.Context, runner AdbRunner, serial, pkg string, opts CrashOptions) (CrashEvidence, error) {
	if runner == nil || strings.TrimSpace(serial) == "" || !validPackage(pkg) || opts.Redact == nil {
		return CrashEvidence{}, &CrashCollectError{Step: "options"}
	}
	red := &redactor{f: opts.Redact}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultCrashTimeout
	}
	maxBytes := opts.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultCrashTextBytes
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	exitOut, err := runner.RunArgs(ctx, "-s", serial, "shell", "dumpsys", "activity", "exit-info", shellquote.Arg(pkg))
	if err != nil {
		return CrashEvidence{}, &CrashCollectError{Step: "exit-info", cause: contextCause(ctx, err)}
	}
	ev := CrashEvidence{Package: pkg}
	for _, rec := range ParseExitInfoFiltered(string(exitOut), pkg, ExitFilter{OnlyUser: true, User: opts.User, Now: opts.DeviceNow}) {
		rec.Process = cutRunes(red.apply(rec.Process), maxProcessRunes)
		rec.Description = cutRunes(red.apply(rec.Description), maxExitDescription)
		ev.Exits = append(ev.Exits, rec)
	}
	for i := range ev.Exits {
		if opts.PID == 0 || ev.Exits[i].PID == opts.PID {
			ev.Exit = &ev.Exits[i]
			break
		}
	}
	pid := opts.PID
	if pid == 0 && ev.Exit != nil {
		pid = ev.Exit.PID
	}
	if pid == 0 {
		return finishEvidence(ev, red) // nothing to read the crash buffer for
	}
	bufOut, err := runner.RunArgs(ctx, "-s", serial, "logcat", "-b", "crash", "-d", "-v", "threadtime")
	if err != nil {
		return ev, &CrashCollectError{Step: "crash buffer", cause: contextCause(ctx, err)}
	}
	ct := ParseCrashBuffer(string(bufOut), pid, pkg, opts.MaxLines, maxBytes)
	ev.Truncated = ct.Truncated
	kept := 0
	for _, l := range ct.Lines {
		masked := cutRunes(red.apply(l), maxCrashLineRunes)
		// the byte budget is counted on the masked text, which a masking function may have made longer
		if kept+len(masked)+1 > maxBytes {
			ev.Truncated = true
			break
		}
		kept += len(masked) + 1
		ev.CrashLines = append(ev.CrashLines, masked)
	}
	ev.Summary = cutRunes(red.apply(ct.Summary), maxCrashSummary)
	return finishEvidence(ev, red)
}

// finishEvidence returns the evidence, or an options error when the masking function panicked on any of it.
func finishEvidence(ev CrashEvidence, red *redactor) (CrashEvidence, error) {
	if red.failed {
		return CrashEvidence{}, &CrashCollectError{Step: "options"}
	}
	return ev, nil
}

// cleanDeviceText removes what could hide or reorder text on a screen or in a report, by the rule the WebView text uses (hiddenRune), and turns a tab into a space.
// Visible text is unchanged.
func cleanDeviceText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for len(s) > 0 {
		r, n := utf8.DecodeRuneInString(s)
		s = s[n:]
		switch {
		case r == '\t':
			b.WriteByte(' ')
		case hiddenRune(r):
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// cutRunes keeps at most n runes, and marks a cut with an ellipsis (counted in n).
func cutRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
}

// capBytes keeps at most n bytes, without cutting a character in half.
func capBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[:n]
	for len(s) > 0 && !utf8.ValidString(s[max(0, len(s)-4):]) {
		s = s[:len(s)-1]
	}
	return s
}
