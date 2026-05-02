# meta view

## world state

- after `git pull`, repository `main` and `origin/main` are both at
  `368d182` on May 1, 2026 in the local maintainer workspace
- the canonical checked-in maintainer backlog is still
  `factory/logs/meta/asks.md`; no item in that file is marked urgent
- the checked-in workflow inboxes still contain only tracked `.gitkeep`
  sentinels:
  - `factory/inputs/BATCH/default/.gitkeep`
  - `factory/inputs/idea/default/.gitkeep`
  - `factory/inputs/plan/default/.gitkeep`
  - `factory/inputs/task/default/.gitkeep`
  - `factory/inputs/thoughts/default/.gitkeep`
- the workspace-local `factory/inputs/**` surface still has ignored residue
  beyond the tracked sentinels, so those files remain local context only and
  not checked-in workflow truth:
  - `factory/inputs/idea/default/collapse-dashboard-fallback-work-item-collectors.md`
  - `factory/inputs/idea/default/consolidate-dashboard-session-fallback-workitem-collectors.md`
  - `factory/inputs/idea/default/dedupe-list-work-legacy-pagination-fallback.md`
  - `factory/inputs/idea/default/dedupe-replay-factory-merge-helpers.md`
  - `factory/inputs/idea/default/dedupe-worker-event-exit-code-extraction.md`
  - `factory/inputs/idea/default/derive-throttle-windows-from-completed-dispatch-history.md`
  - `factory/inputs/idea/default/derive-throttle-windows-from-event-history.md`
  - `factory/inputs/idea/default/inline-batch-relation-duplicate-rejection.md`
  - `factory/inputs/idea/default/prd-api-model-contract-cleanup.md`
  - `factory/inputs/idea/default/prd-cli-consumer-installation.md`
  - `factory/inputs/idea/default/prd-current-factory-default-runtime-support.md`
  - `factory/inputs/idea/default/prd-functional-test-suite-decomposition.md`
  - `factory/inputs/idea/default/prd-goreleaser-release-pipeline.md`
  - `factory/inputs/idea/default/retire-duplicate-ui-script-copies.md`
  - `factory/inputs/idea/default/retire-dispatcher-throttle-pause-map.md`
  - `factory/inputs/plan/default/retire-dispatch-result-hook-syncdispatch-cache.md`
- the current GitHub lane state changed again after the previous checked-in
  world model:
  - open PR `#33` `prd-api-model-contract-cleanup`
  - open PR `#30` `prd-functional-test-suite-decomposition`
  - merged PR `#44` `inline-batch-relation-duplicate-rejection`
  - merged PR `#43` `collapse-dashboard-fallback-work-item-collectors`
  - merged PR `#42` `retire-dispatcher-throttle-pause-map`
  - merged PR `#41` `dedupe-replay-factory-merge-helpers`
  - merged PR `#40` `dedupe-worker-event-exit-code-extraction`
  - merged PR `#39` `chaining-trace-ids`
  - merged PR `#38` `prd-current-factory-default-runtime-support`
  - merged PR `#37` `prd-cli-consumer-installation`
  - merged PR `#36` `retire-dispatch-result-hook-syncdispatch-cache`
  - merged PR `#35` `consolidate-dashboard-session-fallback-workitem-collectors`
- the worktree is currently clean even though ignored local workflow-input
  residue remains under `factory/inputs/**`
- the broad throttle customer ask remains the highest-value architecture ask,
  but its implementation posture is unchanged from the latest dispatcher
  cleanup:
  - `pkg/factory/internal/throttle/windows.go` still owns the pure helper that
    derives active provider/model throttle windows from normalized failure
    history, pause duration, and explicit clock time
  - `pkg/factory/subsystems/subsystem_dispatcher.go` no longer stores the
    mutable `throttlePauses` map and now derives current active pauses from
    `snapshot.DispatchHistory`
  - there is still no checked-in `factory.guards` lowering path for
    `INFERENCE_THROTTLE_GUARD`
- the previous next cleanup lane has already landed on `main`:
  - `pkg/cli/dashboard/dashboard.go` no longer owns separate completed and
    failed fallback wrapper helpers after merged PR `#43`
  - `pkg/factory/work_request.go` no longer owns the standalone
    `rejectDuplicateBatchRelation` helper after merged PR `#44`
- one new narrow cleanup seam is queueable outside the remaining open PR file
  sets:
  - `ui/scripts/normalize-dist-output copy.mjs` and
    `ui/scripts/write-replay-coverage-report copy.ts` are still tracked beside
    the canonical scripts used by `ui/package.json`
  - the checked-in docs and package scripts reference only the canonical
    `ui/scripts/normalize-dist-output.mjs` and
    `ui/scripts/write-replay-coverage-report.ts` paths
  - deleting the tracked `copy` files is a dead-code cleanup lane entirely
    outside `#33` API/config/public-contract work and `#30`
    `tests/functional/**` reorganization

## current blockers

1. the broad `INFERENCE_THROTTLE_GUARD` customer ask still spans config shape,
   guard lowering, and transition ownership, so it is still too large for a
   safe single lane
2. open PR `#33` occupies most of the active API/config/public-contract file
   sets that a guard-lowering lane would need
3. open PR `#30` occupies the `tests/functional/**` reorganization lane, so
   new cleanup work should avoid functional test file moves for now
4. the previous checked-in world model is now stale in four important ways:
   - it still described upstream `HEAD` as `23729cb`
   - it did not account for merged PR `#44`
   - it still treated the batch-relation duplicate-helper cleanup as only
     queueable even though that seam has already landed on `main`
   - it did not account for the now-validated dead-code seam in `ui/scripts/`
5. workspace-local ignored residue can drift independently of `main`:
   - `factory/inputs/plan/default/retire-dispatch-result-hook-syncdispatch-cache.md`
     still exists locally even though PR `#36` already removed the
     `syncDispatch` cache from `pkg/factory/runtime/dispatch_result_hook.go`

## theory of mind

- merged PR history and live open PR file sets must keep winning over both the
  checked-in meta view and ignored `factory/inputs/**` residue; this repository
  is changing quickly enough that the checked-in world model can drift within
  hours
- the highest-value live customer problem is still global throttling, but the
  remaining gap is still clearly the public/config/lowering layer:
  - dispatcher-local duplicate throttle state has already been removed on
    `main`
  - the remaining work is a later `factory.guards` /
    `INFERENCE_THROTTLE_GUARD` lane
- with PRs `#43` and `#44` merged, the best available cleanup work is no
  longer in the CLI dashboard fallback path or batch-relation validation path;
  both seams are already complete on `main`
- the remaining live collision surface is still narrow:
  - `#33` owns API/config/public guard contract cleanup
  - `#30` owns functional test package decomposition
- because those open PRs stay out of `ui/scripts/**`, the best available
  queueable follow-up is now a dead-code cleanup that removes tracked duplicate
  `copy` scripts without changing package-owned workflow entrypoints
- when a cleanup lane already exists as ignored local residue under
  `factory/inputs/**`, it may already be merged on `main`; the maintainer loop
  should refresh the world model instead of re-queuing recently landed work
- package script wiring and checked-in docs are a strong source of truth for
  canonical UI tooling entrypoints; tracked sibling `copy` files should be
  treated as suspect dead code until proven otherwise

## next best move

- update the checked-in meta world model and progress log now
- leave `factory/logs/meta/asks.md` unchanged; the priority order is still
  correct
- do not re-queue already-landed lanes such as `#44`, `#43`, `#42`, `#41`,
  `#40`, `#38`, `#37`, `#36`, or `#35`
- queue one new ignored cleanup idea for the duplicate UI script seam:
  - delete `ui/scripts/normalize-dist-output copy.mjs`
  - delete `ui/scripts/write-replay-coverage-report copy.ts`
  - preserve `ui/package.json` script wiring and checked-in doc references to
    the canonical script paths

## customer asks

- `factory/logs/meta/asks.md` remains the only checked-in backlog surface
- no ask is marked urgent as of May 1, 2026 in the maintainer workspace
- the throttling ask is still the most important architecture-level customer
  ask, but its remaining work is now clearly factory-level config/lowering
  rather than dispatcher-local duplicate state cleanup
- the quality and website-quality asks remain broader follow-on programs, but
  removing tracked dead UI tooling copies is still aligned with their
  simplification goals
- the next throttle follow-up should stay decomposed and should not overlap the
  already-open API/config contract lane in `#33`
