import type { Timers } from "../src/device-view/timing.ts";

export class FakeTimers implements Timers {
  pending = new Map<number, { fn: () => void; ms: number }>();
  private n = 0;
  setTimeout(fn: () => void, ms: number): unknown {
    const id = ++this.n;
    this.pending.set(id, { fn, ms });
    return id;
  }
  clearTimeout(h: unknown): void {
    this.pending.delete(h as number);
  }
  /** Fire all pending timers once, in creation order. */
  runAll(): void {
    const items = [...this.pending.entries()];
    this.pending.clear();
    for (const [, t] of items) t.fn();
  }
  delays(): number[] {
    return [...this.pending.values()].map((t) => t.ms);
  }
}

export async function waitFor(pred: () => boolean, what: string, timeoutMs = 3000): Promise<void> {
  const start = Date.now();
  while (!pred()) {
    if (Date.now() - start > timeoutMs) throw new Error(`timed out waiting for ${what}`);
    await new Promise((r) => setTimeout(r, 10));
  }
}
