// Small timing helpers with injectable clocks so they are unit-testable.

export const BACKOFF_MIN_MS = 500;
export const BACKOFF_MAX_MS = 8000;

/** Exponential backoff: 500, 1000, 2000, 4000, 8000, 8000 ... */
export function backoffDelay(attempt: number): number {
  const a = Math.max(0, Math.floor(attempt));
  return Math.min(BACKOFF_MAX_MS, BACKOFF_MIN_MS * 2 ** Math.min(a, 10));
}

/** Allows at most one call per `intervalMs`; `tryAcquire` reports whether now is allowed. */
export class RateGate {
  private last = -Infinity;
  private readonly intervalMs: number;
  private readonly now: () => number;
  constructor(intervalMs: number, now: () => number) {
    this.intervalMs = intervalMs;
    this.now = now;
  }
  tryAcquire(): boolean {
    const t = this.now();
    if (t - this.last < this.intervalMs) return false;
    this.last = t;
    return true;
  }
  reset(): void {
    this.last = -Infinity;
  }
}

export interface Timers {
  setTimeout(fn: () => void, ms: number): unknown;
  clearTimeout(h: unknown): void;
}

export const realTimers: Timers = {
  setTimeout: (fn, ms) => setTimeout(fn, ms),
  clearTimeout: (h) => clearTimeout(h as ReturnType<typeof setTimeout>),
};

/** Collects typed text and flushes it at most once per `intervalMs`. */
export class TextBatcher {
  private buf = "";
  private handle: unknown = null;
  private readonly flushFn: (text: string) => void;
  private readonly intervalMs: number;
  private readonly timers: Timers;
  constructor(flushFn: (text: string) => void, intervalMs = 50, timers: Timers = realTimers) {
    this.flushFn = flushFn;
    this.intervalMs = intervalMs;
    this.timers = timers;
  }
  push(s: string): void {
    if (s === "") return;
    this.buf += s;
    if (this.handle === null) {
      this.handle = this.timers.setTimeout(() => this.flush(), this.intervalMs);
    }
  }
  flush(): void {
    if (this.handle !== null) {
      this.timers.clearTimeout(this.handle);
      this.handle = null;
    }
    if (this.buf === "") return;
    const s = this.buf;
    this.buf = "";
    this.flushFn(s);
  }
  cancel(): void {
    if (this.handle !== null) this.timers.clearTimeout(this.handle);
    this.handle = null;
    this.buf = "";
  }
}

/** Latest-wins slot: `offer` replaces anything not yet taken. */
export class LatestSlot<T> {
  private value: T | undefined;
  private has = false;
  /** Returns the value that was dropped (replaced), if any. */
  offer(v: T): T | undefined {
    const dropped = this.has ? this.value : undefined;
    this.value = v;
    this.has = true;
    return dropped;
  }
  take(): T | undefined {
    if (!this.has) return undefined;
    const v = this.value;
    this.value = undefined;
    this.has = false;
    return v;
  }
  clear(): void {
    this.value = undefined;
    this.has = false;
  }
}
