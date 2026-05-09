# meta view

## world state

- as of `2026-05-09T17:04:00+09:00`, live `origin/main` points at merged
  `PR #177` commit `ca69cbc`, which closed the replay-contract tagged-helper
  cleanup after merged `PR #176` and merged `PR #174`
- local `main`, `meta-refresh-world-state-20260509-160349`, and
  `fix-gocoveragecheck-zero-coverage-report-gap` currently point at the same
  local meta-refresh stack, while `origin/main` remains at `ca69cbc`
- the canonical maintainer ask surface remains `factory/logs/meta/asks.md`
- the canonical ask file on live `main` is active again; it currently asks for
  external-checklist conformance work, stronger backend and website coverage,
  and ongoing code simplification
- the tracked maintainer workflow inputs remain sentinel-only under
  `factory/inputs/**`; live work items there are ignored operating state

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
- the ignored replay-helper idea
  `factory/inputs/idea/default/dedupe-replay-contract-tagged-helpers.md` is
  stale because merged `PR #177` landed that exact lane on `main`
- the visible local ignored idea surface still contains one unrelated PRD-style
  residue:
  `factory/inputs/idea/default/website-edit-running-factory-workstations.md`
- the visible local ignored idea surface also includes one active
  maintainer-owned cleanup request already advanced into an open worker lane:
  `factory/inputs/idea/default/fix-gocoveragecheck-zero-coverage-report-gap.md`

## customer-ask truth

- the active canonical ask backlog currently includes:
  - repository conformance work against the linked website and backend
    checklists
  - backend functional coverage toward `90%` of non-generated `pkg/**`
  - website test coverage toward `90%` of non-generated `ui/src/**`
  - ongoing simplification, dead-code removal, and duplication cleanup
- there is no current instruction in the live ask file to keep a minimum
  number of simultaneous cleanup lanes in flight

## recent repo movement

- recent merged PRs on `main` now include:
  - `#177` `dedupe-replay-contract-tagged-helpers`
  - `#176` `split-bootstrap-portability-functionallong-helpers`
  - `#174` `dedupe-service-smoke-pipeline-config-builders`
  - `#170` `weird-work-names`
  - `#169` `collapse-replay-safe-diagnostics-rehydration`
  - `#166` `simplify-loaded-runtime-definition-lookups`
- `gh pr list --state open` on `2026-05-09` now reports:
  - `#179` `fix-gocoveragecheck-zero-coverage-report-gap`
  - `#178` `docs: refresh meta world state`
  - `#175` `docs: refresh meta world state`
  - `#173` `docs: refresh meta world state`
  - `#172` `same-trace`
  - `#171` `workflow-graph-padding`
  - `#167` `localize-work-outcome-trend-cards-copy`
  - `#163`, `#152`, `#145`, `#143`, `#139`, `#123`, `#120`
    `docs: refresh meta world state`
  - `#141` `audit-repository-against-2026-website-and-backend-checklists`

## open-lane truth

- `PR #141` owns the repository-wide external checklist audit lane and also
  touches the meta-doc pair, so it is not isolated from worldview updates
- `PR #179` owns the live `cmd/gocoveragecheck` zero-coverage-gap lane
- `PR #167` owns the current `ui/src/features/work-outcome/*` localization lane
- `PR #171` owns the dashboard-shell and workflow-graph padding lane
- `PR #172` owns the same-trace guard lane across config, petri, API, and
  functional coverage
- `PR #178` is now the freshest open meta-refresh branch; the older open
  meta-refresh PRs are stale duplicates on the same file pair
- the replay-helper lane is closed on live `main` through merged `PR #177`
- the bootstrap-portability helper split lane is closed on live `main` through
  merged `PR #176`
- the smoke-helper dedupe lane is closed on live `main` through merged
  `PR #174`

## replay truth

- `factory/logs/agent-fails.json` and
  `factory/logs/agent-fails.replay.json` remain the checked-in replay sample
  pair described in `factory/README.md`
- the replay pair is still historical fixture coverage rather than an exact
  copy of the current workflow contract
- one replay rejection payload is still quoted oddly as `"\"<REJECTED>\"\n"`;
  treat that as fixture history rather than live workflow behavior

## current maintainer decision

- this cycle does not queue a new cleanup request
- reason:
  - the active customer asks explicitly prioritize stronger backend coverage
    evidence and code quality simplification
  - a live `go run ./cmd/gocoveragecheck -min 80 -timeout 300s` run on
    `2026-05-09` printed `0.0%` coverage for
    `pkg/apisurface`, `pkg/buffers`, and `pkg/cli/default`
    but still exited successfully with `Go coverage 86.6% meets minimum 80.0%`
  - that means the repo-owned backend coverage gate is currently overstating
    quality and can let zero-coverage backend packages pass the customer-facing
    lane
  - the exact seam is narrow and local to `cmd/gocoveragecheck` plus its tests
  - that seam is already owned by open `PR #179`, which changes only
    `cmd/gocoveragecheck/main.go` and `cmd/gocoveragecheck/main_test.go`
  - creating another idea over the same lane would duplicate in-flight work
    instead of improving the queue

## theory of mind

- the authoritative world model comes from live upstream git state, the
  checked-in workflow contract, the canonical ask file, current PR ownership,
  ignored queue residue, and direct command/code reads together
- stale local summaries do not override live `main`; when the canonical ask
  file on upstream reintroduces backlog work, treat older “no asks” notes as
  invalid immediately
- reason about `factory/inputs/**` in two layers:
  checked-in contract versus ignored operating state
- prune ignored local idea files once their owning PR merges; otherwise the
  canonical inbox can preserve stale work that the live repo already finished
- treat delegated explorer suggestions as hypotheses; re-verify them against
  live `main` before dispatching new cleanup work because recent merges can
  invalidate an otherwise plausible seam within the same cycle
- when the repo-owned coverage command prints backend packages at `0.0%` while
  still exiting successfully, prefer tightening that gate before queueing a
  broader package-by-package test-authoring campaign
- when `go test -coverpkg` summary output says a backend-owned package is at
  `0.0%`, treat that as package-local zero coverage even if the aggregate
  profile shows transitive hits from other packages' tests
