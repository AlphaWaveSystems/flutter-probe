import { describe, it } from "node:test";
import assert from "node:assert/strict";
import {
  MAX_CLIENT_MESSAGE_BYTES,
  ProtocolError,
  chunkText,
  decodeFrame,
  encodeClientMessage,
  isSafeWsUrl,
  parseServerMessage,
} from "../src/device-view/protocol.ts";

function makeFrame(header: unknown, image: Uint8Array = new Uint8Array([1, 2, 3]), lenOverride?: number): Uint8Array {
  const h = typeof header === "string" ? new TextEncoder().encode(header) : new TextEncoder().encode(JSON.stringify(header));
  const out = new Uint8Array(5 + h.length + image.length);
  out[0] = 0x01;
  new DataView(out.buffer).setUint32(1, lenOverride ?? h.length, false);
  out.set(h, 5);
  out.set(image, 5 + h.length);
  return out;
}

const good = { seq: 7, ts: 1700000000000, w: 360, h: 780, fmt: "jpeg" };

describe("decodeFrame", () => {
  it("decodes a valid frame and exposes the image bytes", () => {
    const f = decodeFrame(makeFrame(good, new Uint8Array([9, 8, 7, 6])));
    assert.equal(f.seq, 7);
    assert.equal(f.w, 360);
    assert.equal(f.h, 780);
    assert.equal(f.fmt, "jpeg");
    assert.deepEqual([...f.image], [9, 8, 7, 6]);
  });

  it("accepts an ArrayBuffer and a subarray with a byte offset", () => {
    const frame = makeFrame({ ...good, fmt: "png" });
    const padded = new Uint8Array(frame.length + 10);
    padded.set(frame, 10);
    assert.equal(decodeFrame(padded.subarray(10)).fmt, "png");
    assert.equal(decodeFrame(frame.buffer as ArrayBuffer).seq, 7);
  });

  const bad: Array<[string, () => Uint8Array]> = [
    ["empty", () => new Uint8Array(0)],
    ["shorter than the fixed prefix", () => new Uint8Array([1, 0, 0, 0])],
    ["wrong type byte", () => { const f = makeFrame(good); f[0] = 0x02; return f; }],
    ["zero header length", () => makeFrame(good, undefined, 0)],
    ["huge header length", () => makeFrame(good, undefined, 0xffffffff)],
    ["header length past the end", () => makeFrame(good, undefined, 5000)],
    ["header length consuming all bytes (no image)", () => makeFrame(good, new Uint8Array(0))],
    ["truncated header", () => makeFrame(good).subarray(0, 12)],
    ["header is not JSON", () => makeFrame("{not json")],
    ["header is an array", () => makeFrame([1, 2])],
    ["header is invalid UTF-8", () => {
      const f = makeFrame("xxxx");
      f[5] = 0xff; f[6] = 0xfe;
      return f;
    }],
    ["missing seq", () => makeFrame({ ...good, seq: undefined })],
    ["negative seq", () => makeFrame({ ...good, seq: -1 })],
    ["fractional seq", () => makeFrame({ ...good, seq: 1.5 })],
    ["non-numeric ts", () => makeFrame({ ...good, ts: "now" })],
    ["zero width", () => makeFrame({ ...good, w: 0 })],
    ["absurd height", () => makeFrame({ ...good, h: 1e9 })],
    ["unknown format", () => makeFrame({ ...good, fmt: "gif" })],
  ];
  for (const [name, build] of bad) {
    it(`rejects: ${name}`, () => {
      assert.throws(() => decodeFrame(build()), ProtocolError);
    });
  }
});

describe("parseServerMessage", () => {
  it("parses hello with defaults for missing parts", () => {
    const m = parseServerMessage(JSON.stringify({ t: "hello", v: 1, screen: { w: 360, h: 780 }, caps: { input: true } }));
    assert.ok(m && m.t === "hello");
    assert.equal(m.screen.dpr, 1);
    assert.equal(m.caps.input, true);
    assert.equal(m.caps.hit, false);
    assert.equal(m.device.id, "");
  });

  it("rejects hello without a usable screen", () => {
    assert.equal(parseServerMessage('{"t":"hello","screen":{"w":0,"h":10}}'), null);
  });

  it("drops tree nodes with non-numeric bounds and keeps the rest", () => {
    const m = parseServerMessage(
      JSON.stringify({
        t: "tree", seq: 3,
        nodes: [
          { i: 0, p: -1, type: "A", key: "k", text: null, label: null, x: 0, y: 0, w: 10, h: 10, interactive: true },
          { i: 1, p: 0, type: "B", x: "0", y: 0, w: 1, h: 1 },
          "junk",
        ],
      }),
    );
    assert.ok(m && m.t === "tree");
    assert.equal(m.nodes.length, 1);
    assert.equal(m.nodes[0]!.key, "k");
  });

  it("parses hit with null node, ack and status", () => {
    const hit = parseServerMessage('{"t":"hit","id":4,"node":null}');
    assert.deepEqual(hit, { t: "hit", id: 4, node: null });
    const ack = parseServerMessage('{"t":"ack","id":2,"ok":false,"err":"bad"}');
    assert.deepEqual(ack, { t: "ack", id: 2, ok: false, err: "bad" });
    const st = parseServerMessage('{"t":"status","state":"error","msg":"x"}');
    assert.deepEqual(st, { t: "status", state: "error", msg: "x" });
  });

  it("returns null for unknown types, non-objects and invalid JSON", () => {
    assert.equal(parseServerMessage('{"t":"nope"}'), null);
    assert.equal(parseServerMessage("[1]"), null);
    assert.equal(parseServerMessage("{"), null);
    assert.equal(parseServerMessage('{"x":1}'), null);
  });
});

describe("encodeClientMessage limits", () => {
  it("encodes valid messages", () => {
    assert.equal(encodeClientMessage({ t: "tap", x: 1, y: 2 }), '{"t":"tap","x":1,"y":2}');
    assert.ok(encodeClientMessage({ t: "pause" }));
  });

  it("rejects non-finite and out-of-range coordinates", () => {
    assert.equal(encodeClientMessage({ t: "tap", x: NaN, y: 0 }), null);
    assert.equal(encodeClientMessage({ t: "tap", x: Infinity, y: 0 }), null);
    assert.equal(encodeClientMessage({ t: "tap", x: -1, y: 0 }), null);
    assert.equal(encodeClientMessage({ t: "tap", x: 0, y: 20001 }), null);
    assert.ok(encodeClientMessage({ t: "tap", x: 20000, y: 0 }));
    assert.equal(encodeClientMessage({ t: "swipe", x1: 0, y1: 0, x2: 30000, y2: 0, ms: 100 }), null);
    assert.equal(encodeClientMessage({ t: "hit", id: 1, x: 0, y: -0.5 }), null);
    assert.equal(encodeClientMessage({ t: "long_press", x: 1, y: 1, ms: NaN }), null);
  });

  it("rejects messages over 4 KiB measured in UTF-8 bytes", () => {
    assert.ok(encodeClientMessage({ t: "text", s: "a".repeat(1000) }));
    assert.equal(encodeClientMessage({ t: "text", s: "a".repeat(MAX_CLIENT_MESSAGE_BYTES) }), null);
    // 2000 two-byte characters is 4000 bytes of text plus JSON overhead: over the limit.
    assert.equal(encodeClientMessage({ t: "text", s: "é".repeat(2100) }), null);
  });
});

describe("chunkText", () => {
  it("keeps every chunk under the byte limit and loses nothing", () => {
    const s = "abé\u{1F600}".repeat(800);
    const chunks = chunkText(s, 1024);
    assert.ok(chunks.length > 1);
    assert.equal(chunks.join(""), s);
    for (const c of chunks) {
      assert.ok(new TextEncoder().encode(c).length <= 1024);
      assert.ok(encodeClientMessage({ t: "text", s: c }));
    }
  });

  it("returns nothing for empty text", () => {
    assert.deepEqual(chunkText(""), []);
  });
});

describe("isSafeWsUrl", () => {
  it("allows ws and wss without credentials only", () => {
    assert.equal(isSafeWsUrl("ws://127.0.0.1:1234/device/v1"), true);
    assert.equal(isSafeWsUrl("wss://example.test/x"), true);
    assert.equal(isSafeWsUrl("http://127.0.0.1/x"), false);
    assert.equal(isSafeWsUrl("javascript:alert(1)"), false);
    assert.equal(isSafeWsUrl("ws://user:pw@127.0.0.1/x"), false);
    assert.equal(isSafeWsUrl("not a url"), false);
  });
});
