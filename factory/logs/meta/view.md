# meta view

## world state

- repository `HEAD` is `d0c4288` on `main` after `git pull --ff-only` on
  May 1, 2026, and `origin/main` is at the same commit.
- the canonical checked-in maintainer backlog is still
  `factory/logs/meta/asks.md`; no item in that file is marked urgent.
- the checked-in workflow inboxes still contain only tracked `.gitkeep`
  sentinels:
  - `factory/inputs/BATCH/default/.gitkeep`
  - `factory/inputs/idea/default/.gitkeep`
  - `factory/inputs/plan/default/.gitkeep`
  - `factory/inputs/task/default/.gitkeep`
  - `factory/inputs/thoughts/default/.gitkeep`
- this workspace still has ignored local residue under the inbox surface:
  - `factory/inputs/idea/default/align-process-review-loop-contract.md`
  - `factory/inputs/idea/default/dedupe-dispatcher-throttle-pause-filter.md`
  - `factory/inputs/idea/default/systems-cleanup.md`
  - `factory/inputs/idea/default/test-cleanup.md`
  - `factory/inputs/task/default/ci-cd.md`
- the recent GitHub lane state on May 1, 2026 is now:
  - open PR `#22` `align-process-review-loop-contract`
  - merged PR `#21` `dedupe-dispatcher-throttle-pause-filter`
  - open PR `#20` `test-cleanup`
  - merged PR `#19` `systems-cleanup`
  - merged PR `#18` `api-clean`
  - merged PR `#17` `ci-cd`
  - open PR `#16` `dedupe-root-factory-artifact-contract-entries`
  - open PR `#4` `standardize-contract-guard-skip-policy`
- the narrow throttle dedupe follow-up is no longer pending:
  - `pkg/factory/subsystems/subsystem_dispatcher.go` no longer carries the
    redundant post-scheduler pause filter removed by PR `#21`
  - `pkg/factory/projections/throttle_pause_projection.go` now owns the
    projection path for throttle-pause observability
  - `pkg/service/factory.go` now enriches the rendered world view through the
    throttle-pause projection wrapper rather than the base world-view builder
  - `docs/development/dispatcher-throttle-pause-audit-closeout.md` records the
    closeout evidence and the deliberately deferred broader redesign
- the CI/CD ask has partially landed:
  - `.github/workflows/ci.yml` now exists on `main`
  - `README.md` and `docs/development/development.md` document the repository
    CI workflow and local reproduction commands
- the functional-test cleanup ask also advanced:
  - `docs/development/functional-test-cleanup-closeout.md` records the
    remaining behavioral coverage
  - workstation prompts now explicitly steer away from structural/meta tests
- the historical replay still shows a live workflow-contract mismatch in the
  checked-in maintainer loop:
  - `process` completions in `factory/logs/agent-fails.replay.json`:
    `9 ACCEPTED <COMPLETE>`, `27 REJECTED <CONTINUE>`
  - `review` completions: `5 ACCEPTED <COMPLETE>`, `4 REJECTED <REJECTED>`
  - `factory/workers/processor/AGENTS.md` still configures stop token
    `<COMPLETE>` only
  - `factory/workstations/process/AGENTS.md` still instructs the executor to
    return `<CONTINUE>` when only one story iteration is done
  - `factory/factory.json` still maps `process` rejection back to `task:init`
    and `review` rejection back to `task:init`
- that mismatch is actively being worked in open PR `#22`:
  - the PR is `MERGEABLE` as of May 1, 2026
  - its diff reaches the expected contract surfaces:
    `factory/factory.json`, `factory/workers/processor/AGENTS.md`,
    `factory/workstations/process/AGENTS.md`, replay fixtures, generated API
    surfaces, and focused behavioral coverage
  - the cleanup lane should not be re-queued while `#22` is still open
- the broader throttle customer ask remains open at the architecture level:
  - the current system still keeps pause state as dispatcher-owned runtime
    memory keyed by provider/model
  - the ask for config-authored `factory.guards` with
    `INFERENCE_THROTTLE_GUARD` is not implemented on `main`
  - that redesign should now be considered separately from the already-landed
    duplicate-filter cleanup
- there is another narrow cleanup lane ready after the process/review contract
  work merges:
  - the work-request trace alias normalization path is duplicated across
    `pkg/api/handlers.go`, `pkg/factory/work_request.go`, and
    `pkg/factory/work_request_json.go`
  - both the `currentChainingTraceId` vs `traceId` conflict rule and the
    `current-or-legacy` fallback helper are implemented more than once
  - this is distinct from the currently open PR set and fits the preferred
    dead-code / legacy-handling cleanup direction

## current blockers

1. the checked-in world view had drifted behind `HEAD` again and still
   described `main` as `d55b57e` without the new open PR `#22`.
2. `main` still has the process/review contract mismatch until `#22` merges,
   even though the fix is already in flight.
3. the broad `INFERENCE_THROTTLE_GUARD` customer ask is still too large for a
   safe direct jump from the current dispatcher-owned runtime policy.

## theory of mind

- the repository state has advanced in the right direction, but the checked-in
  meta model must keep pace with active PR lanes as well as merged ones.
- the highest-signal maintainer-loop issue on `main` is still the
  `<CONTINUE>`/rejection mismatch, but it is no longer an unowned problem:
  PR `#22` already covers the intended contract alignment with code, docs, API,
  and focused coverage.
- the best next cleanup idea should therefore avoid the active PR set and stay
  narrow.
- the next defensible cleanup after `#22` is to centralize work-request trace
  alias normalization so legacy `traceId` handling lives in one place instead
  of being duplicated across API and factory parsing paths.
- the right follow-up for the broader throttling ask is still a later,
  dedicated `INFERENCE_THROTTLE_GUARD` design lane rather than another narrow
  dispatcher tweak.

## next best move

- update the checked-in meta world model and progress log now.
- do not queue another process/review contract idea while PR `#22` is open,
  because that would duplicate an active cleanup lane.
- queue one new ignored cleanup idea for the distinct trace-normalization
  duplication so the next worker has a ready follow-up once the current PR
  stack clears.
- leave the broader `INFERENCE_THROTTLE_GUARD` redesign for a later dedicated
  lane after the maintainer loop semantics are cleaner.

## customer asks

- `factory/logs/meta/asks.md` remains the only checked-in backlog surface.
- no ask is marked urgent as of May 1, 2026.
- the throttling ask is still active, but the already-merged PR `#21` closed
  only the narrow duplicate-filter cleanup slice.
- the best next customer-facing cleanup lane on `main` is still the
  process/review contract alignment, but that work is already active in PR
  `#22`.
- after that lands, the next narrow cleanup lane should be the duplicated
  work-request trace normalization path rather than another overlapping control-
  plane request.
