import { describe, it } from "node:test";
import assert from "node:assert/strict";
import {
  backingSize,
  classifyGesture,
  deviceRectToCanvas,
  hitTestNodes,
  letterbox,
  mapPointer,
} from "../src/device-view/mapping.ts";
import { selectorFor, quoteString } from "../src/device-view/selector.ts";

const close = (a: number, b: number, eps = 1e-6) => assert.ok(Math.abs(a - b) < eps, `${a} != ${b}`);

describe("letterbox", () => {
  it("pillarboxes a tall device in a wide canvas", () => {
    const r = letterbox(800, 400, 360, 720); // scale 400/720
    close(r.h, 400);
    close(r.w, 200);
    close(r.x, 300);
    close(r.y, 0);
  });
  it("letterboxes a wide device in a tall canvas", () => {
    const r = letterbox(400, 800, 720, 360);
    close(r.w, 400);
    close(r.h, 200);
    close(r.y, 300);
  });
  it("returns an empty rect for degenerate sizes", () => {
    assert.deepEqual(letterbox(0, 10, 10, 10), { x: 0, y: 0, w: 0, h: 0 });
    assert.deepEqual(letterbox(10, 10, 0, 10), { x: 0, y: 0, w: 0, h: 0 });
  });
});

describe("backingSize", () => {
  it("scales by devicePixelRatio and guards bad values", () => {
    assert.deepEqual(backingSize(100, 50, 2), { w: 200, h: 100 });
    assert.deepEqual(backingSize(100.4, 50, 1.5), { w: 151, h: 75 });
    assert.deepEqual(backingSize(100, 50, 0), { w: 100, h: 50 });
    assert.deepEqual(backingSize(100, 50, NaN), { w: 100, h: 50 });
    assert.deepEqual(backingSize(0, 0, 2), { w: 1, h: 1 });
  });
});

describe("mapPointer", () => {
  const screen = { w: 360, h: 720 };

  it("maps through letterbox bars at dpr 1", () => {
    // 800x400 canvas, device drawn at x=300..500.
    const rect = { x: 0, y: 0, w: 800, h: 400 };
    const canvas = { w: 800, h: 400 };
    const c = mapPointer({ clientX: 400, clientY: 200, rect, canvas, screen })!;
    close(c.x, 180);
    close(c.y, 360);
    assert.equal(c.inside, true);
    const tl = mapPointer({ clientX: 300, clientY: 0, rect, canvas, screen })!;
    close(tl.x, 0);
    close(tl.y, 0);
  });

  it("is devicePixelRatio aware: backing store twice the CSS size gives the same result", () => {
    const rect = { x: 10, y: 20, w: 400, h: 200 };
    const at1 = mapPointer({ clientX: 210, clientY: 120, rect, canvas: { w: 400, h: 200 }, screen })!;
    const at2 = mapPointer({ clientX: 210, clientY: 120, rect, canvas: { w: 800, h: 400 }, screen })!;
    close(at1.x, at2.x);
    close(at1.y, at2.y);
    close(at2.x, 180);
    close(at2.y, 360);
  });

  it("handles fractional dpr (1.5) with rounded backing size", () => {
    const rect = { x: 0, y: 0, w: 333, h: 667 };
    const canvas = backingSize(333, 667, 1.5);
    const p = mapPointer({ clientX: 333, clientY: 667, rect, canvas, screen })!;
    assert.ok(p.x <= 360 && p.y <= 720);
    assert.ok(p.x > 359 && p.y > 719);
  });

  it("clamps pointers in the bars and outside the canvas, and reports inside=false", () => {
    const rect = { x: 0, y: 0, w: 800, h: 400 };
    const canvas = { w: 800, h: 400 };
    const left = mapPointer({ clientX: 10, clientY: 200, rect, canvas, screen })!;
    assert.equal(left.inside, false);
    close(left.x, 0);
    close(left.y, 360);
    const far = mapPointer({ clientX: 5000, clientY: -300, rect, canvas, screen })!;
    assert.equal(far.inside, false);
    close(far.x, 360);
    close(far.y, 0);
  });

  it("returns null when the canvas or screen has no size", () => {
    assert.equal(mapPointer({ clientX: 1, clientY: 1, rect: { x: 0, y: 0, w: 0, h: 0 }, canvas: { w: 0, h: 0 }, screen }), null);
    assert.equal(mapPointer({ clientX: 1, clientY: 1, rect: { x: 0, y: 0, w: 10, h: 10 }, canvas: { w: 10, h: 10 }, screen: { w: 0, h: 0 } }), null);
  });
});

describe("deviceRectToCanvas", () => {
  it("is the inverse of mapPointer for the rectangle corners", () => {
    const screen = { w: 360, h: 720 };
    const canvas = { w: 800, h: 400 };
    const r = deviceRectToCanvas({ x: 0, y: 0, w: 360, h: 720 }, canvas, screen);
    close(r.x, 300);
    close(r.w, 200);
    close(r.h, 400);
    const half = deviceRectToCanvas({ x: 180, y: 360, w: 36, h: 72 }, canvas, screen);
    close(half.x, 400);
    close(half.y, 200);
    close(half.w, 20);
  });
});

describe("classifyGesture table", () => {
  const cases: Array<[number, number, string]> = [
    // maxMovePx, durationMs, expected
    [0, 0, "tap"],
    [0, 120, "tap"],
    [10, 300, "tap"], // exactly 10 px and 300 ms is still a tap
    [3, 299, "tap"],
    [0, 301, "tap"], // slow tap between 300 and 500 ms
    [0, 499, "tap"],
    [0, 500, "long_press"],
    [9, 2000, "long_press"],
    [10.01, 100, "swipe"],
    [11, 0, "swipe"],
    [250, 400, "swipe"],
    [11, 900, "swipe"], // moving beats holding
  ];
  for (const [move, ms, want] of cases) {
    it(`move ${move}px for ${ms}ms is ${want}`, () => {
      assert.equal(classifyGesture({ maxMovePx: move, durationMs: ms }), want);
    });
  }
});

describe("hitTestNodes", () => {
  const n = (x: number, y: number, w: number, h: number, interactive: boolean, key: string | null = null) => ({
    x, y, w, h, interactive, key,
  });
  it("prefers the smallest interactive node containing the point", () => {
    const root = n(0, 0, 100, 100, false);
    const panel = n(0, 0, 50, 50, false);
    const btn = n(10, 10, 20, 20, true);
    assert.equal(hitTestNodes([root, panel, btn], 15, 15), btn);
    assert.equal(hitTestNodes([root, panel, btn], 40, 40), panel);
    assert.equal(hitTestNodes([root, panel, btn], 200, 200), null);
  });
  it("treats keyed nodes as preferred", () => {
    const big = n(0, 0, 100, 100, false, "k");
    const small = n(10, 10, 5, 5, false);
    assert.equal(hitTestNodes([big, small], 12, 12), big);
  });
});

describe("selectorFor", () => {
  const base = { key: null, text: null, label: null, type: "Container" };
  it("prefers #key", () => {
    assert.equal(selectorFor({ ...base, key: "loginButton", text: "Log in" }), "#loginButton");
  });
  it("falls back to a quoted text, then quoted label, then the type", () => {
    assert.equal(selectorFor({ ...base, text: "Log in" }), '"Log in"');
    assert.equal(selectorFor({ ...base, label: "Close dialog" }), '"Close dialog"');
    assert.equal(selectorFor(base), "Container");
  });
  it("skips keys the lexer cannot read as #id and empty strings", () => {
    assert.equal(selectorFor({ ...base, key: "a-b c", text: "Go" }), '"Go"');
    assert.equal(selectorFor({ ...base, key: "", text: "  ", label: "L" }), '"L"');
    assert.equal(selectorFor({ ...base, key: "[<'x'>]" }), "Container");
  });
  it("escapes quotes, backslashes, newlines and tabs", () => {
    assert.equal(selectorFor({ ...base, text: 'Say "hi"' }), '"Say \\"hi\\""');
    assert.equal(quoteString("a\\b"), '"a\\\\b"');
    assert.equal(quoteString("l1\nl2\tx\r\n"), '"l1\\nl2\\tx\\n"');
  });
  it("keeps unicode keys and trims text", () => {
    assert.equal(selectorFor({ ...base, key: "botão_1" }), "#botão_1");
    assert.equal(selectorFor({ ...base, text: "  Hi  " }), '"Hi"');
  });
});
