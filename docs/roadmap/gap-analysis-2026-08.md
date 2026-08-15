# FlutterProbe vs. Maestro — Gap Analysis (2026-08-14 snapshot)

Archived reference. For active tracking, see `maestro-competitive-roadmap.md` in this directory —
this file is the point-in-time research that produced it and won't be kept in sync afterward.

Full interactive version: https://claude.ai/code/artifact/77e4a70f-260b-43ec-ba3f-522c1cf557ae

Compared: FlutterProbe v0.10.4 (at research time; `main` has since moved to v0.11.0) vs. Maestro
cli-2.8.0 (147 GitHub releases, 2022–2026).

## Where FlutterProbe already wins

- Physical iOS devices (Maestro: simulators only, its most-requested unshipped gap).
- Flutter element-tree access (Keys, widget types, geometry) vs. Maestro's semantics-tree-only
  interaction, which requires manual `Semantics` widgets and can't see Flutter `Key`s at all.
- Biometric testing (`enroll biometric`, `biometric match/no match`) — Maestro issue #282, open
  since 2022.
- Network mocking (device-side mock + real HTTP calls) — Maestro deprecated its entire mocking
  stack in 2023 and never replaced it.
- Drag and drop — Maestro issue #1203, 50+ reactions, open.
- Multi-device composite tests with `sync` barriers — no Maestro equivalent.
- Native-prompt signals (`deliver signal`) and an arbitrary-Dart escape hatch (`run dart:`).
- Data-driven tables (inline + CSV) — Maestro has env vars/JS but no table-driven runner.
- Parallelism without a cloud paywall (BYO device farms) vs. Maestro's $250/device/month cloud,
  which replaced a cheaper tier in a move that upset their community (Robin rebrand, early 2025).

## Where Maestro wins (as of the research date)

- ~~AI visual assertions~~ — **closed 2026-08-14**: FlutterProbe v0.11.0 shipped all 4 phases
  (`see ... with ai`, `assert no visual defects with ai`, `ai.provider: local`,
  `read ... with ai into <var>`) with a local-first/BYOK design explicitly positioned against
  Maestro's mandatory-cloud-upload model. See `docs/prd/ai-visual-assertions-prd.md` and
  `docs/research/maestro-ai-assertions-investigation.md`.
- Native/system UI automation (pickers, share sheets) — tracked as Phase 2 in the roadmap,
  design already scoped in `docs/proposals/pt13-native-ui-bridging.md`.
- WebView content (Chrome DevTools hierarchy mode) — no FlutterProbe equivalent yet.
- Flutter Web / web platform — no FlutterProbe target yet.
- Per-element visual regression (`assertScreenshot` + `cropOn`) — FlutterProbe compares full
  screenshots only; tracked as roadmap E-2.
- Flow ergonomics: `retry` blocks, `optional: true` per step — tracked as roadmap E-1.
- `addMedia` (gallery seeding), `travel` (motion routes), in-app deep-link `openLink`.
- Tooling maturity: Maestro Studio desktop app + Maestro Viewer vs. FlutterProbe Studio (beta).

## Maestro's 2025–2026 strategic direction

MCP-first agent integration (dedicated `maestro chat` discontinued in 2.7.0 in favor of MCP),
heavy AI investment since 1.38 (Aug 2024), cloud consolidation under the Robin-derived pricing
model. FlutterProbe already ships an 18-tool MCP server + a Claude Desktop `.mcpb` extension —
parity exists, the gap is visibility, not capability (roadmap G-1/G-2).

## Sources

github.com/mobile-dev-inc/maestro releases & CHANGELOG (147 releases through cli-2.8.0),
docs.maestro.dev command reference & platform docs, maestro.dev/pricing, G2 reviews, GitHub issue
tracker (reaction-ranked); FlutterProbe repo audit — parser/AST, CLI, CHANGELOG history,
`IMPROVEMENT_TASKS.md` (PT-01…PT-27), `DONE.md`, PT-13 proposal.
