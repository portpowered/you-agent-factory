# meta view

## world state

- as of `2026-05-02T19:01:28.0034145-07:00`, `origin/main` is at `4583faf`
  (`Merge pull request #61 from portpowered/ralph/browser-shared-action-primitives`)
- local `main` is currently diverged from `origin/main`:
  - `HEAD` is `bef1664` (`update the logs`)
  - branch status is `ahead 2, behind 6`
  - `git pull --ff-only` failed because the branch is not fast-forwardable
- the canonical checked-in maintainer ask surface remains
  `factory/logs/meta/asks.md`
- the current ask guidance still says to clean up meta state only and not
  submit new tasks on this pass
- replay evidence remains stable:
  - `process`: `9 ACCEPTED <COMPLETE>`, `27 CONTINUE <CONTINUE>`
  - `review`: `5 ACCEPTED <COMPLETE>`, `4 REJECTED <REJECTED>`

## repo truth that changed since the last summary

- the old `.gitkeep`-only workflow-input summary is no longer true
- tracked checked-in workflow inputs now include real queue artifacts under:
  - `factory/inputs/idea/default/*.md`
  - `factory/inputs/plan/default/*.md`
  - `factory/inputs/task/default/*.md`
- recent maintainer-request lanes are already merged on `origin/main`:
  - PR `#62` `align-dashboard-work-summary-count-semantics`
  - PR `#63` `retire-current-selection-inference-duplication`
  - PR `#64` `retire-dashboard-bento-layout-ownership`
  - PR `#65` `retire-dashboard-format-helper-ownership`
  - PR `#61` `browser-shared-action-primitives`
- there is no longer a live open-PR blocker in the old `#61` file set because
  `#61` merged at `2026-05-03T01:06:33Z`

## local workspace reality

- `factory/logs/meta/asks.md` is already locally modified and should remain
  untouched on this pass
- `factory/logs/weird-number-summary.jsonl` remains untracked local evidence
- the worktree also contains broader local UI edits that make the branch dirty:
  - `ui/src/components/dashboard/bento.tsx`
  - `ui/src/components/dashboard/button.test.tsx`
  - `ui/src/components/dashboard/classnames.ts`
  - `ui/src/components/dashboard/formatters.ts`
  - `ui/src/components/dashboard/index.ts`
  - `ui/src/components/dashboard/mutation-dialog.test.tsx`
  - `ui/src/components/dashboard/mutation-dialog.tsx`
  - `ui/src/components/dashboard/tick-slider-control.tsx`
  - `ui/src/components/dashboard/visualization-dependencies.test.tsx`
  - `ui/src/components/dashboard/widget-board.tsx`
  - `ui/src/features/header/tick-slider-control.tsx`
  - `ui/src/features/workflow-activity/mutation-dialog.test.tsx`
  - `ui/src/features/workflow-activity/mutation-dialog.tsx`

## theory of mind

- the checked-in maintainer truth comes from tracked files, git history, and
  merged PR state, not from older compact summaries
- `factory/inputs/**` is an active checked-in workflow surface in this repo,
  not just empty starter scaffolding
- the highest-value action for this pass is to repair stale maintainer context
  so future passes do not reason from false branch state, false PR state, or
  false queue-state assumptions
- because the canonical ask surface explicitly says not to submit new tasks,
  queueing a new cleanup idea would be misaligned even though new cleanup
  opportunities likely exist

## next best move

- keep `factory/logs/meta/view.md` and `factory/logs/meta/progress.txt`
  accurate and compact
- leave `factory/logs/meta/asks.md` unchanged
- do not submit new work under `factory/inputs/**` on this pass
- if branch reconciliation is attempted later, account for the existing dirty
  UI worktree before merging `origin/main`
