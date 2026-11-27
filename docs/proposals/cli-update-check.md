# Design: CLI update check

Status: proposal, approved by the owner on 2026-11-27 (decisions below). No code yet; the implementation is a separate change that starts after this proposal merges.

## The goal
Each time `probe` runs, it checks GitHub for a newer stable release. If there is one, it tells the person, once, on stderr, how to update. It never updates the binary itself.

## Decisions (owner, 2026-11-27)
1. **Trigger:** every command, including `probe version`, `probe prefs` and `probe help`. The suppression rules below still apply.
2. **Frequency:** a conditional request (ETag) on every start. A 24-hour cache is the fallback, used only if the conditional behaviour is not confirmed when it is verified (see "Before building").
3. **Default:** on, with an opt-out.
4. **Display:** the stderr notice, and the same status in `probe version` and `probe doctor`, so the version is reported consistently across the CLI.
5. **Canary builds** (`-next.N`) are told about the latest stable release.
6. **Endpoint:** the GitHub releases API. No FlutterProbe-run service.

## Behaviour
- The check runs in the background. The command never waits for it. The notice prints at exit, and only if the answer arrived within a 1.5-second budget.
- The notice is one line on stderr, for example: `A new FlutterProbe version is available: 0.29.0 (you have 0.28.1). Update: brew upgrade probe`. When the binary sits under Homebrew's prefix the hint is `brew upgrade probe`; otherwise it links to the release page. A canary installed as `probe@next` gets the switch back to stable instead: `brew uninstall probe@next && brew install AlphaWaveSystems/tap/probe`.
- Silent in CI, with `--format json` or `junit`, when stderr is not a terminal, in `probe mcp-server` (the MCP stdio path inside `probe`), and on dev builds (version `dev`). There is no `--quiet` flag today; the rule applies to one if it is added.
- Offline, timeouts, rate limits and malformed answers are all silent.
- Versions are compared as semver against the latest stable release. A `-next.N` build compares its base version.

## Opt-out and privacy
- `PROBE_NO_UPDATE_CHECK=1` or `probe prefs set update-check off` stops every request, not only the notice.
- Each request is an unauthenticated GET to `https://api.github.com/repos/AlphaWaveSystems/flutter-probe/releases/latest`, with a User-Agent of `flutterprobe/<version> (<os>/<arch>)`. It carries no account, cookie, project data or other identifier.
- The CLI documentation states what is sent, and how to turn it off.

## Cache
- `update-check.json` in the existing preferences folder (`os.UserConfigDir()/flutter-probe`), written atomically with mode 0600.
- It holds the last check time, the ETag, the latest version and the release URL.

## Code
- A new package, `internal/updatecheck`: the request, the comparison, the message and the cache.
- `probe version` and `probe doctor` read the same cached status.
- `internal/userprefs` gains an `update-check` preference, following the existing `clear-data-without-asking` pattern.
- `cmd/probe-mcp` and `probe mcp-server` (`internal/cli/mcp.go`) do not run the check, and their stdout is untouched.

## MCP (the owner's rule)
- A new read-only, free MCP tool reports the update status: the current version, the latest stable version and whether an update is available.
- Before the feature counts as done, it is verified through the real MCP stdio path: the notice never reaches stdout, the tool answers, and the opt-out stops the requests.
- `website/src/content/docs/tools/mcp.md` and the tool description are updated, per the existing rule.

## Tests
- Semver: prereleases, `-next.N`, equal, older and malformed versions.
- The cache and the ETag behaviour against a local test server. Timeouts and errors stay silent.
- Each suppression path, and the stdout-clean checks for JSON and MCP output.
- No test touches the real network.

## Docs and release
- CLI reference, installation page (how to update), the CLI privacy statement, the MCP docs and the CHANGELOG (Added).
- This is a new feature, so the next release is 0.29.0, through the normal pipeline.

## Before building
- Confirm GitHub's current rules for conditional requests (ETag and 304 responses) and rate limits. Decision 2 depends on them.
- Confirm that `releases/latest` excludes prereleases and drafts.
