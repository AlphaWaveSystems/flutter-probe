# Testing a report upload without GitHub Actions

`probe test --upload-report` is meant for a GitHub Actions job and talks to the hosted service. This folder lets you exercise the whole client side on one machine, with no CI and no service: a fake service, a throwaway license, and the GitHub variables set by hand. It tests the client (the CI facts, the scrubbed payload, the license check, the request), **not** the real service.

| File | What it is |
|---|---|
| `fake_server.py` | A fake service on `127.0.0.1`. `POST /v1/reports` records the request and answers `--status` (default 202). `GET /oidc` plays GitHub's identity-token issuer and returns a **placeholder** token. Python 3, no packages. |
| `sign-test-license/` | Makes a throwaway Ed25519 key pair and a one-day license signed with it, and prints the two `export` lines. It generates the key pair at run time, marks the license (`customer: LOCAL-TEST-ONLY-NOT-A-REAL-LICENSE`, key id `localtest`) and prints a TEST ONLY line on stderr. It refuses to run when `PROBE_CLOUD_URL` names a host that is not this machine. |

## What you need

- A built `probe` and a device the test can run on (an emulator, a simulator or a phone). The upload refuses a report that names no platform.
- Python 3 and Go.

## Steps

```bash
# 1. start the fake service (leave it running; Ctrl-C stops it)
LOG="${TMPDIR:-/tmp}/fake-cloud.log"
python3 scripts/fake-cloud/fake_server.py --log "$LOG"

# 2. in another terminal: a throwaway license that this probe will trust
eval "$(go run ./scripts/fake-cloud/sign-test-license)"

# 3. say where the service is, and pretend to be GitHub Actions
export PROBE_CLOUD_URL=http://127.0.0.1:48899
export GITHUB_ACTIONS=true GITHUB_REPOSITORY=acme/app GITHUB_REF_NAME=main \
       GITHUB_SHA=0123456789abcdef0123456789abcdef01234567 \
       GITHUB_RUN_ID=1001 GITHUB_RUN_ATTEMPT=1 GITHUB_JOB=e2e GITHUB_EVENT_NAME=push

# 4. run a test with the upload on
probe test tests/ --device <id> --upload-report --upload-label local-sim

# 5. see what arrived
cat "$LOG"                                   # one JSON line per request
python3 -m json.tool "$LOG.body.json"         # the last body
```

For a pull request, set `GITHUB_EVENT_NAME=pull_request`, `GITHUB_REF_NAME=7/merge`, `GITHUB_HEAD_REF=<branch>`, `GITHUB_BASE_REF=main` and `GITHUB_EVENT_PATH=<a JSON file>` with `{"repository":{"default_branch":"main"},"pull_request":{"head":{"sha":"<40 hex>"}}}`.

`--upload-dry-run` needs none of this: it prints the payload and a note on what a real upload would refuse, with no key and no network.

## What to expect

| Setup | Result |
|---|---|
| A made-up license key | Nothing is sent: `license: no verification key configured`. The CLI checks the signature itself, and a development build trusts no key unless `PROBE_LICENSE_PUBKEY` names one (step 2 does that). |
| Step 2 license, no identity token | Uploaded. Warnings: no event file (no default branch) and no identity token. The server sees a bearer license key and the report with the CI facts. |
| Add a fake token issuer: `ACTIONS_ID_TOKEN_REQUEST_URL=http://127.0.0.1:48899/oidc?api-version=2.0` and `ACTIONS_ID_TOKEN_REQUEST_TOKEN=x` | Uploaded with an `X-Probe-Attestation` header. The client first asks the issuer for audience `probe-cloud`. It accepts a plain `http` URL because the host is loopback. |
| `--upload-local` outside a git folder | Skipped with a note that local uploads need a git repository. |
| `fake_server.py --status 429` or `503` | The public docs say the client retries these with backoff for up to five minutes, and that any other failure is only a warning. The server can answer these, but the retry behaviour was not exercised with it. |

## What this cannot test

- **Verified runs.** The token the fake issuer returns is a placeholder. The real service checks identity tokens against GitHub's keys, so it would call such a run unattested. Auto-registration and verified verdicts need a real Actions run.
- **The service itself.** The fake server verifies nothing, stores nothing and computes no verdict.
- **The license.** The key pair is made for the test, and the private half is never printed or saved. The public half is trusted only because `PROBE_LICENSE_PUBKEY` names it. A released `probe` trusts none of it **while that variable is unset**: `internal/license` adds the keys it lists to the trusted set in every build, so anyone who controls a `probe` process's environment can make it trust a key of their own. That is local trust only (what the CLI will accept in that process), and the real service checks licenses itself. Keep the variable out of shells and CI jobs that talk to a real service.

The fake server logs only the scheme and length of the license key and the identity token, never their values. It binds `127.0.0.1` only, refuses a request body over 1 MiB (413) or without a plain `Content-Length` (411), and writes its log only inside `--log-dir` (default: the system temp folder), so a typo cannot overwrite a file elsewhere.

## Using `act`

[`act`](https://github.com/nektos/act) runs a workflow in local containers. It is useful for checking which variables a workflow sees, but it does not replace the steps above:

- It sets `GITHUB_ACTIONS`, `GITHUB_REPOSITORY` (`nektos/act` unless you override it), `GITHUB_REF_NAME`, `GITHUB_SHA`, `GITHUB_RUN_ID`, `GITHUB_RUN_ATTEMPT`, `GITHUB_JOB`, `GITHUB_EVENT_NAME` and `GITHUB_EVENT_PATH`, and for `pull_request` events with `-e <event file>` also `GITHUB_BASE_REF`, `GITHUB_HEAD_REF` and `GITHUB_REF_NAME=<n>/merge`. The client read all of them correctly.
- It sets **no** `ACTIONS_ID_TOKEN_REQUEST_URL` or token, even with `permissions: id-token: write`, so the client reports no identity token available.
- Containers are Linux: an Android emulator or an iOS simulator cannot run inside them, so a device test does not run under `act`.
- With Podman instead of Docker, point `DOCKER_HOST` at the machine's socket (`podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}'`), pass `--container-daemon-socket -` (otherwise `act` tries to mount the socket's host path into the container and fails), and use `--bind` with a project folder under your home folder (a folder outside it is not visible to the Podman machine and shows up empty).
