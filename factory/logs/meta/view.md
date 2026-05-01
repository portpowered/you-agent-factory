# meta view

## world state

- repository `HEAD` is `8c84704` on `main` on May 1, 2026, and
  `origin/main` is the same commit after `git pull`
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
  - `factory/inputs/idea/default/consolidate-dashboard-session-fallback-workitem-collectors.md`
  - `factory/inputs/idea/default/dedupe-list-work-legacy-pagination-fallback.md`
  - `factory/inputs/idea/default/dedupe-worker-event-exit-code-extraction.md`
  - `factory/inputs/idea/default/derive-throttle-windows-from-completed-dispatch-history.md`
  - `factory/inputs/idea/default/derive-throttle-windows-from-event-history.md`
  - `factory/inputs/idea/default/prd-api-model-contract-cleanup.md`
  - `factory/inputs/idea/default/prd-cli-consumer-installation.md`
  - `factory/inputs/idea/default/prd-current-factory-default-runtime-support.md`
  - `factory/inputs/idea/default/prd-functional-test-suite-decomposition.md`
  - `factory/inputs/idea/default/prd-goreleaser-release-pipeline.md`
  - `factory/inputs/plan/default/retire-dispatch-result-hook-syncdispatch-cache.md`
- the current GitHub lane state on May 1, 2026 is:
  - open PR `#36` `retire-dispatch-result-hook-syncdispatch-cache`
  - open PR `#35` `consolidate-dashboard-session-fallback-workitem-collectors`
  - open PR `#33` `prd-api-model-contract-cleanup`
  - open PR `#30` `prd-functional-test-suite-decomposition`
  - merged PR `#34` `dedupe-list-work-legacy-pagination-fallback`
  - merged PR `#32` `shadcn-components-for-website`
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
- the worktree is currently clean even though ignored local workflow-input
  residue remains under `factory/inputs/**`
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
  through open/just-landed lanes:
  - PR `#33` is the current broad API model contract lane and already reaches
    `api/`, `pkg/config/`, `pkg/interfaces/`, `pkg/api/`, docs, examples,
    replay fixtures, and generated artifacts
  - PR `#32` just landed the current website component/system lane and reaches
    shared UI primitives, feature components, stories, tests, styles, and
    built assets
- the last queued narrow cleanup seams are no longer just candidates:
  - PR `#35` now owns the dashboard-only fallback-collector simplification in
    `pkg/cli/dashboard/dashboard.go`
  - PR `#36` now owns the `pkg/factory/runtime/dispatch_result_hook.go`
    `syncDispatch` cache retirement
- one newer narrow cleanup seam is queueable outside the active PR file sets:
  - `pkg/workers/script.go` and `pkg/workers/recording_provider.go` each own a
    small helper that derives emitted event `ExitCode` values from command
    diagnostics
  - `scriptResponseExitCode` and `providerErrorExitCode` are package-local,
    single-caller helpers for the same boundary-event concern
  - nearby focused worker tests already assert exit-code behavior, so this seam
    is low-risk and does not need public contract changes

## current blockers

1. the broad `INFERENCE_THROTTLE_GUARD` customer ask still spans config shape,
   guard lowering, scheduler ownership, and observability, so it is too large
   for a safe single lane
2. open PRs `#33`, `#30`, `#35`, and `#36` now occupy most of the active
   public-contract, functional-test, dashboard, and dispatch-hook file sets
3. the previous checked-in world model was stale in three important ways:
   - it still described `HEAD` as `24a6463`
   - it still omitted newly open PRs `#35` and `#36`
   - it still recommended the dashboard fallback cleanup as the next lane even
     though that lane is already in review
4. the broad public/model ask is active in PR `#33`, so the next cleanup lane
   should avoid `pkg/api/`, `pkg/config/`, `pkg/interfaces/`, and nearby
   public-contract surfaces until that lane settles

## theory of mind

- merged PR history and live open PR file sets must keep winning over both the
  checked-in meta view and ignored `factory/inputs/**` residue; this repository
  is changing quickly enough that the checked-in world model can drift within
  hours
- the highest-value live customer problem is still global throttling, but the
  remaining gap is now clearly on the public/config/lowering side rather than
  on throttle timing reconstruction
- because PR `#33` is a wide public-model lane and PR `#30` is a wide
  functional-test restructuring lane, new follow-up work should avoid those
  surfaces rather than trying to parallelize into them
- the website-quality lane advanced materially with merged PR `#32`, so the
  next cleanup task no longer needs to spend the one available sidecar slot on
  shared-component cleanup
- because PRs `#35` and `#36` now occupy the last two narrow local cleanup
  seams already identified, the next sidecar task should move to another
  package-local duplication site instead of piling onto those live branches
- the best available cleanup work right now is a narrow `pkg/workers` event
  emission dedupe that consolidates exit-code extraction ownership without
  changing public contracts or runtime scheduling behavior
- a sidecar candidate that looks package-local can still collide with an open
  umbrella PR, so file-set checks must include all active PRs before queuing
  new work

## next best move

- update the checked-in meta world model and progress log now
- leave `factory/logs/meta/asks.md` unchanged; the priority order is still
  correct
- queue one new ignored cleanup idea for worker event emission:
  preserve current emitted script/inference response payloads, but consolidate
  duplicate `ExitCode` extraction ownership across `pkg/workers/script.go` and
  `pkg/workers/recording_provider.go`
- keep the already-open dashboard and `syncDispatch` lanes as active local
  context rather than re-queueing them
- avoid re-queuing already-landed lanes and avoid colliding with open PRs
  `#33`, `#30`, `#35`, and `#36`

## customer asks

- `factory/logs/meta/asks.md` remains the only checked-in backlog surface
- no ask is marked urgent as of May 1, 2026
- the throttling ask is still the most important architecture-level customer
  ask
- the quality and website-quality asks are now partially represented by live
  and freshly merged lanes, but they remain broader than the current narrow
  cleanup queue
- the next throttle follow-up should stay decomposed and should not overlap the
  already-open public-model contract lane
