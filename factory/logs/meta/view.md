# meta view

## world state

- as of `2026-05-02T20:02:32.8868149-07:00`, `HEAD` and `origin/main` both
  point to `38f6f50` (`docs: refresh meta world state`)
- local branch status is aligned with remote; the repo is dirty because of
  local tracked UI edits, a local edit to `factory/logs/meta/asks.md`, and
  untracked helper files
- the canonical checked-in maintainer ask surface remains
  `factory/logs/meta/asks.md`
- the current ask guidance says to clean up meta summaries only and not submit
  new cleanup tasks on this pass
- replay evidence remains stable:
  - `process`: `9 ACCEPTED <COMPLETE>`, `27 CONTINUE <CONTINUE>`
  - `review`: `5 ACCEPTED <COMPLETE>`, `4 REJECTED <REJECTED>`

## repo truth

- `factory/README.md` still describes a checked-in repository-maintainer
  workflow with canonical inboxes under `factory/inputs/{BATCH,idea,plan,task,thoughts}/default/`
- tracked `factory/inputs/**` content is sentinel-only right now:
  - `factory/inputs/BATCH/default/.gitkeep`
  - `factory/inputs/idea/default/.gitkeep`
  - `factory/inputs/plan/default/.gitkeep`
  - `factory/inputs/task/default/.gitkeep`
  - `factory/inputs/thoughts/default/.gitkeep`
- real workflow items currently visible under `factory/inputs/**` are local
  gitignored residue, not checked-in repo truth
- `docs/guides/batch-inputs.md` remains the contract for canonical
  `FACTORY_REQUEST_BATCH` submissions when mixed work types or dependency
  ordering are needed
- `factory/factory.json` still models the maintainer loop as
  `thoughts -> idea -> plan -> task`, with `process` and `review` repeaters,
  a logical `consume` step, and loop breakers
- recent maintainer-request branches already merged on `main` still include:
  - PR `#61` `browser-shared-action-primitives`
  - PR `#62` `align-dashboard-work-summary-count-semantics`
  - PR `#63` `retire-current-selection-inference-duplication`
  - PR `#64` `retire-dashboard-bento-layout-ownership`
  - PR `#65` `retire-dashboard-format-helper-ownership`

## local workspace reality

- `factory/logs/meta/asks.md` is already locally modified and should remain
  untouched on this pass
- `factory/logs/weird-number-summary.jsonl` remains untracked local evidence
- the dirty UI worktree is broader than the last summary and currently spans:
  - dashboard component/test edits
  - current-selection refactors and helper splits
  - workflow-activity, trace-drilldown, and work-outcome edits
  - new shared UI helper files such as `ui/src/lib/cx.ts`

## theory of mind

- the authoritative maintainer model must come from tracked files, live git
  state, and current factory topology, not from stale compact summaries
- `factory/inputs/**` should be reasoned about in two layers:
  - checked-in contract: sentinel directories preserved in git
  - local operating residue: ignored work items used by the workflow
- the current ask surface intentionally narrows this pass to meta-hygiene, so
  updating `view.md` and `progress.txt` is the correct move

## next best move

- keep `factory/logs/meta/view.md` and `factory/logs/meta/progress.txt`
  compact and current
- leave `factory/logs/meta/asks.md` unchanged
- do not submit new work under `factory/inputs/**` on this pass
- if a later pass needs queue cleanup or code cleanup, start from the current
  dirty-worktree reality instead of assuming the repo itself is diverged
