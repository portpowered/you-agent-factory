# meta view

## world state

- as of `2026-05-06T03:05:05.3321512-07:00`, local `HEAD` on `main` points to
  `02643f8`
  (`Merge pull request #119 from portpowered/ralph/dedupe-functional-api-server-harnesses`)
  and matches `origin/main`
- the canonical maintainer ask surface remains `factory/logs/meta/asks.md`
- the local worktree is not clean:
  - canonical `factory/inputs/**` remains tracked-sentinel-only
  - there is no checked-in cleanup request currently queued under
    `factory/inputs/**`
  - `factory/logs/meta/asks.md` carries a local tracked edit and should be
    treated as user-owned state for this refresh
  - ignored local workflow residue under `factory/inputs/**` must still be
    treated as operating state rather than checked-in queue truth

## workflow truth

- `factory/factory.json` still defines five work types: `thoughts`, `idea`,
  `plan`, `task`, and `cron-triggers`
- the checked-in maintainer loop remains:
  `thoughts:init -> ideafy -> thoughts:complete`
  `idea:init -> plan -> idea:to-complete + plan:init`
  `plan:init -> setup-workspace -> plan:complete + task:init`
  `task:init -> process -> task:in-review -> review -> task:to-complete`
  `consume` completes same-name `idea` + `task` pairs once both reach
  `to-complete`
- topology details that still matter:
  - `process` and `review` run in `.claude/worktrees/{{name}}`
  - shared `executor-slot` capacity is `10`; each staffed workstation requests
    `1`
  - hourly `cleaner` emits `cron-triggers:complete`
  - `executor-loop-breaker` fails `task:init` after `process` visit `50`
  - `review-loop-breaker` fails `task:in-review` after `review` visit `10`

## input surface truth

- tracked `factory/inputs/**` content is still sentinel-only:
  - `factory/inputs/BATCH/default/.gitkeep`
  - `factory/inputs/idea/default/.gitkeep`
  - `factory/inputs/plan/default/.gitkeep`
  - `factory/inputs/task/default/.gitkeep`
  - `factory/inputs/thoughts/default/.gitkeep`
- `.gitignore` still ignores live workflow submissions under `factory/inputs/**`
  except those sentinel paths
- the file watcher still enforces the documented three-segment watched-input
  contract and no longer accepts direct
  `factory/inputs/<work-type>/<file>` submissions as an implicit `default`
  channel fallback
- stale ignored residue for merged tasks existed at the start of this refresh:
  - `factory/inputs/idea/default/retire-transition-topology-runtime-lookup-adapter.md`
  - `factory/inputs/task/default/consolidate-static-command-runner-test-helpers.md`
  - `factory/inputs/task/default/dedupe-functional-api-server-harnesses.md`
  - `factory/inputs/task/default/retire-transition-topology-runtime-lookup-adapter.md`
- those ignored files were local operating leftovers rather than checked-in
  queue truth and have been pruned during this refresh

## customer-ask truth

- the import/export P0 lane remains materially closed on `main` through merged
  PRs `#67`, `#68`, `#69`, `#70`, `#71`, `#72`, `#93`, and `#109`
- the selected-work current-selection ask is materially satisfied on `main`
  through merged PRs `#74`, `#77`, and `#110`
- the submit-work copy ask is satisfied on `main` through merged PR `#75`
- the header verbosity, chart layout, branding/iconography, and button-tone
  asks are materially satisfied on `main` through merged PRs `#83`, `#84`,
  `#85`, `#86`, `#87`, and `#98`
- the remaining open asks in `factory/logs/meta/asks.md` are broader program
  work rather than narrow customer-visible regressions:
  - standards-migration checklist tracking
  - backend and website `100%` coverage target
  - docs audit
  - manual QA
  - systems-quality documentation

## replay truth

- `factory/logs/agent-fails.json` and
  `factory/logs/agent-fails.replay.json` remain the checked-in replay sample
  pair described in `factory/README.md`
- the replay pair is still historical fixture coverage rather than an exact
  copy of the current workflow contract; it predates `to-complete`, `consume`,
  and the current `executor-slot` capacity of `10`
- replay outcome counts remain unchanged in the sample:
  - `process`: `9 ACCEPTED <COMPLETE>`, `27 CONTINUE <CONTINUE>`
  - `review`: `5 ACCEPTED <COMPLETE>`, `4 REJECTED <REJECTED>`
- one replay rejection payload is still oddly quoted as `"\"<REJECTED>\"\n"`;
  treat that as a fixture quirk rather than current workflow truth

## recent repo movement

- recent merged PRs on `main` now include:
  - `#119` `dedupe-functional-api-server-harnesses`, merged on
    `2026-05-06T09:27:38Z`
  - `#118` `retire-transition-topology-runtime-lookup-adapter`, merged on
    `2026-05-06T09:26:11Z`
  - `#117` `consolidate-static-command-runner-test-helpers`, merged on
    `2026-05-06T09:22:29Z`
  - `#115` `Import niceties`, merged on `2026-05-06T08:53:55Z`
  - `#114` `consolidate-functional-factory-event-tick-helpers`, merged on
    `2026-05-06T08:18:50Z`
  - `#113` `docs: refresh meta world state`, merged on
    `2026-05-06T08:08:40Z`
  - `#112` `updated website export to support exporting bundled files`, merged
    on `2026-05-06T07:45:59Z`
  - `#111` `remove-init-default-models`, merged on `2026-05-06T07:09:23Z`
- `gh pr list --state open` currently reports no open PRs
- there is no currently open PR owning the next helper-cleanup lane

## next cleanup candidate

- there is no remaining narrow unowned customer-visible ask gap on `main`
- the next non-overlapping cleanup seam is in runtime API functional helper
  duplication:
  - `tests/functional/internal/support/events.go` already owns
    `StringPointerValue` and `FactoryWorksValue`
  - `tests/functional/internal/support/harness.go` already owns
    `HasWorkTokenInPlace`
  - `tests/functional/runtime_api/api_batch_submission_boundary_smoke_test.go`
    still carries local `factoryWorksValue` and `factoryRelationsValue`
  - `tests/functional/runtime_api/api_service_mode_observability_smoke_test.go`
    still carries local `stringValue` and `hasWorkTokenInPlace`
- the next dispatch should consolidate those runtime API suites onto the shared
  support helpers, adding a shared `FactoryRelationsValue` sibling only if
  needed, without changing runtime API behavior

## theory of mind

- the authoritative world model comes from live `main`, the checked-in workflow
  contract, and current PR state together; stale checked-in summaries are only
  safe after revalidation
- `factory/inputs/**` must always be reasoned about in two layers:
  checked-in contract versus ignored operating residue
- when the current branch is not `main`, refresh the worldview from live
  `main` before queueing cleanup work; branch-local open PRs can otherwise hide
  overlap
- after a helper-dedupe PR merges, inspect adjacent suites in the same package
  for smaller leftover nil-unwrapper or token-search clones before inventing a
  new helper owner
- deadcode-baseline output is only a candidate generator:
  build-tagged functional helpers must be checked in both default and
  `functionallong` lanes before treating them as dead
- when a shared functional support helper already exists, prefer collapsing
  local suite copies onto it instead of inventing another abstraction layer
