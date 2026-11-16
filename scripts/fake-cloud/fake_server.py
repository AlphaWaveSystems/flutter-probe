#!/usr/bin/env python3
"""A fake Probe Cloud on this machine, for testing `probe test --upload-report` without GitHub Actions.

It listens on 127.0.0.1 only and plays two roles:
  POST /v1/reports  the hosted service: it records the request and answers with --status (default 202)
  GET  /oidc        a fake GitHub identity-token issuer: it records the request and returns a placeholder token

It records what arrived as one JSON line per request in --log, and keeps the last upload's body in <log>.body.json.
It never writes the license key or the identity token: only their scheme and length. It does not verify anything,
so a token it hands out is a placeholder that a real server would reject.

  python3 scripts/fake-cloud/fake_server.py --log /tmp/fake-cloud.log
"""
import argparse
import http.server
import json
import urllib.parse

args = None


def log(entry):
    with open(args.log, "a") as f:
        f.write(json.dumps(entry) + "\n")


class Handler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        u = urllib.parse.urlparse(self.path)
        if u.path != "/oidc":
            self.send_response(404)
            self.end_headers()
            return
        auth = self.headers.get("Authorization") or ""
        log({"event": "oidc_request", "audience": urllib.parse.parse_qs(u.query).get("audience"),
             "auth_scheme": auth.split(" ")[0], "auth_len": len(auth)})
        body = json.dumps({"value": "placeholder.identity.token"}).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_POST(self):
        n = int(self.headers.get("Content-Length") or 0)
        body = self.rfile.read(n) if n else b""
        auth = self.headers.get("Authorization") or ""
        attest = self.headers.get("X-Probe-Attestation") or ""
        log({"event": "upload", "path": self.path, "content_type": self.headers.get("Content-Type"),
             "auth_scheme": auth.split(" ")[0], "auth_len": len(auth),
             "attestation_present": bool(attest), "attestation_len": len(attest), "body_bytes": len(body),
             "answered": args.status})
        with open(args.log + ".body.json", "wb") as f:
            f.write(body)
        self.send_response(args.status)
        self.send_header("Content-Length", "0")
        self.end_headers()

    def log_message(self, *a):  # keep the terminal quiet
        pass


def main():
    global args
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--port", type=int, default=48899)
    p.add_argument("--log", default="fake-cloud.log", help="where to record the requests (one JSON line each)")
    p.add_argument("--status", type=int, default=202, help="the status an upload is answered with (try 429, 503, 401)")
    args = p.parse_args()
    print(f"fake Probe Cloud on http://127.0.0.1:{args.port} (log: {args.log}); stop it with Ctrl-C", flush=True)
    http.server.HTTPServer(("127.0.0.1", args.port), Handler).serve_forever()


if __name__ == "__main__":
    main()
