# meta view

## world state

- as of `2026-05-09T10:03:07+09:00`, local `HEAD` on `main` points to
  `3327a97` (`Merge pull request #166 from
  portpowered/ralph/simplify-loaded-runtime-definition-lookups`) and matches
  `origin/main`
- the canonical maintainer ask surface remains `factory/logs/meta/asks.md`
- the checked-in maintainer workflow inputs remain tracked-sentinel-only under
  `factory/inputs/**`
- the live worktree is already dirty outside this meta refresh from unrelated
  local edits in OpenAPI, generated API artifacts, worker/provider code, plus
  untracked local planning or script files; those are not maintainer-owned for
  this cycle

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
- before this refresh, the checked-in inbox surface had no tracked or ignored
  live work items beyond `.gitkeep`
- after this refresh, one new ignored maintainer idea is queued locally under
  `factory/inputs/idea/default/` for a non-overlapping backend cleanup seam:
  `collapse-replay-safe-diagnostics-rehydration.md`

## customer-ask truth

- the canonical ask file still carries one broad active quality lane plus the
  autonomy notice through `2026-05-25`
- the active quality asks remain:
  - follow the external website and backend checklists and create alignment
    tasks
  - keep backend and website testing moving toward declared high-coverage goals
  - keep simplifying backend and website ownership where duplicate or stale
    logic remains
  - keep at least three non-overlapping tasks running at a time
- the linked external checklist repository still exposes the same top-level
  checklist files on `main` on `2026-05-09`:
  - `backend-development-checklist.md`
  - `website-development-checklist.md`
- the linked external `asks.md` source still has an evidence gap on
  `2026-05-09`: the GitHub `asks.md` page does not resolve to a readable
  checklist document, so it remains unavailable evidence rather than usable
  customer input

## recent repo movement

- recent merged PRs on `main` now include:
  - `#166` `simplify-loaded-runtime-definition-lookups`
  - `#165` `localize-workflow-activity-graph-import-copy`
  - `#164` `localize-terminal-work-card-copy`
  - `#162` `localize-dashboard-flow-axis-legend-copy`
  - `#151` `localize-dashboard-header-timeline-and-stream-status`
  - `#150` `retire-template-fields-variadic-worktree-shim`
- `gh pr list --state open` on `2026-05-09` reports:
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

- `PR #141` still owns the repository-wide external checklist audit lane and
  should be treated as the active umbrella evidence task rather than a product
  code cleanup branch
- `PR #167` owns the current downstream UI localization lane for
  `ui/src/features/work-outcome/*`
- the previously recorded dashboard-header localization seam is already closed
  on live `main` through merged `PR #151`
- the previously recorded template-field variadic worktree-shim seam is already
  closed on live `main` through merged `PR #150`
- with `#141` and `#167` still open, one additional non-overlapping queued idea
  is enough to satisfy the standing ask to keep at least three tasks in flight

## replay truth

- `factory/logs/agent-fails.json` and
  `factory/logs/agent-fails.replay.json` remain the checked-in replay sample
  pair described in `factory/README.md`
- the replay pair is still historical fixture coverage rather than an exact
  copy of the current workflow contract
- one replay rejection payload is still quoted oddly as `"\"<REJECTED>\"\n"`;
  treat that as fixture history rather than live workflow behavior

## next cleanup candidate

- the next maintainer-owned non-overlapping cleanup seam is replay diagnostics
  dedupe:
  - `pkg/interfaces/safe_diagnostics.go` already owns canonical conversions for
    worker-internal diagnostics, generated safe diagnostics, provider-session
    metadata, and provider-failure metadata
  - `pkg/replay/event_reducer.go` still rebuilds
    `interfaces.WorkDiagnostics` through replay-local helpers
    `workDiagnosticsFromSafe`, `renderedPromptDiagnosticFromSafe`, and
    `providerDiagnosticFromSafe` immediately after decoding the same safe
    boundary through `interfaces.SafeWorkDiagnosticsFromGenerated(...)`
  - the live replay consumer path still needs `interfaces.WorkDiagnostics`, but
    it does not need those conversions to remain replay-local
  - this is a narrow simplification lane that removes duplicate conversion
    ownership without changing generated contracts, API behavior, or customer
    visible runtime output

## theory of mind

- the authoritative world model comes from live `main`, the checked-in
  workflow contract, the canonical ask file, current PR state, and the
  currently readable external checklist sources together
- stale “next seam” notes can expire within hours once remote cleanup branches
  merge; re-check the concrete code and PR state before re-queuing an old lane
- reason about `factory/inputs/**` in two layers:
  checked-in contract versus ignored operating state
- when the standing ask requires at least three non-overlapping tasks, count
  open product or audit PRs that still own active lanes, then queue only enough
  new ignored ideas to restore the minimum
- when a cleanup seam is already centralized in `pkg/interfaces` but one
  downstream subsystem still duplicates the inverse conversion locally, prefer
  collapsing that duplicate owner before inventing new abstractions
- when the external checklist repo exposes only some of the requested files,
  record the missing source as an evidence gap and continue from the verified
  documents instead of inferring the absent content
