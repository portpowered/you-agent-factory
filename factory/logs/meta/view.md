# meta view

## world state

- as of `2026-05-03T04:03:50.3625396-07:00`, `HEAD` and `origin/main` both
  point to `7c98bd7` (`update the coverage to not generated`); branch
  divergence is `0/0`
- the canonical maintainer ask surface remains
  `factory/logs/meta/asks.md`; the current checked-in ask text is still
  meta-only and explicitly says not to submit new tasks
- the worktree is dirty outside this pass:
  - tracked local edit to `factory/logs/meta/asks.md`
  - tracked local backend edits under `pkg/factory/projections/**`
  - tracked local UI edits under `ui/src/**`
  - untracked local files including `button-colors.md`,
    `factory/logs/weird-number-summary.jsonl`,
    `pkg/factory/projections/testdata/`, and `test-release.jsonl`

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
- visible files or folders under `factory/inputs/**`, including the local
  `factory/inputs/tasks/default` typo path, are ignored operating residue and
  not checked-in repo truth
- `.gitignore` still ignores `factory/inputs/**` except the canonical sentinel
  paths above
- the public checked-in maintainer contract uses canonical inboxes
  `BATCH`, `idea`, `plan`, `task`, and `thoughts`, but older `story` or
  `tasks` wording still exists in some repo docs and CLI help
- the watcher accepts a slightly wider path surface than the public docs show:
  files directly under `factory/inputs/<work-type>/` are treated as the
  default channel even though the docs emphasize the
  `factory/inputs/<work_type-or-BATCH>/<channel>/<filename>` shape

## replay truth

- `factory/logs/agent-fails.json` and
  `factory/logs/agent-fails.replay.json` remain the checked-in replay sample
  pair described in `factory/README.md`
- the replay pair is historical fixture coverage, not an exact copy of the
  current checked-in workflow contract: the sample still reflects the older
  topology without `to-complete` hold states, `consume`, or the current
  `executor-slot` capacity of `10`
- replay outcome counts remain unchanged in the sample:
  - `process`: `9 ACCEPTED <COMPLETE>`, `27 CONTINUE <CONTINUE>`
  - `review`: `5 ACCEPTED <COMPLETE>`, `4 REJECTED <REJECTED>`

## recent repo movement

- recent merged cleanup PRs on `main` are now:
  - `#65` `retire-dashboard-format-helper-ownership`
  - `#64` `retire-dashboard-bento-layout-ownership`
  - `#63` `retire-current-selection-inference-duplication`
  - `#62` `align-dashboard-work-summary-count-semantics`
  - `#61` `browser-shared-action-primitives`
  - `#60` `browser-integration-png-export-import-roundtrip`

## theory of mind

- the authoritative world model must come from live git state plus the
  checked-in workflow contract, not from the replay sample alone
- `factory/inputs/**` must always be reasoned about in two layers:
  checked-in contract vs ignored local operating residue
- the checked-in ask still forbids queueing follow-up work, so this pass should
  refresh meta state only and leave `factory/logs/meta/asks.md` untouched
