# Repository Maintainer View

Canonical local world-view summary for the repository maintainer workflow.
This file is gitignored; it is not part of checked-in history.

Companion surfaces:

- `factory/internal/asks.md`
- `factory/internal/progress.md`
- `factory/internal/meta.md`

## Current view (2026-06-10 UTC)

### Branch and workspace

- Branch `manual-fixes-0` is **ahead of origin by 8 commits** with in-flight
  factory-graph editor, event-stream topology, validation-payload, and UI test
  behavior work (open PR #783).
- `git pull` is clean against `origin/manual-fixes-0`; large unstaged deltas
  remain locally (factory topology, UI tests, deleted legacy factory docs).
- Root scratch files `test.js` / `test-2.js` look like abandoned browser-fetch
  experiments and should not be committed.

### Factory operator loop

- Checked-in `factory/factory.json` loads with **current source** after layout
  boundary validation was relaxed so `layout.nodes[].size` is optional again.
- The globally installed `you` at `~/.local/bin/you` was **stale** and still
  required node sizes; rebuild with `make build` and run `./bin/you` on this
  branch until the install is refreshed.
- Factory topology changes on the branch:
  - `cleaner` cron workstation removed.
  - `though-retrigger` hourly cron emits `thoughts:init` loopback work.
  - `review` work type and paired review flow added to the maintainer pipeline.
  - `factory/docs/batch-inputs.md` bundled as a supporting doc.

### Active cleanup tracks (non-overlapping)

| Track | Status |
|-------|--------|
| Meta-test retirement (UI barrels, trace drilldown inventories, petri/world-view contract guards) | **In flight** — open PRs #784–#789 |
| Exhaustion-rule AST contract guard retirement | **Dispatched** — idea `exhaustion-rule-contract-guard-meta-test-retirement` submitted 2026-06-10 |
| Dynamic Workflows v0 contract-repair (B1–B12) | **Planned** — Batch 001 merged; Batch 002 skeleton **no-go** until cross-surface gaps close (`docs/internal/development/plans/dynamic-workflows/dynamic-workflows-plan.md`) |
| manual-fixes-0 graph editor / topology save | **In flight** — PR #783; do not stomp with unrelated cleanups |

### Customer asks

No active checked-in asks (`factory/internal/asks.md`). Prior
`service-cleanup-on-success` ask is delivered.

### Problems worth fixing next (priority order)

1. **Meta view accuracy** — keep theory-of-mind aligned as manual-fixes-0 lands.
2. **Contract-repair batch** for Dynamic Workflows v0 blocking gaps before fake-session skeleton.
3. **Meta-test retirement** — continue replacing AST/field-inventory guards with runtime/API/CLI/UI/event behavioral assertions.
4. **Structural simplification** — dead code, redundant legacy shims, overlapping `pkg/interfaces` only after behavioral coverage exists.
