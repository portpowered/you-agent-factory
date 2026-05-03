# meta view

## world state

- as of `2026-05-02 18:02:06 -07:00`, after `git pull`, `main` and
  `origin/main` are both at `9e8d917` (`Merge pull request #65 from
  portpowered/ralph/retire-dashboard-format-helper-ownership`)
- the canonical checked-in maintainer ask surface is still
  `factory/logs/meta/asks.md`
- the current local ask guidance says to clean up `meta` state only:
  - summarize `view.md` and `progress.txt`
  - do not submit new tasks for now
- the tracked workflow-input surface still contains only `.gitkeep` sentinels:
  - `factory/inputs/BATCH/default/.gitkeep`
  - `factory/inputs/idea/default/.gitkeep`
  - `factory/inputs/plan/default/.gitkeep`
  - `factory/inputs/task/default/.gitkeep`
  - `factory/inputs/thoughts/default/.gitkeep`
- any extra files under `factory/inputs/**` remain workspace-local residue and
  not checked-in queue truth
- the local worktree is dirty only for non-canonical evidence:
  - `factory/logs/meta/asks.md` has a pre-existing local wording edit
  - `factory/logs/weird-number-summary.jsonl` is untracked local evidence for
    an already-merged dashboard bug
- live PR state relevant to maintainer routing is now:
  - open PR `#61` `browser-shared-action-primitives`
  - merged PR `#65` `retire-dashboard-format-helper-ownership`
  - merged PR `#64` `retire-dashboard-bento-layout-ownership`
  - merged PR `#63` `retire-current-selection-inference-duplication`
  - merged PR `#62` `align-dashboard-work-summary-count-semantics`
  - merged PRs `#48`, `#46`, and `#42` already satisfy the old throttle ask
- the previously queued website helper seam is no longer live on `main`:
  - `ui/src/components/ui/formatters.ts` now owns the shared formatter helpers
  - `ui/src/components/ui/place-labels.ts` now owns the shared place-label
    helpers
  - `ui/src/components/dashboard/formatters.ts` and
    `ui/src/components/dashboard/place-labels.ts` are now compatibility shims
- replay evidence remains stable and compact enough to summarize:
  - `process`: `9 ACCEPTED <COMPLETE>`, `27 CONTINUE <CONTINUE>`
  - `review`: `5 ACCEPTED <COMPLETE>`, `4 REJECTED <REJECTED>`

## current blockers

1. the old checked-in meta view is stale and overgrown:
   - it still anchored on `e87f9d3`
   - it still treated the formatter/place-label lane as pending even though
     PR `#65` merged at `2026-05-03T00:39:01Z`
2. the canonical ask surface currently blocks new queue submissions until the
   meta surfaces are cleaned up
3. `factory/logs/meta/asks.md` is locally modified already, so this pass must
   not rewrite it
4. any future cleanup idea must stay out of the live PR `#61` file set unless
   that PR closes or changes scope first

## theory of mind

- live `HEAD`, open PR file sets, and merged PR history must keep winning over
  stale meta prose and ignored `factory/inputs/**` residue
- the repository is moving fast enough that queued ideas can become obsolete
  within hours; maintainer state should capture the current answer, not the
  whole path taken to get there
- the highest-value maintainer action for this pass is accuracy and prompt-size
  reduction, not dispatching more cleanup work
- until `factory/logs/meta/asks.md` re-opens the queue, new task submission
  would be misaligned with the checked-in maintainer contract

## next best move

- keep `factory/logs/meta/view.md` and `factory/logs/meta/progress.txt`
  compact and current
- leave `factory/logs/meta/asks.md` unchanged
- do not create new files under `factory/inputs/**` on this pass
