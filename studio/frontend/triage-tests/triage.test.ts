import { test } from "node:test";
import assert from "node:assert/strict";
import { triageRows, type StepTriage } from "../src/triage.ts";

// The fixture app's known failing step (native-test-apps/studio-fixture): the rules give a wait line and a reason.
test("a suggested fix and a reason render as two rows, fix first", () => {
  const t: StepTriage = {
    fix: 'wait until "This text does not exist" appears',
    reason: 'see found no "This text does not exist" on screen after 0.0 s.',
  };
  assert.deepEqual(triageRows(t), [
    { label: "Suggested fix", text: 'wait until "This text does not exist" appears' },
    { label: "Reason", text: 'see found no "This text does not exist" on screen after 0.0 s.' },
  ]);
});

// An id selector has a reason and no fix: the Results pane shows the reason alone.
test("a reason without a fix renders one row", () => {
  assert.deepEqual(triageRows({ reason: "tap found no element with the id probe_open_paywall on screen after 1.0 s." }), [
    { label: "Reason", text: "tap found no element with the id probe_open_paywall on screen after 1.0 s." },
  ]);
});

// The Go side omits an empty fix; an empty string must not render a blank suggested-fix row either.
test("an empty fix is not shown", () => {
  assert.deepEqual(triageRows({ fix: "", reason: "x" }), [{ label: "Reason", text: "x" }]);
});
