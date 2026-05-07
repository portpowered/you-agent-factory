# meta view

## world state

- as of `2026-05-06T16:03:38.4962364-07:00`, local `HEAD` on
  `meta-refresh-world-state-20260506-050415` points to `a25885a`
  (`docs: refresh meta world state`) and has been rebased onto live
  `origin/main` through `4f21b7b`
  (`cover-gocoveragecheck-command-owner-threshold-and-entrypoint-branches (#134)`)
- the canonical maintainer ask surface remains `factory/logs/meta/asks.md`
- the local worktree is not clean:
  - canonical `factory/inputs/**` remains tracked-sentinel-only
  - there is no checked-in cleanup request currently queued under
    `factory/inputs/**`
  - `factory/logs/meta/asks.md` carries a local tracked edit and should be
    treated as user-owned state for this refresh
  - tracked meta-log updates are required because the last checked-in summary
    predates merged PR `#134`
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
- the visible ignored local idea residue after rebasing onto live `main` was:
  - `factory/inputs/idea/default/cover-gocoveragecheck-command-owner-threshold-and-entrypoint-branches.md`
- that ignored idea is now stale queue residue rather than checked-in queue
  truth because merged PR `#134` already landed that exact cleanup on `main`
- it has been replaced during this refresh with one narrower customer-ask
  follow-up idea:
  - `factory/inputs/idea/default/cover-functionallane-command-owner-error-and-entrypoint-branches.md`

## customer-ask truth

- the canonical ask surface is now narrower than the last checked-in summary
  because the user-owned tracked edit in `factory/logs/meta/asks.md` has
  collapsed it to one active quality lane plus an autonomy notice
- the remaining active asks are broader program work rather than narrow
  customer-visible regressions:
  - follow the external website/backend checklist set and create alignment
    tasks
  - raise backend and website testing toward a declared `100%` minimum

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
  - `#134` `cover-gocoveragecheck-command-owner-threshold-and-entrypoint-branches`, merged on
    `2026-05-06T22:26:56Z`
  - `#133` `cover-releasetagcheck-git-tag-wrapper-branches`, merged on
    `2026-05-06T21:11:35Z`
  - `#132` `cover-deadcodecheck-command-owner-branches`, merged on
    `2026-05-06T20:27:02Z`
  - `#131` `close-backend-coverage-coverpkg-summary-tail-gap`, merged on
    `2026-05-06T19:11:57Z`
  - `#130` `close-backend-coverage-ok-summary-gap`, merged on
    `2026-05-06T18:10:41Z`
  - `#129` `cover-releaseprep-command-entrypoint`, merged on
    `2026-05-06T17:26:30Z`
  - `#128` `cover-releasesmoke-command-entrypoint`, merged on
    `2026-05-06T16:25:12Z`
  - `#127` `cover-releasetagcheck-command-entrypoint`, merged on
    `2026-05-06T15:17:38Z`
  - `#126` `cover-functionallane-command-entrypoint`, merged on
    `2026-05-06T14:18:07Z`
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
  code cleanup lane; `#123` remains the latest pushed refresh branch for this
  turn

## next cleanup candidate

- merged PR `#134` closes the previously recorded `cmd/gocoveragecheck`
  command-owner threshold and entrypoint seam on live `main`
- the next non-overlapping dispatch should keep advancing the broad quality ask
  by tightening another existing repo-owned maintainer gate instead of
  broadening into a package-by-package test campaign:
  - `Makefile` still routes the default functional lane through
    `go run ./cmd/functionallane -jobs $(FUNCTIONAL_DEFAULT_JOBS) -count=1 -timeout $(GO_TEST_TIMEOUT)`
  - `docs/processes/development-guide-relevant-files.md` still names
    `cmd/functionallane/` as the repo-owned default functional-lane owner
  - merged PR `#126` already covered the thin command-entrypoint routing seam
    for this package, but the same package still lacks direct coverage for the
    `main` error path, `failf` exit behavior, `run()` success path, and the
    discovery and test-execution failure wrappers
  - `go test -cover ./cmd/functionallane` now reports `82.0%` statement
    coverage, materially below the sibling maintainer-gate command packages
- the next idea should add focused package-local tests for
  `cmd/functionallane`'s remaining command-owner error and entrypoint branches
  without changing functional-lane package discovery policy, widening the lane
  scope, or pushing these assertions down into unrelated functional suites

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
- when a repo-owned coverage gate parses `go test` package summaries, account
  for both bare `pkg/path  coverage: ...` lines and `ok pkg/path ... coverage:
  ...` lines; backend packages can surface `0.0%` through either shape
- when `go test -coverpkg` appends `in <package list>` after
  `coverage: ... of statements`, treat that as the same package-summary shape
  rather than assuming simplified fixture lines match the live output exactly
- when one repo-owned command entrypoint gains a thin test seam to satisfy a
  coverage ask, inspect sibling repo-owned lane commands next before pushing
  equivalent coverage assertions down into unrelated downstream packages
- when a GitHub workflow shells through a repo-owned `cmd/` entrypoint, treat
  its output format and flag-routing behavior as command-owner seams even if
  helper packages beneath it already have unit tests
- when a root `Makefile` maintainer command still shells through a repo-owned
  `cmd/` entrypoint and the internal policy package already has behavioral
  tests, prefer adding command-local seam coverage there before widening the
  scope into release-process refactors
- when a root maintainer gate such as `make deadcode` still shells through a
  repo-owned `cmd/` entrypoint and the command test file only covers helper
  functions, treat the missing baseline-drift and failure-path coverage as the
  next narrow quality seam before inventing new repo-wide deadcode work
- when a repo-owned command-entrypoint PR closes the main routing seam, inspect
  the same package for any remaining `0.0%` subprocess-wrapper function before
  widening scope into a different command or a repo-wide coverage campaign
- when a repo-owned coverage gate already has parser and zero-coverage
  regression tests but still trails sibling maintainer commands on package
  coverage, prefer adding command-owner success and threshold-branch coverage
  there before widening scope into low-coverage application packages
- when a repo-owned functional-lane command already has its thin entrypoint
  seam covered, inspect the same package next for untested `main` error paths,
  `failf` exit behavior, and wrapped discovery or execution failures before
  widening scope into functional test rewrites or broader coverage programs
