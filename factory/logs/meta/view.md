# meta view

## world state

- as of `2026-05-06T07:04:44.5861978-07:00`, local `HEAD` on
  `meta-refresh-world-state-20260506-050415` points to `87324ab`
  (`docs: refresh meta world state`) and has been rebased onto live
  `origin/main` through `d3785b4`
  (`Merge pull request #125 from portpowered/ralph/close-backend-coverage-profile-gap`)
- the canonical maintainer ask surface remains `factory/logs/meta/asks.md`
- the local worktree is not clean:
  - canonical `factory/inputs/**` remains tracked-sentinel-only
  - there is no checked-in cleanup request currently queued under
    `factory/inputs/**`
  - `factory/logs/meta/asks.md` carries a local tracked edit and should be
    treated as user-owned state for this refresh
  - tracked meta-log updates are required because the last checked-in summary
    predates merged PR `#125`
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
- the visible ignored local idea residue at the start of this refresh was:
  - `factory/inputs/idea/default/close-backend-coverage-profile-gap.md`
- that ignored idea was stale queue residue rather than checked-in queue truth
  because merged PR `#125` already landed that backend coverage profile-gap
  cleanup on `main`
- it has been replaced during this refresh with one narrower customer-ask
  follow-up idea:
  - `factory/inputs/idea/default/cover-functionallane-command-entrypoint.md`

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
  - backend and website `100%` coverage target plus stronger test enforcement
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
  - `#125` `close-backend-coverage-profile-gap`, merged on
    `2026-05-06T13:46:50Z`
  - `#124` `add-backend-zero-coverage-package-gate`, merged on
    `2026-05-06T12:39:23Z`
  - `#122` `collapse-runtime-api-functional-server-lifecycle-owner`, merged on
    `2026-05-06T11:29:08Z`
  - `#121` `consolidate-runtime-api-functional-support-helpers`, merged on
    `2026-05-06T10:20:03Z`
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
- `gh pr list --state open` currently reports two open PRs:
  - `#123` `docs: refresh meta world state`
  - `#120` `docs: refresh meta world state`
- PRs `#120` and `#123` are meta-log refresh branches and do not own the next
  code cleanup lane; `#123` is the latest pushed refresh branch for this turn

## next cleanup candidate

- there is no remaining narrow unowned customer-visible ask gap on `main`
- merged PR `#125` materially closes the previously recorded backend
  coverage-profile loophole on `main`:
  - `cmd/gocoveragecheck/main.go` now reads package-level `0.0%` coverage from
    real `go test` summary output in addition to the parsed coverage profile
  - `cmd/gocoveragecheck/main_test.go` now covers both present-in-profile and
    missing-from-profile backend zero-coverage cases plus excluded packages
  - `cmd/factory/main.go` and `cmd/factory/main_test.go` now keep the thin CLI
    entrypoint directly testable so that repo-owned backend coverage can count
    the command owner instead of pushing entrypoint assertions into unrelated
    package tests
- the next non-overlapping dispatch should keep advancing the broad P0 testing
  ask through adjacent repo-owned command surfaces instead of broadening into a
  package-by-package coverage campaign:
  - `Makefile` still routes `test-functional` through
    `go run ./cmd/functionallane`
  - `cmd/functionallane/main.go` owns functional package discovery,
    `internal/support` filtering, config parsing, and the final `go test`
    command invocation for that repo-owned lane
  - there is still no checked-in `cmd/functionallane/main_test.go`
  - the repo just established the narrower command-entrypoint pattern in
    `cmd/factory`, so `cmd/functionallane` is the nearest sibling seam in the
    same quality lane
- the next idea should make `cmd/functionallane` directly testable with focused
  command-owner coverage, without replacing `make test-functional`, changing
  package selection semantics, or broadening into new functional scenarios

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
- when a shared functional support server lifecycle owner already exists, treat
  remaining package-local test bootstrappers as cleanup seams even if they
  still keep package-specific request helpers
- when a broad quality or coverage ask is open, prefer tightening an existing
  repo-owned enforcement seam before queueing a repo-wide test-authoring
  program
- when one repo-owned command entrypoint gains a thin test seam to satisfy a
  coverage ask, inspect sibling repo-owned lane commands next before pushing
  equivalent coverage assertions down into unrelated downstream packages
