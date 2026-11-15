import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { MAX_SEND_HEADERS, allowText, errorText, parseHeaders, summarize, type TargetInfo } from "../src/send.ts";

const target = (extra: Partial<TargetInfo> = {}): TargetInfo => ({ host: "api.example.test", port: "443", key: "api.example.test:443", allowed: false, addresses: ["93.184.216.34"], classes: ["the internet"], local: false, ...extra });

describe("parseHeaders", () => {
  it("reads Name: value lines, trims them and skips blank lines", () => {
    assert.deepEqual(parseHeaders("Accept: application/json\n\n  X-A :  b c  \r\nContent-Type:text/plain"), {
      headers: [{ name: "Accept", value: "application/json" }, { name: "X-A", value: "b c" }, { name: "Content-Type", value: "text/plain" }],
      error: "",
    });
  });
  it("keeps colons in a value", () => {
    assert.deepEqual(parseHeaders("Referer: https://h.example.test:8443/x").headers, [{ name: "Referer", value: "https://h.example.test:8443/x" }]);
  });
  it("says which line is not a header", () => {
    assert.equal(parseHeaders("A: b\nnot a header\nC: d").error, 'Line 2 is not "Name: value".');
    assert.equal(parseHeaders(": nameless").error, 'Line 1 is not "Name: value".');
    assert.deepEqual(parseHeaders("A: b\nbad").headers, []);
  });
  it("allows an empty value and refuses too many headers", () => {
    assert.deepEqual(parseHeaders("X-Empty:").headers, [{ name: "X-Empty", value: "" }]);
    const many = Array.from({ length: MAX_SEND_HEADERS + 1 }, (_, i) => `X-${i}: v`).join("\n");
    assert.match(parseHeaders(many).error, /At most 50/);
    assert.equal(parseHeaders(many.split("\n").slice(0, MAX_SEND_HEADERS).join("\n")).error, "");
  });
  it("is empty for no text", () => {
    assert.deepEqual(parseHeaders(""), { headers: [], error: "" });
  });
});

describe("allowText", () => {
  it("names the host, the port and where the name leads", () => {
    const t = allowText(target());
    assert.equal(t.title, "Send requests to api.example.test:443?");
    assert.match(t.lines[0], /93\.184\.216\.34 \(the internet\)/);
    assert.equal(t.warning, "");
  });
  it("shows an IPv6 address with its brackets, and the unicode name as a label", () => {
    const v6 = allowText({ ...target(), host: "::1", key: "[::1]:8080", port: "8080" });
    assert.equal(v6.title, "Send requests to [::1]:8080?");
    const idn = allowText({ ...target(), host: "xn--bcher-kva.test", key: "xn--bcher-kva.test:443", label: "b\u00fccher.test" });
    assert.equal(idn.title, "Send requests to xn--bcher-kva.test:443?");
    assert.match(idn.lines[0], /Written as b\u00fccher\.test; Studio uses xn--bcher-kva\.test/);
  });
  it("gives the stronger warning for an address on this machine or its network", () => {
    const t = allowText(target({ local: true, addresses: ["127.0.0.1", "10.0.0.5"], classes: ["this machine", "a private network"] }));
    assert.match(t.warning, /this machine or your own network/);
    assert.match(t.lines[0], /127\.0\.0\.1 \(this machine\), 10\.0\.0\.5 \(a private network\)/);
  });
  it("says when the name could not be looked up", () => {
    assert.match(allowText(target({ addresses: [], classes: [] })).lines[0], /could not be looked up/);
  });
});

describe("summarize", () => {
  it("shows the status, the time and where it came from", () => {
    assert.equal(summarize({ status: 200, durationMs: 12, remoteClass: "this machine", hops: [] }).line, "200 · 12 ms · from this machine");
    assert.equal(summarize({ status: 404, durationMs: 3, remoteClass: "", hops: [] }).line, "404 · 3 ms");
  });
  it("lists the redirects that were followed", () => {
    assert.deepEqual(summarize({ status: 200, durationMs: 1, remoteClass: "", hops: [{ from: "a", to: "https://h/b", status: 302 }, { from: "b", to: "https://h/c", status: 301 }] }).hops, ["302 → https://h/b", "301 → https://h/c"]);
  });
  it("says a redirect to another origin was not followed, and that Studio will ask", () => {
    const s = summarize({ status: 302, durationMs: 1, remoteClass: "", hops: [], stopped: { from: "a", to: "https://other.test/x", status: 302 }, stoppedHost: "other.test" });
    assert.match(s.stopped, /to other\.test was not followed/);
    assert.match(s.stopped, /ask before it calls that host/);
    assert.equal(summarize({ status: 200, durationMs: 1, remoteClass: "", hops: [] }).stopped, "");
  });
  it("says when the body was cut", () => {
    assert.match(summarize({ status: 200, durationMs: 1, remoteClass: "", hops: [], truncated: true }).line, /longer than 1 MB/);
  });
});

describe("errorText", () => {
  it("drops the Error prefix and keeps the sentence", () => {
    assert.equal(errorText(new Error("only http and https can be called")), "only http and https can be called");
    assert.equal(errorText("plain"), "plain");
  });
});
