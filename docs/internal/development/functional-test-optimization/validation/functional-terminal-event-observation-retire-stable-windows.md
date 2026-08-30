# Validation report: functional terminal event observation and stable-window retirement

## Environment and artifact

- Validated source commit: `bf6dd539c2952ea74602b70beb496c2c21753f7b`; all
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
| OBS-SPINE representative journey | PASS | `go test -tags=functionallong ./tests/functional/internal/support -run 'TestTerminalFactoryEventObservation|Test.*TerminalEvent.*Spine' -count=1 -v` exited `0` (`ok .../internal/support 0.823s`). The root-built journey observed the canonical post-cursor `RUN_RESPONSE` after the process helper's dependent captures, and retained its existing public Work/session assertions. | Race result and terminal CI. |
| Final owned migration census | PASS | At `bf6dd539c2`, the final `rg` census has zero owned direct helper callers, zero owned stable mode references, and zero owned stable-only wrappers. Remaining hits are the listed provider, provider-session, worker, and ACP owner handoffs; no excluded file changed. The pre-existing `WaitForSessionTerminalStatus` remains as a separately documented live/shared-session status-projection compatibility contract; it is not the standalone process terminal-event path. | Excluded live-lane behavior and the live/shared status contract remain with their owning compatibility boundaries. |
| Assertion preservation | PASS (with diagnostics) | The final ledger maps the migrated callers to their exact values. The clean sweep produced 44 passing and 7 nonzero runs across the 17 packages; event/response, relationship, support, and the other owned passing packages retained their assertions. The nonzero runs reproduce known replay/repeater signatures or one shared runtime-api contention run rather than an observer-specific failure. | The known baseline fixtures/contention still need their owner lanes. |
| Touched package loopback | PASS (baseline-preserving) | Three clean-head `go test -tags=functionallong -count=1` invocations ran for each of the 17 exact packages. Fourteen packages passed all three; `runtime_api` failed once then passed twice; `replay_contracts` and `workstations/repeater` reproduced their known nonzero signatures on all three runs. | Those pre-existing environmental/fixture failures are not repaired by this lane. |
| Repository vet | PASS | `go vet ./...` exited `0` with no output from the final clean checkout. | CI race and terminal CI. |
| Performance direction | PASS (bounded optimization; no directional claim) | The evidence ledger records all 51 final after invocations beside the 51 before invocations. The final after median sum is `379.229s` versus `276.263s` before; this saturated shared Windows-host result is noisy and not an improvement claim. One bounded optimization pass removed fixed polling/stable-window padding from owned completion paths, and no local threshold is asserted. | Windows host variance, shared contention, and terminal CI timing. |
| Public/generated/UI surface | PASS | The implementation commit contains functional support/tests and the evidence docs only; no production, OpenAPI, generated, CLI contract, UI, localization, or accessibility surface changed. | Review-owned diff and CI policy checks. |
| Remote runtime proof | NOT APPLICABLE | This is a local test-harness synchronization change with no remote dependency, production behavior, or paid call. | Remote provider behavior is intentionally outside scope. |

## Clean package results

Each row is three exact clean-checkout invocations of
`go test -tags=functionallong -count=1 <package>`. Values are PowerShell
wall times rounded to milliseconds; the three exit codes are listed in run
order.

| Package | Run 1 | Run 2 | Run 3 | Median wall | Observed result |
| --- | --- | --- | --- | ---: | --- |
| `./tests/functional/internal/support` | 0 / 28.768s | 0 / 18.464s | 0 / 14.887s | 18.464s | PASS |
| `./tests/functional/events/factory_events` | 0 / 14.237s | 0 / 10.284s | 0 / 12.498s | 12.498s | PASS |
| `./tests/functional/events/response_events` | 0 / 4.666s | 0 / 4.867s | 0 / 4.612s | 4.666s | PASS |
| `./tests/functional/factory/definitions` | 0 / 26.301s | 0 / 27.159s | 0 / 23.266s | 26.301s | PASS |
| `./tests/functional/factory/replay_contracts` | 0 / 12.685s | 0 / 12.313s | 0 / 11.900s | 12.313s | PASS |
| `./tests/functional/factory/visualization/runtime_metrics` | 0 / 13.307s | 0 / 12.985s | 0 / 14.879s | 13.307s | PASS |
| `./tests/functional/operator_settings/root_composition` | 0 / 3.579s | 0 / 3.344s | 0 / 3.185s | 3.344s | PASS |
| `./tests/functional/recordings/root_composition` | 0 / 5.674s | 0 / 6.946s | 0 / 6.874s | 6.874s | PASS |
| `./tests/functional/replay_contracts` | 1 / 75.142s | 1 / 130.369s | 1 / 136.329s | 130.369s | FAIL: existing replay fixture/configuration failures, including copied-root artifact loading, worker-type/stop-token drift, CLI stdout, generated relation count, and replay workstation-map assumptions. |
| `./tests/functional/runtime_api` | 1 / 60.313s | 0 / 52.336s | 0 / 32.083s | 52.336s | MIXED: one shared packaged-install/listener-contention run, followed by two passes. |
| `./tests/functional/work/recovery` | 0 / 5.603s | 0 / 6.828s | 0 / 22.356s | 6.828s | PASS |
| `./tests/functional/work/relationships` | 0 / 7.073s | 0 / 6.400s | 0 / 5.924s | 6.400s | PASS |
| `./tests/functional/work/root_composition` | 0 / 4.317s | 0 / 4.269s | 0 / 4.149s | 4.269s | PASS |
| `./tests/functional/work/submission` | 0 / 28.010s | 0 / 31.865s | 0 / 31.857s | 31.857s | PASS |
| `./tests/functional/workflow` | 0 / 9.199s | 0 / 7.430s | 0 / 7.914s | 7.914s | PASS |
| `./tests/functional/workstations/repeater` | 1 / 26.955s | 1 / 35.671s | 1 / 28.447s | 28.447s | FAIL: existing `TestRalphLoop_TemplateFieldsResolvePerIteration` Windows path-separator assertion (`ralph/ralph-loop` versus `ralph\\ralph-loop`). |
| `./tests/functional/workstations/watcher` | 0 / 13.042s | 0 / 13.046s | 0 / 10.346s | 13.042s | PASS |

## Customer journey

1. A detached worktree was created at the validated commit and reported an
   empty status before any check ran.
2. The representative support suite executed through the root-built process
   and public canonical Factory Event SSE. Its observer established the
   retained cursor boundary, ignored retained history, and accepted the first
   post-cursor `RUN_RESPONSE`; the existing session and Work outcomes passed.
3. The exact 17 ledger packages were each run three times from the clean
   checkout (51 invocations total). Forty-four invocations passed. The seven
   nonzero invocations were confined to the known replay and repeater baseline
   signatures plus one shared runtime-api contention run; the next two
   runtime-api runs passed. No source repair or shared global cleanup was
   performed for those unrelated findings.
4. The clean checkout ran `go vet ./...` to exit `0`, then ran the final census:

   ```text
   rg -n 'WaitForTerminalStatus|terminalObservationStableWindow|RunFactoryToCompletionWithEdgesAndWorkStable|RunFactoryToCompletionWithEdgesAndObservationsStable' tests/functional
   ```

   It returned only the documented excluded owner handoffs and the two
   compatibility boundaries; `CENSUS_EXIT=0`. The owned standalone terminal
   journey has no direct status helper or stable-window caller.
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
| BASELINE-TERM-001 | informational | Run the three exact clean package invocations for `replay_contracts` and `workstations/repeater` listed above. | The implementation lane should preserve the frozen baseline signatures and avoid adding an observer-specific failure. | The replay fixture/configuration failures and repeater Windows path-separator assertion reproduced on all three runs; the 14 other package rows did not reproduce a source-specific observer failure. | Story 001 ledger, Story 003 after table, and this clean package table. |
| ENV-TERM-002 | informational | Run `./tests/functional/runtime_api` three times under the shared Windows workspace. | A clean loopback package run should be repeatable when shared install/listener state is available. | Run 1 failed with shared packaged-install/listener contention; runs 2 and 3 passed. This is host contention, not a source-specific assertion regression. | Final clean package table and the three raw command outcomes retained in the session evidence. |
| REVIEW-TERM-003 | resolved | Run `./tests/functional/work/relationships` and the support terminal-observation tests at the final head. | Relationship scenarios and the terminal-event spine must retain their public session/Work behavior while using event completion. | The review-found live/shared-session compatibility regression was isolated to the status-projection helper, which now uses a bounded adaptive retry contract; the standalone process path uses post-cursor `RUN_RESPONSE`. All three relationship runs and the focused support suite passed. | Review fix commits `de88a049`, `d2918457`, `bf6dd539`; final clean package table and OBS-SPINE result. |

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
