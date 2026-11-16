package runner

import (
	"context"
	"fmt"
	"sync"
)

// autoClearState remembers, for one run, the answer to the question "clear data before each test?": it is asked when the first test starts, and
// the answer (yes, or the reason for no) holds for the rest of the run, so a run is not asked once per test.
type autoClearState struct {
	mu     sync.Mutex
	asked  bool
	answer error
}

// clearBeforeTest wipes the app's data and relaunches before a test, using the executor of the attempt so the run follows the relaunched app. The
// first call asks (DeviceContext.AskClearData: --yes, or the confirmation, which on a simulator or an emulator the person's own setting can answer);
// a no is the error of every test of the run, and no test runs on data that was meant to be wiped.
func (r *Runner) clearBeforeTest(ctx context.Context, exec *Executor) error {
	r.autoClear.mu.Lock()
	if !r.autoClear.asked {
		r.autoClear.asked = true
		r.autoClear.answer = r.deviceCtx.AskClearData(true)
	}
	answer := r.autoClear.answer
	r.autoClear.mu.Unlock()
	if answer != nil {
		return answer // already named: "clear data before each test: ..."
	}
	if err := exec.clearAppData(ctx, true); err != nil {
		return fmt.Errorf("clear data before each test: %w", err)
	}
	return nil
}
