package runner

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

type fakeStepRecorder struct {
	events  []StepEvent
	methods []string
	clients []probelink.ProbeClient
	err     error
}

func (r *fakeStepRecorder) BeginStep(client probelink.ProbeClient, ev StepEvent) error {
	r.methods = append(r.methods, "begin")
	r.clients = append(r.clients, client)
	r.events = append(r.events, ev)
	return r.err
}

func (r *fakeStepRecorder) EndStep(client probelink.ProbeClient, ev StepEvent) error {
	r.methods = append(r.methods, "end")
	r.clients = append(r.clients, client)
	r.events = append(r.events, ev)
	return r.err
}

// A recorder gets the element an action step acts on, for the verbs that act on one. The selector is the resolved one,
// and a typed value never reaches the target.
func TestStepTargetIsSetOnlyForElementVerbs(t *testing.T) {
	rec := &fakeStepRecorder{}
	e := NewExecutor(nil, nil, nil, time.Second, false)
	e.recorder = rec
	e.SetVar("item", "Sign in")
	sel := parser.Selector{Kind: parser.SelectorText, Text: "<item>"}

	e.emitStep(parser.ActionStep{Verb: parser.VerbTap, Sel: &sel}, "tap", StepStarted, 0, nil)
	e.emitStep(parser.ActionStep{Verb: parser.VerbType, Sel: &sel, Text: "hunter2"}, "type", StepStarted, 0, nil)
	e.emitStep(parser.ActionStep{Verb: parser.VerbGoBack}, "go back", StepStarted, 0, nil)
	e.emitStep(parser.ActionStep{Verb: parser.VerbTap, Sel: &sel}, "tap", StepPassed, time.Second, nil)
	if len(rec.events) != 4 {
		t.Fatalf("events=%d, want 4", len(rec.events))
	}
	tap, typed := rec.events[0].Target, rec.events[1].Target
	if tap == nil || tap.Verb != "tap" || tap.Text != `"Sign in"` || tap.Lookup.Text != "Sign in" {
		t.Fatalf("tap target = %+v", tap)
	}
	if typed == nil || typed.Verb != "type" || typed.Text != `"Sign in"` || typed.Lookup.Text != "Sign in" {
		t.Fatalf("type target = %+v", typed)
	}
	if rec.events[2].Target != nil {
		t.Fatalf("go back has a target: %+v", rec.events[2].Target)
	}
	if rec.events[3].Target != nil {
		t.Fatalf("an end event carries a target: %+v", rec.events[3].Target)
	}
}

func TestRecorderHookParsedTest(t *testing.T) {
	for _, mode := range []string{"record", "errors", "no recorder", "no callback"} {
		t.Run(mode, func(t *testing.T) {
			client := &scriptedClient{fakeAIClient: &fakeAIClient{}, tapAlwaysOK: true}
			recorder := &fakeStepRecorder{}
			if mode == "errors" {
				recorder.err = errors.New("recording failed")
			}
			calls := 0
			opts := RunOptions{Timeout: time.Second, DeviceName: "Test Phone"}
			if mode != "no recorder" {
				opts.NewRecorder = func(device string) StepRecorder {
					calls++
					if device != "Test Phone" {
						t.Errorf("device = %q", device)
					}
					return recorder
				}
			}
			r := New(&config.Config{}, client, nil, opts)
			var observed []StepEvent
			if mode != "no callback" {
				r.OnStep(func(ev StepEvent) { observed = append(observed, ev) })
			}
			prog, err := parser.ParseFile("test \"recorded\"\n  tap \"First\"\n  tap \"Second\"\n")
			if err != nil {
				t.Fatal(err)
			}
			result := r.runSingleTest(context.Background(), prog, prog.Tests[0], "recorded.probe", nil, -1)
			if !result.Passed || result.Error != nil {
				t.Fatalf("result: %+v", result)
			}
			events := observed
			if mode == "no callback" {
				events = recorder.events
			}
			if len(events) != 4 {
				t.Fatalf("events: %+v", events)
			}
			for i, ev := range events {
				want := StepStarted
				if i%2 == 1 {
					want = StepPassed
				}
				if ev.Status != want || ev.Line != 2+i/2 || ev.File != "recorded.probe" || ev.TestName != "recorded" || ev.Attempt != 1 || ev.Depth != 0 || ev.Description == "" || ev.At.IsZero() || ev.Error != nil {
					t.Errorf("event %d: %+v", i, ev)
				}
				if i%2 == 0 && ev.Duration != 0 {
					t.Errorf("start duration: %v", ev.Duration)
				}
			}
			if mode == "no recorder" {
				return
			}
			if !reflect.DeepEqual(recorder.methods, []string{"begin", "end", "begin", "end"}) {
				t.Errorf("methods: %v", recorder.methods)
			}
			if mode != "no callback" && !reflect.DeepEqual(observed, recorder.events) {
				t.Errorf("callback and recorder events differ")
			}
			for _, got := range recorder.clients {
				if got != client {
					t.Error("wrong recorder client")
				}
			}
			if r.newExecutor().recorder != recorder || calls != 1 {
				t.Errorf("recorder not reused; factory calls = %d", calls)
			}
		})
	}
}

func TestRecorderHookFailureStatuses(t *testing.T) {
	client := &scriptedClient{fakeAIClient: &fakeAIClient{}, tapFailN: 10}
	e := newScriptedExecutor(client)
	recorder := &fakeStepRecorder{err: errors.New("recording failed")}
	e.recorder = recorder
	for i, optional := range []bool{false, true} {
		err := e.RunStep(context.Background(), parser.ActionStep{Verb: parser.VerbTap, Sel: textSel("Missing"), Line: i + 1, Optional: optional})
		if (err == nil) != optional {
			t.Fatalf("optional=%v: %v", optional, err)
		}
	}
	if !reflect.DeepEqual(recorder.methods, []string{"begin", "end", "begin", "end"}) {
		t.Fatalf("methods: %v", recorder.methods)
	}
	if recorder.events[1].Status != StepFailed || recorder.events[3].Status != StepSkipped {
		t.Fatalf("events: %+v", recorder.events)
	}
}

func TestRecorderHookCompositeFactoryOncePerDevice(t *testing.T) {
	var names []string
	var recorders []*fakeStepRecorder
	devices := []CompositeDevice{
		{Alias: "a", DeviceName: "Phone A", Client: &scriptedClient{fakeAIClient: &fakeAIClient{}, tapAlwaysOK: true}},
		{Alias: "b", DeviceName: "Phone B", Client: &scriptedClient{fakeAIClient: &fakeAIClient{}, tapAlwaysOK: true}},
	}
	cr := NewCompositeRunner(&config.Config{}, devices, RunOptions{Timeout: time.Second, NewRecorder: func(name string) StepRecorder {
		names = append(names, name)
		rec := &fakeStepRecorder{}
		recorders = append(recorders, rec)
		return rec
	}})
	test := parser.CompositeTestDef{Name: "both", Body: []parser.Step{
		parser.DeviceStep{Alias: "a", Step: parser.ActionStep{Verb: parser.VerbTap, Sel: textSel("Go"), Line: 1}},
		parser.DeviceStep{Alias: "b", Step: parser.ActionStep{Verb: parser.VerbTap, Sel: textSel("Go"), Line: 2}},
	}}
	for i := 0; i < 2; i++ {
		if result := cr.RunCompositeTest(context.Background(), test, "both.probe"); !result.Passed {
			t.Fatalf("result: %+v", result)
		}
	}
	if !reflect.DeepEqual(names, []string{"Phone A", "Phone B"}) {
		t.Fatalf("factory names: %v", names)
	}
	for i, rec := range recorders {
		if len(rec.events) != 4 {
			t.Fatalf("device %d events: %+v", i, rec.events)
		}
		for _, client := range rec.clients {
			if client != devices[i].Client {
				t.Error("wrong composite client")
			}
		}
	}
}
