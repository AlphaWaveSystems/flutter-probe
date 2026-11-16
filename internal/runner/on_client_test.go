package runner

import (
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

type onClientFake struct{ probelink.ProbeClient }

// When a step reconnects the run, the Runner keeps the new client and tells the caller (Studio's live view must follow the run).
func TestRunnerTellsTheCallerWhenAStepReconnects(t *testing.T) {
	first, second := &onClientFake{}, &onClientFake{}
	var got []probelink.ProbeClient
	r := New(&config.Config{}, first, nil, RunOptions{OnClient: func(c probelink.ProbeClient) { got = append(got, c) }})
	exec := r.newExecutor()
	if exec.onReconnect == nil {
		t.Fatal("the executor has no reconnect callback")
	}
	exec.onReconnect(second)
	if r.client != second {
		t.Error("the Runner kept the old client")
	}
	if len(got) != 1 || got[0] != second {
		t.Errorf("OnClient calls = %v, want the new client once", got)
	}
}

func TestRunnerWithoutOnClientStillSwapsTheClient(t *testing.T) {
	first, second := &onClientFake{}, &onClientFake{}
	r := New(&config.Config{}, first, nil, RunOptions{})
	r.newExecutor().onReconnect(second)
	if r.client != second {
		t.Error("the Runner kept the old client")
	}
}
