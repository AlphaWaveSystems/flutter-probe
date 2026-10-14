// Device view protocol v1: message types, the binary frame decoder and the
// client message encoder. Pure module: no DOM, no Wails, no VS Code imports.

export const PROTOCOL_VERSION = 1;
export const FRAME_MAGIC = 0x01;
/** Hub limit for one incoming (client to server) message. */
export const MAX_CLIENT_MESSAGE_BYTES = 4096;
/** Hub limit for client messages per second. */
export const MAX_CLIENT_MESSAGES_PER_SECOND = 50;
/** Coordinates the hub accepts. */
export const MAX_COORD = 20000;
/** Sanity bound for the JSON header of a binary frame. */
export const MAX_FRAME_HEADER_BYTES = 4096;
/** Sanity bound for the image side length declared in a frame header. */
export const MAX_FRAME_DIMENSION = 16384;
/** Sanity bound for an entire frame. */
export const MAX_FRAME_BYTES = 32 * 1024 * 1024;

export type FrameFormat = "jpeg" | "png";

export interface DeviceNode {
  i: number;
  p: number;
  type: string;
  key: string | null;
  text: string | null;
  label: string | null;
  x: number;
  y: number;
  w: number;
  h: number;
  interactive: boolean;
}

export interface DeviceInfo {
  id: string;
  platform: string;
  name: string;
}

export interface ScreenInfo {
  w: number;
  h: number;
  dpr: number;
}

export interface Caps {
  input: boolean;
  hit: boolean;
  bounds: boolean;
  jpeg: boolean;
}

export type StatusState =
  | "connecting"
  | "connected"
  | "reconnecting"
  | "error"
  | "paused"
  | "taken"
  | "closed";

export interface Status {
  state: StatusState;
  msg: string;
}

export interface FrameHeader {
  seq: number;
  ts: number;
  w: number;
  h: number;
  fmt: FrameFormat;
}

export interface DecodedFrame extends FrameHeader {
  /** View into the received buffer; not copied. */
  image: Uint8Array;
}

export type ServerMessage =
  | { t: "hello"; v: number; device: DeviceInfo; screen: ScreenInfo; caps: Caps }
  | { t: "tree"; seq: number; nodes: DeviceNode[] }
  | { t: "hit"; id: number; node: DeviceNode | null }
  | { t: "status"; state: string; msg: string }
  | { t: "ack"; id: number; ok: boolean; err: string };

export interface Config {
  fps?: number;
  quality?: number;
  scale?: number;
  tree?: boolean;
}

export type ClientMessage =
  | { t: "tap"; x: number; y: number; id?: number }
  | { t: "long_press"; x: number; y: number; ms: number; id?: number }
  | { t: "swipe"; x1: number; y1: number; x2: number; y2: number; ms: number; id?: number }
  | { t: "text"; s: string; id?: number }
  | { t: "hit"; id: number; x: number; y: number }
  | { t: "pause" }
  | ({ t: "config" } & Config)
  | { t: "resume" };

export class ProtocolError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "ProtocolError";
  }
}

function isRecord(v: unknown): v is Record<string, unknown> {
  return typeof v === "object" && v !== null && !Array.isArray(v);
}

function isPosInt(v: unknown, max: number): v is number {
  return typeof v === "number" && Number.isInteger(v) && v > 0 && v <= max;
}

const textDecoder = new TextDecoder("utf-8", { fatal: true });
const textEncoder = new TextEncoder();

/**
 * Decode a binary frame: byte 0 = 0x01, bytes 1..4 = big-endian uint32 header
 * length N, N bytes of UTF-8 JSON header, rest is the image. Throws
 * ProtocolError for any malformed or truncated input.
 */
export function decodeFrame(input: ArrayBuffer | Uint8Array): DecodedFrame {
  const bytes = input instanceof Uint8Array ? input : new Uint8Array(input);
  if (bytes.byteLength < 5) throw new ProtocolError("frame too short");
  if (bytes.byteLength > MAX_FRAME_BYTES) throw new ProtocolError("frame too large");
  if (bytes[0] !== FRAME_MAGIC) throw new ProtocolError("bad frame type byte");
  const n = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength).getUint32(1, false);
  if (n === 0 || n > MAX_FRAME_HEADER_BYTES) throw new ProtocolError("bad frame header length");
  if (5 + n >= bytes.byteLength) throw new ProtocolError("truncated frame");
  let header: unknown;
  try {
    header = JSON.parse(textDecoder.decode(bytes.subarray(5, 5 + n)));
  } catch {
    throw new ProtocolError("frame header is not valid JSON");
  }
  if (!isRecord(header)) throw new ProtocolError("frame header is not an object");
  const { seq, ts, w, h, fmt } = header;
  if (typeof seq !== "number" || !Number.isInteger(seq) || seq < 0) {
    throw new ProtocolError("bad seq");
  }
  if (typeof ts !== "number" || !Number.isFinite(ts)) throw new ProtocolError("bad ts");
  if (!isPosInt(w, MAX_FRAME_DIMENSION) || !isPosInt(h, MAX_FRAME_DIMENSION)) {
    throw new ProtocolError("bad frame size");
  }
  if (fmt !== "jpeg" && fmt !== "png") throw new ProtocolError("bad frame format");
  return { seq, ts, w, h, fmt, image: bytes.subarray(5 + n) };
}

function num(v: unknown, fallback: number): number {
  return typeof v === "number" && Number.isFinite(v) ? v : fallback;
}

/** Coerce an untrusted value into a DeviceNode, or null when unusable. */
export function parseNode(v: unknown): DeviceNode | null {
  if (!isRecord(v)) return null;
  const x = num(v.x, NaN);
  const y = num(v.y, NaN);
  const w = num(v.w, NaN);
  const h = num(v.h, NaN);
  if (Number.isNaN(x) || Number.isNaN(y) || Number.isNaN(w) || Number.isNaN(h)) return null;
  const str = (s: unknown): string | null => (typeof s === "string" ? s : null);
  return {
    i: num(v.i, -1),
    p: num(v.p, -1),
    type: typeof v.type === "string" ? v.type : "",
    key: str(v.key),
    text: str(v.text),
    label: str(v.label),
    x,
    y,
    w,
    h,
    interactive: v.interactive === true,
  };
}

/**
 * Parse a text message from the hub. Returns null for unknown or malformed
 * messages (they are ignored by the caller).
 */
export function parseServerMessage(raw: string): ServerMessage | null {
  let v: unknown;
  try {
    v = JSON.parse(raw);
  } catch {
    return null;
  }
  if (!isRecord(v) || typeof v.t !== "string") return null;
  switch (v.t) {
    case "hello": {
      const device = isRecord(v.device) ? v.device : {};
      const screen = isRecord(v.screen) ? v.screen : {};
      const caps = isRecord(v.caps) ? v.caps : {};
      const sw = num(screen.w, 0);
      const sh = num(screen.h, 0);
      if (sw <= 0 || sh <= 0) return null;
      return {
        t: "hello",
        v: num(v.v, 0),
        device: {
          id: typeof device.id === "string" ? device.id : "",
          platform: typeof device.platform === "string" ? device.platform : "",
          name: typeof device.name === "string" ? device.name : "",
        },
        screen: { w: sw, h: sh, dpr: Math.max(num(screen.dpr, 1), 0.1) },
        caps: {
          input: caps.input === true,
          hit: caps.hit === true,
          bounds: caps.bounds === true,
          jpeg: caps.jpeg === true,
        },
      };
    }
    case "tree": {
      if (!Array.isArray(v.nodes)) return null;
      const nodes: DeviceNode[] = [];
      for (const n of v.nodes) {
        const node = parseNode(n);
        if (node) nodes.push(node);
      }
      return { t: "tree", seq: num(v.seq, 0), nodes };
    }
    case "hit":
      return { t: "hit", id: num(v.id, -1), node: parseNode(v.node) };
    case "status":
      return {
        t: "status",
        state: typeof v.state === "string" ? v.state : "",
        msg: typeof v.msg === "string" ? v.msg : "",
      };
    case "ack":
      return {
        t: "ack",
        id: num(v.id, -1),
        ok: v.ok === true,
        err: typeof v.err === "string" ? v.err : "",
      };
    default:
      return null;
  }
}

function coordOk(v: number): boolean {
  return Number.isFinite(v) && v >= 0 && v <= MAX_COORD;
}

/**
 * Serialise a client message. Returns null when the message would violate the
 * hub limits (size over 4 KiB, coordinates not finite or outside [0, 20000]),
 * so the caller can drop it instead of being disconnected.
 */
export function encodeClientMessage(msg: ClientMessage): string | null {
  switch (msg.t) {
    case "tap":
    case "hit":
      if (!coordOk(msg.x) || !coordOk(msg.y)) return null;
      break;
    case "long_press":
      if (!coordOk(msg.x) || !coordOk(msg.y) || !Number.isFinite(msg.ms)) return null;
      break;
    case "swipe":
      if (![msg.x1, msg.y1, msg.x2, msg.y2].every(coordOk) || !Number.isFinite(msg.ms)) {
        return null;
      }
      break;
    default:
      break;
  }
  const s = JSON.stringify(msg);
  if (textEncoder.encode(s).byteLength > MAX_CLIENT_MESSAGE_BYTES) return null;
  return s;
}

/**
 * Split text into chunks whose `text` message fits the hub size (4 KiB) and 1000 character limits. Splits
 * on code point boundaries so surrogate pairs stay intact.
 */
export function chunkText(s: string, maxBytes = 1024, maxChars = 1000): string[] {
  const out: string[] = [];
  let cur = "";
  let curBytes = 0;
  let curChars = 0;
  for (const ch of s) {
    const b = textEncoder.encode(ch).byteLength;
    if ((curBytes + b > maxBytes || curChars + 1 > maxChars) && cur !== "") {
      out.push(cur);
      cur = "";
      curBytes = 0;
      curChars = 0;
    }
    cur += ch;
    curBytes += b;
    curChars++;
  }
  if (cur !== "") out.push(cur);
  return out;
}

/**
 * True when `url` is a ws: URL for this machine only: host 127.0.0.1 or
 * localhost, an explicit numeric port, no credentials. wss: is rejected.
 */
export function isSafeWsUrl(url: string): boolean {
  try {
    const u = new URL(url);
    return (
      u.protocol === "ws:" &&
      u.username === "" &&
      u.password === "" &&
      (u.hostname === "127.0.0.1" || u.hostname === "localhost") &&
      /^[0-9]+$/.test(u.port) &&
      Number(u.port) >= 1 &&
      Number(u.port) <= 65535
    );
  } catch {
    return false;
  }
}
