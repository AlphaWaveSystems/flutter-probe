# Scope: how an editor extension talks to FlutterProbe

Status: proposal, no code. It reads what exists today so that the next step is chosen with the risks in view.

## Where things stand
Five local surfaces already exist:

| Surface | Protocol | Authentication | What a stray local process can reach | What a web page can reach |
|---|---|---|---|---|
| `probe` CLI | a process: arguments, stdout, exit code | none (the caller owns the process) | whatever the user can do | nothing |
| `probe-mcp` over stdio | JSON-RPC on the child's stdin/stdout | none (the parent owns the pipe) | the parent only | nothing |
| `probe-mcp --http` | HTTP `POST /mcp` on loopback | a random 256-bit bearer in a 0600 file in a 0700 directory, or one from the environment | every tool, for a process of the same user that reads the file | nothing: `Origin`, `Sec-Fetch-Site` and `Host` are refused and no CORS header is sent |
| Studio automation endpoint (`PROBE_STUDIO_AUTOMATION=1`) | HTTP+JSON on loopback, 16 methods | a random bearer in a 0600 discovery file | device control, screenshots, widget tree, reading and running `.probe` files inside the allowed folders | nothing: any `Origin` and cross-site fetches are refused |
| Studio headless bridge (`PROBE_STUDIO_HEADLESS=1`, tests only) | HTTP+JSON and server-sent events on loopback | a token that is embedded in a page the server serves | every `App` method except four window-only ones | nothing, by `Host`, `Origin` and `Sec-Fetch` checks |
| device view hub (`probe device view`) | WebSocket on loopback | a per-run 256-bit token printed on the first stdout line | the screen and taps of one device, with the token | only with the token and a loopback or `vscode-webview://` origin |

The VS Code extension uses the CLI (spawned for `device list` and `lint`; the terminal for `test` and `studio`) and the device view hub (a child process, token read from its first output line and given to the webview only, with a CSP that limits the webview's connections to the hub). It does not use MCP, the automation endpoint or the bridge.

Today the decision table for the methods Studio binds (`studio/bound_methods_test.go`) is a map from each method to a category, plus three lists: the automation endpoint's method names, the methods of `App` it can reach, and the methods the headless bridge refuses. A pending change turns it into a data file with one row per method that also records who may call it (window, bridge, automation), what it touches and whether it asks. The `callers`, `touches` and `confirm` fields below are those pending fields, and this note assumes that form (it is called the decision table below). Categories that may never reach the automation endpoint: confirmations, writing the workspace, secrets, the network, AI providers, choosing the workspace, writing outside it, and the person's preferences.

## What the transport must be, and what it must not be
- **A way for an editor to ask for things the person could do by hand**: lint a text, list devices, run a file or a test on the connected device, read the results, open a screen view. Nothing the editor can ask is something the command line cannot already do.
- **Not a second route around consent.** The editor never answers a confirmation, never passes `--yes` (the MCP server already refuses `-y`/`--yes` in every argument), never sets the person's preference for answering clear-data questions, and never reaches a method in a category the decision table keeps from automation.
- **Not reachable from a web page.** Whatever the protocol, a page in a browser must get nothing, with or without a token it cannot know.

## Threat model
| Actor | Can | The transport's answer |
|---|---|---|
| a web page in the user's browser | send requests to `127.0.0.1` (DNS rebinding, `fetch`, forms, WebSocket) | refuse any `Origin` and cross-site fetch, check `Host`, JSON-only content type, no CORS, a token the page cannot learn |
| another user on the machine | read files with loose modes, connect to a port | files 0600 in a 0700 folder, checked on write and refuse a link or a folder open to others, loopback only |
| another process of the same user | read the token file, connect | **not defended**: the same user can do anything the user can; the rule is to expose nothing that user could not do, to keep tokens out of argv, environment and logs, and to say so in the docs |
| a hostile workspace (a repository the person opened) | contain settings, files, a `bin/probe` | the extension must not run a binary or use a secret that a workspace file names without the person's say-so (see the first fixes) |
| another extension or webview | talk to the extension's webview, learn a token it is given | the token goes to its own webview only; a CSP that limits connections; the hub accepts a `vscode-webview://` origin of any id, so the token is the only barrier there, and it must stay out of logs |
| a stale or crashed server | leave a token file behind | write by rename, remove only if it still holds this server's token, check `pid` on read |

## What the code reading found
Reading the code for this note found the six things below. None is a vulnerability a web page can use; they made the same-user and workspace cases worse than they needed to be. Findings 1 to 3 are fixed and merged; 4 to 6 are open.
1. **Fixed: the VS Code extension put cloud secrets and a relay token in the arguments of a command sent to the terminal** (`--cloud-secret`, `--cloud-key`, `--relay-token`, from the workspace settings), where they reached shell history and the process list. They now reach a run as environment variables, and the settings that hold them are machine-scoped, not workspace-scoped. (VS Code already disables an extension in Restricted Mode when it declares no `untrustedWorkspaces` capability, so this mattered after the person trusted the folder.)
2. **Fixed: `flutterprobe.probePath` and `<workspace>/bin/probe` let a repository make the extension run a binary it contained.** The paths of the tools the extension runs are now machine-scoped settings, and a binary inside the workspace is used only when the person opts in. (As for finding 1, this mattered after the person trusted the folder.) Still open: the same rule for the `tools:` block of a repository's `probe.yaml`, which the CLI reads; it is in progress and not merged, so a repository's `probe.yaml` can still name a tool path for the CLI.
3. **A documented trust-model fact, not a bug: the extension spawns children with the whole environment.** An allowlist would break the documented ProbeScript `${NAME}`, which reads any environment variable when a step runs (`internal/runner/executor.go`, `expandEnv`; `website/src/content/docs/probescript/syntax.md`); most runs go through terminals the extension creates, whose shell loads its own profile; and since finding 2 the binary the extension runs is one the user chose. The small mitigation has shipped: only test runs receive the three cloud secrets, and children that do not need them do not get them.
4. **The automation discovery file** is written with 0600 in a 0700 folder created for it, but an existing folder or file is not re-checked, a symbolic link is not refused, and with no user config folder it falls back to the system temp folder. `probe-mcp --http` already does all three checks (`writeTokenFile`); the same function should serve both.
5. **The headless bridge's token** is served in an unauthenticated script and the event stream takes it in a URL. It is a test-only surface that must stay that way: it is off by default, and the docs and the decision table must never describe it as a transport for an editor.
6. **Two `probe-mcp --http` servers of one user overwrite each other's token file.** It warns; a per-port file name would remove the surprise.

## Options for the first transport
- **A. A stdio child process** (`probe ide serve`, JSON-RPC, like MCP stdio). No port, no token, no file; the extension owns the pipe and the lifetime (`--exit-on-stdin-close`, as the device view already has). Reaches only what the CLI reaches. It cannot see a Studio window.
- **B. The automation endpoint**, discovered through its file. It reaches Studio's connected device and workspace, so an editor and Studio can share one connection. It needs the file hardening above, and a method added to it needs an entry in the decision table with `callers: automation`.
- **C. MCP over HTTP.** It exists, with the strongest checks of the three, but it is shaped for agents (tools, long jobs) and carries every tool.

Recommendation: **A first**, then B for the cases that need Studio's live connection. A gives the editor the common actions with the least surface, and its tests are the CLI's. B's hardening is cheap and is needed anyway. C stays for agents.

## How it relates to the bound methods and MCP
- Every method the IDE transport offers is a row in the decision table with `callers` including a new value, `ide`, and the table's checks apply: it must not be in a category `automationNever` names, `touches` must say files, devices or network truthfully, and a method that can destroy anything has `confirm: asks` and gets its question from the person, never from the editor.
- Where MCP and the IDE transport offer the same action, one implementation serves both (the CLI's), so the consent flags, the redaction and the limits are written once.
- The editor's UI may show a question (the editor's own dialog), but the answer travels to the process that asks and counts only for the pending request with that identifier, once, as `AnswerConfirm` does; a script cannot answer it.

## First slice
1. Findings 4 to 6 above (the automation discovery file, the bridge's token, the MCP token file name), as three small changes with a test and a mutant each. Findings 1 to 3 are done (secrets out of argv and workspace settings, tool paths machine-scoped, secrets only for test runs); the `tools:` rule for a repository's `probe.yaml` is the known open item.
2. Make `writeTokenFile` the one writer of token files and use it for the automation discovery file, with the refusal tests it already has.
3. `probe ide serve` over stdio with two read-only methods, `lint_text` and `list_devices`, through the same code as the CLI commands; a protocol test (one request, one answer, malformed input, oversize input, closing stdin ends it) and rows in the decision table.
4. A short page for extension authors: what the transport is, what it will never do, how consent works.

Not in the first slice: running tests from the editor through the new transport (the terminal path stays until the consent flow for it is written), any network method, any method in a category kept from automation.

## Measurable targets
1. 0 methods reachable through the IDE transport in a category `automationNever` names (a test over the decision table).
2. 0 secrets in argv, in shell history, in logs and in the extension's output channel, for the settings that exist today (a test that plants a secret in each and searches the spawned arguments and output).
3. The transport answers a malformed, oversize or truncated request without a crash and without leaving a process behind (a fuzz test, with the seeds in the repository and a documented way to run longer).
4. A web page gets nothing: the browser-origin matrix already used for MCP over HTTP and the automation endpoint, run against any new listener.
5. Startup to first answer under 300 ms for `lint_text` on a small file, so an editor can call it on save.

## Open decisions
- Option A first, as recommended, or B.
- Whether `ide` is a new value in the table or the IDE transport shares `automation`'s set (proposal: a new value, so the two can differ).
- Whether a hostile-workspace default (`probePath` ignored until trusted) is acceptable for people who keep a binary in `bin/` today.
