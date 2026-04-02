## What changed and why

<!-- Describe the change and the motivation behind it. -->

## Checklist

### All PRs
- [ ] `go build ./...` passes locally
- [ ] `go test ./...` passes locally
- [ ] `CHANGELOG.md` (root) updated

### If `probe_agent/lib/` changed
- [ ] `dart analyze lib/` passes (`cd probe_agent && dart analyze lib/ --fatal-infos`)
- [ ] `flutter test` passes (`cd probe_agent && flutter test`)
- [ ] `dart pub publish --dry-run` passes (`cd probe_agent && dart pub publish --dry-run`)
- [ ] `probe_agent/CHANGELOG.md` updated (required — CI will fail without it)
- [ ] `probe_agent/README.md` updated if behavior visible on pub.dev changed (install steps, new commands, new options)

### If new ProbeScript commands or CLI flags were added
- [ ] `probe_agent/README.md` — Features section updated
- [ ] `website/src/content/docs/` — relevant doc page updated
- [ ] `website/astro.config.mjs` — new doc page added to sidebar if applicable

### If this is a release PR
- [ ] `probe_agent/pubspec.yaml` version bumped
- [ ] `vscode/package.json` version bumped
- [ ] `docs/wiki/Home.md` current version updated
