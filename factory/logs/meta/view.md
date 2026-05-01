# meta view

## world state

- repository `HEAD` is `21884a0` on `main` on May 1, 2026, and
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
  - `factory/inputs/idea/default/prd-api-model-contract-cleanup.md`
  - `factory/inputs/idea/default/prd-functional-test-suite-decomposition.md`
  - `factory/inputs/idea/default/prd-goreleaser-release-pipeline.md`
  - `factory/inputs/idea/default/derive-throttle-windows-from-event-history.md`
- the current GitHub lane state on May 1, 2026 is:
  - open PR `#20` `test-cleanup`
  - open PR `#16` `dedupe-root-factory-artifact-contract-entries`
  - open PR `#4` `standardize-contract-guard-skip-policy`
  - merged PR `#27` `dedupe-generated-boundary-alias-rejection-coverage`
  - merged PR `#26` `dedupe-retired-boundary-alias-rejection-tables`
  - merged PR `#25` `retire-scriptwrap-build-args-shim`
  - merged PR `#24` `consolidate-public-factory-enum-alias-ownership`
  - merged PR `#23` `centralize-work-request-trace-normalization`
  - merged PR `#22` `align-process-review-loop-contract`
  - merged PR `#21` `dedupe-dispatcher-throttle-pause-filter`
- the broad throttle customer ask remains open, and the current checked-in code
  still keeps pause ownership outside the normal guard path:
  - dispatcher-owned pause state still lives in
    `pkg/factory/subsystems/subsystem_dispatcher.go` as runtime memory keyed by
    provider/model
  - throttled provider failures still classify through
    `pkg/interfaces/provider_failure.go` and `pkg/workers/provider_errors.go`
  - runtime snapshots and dashboard/world-view projections still mirror that
    dispatcher-owned pause state through `pkg/factory/engine/runtime_state.go`,
    `pkg/interfaces/engine_state.go`, `pkg/interfaces/engine_runtime.go`, and
    `pkg/factory/projections/throttle_pause_projection.go`
  - the ask for config-authored `factory.guards` with
    `INFERENCE_THROTTLE_GUARD` is still not implemented on `main`
- the current customer-facing guard surface is narrower than the ask requires:
  - `pkg/interfaces/factory_config.go` currently exposes workstation guards
    `visit_count` and `matches_fields`
  - per-input guards remain `all_children_complete`, `any_child_failed`, and
    `same_name`
  - there is no factory-level guard owner yet, so the throttle redesign still
    needs decomposition across config, mapping, validation, and lowering
- one inspected narrow API cleanup candidate is still not ready for queueing:
  - `pkg/api/server.go` registers a handwritten `/work` route that forwards to
    `ListWork`
  - that shim intentionally preserves tolerant `maxResults` parsing before the
    generated server's stricter integer binding runs
  - removing it would change current public request tolerance, so it is not a
    pure dead-code cleanup

## current blockers

1. the broad `INFERENCE_THROTTLE_GUARD` customer ask still spans config shape,
   guard lowering, scheduler enablement, and observability, so it is too large
   for a safe single cleanup lane
2. open PRs `#20`, `#16`, and `#4` still occupy the active functional-test,
   artifact-contract, and contract-guard file sets, so new queued work should
   stay outside those lanes
3. the checked-in world model was stale because it no longer matched either the
   current `HEAD` or the current ignored workflow-input residue
4. the `/work` pagination shim is behavior-bearing compatibility code today,
   not confirmed dead code

## theory of mind

- merged PR history must keep winning over ignored `factory/inputs/**` residue,
  but the ignored surface is still useful as a signal of what maintainers are
  exploring locally; that surface has shifted from the earlier cleanup ideas to
  three PRD-oriented requests plus a new throttle decomposition request
- the highest-value live customer problem is still global throttling, and the
  code now clearly shows why the ask cannot jump straight to
  `INFERENCE_THROTTLE_GUARD`: pause ownership is split across dispatcher
  runtime memory, provider-failure classification, engine snapshots, and world
  view projections
- the right first seam is not public config yet; it is the derivation logic the
  ask explicitly wants, namely computing active provider/model throttle windows
  from event history or accumulated work results at a given clock time
- once that derivation seam exists, a later lane can lower a new authored
  throttle guard into ordinary guard evaluation without preserving the current
  dispatcher-owned mutable pause map
- an apparent duplicate path is not automatically cleanup-ready:
  the `/work` router shim still preserves tolerant public pagination parsing the
  generated binding would reject

## next best move

- update the checked-in meta world model and progress log now
- leave `factory/logs/meta/asks.md` unchanged; the priority order is still
  correct
- queue one narrow ignored idea for the throttling ask:
  derive active provider/model throttle windows from event history or work
  results without changing scheduler gating, public config, or dashboard
  contracts yet
- avoid re-queuing already-landed cleanup lanes and avoid colliding with open
  PRs `#20`, `#16`, and `#4`

## customer asks

- `factory/logs/meta/asks.md` remains the only checked-in backlog surface
- no ask is marked urgent as of May 1, 2026
- the throttling ask is still the most important architecture-level customer
  ask
- the first newly queued decomposition step for that ask is now the future
  event-history throttle-window derivation lane, not the full
  `INFERENCE_THROTTLE_GUARD` implementation in one jump
