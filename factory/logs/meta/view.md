# meta view

## world state

- as of `2026-05-02T21:01:24.4838148-07:00`, `HEAD` and `origin/main` both
  point to `c42c2ef` (`docs: refresh meta world state`); branch divergence is
  `0/0`
- the canonical maintainer ask surface is still
  `factory/logs/meta/asks.md`
- the current local ask text says this pass is meta-only: refresh
  `view.md` and `progress.txt`, do not submit new tasks
- the worktree is dirty outside this pass:
  - tracked local edit to `factory/logs/meta/asks.md`
  - tracked UI edits and helper splits across `ui/**`
  - untracked local files including `factory/logs/weird-number-summary.jsonl`

## workflow truth

- `factory/factory.json` defines this checked-in maintainer loop:
  `thoughts:init -> ideafy -> thoughts:complete`
  `idea:init -> plan -> idea:to-complete + plan:init`
  `plan:init -> setup-workspace -> plan:complete + task:init`
  `task:init -> process -> task:in-review -> review -> task:to-complete`
  `consume` completes same-name `idea` + `task` pairs after both reach
  `to-complete`
- the topology also includes:
  - hourly `cleaner` cron
  - `executor-loop-breaker` at `process` visit `50`
  - `review-loop-breaker` at `review` visit `10`
- `factory/README.md` and `docs/guides/batch-inputs.md` remain aligned with
  the checked-in workflow contract

## input surface truth

- tracked `factory/inputs/**` content is still sentinel-only:
  - `factory/inputs/BATCH/default/.gitkeep`
  - `factory/inputs/idea/default/.gitkeep`
  - `factory/inputs/plan/default/.gitkeep`
  - `factory/inputs/task/default/.gitkeep`
  - `factory/inputs/thoughts/default/.gitkeep`
- visible markdown under `factory/inputs/**` is local operating residue, not
  checked-in repo truth
- `.gitignore` still ignores `factory/inputs/**` except those sentinel paths

## recent repo movement

- recent merged cleanup PRs on `main`:
  - `#61` `browser-shared-action-primitives`, merged `2026-05-03T01:06:33Z`
  - `#62` `align-dashboard-work-summary-count-semantics`, merged
    `2026-05-02T21:37:06Z`
  - `#63` `retire-current-selection-inference-duplication`, merged
    `2026-05-02T22:38:08Z`
  - `#64` `retire-dashboard-bento-layout-ownership`, merged
    `2026-05-02T23:37:13Z`
  - `#65` `retire-dashboard-format-helper-ownership`, merged
    `2026-05-03T00:39:01Z`
- replay evidence is unchanged:
  - `process`: `9 ACCEPTED <COMPLETE>`, `27 CONTINUE <CONTINUE>`
  - `review`: `5 ACCEPTED <COMPLETE>`, `4 REJECTED <REJECTED>`

## theory of mind

- the authoritative world model comes from tracked workflow files, live git
  state, ignore rules, and the current canonical ask text
- `factory/inputs/**` must always be reasoned about in two layers:
  checked-in contract vs ignored local operating residue
- this pass should stay meta-only: refresh compact summaries, leave
  `factory/logs/meta/asks.md` untouched, and do not queue new work
