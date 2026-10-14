// DeviceConnection logic against a fake WebSocket: close codes, the
// "taken" state, reconnect(), and the flapping guard for the backoff counter.
import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { DeviceConnection } from "../src/device-view/connection.ts";
import type { WebSocketLike } from "../src/device-view/connection.ts";
import type { Status } from "../src/device-view/protocol.ts";
import { FakeTimers } from "./helpers.ts";

class FakeWS implements WebSocketLike {
  static all: FakeWS[] = [];
  binaryType = "";
  readyState = 1;
  onopen: ((ev: unknown) => void) | null = null;
  onmessage: ((ev: { data: unknown }) => void) | null = null;
  onclose: ((ev: unknown) => void) | null = null;
  onerror: ((ev: unknown) => void) | null = null;
  sent: string[] = [];
  url: string;
  constructor(url: string) {
    this.url = url;
    FakeWS.all.push(this);
  }
  send(d: string): void {
    this.sent.push(d);
  }
  close(): void {
    this.readyState = 3;
  }
  hello(): void {
    this.onmessage?.({
      data: JSON.stringify({
        t: "hello",
        v: 1,
        device: { id: "d", platform: "p", name: "n" },
        screen: { w: 100, h: 200, dpr: 1 },
        caps: { input: true, hit: true, bounds: true, jpeg: true },
      }),
    });
  }
  closeWith(code: number): void {
    this.readyState = 3;
    this.onclose?.({ code });
  }
}

function setup(clock: { t: number }) {
  FakeWS.all = [];
  const timers = new FakeTimers();
  const statuses: Status[] = [];
  const conn = new DeviceConnection({
    url: "ws://127.0.0.1:4000/device/v1",
    token: "tok",
    WebSocketImpl: FakeWS,
    timers,
    now: () => clock.t,
    handlers: {
      onStatus: (s) => statuses.push(s),
      onHello: () => undefined,
      onFrame: () => undefined,
      onTree: () => undefined,
      onHit: () => undefined,
      onAck: () => undefined,
    },
  });
  return { conn, timers, statuses };
}

describe("DeviceConnection close handling", () => {
  it("close code 4001 reports taken and does not reconnect", () => {
    const { conn, timers, statuses } = setup({ t: 0 });
    conn.start();
    const ws = FakeWS.all[0]!;
    ws.hello();
    ws.closeWith(4001);
    assert.equal(statuses.at(-1)!.state, "taken");
    assert.equal(timers.pending.size, 0);
    assert.equal(FakeWS.all.length, 1);
    // Becoming visible again must not take the stream back either.
    conn.retryNow();
    assert.equal(FakeWS.all.length, 1);
  });

  it("reconnect() after taken opens a new connection and returns to connected", () => {
    const { conn, timers, statuses } = setup({ t: 0 });
    conn.start();
    FakeWS.all[0]!.hello();
    FakeWS.all[0]!.closeWith(4001);
    conn.reconnect();
    assert.equal(FakeWS.all.length, 2);
    FakeWS.all[1]!.hello();
    assert.equal(statuses.at(-1)!.state, "connected");
    assert.equal(timers.pending.size, 0);
    conn.destroy();
  });

  it("other close codes still retry with backoff", () => {
    const { conn, timers, statuses } = setup({ t: 0 });
    conn.start();
    FakeWS.all[0]!.hello();
    FakeWS.all[0]!.closeWith(1006);
    assert.deepEqual(timers.delays(), [500]);
    assert.equal(statuses.at(-1)!.state, "reconnecting");
    conn.destroy();
  });

  it("a hello within 5 seconds of the previous one does not reset the backoff", () => {
    const clock = { t: 0 };
    const { conn, timers } = setup(clock);
    conn.start();
    FakeWS.all[0]!.hello();
    const seen: number[] = [];
    for (let i = 0; i < 4; i++) {
      clock.t += 400; // flapping: connected for less than 5 seconds
      FakeWS.all.at(-1)!.closeWith(1006);
      seen.push(timers.delays()[0]!);
      timers.runAll();
      FakeWS.all.at(-1)!.hello();
    }
    assert.deepEqual(seen, [500, 1000, 2000, 4000]);
    // A stable connection (more than 5 seconds) resets it.
    clock.t += 6000;
    FakeWS.all.at(-1)!.hello();
    FakeWS.all.at(-1)!.closeWith(1006);
    assert.deepEqual(timers.delays(), [500]);
    conn.destroy();
  });
});
