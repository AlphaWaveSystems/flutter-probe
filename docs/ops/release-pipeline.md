# Release pipeline: canary → dogfood → stable

Every release passes through a canary channel and a dogfood gate before it
becomes the stable version that `brew install probe` and pub.dev serve. No
step in this pipeline depends on a human; the gates are checks, labels and
timers.

## Channels

| Channel | Tag | Workflow | Installs as |
| --- | --- | --- | --- |
| Canary | `vX.Y.Z-next.N` | `release-canary.yml` | `brew install AlphaWaveSystems/tap/probe@next`; agent from the tagged source (git dependency, `path: probe_agent`); a pub.dev pre-release is published only when the pubspec version carries `-next.N`. User guide: website `getting-started/installation` |
| Stable | `vX.Y.Z` | `release.yml` + `publish-pub-dev.yml` | `brew install probe`; pub.dev stable |

`scripts/release.sh 0.25.2-next.1` bumps the version files, commits and tags a
canary; `scripts/release.sh 0.25.2` tags the stable release once the gate has
passed.

## Stages

1. **Merge.** A PR merges only after CI and the `review-agent.yml` check pass.
   The review agent runs an automated review and submits an approving review
   on a clean verdict; without a configured reviewer it fails closed and the PR
   waits for a human. It only reviews same-repository branches from owners,
   members and collaborators, reviews the diff (never a checkout of the PR
   branch) as untrusted input, and fails a branch that is behind `main` so a
   stale branch cannot silently revert what `main` merged since.
2. **Canary.** Tagging `vX.Y.Z-next.N` publishes a GitHub pre-release, the
   `probe@next` Homebrew formula and pre-release Dart packages. The stable
   formula is untouched. The iOS system-dialog driver (`probe-ios-driver.zip`)
   is built by a separate, non-blocking `ios-driver` job and attached to the
   pre-release afterwards, so `probe ios-driver install` works against a
   canary; if that job fails the canary stays valid and the install reports a
   404. Canaries carry no Studio, `.mcpb` or Windows assets; those are stable only.
3. **Dogfood gate** (`dogfood-gate.yml`, dispatched with the canary version):
   - the fixture twin-app suite runs on named iOS simulator and Android
     emulator devices with the pinned canary (`PROBE_AGENT_NO_UPGRADE=1`);
   - an internal dogfood app then runs its own full E2E suite against the same
     canary. The app registers itself on the runner as `~/dogfood/gate.sh`; the
     repository never names it.
   - Both must pass. **One green gate of about an hour or more on the named
     devices** is required before a stable tag; there is no second-day gate.
     The 24-hour auto-rollback watch after the stable tag (stage 5) stays. The
     separate 14-day soak toward 1.0 (issue #325) is unrelated and unchanged.
   - Without a self-hosted runner (`vars.DOGFOOD_RUNNER_READY` unset) the
     workflow opens a `dogfood: gate request` issue instead; the gate is then
     run locally and reported with the `dogfood/passed` or `dogfood/failed`
     label and the failing test names.
4. **Promote.** Tag `vX.Y.Z`. `release.yml` builds, publishes the release, and
   updates the stable Homebrew formula; `publish-pub-dev.yml` publishes the
   stable Dart packages; CHANGELOG and docs are updated in the release PR.
5. **Rollback** (`auto-rollback.yml`). An issue labelled `prod-incident` within
   24 hours of a stable release marks the previous stable release as `latest`
   again, restores its Homebrew formula and comments on the issue. Tags are
   never moved or deleted. pub.dev packages cannot be unpublished; pin the
   agent to the previous version until the hotfix ships.

## Review agent providers

The repository variable `REVIEW_PROVIDER` selects who reviews (default `oauth`).
Whatever the mode, a missing credential fails the check; it is never skipped.

| `REVIEW_PROVIDER` | Credential | Runs on | Can approve |
| --- | --- | --- | --- |
| `oauth` (default) | secret `CLAUDE_CODE_OAUTH_TOKEN`, created with `claude setup-token` | hosted runner | yes |
| `anthropic` | secret `ANTHROPIC_API_KEY` | hosted runner | yes |
| `local` | variable `REVIEW_LOCAL_URL` (an OpenAI-compatible endpoint; optional `REVIEW_LOCAL_MODEL`) | self-hosted `dogfood-mac` runner | **no** |

`local` is an extra required pre-screen only (secrets, leaked internal names,
CHANGELOG and docs checks). It posts no review, so a human approval is still
required by branch protection; the check just goes red when the pre-screen
finds something. The runner needs `git`, `gh`, `jq`, `curl` and `openssl`.

How the agent stays safe, in every mode:

- It reviews only branches in this repository by owners, members and
  collaborators. Anything else fails the check (a skipped required check would
  count as passing, so it fails instead of skipping).
- The model has no tools. It receives one prompt: the repository conventions
  from the trusted base commit and the diff between random markers, declared
  untrusted. Nothing from the PR (diff, title, body, branch name) reaches a
  shell through `${{ }}`; values pass through `env:` and are validated.
- A credential-shaped string in the added lines fails the check before any model
  sees the diff. Diffs over 400 KB need a human.
- The verdict counts only as the exact first line of the reply
  (`VERDICT: APPROVE` or `VERDICT: REQUEST_CHANGES`); anything else rejects.
- An approval is bound to the reviewed commit, so a later push cannot inherit
  it. The branch must contain the current tip of its base at review time and
  again just before the review is submitted. Branch protection should also keep
  "require branches to be up to date" and the merge queue on, which re-check
  at merge time.
- The Claude Code CLI version is pinned (variable `CLAUDE_CODE_VERSION`
  overrides the default) and the actions are pinned by commit.
- Dependabot PRs are not reviewed by this agent (their author association is
  not trusted and Actions secrets are not exposed to them); they follow their
  own soak workflow.

## Before a release: the release PR checklist

Do this in the release PR (the version bump and the CHANGELOG section), before the canary tag. `scripts/release.sh` only handles
three of these files and creates a commit and tag on the current branch, so the release PR does the rest by hand.

Version-bearing files (set all to the new `X.Y.Z`):

- `VERSION` (the landing page badge reads it at build time)
- `probe_agent/pubspec.yaml` and `probe_agent/lib/src/agent_version.dart`
- `vscode/package.json` and both `version` entries of `vscode/package-lock.json`
- `docs/wiki/Home.md` ("Current version")
- `ios-driver/UITests/Driver.swift` (the version the driver reports)
- `native-test-apps/flutter/pubspec.lock` (run `flutter pub get` there)
- `CHANGELOG.md` (move Unreleased into `[X.Y.Z] - date`, grouped Added / Changed / Security / Fixed / Docs) and `probe_agent/CHANGELOG.md`

Greps that find what a bump misses (run them with the **previous** version `P`, and fix every hit that is not history or test data):

```bash
git grep -n "P" -- . ':!CHANGELOG.md' ':!probe_agent/CHANGELOG.md' ':!go.work.sum' ':!docs/research' ':!docs/prd' ':!docs/evidence'
git grep -n "flutter_probe_agent: *\^0" -- README.md website probe_agent/README.md docs/wiki
```

- **Install snippets.** The second grep lists every `flutter_probe_agent: ^0.x.y` snippet. For a 0.x package a caret constraint
  means "this minor only" (`^0.5.3` resolves to 0.5.x), so a stale snippet makes a new user fetch an agent far older than the CLI.
  Each release sets them to the new minor (for example `^0.25.0`); historic blog posts are left alone.
- **Canary and tag examples** in `docs/ops/release-pipeline.md`, `.github/workflows/dogfood-gate.yml` and the install guide ("Try a canary
  build") use the current `X.Y.Z-next.N`.
- **MCP tool count**: compare the count in the README, the MCP guide and `mcpb/template/manifest.json` with the server's `tools/list`
  (`printf` an initialize and a `tools/list` request into `probe-mcp`), and update the text when it changed.
- **Grammar**: if the parser changed, the EBNF page and `internal/parser/grammar_test.go` are updated in the same PR.
- **Surfaces**: README, `probe_agent/README.md`, the website pages the release touches, `llms.txt`, the MCP guide in `internal/mcp/server.go`, the VS Code
  README, snippets and completions, the wiki.

## After a stable tag: verify

- the release is "latest" and carries the CLI and MCP binaries for every platform, the Studio archives, the `.mcpb` extensions **and `probe-ios-driver.zip`** (`gh release view vX.Y.Z --json assets`);
- the stable Homebrew formula has the new version and its sha256 values equal the downloaded assets;
- pub.dev shows the new agent version;
- `probe version` prints the new version and `probe ios-driver install` downloads the driver.

## Rules

- A hotfix for a `prod-incident` may use a shorter dogfood run than the one-hour gate, never skip the fixture suite.
- A canary never moves a consuming app's version lockfile; only a green stable
  does.
- The length of the dogfood gate (about an hour on the named devices) cannot be shortened by automation.

## Required repository settings

- Branch protection on `main`: required checks `CI`, `Review agent`; allow
  reviews from the `github-actions` app to satisfy the approval requirement;
  merge queue enabled.
- Secrets: `HOMEBREW_TAP_TOKEN` (exists) and the credential for the chosen
  review provider (below).
- Variable `DOGFOOD_RUNNER_READY=true` once a macOS self-hosted runner with the
  `dogfood-mac` label is registered.
- Labels: `dogfood/pending`, `dogfood/passed`, `dogfood/failed`,
  `prod-incident`.

## Disclosure guard

`.github/workflows/disclosure-guard.yml` scans every pull request (added lines, changed file names, commit messages, title and body) and every push to `main` against a maintainer-defined list of patterns kept in the repository secret `DISCLOSURE_DENYLIST`, one extended regex per line. The list is deliberately not in the repository. A match fails the check and is reported by file and line plus a rule number; the matched text is never printed. The scanner is `scripts/disclosure-scan.sh <repo> <git-range> <patterns-file>`; maintainers can run it locally with the same patterns file before opening a pull request. Pull requests from forks cannot see the secret and fail with instructions: a maintainer re-opens them from a branch in this repository. Add the check `Disclosure guard` to the required checks of the default branch once the secret is set.
