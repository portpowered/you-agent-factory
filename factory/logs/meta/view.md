# meta view

## world state

- as of `2026-05-09T11:02:43+09:00`, local `HEAD` on `main` points to
  `f0a6150` (`docs: refresh meta world state`), is ahead of `origin/main` by
  one local meta commit, and matches the head of open PR `#168`
  (`meta-refresh-world-state-20260509-100307`)
- the canonical maintainer ask surface remains `factory/logs/meta/asks.md`
- the checked-in maintainer workflow inputs remain tracked-sentinel-only under
  `factory/inputs/**`
- the live worktree is already dirty before this refresh from a user-owned
  tracked edit in `factory/logs/meta/asks.md`; treat that ask-file edit as the
  canonical local customer-routing truth for this cycle rather than as noise

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
- before this refresh, the checked-in inbox surface had no tracked live work
  items beyond `.gitkeep`, but ignored operating residue included one markdown
  cleanup request misplaced under `factory/inputs/plan/default/`
- after this refresh, that ignored cleanup request has been reconciled into the
  canonical standalone idea inbox under `factory/inputs/idea/default/`:
  `collapse-replay-safe-diagnostics-rehydration.md`

## customer-ask truth

- the current canonical local ask file now says there are no active customer
  asks: `for now no asks exists.`
- that local tracked ask-file edit supersedes the previously recorded quality,
  checklist, coverage, simplification, and minimum-concurrency backlog for this
  cycle
- there is therefore no active customer-directed requirement right now to keep
  three tasks in flight or to derive new checklist work from the external
  repository
- the earlier external-checklist lane remains historical context only until the
  canonical ask file reintroduces it or a maintainer document redirects it

## recent repo movement

- recent merged PRs on `main` now include:
  - `#166` `simplify-loaded-runtime-definition-lookups`
  - `#165` `localize-workflow-activity-graph-import-copy`
  - `#164` `localize-terminal-work-card-copy`
  - `#162` `localize-dashboard-flow-axis-legend-copy`
  - `#151` `localize-dashboard-header-timeline-and-stream-status`
  - `#150` `retire-template-fields-variadic-worktree-shim`
- `gh pr list --state open` on `2026-05-09` reports:
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

- `PR #141` still owns the repository-wide external checklist audit lane as an
  in-flight repo task, but that lane is no longer backed by an active canonical
  customer ask in the current local workspace
- `PR #167` owns the current downstream UI localization lane for
  `ui/src/features/work-outcome/*`
- `PR #168` owns the currently open meta-world refresh lane for the previous
  checked-in worldview
- the previously recorded dashboard-header localization seam is already closed
  on live `main` through merged `PR #151`
- the previously recorded template-field variadic worktree-shim seam is already
  closed on live `main` through merged `PR #150`
- no active canonical ask currently requires a minimum number of simultaneous
  lanes, so additional queueing should be driven by repo truth and cleanup
  value rather than by the withdrawn concurrency target

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
- that cleanup is already queued locally in the canonical ignored idea inbox,
  so this refresh does not need to create another overlapping request

## theory of mind

- the authoritative world model comes from live `main`, the checked-in
  workflow contract, the canonical ask file, current PR state, and the
  currently readable external checklist sources together
- when `factory/logs/meta/asks.md` changes locally, treat that edit as the
  immediate routing truth even if it withdraws a previously recorded backlog
- stale “next seam” notes can expire within hours once remote cleanup branches
  merge; re-check the concrete code and PR state before re-queuing an old lane
- reason about `factory/inputs/**` in two layers:
  checked-in contract versus ignored operating state
- when a standalone cleanup request exists as markdown, keep it in
  `factory/inputs/idea/default/`; a markdown file under `plan/default` is
  operating-state drift, not the canonical queue shape
- when a cleanup seam is already centralized in `pkg/interfaces` but one
  downstream subsystem still duplicates the inverse conversion locally, prefer
  collapsing that duplicate owner before inventing new abstractions
- when the external checklist repo exposes only some of the requested files,
  record the missing source as an evidence gap and continue from the verified
  documents instead of inferring the absent content
