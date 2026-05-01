# meta view

## world state

- repository `HEAD` is `ca942db` on `main` on May 1, 2026, and
  `origin/main` is the same commit after `git pull --ff-only`
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
  - `factory/inputs/idea/default/align-process-review-loop-contract.md`
  - `factory/inputs/idea/default/centralize-work-request-trace-normalization.md`
  - `factory/inputs/idea/default/consolidate-public-factory-enum-alias-ownership.md`
  - `factory/inputs/idea/default/dedupe-dispatcher-throttle-pause-filter.md`
  - `factory/inputs/idea/default/dedupe-generated-boundary-alias-rejection-coverage.md`
  - `factory/inputs/idea/default/dedupe-retired-boundary-alias-rejection-tables.md`
  - `factory/inputs/idea/default/retire-scriptwrap-build-args-shim.md`
  - `factory/inputs/idea/default/systems-cleanup.md`
  - `factory/inputs/idea/default/test-cleanup.md`
  - `factory/inputs/task/default/ci-cd.md`
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
- the previous generated-boundary alias cleanup lane is now complete on `main`:
  - PR `#27` merged on May 1, 2026
  - `pkg/config/openapi_factory_test.go` no longer carries the redundant
    retired generated-boundary alias rejection cases
  - `pkg/config/factory_config_mapping_test.go` remains the canonical owner for
    that rejection seam, including nested `definition` and `definition.cron`
    paths
- the broader throttle customer ask remains open at the architecture level:
  - pause state still lives as dispatcher-owned runtime memory keyed by
    provider/model in `pkg/factory/subsystems/subsystem_dispatcher.go`
  - the ask for config-authored `factory.guards` with
    `INFERENCE_THROTTLE_GUARD` is still not implemented on `main`
- one inspected narrow API cleanup candidate is not ready for queueing:
  - `pkg/api/server.go` registers a handwritten `/work` route that forwards to
    `ListWork`
  - that shim intentionally preserves tolerant `maxResults` parsing before the
    generated server's stricter integer binding runs
  - removing it would change current public request tolerance, so it is not a
    pure dead-code cleanup

## current blockers

1. the broad `INFERENCE_THROTTLE_GUARD` customer ask is still too large for a
   safe direct jump from the current dispatcher-owned runtime policy
2. open PRs `#20`, `#16`, and `#4` still occupy their respective file sets, so
   new cleanup work should stay outside those lanes
3. the most recent previously queued narrow cleanup lane already landed as PR
   `#27`, so any world model still pointing to it is stale
4. the `/work` pagination shim is behavior-bearing compatibility code today,
   not confirmed dead code

## theory of mind

- the checked-in meta model drifted again because it still treated
  `dedupe-generated-boundary-alias-rejection-coverage` as the next lane even
  though PR `#27` has already landed on `main`
- ignored `factory/inputs/**` residue can outlive the merge status of the lane
  it originally described, so merged PR history must continue to win over both
  local residue and the previous checked-in meta view
- the highest-value customer problem is still the throttle redesign, but the
  live implementation spans dispatcher runtime pause state, runtime snapshots,
  and transition enablement behavior, so it still needs decomposition before a
  safe executable request can be queued
- an apparent duplicate path is not automatically cleanup-ready:
  the `/work` router shim looks redundant structurally, but the inline comment
  and current code show it preserves tolerant public pagination parsing that
  the generated binding would otherwise reject
- the right immediate maintainer action is to refresh the checked-in world
  model and avoid fabricating another cleanup idea until a distinct,
  behavior-safe seam is confirmed

## next best move

- update the checked-in meta world model and progress log now
- do not queue another generated-boundary alias, retired-boundary alias,
  worker build-args, enum-alias, trace-normalization, or process/review cleanup
  idea, because those lanes already landed on `main`
- leave `factory/logs/meta/asks.md` unchanged for now; no ask is urgent and the
  throttle redesign still needs a narrower executable decomposition
- use the next meta iteration either to decompose the
  `INFERENCE_THROTTLE_GUARD` ask into a smaller guard-design lane or to find a
  fresh cleanup seam that does not alter public behavior

## customer asks

- `factory/logs/meta/asks.md` remains the only checked-in backlog surface
- no ask is marked urgent as of May 1, 2026
- the throttling ask is still active, but no checked-in change has yet moved
  pause enforcement into config-authored guards
- the next customer-adjacent architectural step is still the future
  `INFERENCE_THROTTLE_GUARD` lane, but it should be queued only after it is
  decomposed into a narrow, non-overlapping request
