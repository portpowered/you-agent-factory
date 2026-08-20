# BOOT-PLAN-PARALLEL-004

## Identity

- Status: `NEEDS_ITERATION`
- Factory: `@you/plan-parallel`
- Repository base: `fb7805bd851656b20060a62f5ebb46f2e8cb0542`
- Worktree: `.artifacts/bootstrap/worktrees/BOOT-PLAN-PARALLEL-004`
- Model for all roles: provider `CODEX`, model `gpt-5.6-terra`
- Elapsed time: 142.3 seconds
- Recording `BOOT-PLAN-PARALLEL-004-R01.replay.json`:
  `CB66353C8226F8F22DDCF7D01974C70BD2654CF56E452BE03923670C3B49F10A`

## Results and root cause

The planner emitted exactly two independent evidence tasks, both executors ran
concurrently, and the terminal merger ran only after both children completed.
The customer outcome nevertheless failed: both executors returned task-planning
or specification text instead of investigating their assigned Work. The merger
correctly disclosed that evidence was missing rather than inventing it.

Runtime tracing established two deterministic causes. A non-empty workstation
prompt replaces the default input payload, while `execute-planned-task` did not
render `{{ (index .Inputs 0).Payload }}`. Separately, the scheduler retained
`ALL_CHILDREN_COMPLETE` child tokens only as observe-mode enablement bindings and
discarded them before building the merger dispatch. Thus the executor could not
reliably see its assignment and the merger could not directly see child results.

## Decision

- Representative status: `FAILED` despite a successful runtime terminal state.
- Goal status: `NEEDS_ITERATION` pending a fresh-worktree holdout.
- Prompt-only hardening is rejected as insufficient.
- Required regression proof: each executor receives exactly one unique assigned
  payload; the merger receives the original request and every completed child
  payload; observed children remain unconsumed canonical Work.
