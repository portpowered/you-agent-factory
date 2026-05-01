# meta view

## world state

- repository `HEAD` is `25580f5` on `main` on May 1, 2026, and
  `origin/main` is the same commit after `git fetch origin`
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
  - `factory/inputs/idea/default/dedupe-list-work-legacy-pagination-fallback.md`
  - `factory/inputs/idea/default/derive-throttle-windows-from-completed-dispatch-history.md`
  - `factory/inputs/idea/default/derive-throttle-windows-from-event-history.md`
  - `factory/inputs/idea/default/prd-api-model-contract-cleanup.md`
  - `factory/inputs/idea/default/prd-functional-test-suite-decomposition.md`
  - `factory/inputs/idea/default/prd-goreleaser-release-pipeline.md`
- the current GitHub lane state on May 1, 2026 is:
  - open PR `#33` `prd-api-model-contract-cleanup`
  - open PR `#32` `shadcn-components-for-website`
  - open PR `#30` `prd-functional-test-suite-decomposition`
  - open PR `#20` `test-cleanup`
  - open PR `#16` `dedupe-root-factory-artifact-contract-entries`
  - open PR `#4` `standardize-contract-guard-skip-policy`
  - merged PR `#34` `dedupe-list-work-legacy-pagination-fallback`
  - merged PR `#31` `derive-throttle-windows-from-completed-dispatch-history`
  - merged PR `#29` `prd-goreleaser-release-pipeline`
  - merged PR `#28` `derive-throttle-windows-from-event-history`
  - merged PR `#27` `dedupe-generated-boundary-alias-rejection-coverage`
  - merged PR `#26` `dedupe-retired-boundary-alias-rejection-tables`
  - merged PR `#25` `retire-scriptwrap-build-args-shim`
  - merged PR `#24` `consolidate-public-factory-enum-alias-ownership`
  - merged PR `#23` `centralize-work-request-trace-normalization`
  - merged PR `#22` `align-process-review-loop-contract`
  - merged PR `#21` `dedupe-dispatcher-throttle-pause-filter`
- the worktree is not clean even though `HEAD` matches remote:
  - modified `README.md`
  - modified `ui/dist/assets/index.js`
  - modified `ui/dist_stamp.go`
  - untracked `docs/reference/references.md`
- the broad throttle customer ask remains open, but the repository posture is
  more decomposed than the previous checked-in view captured:
  - `pkg/factory/internal/throttle/windows.go` now owns the pure helper that
    derives active provider/model throttle windows from normalized failure
    history, pause duration, and an explicit clock time
  - `pkg/factory/subsystems/subsystem_dispatcher.go` now reconstructs throttle
    failure history from `snapshot.DispatchHistory` using exact
    `interfaces.CompletedDispatch.EndTime` values
  - `pkg/factory/subsystems/subsystem_dispatcher.go` still owns the mutable
    runtime pause map and still gates scheduling by dispatcher-owned state
  - the ask for config-authored `factory.guards` with
    `INFERENCE_THROTTLE_GUARD` is still not implemented on `main`
- the release-pipeline lane is fully landed on `main`, and the small `/work`
  pagination cleanup lane is now landed too:
  - PR `#29` added `.goreleaser.yml`, release workflows, release smoke
    commands, and release smoke fixtures
  - PR `#34` removed the duplicate tolerant `maxResults` fallback from
    `pkg/api/handlers.go` while preserving the handwritten `/work`
    compatibility wrapper in `pkg/api/server.go`
- the public/model cleanup and website quality asks are now actively in flight
  through open PRs:
  - PR `#33` is the current broad API model contract lane and already reaches
    `api/`, `pkg/config/`, `pkg/interfaces/`, `pkg/api/`, docs, examples,
    replay fixtures, and generated artifacts
  - PR `#32` is the current website component/system lane and already reaches
    shared UI primitives, feature components, stories, tests, styles, and
    built assets
- one new narrow cleanup seam is queueable outside the active PR file sets:
  - `pkg/factory/runtime/dispatch_result_hook.go` stores a `syncDispatch` bool
    that is derived once from `planner != nil`
  - the same hook still reads `h.planner` directly for delivery-tick planning,
    replay-tick validation, and planned result replacement
  - that cached bool is therefore duplicate local runtime state and a narrow
    simplification target

## current blockers

1. the broad `INFERENCE_THROTTLE_GUARD` customer ask still spans config shape,
   guard lowering, scheduler ownership, and observability, so it is too large
   for a safe single lane
2. open PRs `#33`, `#32`, `#30`, `#20`, `#16`, and `#4` now occupy most of
   the public contract, UI, functional-test, artifact-contract, and guard-
   policy file sets
3. the previous checked-in world model was stale in three important ways:
   - it still described `HEAD` as `acac8fc`
   - it did not include merged PR `#34`
   - it did not include newly open PRs `#33` and `#32`
4. one initially plausible cleanup seam in `pkg/cli/run` is currently blocked
   because PR `#33` already touches `pkg/cli/run/run_test.go`, so that lane is
   not actually conflict-free

## theory of mind

- merged PR history and live open PR file sets must keep winning over both the
  checked-in meta view and ignored `factory/inputs/**` residue; this repository
  is changing quickly enough that the checked-in world model can drift within
  hours
- the highest-value live customer problem is still global throttling, but the
  remaining gap is now clearly on the public/config/lowering side rather than
  on throttle timing reconstruction
- because PR `#33` is a wide public-model lane and PR `#32` is a wide website
  quality lane, new follow-up work should avoid those surfaces rather than
  trying to parallelize into them
- the best available cleanup work right now is not another public-contract or
  UI simplification; it is a small backend state-reduction seam that removes
  duplicate cached state without changing behavior
- a sidecar candidate that looks package-local can still collide with an open
  umbrella PR, so file-set checks must include all active PRs before queuing
  new work

## next best move

- update the checked-in meta world model and progress log now
- leave `factory/logs/meta/asks.md` unchanged; the priority order is still
  correct
- queue one new ignored cleanup idea for
  `pkg/factory/runtime/dispatch_result_hook.go`: preserve current
  planner-driven synchronous dispatch behavior, but retire the duplicate
  cached `syncDispatch` state
- avoid re-queuing already-landed lanes and avoid colliding with open PRs
  `#33`, `#32`, `#30`, `#20`, `#16`, and `#4`

## customer asks

- `factory/logs/meta/asks.md` remains the only checked-in backlog surface
- no ask is marked urgent as of May 1, 2026
- the throttling ask is still the most important architecture-level customer
  ask
- the quality and website-quality asks are now partially represented by live
  PRs, but they remain broader than the current narrow cleanup queue
- the next throttle follow-up should stay decomposed and should not overlap the
  already-open public-model contract lane
