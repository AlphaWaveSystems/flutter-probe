package parser_test

import (
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

func TestPerfStatementsParse(t *testing.T) {
	steps := firstTest(t, `  start measuring "checkout"
  see memory below 300 MB
  see memory growth below 20.5 MB
  see cpu peak below 90 percent
  see slowest frame below 100 ms
  see data transferred below 2 MB
  see data transferred below 500 KB
  see response "/api/pay" below 800 ms
  stop measuring
`)
	if len(steps) != 9 {
		t.Fatalf("got %d steps: %#v", len(steps), steps)
	}
	if s := steps[0].(parser.PerfStep); s.Kind != parser.PerfStart || s.Name != "checkout" {
		t.Errorf("start = %+v", s)
	}
	want := []struct {
		metric string
		limit  float64
	}{{"memory", 300}, {"memory_growth", 20.5}, {"cpu_peak", 90}, {"frame_max", 100}, {"data", 2048}, {"data", 500}}
	for i, w := range want {
		s := steps[i+1].(parser.PerfStep)
		if s.Kind != parser.PerfCheck || s.Metric != w.metric || s.Limit != w.limit {
			t.Errorf("check %d = %+v, want %+v", i, s, w)
		}
	}
	h := steps[7].(parser.HTTPStep)
	if h.Kind != parser.HTTPSeeResponse || h.Check.Kind != parser.CheckTime || h.Check.Status != 800 {
		t.Errorf("response time = %+v", h)
	}
	if steps[8].(parser.PerfStep).Kind != parser.PerfStop {
		t.Errorf("stop = %+v", steps[8])
	}
}

func TestPerfWordsStayOrdinaryElsewhere(t *testing.T) {
	steps := firstTest(t, `  see "memory"
  tap "stop"
  start measuring
  see memory below 100
`)
	if _, ok := steps[0].(parser.AssertStep); !ok {
		t.Errorf("see \"memory\" must stay a text assertion: %#v", steps[0])
	}
	if _, ok := steps[1].(parser.ActionStep); !ok {
		t.Errorf("tap \"stop\" must stay a tap: %#v", steps[1])
	}
	if s := steps[2].(parser.PerfStep); s.Name != "" {
		t.Errorf("unnamed start = %+v", s)
	}
	if s := steps[3].(parser.PerfStep); s.Limit != 100 {
		t.Errorf("unit is optional: %+v", s)
	}
}

func TestPerfCheckErrors(t *testing.T) {
	for _, body := range []string{"  see memory below 100 furlongs\n"} {
		if _, err := parser.ParseFile("test \"t\"\n" + body); err == nil {
			t.Errorf("must be a syntax error: %q", body)
		}
	}
}
