# Validation report: functional terminal event observation and stable-window retirement

## Environment and artifact

- Validated source commit: `2b7fe5a23f909c965e0b0e5e756f6e8b2bc6df16`; all
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
| OBS-SPINE representative journey | PASS | `go test -tags=functionallong ./tests/functional/internal/support -run 'TestTerminalFactoryEventObservation|Test.*TerminalEvent.*Spine|TestWaitFor.*TerminalStatus' -count=1 -v` exited `0` (`ok .../internal/support 2.574s`). The root-built journey observed the canonical post-cursor `RUN_RESPONSE` after the process helper's dependent captures, and retained its existing public Work/session assertions. | Race result and terminal CI. |
| Final owned migration census | PASS | At `2b7fe5a23f`, the final `rg` census exited `0` with zero owned direct helper callers, zero owned stable mode references, and zero owned stable-only wrappers. Remaining hits are the listed provider, provider-session, worker, and ACP owner handoffs; no excluded file changed. The pre-existing `WaitForSessionTerminalStatus` remains as a separately documented live/shared-session status-projection compatibility contract; it is not the standalone process terminal-event path. | Excluded live-lane behavior and the live/shared status contract remain with their owning compatibility boundaries. |
| Assertion preservation | PASS (with diagnostics) | The final ledger maps the migrated callers to their exact values. The clean rebased-head witnesses passed the support observer/status/admission suite, the three affected packages, and the hosted-reported `guards_batch` test; the exact reviewer-reported `factory/current` and `guards_batch` tests also pass independently. | The historical 17-package timing ledger is source evidence from the prior implementation head, not a new timing claim for this rebased loopback. |
| Touched package loopback | PASS (baseline-preserving) | From a detached clean worktree at `2b7fe5a23f`, `providers` exited `0` in `31.347s`, `provider_sessions/association` exited `0` in `5.995s`, and `factory/current` exited `0` in `37.870s`; the support spine exited `0` in `2.574s`. The exact `guards_batch::TestConfigDriven_ResourceContention` reviewer failure also exited `0` in `2.065s`. | Full 17-package timing rerun and hosted CI remain separate evidence; no unrelated fixture or baseline was repaired. |
| Repository vet | PASS | `go vet ./...` exited `0` with no output from the final clean checkout. | CI race and terminal CI. |
| Dead-code diagnostic | BLOCKED (known repository baseline drift) | `make deadcode` exited `2` after the checker exited `1`, reporting baseline `3074` versus current `3067`. The current-only entries are unrelated Windows lock helpers; the removed session-status seam and terminal observer are absent from the current-only set. | The repository's existing Windows/Unix build-tag drift requires the shared dead-code gate/owner; this lane does not change the baseline. |
| Performance direction | PASS (historical bounded optimization; no rebased directional claim) | The evidence ledger records all 51 prior after invocations beside the 51 before invocations. The prior after median sum was `270.772s` versus `276.263s` before; this shared-host result is noisy and is not used as a directional improvement claim. One bounded optimization pass removed fixed polling/stable-window padding from owned completion paths, and no local threshold is asserted. | The 51 timing invocations were not repeated after the rebase; Windows host variance, shared contention, and terminal CI timing remain unproven here. |
| Public/generated/UI surface | PASS | The implementation commit contains functional support/tests and the evidence docs only; no production, OpenAPI, generated, CLI contract, UI, localization, or accessibility surface changed. | Review-owned diff and CI policy checks. |
| Remote runtime proof | NOT APPLICABLE | This is a local test-harness synchronization change with no remote dependency, production behavior, or paid call. | Remote provider behavior is intentionally outside scope. |

## Current final-head compatibility witnesses

These commands ran from the detached clean worktree at `2b7fe5a23f`. The
three affected packages named by the PRD and the support observer spine all
passed; the exact hosted `guards_batch` failure was also replayed separately.

| Package or witness | Command/result | Observed result |
| --- | --- | --- |
| `./tests/functional/internal/support` | `go test -tags=functionallong ./tests/functional/internal/support -run 'TestTerminalFactoryEventObservation|Test.*TerminalEvent.*Spine|TestWaitFor.*TerminalStatus' -count=1 -v` — exit `0`, `2.574s` | PASS; cursor anchoring, post-cursor `RUN_RESPONSE`, status compatibility, failure handling, and root-built spine. |
| `./tests/functional/providers` | `go test -tags=functionallong ./tests/functional/providers -count=1 -timeout=20m` — exit `0`, `31.347s` | PASS |
| `./tests/functional/provider_sessions/association` | `go test -tags=functionallong ./tests/functional/provider_sessions/association -count=1 -timeout=20m` — exit `0`, `5.995s` | PASS |
| `./tests/functional/factory/current` | `go test -tags=functionallong ./tests/functional/factory/current -count=1 -timeout=20m` — exit `0`, `37.870s` | PASS |
| `./tests/functional/guards_batch::TestConfigDriven_ResourceContention` | `go test -tags=functionallong ./tests/functional/guards_batch -run '^TestConfigDriven_ResourceContention$' -count=1 -v` — exit `0`, `2.065s` | PASS; exact reviewer-reported failure does not reproduce locally. |

## Historical package timing ledger

The table below is the committed three-run timing ledger from the pre-rebase
implementation evidence. It remains useful for the bounded optimization
comparison, but it is not claimed as a new final-head package sweep. The
current final-head package witnesses are listed above.

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

1. A detached worktree was created at `2b7fe5a23f` and reported an empty status
   before any check ran.
2. The representative support suite executed through the root-built process
   and public canonical Factory Event SSE. Its observer established the
   retained cursor boundary, ignored retained history, and accepted the first
   post-cursor `RUN_RESPONSE`; the existing session and Work outcomes passed.
3. The three affected packages and the exact hosted `guards_batch` failure
   were rerun at the latest rebased head; all passed. The historical 51-invocation
   ledger remains identified separately above, and no source repair or shared
   global cleanup was performed for unrelated findings.
4. The clean checkout ran `go vet ./...` to exit `0`, ran `make deadcode` to
   reproduce the known baseline drift, then ran the final census:

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
| BASELINE-TERM-001 | informational | Run the historical three exact clean package invocations for `replay_contracts` and `workstations/repeater` listed above. | The implementation lane should preserve the frozen baseline signatures and avoid adding an observer-specific failure. | The replay fixture/configuration failures and repeater Windows path-separator assertion reproduced in the pre-rebase ledger; they are not reclassified as current-head results by this loopback. | Story 001 ledger, Story 003 after table, and the historical timing table. |
| ENV-TERM-002 | informational | Run the current affected-package witnesses under the shared Windows workspace. | A clean loopback package run should be repeatable when shared install/listener state is available. | `providers`, `provider_sessions/association`, `factory/current`, support, and the exact `guards_batch` reviewer test all passed at `2b7fe5a23f`; unrelated shared-host contention was not observed in these runs. | Current final-head compatibility witness table. |
| REVIEW-TERM-003 | resolved | Run the support compatibility tests and the three affected packages at the latest rebased head. | The review-found live/shared-session compatibility regression and hosted-reported terminal-event failures must not reproduce in the corrected source. | The status-projection compatibility test does not open `/events`; the standalone process path uses post-cursor `RUN_RESPONSE`; all three affected packages and the exact `factory/current` and `guards_batch` tests passed locally. | Current final-head witness table and clean census. |
| GATE-TERM-004 | blocking shared gate | Run `make deadcode` at `2b7fe5a23f`. | The repository dead-code ratchet should be clean or have an approved baseline allowance. | The unchanged baseline drift remains: `3074` baseline, `3067` current, checker exit `1`; current-only findings are unrelated Windows lock helpers. | Clean final-head dead-code command output; shared dead-code owner. |

## Verdict

PASS for the local VAL-001 clean-room behavior witnesses at `2b7fe5a23f`.
The implementation handoff remains BLOCKED until this rebased head and this
current report are pushed, required PR CI is started, and the review feedback
is updated. The known dead-code baseline drift is recorded as a shared-gate
diagnostic, not silently repaired or claimed as passing behavior; review owns
terminal CI, conflict resolution, and merge.

## Delta-plan request [Required for FAIL/BLOCKED]

Required delta: push the rebased source and report, start current-head PR CI,
and classify its result. If the exact hosted functional failures recur, keep
the reproduction and smallest support-owned fix request in the PR discussion;
do not repair unrelated package fixtures. If the dead-code ratchet remains at
the same Windows/Unix baseline drift, post the run URL and checker output for
the shared dead-code owner rather than changing its baseline in this lane.
