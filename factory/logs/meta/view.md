# meta view

## world state

- as of `2026-05-06T01:04:51.9722496-07:00`, local `HEAD` on `main` points to
  `6b95577`
  (`Merge pull request #111 from portpowered/remove-init-default-models`) and
  matches `origin/main`
- the canonical maintainer ask surface remains `factory/logs/meta/asks.md`
- the local worktree is not clean:
  - canonical `factory/inputs/**` remains tracked-sentinel-only
  - there is no checked-in cleanup request currently queued under
    `factory/inputs/**`
  - unrelated untracked local residue exists at the repo root and under
    `factory/` and should not be treated as canonical queue state

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
- there is no pre-existing ignored idea residue in the canonical inboxes at the
  start of this refresh; any new idea file written this turn is fresh local
  operating state rather than stale carry-over

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
  - website `90%` coverage target
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
  - `#111` `remove-init-default-models`, merged on `2026-05-06T07:09:23Z`
  - `#110` `workstation-request-current-selection-cleanup`, merged on
    `2026-05-06T03:28:00Z`
  - `#109` `inline-supporting-file-content-on-export-and-thin-factory-import`,
    merged on `2026-05-06T03:23:17Z`
  - `#108` `remove-deadcode-2026-may`, merged on `2026-05-06T03:05:09Z`
  - `#107` `Branch check`, merged on `2026-05-06T02:56:30Z`
  - `#106` `dedupe-functional-agent-config-and-arg-sequence-test-helpers`,
    merged on `2026-05-06T00:15:17Z`
  - `#105` `dedupe-functional-dispatch-history-test-helpers`, merged on
    `2026-05-05T23:16:03Z`
- `gh pr list --state open` currently reports one open PR:
  - `#112` `website-export`, opened on `2026-05-06T07:45:59Z`
- PR `#112` owns the current export/bundled-file work on its branch, so new
  cleanup dispatches should avoid portability/export overlap until that lane
  merges or closes

## next cleanup candidate

- there is no remaining narrow unowned customer-visible ask gap on `main`
- the next non-overlapping cleanup seam is in functional test helper
  duplication:
  - `tests/functional/internal/support/events.go` already owns
    `LastFactoryEventTick`
  - `tests/functional/replay_contracts/short_helpers_test.go`
  - `tests/functional/replay_contracts/replay_record_end_to_end_long_test.go`
  - `tests/functional/runtime_api/api_inference_events_test.go`
    still carry local `lastFactoryEventTick` copies
- the next dispatch should consolidate those suites onto the shared support
  helper without changing replay, runtime API, or projection behavior

## theory of mind

- the authoritative world model comes from live `main`, the checked-in workflow
  contract, and current PR state together; stale checked-in summaries are only
  safe after revalidation
- `factory/inputs/**` must always be reasoned about in two layers:
  checked-in contract versus ignored operating residue
- when the current branch is not `main`, refresh the worldview from live
  `main` before queueing cleanup work; branch-local open PRs can otherwise hide
  overlap
- deadcode-baseline output is only a candidate generator:
  build-tagged functional helpers must be checked in both default and
  `functionallong` lanes before treating them as dead
- when a shared functional support helper already exists, prefer collapsing
  local suite copies onto it instead of inventing another abstraction layer
