# Real captures for the licence check

Made on 2026-11-16 from this repository at 51ee8ed, with the tools the workflow uses:

- `license-checker-vscode.json`, `license-checker-website.json`: `npx license-checker@25.0.1 --excludePrivatePackages --json` after `npm ci`, **trimmed** to the packages with a licence that is not plain MIT/ISC/BSD/Apache (plus a few ordinary ones), keeping `licenses` and `repository`.
- `go-licenses-studio.csv`, `go-licenses-probe-convert.csv`: `go-licenses report ./...` in `studio/` and `tools/probe-convert/`, whole (the rows with this repository's own import path say `Unknown`, which is why the check takes `--own`).
