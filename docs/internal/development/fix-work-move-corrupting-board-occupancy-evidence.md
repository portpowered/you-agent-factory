# Work move restore-conflict characterization

Status: GATE-CHAR is partially complete and blocked on the supplied recording artifact. This ledger records the executable evidence that was available before any production edit.

Date: 2026-08-30
Parent SHA: `42eeee4472656b8290f798c36a5b8c871b24d7d0`
Environment: Windows `amd64`, Go `go1.25.0`

## Scope and dependency status

The PRD identifies a 20,502-event/247-Work supplied recording and requires exact verification of sequences `15713`, `15721`, `15867`, `15963`, and `19260`. No path to that artifact is present in `prd.json`, this checkout, the sibling worktrees, Downloads, Desktop, or the temporary directory. The repository search found no JSON/replay artifact at least 20 MB in the checkout; the user-provided tokenizer JSON files in Downloads were unrelated and were not read.

Because the supplied artifact is unavailable, the exact sequence/state/place claims below are recorded as the PRD's incident description, not as verified recording evidence. The missing artifact is a mandatory dependency for the supplied-recording half of this story; its SHA-256 and exact event payloads must be added before GATE-CHAR can be marked complete. No raw recording content or prompts were copied into this repository.

The available compact sibling fixture is:

`tests/functional/sessions/root_composition/testdata/automation-work-missing-occupancy.replay.json`

Its SHA-256 is `8F16213F606ED53FA88CA90AFF1DF9DA710F3091ECBFBFB419E6F320019EDE81`. It has ten ordered events (`RUN_REQUEST`, `INITIAL_STRUCTURE_REQUEST`, `SESSION_STARTED`, `FACTORY_STATE_RESPONSE`, `WORK_REQUEST`, ordinary `DISPATCH_REQUEST`, ordinary `DISPATCH_RESPONSE`, `WORK_STATE_CHANGE`, cron `DISPATCH_REQUEST`, and accepted cron `DISPATCH_RESPONSE`). The existing sibling functional test proves its supported automation-without-occupancy recovery; it is not the missing 20,502-event incident artifact.

## Incident invariant and expected ordering

The PRD describes two valid operator moves of the incident Work from `idea:complete` to `idea:to-complete`, followed by accepted dispatch results that return the Work to `idea:complete`; sequence `19260` is the latest named event. That ordering explains why the current projected occupancy at `idea:complete` is authoritative while the move records remain historical audit facts. Exact states, places, Work ID, event types, outcomes, and per-sequence payloads cannot be verified without the supplied file.

## Current code path

The relevant boundaries at the parent SHA are:

- `pkg/services/factory_runtime/internal/services/orchestration/runtime/factory.go:614-669` accepts an operator move, records the `WorkStateChange` through the runtime event-history port, and preserves source/request metadata.
- `pkg/services/factory_runtime/internal/services/orchestration/engine/work_move.go:23-82` finds the live Work token, rejects an in-flight dispatch, resolves the target place, applies the synchronous marking mutation, and returns the move result.
- `pkg/services/recordings/internal/projections/world_state_dispatch.go:643-699` records immutable work-state-change history and applies the destination token to the projected world state.
- `pkg/services/recordings/internal/projections/world_state.go:31-40` reconstructs the canonical world state, and `:733-752` rebuilds `PlaceOccupancyByID` from the projected token index. This is the source of the current placement used by restore.
- `pkg/services/factory_runtime/internal/services/orchestration/runtime/clean_invocation.go:263-291` builds and validates the restored Work state. `:293-355` rejects a Work that is actually present in two current occupancy places.
- `pkg/services/factory_runtime/internal/services/orchestration/runtime/clean_invocation.go:651-687` currently builds `restoredWorkPlacements` by adding current occupancy, active-dispatch inputs, historical work-state-change destinations, then completed-dispatch candidates. The history helper is called after occupancy and reports a conflict rather than treating history as fallback.
- `pkg/services/factory_runtime/internal/services/orchestration/runtime/restored_work_recovery.go:166-181` derives current occupancy candidates, `:247-264` selects the latest historical destination, and `:303-312` emits the shared `has conflicting current places` error.

The resulting parent behavior is the specific gap: a current `idea:complete` occupancy and an older valid move destination `idea:to-complete` are merged into one placement map as simultaneous current places.

## Focused coverage inventory

The following existing owner tests were run before production changes:

```text
go test ./pkg/services/factory_runtime/internal/services/orchestration/runtime -run 'TestNew_(RestoresOtherWorkWhenCompletedCronWorkHasNoOccupancy|RestoresCompletedWorkFromCanonicalDispatchOutputWhenOccupancyIsMissing|RestoredWorkStateChangeSupersedesCompletedDispatchOutputPlacement|DoesNotRedispatchCompletedAutomationWithCanonicalCompletionPlacement|RejectsConflictingCurrentWorkPlacements)' -count=1 -v
```

Result: exit `0`; all five selected tests passed in `0.127s`. The last selector is the current duplicate/conflict characterization already present in the owner suite; it passes because the parent rejects the conflict that the new behavior must distinguish from a genuine duplicate current occupancy.

```text
go test ./pkg/services/recordings/internal/projections/... -run 'TestReconstructFactoryWorldState_(WorkStateChangeMovesFromFailedToInProgress|WorkStateChangeMovesFromInitialToArbitraryState|OutputWorkStateReconstructsTerminalPlaceWithoutExplicitOutputPlace)' -count=1 -v
```

Result: exit `0`; the three selected projection tests passed in `projectiontests` (`0.145s`). Other matched packages reported no tests and a successful package result.

```text
go test ./tests/functional/sessions/root_composition -run '^TestAutomationWorkWithoutRecordedOccupancyRestoresThroughRecordingProjection$' -count=1 -v
```

Result: exit `0`; the existing PR #2320 sibling recovery test passed in `4.557s`. That live-owned package was not edited.

The new functional package requested by the PRD is reserved for the implementation story: `tests/functional/sessions/board_occupancy_resume`. The nearest `root_composition` and `resume_from_recording` packages remain live-owned for this characterization slice. No OS-spawn baseline or intentional-OS inventory was changed.

## Isolated parent-red reproduction

The temporary test-only assertion was applied in a detached worktree at the parent SHA and then removed. Its compact state fixture contained one terminal Work at current place `task:done` and one historical move record whose latest destination was `task:init`. The assertion attempted to construct the runtime and then read the engine marking at `task:done`, which represents the later public Work/list assertion without changing production code.

Temporary test file SHA-256: `A1243D47AC752B2533FEE1FA3EEBE98A6B39A75638F0A405BBF940EF0E47E1FC`

Patch-stream hash: `1ef317a2ee94a2c710b31d9a68294241a73acbcc`

Command:

```text
go test ./pkg/services/factory_runtime/internal/services/orchestration/runtime -run '^TestCharacterizeCurrentOccupancyDoesNotConflictWithMoveHistory$' -count=1 -v
```

Exit: `1` (expected parent-red result).

Sanitized output:

```text
=== RUN   TestCharacterizeCurrentOccupancyDoesNotConflictWithMoveHistory
    restore_move_corruption_characterization_test.go:54: resume before Work/list snapshot: restore Factory Runtime Work board: restore Work board: Work "work-current-terminal-after-move" has conflicting current places "task:done" and "task:init"
--- FAIL: TestCharacterizeCurrentOccupancyDoesNotConflictWithMoveHistory
FAIL
```

This proves the named failure and the regression identity at the restore boundary. It does not prove corrected resume/list behavior, the supplied recording, a built executable, CI, or merge. The temporary failing file was deleted before the detached worktree was removed; no failing test is part of this change.

## Safety, ownership, and remaining edges

- A one-time check showed `7439=FREE`. Ports `7437` and `7438` were not accessed.
- `gh pr list --head fix-work-move-corrupting-board-occupancy --state all ...` returned `[]`; this branch had no existing PR to review.
- PR #2498 (`runtime-guard-terminal-tokens-against-late-dispatch-results`) is open and owns its terminal/failed-versus-active-dispatch production surface; it was not edited. PR #2320 (`fix(runtime): restore missing automation place occupancy`) is merged and its sibling behavior remains green.
- The characterization used the runtime test helper directly and did not spawn an OS process. The new functional package will use `root.BuildProcess`/`Process.Execute` and provider command-runner boundaries in the implementation story.
- The next story must change placement precedence so valid current occupancy wins, retain move history, preserve active-dispatch precedence, and keep genuine duplicate current occupancy and unsupported missing occupancy fail-closed. The supplied recording path/hash and exact sequence payloads remain the blocking evidence delta for this story.
