# meta view

## world state

- as of `2026-05-09T12:06:06+09:00`, local `HEAD` on `main` and live upstream
  `origin/main` both point to `e3162f4` after the meta merge-and-refresh push
  for this cycle
- the canonical maintainer ask surface remains `factory/logs/meta/asks.md`
- the current canonical local ask file says there are no active customer asks:
  `for now no asks exists.`
- the tracked maintainer workflow inputs remain sentinel-only under
  `factory/inputs/**`; live work items there are ignored operating state
- the local worktree is dirty before this refresh from tracked local edits in
  `factory/logs/meta/asks.md` and `factory/workers/workspace-setup/AGENTS.md`;
  treat those as existing local state, not as noise to revert

## workflow truth

- `factory/factory.json` defines five work types: `thoughts`, `idea`, `plan`,
  `task`, and `cron-triggers`
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
  `factory/inputs/idea/default/collapse-replay-safe-diagnostics-rehydration.md`
  is stale because its owning work merged through PR `#169`
- after pruning that stale residue, the next maintainer-owned ignored backlog
  slot should be a fresh standalone idea rather than a duplicate replay request

## customer-ask truth

- the local canonical ask file now withdraws the earlier checklist, coverage,
  simplification, and minimum-concurrency backlog for this cycle
- there is therefore no active customer-directed requirement right now to keep
  a minimum number of simultaneous lanes in flight
- open PRs can still inform overlap checks, but they no longer count as
  customer-required work unless the canonical ask file reintroduces them

## recent repo movement

- recent merged PRs on `main` now include:
  - `#170` `weird-work-names`
  - `#169` `collapse-replay-safe-diagnostics-rehydration`
  - `#166` `simplify-loaded-runtime-definition-lookups`
  - `#165` `localize-workflow-activity-graph-import-copy`
  - `#164` `localize-terminal-work-card-copy`
  - `#162` `localize-dashboard-flow-axis-legend-copy`
- `gh pr list --state open` on `2026-05-09` reports:
  - `#172` `same-trace`
  - `#171` `workflow-graph-padding`
  - `#168` `docs: refresh meta world state`
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
- the previously opened meta-refresh branches such as `PR #168` now reflect an
  older worldview than live `main`
- the replay diagnostics dedupe lane is closed on live `main` through merged
  `PR #169`, so it should not remain in the ignored inbox

## replay truth

- `factory/logs/agent-fails.json` and
  `factory/logs/agent-fails.replay.json` remain the checked-in replay sample
  pair described in `factory/README.md`
- the replay pair is still historical fixture coverage rather than an exact
  copy of the current workflow contract
- one replay rejection payload is still quoted oddly as `"\"<REJECTED>\"\n"`;
  treat that as fixture history rather than live workflow behavior

## next cleanup candidate

- the next maintainer-owned non-overlapping cleanup seam is service-smoke test
  helper dedupe:
  - `tests/functional/smoke/short_helpers_test.go` defines
    `simpleServicePipelineConfig` and `twoStageServicePipelineConfig`
  - `tests/functional/smoke/service_lifecycle_smoke_long_test.go` defines the
    same two helpers again for the `functionallong` lane
  - `tests/functional/smoke/service_config_override_alignment_test.go` already
    consumes the short-lane shared helper, so the duplicate long-lane owner is
    now the drift risk
  - this is a narrow simplification lane that removes duplicated fixture
    builders while preserving the observable service smoke behavior in both the
    default and `functionallong` test lanes
- that seam is not covered by open PRs `#141`, `#167`, `#168`, `#171`, or
  `#172`

## theory of mind

- the authoritative world model comes from live upstream git state, the
  checked-in workflow contract, the canonical ask file, current PR ownership,
  and direct code reads together
- when `factory/logs/meta/asks.md` changes locally, treat that edit as the
  immediate routing truth even if it withdraws a previously active backlog
- reason about `factory/inputs/**` in two layers:
  checked-in contract versus ignored operating state
- prune ignored local idea files once their owning PR merges; otherwise the
  local queue can preserve stale work that the live repo already finished
- treat delegated explorer suggestions as provisional after a fast-forward or
  merge; confirm the seam still exists on live `main` before re-queuing it
- when one functional test lane already exposes shared fixture builders, prefer
  reusing that owner across build-tag variants instead of keeping duplicated
  config definitions in long and short suites
