// WebSocket client for the device view hub: reconnect with exponential
// backoff, frame/message dispatch, send-side limits. No DOM access, so it runs
// under Node in tests with an injected WebSocket constructor.
//
// Token rule: the token is only used to build the WebSocket URL. It is never
// logged, never placed in a status message or error, and never exposed by
// any getter.

import {
  MAX_CLIENT_MESSAGES_PER_SECOND,
  ProtocolError,
  decodeFrame,
  encodeClientMessage,
  isSafeWsUrl,
  parseServerMessage,
} from "./protocol.ts";
import type {
  ClientMessage,
  Config,
  DecodedFrame,
  DeviceNode,
  ServerMessage,
  Status,
} from "./protocol.ts";
import { backoffDelay, realTimers } from "./timing.ts";
import type { Timers } from "./timing.ts";

/** The subset of the WebSocket API this client uses. */
export interface WebSocketLike {
  binaryType: string;
  readyState: number;
  onopen: ((ev: unknown) => void) | null;
  onmessage: ((ev: { data: unknown }) => void) | null;
  onclose: ((ev: unknown) => void) | null;
  onerror: ((ev: unknown) => void) | null;
  send(data: string): void;
  close(): void;
}

export type WebSocketCtor = new (url: string) => WebSocketLike;

const WS_OPEN = 1;
/** Close code the hub uses when another client with the same token took over. */
export const CLOSE_REPLACED = 4001;
/** A hello this soon after the previous one does not reset the backoff (flapping guard). */
const STABLE_MS = 5000;
/** After this many consecutive failed attempts the state is reported as "error" (still retrying). */
const ERROR_AFTER_ATTEMPTS = 5;

export interface ConnectionHandlers {
  onStatus(s: Status): void;
  onHello(m: Extract<ServerMessage, { t: "hello" }>): void;
  onFrame(f: DecodedFrame): void;
  onTree(nodes: DeviceNode[]): void;
  onHit(id: number, node: DeviceNode | null): void;
  onAck(id: number, ok: boolean, err: string): void;
}

export interface ConnectionOptions {
  url: string;
  token: string;
  handlers: ConnectionHandlers;
  WebSocketImpl?: WebSocketCtor;
  timers?: Timers;
  now?: () => number;
}

export class DeviceConnection {
  private ws: WebSocketLike | null = null;
  private attempt = 0;
  private retryHandle: unknown = null;
  private destroyed = false;
  private paused = false;
  private config: Config = {};
  private sentAt: number[] = [];
  private everConnected = false;
  private taken = false;
  private lastHelloAt = -Infinity;
  private readonly baseUrl: string;
  private readonly token: string;
  private readonly h: ConnectionHandlers;
  private readonly Ctor: WebSocketCtor;
  private readonly timers: Timers;
  private readonly now: () => number;
  /** Number of client messages dropped because of the per-second budget. */
  droppedMessages = 0;
  /** Number of malformed binary frames ignored. */
  malformedFrames = 0;

  constructor(o: ConnectionOptions) {
    this.baseUrl = o.url;
    this.token = o.token;
    this.h = o.handlers;
    this.Ctor = o.WebSocketImpl ?? (globalThis.WebSocket as unknown as WebSocketCtor);
    this.timers = o.timers ?? realTimers;
    this.now = o.now ?? (() => Date.now());
  }

  start(): void {
    if (!isSafeWsUrl(this.baseUrl)) {
      this.h.onStatus({ state: "error", msg: "invalid device view address" });
      return;
    }
    this.open();
  }

  get isOpen(): boolean {
    return this.ws !== null && this.ws.readyState === WS_OPEN;
  }

  private buildUrl(): string {
    const u = new URL(this.baseUrl);
    u.searchParams.set("token", this.token);
    return u.toString();
  }

  private open(): void {
    if (this.destroyed) return;
    this.h.onStatus({
      state: this.everConnected || this.attempt > 0 ? "reconnecting" : "connecting",
      msg: this.attempt > 0 ? "reconnecting" : "connecting",
    });
    let ws: WebSocketLike;
    try {
      ws = new this.Ctor(this.buildUrl());
    } catch {
      this.scheduleRetry();
      return;
    }
    ws.binaryType = "arraybuffer";
    this.ws = ws;
    ws.onopen = () => {
      if (this.ws !== ws) return;
      // Resend session state; the hub starts a fresh session per connection.
      if (Object.keys(this.config).length > 0) this.sendRaw({ t: "config", ...this.config });
      if (this.paused) this.sendRaw({ t: "pause" });
    };
    ws.onmessage = (ev) => {
      if (this.ws !== ws) return;
      this.onMessage(ev.data);
    };
    ws.onerror = () => {
      // Details are intentionally not surfaced (they may include the URL).
    };
    ws.onclose = (ev) => {
      if (this.ws !== ws) return;
      this.ws = null;
      if (this.destroyed) return;
      const code = (ev as { code?: unknown } | null)?.code;
      if (code === CLOSE_REPLACED) {
        // Another window took the stream. Do not fight over it.
        this.taken = true;
        this.h.onStatus({ state: "taken", msg: "taken over by another window" });
        return;
      }
      this.scheduleRetry();
    };
  }

  private onMessage(data: unknown): void {
    if (typeof data === "string") {
      const m = parseServerMessage(data);
      if (!m) return;
      switch (m.t) {
        case "hello":
          {
            const t = this.now();
            if (t - this.lastHelloAt >= STABLE_MS) this.attempt = 0;
            this.lastHelloAt = t;
          }
          this.taken = false;
          this.everConnected = true;
          this.h.onHello(m);
          this.h.onStatus(
            this.paused
              ? { state: "paused", msg: "paused" }
              : { state: "connected", msg: "connected" },
          );
          break;
        case "tree":
          this.h.onTree(m.nodes);
          break;
        case "hit":
          this.h.onHit(m.id, m.node);
          break;
        case "ack":
          this.h.onAck(m.id, m.ok, m.err);
          break;
        case "status":
          this.h.onStatus({ state: normalizeState(m.state), msg: m.msg });
          break;
      }
      return;
    }
    if (data instanceof ArrayBuffer || ArrayBuffer.isView(data)) {
      try {
        const bytes =
          data instanceof ArrayBuffer
            ? new Uint8Array(data)
            : new Uint8Array(data.buffer, data.byteOffset, data.byteLength);
        this.h.onFrame(decodeFrame(bytes));
      } catch (e) {
        if (!(e instanceof ProtocolError)) throw e;
        this.malformedFrames++;
      }
    }
  }

  private scheduleRetry(): void {
    if (this.destroyed || this.retryHandle !== null) return;
    const delay = backoffDelay(this.attempt);
    this.attempt++;
    const failed = this.attempt >= ERROR_AFTER_ATTEMPTS;
    this.h.onStatus({
      state: failed ? "error" : "reconnecting",
      msg: failed ? "device view unreachable, retrying" : "reconnecting",
    });
    this.retryHandle = this.timers.setTimeout(() => {
      this.retryHandle = null;
      this.open();
    }, delay);
  }

  /** Reconnect after the stream was taken by another window (or retry at once). */
  reconnect(): void {
    if (this.destroyed) return;
    this.taken = false;
    this.attempt = 0;
    this.retryNow();
  }

  /** Immediately retry (used when the page becomes visible again). */
  retryNow(): void {
    if (this.destroyed || this.ws !== null || this.taken) return;
    if (this.retryHandle !== null) {
      this.timers.clearTimeout(this.retryHandle);
      this.retryHandle = null;
    }
    this.open();
  }

  private budgetOk(): boolean {
    const t = this.now();
    while (this.sentAt.length > 0 && t - this.sentAt[0]! >= 1000) this.sentAt.shift();
    if (this.sentAt.length >= MAX_CLIENT_MESSAGES_PER_SECOND) return false;
    this.sentAt.push(t);
    return true;
  }

  private sendRaw(msg: ClientMessage): boolean {
    if (!this.isOpen) return false;
    const s = encodeClientMessage(msg);
    if (s === null) return false;
    this.ws!.send(s);
    return true;
  }

  /**
   * Send a client message. Returns false when it was dropped: not connected,
   * invalid or oversized, or over the 50 messages/second budget. Control
   * messages (pause, resume, config) bypass the budget.
   */
  send(msg: ClientMessage): boolean {
    const control = msg.t === "pause" || msg.t === "resume" || msg.t === "config";
    if (!this.isOpen) return false;
    const encoded = encodeClientMessage(msg);
    if (encoded === null) return false;
    if (!control && !this.budgetOk()) {
      this.droppedMessages++;
      return false;
    }
    this.ws!.send(encoded);
    return true;
  }

  setPaused(p: boolean): void {
    if (this.paused === p) return;
    this.paused = p;
    this.sendRaw({ t: p ? "pause" : "resume" });
    if (this.isOpen) {
      this.h.onStatus(p ? { state: "paused", msg: "paused" } : { state: "connected", msg: "connected" });
    }
    if (!p) this.retryNow();
  }

  setConfig(c: Config): void {
    this.config = { ...this.config, ...c };
    this.sendRaw({ t: "config", ...c });
  }

  destroy(): void {
    this.destroyed = true;
    if (this.retryHandle !== null) this.timers.clearTimeout(this.retryHandle);
    this.retryHandle = null;
    const ws = this.ws;
    this.ws = null;
    if (ws) {
      ws.onopen = ws.onmessage = ws.onclose = ws.onerror = null;
      try {
        ws.close();
      } catch {
        // already closed
      }
    }
    this.h.onStatus({ state: "closed", msg: "closed" });
  }
}

function normalizeState(s: string): Status["state"] {
  switch (s) {
    case "connected":
    case "reconnecting":
    case "error":
    case "paused":
    case "taken":
      return s;
    default:
      return "connected";
  }
}
