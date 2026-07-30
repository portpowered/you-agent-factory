# BOOT-PLAN-PARALLEL-002

## Identity

- Status: `NEEDS_ITERATION`
- Factory: `@you/plan-parallel`
- Repository base: `3c9615a40`
- Worktree: `.artifacts/bootstrap/worktrees/BOOT-PLAN-PARALLEL-002`
- Model for all roles: provider `CODEX`, model `gpt-5.6-terra`
- Recording `BOOT-PLAN-PARALLEL-002-R01.replay.json`:
  `F5AC9DA0132B8BBAD7FB41FFC88E09C15E4494088F455980CE5FC6B877D5FCA3`

## Results and required iteration

The planner emitted a valid five-item `FACTORY_REQUEST_BATCH`: four independent
evidence tasks and one dependent synthesis task with four `DEPENDS_ON` edges.
All four evidence tasks dispatched concurrently and completed. The dependent
synthesis dispatched only after its prerequisites, but received an intermittent
provider `permanent_bad_request`, so the parent failed and the terminal merger
did not fabricate a result.

The graph exposed a factory-shape inefficiency: the generated synthesis task
duplicated the packaged `merge-plan-results` workstation, which already owns
all-child fan-in and customer-facing synthesis. The package prompt is therefore
being tightened to forbid catch-all synthesis/merge tasks while retaining
dependencies for genuine execution prerequisites. A fresh worktree holdout is
required after generation and tests.

## Decision

- Representative status: `FAILED`.
- Goal status: `NEEDS_ITERATION` pending the simplified-graph holdout.
- Failure propagation and merge gating behaved correctly.
