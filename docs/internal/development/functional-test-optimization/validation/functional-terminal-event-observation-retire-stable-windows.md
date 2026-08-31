# Validation report: functional terminal event observation and stable-window retirement

## Environment and artifact

- Validated source commit: `6d5dfc43fe0867d1ab9592b196382f66af7f52d4`; all
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
| OBS-SPINE representative journey | PASS | `go test -tags=functionallong ./tests/functional/internal/support -run 'TestTerminalFactoryEventObservation|Test.*TerminalEvent.*Spine|TestWaitFor.*TerminalStatus' -count=1 -v` exited `0` (`ok .../internal/support 0.971s`). The root-built journey observed the canonical post-cursor `RUN_RESPONSE` after the process helper's dependent captures, and retained its existing public Work/session assertions. | Race result and terminal CI. |
| Final owned migration census | PASS | At `6d5dfc43fe`, the final `rg` census exited `0` with zero owned direct helper callers, zero owned stable mode references, and zero owned stable-only wrappers. Remaining hits are the listed provider, provider-session, worker, and ACP owner handoffs; no excluded file changed. The pre-existing `WaitForSessionTerminalStatus` remains as a separately documented live/shared-session status-projection compatibility contract; it is not the standalone process terminal-event path. | Excluded live-lane behavior and the live/shared status contract remain with their owning compatibility boundaries. |
| Assertion preservation | PASS (with diagnostics) | The final ledger maps the migrated callers to their exact values. The folded engine regression passed while keeping the rebased shared unit inventory at `18,286` entries; the clean-head support, ACP, three affected packages, and hosted-reported `guards_batch` witnesses all pass. | The historical 17-package timing ledger is source evidence from the prior implementation head, not a new timing claim for this rebased loopback. |
| Touched package loopback | PASS (baseline-preserving) | From a detached clean worktree at `6d5dfc43fe`, `providers` exited `0` in `20.711s`, `provider_sessions/association` exited `0` in `2.833s`, and `factory/current` exited `0` in `21.621s`; the support spine exited `0` in `0.971s`. The exact `guards_batch::TestConfigDriven_ResourceContention` reviewer failure exited `0` in `0.864s`, and all eight ACP-stdio tests exited `0` in `12.911s`. | Full 17-package timing rerun and hosted CI remain separate evidence; no unrelated fixture or baseline was repaired. |
| Repository vet | BLOCKED (shared-host contention) | `go vet ./...` and focused vet attempts for the changed engine/support packages were started from the detached clean worktree and stopped after bounded multi-minute runs with no diagnostics or exit result while other repository-wide Go jobs occupied the host. The prior clean vet result is retained only as historical evidence, not claimed for this head. | Current-head repository vet and CI race remain unproven. |
| Dead-code diagnostic | BLOCKED (shared gate / host contention) | `make deadcode` was started at the rebased source head and stopped after a bounded approximately 210-second run with no diagnostic output. The prior hosted diagnostic recorded unchanged baseline `3074` versus current `3069`; this branch does not edit the shared dead-code baseline. | Current-head dead-code output requires the shared host/gate; this lane does not change the baseline. |
| Performance direction | PASS (historical bounded optimization; no rebased directional claim) | The evidence ledger records all 51 prior after invocations beside the 51 before invocations. The prior after median sum was `270.772s` versus `276.263s` before; this shared-host result is noisy and is not used as a directional improvement claim. One bounded optimization pass removed fixed polling/stable-window padding from owned completion paths, and no local threshold is asserted. | The 51 timing invocations were not repeated after the rebase; Windows host variance, shared contention, and terminal CI timing remain unproven here. |
| Public/generated/UI surface | PASS | The implementation changes remain inside internal runtime ordering, functional support/tests, and evidence docs; no public OpenAPI, generated, CLI contract, UI, localization, persisted-data, or accessibility surface changed. | Review-owned diff and CI policy checks. |
| Remote runtime proof | NOT APPLICABLE | This is a local test-harness synchronization change with no remote dependency, production behavior, or paid call. | Remote provider behavior is intentionally outside scope. |

## Current final-head compatibility witnesses

These commands ran from the detached clean worktree at `6d5dfc43fe`. The
three affected packages named by the PRD and the support observer spine all
passed; the exact hosted `guards_batch` failure was also replayed separately.

| Package or witness | Command/result | Observed result |
| --- | --- | --- |
| `./tests/functional/internal/support` | `go test -tags=functionallong ./tests/functional/internal/support -run 'TestTerminalFactoryEventObservation|Test.*TerminalEvent.*Spine|TestWaitFor.*TerminalStatus' -count=1 -v` — exit `0`, `0.971s` | PASS; cursor anchoring, post-cursor `RUN_RESPONSE`, status compatibility, failure handling, and root-built spine. |
| `./tests/functional/providers` | `go test -tags=functionallong ./tests/functional/providers -count=1 -timeout=20m` — exit `0`, `20.711s` | PASS |
| `./tests/functional/provider_sessions/association` | `go test -tags=functionallong ./tests/functional/provider_sessions/association -count=1 -timeout=20m` — exit `0`, `2.833s` | PASS |
| `./tests/functional/factory/current` | `go test -tags=functionallong ./tests/functional/factory/current -count=1 -timeout=20m` — exit `0`, `21.621s` | PASS |
| `./tests/functional/guards_batch::TestConfigDriven_ResourceContention` | `go test -tags=functionallong ./tests/functional/guards_batch -run '^TestConfigDriven_ResourceContention$' -count=1 -v` — exit `0`, `0.864s` | PASS; exact reviewer-reported failure does not reproduce locally. |
| `./tests/functional/transport/acp/stdio` | `go test -tags=functionallong ./tests/functional/transport/acp/stdio -count=1 -timeout=15m -v` — exit `0`, `12.911s` | PASS; all eight ACP-stdio tests reported, including the six absent from the hosted partial inventory. |
| `./pkg/services/factory_runtime/internal/services/orchestration/engine` | `go test ./pkg/services/factory_runtime/internal/services/orchestration/engine -run '^TestDispatchResultHook_CompletionRecordedAtObservedTick$' -count=1 -v` — exit `0`, `0.025s` | PASS; response observation sees the published terminal dispatch snapshot while completion remains recorded at the observed tick. |

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

1. A detached worktree was created at `6d5dfc43fe` and reported an empty status
   before any check ran.
2. The representative support suite executed through the root-built process
   and public canonical Factory Event SSE. Its observer established the
   retained cursor boundary, ignored retained history, and accepted the first
   post-cursor `RUN_RESPONSE`; the existing session and Work outcomes passed.
3. The three affected packages, all eight ACP-stdio tests, the folded engine
   regression, and the exact hosted `guards_batch` failure were rerun at the
   latest rebased head; all passed. The historical 51-invocation ledger remains
   identified separately above, and no source repair or shared global cleanup
   was performed for unrelated findings.
4. The clean checkout ran the final census. Repository-wide `go vet ./...`,
   focused vet, and `make deadcode` were each stopped after bounded runs with
   no diagnostics because the shared Windows host was saturated; their results
   are recorded as unproven rather than passed:

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
| ENV-TERM-002 | informational | Run the current affected-package witnesses under the shared Windows workspace. | A clean loopback package run should be repeatable when shared install/listener state is available. | `providers`, `provider_sessions/association`, `factory/current`, support, ACP-stdio, and the exact `guards_batch` reviewer test all passed at `6d5dfc43fe`; repository-wide vet/dead-code commands were separately host-contended and are not claimed here. | Current final-head compatibility witness table. |
| REVIEW-TERM-003 | resolved | Run the support compatibility tests, ACP-stdio package, and the three affected packages at the latest rebased head. | The review-found live/shared-session compatibility regression and hosted-reported terminal-event failures must not reproduce in the corrected source. | The status-projection compatibility test does not open `/events`; the standalone process path uses post-cursor `RUN_RESPONSE`; all eight ACP tests, all three affected packages, the exact `factory/current` and `guards_batch` tests, and the folded engine regression passed locally. | Current final-head witness table and clean census. |
| REVIEW-TERM-004 | resolved | Compare the changed head with the rebased shared unit inventory. | The runtime-snapshot assertion must prove the ordering fix without increasing the shared top-level test inventory. | The new assertion is folded into `TestDispatchResultHook_CompletionRecordedAtObservedTick`; current `testInventory` remains `18,286`, matching `origin/main` baseline `1194982e`. | `git diff origin/main...HEAD`; `docs/internal/baselines/go-unit-lane-latency-budget.v1.json`. |
| GATE-TERM-004 | blocking shared gate | Run `go vet ./...` and `make deadcode` at `6d5dfc43fe`. | Repository-wide static checks should produce current-head results. | Both commands were stopped after bounded host-contended runs with no diagnostics; the prior hosted dead-code diagnostic remains baseline `3074` versus current `3069`, and no shared baseline was edited. | Current validation process output; prior hosted run `33351659185`; shared dead-code owner. |

## Verdict

BLOCKED for the overall VAL-001 loopback at `6d5dfc43fe`: the support, ACP,
affected-package, engine, and census behavior witnesses pass, while repository
vet/dead-code output was not obtainable under the saturated shared host and
current PR CI has not yet been rerun. No defect was silently repaired; the
shared dead-code baseline remains untouched. The implementation handoff also
requires this report and source head to be pushed, required CI to start, and
the review feedback to be updated; review owns terminal CI, conflict
resolution, and merge.

## Delta-plan request [Required for FAIL/BLOCKED]

- Affected behavior and criterion: current-head GATE-CI, repository vet, and
  dead-code diagnostics remain unproven after the clean executable witnesses.
- Root-cause evidence or remaining uncertainty: the prior hosted run timed out
  in ACP-stdio after only two tests were recorded; the current clean ACP
  package reports all eight tests in `12.911s`, but hosted CI has not yet been
  rerun on this head. Local vet/dead-code attempts were host-contended.
- Smallest recommended correction/prerequisite: push `6d5dfc43fe` plus this
  report, start current-head CI, and classify its terminal coverage/latency/
  lint result; do not raise timeouts, add retries, or edit shared baselines.
- Dependencies and retest scope: review-owned hosted CI and shared dead-code
  gate, with the current support, ACP, three affected packages, and engine
  witnesses as the clean-room retest set.
