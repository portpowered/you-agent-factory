# meta view

## world state

- repository `HEAD` is `4a20238` on `main` on May 1, 2026, and
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
  - `factory/inputs/idea/default/derive-throttle-windows-from-event-history.md`
  - `factory/inputs/idea/default/prd-api-model-contract-cleanup.md`
  - `factory/inputs/idea/default/prd-functional-test-suite-decomposition.md`
  - `factory/inputs/idea/default/prd-goreleaser-release-pipeline.md`
- the current GitHub lane state on May 1, 2026 is:
  - open PR `#30` `prd-functional-test-suite-decomposition`
  - open PR `#29` `prd-goreleaser-release-pipeline`
  - open PR `#20` `test-cleanup`
  - open PR `#16` `dedupe-root-factory-artifact-contract-entries`
  - open PR `#4` `standardize-contract-guard-skip-policy`
  - merged PR `#28` `derive-throttle-windows-from-event-history`
  - merged PR `#27` `dedupe-generated-boundary-alias-rejection-coverage`
  - merged PR `#26` `dedupe-retired-boundary-alias-rejection-tables`
  - merged PR `#25` `retire-scriptwrap-build-args-shim`
  - merged PR `#24` `consolidate-public-factory-enum-alias-ownership`
  - merged PR `#23` `centralize-work-request-trace-normalization`
  - merged PR `#22` `align-process-review-loop-contract`
  - merged PR `#21` `dedupe-dispatcher-throttle-pause-filter`
- the broad throttle customer ask remains open, but one important
  decomposition step is now landed on `main`:
  - `pkg/factory/internal/throttle/windows.go` now contains a pure internal
    helper that derives active provider/model throttle windows from normalized
    failure history, pause duration, and an explicit clock time
  - `pkg/factory/subsystems/subsystem_dispatcher.go` still owns the mutable
    runtime pause map and still gates scheduling by that dispatcher-owned state
  - `pkg/factory/subsystems/subsystem_dispatcher.go` currently adapts runtime
    data into that helper by reading `snapshot.Results` and assigning every
    throttle failure the current observation time rather than the dispatch's
    actual completion time
  - the runtime already carries richer completion timing in
    `snapshot.DispatchHistory` through `interfaces.CompletedDispatch.EndTime`,
    so the event-time seam is present but not yet consumed by the throttle path
  - the ask for config-authored `factory.guards` with
    `INFERENCE_THROTTLE_GUARD` is still not implemented on `main`
- the current customer-facing guard surface is still narrower than the ask
  requires:
  - `pkg/interfaces/factory_config.go` currently exposes workstation guards
    `visit_count` and `matches_fields`
  - per-input guards remain `all_children_complete`, `any_child_failed`, and
    `same_name`
  - there is still no factory-level guard owner yet, so the throttle redesign
    still needs decomposition across config, mapping, validation, and lowering
- one inspected narrow API cleanup candidate is still not ready for queueing:
  - `pkg/api/server.go` registers a handwritten `/work` route that forwards to
    `ListWork`
  - that shim intentionally preserves tolerant `maxResults` parsing before the
    generated server's stricter integer binding runs
  - `pkg/api/server_test.go` still codifies that tolerant behavior, so
    removing the shim would change the current public request contract rather
    than retire dead code

## current blockers

1. the broad `INFERENCE_THROTTLE_GUARD` customer ask still spans config shape,
   guard lowering, scheduler enablement, and observability, so it is too large
   for a safe single lane
2. open PRs `#30`, `#29`, `#20`, `#16`, and `#4` occupy the current
   functional-test, release-pipeline, artifact-contract, and guard-policy file
   sets, so any new queued work should stay outside those lanes
3. the checked-in world model was stale because it still treated the throttle
   derivation seam as only queued even though PR `#28` has merged
4. the `/work` pagination shim is behavior-bearing compatibility code today,
   not confirmed dead code

## theory of mind

- merged PR history must keep winning over ignored `factory/inputs/**` residue,
  but the ignored surface is still useful as a signal of what maintainers are
  exploring locally; that surface now mixes one already-landed throttle idea
  with two active PRD lanes
- the highest-value live customer problem is still global throttling, but the
  posture has changed: the pure derivation primitive now exists, so the next
  bottleneck is the adapter layer that feeds it
- the most important remaining mismatch with the customer's requested model is
  not only mutable dispatcher-owned pause state; it is that the current runtime
  still reconstructs throttle windows from tick-local `snapshot.Results`
  observed at `now` instead of from the actual completed-dispatch/event times
  already present in runtime history
- that makes the next good seam narrower than authored guards:
  move the throttle derivation input from observation-time results to completed
  dispatch history or an equivalent event-time projection, while leaving public
  config and dashboard contracts alone
- an apparent duplicate path is not automatically cleanup-ready:
  the `/work` router shim still preserves tolerant public pagination parsing
  that the generated binding would reject

## next best move

- update the checked-in meta world model and progress log now
- leave `factory/logs/meta/asks.md` unchanged; the priority order is still
  correct
- queue one new narrow ignored idea for the throttling ask:
  derive active provider/model throttle windows from completed dispatch history
  and exact completion times rather than from tick-local observed results
- avoid re-queuing already-landed cleanup lanes and avoid colliding with open
  PRs `#30`, `#29`, `#20`, `#16`, and `#4`

## customer asks

- `factory/logs/meta/asks.md` remains the only checked-in backlog surface
- no ask is marked urgent as of May 1, 2026
- the throttling ask is still the most important architecture-level customer
  ask
- the first derivation seam for that ask has now landed via PR `#28`
- the next follow-up for that ask should be event-time-backed throttle history,
  not the full `INFERENCE_THROTTLE_GUARD` implementation in one jump
