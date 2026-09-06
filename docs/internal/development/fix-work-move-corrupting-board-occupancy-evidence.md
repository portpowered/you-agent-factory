# Work move restore-conflict characterization

Status: GATE-CHAR complete under the operator amendment. The supplied recording was replaced by an independently generated recording from a disposable isolated rig; the original 20,502-event/247-Work counts are not used as acceptance requirements.

Date: 2026-08-30
Generated parent SHA: `8442846cb31f225a8c128228235874aa518ce31c`
Environment: Windows `amd64`, Go `go1.25.0`

## Scope and dependency status

The operator amendment at `2026-08-30T21:02:01.631823-07:00` authorizes a self-generated recording and makes the supplied recording's event/Work counts non-binding. The generated artifact below is the evidence for the incident sequence. No raw recording content, prompt, model output, or secret was copied into this repository.

The committed sanitized rig inputs are:

- `docs/internal/development/fix-work-move-corrupting-board-occupancy-rig-factory.json`
- `docs/internal/development/fix-work-move-corrupting-board-occupancy-rig-work.json`

The transient generated prefix was `source-prefix.replay.json` under an isolated scratch directory. It had 15 events, bytes `186005`, and SHA-256 `C4D5C96DF59EB7BC8BB79CFDB4B43F438641E3FB9024ED03B4CAE8E5CC8DF2E1`. Its generated Factory was `board-occupancy-repro`; its only Work was `batch-board-occupancy-repro-request-current-terminal-after-move`. The artifact was loaded and projected before the diagnostic test was removed.

The available compact sibling fixture is:

`tests/functional/sessions/root_composition/testdata/automation-work-missing-occupancy.replay.json`

Its SHA-256 is `8F16213F606ED53FA88CA90AFF1DF9DA710F3091ECBFBFB419E6F320019EDE81`. It has ten ordered events (`RUN_REQUEST`, `INITIAL_STRUCTURE_REQUEST`, `SESSION_STARTED`, `FACTORY_STATE_RESPONSE`, `WORK_REQUEST`, ordinary `DISPATCH_REQUEST`, ordinary `DISPATCH_RESPONSE`, `WORK_STATE_CHANGE`, cron `DISPATCH_REQUEST`, and accepted cron `DISPATCH_RESPONSE`). The existing sibling functional test proves its supported automation-without-occupancy recovery; it remains separate from the generated incident artifact.

## Generated incident recording

The parent executable was built from the detached `origin/main` worktree, not from the operator checkout or `bin/you.exe`:

```text
git worktree add --detach origin/main <scratch>\parent
go build -o <scratch>\bin\you-verify.exe ./cmd/factory
```

The resulting parent binary was 76,016,640 bytes with SHA-256 `49C67325F3B8CC9284B88AE3EC3B0925B471F6558AF371DEBABB39CAF3BC9982`. The run used a copied `factory/` directory with the committed rig Factory JSON replacing `factory.json`, the committed batch Work JSON as `rig-work.json`, an isolated `HOME`/`USERPROFILE`, and loopback port `127.0.0.1:7439`. Ports `7437` and `7438` were not accessed.

The live generation command was:

```text
you-verify.exe run --dir <scratch>\prefix-run\factory --continuously --with-server --listen 127.0.0.1:7439 --with-mock-workers --work <scratch>\prefix-run\rig-work.json --record <scratch>\prefix-run\source-prefix.replay.json
```

After the Work appeared at `idea:complete`, these two operator moves were sent to the live server and both returned success:

```text
you-verify.exe --server http://127.0.0.1:7439 --json work move batch-board-occupancy-repro-request-current-terminal-after-move to-complete --request-id board-occupancy-move-1
you-verify.exe --server http://127.0.0.1:7439 --json work move batch-board-occupancy-repro-request-current-terminal-after-move to-complete --request-id board-occupancy-move-2
```

The recording was captured at a durable prefix after the second accepted dispatch and the disposable source process was then terminated. The JSON artifact parsed successfully and the Recordings loader accepted it. At the selected final tick, canonical projection produced current occupancy `idea:complete`, two immutable `WORK_STATE_CHANGE` records to `idea:to-complete`, and two accepted completed dispatches whose output Work was again `complete/TERMINAL`.

## Incident invariant and observed ordering

The generated recording observes the same behavior lane with a smaller controlled rig. The relevant canonical event order is:

| Sequence | Tick | Event | Observed value |
| ---: | ---: | --- | --- |
| 4 | 2 | `WORK_REQUEST` | Work ID `batch-board-occupancy-repro-request-current-terminal-after-move`, type `idea`, state `complete` |
| 5 | 3 | `WORK_STATE_CHANGE` | `idea:complete` -> `idea:to-complete`, request `board-occupancy-move-1` |
| 6 | 4 | `DISPATCH_REQUEST` | input is the incident Work |
| 9 | 6 | `DISPATCH_RESPONSE` | `ACCEPTED`; output is the incident Work at `complete/TERMINAL` |
| 10 | 7 | `WORK_STATE_CHANGE` | `idea:complete` -> `idea:to-complete`, request `board-occupancy-move-2` |
| 11 | 8 | `DISPATCH_REQUEST` | input is the incident Work |
| 14 | 10 | `DISPATCH_RESPONSE` | `ACCEPTED`; output is the incident Work at `complete/TERMINAL` |

The final canonical projection has exactly one current occupancy for the Work at `idea:complete`, while `WorkStateChangesByWorkID` retains both ordered move records with destination `idea:to-complete`. The accepted dispatch outputs independently corroborate the terminal Work value. The parent-red restore check then fails because the old resolver merges that historical destination into current occupancy; this is the value-level defect under test.

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

This proves the named failure and the regression identity at the restore boundary. It does not prove corrected resume/list behavior with the final built executable, CI, or merge. The temporary failing file was deleted before the detached worktree was removed; no failing test is part of this change.

## Generated parent-red restore check

To bind the generated event facts to the old runtime behavior, a temporary assertion used the projected values above (one current `idea:complete` occupancy plus two historical `idea:to-complete` moves) with a matching four-state `idea` topology. It was copied into the detached parent worktree only for this check and then deleted.

Command:

```text
go test ./pkg/services/factory_runtime/internal/services/orchestration/runtime -run '^TestTemporaryParentRedGeneratedOccupancy$' -count=1 -v
```

Exit: `0` because the assertion expects the parent to reject the generated recording-shaped state. Observed value:

```text
generated recording-shaped parent-red error: restore Factory Runtime Work board: restore Work board: Work "batch-board-occupancy-repro-request-current-terminal-after-move" has conflicting current places "idea:complete" and "idea:to-complete"
```

The same state is accepted by the fixed branch's resolver, which treats current occupancy as authoritative and retains the two move records for audit. This is a diagnostic bridge, not a committed test; the committed functional regression is the parent-red/head-green fixture in `tests/functional/sessions/board_occupancy_resume`.

## Reviewer regeneration instructions

From a clean checkout, use an isolated detached parent build and never use ports `7437` or `7438`:

```text
$scratch = Join-Path $env:TEMP 'board-occupancy-review'
git worktree add --detach origin/main "$scratch\parent"
New-Item -ItemType Directory -Force "$scratch\bin", "$scratch\run", "$scratch\run\home" | Out-Null
Push-Location "$scratch\parent"
go build -o "$scratch\bin\you-verify.exe" ./cmd/factory
Pop-Location
Copy-Item -Recurse -Force .\factory "$scratch\run\factory"
Copy-Item .\docs\internal\development\fix-work-move-corrupting-board-occupancy-rig-factory.json "$scratch\run\factory\factory.json"
Copy-Item .\docs\internal\development\fix-work-move-corrupting-board-occupancy-rig-work.json "$scratch\run\rig-work.json"
```

Run the source with `HOME` and `USERPROFILE` pointed at `$scratch\run\home`:

```text
$env:HOME = "$scratch\run\home"
$env:USERPROFILE = $env:HOME
& "$scratch\bin\you-verify.exe" run --dir "$scratch\run\factory" --continuously --with-server --listen 127.0.0.1:7439 --with-mock-workers --work "$scratch\run\rig-work.json" --record "$scratch\run\source-prefix.replay.json"
& "$scratch\bin\you-verify.exe" --server http://127.0.0.1:7439 --json work list
& "$scratch\bin\you-verify.exe" --server http://127.0.0.1:7439 --json work move batch-board-occupancy-repro-request-current-terminal-after-move to-complete --request-id board-occupancy-move-1
& "$scratch\bin\you-verify.exe" --server http://127.0.0.1:7439 --json work move batch-board-occupancy-repro-request-current-terminal-after-move to-complete --request-id board-occupancy-move-2
```

Wait for each accepted dispatch to be durable in the explicit recording, terminate only the disposable source process, and inspect the recording for the exact sequence/value table above. Then run the parent binary with `--resume <scratch>\run\source-prefix.replay.json --record <scratch>\run\parent-successor.replay.json`; the parent should expose the safe local-input failure while the fixed head should reach the successor server and list the Work once at `idea:complete`. Keep every output, log, recording, and home path under `$scratch`, and remove the detached worktree and scratch directory after inspection.

## Safety, ownership, and remaining edges

- A one-time check showed `7439=FREE`. Ports `7437` and `7438` were not accessed.
- `gh pr list --head fix-work-move-corrupting-board-occupancy --state all ...` returned `[]`; this branch had no existing PR to review.
- PR #2498 (`runtime-guard-terminal-tokens-against-late-dispatch-results`) is open and owns its terminal/failed-versus-active-dispatch production surface; it was not edited. PR #2320 (`fix(runtime): restore missing automation place occupancy`) is merged and its sibling behavior remains green.
- The generated source run used one detached parent executable and one disposable server process; no provider, paid call, or operator daemon was used. The committed functional package continues to use `root.BuildProcess`/`Process.Execute` and provider command-runner boundaries without OS spawns.
- Story 001 is now complete under the amendment. The remaining edge is Story 003's final fixed-head built-binary `--resume`/public `work list` proof, repository gates, rebase, push, PR, and CI start; those are not claimed by this characterization ledger.
