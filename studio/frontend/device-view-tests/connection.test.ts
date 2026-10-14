// Integration test: DeviceConnection against the mock hub over loopback,
// using Node's built-in WebSocket client. No DOM, no network beyond 127.0.0.1.
import { after, describe, it } from "node:test";
import assert from "node:assert/strict";
import { DeviceConnection } from "../src/device-view/connection.ts";
import type { WebSocketCtor } from "../src/device-view/connection.ts";
import type { DecodedFrame, DeviceNode, Status } from "../src/device-view/protocol.ts";
import { startMockServer } from "../device-view-demo/mock-server.mjs";
import { FakeTimers, waitFor } from "./helpers.ts";

const NodeWS = globalThis.WebSocket as unknown as WebSocketCtor;
const TOKEN = "tok-secret-1234567890";

function setup(url: string, token: string, timers?: FakeTimers) {
  const log = {
    statuses: [] as Status[],
    hellos: 0,
    frames: [] as DecodedFrame[],
    trees: [] as DeviceNode[][],
    hits: [] as Array<[number, DeviceNode | null]>,
    acks: [] as Array<[number, boolean]>,
  };
  const conn = new DeviceConnection({
    url,
    token,
    WebSocketImpl: NodeWS,
    timers,
    handlers: {
      onStatus: (s) => log.statuses.push(s),
      onHello: () => log.hellos++,
      onFrame: (f) => log.frames.push(f),
      onTree: (n) => log.trees.push(n),
      onHit: (id, n) => log.hits.push([id, n]),
      onAck: (id, ok) => log.acks.push([id, ok]),
    },
  });
  return { conn, log };
}

describe("DeviceConnection against the mock hub", () => {
  const servers: Array<{ close(): Promise<void> }> = [];
  after(async () => {
    for (const s of servers) await s.close();
  });

  it("connects, receives hello and decodable PNG frames, and never leaks the token into statuses", async () => {
    const srv = await startMockServer({ token: TOKEN, fps: 30 });
    servers.push(srv);
    const { conn, log } = setup(srv.url, TOKEN);
    conn.start();
    await waitFor(() => log.hellos === 1 && log.frames.length >= 2, "hello and two frames");
    const f = log.frames[0]!;
    assert.equal(f.fmt, "png");
    assert.equal(f.w, 360);
    assert.deepEqual([...f.image.subarray(1, 4)], [0x50, 0x4e, 0x47]); // "PNG"
    assert.ok(log.frames[1]!.seq > f.seq);
    assert.ok(log.statuses.some((s) => s.state === "connected"));
    assert.ok(!JSON.stringify(log.statuses).includes(TOKEN));
    assert.equal(conn.malformedFrames, 0);
    conn.destroy();
    assert.equal(log.statuses.at(-1)!.state, "closed");
  });

  it("round-trips hit requests and input acks, and delivers input to the hub", async () => {
    const srv = await startMockServer({ token: TOKEN, fps: 5 });
    servers.push(srv);
    const { conn, log } = setup(srv.url, TOKEN);
    conn.start();
    await waitFor(() => conn.isOpen && log.hellos === 1, "hello");
    assert.equal(conn.send({ t: "hit", id: 41, x: 50, y: 320 }), true);
    await waitFor(() => log.hits.length === 1, "hit reply");
    assert.equal(log.hits[0]![0], 41);
    assert.equal(log.hits[0]![1]!.key, "loginButton");
    conn.send({ t: "hit", id: 42, x: 1, y: 1 });
    await waitFor(() => log.hits.length === 2, "second hit reply");
    assert.equal(log.hits[1]![1], null);

    assert.equal(conn.send({ t: "tap", x: 10, y: 20, id: 7 }), true);
    await waitFor(() => log.acks.length === 1, "ack");
    assert.deepEqual(log.acks[0], [7, true]);
    conn.send({ t: "text", s: "hello" });
    await waitFor(() => srv.received.some((m) => m.t === "text"), "text at hub");
    conn.destroy();
  });

  it("drops invalid messages locally and enforces the 50 messages/second budget", async () => {
    const srv = await startMockServer({ token: TOKEN, fps: 1 });
    servers.push(srv);
    const { conn, log } = setup(srv.url, TOKEN);
    conn.start();
    await waitFor(() => conn.isOpen && log.hellos === 1, "hello");
    assert.equal(conn.send({ t: "tap", x: NaN, y: 0 }), false);
    assert.equal(conn.send({ t: "text", s: "x".repeat(5000) }), false);
    let sent = 0;
    for (let i = 0; i < 80; i++) if (conn.send({ t: "tap", x: 1, y: 1 })) sent++;
    assert.equal(sent, 50);
    assert.equal(conn.droppedMessages, 30);
    // pause/resume are control messages and bypass the budget
    assert.equal(conn.send({ t: "pause" }), true);
    conn.destroy();
  });

  it("sends pause and resume, and config is replayed after reconnect", async () => {
    const srv = await startMockServer({ token: TOKEN, fps: 20 });
    servers.push(srv);
    const timers = new FakeTimers();
    const { conn, log } = setup(srv.url, TOKEN, timers);
    conn.start();
    await waitFor(() => log.hellos === 1, "hello");
    conn.setConfig({ fps: 10, tree: true });
    await waitFor(() => log.trees.length === 1, "tree after config");
    assert.equal(log.trees[0]!.length, 4);

    conn.setPaused(true);
    await waitFor(() => srv.received.some((m) => m.t === "pause"), "pause at hub");
    assert.equal(log.statuses.at(-1)!.state, "paused");
    conn.setPaused(false);
    await waitFor(() => srv.received.some((m) => m.t === "resume"), "resume at hub");

    // Drop the connection: client must schedule a reconnect with the first backoff delay.
    srv.received.length = 0;
    srv.dropClients();
    await waitFor(() => timers.pending.size === 1, "retry scheduled");
    assert.deepEqual(timers.delays(), [500]);
    assert.equal(log.statuses.at(-1)!.state, "reconnecting");
    timers.runAll();
    await waitFor(() => log.hellos === 2, "second hello");
    await waitFor(() => srv.received.some((m) => m.t === "config"), "config replayed");
    conn.destroy();
  });

  it("backs off exponentially against a wrong token and reports error without the token", async () => {
    const srv = await startMockServer({ token: TOKEN });
    servers.push(srv);
    const timers = new FakeTimers();
    const { conn, log } = setup(srv.url, "wrong-token", timers);
    conn.start();
    const delays: number[] = [];
    for (let i = 0; i < 6; i++) {
      await waitFor(() => timers.pending.size === 1, `retry ${i}`);
      delays.push(...timers.delays());
      timers.runAll();
    }
    assert.deepEqual(delays, [500, 1000, 2000, 4000, 8000, 8000]);
    assert.equal(log.hellos, 0);
    assert.ok(log.statuses.some((s) => s.state === "error"));
    const dump = JSON.stringify(log.statuses);
    assert.ok(!dump.includes("wrong-token") && !dump.includes(TOKEN));
    conn.destroy();
    assert.equal(timers.pending.size, 0);
  });

  it("rejects non-websocket URLs without opening a socket", () => {
    const { conn, log } = setup("http://127.0.0.1:1/x", TOKEN);
    conn.start();
    assert.equal(log.statuses.at(-1)!.state, "error");
    assert.equal(conn.isOpen, false);
  });
});
