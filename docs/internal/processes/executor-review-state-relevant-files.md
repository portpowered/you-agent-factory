# Executor Review State Relevant Files

Use this map when investigating executor/review queue-state divergence for dynamic
workflow recovery lanes or reconciling same-trace task and review residue.

- `factory/factory.json` authors the `process` workstation outputs (`task:in-review`
  plus `review:init`) and the `review` workstation inputs/outputs for the standard
  `task:init -> process -> task:in-review -> review -> task:to-complete` lane.
- `factory/workstations/ideafy/AGENTS.md` documents the planner factory flow,
  `you work list`, and bounded `you work move` repairs for stranded task or idea
  tokens.
- `tests/functional/guards_batch/executor_review_state_lane_classification.go`
  classifies live queue snapshots into mismatch causes and planner dispositions
  and evaluates bounded manual-repair preconditions for failed-post-processing lanes.
- `tests/functional/guards_batch/executor_review_state_manual_repair_test.go`
  proves allowed and blocked manual-repair shapes plus `task:failed` -> `task:init`
  operator moves for the bounded repair path.
- `tests/functional/guards_batch/executor_review_state_reconcile_test.go`
  proves duplicate `review:init` collapse through the review workstation after
  executor/review reconcile wiring; stale `task:init`/`task:failed` cleanup is
  covered in `pkg/factory/subsystems/executor_review_reconcile_test.go`.
- `tests/functional/workflow/process_review_contract_long_test.go` proves the
  owning `process`/`review` workstation contract for continue, rejection, and
  loop-breaker paths.
- `pkg/cli/work/list.go` and `pkg/cli/work/move.go` are the operator boundaries
  for queue inspection and bounded manual repair.

## Residual Recovery Lane Disposition (2026-06-15 UTC)

Story 001 maps live queue symptoms for each named recovery lane. Evidence comes
from `you work list --session '~default' --name <lane> --json` against the default
factory session and from harness reproduction in
`executor_review_state_lane_evidence_test.go`.

| Lane | Key work IDs | Same-trace lineage | Live queue symptom | Mismatch cause | Planner disposition |
| --- | --- | --- | --- | --- | --- |
| `dynamic-workflows-recovery-session-backend-runtime` | idea `batch-dynamic-workflows-session-backend-recovery-20260614-dynamic-workflows-recovery-session-backend-runtime`, plan `work-plan-23`, task `work-task-24` | recovery trace `trace-dynamic-workflows-session-backend-recovery-20260614` (depth 1 idea → depth 2 plan → depth 3 task) | idea at `idea:to-complete`, plan `plan:complete`, task `task:failed` while worktree progress shows review-ready implementation | `failed_post_processing` | `safe_manual_repair` |
| `dynamic-workflows-recovery-mcp-install-plan-scope` | idea `batch-dynamic-workflows-mcp-install-plan-recovery-20260615-dynamic-workflows-recovery-mcp-install-plan-scope`, failed plan `work-plan-42`, spawned plan `batch-request-3cd2c196a6298845ba8df93e3da96747-dynamic-workflows-recovery-mcp-install-plan-scope`, task `work-task-58` | recovery trace `trace-dynamic-workflows-mcp-install-plan-recovery-20260615` plus spawned trace `trace-385de5ce7824a0a692250026d9388463` from setup-workspace | recovery idea still at `idea:to-complete` with `work-plan-42` failed; spawned trace has `plan:complete` and `work-task-58` `task:failed` while worktree shows completed install-path work | `failed_post_processing` on spawned trace; recovery-trace rows are `historical_residual_queue_state` | `safe_manual_repair` for spawned `work-task-58`; recovery-trace idea/plan rows are `superseded_queue_noise` |
| `dynamic-workflows-recovery-setup-workspace-git-pull-hygiene` | idea `batch-dynamic-workflows-setup-workspace-recovery-20260615-dynamic-workflows-recovery-setup-workspace-git-pull-hygiene`, failed plan `work-plan-49`, spawned plan `batch-request-ff69335cae42b911f05ad8e790fb207d-dynamic-workflows-recovery-setup-workspace-git-pull-hygiene`, task `work-task-59`, reviews `work-review-64`, `work-review-65`, `work-review-69` | recovery trace `trace-dynamic-workflows-setup-workspace-recovery-20260615` plus spawned trace `trace-e3bfbf2efbff251737c0df2a5433efb3` | spawned trace shows `work-task-59` at `task:in-review` with three active `review:init` tokens for the same trace while worktree progress shows completed stories | `duplicate_review_creation` on spawned trace; recovery-trace rows are `historical_residual_queue_state` | `needs_runtime_reconcile` for duplicate reviews; recovery-trace rows are `superseded_queue_noise` |

### How to read the mismatch

- **`failed_post_processing`**: executor or review work completed in the worktree,
  but the durable queue still shows `task:failed` or another blocking task state
  for the authoritative spawned trace. This is not projection drift; runtime
  marking and `you work list` agree on the failed or stranded task token.
- **`duplicate_review_creation`**: more than one active `review:init` token exists
  for the same spawned trace while the paired task remains in `task:in-review`.
  This is an active queue-shape defect on the owning `process`/`review` path,
  not merely historical residue.
- **`historical_residual_queue_state`**: recovery-batch trace rows (`idea:to-complete`
  plus `plan:failed`) remain in the durable queue after setup-workspace spawned a
  newer plan/task trace. Treat these as superseded relative to the spawned trace.
- **`superseded_queue_noise`**: rows that no longer own the lane's authoritative
  implementation outcome and should not block story 002 reconciliation work on
  the spawned trace.

### Worktree versus queue evidence

Worktree `progress.txt` files for the three lanes show completed implementation
and verification, which is why the classifier sets `worktreeComplete=true` for
live queue snapshots. The divergence is queue residue and duplicate review state,
not missing worktree delivery.

## Bounded Historical Manual Repair (Story 003)

Use `you work move` only when `evaluateExecutorReviewManualRepairPreconditions`
in `executor_review_state_lane_classification.go` returns true for the target
lane's live queue evidence. The helper is proven in
`executor_review_state_manual_repair_test.go`.

### Preconditions (all must be true)

1. Planner disposition is `safe_manual_repair` with mismatch cause
   `failed_post_processing`.
2. Worktree delivery for the lane is already complete (`worktreeComplete=true`
   in the classifier).
3. The authoritative trace (spawned trace when present, otherwise the recovery
   trace) shows exactly one `task:failed` token and a `plan:complete` token.
4. The authoritative trace has zero active `review:init` tokens (no duplicate
   review shape on the trace being repaired).
5. Recovery-trace `idea:to-complete` plus `plan:failed` rows on split-trace
   lanes are `superseded_queue_noise` and must not be moved.

### Allowed bounded move

When all preconditions hold, move only the authoritative failed task back to
`task:init` so the existing `process` workstation can re-enter the lane:

```sh
you work move --session '~default' <authoritative-task-work-id> init
```

Expected post-repair observable state:

- The failed task token moves from `task:failed` to `task:init`.
- No manual move touches recovery-trace idea or plan rows.
- Other lanes continue through the normal `process` / `review` path without
  bypassing review or deleting duplicate review tokens by hand.

### Unsafe counterexamples (do not manual move)

| Condition | Why manual move is unsafe |
| --- | --- |
| `duplicate_review_creation` / more than one active `review:init` on the authoritative trace | Story 002 `executorReviewReconcileMutations` owns duplicate collapse on process/review completion; manual deletion would bypass reconcile ownership. |
| `worktreeComplete=false` | Work may still be in flight; moving failed tasks would skip real executor or review work. |
| Recovery-trace `plan:failed` rows while spawned trace is authoritative | Those rows are superseded queue noise relative to the spawned trace outcome. |
| Multiple `task:failed` tokens on the authoritative trace | Ambiguous ownership; classify further before any operator move. |
| Lane disposition `needs_runtime_reconcile` | Runtime reconcile must run first; no manual shortcut replaces it. |

### Per-lane manual-repair outcome

| Lane | Manual move required? | Notes |
| --- | --- | --- |
| `dynamic-workflows-recovery-session-backend-runtime` | Yes, when live preconditions still hold | Move `work-task-24` from `failed` to `init` on recovery trace `trace-dynamic-workflows-session-backend-recovery-20260614`. |
| `dynamic-workflows-recovery-mcp-install-plan-scope` | Yes, when live preconditions still hold | Move spawned `work-task-58` from `failed` to `init` on trace `trace-385de5ce7824a0a692250026d9388463`; ignore recovery-trace rows. |
| `dynamic-workflows-recovery-setup-workspace-git-pull-hygiene` | **No** | Duplicate `review:init` on spawned trace `trace-e3bfbf2efbff251737c0df2a5433efb3` is owned by story 002 runtime reconcile; completing review (or a forward process output) collapses residue without `you work move`. |

## Follow-up ownership

- Story 002 owns runtime/projection reconciliation for duplicate `review:init`
  cleanup and stale task residue on the spawned trace via
  `executorReviewReconcileMutations` in `pkg/factory/subsystems/executor_review_reconcile.go`,
  wired from `subsystem_transitioner.go` after process/review accepted outputs.
  Reconcile keys duplicate `review:init` by same chaining trace **and** lane name
  (`TokenColor.Name`) so sibling lanes sharing one trace keep legitimate review work.
- Story 003 owns bounded manual-repair preconditions when investigation proves
  runtime behavior is already correct for a lane shape.
- Story 004 owns focused replay and lifecycle verification across executor
  completion, review completion, and the duplicate-review regression shape.
- `tests/functional/guards_batch/executor_review_state_lifecycle_test.go`
  proves completed-executor supersede, completed-review residue cleanup, full
  process→review terminal convergence, duplicate-review regression for lane three,
  replayed projection parity via `ReconstructFactoryWorldState`, and planner
  classification evidence for all three named PRD lanes.
- `pkg/factory/subsystems/executor_review_reconcile_test.go` includes transitioner
  pipeline coverage for both process-accept and review-accept reconcile paths.
