# Late dispatch result characterization

Status: `runtime-guard-terminal-tokens-against-late-dispatch-results-001` passes. This is a test-only enabler; no production, OpenAPI, generated, or public-process behavior changed.

## Deterministic harness

- `engine/engine_test_helpers_test.go` adds `deterministicResultGate`, a package-local closed-channel handshake guarded by `sync.Once`.
- `engine/engine_dispatch_test.go` holds the result inside the dispatch-result hook, moves the same Work token to `task:complete`, then releases the result and records the pre-fix result path's completion behavior.
- `subsystems/history_transitioner_pipeline_test.go` characterizes `ACCEPTED`, `CONTINUE`, `REJECTED`, `FAILED`, and `CANCELED` routing and completion values. The canceled case models a consumed token as absent from the marking with a held `CONSUME` claim, so restoration is asserted against the actual dispatch boundary.

The harness uses no sleeps, retries, eventually loops, timeout inflation, skips, source-scanning/meta tests, or unsafe behavior as an expected result.

## Controlled pre-fix failure

Parent commit: `47285a02c116394b0bdf3078cdd59909dc12e825`.

In a detached worktree at that SHA, the characterization diff was applied together with this transient-only assertion (it was not committed):

```go
failed := snapshot.Marking.TokensInPlace("task:failed")
if len(failed) != 0 {
	t.Fatalf("late dispatch result mutated terminal Work into failed state: got %d failed token(s)", len(failed))
}
```

Command, `-count=1` (one run), Go `go1.25.0 windows/amd64`:

```text
go test ./pkg/services/factory_runtime/internal/services/orchestration/subsystems ./pkg/services/factory_runtime/internal/services/orchestration/engine ./pkg/services/factory_runtime/internal/services/orchestration/runtime -run 'Test.*(LateDispatch|TerminalResult|MoveWork)' -count=1
```

Exit code: `1`.

Exact observed output:

```text
ok  	github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/subsystems	0.050s
--- FAIL: TestEngine_LateDispatchResultGateOrdersTerminalPlacementBeforeRelease (0.00s)
    engine_dispatch_test.go:400: late dispatch result mutated terminal Work into failed state: got 1 failed token(s)
FAIL
FAIL	github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/engine	0.073s
ok  	github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/runtime	0.093s
FAIL
```

This proves the pre-fix failure identity: after terminal placement, the stale failed result creates one `task:failed` token. It does not prove the later guard, diagnostic event, public move wiring, or replay behavior; those remain explicitly assigned to `TASK-002/GATE-UNIT`, `TASK-003/GATE-FUNC`, and `TASK-004/GATE-REPLAY`.

## Working-head verification

Story commit: `8614bd5e50` (`test(runtime): characterize late dispatch result ordering`).

Declared focused procedure at the working head:

```text
go test ./pkg/services/factory_runtime/internal/services/orchestration/subsystems ./pkg/services/factory_runtime/internal/services/orchestration/engine ./pkg/services/factory_runtime/internal/services/orchestration/runtime -run 'Test.*(LateDispatch|TerminalResult|MoveWork)' -count=1
```

Result: exit `0`; all three packages passed.

The same command with `-race` also passed for all three packages; no race or deadlock report was observed for the new handshake. The focused neighboring suites also passed:

```text
go test ./pkg/services/factory_runtime/internal/services/orchestration/subsystems -run 'TestHistoryTransitionerPipeline_|TestTransitioner_CanceledDispatchRestoresConsumedWorkWithoutFailureRoute' -count=1
go test ./pkg/services/factory_runtime/internal/services/orchestration/engine -run '^TestMoveWork_|^TestEngine_LateDispatchResultGateOrdersTerminalPlacementBeforeRelease$' -count=1
go test ./pkg/services/factory_runtime/internal/services/orchestration/runtime -run '^TestMoveWork_|^TestControlMoveWork_MapsDuplicateRequestIDToRootConflict$' -count=1
go test ./pkg/services/factory_runtime/internal/services/dispatch_planning/internal/service -run 'TestRetire(AcceptsEachTerminalOutcomeExactlyOnce|RejectsUnknownAndConflictingResultsWithoutMutation|RejectsResultForUnpublishedIntent)' -count=1
```

## Existing contract inventory

- `engine/work_move_test.go`: valid `init -> complete`; missing `ErrMoveWorkNotFound`; invalid target `ErrMoveWorkInvalidState`; active dispatch `ErrMoveWorkInFlightDispatch`; terminated engine `ErrMoveWorkEngineTerminated`; duplicate dispatch/request behavior; and zero dispatch-handler calls with no dispatch event for a successful move.
- `runtime/factory_move_work_idempotency_test.go`: duplicate request returns `ErrMoveWorkRequestAlreadyApplied`, while the control boundary maps it to `ErrMoveWorkRequestConflict`.
- `runtime/factory_move_work_test.go`: public active-dispatch visibility and terminal worker cancellation/process-gone release behavior remain covered by the existing tests; public active-dispatch MoveWork currently reports `ErrMoveWorkNotFound` because the consumed token is not in the marking.
- `subsystems/history_transitioner_pipeline_test.go` and `subsystems/subsystem_history_test.go`: accepted, continue, rejected, failed, fallback/requeue, and canceled restoration routes use the consumed dispatch token identity.
- `dispatch_planning/internal/service/service_test.go`: each terminal retirement is accepted once, duplicate retirement is idempotent, unknown/conflicting results do not mutate the intent, and unpublished results are rejected.

The direct engine terminal placement in the harness is deliberate: current `MoveWork` refuses an active dispatch with `ErrMoveWorkInFlightDispatch`, while invalidation is the later `TASK-003` behavior. This slice isolates the result-time ordering race without broadening production scope.
