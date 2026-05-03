# meta view

## world state

- as of `2026-05-02T22:01:36.2540358-07:00`, `HEAD` and `origin/main` both
  point to `bda83e1` (`docs: compact meta state summaries`); branch
  divergence is `0/0`
- the canonical maintainer ask surface remains
  `factory/logs/meta/asks.md`
- the current checked-in ask text keeps this pass meta-only: refresh
  `view.md` and `progress.txt`, do not submit new tasks
- the worktree is dirty outside this pass:
  - tracked local edit to `factory/logs/meta/asks.md`
  - tracked and untracked UI helper refactors across `ui/**`
  - untracked local file `factory/logs/weird-number-summary.jsonl`

## workflow truth

- `factory/factory.json` defines five work types: `thoughts`, `idea`, `plan`,
  `task`, and `cron-triggers`
- the checked-in maintainer loop is:
  `thoughts:init -> ideafy -> thoughts:complete`
  `idea:init -> plan -> idea:to-complete + plan:init`
  `plan:init -> setup-workspace -> plan:complete + task:init`
  `task:init -> process -> task:in-review -> review -> task:to-complete`
  `consume` completes same-name `idea` + `task` pairs once both reach
  `to-complete`
- easy-to-miss topology facts that still matter:
  - `process` and `review` run in `.claude/worktrees/{{name}}`
  - shared `executor-slot` capacity is `10`; each workstation requests `1`
  - hourly `cleaner` cron emits `cron-triggers:complete`
  - `executor-loop-breaker` fails `task:init` after `process` visit `50`
  - `review-loop-breaker` fails `task:in-review` after `review` visit `10`
- `factory/README.md` and `docs/guides/batch-inputs.md` still match the
  checked-in workflow contract

## input surface truth

- tracked `factory/inputs/**` content is still sentinel-only:
  - `factory/inputs/BATCH/default/.gitkeep`
  - `factory/inputs/idea/default/.gitkeep`
  - `factory/inputs/plan/default/.gitkeep`
  - `factory/inputs/task/default/.gitkeep`
  - `factory/inputs/thoughts/default/.gitkeep`
- visible files under `factory/inputs/**` are ignored local operating residue,
  not checked-in repo truth
- `.gitignore` still ignores `factory/inputs/**` except those sentinel paths

## recent repo movement

- recent merged cleanup PRs on `main`:
  - `#61` `browser-shared-action-primitives`
  - `#65` `retire-dashboard-format-helper-ownership`
  - `#64` `retire-dashboard-bento-layout-ownership`
  - `#63` `retire-current-selection-inference-duplication`
  - `#62` `align-dashboard-work-summary-count-semantics`
- replay evidence is unchanged:
  - `process`: `9 ACCEPTED <COMPLETE>`, `27 CONTINUE <CONTINUE>`
  - `review`: `5 ACCEPTED <COMPLETE>`, `4 REJECTED <REJECTED>`

## theory of mind

- the authoritative world model comes from tracked workflow files, live git
  state, ignore rules, and the current canonical ask text
- `factory/inputs/**` must always be reasoned about in two layers:
  checked-in contract vs ignored local operating residue
- this pass remains meta-only: refresh compact summaries, leave
  `factory/logs/meta/asks.md` untouched, and do not queue new work
