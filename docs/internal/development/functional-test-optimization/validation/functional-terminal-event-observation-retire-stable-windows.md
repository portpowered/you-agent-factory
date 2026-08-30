# Validation report: functional terminal event observation and stable-window retirement

## Environment and artifact

- Validated source commit: `4ebf7591552058c2075debe08dde0f38421e8779`; all
  loopback checks below ran from a detached clean worktree at this exact head.
  This report records that read-only validation and does not change executable
  source.
- Environment and configuration: Windows 11 Home build `26200`, Go
  `go1.25.0 windows/amd64`, PowerShell, repository `go.mod`, no remote or
  paid-provider credentials used.
- Customer entry point: local-real `root.BuildProcess` and `Process.Execute`
  through the public CLI, loopback HTTP, Factory Session, Work, and canonical
  Factory Event SSE boundaries.
- Real and substituted dependencies: real application wiring, filesystem,
  replay/event projections, and loopback listener; worker/provider effects
  remain controlled only through the existing `edges.Edges` boundaries used by
  the functional scenarios.
- Cost/call budget used: zero remote or paid calls; maximum cost `$0`.

## Project criteria

| Criterion | PASS/FAIL/BLOCKED | Evidence | Unproven edge |
| --- | --- | --- | --- |
| Clean checkout | PASS | `git worktree add --detach` created a detached worktree at the validated commit; `git status --short` reported `CLEAN_STATUS=clean`; the worktree was removed after validation. | Review-owned merge/conflict state. |
| OBS-SPINE representative journey | PASS | `go test -tags=functionallong ./tests/functional/internal/support -run 'TestTerminalFactoryEventObservation|Test.*TerminalEvent.*Spine' -count=1 -v` exited `0` (`ok .../internal/support 0.945s`). The root-built journey observed the canonical post-cursor `RUN_RESPONSE` and retained its existing public Work/session assertions. | Race result and terminal CI. |
| Final owned migration census | PASS | The final `rg` census has zero owned direct helper callers, zero owned stable mode references, and zero owned stable-only wrappers. Remaining hits are the listed provider, provider-session, worker, and ACP owner handoffs; no excluded file changed. | Excluded live-lane behavior remains with its owning lanes. |
| Assertion preservation | PASS | The final ledger maps the migrated callers to their exact values. The clean package run passed all 13 packages whose baseline was runnable, and the four failing package families reproduced the frozen Story 001 signatures rather than new observer failures. | The known baseline fixtures/contention still need their owner lanes. |
| Touched package loopback | PASS (baseline-preserving) | One clean-head `go test -tags=functionallong -count=1` invocation ran for each of the 17 exact packages. 13 exited `0`; recordings, replay, runtime API, and repeater reproduced the same 4 baseline failure families and exact named tests recorded below. | Those pre-existing environmental/fixture failures are not repaired by this lane. |
| Repository vet | PASS | `go vet ./...` exited `0` with no output from the clean checkout. | CI race and terminal CI. |
| Performance direction | PASS (directional) | The evidence ledger records all 51 after invocations beside all 51 before invocations. Ten of 17 package medians improved; the median sum decreased `276.263s` to `259.943s`, led by repeater `32.412s` to `21.353s` and watcher `15.256s` to `10.205s`. | Windows host variance and terminal CI timing. |
| Public/generated/UI surface | PASS | The implementation commit contains functional support/tests and the evidence docs only; no production, OpenAPI, generated, CLI contract, UI, localization, or accessibility surface changed. | Review-owned diff and CI policy checks. |
| Remote runtime proof | NOT APPLICABLE | This is a local test-harness synchronization change with no remote dependency, production behavior, or paid call. | Remote provider behavior is intentionally outside scope. |

## Clean package results

Each row is the exact clean-checkout command
`go test -tags=functionallong -count=1 <package>`:

| Package | Exit | Observed result |
| --- | ---: | --- |
| `./tests/functional/internal/support` | 0 | `ok .../internal/support 6.658s` |
| `./tests/functional/events/factory_events` | 0 | `ok .../events/factory_events 4.835s` |
| `./tests/functional/events/response_events` | 0 | `ok .../events/response_events 1.556s` |
| `./tests/functional/factory/definitions` | 0 | `ok .../factory/definitions 18.695s` |
| `./tests/functional/factory/replay_contracts` | 0 | `ok .../factory/replay_contracts 9.950s` |
| `./tests/functional/factory/visualization/runtime_metrics` | 0 | `ok .../runtime_metrics 10.739s` |
| `./tests/functional/operator_settings/root_composition` | 0 | `ok .../operator_settings/root_composition 1.324s` |
| `./tests/functional/recordings/root_composition` | 1 | `TestRecordingsPortableBuildValidateAndTransportsActivateThroughRootBuildProcessAfterLifecycle`; shared `@you/full-flow` staging-owner contention, matching the before ledger. |
| `./tests/functional/replay_contracts` | 1 | Eight known replay fixture/contention tests: `TestReplayEventStreamArtifactSmoke_ReplaysCheckedInSampleArtifact`, `...WithCopiedRootFactoryDefinition`, `TestReplayFactoryOnlySerializationSmoke_RecordReplayUsesRunStartedFactoryPayload`, `TestRecordReplayEndToEnd_CLIRecordReplayAndRegressionHarnessSucceed`, `TestReplayDriftDisclosureOnStdoutForChangedWorkerDefinition`, `TestRecordReplayEndToEnd_FactoryRequestBatchAndWorkerGeneratedBatchReplayDeterministically`, `TestReplayRegressionHarness_AssertsExpectedDivergence`, and `TestReplayRuntimeConfigSmoke_CanonicalWorkstationsDriveDispatchAndReplay`. |
| `./tests/functional/runtime_api` | 1 | `TestModelTransportSmoke_PullUsesConfiguredLegacyCacheWithoutNetwork`; shared packaged-install contention, matching the before ledger. |
| `./tests/functional/work/recovery` | 0 | `ok .../work/recovery 3.182s` |
| `./tests/functional/work/relationships` | 0 | `ok .../work/relationships 3.307s` |
| `./tests/functional/work/root_composition` | 0 | `ok .../work/root_composition 1.883s` |
| `./tests/functional/work/submission` | 0 | `ok .../work/submission 20.800s` |
| `./tests/functional/workflow` | 0 | `ok .../workflow 5.388s` |
| `./tests/functional/workstations/repeater` | 1 | `TestRalphLoop_TemplateFieldsResolvePerIteration`; unchanged Windows path separator assertion (`ralph/ralph-loop` versus the expected `ralph\\ralph-loop`). |
| `./tests/functional/workstations/watcher` | 0 | `ok .../workstations/watcher 12.623s` |

## Customer journey

1. A detached worktree was created at the validated commit and reported an
   empty status before any check ran.
2. The representative support suite executed through the root-built process
   and public canonical Factory Event SSE. Its observer established the
   retained cursor boundary, ignored retained history, and accepted the first
   post-cursor `RUN_RESPONSE`; the existing session and Work outcomes passed.
3. The exact 17 ledger packages were run once from the clean checkout. The 13
   passing packages retained their existing behavior assertions. The four
   nonzero packages reproduced the baseline contention, fixture, and Windows
   separator signatures documented in the evidence ledger; no source repair or
   shared global cleanup was performed.
4. The clean checkout ran `go vet ./...` to exit `0`, then ran the final census:

   ```text
   rg -n 'WaitForTerminalStatus|terminalObservationStableWindow|RunFactoryToCompletionWithEdgesAndWorkStable|RunFactoryToCompletionWithEdgesAndObservationsStable' tests/functional
   ```

   It returned only the documented excluded owner handoffs and their two
   compatibility boundaries; `CENSUS_EXIT=0`.
5. The clean worktree was removed after the read-only loopback. No browser
   verification applies because this lane changes no UI or browser surface.

## Cross-task integration and usability

- Documentation discoverability: the final census, assertion inventory, and
  before/after timing ledger are in
  `docs/internal/development/functional-test-optimization/functional-terminal-event-observation-retire-stable-windows.md`.
- Permission and error behavior: unchanged; existing malformed stream,
  rejection, timeout, cancellation, failure, and public projection assertions
  remain in the functional support and owned packages.
- Persistence/reload behavior: live recording, historical replay, resume,
  Work lineage, event ordering, and metrics/cost assertions remain owned by
  their existing scenarios; replay-only paths use the retained canonical Work
  projection because historical replay does not emit a new `RUN_RESPONSE`.
- Accessibility/keyboard/responsive behavior: not applicable; no UI changed.
- Operational signals: each observer has bounded stream, reader goroutine, and
  capacity-one terminal/error signaling with deterministic cleanup; the clean
  census confirms excluded ownership rather than silently migrating it.

## Findings

| ID | Severity | Reproduction | Expected | Actual | Evidence |
| --- | --- | --- | --- | --- | --- |
| BASELINE-TERM-001 | informational | Run the four nonzero clean package commands listed above on the validated head. | The implementation lane should preserve the frozen baseline signatures and avoid adding an observer-specific failure. | The same packaged-install contention, replay fixture/assertion, runtime logical-target/fixture, and repeater Windows path failures reproduced; the 13 other packages and the OBS-SPINE passed. | Story 001 ledger, Story 003 after table, and this clean package table. |

## Verdict

PASS for VAL-001 local clean-room validation and the terminal-observation
implementation evidence. The known baseline failures are recorded as
non-blocking owner-lane diagnostics, not silently repaired or claimed as
passing behavior. Required PR CI, race terminal status, review feedback,
conflict resolution, and merge remain review-owned.

## Delta-plan request [Required for FAIL/BLOCKED]

Not applicable: no new implementation defect or blocking clean-room finding
was localized. The four baseline failure families remain assigned to their
existing owner lanes as recorded in the evidence ledger.
