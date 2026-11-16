package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/device"
)

// TestAskClearDataGate checks the one gate for a data wipe, for a single device and for a lane: --yes, a person's answer, and the hint
// when nobody is there to ask.
func TestAskClearDataGate(t *testing.T) {
	for _, tc := range []struct {
		name       string
		allow      bool // --yes
		hasConfirm bool
		answer     bool // what the confirmation says
		noPrompt   bool // a lane
		perTest    bool
		wantErr    string // "" = the wipe goes ahead; otherwise a substring of the error
		wantAsks   int
	}{
		{name: "--yes lets the step through without asking", allow: true, wantAsks: 0},
		{name: "no --yes and no confirmation: the hint", wantErr: "pass --yes (-y)", wantAsks: 0},
		{name: "a person's yes lets it through", hasConfirm: true, answer: true, wantAsks: 1},
		{name: "a person's no is cancelled", hasConfirm: true, answer: false, wantErr: "clear app data: cancelled by user", wantAsks: 1},
		{name: "a lane's no is the hint, not cancelled", hasConfirm: true, answer: false, noPrompt: true, wantErr: "pass --yes (-y)", wantAsks: 1},
		{name: "a lane with no confirmation: the hint", noPrompt: true, wantErr: "pass --yes (-y)", wantAsks: 0},
		{name: "the per-test question names the setting", hasConfirm: true, answer: false, perTest: true,
			wantErr: "clear data before each test: cancelled by user", wantAsks: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			asks := 0
			var msg string
			dc := &DeviceContext{AppID: "com.example.app", Serial: "emulator-5590", AllowClearData: tc.allow, NoPrompt: tc.noPrompt}
			if tc.hasConfirm {
				dc.Confirm = func(m string) bool {
					asks++
					msg = m
					return tc.answer
				}
			}
			err := dc.AskClearData(tc.perTest)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("the wipe must go ahead, got %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("the error must contain %q, got %v", tc.wantErr, err)
			}
			if asks != tc.wantAsks {
				t.Errorf("questions asked = %d, want %d", asks, tc.wantAsks)
			}
			if tc.perTest && asks == 1 && !strings.Contains(msg, "BEFORE EVERY TEST") {
				t.Errorf("the per-test question must say it runs before every test, got %q", msg)
			}
		})
	}
}

// TestLaneGateAnswersEveryLaneTheSameWay runs each gate over the lanes a run can have (an emulator the person allowed, an emulator they did
// not, a physical phone, an iOS simulator and an iOS phone). A lane's question is answered by its gate, and a gate never asks.
func TestLaneGateAnswersEveryLaneTheSameWay(t *testing.T) {
	type lane struct {
		platform device.Platform
		serial   string
	}
	lanes := []lane{
		{device.PlatformAndroid, "emulator-5590"},
		{device.PlatformAndroid, "R58M12ABCDE"},
		{device.PlatformIOS, "SIM-UDID-1"},
		{device.PlatformIOS, "00008030-PHONE"},
	}
	verified := map[string]bool{"emulator-5590": true, "SIM-UDID-1": true}
	// A lane confirmation as the cli builds it: it answers from the device's verification and never asks.
	allowVerified := func(platform device.Platform, serial string) ConfirmFunc {
		return func(string) bool { return verified[serial] }
	}
	for _, tc := range []struct {
		name string
		gate ClearGate
		// the lanes whose wipe goes ahead
		wipes map[string]bool
	}{
		{"--yes: every lane wipes", ClearGate{Yes: true}, map[string]bool{"emulator-5590": true, "R58M12ABCDE": true, "SIM-UDID-1": true, "00008030-PHONE": true}},
		{"no --yes, no lane confirmation: no lane wipes", ClearGate{}, map[string]bool{}},
		{"no --yes, verified devices allowed: only the verified lanes wipe", ClearGate{Lane: allowVerified}, map[string]bool{"emulator-5590": true, "SIM-UDID-1": true}},
		{"--yes and verified devices allowed: every lane wipes", ClearGate{Yes: true, Lane: allowVerified}, map[string]bool{"emulator-5590": true, "R58M12ABCDE": true, "SIM-UDID-1": true, "00008030-PHONE": true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, l := range lanes {
				dc := &DeviceContext{Platform: l.platform, Serial: l.serial, AppID: "com.example.app"}
				dc.ApplyClearGate(tc.gate)
				// the clear step and the question asked once before a run's wipes (clear data before each test) answer the same way
				for _, perTest := range []bool{false, true} {
					wiped := dc.AskClearData(perTest) == nil
					if wiped != tc.wipes[l.serial] {
						t.Errorf("lane %s (%s), perTest=%v: wipe = %v, want %v", l.serial, l.platform, perTest, wiped, tc.wipes[l.serial])
					}
				}
			}
		})
	}
}

// TestLaneContextsTakeTheClearGate checks the wiring: a parallel run's lane contexts get the run's gate, with the lane's platform and serial,
// and never a prompt of their own.
func TestLaneContextsTakeTheClearGate(t *testing.T) {
	dir := t.TempDir()
	fine := writeHarnessFile(t, dir, "fine.probe", "test \"fine\"\n  tap \"Fine\"\n")
	h := newHarnessRun(t, harnessConfig(), [][]string{{fine}})
	po := h.po
	dr := &po.devices[0]

	po.opts.ClearGate = ClearGate{Yes: false}
	dc := po.deviceContext(context.Background(), dr, "com.example.app", time.Second, time.Second)
	if dc.AllowClearData || !dc.NoPrompt || dc.Confirm != nil {
		t.Errorf("no --yes and no lane confirmation: AllowClearData=%v NoPrompt=%v Confirm=%v, want false, true, nil",
			dc.AllowClearData, dc.NoPrompt, dc.Confirm != nil)
	}

	po.opts.ClearGate = ClearGate{Yes: true}
	dc = po.deviceContext(context.Background(), dr, "com.example.app", time.Second, time.Second)
	if !dc.AllowClearData || !dc.NoPrompt {
		t.Errorf("--yes: AllowClearData=%v NoPrompt=%v, want true, true", dc.AllowClearData, dc.NoPrompt)
	}

	var gotPlatform device.Platform
	var gotSerial string
	po.opts.ClearGate = ClearGate{Lane: func(p device.Platform, s string) ConfirmFunc {
		gotPlatform, gotSerial = p, s
		return nil
	}}
	dc = po.deviceContext(context.Background(), dr, "com.example.app", time.Second, time.Second)
	if gotPlatform != dr.Platform || gotSerial != dr.DeviceID {
		t.Errorf("the lane confirmation got (%v, %q), want the lane's (%v, %q)", gotPlatform, gotSerial, dr.Platform, dr.DeviceID)
	}
	if dc.AllowClearData || !dc.NoPrompt || dc.Confirm != nil {
		t.Errorf("a lane with a nil confirmation must refuse with the hint: AllowClearData=%v NoPrompt=%v Confirm=%v", dc.AllowClearData, dc.NoPrompt, dc.Confirm != nil)
	}
}

// TestLaneContextRefusesEveryClearWithoutYes is the regression for a repo that sets clear data before each test on a parallel run: a lane
// context, built the way the orchestrator builds it, refuses every clear that is not --yes. That covers the clear step and the question asked
// before a run's wipes, and a refused clear never reaches adb's pm clear.
func TestLaneContextRefusesEveryClearWithoutYes(t *testing.T) {
	dir := t.TempDir()
	fine := writeHarnessFile(t, dir, "fine.probe", "test \"fine\"\n  tap \"Fine\"\n")
	adb := newHarnessADB(t)
	h := newHarnessRunADB(t, harnessConfig(), [][]string{{fine}}, adb)
	po := h.po
	po.opts.ClearGate = ClearGate{Yes: false} // no --yes, no lane confirmation
	dc := po.deviceContext(context.Background(), &po.devices[0], "com.example.app", time.Second, time.Second)
	for _, perTest := range []bool{false, true} {
		if err := dc.AskClearData(perTest); err == nil || !strings.Contains(err.Error(), "pass --yes") {
			t.Errorf("perTest=%v: a lane without --yes must refuse with the hint, got %v", perTest, err)
		}
	}
	if err := dc.ClearAppData(context.Background()); err == nil || !strings.Contains(err.Error(), "pass --yes") {
		t.Errorf("the clear step on a lane without --yes must refuse with the hint, got %v", err)
	}
	log, _ := os.ReadFile(filepath.Join(filepath.Dir(adb), "adb.log"))
	if strings.Contains(string(log), "pm clear") {
		t.Errorf("a refused lane must not reach pm clear; the adb log says:\n%s", log)
	}
}
