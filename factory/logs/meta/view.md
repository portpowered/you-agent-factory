# meta view

## world state

- as of `2026-05-03T05:04:02.2575711-07:00`, `HEAD` on `fixlines` points to
  `ce9872f` (`add fixes for edges missing`) while `origin/main` points to
  `ec20d4c` (`docs: refresh meta world state`); branch divergence versus
  `origin/main` is `1/0`
- `origin/fixlines` matches `HEAD`, and the worktree is currently clean
- the canonical maintainer ask surface remains `factory/logs/meta/asks.md`;
  unlike the prior pass, the checked-in ask is active and no longer limits this
  loop to meta-only refresh

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
- visible folders under `factory/inputs/**`, including the local
  `factory/inputs/tasks/default` typo path, are ignored operating residue and
  not checked-in repo truth
- `.gitignore` still ignores `factory/inputs/**` except the canonical sentinel
  paths above
- the watcher still accepts files directly under `factory/inputs/<work-type>/`
  as the default channel even though the public docs emphasize
  `factory/inputs/<work_type-or-BATCH>/<channel>/<filename>`

## customer-ask truth

- the customer’s P0 throttle ask is no longer hypothetical: the repo already
  contains a root-level `FactoryGuardConfig` with `modelProvider`, optional
  `model`, and `refreshWindow`, plus runtime lowering into
  `pkg/petri/inference_throttle_guard.go`
- recent merged throttle cleanup lineage on `main` includes:
  - `#48` `retire-legacy-throttle-fallback-after-authored-guard`
  - `#46` `factory-level-inference-throttle-guard`
  - `#42` `retire-dispatcher-throttle-pause-map`
  - `#31` `derive-throttle-windows-from-completed-dispatch-history`
- the remaining highest-signal throttle legacy seam is
  `InferenceThrottleGuard.WatchedTransitionIDs`, which still preserves a
  transition-ID fallback path when runtime worker/provider lookup misses
- no dedicated checked-in standards-alignment checklist exists yet for
  `STD-016` / `STD-017`; the current durable tracking surface for that ask is
  still `factory/logs/meta/progress.txt`

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
- the current branch also carries one unmerged UI/test commit ahead of `main`:
  `ce9872f` `add fixes for edges missing`

## theory of mind

- the authoritative world model must come from live git state plus the
  checked-in workflow contract, not from the replay sample alone
- `factory/inputs/**` must always be reasoned about in two layers:
  checked-in contract vs ignored local operating residue
- the authored inference throttle guard design mostly matches the current
  customer ask already, so the right next move is cleanup of the remaining
  fallback seam rather than another broad throttle redesign
- the current standards-quality ask needs explicit checklist tracking in meta
  progress until the repo gains a dedicated checked-in standards surface
