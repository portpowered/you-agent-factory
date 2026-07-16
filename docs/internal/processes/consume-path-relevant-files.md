# Consume Path Relevant Files

Use this map when changing terminal reviewed-lane completion through the
`consume` workstation or investigating stranded `idea:to-complete` /
`task:to-complete` pairs.

- `factory/factory.json` authors the checked-in `consume` workstation with
  guarded `idea:to-complete` first and unguarded `task:to-complete` second.
- `pkg/config/maptests/config_mapper_input_guards_test.go` proves mapper output
  for idea-first consume ordering with auto-attached `DependencyGuard` on the
  task arc.
- `pkg/factory/scheduler/enablement.go` defers peer-binding guards (`SAME_NAME`,
  `SAME_TRACE_ID`, bound `MATCHES_FIELDS`) until independent guards bind their
  arcs; regression coverage lives in `pkg/factory/scheduler/enablement_test.go`.
- `tests/functional/guards_batch/same_name_consume_path_ownership_test.go`
  classifies stranded outcomes into `runtime_correct`,
  `historical_queue_artifact`, and `projection_visibility_gap` using runtime
  marking plus reconstructed projection queue visibility.
- `tests/functional/guards_batch/same_name_consume_path_regression_test.go`
  proves reviewed same-name pairs complete without stranding new task tokens
  for built-in consume input order and blocks mismatched names.
- `tests/functional/guards_batch/same_name_consume_path_historical_recovery_test.go`
  bounds duplicate-history and queue-artifact recovery, documents bounded
  historical manual-repair preconditions, and proves unrelated reviewed lanes
  still complete when an orphan task token remains.
- `tests/functional/guards_batch/same_name_consume_path_cell_disposition_test.go`
  leaves reviewer-verifiable disposition evidence for the three PRD stranded
  cells using observable runtime and projection outcomes.
- `internal/testutil/service_harness.go` exposes `MoveWork` for focused operator-move
  recovery tests through the service layer.
- `pkg/transports/cli/work/move.go` and `pkg/service/runtime_sessions.go` are the CLI/API
  operator-move boundaries for bounded historical cleanup.

## Bounded Historical Manual Repair

Use `you work move` only when all observable preconditions are true for the
target cell name:

1. Runtime marking shows `idea:complete` for the cell name.
2. Runtime marking shows exactly one orphan `task:to-complete` token for the
   same cell name.
3. Runtime marking has no `idea:to-complete` token for that cell name.
4. Projection queue visibility matches runtime marking for both consume inputs
   (not a `projection_visibility_gap`).

Expected post-repair observable state:

- The orphan task token moves to `task:complete`.
- No new `task:to-complete` token remains for that cell name.
- Other reviewed lanes with live same-name pairs continue to complete through
  the existing consume path without bypassing `SAME_NAME` safeguards.

This move is a bounded historical cleanup for duplicate task residue left after
a successful consume. It is not a generic shortcut for future reviewed lanes
where both twins are still queued at `to-complete`.

When preconditions are not met (for example task-only residue without
`idea:complete`, or both twins still queued for consume), do not apply a manual
move; classify the cell with the ownership tests above and queue a bounded
runtime or projection repair instead.

## Reviewed CLI and MCP Cell Disposition

Story 004 maps the live queue symptom for each PRD cell to a follow-up planner
disposition. Evidence comes from
`same_name_consume_path_cell_disposition_test.go`, which reproduces the orphan
`task:to-complete` pattern (successful consume plus duplicate task residue) and
evaluates runtime marking plus projection queue parity.

| Cell | Live queue symptom | Ownership layer | Disposition | Follow-up action |
| --- | --- | --- | --- | --- |
| `dynamic-workflows-cell-cli-validate-list` | `task:to-complete` stranded; idea twin hidden at `idea:complete` | `historical_queue_artifact` | `needs_bounded_manual_move` | `you work move <orphan-work-id> complete` when bounded preconditions hold |
| `dynamic-workflows-cell-cli-run-status-result` | same orphan pattern | `historical_queue_artifact` | `needs_bounded_manual_move` | same bounded manual move |
| `dynamic-workflows-cell-mcp-tools` | same orphan pattern | `historical_queue_artifact` | `needs_bounded_manual_move` | same bounded manual move; post-repair disposition is `complete` |

Fresh reviewed same-name pairs for these cell names complete through the
repaired consume path without manual intervention (`cellDispositionComplete` in
the disposition test). The stranded live cells are historical residue from
duplicate task submissions after an earlier successful consume, not a live
runtime defect after story 002 enablement repair.

Expected post-repair observable state for each manual move:

- Orphan `task:to-complete` token moves to `task:complete`.
- No `task:to-complete` token remains for the cell name.
- `idea:complete` remains unchanged for the hidden idea twin.
- Other reviewed lanes continue to complete through the existing consume path.
