// Mock device view hub for the demo page and the integration tests.
//
// Implements the server side of device view protocol v1 with Node built-ins
// only (a minimal RFC 6455 text/binary implementation): token check, hello,
// binary frames of a generated PNG (or a PNG/JPEG file passed with --image),
// tree and hit replies, acks for input messages, pause/resume/config.
//
// Usage: node device-view-demo/mock-server.mjs [--port 8765] [--token demo] [--fps 5] [--image file]
// It binds 127.0.0.1 only.

import http from "node:http";
import crypto from "node:crypto";
import zlib from "node:zlib";
import fs from "node:fs";
import { fileURLToPath } from "node:url";

export const SCREEN = { w: 360, h: 780, dpr: 1 };

export const NODES = [
  { i: 0, p: -1, type: "Scaffold", key: null, text: null, label: null, x: 0, y: 0, w: 360, h: 780, interactive: false },
  { i: 1, p: 0, type: "TextField", key: "emailField", text: null, label: "Email", x: 30, y: 200, w: 300, h: 56, interactive: true },
  { i: 2, p: 0, type: "ElevatedButton", key: "loginButton", text: "Log in", label: null, x: 30, y: 300, w: 300, h: 56, interactive: true },
  { i: 3, p: 0, type: "TextButton", key: null, text: 'Say "hi"', label: null, x: 30, y: 380, w: 300, h: 48, interactive: true },
];

const CRC_TABLE = (() => {
  const t = new Uint32Array(256);
  for (let n = 0; n < 256; n++) {
    let c = n;
    for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
    t[n] = c >>> 0;
  }
  return t;
})();

function crc32(buf) {
  let c = 0xffffffff;
  for (const b of buf) c = CRC_TABLE[(c ^ b) & 0xff] ^ (c >>> 8);
  return (c ^ 0xffffffff) >>> 0;
}

function chunk(type, data) {
  const len = Buffer.alloc(4);
  len.writeUInt32BE(data.length);
  const td = Buffer.concat([Buffer.from(type, "ascii"), data]);
  const crc = Buffer.alloc(4);
  crc.writeUInt32BE(crc32(td));
  return Buffer.concat([len, td, crc]);
}

/** Generate a small PNG of a fake login screen at the logical screen size. */
export function generateTestPng() {
  const { w, h } = SCREEN;
  const raw = Buffer.alloc((w * 3 + 1) * h);
  const fillRect = (x0, y0, rw, rh, rgb) => {
    for (let y = y0; y < y0 + rh && y < h; y++) {
      for (let x = x0; x < x0 + rw && x < w; x++) {
        const o = y * (w * 3 + 1) + 1 + x * 3;
        raw[o] = rgb[0];
        raw[o + 1] = rgb[1];
        raw[o + 2] = rgb[2];
      }
    }
  };
  for (let y = 0; y < h; y++) {
    const shade = 235 - Math.floor((y / h) * 30);
    fillRect(0, y, w, 1, [shade, shade, 245]);
  }
  fillRect(0, 0, w, 90, [63, 81, 181]);
  for (const n of NODES.slice(1)) {
    const color = n.type === "ElevatedButton" ? [63, 81, 181] : n.type === "TextField" ? [255, 255, 255] : [200, 200, 220];
    fillRect(n.x, n.y, n.w, n.h, color);
  }
  const ihdr = Buffer.alloc(13);
  ihdr.writeUInt32BE(w, 0);
  ihdr.writeUInt32BE(h, 4);
  ihdr[8] = 8; // bit depth
  ihdr[9] = 2; // RGB
  return Buffer.concat([
    Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
    chunk("IHDR", ihdr),
    chunk("IDAT", zlib.deflateSync(raw)),
    chunk("IEND", Buffer.alloc(0)),
  ]);
}

function wsFrame(opcode, payload) {
  const len = payload.length;
  let head;
  if (len < 126) {
    head = Buffer.from([0x80 | opcode, len]);
  } else if (len < 65536) {
    head = Buffer.alloc(4);
    head[0] = 0x80 | opcode;
    head[1] = 126;
    head.writeUInt16BE(len, 2);
  } else {
    head = Buffer.alloc(10);
    head[0] = 0x80 | opcode;
    head[1] = 127;
    head.writeBigUInt64BE(BigInt(len), 2);
  }
  return Buffer.concat([head, payload]);
}

/** Build a binary frame per protocol section B. Exported for tests. */
export function buildFrame(seq, w, h, fmt, image) {
  const header = Buffer.from(JSON.stringify({ seq, ts: Date.now(), w, h, fmt }), "utf8");
  const n = Buffer.alloc(5);
  n[0] = 0x01;
  n.writeUInt32BE(header.length, 1);
  return Buffer.concat([n, header, image]);
}

function detectFormat(buf) {
  return buf[0] === 0x89 && buf[1] === 0x50 ? "png" : "jpeg";
}

/**
 * Start the mock hub.
 * @param {{port?: number, token?: string, fps?: number, image?: Buffer, requireOrigin?: boolean}} o
 */
export async function startMockServer(o = {}) {
  const token = o.token ?? "demo";
  const image = o.image ?? generateTestPng();
  const fmt = detectFormat(image);
  let fps = o.fps ?? 5;
  const clients = new Set();
  const received = [];
  let seq = 0;

  const server = http.createServer((_req, res) => {
    res.writeHead(404).end();
  });

  const allSockets = new Set();
  server.on("connection", (s) => {
    allSockets.add(s);
    s.on("close", () => allSockets.delete(s));
  });

  server.on("upgrade", (req, socket) => {
    const url = new URL(req.url ?? "/", "http://127.0.0.1");
    const given = Buffer.from(url.searchParams.get("token") ?? "");
    const want = Buffer.from(token);
    const ok = given.length === want.length && crypto.timingSafeEqual(given, want);
    if (url.pathname !== "/device/v1" || !ok) {
      socket.end("HTTP/1.1 401 Unauthorized\r\nConnection: close\r\nContent-Length: 0\r\n\r\n");
      return;
    }
    const key = req.headers["sec-websocket-key"];
    if (typeof key !== "string") {
      socket.end("HTTP/1.1 400 Bad Request\r\n\r\n");
      return;
    }
    const accept = crypto
      .createHash("sha1")
      .update(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11")
      .digest("base64");
    socket.write(
      "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n" +
        `Sec-WebSocket-Accept: ${accept}\r\n\r\n`,
    );
    const client = { socket, paused: false, tree: false, buf: Buffer.alloc(0), times: [] };
    clients.add(client);
    sendText(client, {
      t: "hello",
      v: 1,
      device: { id: "mock-1", platform: "mock", name: "Mock device" },
      screen: SCREEN,
      caps: { input: true, hit: true, bounds: true, jpeg: true },
    });
    socket.on("data", (d) => {
      client.buf = Buffer.concat([client.buf, d]);
      try {
        parseFrames(client);
      } catch {
        socket.destroy();
      }
    });
    socket.on("close", () => clients.delete(client));
    socket.on("error", () => clients.delete(client));
  });

  function sendText(c, obj) {
    if (!c.socket.destroyed) c.socket.write(wsFrame(0x1, Buffer.from(JSON.stringify(obj), "utf8")));
  }

  function parseFrames(c) {
    for (;;) {
      const b = c.buf;
      if (b.length < 2) return;
      const opcode = b[0] & 0x0f;
      const masked = (b[1] & 0x80) !== 0;
      let len = b[1] & 0x7f;
      let off = 2;
      if (len === 126) {
        if (b.length < 4) return;
        len = b.readUInt16BE(2);
        off = 4;
      } else if (len === 127) {
        if (b.length < 10) return;
        len = Number(b.readBigUInt64BE(2));
        off = 10;
      }
      if (!masked) throw new Error("unmasked client frame");
      if (len > 4096) {
        c.socket.end(wsFrame(0x8, Buffer.from([0x03, 0xf1])));
        return;
      }
      if (b.length < off + 4 + len) return;
      const mask = b.subarray(off, off + 4);
      const payload = Buffer.from(b.subarray(off + 4, off + 4 + len));
      for (let i = 0; i < payload.length; i++) payload[i] ^= mask[i % 4];
      c.buf = b.subarray(off + 4 + len);
      if (opcode === 0x8) {
        c.socket.end(wsFrame(0x8, Buffer.alloc(0)));
        return;
      }
      if (opcode === 0x9) {
        c.socket.write(wsFrame(0xa, payload));
        continue;
      }
      if (opcode === 0x1) onClientMessage(c, payload.toString("utf8"));
    }
  }

  function onClientMessage(c, text) {
    const now = Date.now();
    c.times = c.times.filter((t) => now - t < 1000);
    c.times.push(now);
    if (c.times.length > 50) {
      sendText(c, { t: "status", state: "error", msg: "rate limit: message dropped" });
      return;
    }
    let m;
    try {
      m = JSON.parse(text);
    } catch {
      return;
    }
    received.push(m);
    switch (m.t) {
      case "pause":
        c.paused = true;
        break;
      case "resume":
        c.paused = false;
        break;
      case "config":
        if (typeof m.fps === "number" && m.fps >= 1 && m.fps <= 30) fps = m.fps;
        if (typeof m.tree === "boolean") {
          c.tree = m.tree;
          if (m.tree) sendText(c, { t: "tree", seq, nodes: NODES });
        }
        break;
      case "hit": {
        let best = null;
        for (const n of NODES.slice(1)) {
          if (m.x >= n.x && m.x <= n.x + n.w && m.y >= n.y && m.y <= n.y + n.h) best = n;
        }
        sendText(c, { t: "hit", id: m.id, node: best });
        break;
      }
      case "tap":
      case "long_press":
      case "swipe":
      case "text":
        if (m.id !== undefined) sendText(c, { t: "ack", id: m.id, ok: true, err: "" });
        break;
      default:
        break;
    }
  }

  let frameTimer = null;
  const pump = () => {
    for (const c of clients) {
      if (c.paused || c.socket.destroyed) continue;
      c.socket.write(wsFrame(0x2, buildFrame(++seq, SCREEN.w, SCREEN.h, fmt, image)));
    }
    frameTimer = setTimeout(pump, Math.round(1000 / fps));
  };
  pump();

  await new Promise((resolve) => server.listen(o.port ?? 0, "127.0.0.1", resolve));
  const port = server.address().port;

  return {
    port,
    token,
    url: `ws://127.0.0.1:${port}/device/v1`,
    received,
    clientCount: () => clients.size,
    /** Close all client sockets without stopping the server (to test reconnect). */
    dropClients() {
      for (const c of clients) c.socket.destroy();
    },
    async close() {
      clearTimeout(frameTimer);
      for (const c of clients) c.socket.destroy();
      for (const s of allSockets) s.destroy();
      await new Promise((resolve) => server.close(resolve));
    },
  };
}

function arg(name, def) {
  const i = process.argv.indexOf(`--${name}`);
  return i > 0 ? process.argv[i + 1] : def;
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const imgPath = arg("image", "");
  const s = await startMockServer({
    port: Number(arg("port", "8765")),
    token: arg("token", "demo"),
    fps: Number(arg("fps", "5")),
    image: imgPath ? fs.readFileSync(imgPath) : undefined,
  });
  console.log(JSON.stringify({ url: s.url, token: s.token, device: "mock-1" }));
}
