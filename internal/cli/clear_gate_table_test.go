package cli

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/runner"
)

// The clear gate as one table: every entry point that builds a device context for a clear, against every source a clear can be answered
// from. Each cell builds its context with the constructor a real run uses, and asks it through the real AskClearData. The iOS cells also ask
// the real AskReinstallForNotifications. The expected outcomes are written here, so a change to a rule breaks a row.
//
// Outcomes: wipe (the clear may go ahead), hint (refused, nobody there to ask: the hint to pass --yes), cancelled (a person said no).
// "n/t" marks a cell this table does not run: an iOS device whose setting is on is checked by simctl, which a test cannot use. The live check
// covers those cells.

const (
	tableAppID      = "com.example.app"
	verifiedAndroid = "emulator-5590" // listed by clearGateADB as a verified emulator
	physicalAndroid = "R58M12ABCDE"   // not listed: a physical phone
	simulatorIOS    = "SIM-UDID-1"
	physicalIOS     = "00008030-PHONE"
	keychainPhrase  = "also resets the keychain of ALL apps on this simulator"
	reinstallKept   = "Its keychain items are kept"
)

// entryKind is how a device context for a clear is built in a run.
type entryKind int

const (
	kindSingle    entryKind = iota // `probe test` on one device: singleDeviceContext, asks the person when there is a terminal
	kindLane                       // a parallel lane: NewLaneContext with the run's clear gate (nobody to ask)
	kindComposite                  // a composite device: compositeAndroidContext or compositeIOSContext with the run's clear gate
)

type entry struct {
	name     string
	platform device.Platform
	kind     entryKind
}

// answer is one source a clear can be answered from.
type answer struct {
	name       string
	yes        bool // --yes
	terminal   bool // a person is there to ask; personSays is the answer
	personSays bool
	setting    bool // the person's setting for simulators and emulators
	physical   bool // the device is a physical phone, not a simulator or emulator
}

// tableAnswers are the answer sources, in the order of the outcome rows below.
var tableAnswers = []answer{
	{name: "--yes", yes: true, terminal: true},
	{name: "person says yes", terminal: true, personSays: true},
	{name: "person says no", terminal: true},
	{name: "setting on, simulator or emulator, terminal present", terminal: true, setting: true},
	{name: "setting on, physical device, person says no", terminal: true, setting: true, physical: true},
	{name: "no terminal, setting off"},
	{name: "no terminal, setting on, simulator or emulator", setting: true},
}

// tableEntries are the entry points, in the order of the outcome rows below.
var tableEntries = []entry{
	{"single, Android", device.PlatformAndroid, kindSingle},
	{"single, iOS", device.PlatformIOS, kindSingle},
	{"parallel lane, Android", device.PlatformAndroid, kindLane},
	{"parallel lane, iOS", device.PlatformIOS, kindLane},
	{"composite, Android", device.PlatformAndroid, kindComposite},
	{"composite, iOS", device.PlatformIOS, kindComposite},
}

// wantOutcome[entry] lists the outcome of each answer source, in tableAnswers order.
var wantOutcome = map[string][]string{
	"single, Android":        {"wipe", "wipe", "cancelled", "wipe", "cancelled", "hint", "wipe"},
	"single, iOS":            {"wipe", "wipe", "cancelled", "wipe", "cancelled", "hint", "wipe"},
	"parallel lane, Android": {"wipe", "hint", "hint", "wipe", "hint", "hint", "wipe"},
	"parallel lane, iOS":     {"wipe", "hint", "hint", "n/t", "n/t", "hint", "n/t"},
	"composite, Android":     {"wipe", "hint", "hint", "wipe", "hint", "hint", "wipe"},
	"composite, iOS":         {"wipe", "hint", "hint", "n/t", "n/t", "hint", "n/t"},
}

// wantAsked[entry] is how many questions the person is asked for each answer source. Only a single device has a person to ask.
var wantAsked = map[string][]int{
	"single, Android":        {0, 1, 1, 0, 1, 0, 0},
	"single, iOS":            {0, 1, 1, 0, 1, 0, 0},
	"parallel lane, Android": {0, 0, 0, 0, 0, 0, 0},
	"parallel lane, iOS":     {0, 0, 0, 0, 0, 0, 0},
	"composite, Android":     {0, 0, 0, 0, 0, 0, 0},
	"composite, iOS":         {0, 0, 0, 0, 0, 0, 0},
}

func (a answer) serial(p device.Platform) string {
	switch {
	case p == device.PlatformIOS && a.physical:
		return physicalIOS
	case p == device.PlatformIOS:
		return simulatorIOS
	case a.physical:
		return physicalAndroid
	}
	return verifiedAndroid
}

// built is a device context ready to be asked, with the questions it put to the person.
type built struct {
	dc        *runner.DeviceContext
	questions []string
}

// build makes the context of one cell with the constructor the real run uses. The device manager is the fake adb of clearGateADB for
// Android; an iOS simulator is not a device a test can check, so an iOS cell gets no manager and a stub device check (see the setting rows).
func build(t *testing.T, e entry, a answer) *built {
	t.Helper()
	ctx := context.Background()
	bin, _ := clearGateADB(t)
	dm := device.NewManagerWithPaths(device.ToolPaths{ADB: bin})
	cfg := &config.Config{}
	cfg.Project.App = tableAppID
	serial := a.serial(e.platform)
	b := &built{}
	var ask func(string) bool
	if a.terminal {
		ask = func(msg string) bool {
			b.questions = append(b.questions, msg)
			return a.personSays
		}
	}
	mgr := dm
	if e.platform == device.PlatformIOS {
		mgr = nil
	}
	switch e.kind {
	case kindSingle:
		disposable := func() bool { return dm.IsVerifiedDisposable(ctx, e.platform, serial) }
		if e.platform == device.PlatformIOS {
			disposable = func() bool { return !a.physical } // stub: the iOS verification is the simulator check a test cannot make
		}
		confirm := confirmWithPrefs(ask, a.setting, disposable, io.Discard, tableAppID, serial)
		b.dc = singleDeviceContext(ctx, mgr, cfg, e.platform, serial, "", a.yes, false, confirm)
	case kindLane:
		gate := clearGateFor(ctx, dm, a.yes, a.setting, false, tableAppID)
		b.dc = runner.NewLaneContext(ctx, mgr, cfg, gate, e.platform, serial, tableAppID, 0, 0, 0)
	case kindComposite:
		gate := clearGateFor(ctx, dm, a.yes, a.setting, false, tableAppID)
		if e.platform == device.PlatformIOS {
			b.dc = compositeIOSContext(ctx, nil, serial, gate)
		} else {
			b.dc = compositeAndroidContext(ctx, cfg, dm, serial, gate)
		}
	}
	b.dc.AppID = tableAppID
	return b
}

// notRunnable is true for the cells this table does not run: an iOS lane or composite device whose setting is on needs simctl.
func notRunnable(e entry, a answer) bool {
	return e.platform == device.PlatformIOS && e.kind != kindSingle && a.setting
}

// outcomeOf names what a clear's answer did.
func outcomeOf(err error) string {
	switch {
	case err == nil:
		return "wipe"
	case strings.Contains(err.Error(), "pass --yes"):
		return "hint"
	case strings.Contains(err.Error(), "cancelled by user"):
		return "cancelled"
	}
	return "unexpected: " + err.Error()
}

// checkKeychainWording: an iOS clear's question and refusal name the keychain reset, and an Android clear's do not mention the keychain.
func checkKeychainWording(t *testing.T, p device.Platform, questions []string, err error) {
	t.Helper()
	msgs := append([]string(nil), questions...)
	if err != nil && outcomeOf(err) == "hint" {
		msgs = append(msgs, err.Error())
	}
	for _, m := range msgs {
		has := strings.Contains(m, keychainPhrase)
		if p == device.PlatformIOS && !has {
			t.Errorf("an iOS clear's question and refusal must name the keychain reset, got %q", m)
		}
		if p != device.PlatformIOS && strings.Contains(m, "keychain") {
			t.Errorf("an Android clear must not mention the keychain, got %q", m)
		}
	}
}

// TestClearGateTable walks every entry point against every answer source through the real AskClearData, and the iOS cells through the real
// AskReinstallForNotifications and the per-test question too.
func TestClearGateTable(t *testing.T) {
	for _, e := range tableEntries {
		want := wantOutcome[e.name]
		asked := wantAsked[e.name]
		for i, a := range tableAnswers {
			t.Run(e.name+" / "+a.name, func(t *testing.T) {
				if notRunnable(e, a) {
					t.Skip("needs a simulator verified by simctl: the live check covers it")
				}
				if want[i] == "n/t" {
					t.Fatalf("the table row marks this cell as not runnable, but it is runnable: fix the row")
				}
				b := build(t, e, a)
				err := b.dc.AskClearData(false)
				if got := outcomeOf(err); got != want[i] {
					t.Errorf("clear: outcome %s, want %s (%v)", got, want[i], err)
				}
				if len(b.questions) != asked[i] {
					t.Errorf("clear: questions asked %d, want %d", len(b.questions), asked[i])
				}
				checkKeychainWording(t, e.platform, b.questions, err)

				perTest := build(t, e, a)
				if got := outcomeOf(perTest.dc.AskClearData(true)); got != want[i] {
					t.Errorf("before each test: outcome %s, want %s", got, want[i])
				}
				for _, q := range perTest.questions {
					if !strings.Contains(q, "BEFORE EVERY TEST") {
						t.Errorf("the per-test question must say it runs before every test, got %q", q)
					}
				}

				if e.platform == device.PlatformIOS {
					re := build(t, e, a)
					rerr := re.dc.AskReinstallForNotifications()
					if got := outcomeOf(rerr); got != want[i] {
						t.Errorf("reinstall for notifications: outcome %s, want %s (%v)", got, want[i], rerr)
					}
					for _, q := range re.questions {
						if !strings.Contains(q, reinstallKept) {
							t.Errorf("the reinstall question must say its keychain items are kept, got %q", q)
						}
					}
				}
			})
		}
	}
}

// TestConfirmWithPrefsOutcomes: one row per outcome of the confirmation a run uses. The setting's semantics are these rows: a change that
// breaks one of them breaks a test, whatever entry point calls it.
func TestConfirmWithPrefsOutcomes(t *testing.T) {
	const question = "This will permanently delete ALL app data for com.example.app on SIM-UDID-1.\n" +
		"  SharedPreferences, databases, and files will be wiped. Continue?"
	for _, r := range []struct {
		name         string
		setting      bool // the person's setting for simulators and emulators
		verified     bool // the device is a verified simulator or emulator
		terminal     bool // a person is there to ask
		personSays   bool
		wantAnswer   bool
		wantAsked    int
		wantCheck    int  // how often the device is checked: only when the setting is on, and once per question
		wantNoPrompt bool // no confirmation at all (nil): nobody to ask and the setting is off
		wantLine     bool // the setting answers, and the line says what it answered
	}{
		{name: "setting off, person says yes", terminal: true, personSays: true, wantAnswer: true, wantAsked: 1},
		{name: "setting off, person says no", terminal: true, wantAsked: 1},
		{name: "setting off, no terminal", wantNoPrompt: true},
		{name: "setting on, verified, terminal present", setting: true, verified: true, terminal: true, wantAnswer: true, wantCheck: 1, wantLine: true},
		{name: "setting on, verified, no terminal", setting: true, verified: true, wantAnswer: true, wantCheck: 1, wantLine: true},
		{name: "setting on, not verified, person says yes", setting: true, terminal: true, personSays: true, wantAnswer: true, wantAsked: 1, wantCheck: 1},
		{name: "setting on, not verified, person says no", setting: true, terminal: true, wantAsked: 1, wantCheck: 1},
		{name: "setting on, not verified, no terminal", setting: true, wantCheck: 1},
	} {
		t.Run(r.name, func(t *testing.T) {
			var ask func(string) bool
			asked, checks := 0, 0
			if r.terminal {
				ask = func(string) bool { asked++; return r.personSays }
			}
			disposable := func() bool { checks++; return r.verified }
			var out bytes.Buffer
			confirm := confirmWithPrefs(ask, r.setting, disposable, &out, "com.example.app", "SIM-UDID-1")
			if r.wantNoPrompt {
				if confirm != nil {
					t.Fatal("with the setting off and no terminal there must be no confirmation at all")
				}
				return
			}
			if got := confirm(question); got != r.wantAnswer {
				t.Errorf("answer = %v, want %v", got, r.wantAnswer)
			}
			if asked != r.wantAsked {
				t.Errorf("questions asked = %d, want %d", asked, r.wantAsked)
			}
			if checks != r.wantCheck {
				t.Errorf("device checks = %d, want %d", checks, r.wantCheck)
			}
			line := out.String()
			if r.wantLine != (line != "") {
				t.Errorf("the automatic line printed = %v, want %v (%q)", line != "", r.wantLine, line)
			}
			if r.wantLine && (!strings.Contains(line, "no question for com.example.app on SIM-UDID-1") || strings.Contains(line, "Continue?")) {
				t.Errorf("the automatic line must say what it answered, and must not read as a question, got %q", line)
			}
		})
	}
}
