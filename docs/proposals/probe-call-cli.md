# Design note: a `probe call` command over `internal/httpcall`

Status: proposal, no code. Written so the open questions can be decided before anything is built.

## Why
Studio can send one request to a backend (the Send request panel) with a hardened call (`internal/httpcall`), a host allow made in Go, and a redactor for what comes back. An editor extension (VS Code today) has no such path. It is TypeScript, so it cannot import a Go package. The smallest way to give it one is a `probe` subcommand that wraps the same call, which the extension runs the way it already runs `probe test` and `probe lint`. The same command is useful from a terminal and from CI to check a backend by hand.

## What it would be
```
probe call GET https://api.example.test/health
probe call POST https://api.example.test/items --body-stdin --json
```
- One request per process. Method GET, POST, PUT, PATCH, DELETE, HEAD or OPTIONS (the call has no grammar limit here; `call` steps in ProbeScript stay GET/POST/PUT/DELETE).
- Body, headers and the address go in over **stdin as one JSON document**, never in argv: argv is visible to every process on the machine, and a typed token must not be.
- Output: the status, the time, the redirects followed and the one that was not, the response headers and a text preview of the body, **redacted** (see below). `--json` gives the same as JSON for the extension. Exit code 0 when a response arrived (any status), 1 when the call was refused or failed (a sentence, with no address, header or body in it), 2 for a usage error.
- Nothing is written to disk, nothing is logged, nothing goes to an AI provider, no cookies are kept, no proxy from the environment is used.

## The allow model (decision needed)
Studio asks a person, once per session, per host and port, and shows where the name leads. A command has no dialog, so the decision moves to configuration. Proposal:
1. **Every host is explicit.** A request to a host and port that is not on the allow list is refused with a sentence that names the flag to use. There is no "allow all".
2. **Where the list comes from:** `call.allow:` in `probe.yaml` (host:port entries, the project's own backends) and `--allow host:port` (repeatable) or `PROBE_CALL_ALLOW` for one run. The flag and the environment variable add to the file, never replace what is refused below.
3. **Kinds of address, as in Studio:** cloud metadata, unspecified and multicast addresses are never called. This machine, a private network, link-local and reserved ranges are called only for a host that is on the list **by name or address**, and only when the list entry is written for them (`localhost:3000`, `10.0.0.5:8080`): an entry for a public name never covers an address on this machine or a private network, so a name that resolves somewhere else after the allow (DNS rebinding) is refused at dial time. This is the same rule as Studio's kind-bound allow.
4. **Limits:** only http and https, no user name or password in the address, a method and header allow-list, 30 seconds, 1 MB of response, 256 KB of request, 5 redirects within one origin (another origin is reported, not followed), one request at a time.

Open: should the file list live in `probe.yaml` (shared with the team, reviewed in a PR) or be flags/environment only (nothing to review, nothing shared)? A list in a repository is a decision a reviewer sees; a flag in a script is not.

## The redactor (decision needed)
Studio's answer redaction is in the Studio module (`studio/net_redact.go`, layered on `internal/record`, plus `studio/send_echo.go`, which hides the values that were typed wherever the answer echoes them). A command in the root module cannot import it. Options:
- **A. Extract** the redactors into a root-module package (for example `internal/netredact`) that Studio and the command both import. A pure move with its tests, then both users. The Studio audit that checks the monitor files would follow the package.
- **B. Reuse only `internal/record`** (the recorder's redactor). Smaller, but weaker than what Studio shows for the same answer: the recorder's floor is a subset, and the command would hide less than Studio.
- **C. Build the command into the Studio module.** The command would ship with Studio, not with `probe`; the extension would need Studio installed.

Recommendation: A, as its own change before the command, with the existing tests moved unchanged. Without it the command would show answers less carefully than Studio does, and the two would drift.

## The extension
A command ("Send request") that collects method, address, headers and body in an input box or a `.http`-style document, runs `probe call --json` with the document on stdin, and shows the redacted answer as text in an output channel. It stores nothing (no settings, no history), shows nothing as markup, and passes nothing through argv. "Save as step" from a request already exists as the `call` grammar; the extension could offer it with the same placeholder rules Studio uses (`${PROBE_SAVED_*}`), but that is a separate step.

## Risks
- **A second path that sends requests from the user's machine on an IDE click**, without Studio's dialog. It needs the same review level as the Send panel: an SSRF matrix against a stub dialer, rebinding, redirects, hostile answers (markup, huge, binary, secrets split by hidden characters), the typed-value echo, and mutants.
- **CI:** a command that reads an allow list from the repository and a body from stdin can be driven by a script with secrets in its environment. The answer is redacted and typed values are hidden, but a secret the redactor does not know is shown, as in Studio (its documented limit). The command should say so in its help.
- **argv, logs and shell history:** only the address reaches argv if the allow flag is used; bodies and headers never do.
- **Scope creep:** an AI path, a history, cookies, a proxy, a "reveal" of a redacted value and saving to disk are all out.

## Decisions needed from the owner
1. Is `probe call` a public CLI feature at all, or only an extension helper (hidden, no documentation)?
2. The allow list: `probe.yaml` plus flags, or flags and environment only?
3. The redactor: extract it (A), reuse the recorder's (B), or build into Studio (C)?
4. May a loopback or private address be allowed by name in a repository file, or only by a flag on the command line?
5. May Studio's automation endpoint (the loopback control endpoint scripts and MCP use) ever carry a network method? Today it does not, and a test fails if one is added (`studio/bound_methods_test.go`: no method in the network, AI provider, keychain or workspace-writing categories is reachable from it). The command in this note makes the question less pressing, because an extension or a script would call `probe call` directly, but the owner should say whether the answer is "never" or "only with a decision recorded in that test".

## Order of work if it is approved
1. Move the redactors into a root-module package, tests unchanged (no behaviour change).
2. `probe call` with the allow model, the limits and the redacted output, with its own test matrix and mutants.
3. The extension command, with its own review.
4. Docs for each: the CLI page, the extension page, and the MCP tool list only if a tool is added (it is not planned).
