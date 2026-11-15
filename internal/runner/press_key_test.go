package runner

import (
	"context"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

// keyRecorder records the device actions the executor sends to the agent.
type keyRecorder struct {
	fakeAIClient
	action, value string
	calls         int
}

func (k *keyRecorder) DeviceAction(ctx context.Context, action, value string) error {
	k.action, k.value = action, value
	k.calls++
	return nil
}

// TestPressKeyReachesTheAgent checks that a `press <key>` step reaches the agent as the press_key device action with the key
// name. On iOS the step is refused before anything is sent: the agent's key handling is not verified there yet.
func TestPressKeyReachesTheAgent(t *testing.T) {
	rec := &keyRecorder{}
	e := &Executor{client: rec, vars: map[string]string{}}
	if err := e.runAction(context.Background(), parser.ActionStep{Verb: parser.VerbPressKey, Name: "backspace"}); err != nil {
		t.Fatalf("runAction: %v", err)
	}
	if rec.calls != 1 || rec.action != "press_key" || rec.value != "backspace" {
		t.Errorf("sent %q %q (%d calls); want press_key backspace", rec.action, rec.value, rec.calls)
	}

	ios := &keyRecorder{}
	e = &Executor{client: ios, vars: map[string]string{}, deviceCtx: &DeviceContext{Platform: device.PlatformIOS}}
	if err := e.runAction(context.Background(), parser.ActionStep{Verb: parser.VerbPressKey, Name: "tab"}); err == nil {
		t.Error("press tab on iOS: want a refusal")
	}
	if ios.calls != 0 {
		t.Errorf("press tab on iOS sent %d device actions, want none", ios.calls)
	}
}
