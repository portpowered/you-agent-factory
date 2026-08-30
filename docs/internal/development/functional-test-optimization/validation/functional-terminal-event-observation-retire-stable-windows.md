# Validation report: functional terminal event observation and stable-window retirement

## Environment and artifact

- Validated source commit: `0f85d137b3281df78971b42351fe47674eb98d27`; all
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
| OBS-SPINE representative journey | PASS | `go test -tags=functionallong ./tests/functional/internal/support -run 'TestTerminalFactoryEventObservation|Test.*TerminalEvent.*Spine' -count=1 -v` exited `0` (`ok .../internal/support 1.173s`). The root-built journey observed the canonical post-cursor `RUN_RESPONSE` after the process helper's dependent captures, and retained its existing public Work/session assertions. | Race result and terminal CI. |
| Final owned migration census | PASS | At `0f85d137b3`, the final `rg` census has zero owned direct helper callers, zero owned stable mode references, and zero owned stable-only wrappers. Remaining hits are the listed provider, provider-session, worker, and ACP owner handoffs; no excluded file changed. The pre-existing `WaitForSessionTerminalStatus` remains as a separately documented live/shared-session status-projection compatibility contract; it is not the standalone process terminal-event path. | Excluded live-lane behavior and the live/shared status contract remain with their owning compatibility boundaries. |
| Assertion preservation | PASS (with diagnostics) | The final ledger maps the migrated callers to their exact values. The clean sweep produced 45 passing and 6 nonzero runs across the 17 packages; event/response, relationship, support, runtime-api, and the other owned passing packages retained their assertions. The nonzero runs reproduce known replay and repeater signatures rather than an observer-specific failure. | The known baseline fixtures still need their owner lanes. |
| Touched package loopback | PASS (baseline-preserving) | Three clean-head `go test -tags=functionallong -count=1` invocations ran for each of the 17 exact packages. Fifteen packages passed all three; `replay_contracts` and `workstations/repeater` reproduced their known nonzero signatures on all three runs. | Those pre-existing fixture/configuration and Windows path failures are not repaired by this lane. |
| Repository vet | PASS | `go vet ./...` exited `0` with no output from the final clean checkout. | CI race and terminal CI. |
| Dead-code diagnostic | PASS (no new functional entry) | `make deadcode` reports baseline `3074` versus current `3068`; the only current-only entries are four Windows lock helpers, while Unix/test-platform variants and unchanged legacy support helpers account for the baseline-only entries. The removed session-status seam is absent from the current-only set. | The repository's existing Windows/Unix build-tag drift remains outside this lane. |
| Performance direction | PASS (bounded optimization; no directional claim) | The evidence ledger records all 51 final after invocations beside the 51 before invocations. The final after median sum is `270.772s` versus `276.263s` before; this shared-host result is noisy and is not used as a directional improvement claim. One bounded optimization pass removed fixed polling/stable-window padding from owned completion paths, and no local threshold is asserted. | Windows host variance, shared contention, and terminal CI timing. |
| Public/generated/UI surface | PASS | The implementation commit contains functional support/tests and the evidence docs only; no production, OpenAPI, generated, CLI contract, UI, localization, or accessibility surface changed. | Review-owned diff and CI policy checks. |
| Remote runtime proof | NOT APPLICABLE | This is a local test-harness synchronization change with no remote dependency, production behavior, or paid call. | Remote provider behavior is intentionally outside scope. |

## Clean package results

Each row is three exact clean-checkout invocations of
`go test -tags=functionallong -count=1 <package>`. Values are PowerShell
wall times rounded to milliseconds; the three exit codes are listed in run
order.

| Package | Run 1 | Run 2 | Run 3 | Median wall | Observed result |
| --- | --- | --- | --- | ---: | --- |
| `./tests/functional/internal/support` | 0 / 21.243s | 0 / 8.017s | 0 / 9.520s | 9.520s | PASS |
| `./tests/functional/events/factory_events` | 0 / 7.697s | 0 / 6.181s | 0 / 6.201s | 6.201s | PASS |
| `./tests/functional/events/response_events` | 0 / 3.507s | 0 / 3.342s | 0 / 3.486s | 3.486s | PASS |
| `./tests/functional/factory/definitions` | 0 / 19.151s | 0 / 18.503s | 0 / 17.423s | 18.503s | PASS |
| `./tests/functional/factory/replay_contracts` | 0 / 11.505s | 0 / 11.105s | 0 / 11.652s | 11.505s | PASS |
| `./tests/functional/factory/visualization/runtime_metrics` | 0 / 34.341s | 0 / 32.282s | 0 / 40.359s | 34.341s | PASS |
| `./tests/functional/operator_settings/root_composition` | 0 / 7.970s | 0 / 8.718s | 0 / 4.957s | 7.970s | PASS |
| `./tests/functional/recordings/root_composition` | 0 / 4.875s | 0 / 4.736s | 0 / 4.626s | 4.736s | PASS |
| `./tests/functional/replay_contracts` | 1 / 55.998s | 1 / 58.644s | 1 / 56.334s | 56.334s | FAIL: existing replay fixture/configuration failures, including copied-root artifact loading, worker-type/stop-token drift, CLI stdout, generated relation count, and replay workstation-map assumptions. |
| `./tests/functional/runtime_api` | 0 / 33.507s | 0 / 33.499s | 0 / 33.289s | 33.499s | PASS |
| `./tests/functional/work/recovery` | 0 / 6.770s | 0 / 7.376s | 0 / 6.669s | 6.770s | PASS |
| `./tests/functional/work/relationships` | 0 / 7.813s | 0 / 6.631s | 0 / 6.758s | 6.758s | PASS |
| `./tests/functional/work/root_composition` | 0 / 5.490s | 0 / 5.402s | 0 / 5.768s | 5.490s | PASS |
| `./tests/functional/work/submission` | 0 / 26.239s | 0 / 24.447s | 0 / 23.145s | 24.447s | PASS |
| `./tests/functional/workflow` | 0 / 8.099s | 0 / 7.727s | 0 / 7.616s | 7.727s | PASS |
| `./tests/functional/workstations/repeater` | 1 / 22.022s | 1 / 22.225s | 1 / 23.130s | 22.225s | FAIL: existing `TestRalphLoop_TemplateFieldsResolvePerIteration` Windows path-separator assertion (`ralph/ralph-loop` versus `ralph\\ralph-loop`). |
| `./tests/functional/workstations/watcher` | 0 / 11.260s | 0 / 10.748s | 0 / 11.511s | 11.260s | PASS |

## Customer journey

1. A detached worktree was created at the validated commit and reported an
   empty status before any check ran.
2. The representative support suite executed through the root-built process
   and public canonical Factory Event SSE. Its observer established the
   retained cursor boundary, ignored retained history, and accepted the first
   post-cursor `RUN_RESPONSE`; the existing session and Work outcomes passed.
3. The exact 17 ledger packages were each run three times from the clean
   checkout (51 invocations total). Forty-five invocations passed. The six
   nonzero invocations were confined to the known replay and repeater baseline
   signatures; all three runtime-api runs passed. No source repair or shared
   global cleanup was performed for those unrelated findings.
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
| BASELINE-TERM-001 | informational | Run the three exact clean package invocations for `replay_contracts` and `workstations/repeater` listed above. | The implementation lane should preserve the frozen baseline signatures and avoid adding an observer-specific failure. | The replay fixture/configuration failures and repeater Windows path-separator assertion reproduced on all three runs; the 15 other package rows passed all three runs. | Story 001 ledger, Story 003 after table, and this clean package table. |
| ENV-TERM-002 | informational | Run the exact 17-package loop three times under the shared Windows workspace. | A clean loopback package run should be repeatable when shared install/listener state is available. | No runtime-api contention occurred in this final loopback; the earlier pre-rebase mixed run was superseded by the final-head rerun and is not a final result. | Final clean package table and the three raw command outcomes retained in the session evidence. |
| REVIEW-TERM-003 | resolved | Run `./tests/functional/work/relationships` and the support terminal-observation tests at the final head. | Relationship scenarios and the terminal-event spine must retain their public session/Work behavior while using event completion. | The review-found live/shared-session compatibility regression was isolated to the status-projection helper, which now uses a bounded adaptive retry contract; the standalone process path uses post-cursor `RUN_RESPONSE`. All three relationship runs and the focused support suite passed. | Rebasing preserved review fixes in commits `84b77b44`, `ea2cee69`, `4c271781`; the unreachable helper was removed in `0f85d137b3`; final clean package table and OBS-SPINE result. |

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
