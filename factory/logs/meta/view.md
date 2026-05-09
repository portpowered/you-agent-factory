# meta view

## world state

- as of `2026-05-09T14:02:51+09:00`, live `origin/main` still points at
  merged `PR #174` commit `74b874a`, while local `main` also carries stacked
  meta-only refresh commits, including `c1b5823` on open `PR #175`
- the canonical maintainer ask surface remains `factory/logs/meta/asks.md`
- the current canonical local ask file still says there are no active customer
  asks: `for now no asks exists.`
- the tracked maintainer workflow inputs remain sentinel-only under
  `factory/inputs/**`; live work items there are ignored operating state
- the local worktree is still dirty from tracked local edits in
  `factory/logs/meta/asks.md` and `factory/workers/workspace-setup/AGENTS.md`;
  treat those as existing local state, not as noise to revert

## workflow truth

- `factory/factory.json` still defines five work types: `thoughts`, `idea`,
  `plan`, `task`, and `cron-triggers`
- the checked-in maintainer loop on live `main` is:
  `thoughts:init -> ideafy -> thoughts:complete`
  `idea:init -> plan -> idea:to-complete + plan:init`
  `plan:init -> setup-workspace -> plan:complete + task:init`
  `task:init -> process -> task:in-review -> review -> task:to-complete`
  `idea/task:to-complete -> consume -> idea/task:complete`
- topology details that still matter:
  - `process` and `review` execute in `.claude/worktrees/{{name}}`
  - shared `executor-slot` capacity remains `10`
  - loop breakers still guard repeated `process` and `review` retries

## input surface truth

- tracked `factory/inputs/**` content is still sentinel-only:
  - `factory/inputs/BATCH/default/.gitkeep`
  - `factory/inputs/idea/default/.gitkeep`
  - `factory/inputs/plan/default/.gitkeep`
  - `factory/inputs/task/default/.gitkeep`
  - `factory/inputs/thoughts/default/.gitkeep`
- `.gitignore` still keeps live workflow submissions under `factory/inputs/**`
  out of normal commits except for those sentinel paths
- the previously queued ignored idea
  `factory/inputs/idea/default/dedupe-service-smoke-pipeline-config-builders.md`
  was stale because merged PR `#174` landed that exact lane on `main`
- that stale ignored idea residue has now been pruned locally so the operating
  queue no longer points at already-merged work
- the local ignored operating queue now contains one fresh standalone idea:
  `factory/inputs/idea/default/split-bootstrap-portability-functionallong-helpers.md`

## customer-ask truth

- the local canonical ask file continues to withdraw the earlier checklist,
  coverage, simplification, and minimum-concurrency backlog for this cycle
- there is therefore no active customer-directed requirement right now to keep
  a minimum number of simultaneous lanes in flight
- open PRs can still inform overlap checks, but they do not become asks unless
  `factory/logs/meta/asks.md` reintroduces them

## recent repo movement

- recent merged PRs on `main` now include:
  - `#174` `dedupe-service-smoke-pipeline-config-builders`
  - `#170` `weird-work-names`
  - `#169` `collapse-replay-safe-diagnostics-rehydration`
  - `#166` `simplify-loaded-runtime-definition-lookups`
  - `#165` `localize-workflow-activity-graph-import-copy`
- `gh pr list --state open` on `2026-05-09` reports:
  - `#175` `docs: refresh meta world state`
  - `#173` `docs: refresh meta world state`
  - `#172` `same-trace`
  - `#171` `workflow-graph-padding`
  - `#167` `localize-work-outcome-trend-cards-copy`
  - `#163` `docs: refresh meta world state`
  - `#152` `docs: refresh meta world state`
  - `#145` `docs: refresh meta world state`
  - `#143` `docs: refresh meta world state`
  - `#141` `audit-repository-against-2026-website-and-backend-checklists`
  - `#139` `docs: refresh meta world state`
  - `#123` `docs: refresh meta world state`
  - `#120` `docs: refresh meta world state`

## open-lane truth

- `PR #141` still owns the repository-wide external checklist audit lane
- `PR #167` owns the current `ui/src/features/work-outcome/*` localization lane
- `PR #171` owns the dashboard-shell and workflow-graph padding lane
- `PR #172` owns the same-trace guard lane across config, petri, API, and
  functional coverage
- `PR #175` is the freshest open meta-refresh branch and supersedes `PR #173`
  plus the older still-open meta-refresh PR stack on the same file pair
- the smoke helper dedupe lane is closed on live `main` through merged
  `PR #174`, so it must not be re-queued

## replay truth

- `factory/logs/agent-fails.json` and
  `factory/logs/agent-fails.replay.json` remain the checked-in replay sample
  pair described in `factory/README.md`
- the replay pair is still historical fixture coverage rather than an exact
  copy of the current workflow contract
- one replay rejection payload is still quoted oddly as `"\"<REJECTED>\"\n"`;
  treat that as fixture history rather than live workflow behavior

## current maintainer decision

- this cycle queues one new cleanup request:
  `split-bootstrap-portability-functionallong-helpers`
- reason:
  - the canonical ask surface is still empty, so cleanup choice must be
    justified only by live repo state and non-overlap with active PR ownership
  - direct reads plus `docs/internal/development/deadcode-baseline.txt` show
    bootstrap-portability helper functions reported as unreachable solely
    because they live in always-built `_test.go` files while their only callers
    sit behind `//go:build functionallong`
  - the affected helpers are narrow and isolated to
    `tests/functional/bootstrap_portability/*`, so the cleanup does not overlap
    active ownership in `PR #141`, `PR #167`, `PR #171`, or `PR #172`
  - moving those helpers behind matching build tags removes deadcode-baseline
    noise without changing backend, API, CLI, or UI behavior
  - the alternative duplicated projection seams found by a delegated explorer
    remain real but broader, so they are lower priority than this tighter
    deadcode cleanup

## theory of mind

- the authoritative world model comes from live upstream git state, the
  checked-in workflow contract, the canonical ask file, current PR ownership,
  ignored queue residue, and direct code reads together
- when `factory/logs/meta/asks.md` changes locally, treat that edit as the
  immediate routing truth even if it withdraws a previously active backlog
- reason about `factory/inputs/**` in two layers:
  checked-in contract versus ignored operating state
- prune ignored local idea files once their owning PR merges; otherwise the
  canonical inbox can preserve stale work that the live repo already finished
- treat delegated explorer suggestions as hypotheses; re-verify them against
  live `main` before dispatching new cleanup work because recent merges can
  invalidate an otherwise plausible seam within the same cycle
- when an untagged test helper is only called from `functionallong` suites,
  treat the build-tag mismatch as the real deadcode seam and move the helper
  behind matching tags instead of normalizing the noise into the baseline
- when multiple open meta-refresh PRs touch only
  `factory/logs/meta/view.md` and `factory/logs/meta/progress.txt`, the newest
  live worldview supersedes the older stack rather than creating parallel lane
  ownership
