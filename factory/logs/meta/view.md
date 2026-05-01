# meta view

## world state

- repository `HEAD` is `c213e25` on `main` on May 1, 2026, and
  `origin/main` is the same commit after `git pull --ff-only`.
- the canonical checked-in maintainer backlog is still
  `factory/logs/meta/asks.md`; no item in that file is marked urgent.
- the checked-in workflow inboxes still contain only tracked `.gitkeep`
  sentinels:
  - `factory/inputs/BATCH/default/.gitkeep`
  - `factory/inputs/idea/default/.gitkeep`
  - `factory/inputs/plan/default/.gitkeep`
  - `factory/inputs/task/default/.gitkeep`
  - `factory/inputs/thoughts/default/.gitkeep`
- the workspace-local `factory/inputs/**` surface still has ignored residue
  beyond the tracked sentinels, so that material is local context only and not
  checked-in workflow truth:
  - `factory/inputs/idea/default/align-process-review-loop-contract.md`
  - `factory/inputs/idea/default/centralize-work-request-trace-normalization.md`
  - `factory/inputs/idea/default/dedupe-dispatcher-throttle-pause-filter.md`
  - `factory/inputs/idea/default/systems-cleanup.md`
  - `factory/inputs/idea/default/test-cleanup.md`
  - `factory/inputs/task/default/ci-cd.md`
- the recent GitHub lane state on May 1, 2026 is now:
  - open PR `#20` `test-cleanup`
  - open PR `#16` `dedupe-root-factory-artifact-contract-entries`
  - open PR `#4` `standardize-contract-guard-skip-policy`
  - merged PR `#23` `centralize-work-request-trace-normalization`
  - merged PR `#22` `align-process-review-loop-contract`
  - merged PR `#21` `dedupe-dispatcher-throttle-pause-filter`
  - merged PR `#19` `systems-cleanup`
  - merged PR `#18` `api-clean`
  - merged PR `#17` `ci-cd`
- the previous follow-up lane for work-request trace normalization is no longer
  pending:
  - `pkg/factory/work_request_json.go` now owns the public conflict check via
    `RejectConflictingWorkRequestTraceFields`
  - `pkg/factory/work_request.go` now owns the canonical normalization seam
  - focused tests for normalization and conflict rejection landed with PR `#23`
- the process/review contract cleanup is now repository truth on `main`:
  - PR `#22` is merged, not merely under review
  - checked-in replay evidence now reports process completions as ordinary
    `CONTINUE` iterations instead of rejection churn
  - the embedded adhoc factory config includes `process.onContinue`
- the broader throttle customer ask remains open at the architecture level:
  - pause state still lives as dispatcher-owned runtime memory keyed by
    provider/model
  - the ask for config-authored `factory.guards` with
    `INFERENCE_THROTTLE_GUARD` is still not implemented on `main`
- there is a new narrow cleanup lane ready outside the active PR set:
  - public-factory enum alias tables and canonicalization helpers are
    duplicated across `pkg/config/public_factory_enums.go` and
    `pkg/interfaces/public_factory_enums.go`
  - the duplicated helpers do not have identical fallback behavior today:
    `pkg/config` rejects unsupported values at normalization time, while
    `pkg/interfaces` falls back to the trimmed input for generated/public enum
    conversions
  - `pkg/config/openapi_factory.go` depends on the config-local alias tables,
    while generated/public conversions depend on the interfaces-local copies

## current blockers

1. the broad `INFERENCE_THROTTLE_GUARD` customer ask is still too large for a
   safe direct jump from the current dispatcher-owned runtime policy.
2. open PRs `#20`, `#16`, and `#4` still occupy their respective file sets, so
   new cleanup work should stay outside those lanes.

## theory of mind

- the checked-in meta model was stale because it still treated merged PRs
  `#22` and `#23` as active follow-up lanes.
- `main` now contains both the process/review contract alignment and the
  work-request trace-normalization cleanup, so those ideas should not be
  re-queued.
- the best next cleanup remains a narrow simplification lane that removes
  duplication without overlapping the open PR set.
- the most defensible next cleanup is to centralize public-factory enum alias
  ownership so one package defines the alias tables and canonicalization
  helpers while preserving the intended strict-vs-permissive boundary
  behavior explicitly.
- the right follow-up for the broader throttling ask is still a later,
  dedicated `INFERENCE_THROTTLE_GUARD` design lane rather than another narrow
  dispatcher tweak.

## next best move

- update the checked-in meta world model and progress log now.
- do not queue another trace-normalization or process/review idea, because
  those lanes already landed on `main`.
- queue one new ignored cleanup idea for public-factory enum alias
  consolidation outside the active PR set.
- leave the broader `INFERENCE_THROTTLE_GUARD` redesign for a later dedicated
  lane once a narrow cleanup slot is needed for that customer ask.

## customer asks

- `factory/logs/meta/asks.md` remains the only checked-in backlog surface.
- no ask is marked urgent as of May 1, 2026.
- the throttling ask is still active, but no checked-in change has yet moved
  pause enforcement into config-authored guards.
- the best next customer-adjacent cleanup lane is the public-factory enum
  alias dedupe, because it simplifies boundary normalization without colliding
  with the open PR set or reopening just-landed work.
