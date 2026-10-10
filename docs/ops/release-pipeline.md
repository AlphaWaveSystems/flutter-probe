# Release pipeline: canary → dogfood → stable

Every release passes through a canary channel and a dogfood gate before it
becomes the stable version that `brew install probe` and pub.dev serve. No
step in this pipeline depends on a human; the gates are checks, labels and
timers.

## Channels

| Channel | Tag | Workflow | Installs as |
| --- | --- | --- | --- |
| Canary | `vX.Y.Z-next.N` | `release-canary.yml` | `brew install AlphaWaveSystems/tap/probe@next`; pub.dev pre-release `X.Y.Z-next.N` |
| Stable | `vX.Y.Z` | `release.yml` + `publish-pub-dev.yml` | `brew install probe`; pub.dev stable |

`scripts/release.sh 0.24.0-next.1` bumps the version files, commits and tags a
canary; `scripts/release.sh 0.24.0` tags the stable release once the gate has
passed.

## Stages

1. **Merge.** A PR merges only after CI and the `review-agent.yml` check pass.
   The review agent runs an automated review and submits an approving review
   on a clean verdict; without a configured reviewer it fails closed and the PR
   waits for a human.
2. **Canary.** Tagging `vX.Y.Z-next.N` publishes a GitHub pre-release, the
   `probe@next` Homebrew formula and pre-release Dart packages. The stable
   formula is untouched.
3. **Dogfood gate** (`dogfood-gate.yml`, dispatched with the canary version):
   - the fixture twin-app suite runs on named iOS simulator and Android
     emulator devices with the pinned canary (`PROBE_AGENT_NO_UPGRADE=1`);
   - an internal dogfood app then runs its own full E2E suite against the same
     canary. The app registers itself on the runner as `~/dogfood/gate.sh`; the
     repository never names it.
   - Both must pass. Two green gates on consecutive days are the 48-hour soak.
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

## Rules

- A hotfix for a `prod-incident` may skip the soak, never the fixture suite.
- A canary never moves a consuming app's version lockfile; only a green stable
  does.
- The soak cannot be shortened by automation.

## Required repository settings

- Branch protection on `main`: required checks `CI`, `Review agent`; allow
  reviews from the `github-actions` app to satisfy the approval requirement;
  merge queue enabled.
- Secrets: `HOMEBREW_TAP_TOKEN` (exists), `ANTHROPIC_API_KEY` (review agent).
- Variable `DOGFOOD_RUNNER_READY=true` once a macOS self-hosted runner with the
  `dogfood-mac` label is registered.
- Labels: `dogfood/pending`, `dogfood/passed`, `dogfood/failed`,
  `prod-incident`.
