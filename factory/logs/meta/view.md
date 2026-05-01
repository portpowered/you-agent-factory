# meta view

## world state

- repository `HEAD` is `c8735e0` on
  `ralph/align-process-review-loop-contract` on May 1, 2026, and
  `origin/main` is `606f6c3`.
- the canonical checked-in maintainer backlog is still
  `factory/logs/meta/asks.md`; no item in that file is marked urgent.
- the checked-in workflow inboxes still contain only tracked `.gitkeep`
  sentinels:
  - `factory/inputs/BATCH/default/.gitkeep`
  - `factory/inputs/idea/default/.gitkeep`
  - `factory/inputs/plan/default/.gitkeep`
  - `factory/inputs/task/default/.gitkeep`
  - `factory/inputs/thoughts/default/.gitkeep`
- the workspace-local `factory/inputs/**` surface currently shows only the
  tracked `.gitkeep` sentinels, so there is no active ignored inbox residue to
  factor into the next lane choice.
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
- the checked-in replay evidence now reflects the landed continue-versus-
  rejection contract in the maintainer loop:
  - `process` completions in `factory/logs/agent-fails.replay.json`:
    `9 ACCEPTED <COMPLETE>`, `27 CONTINUE <CONTINUE>`
  - `review` completions: `5 ACCEPTED <COMPLETE>`, `4 REJECTED <REJECTED>`
  - the embedded `process` workstation topology now includes `onContinue`
  - the embedded `process` prompt now reserves rejection for true review
    send-back and treats `<CONTINUE>` as ordinary executor iteration
  - `review` still uses rejection to send work back through `task:init`
- that cleanup is still under review in open PR `#22`
  `align-process-review-loop-contract`, whose diff now includes the aligned
  runtime, prompts, replay fixtures, docs, and focused behavioral coverage.
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

1. open PR `#22` still needs to be merged back to `main` before the cleaned-up
   process/review contract becomes the repository default there.
2. the broad `INFERENCE_THROTTLE_GUARD` customer ask is still too large for a
   safe direct jump from the current dispatcher-owned runtime policy.

## theory of mind

- the repository state has advanced in the right direction, but the checked-in
  meta model must keep pace with active PR lanes as well as merged ones.
- the highest-signal maintainer-loop issue is now implemented on this branch
  and in its checked-in replay evidence, but it does not become repository
  truth until PR `#22` merges.
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
  lane now that the maintainer loop semantics are cleaner.

## customer asks

- `factory/logs/meta/asks.md` remains the only checked-in backlog surface.
- no ask is marked urgent as of May 1, 2026.
- the throttling ask is still active, but the already-merged PR `#21` closed
  only the narrow duplicate-filter cleanup slice.
- the best next customer-facing cleanup lane after `#22` lands should be the
  duplicated work-request trace normalization path rather than another
  overlapping control-plane request.
