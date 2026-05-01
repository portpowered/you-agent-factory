# meta view

## world state

- repository `HEAD` is `acac8fc` on `main` on May 1, 2026, and
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
  - `factory/inputs/idea/default/derive-throttle-windows-from-completed-dispatch-history.md`
  - `factory/inputs/idea/default/derive-throttle-windows-from-event-history.md`
  - `factory/inputs/idea/default/prd-api-model-contract-cleanup.md`
  - `factory/inputs/idea/default/prd-functional-test-suite-decomposition.md`
  - `factory/inputs/idea/default/prd-goreleaser-release-pipeline.md`
  - `factory/inputs/idea/default/dedupe-list-work-legacy-pagination-fallback.md`
- the current GitHub lane state on May 1, 2026 is:
  - open PR `#30` `prd-functional-test-suite-decomposition`
  - open PR `#20` `test-cleanup`
  - open PR `#16` `dedupe-root-factory-artifact-contract-entries`
  - open PR `#4` `standardize-contract-guard-skip-policy`
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
- the broad throttle customer ask remains open, but two important
  decomposition steps are now landed on `main`:
  - `pkg/factory/internal/throttle/windows.go` now contains a pure internal
    helper that derives active provider/model throttle windows from normalized
    failure history, pause duration, and an explicit clock time
  - `pkg/factory/subsystems/subsystem_dispatcher.go` now reconstructs
    throttle failure history from `snapshot.DispatchHistory` using exact
    `interfaces.CompletedDispatch.EndTime` values instead of assigning every
    failure the current observation time
  - `pkg/factory/subsystems/subsystem_dispatcher.go` still owns the mutable
    runtime pause map and still gates scheduling by that dispatcher-owned state
  - the ask for config-authored `factory.guards` with
    `INFERENCE_THROTTLE_GUARD` is still not implemented on `main`
- the release-pipeline PRD lane is no longer just planned:
  - `#29` is now merged, adding `.goreleaser.yml`, release workflows, release
    prep/smoke commands, and release smoke fixtures on `main`
  - the release quality ask now has a checked-in first-phase implementation,
    so future work can focus on follow-on polish instead of initial setup
- the current customer-facing guard surface is still narrower than the ask
  requires:
  - `pkg/interfaces/factory_config.go` currently exposes workstation guards
    `visit_count` and `matches_fields`
  - per-input guards remain `all_children_complete`, `any_child_failed`, and
    `same_name`
  - there is still no factory-level guard owner yet, so the throttle redesign
    still needs decomposition across config, mapping, validation, and lowering
- one inspected narrow API cleanup candidate is now queueable without changing
  the public request contract:
  - `pkg/api/server.go` registers a handwritten `/work` route that forwards to
    `ListWork`
  - that shim intentionally preserves tolerant `maxResults` parsing before the
    generated server's stricter integer binding runs
  - `pkg/api/handlers.go` still re-reads `r.URL.Query().Get("maxResults")`
    and applies a second tolerant fallback even when the handwritten wrapper
    already owns that compatibility behavior
  - the redundant fallback inside `ListWork` is therefore a small cleanup seam,
    while the handwritten route itself remains behavior-bearing

## current blockers

1. the broad `INFERENCE_THROTTLE_GUARD` customer ask still spans config shape,
   guard lowering, scheduler enablement, and observability, so it is too large
   for a safe single lane
2. open PRs `#30`, `#20`, `#16`, and `#4` occupy the current functional-test,
   artifact-contract, and guard-policy file sets, so any new queued work
   should stay outside those lanes
3. the checked-in world model was stale because it still treated the completed-
   dispatch throttle seam and the release-pipeline lane as not yet landed
4. the handwritten `/work` route is still behavior-bearing compatibility code,
   so cleanup must target only the duplicate fallback inside `ListWork`

## theory of mind

- merged PR history must keep winning over ignored `factory/inputs/**` residue,
  but the ignored surface is still useful as a signal of what maintainers are
  exploring locally; that surface now mixes two already-landed lanes, one new
  narrow cleanup request, and two active PRD lanes
- the highest-value live customer problem is still global throttling, but the
  posture changed again after PR `#31`: both the pure derivation helper and
  the completed-dispatch event-time adapter now exist on `main`
- the biggest remaining mismatch with the customer's requested model is no
  longer event-time reconstruction; it is the public/config side:
  there is still no factory-level `factory.guards` owner, no
  `INFERENCE_THROTTLE_GUARD` type, and scheduler gating still lives in
  dispatcher-owned mutable pause state rather than generic guard lowering
- that means the next throttle follow-up should stay decomposed and avoid
  trying to land config shape, validation, lowering, and observability in one
  jump
- an apparent duplicate path can become cleanup-ready when the behavior-bearing
  shim and the redundant fallback inside the callee are separated:
  keep the handwritten `/work` wrapper for tolerant parsing, but dedupe the
  same fallback out of `ListWork`

## next best move

- update the checked-in meta world model and progress log now
- leave `factory/logs/meta/asks.md` unchanged; the priority order is still
  correct
- queue one new narrow ignored cleanup idea for `/work` pagination handling:
  preserve the handwritten compatibility shim, but retire the duplicate
  tolerant `maxResults` fallback from `ListWork`
- avoid re-queuing already-landed cleanup lanes and avoid colliding with open
  PRs `#30`, `#20`, `#16`, and `#4`

## customer asks

- `factory/logs/meta/asks.md` remains the only checked-in backlog surface
- no ask is marked urgent as of May 1, 2026
- the throttling ask is still the most important architecture-level customer
  ask
- the first two throttle derivation seams for that ask have now landed via
  PRs `#28` and `#31`
- the next follow-up for that ask should be a smaller config/lowering
  decomposition, not the full `INFERENCE_THROTTLE_GUARD` implementation in one
  jump
