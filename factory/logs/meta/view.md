# meta view

## world state

- as of `2026-05-03T19:02:46.3958525-07:00`, local `HEAD` on `main` points to
  `4b945a7`
  (`retire-filewatcher-default-channel-fallback (#79)`) and matches
  `origin/main`
- the local worktree is not clean:
  - tracked local edits exist in `factory/logs/meta/asks.md` and
    `factory/workstations/cleaner/AGENTS.md`
  - ignored local workflow residue exists under `factory/inputs/**`
- the canonical maintainer ask surface remains `factory/logs/meta/asks.md`

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
- the current checkout contains ignored operating residue:
  - `factory/inputs/idea/default/align-default-starter-task-input-contract.md`
  - `factory/inputs/idea/default/workstation-non-success-route-arrays.md`
  - an empty local `factory/inputs/tasks/default/` directory
- the file watcher now enforces the documented three-segment watched-input
  contract and no longer accepts direct
  `factory/inputs/<work-type>/<file>` submissions as an implicit `default`
  channel fallback

## customer-ask truth

- the import/export P0 has materially advanced on `main` through merged PRs
  `#67`, `#68`, `#70`, `#71`, `#72`, `#75`, `#76`, `#77`, and now `#78`
- the remaining active ask-owned lane is still PR `#69`
  `workstation-non-success-route-arrays`
- on `main`, the route-array ask is still real across schema, config mapping,
  replay/public projection, and UI factory-definition import surfaces
- PR `#69` continues to own that lane functionally, but it is not yet
  operationally closed:
  - `gh pr checks 69` is still red
  - the branch leaves an unused helper in `pkg/config/layout.go`
  - maintained reference surfaces and fixtures on that branch still teach or
    serialize the retired singular route shape in `docs/workstations.md`,
    `ui/integration/fixtures/event-stream-replay.jsonl`, and
    `factory/logs/agent-fails.replay.json`
- that means the route-array ask is owned, but not yet review-ready, so a
  second overlapping dispatch for the same lane would still be duplicate work
- the selected-work current-selection ask is materially satisfied on `main`
  through merged PRs `#74` and `#77`

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

## recent repo movement

- recent merged PRs on `main` now include:
  - `#79` `retire-filewatcher-default-channel-fallback`, merged on
    `2026-05-04T01:27:11Z`
  - `#78` `remove-list-work-legacy-pagination-shim`, merged on
    `2026-05-04T00:28:40Z`
  - `#77`
    `align-current-selection-relationship-graph-and-dispatch-attempt-details`,
    merged on `2026-05-03T23:43:30Z`
  - `#76`
    `finish-import-preview-dialog-extraction-from-workflow-activity`, merged on
    `2026-05-03T22:25:03Z`
  - `#75` `trim-submit-work-card-intro-copy`, merged on `2026-05-03T22:12:16Z`
  - `#74` `simplify-current-selection-dispatch-detail-surface`, merged on
    `2026-05-03T19:37:13Z`
- the only open PR directly tied to an active ask lane is still:
  - `#69` `workstation-non-success-route-arrays`, opened on `2026-05-03`

## theory of mind

- the authoritative world model still comes from live git state plus the
  checked-in workflow contract, not from replay fixtures alone
- `factory/inputs/**` must always be reasoned about in two layers:
  checked-in contract versus ignored operating residue
- helper planning residue can go stale within one merge cycle, so the meta loop
  has to reconcile ignored backlog files against `main` and open PR state
  before dispatching anything new
- merged PR `#79` consumed the older filewatcher fallback cleanup idea, so
  that ignored idea file became stale residue and should be pruned
- PR ownership and ask completeness are different checks: PR `#69` owns the
  route-array lane, but failing checks plus stale maintained examples still mean
  the ask is not closed
- once the remaining P0 route-array lane is already owned, the best safe
  dispatch is a narrow cleanup outside `#69`
- the next safe non-overlapping cleanup seam is the singular/plural default
  starter contract drift: the checked-in maintainer workflow uses `task`, while
  the public starter/default scaffold, CLI help, and related tests still teach
  `tasks`
