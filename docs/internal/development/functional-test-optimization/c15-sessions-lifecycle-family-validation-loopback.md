# Validation report: C15 Session-family functional-test optimization

## Environment and artifact

- Commit/build identifier: `f5c654842d9b672befb51b2c555c9e917b5dffcc` (implementation
  head validated; this report update is the following documentation commit).
- Environment and configuration: Windows/amd64, Go `go1.25.0`,
  `CGO_ENABLED=1`; clean detached checkout; no paid or remote-provider calls.
- Customer entry point: real `root.BuildProcess`/`Process.Execute` paths at
  the public CLI, HTTP, MCP, Factory Session, Work, dispatch, result, and
  Factory Event boundaries.
- Real and substituted dependencies: local-real production composition with
  the package-owned controlled provider, browser, listener, and schema edges
  already characterized by stories 002–006.
- Cost/call budget used: `$0`; zero paid or remote-provider calls.

## Project criteria

| Criterion | PASS/FAIL/BLOCKED | Evidence | Unproven edge |
| --- | --- | --- | --- |
| Exact 33-test denominator | PASS | In a clean detached checkout at `f5c654842d9b672befb51b2c555c9e917b5dffcc`, `go test ./<package> -list '^Test'` exited `0` for every owned package and listed 9 execution, 7 controls, 3 standalone, 12 lifecycle, and 2 CLI top-level tests. | Future test additions after this artifact. |
| Complete clean-room behavior run | PASS | Exactly one sequential `go test -count=1 -timeout=10m ./<package>` run per package exited `0`; no test failure, skip, or quarantine token was observed. | New-head hosted CI timing and coverage. |
| Nested behavior matrix | PASS | The complete E01–E22, C01–C10, S01–S03, L01–L12, Q01–Q04, and X01–X05 ledger remains mapped to the unchanged top-level witnesses below; stories 002–006 supplied focused behavior and cleanup evidence. | Restart and authorization remain outside this lane. |
| Process/topology reduction | PASS | Eligible setup was consolidated: execution `21 -> 13` retained starts, controls `7 -> 1` API-host start with provider/script command-runner edges (no MockWorkers feature), standalone `4 -> 3` root builds, lifecycle retained its property-required two roles/one listener, and CLI uses one root/responder plus one fewer temporary-root allocation in Q04. | PR-host package timing is review-owned. |
| Zero-residue cleanup | PASS | All clean-room package runs passed package-owned cleanup assertions. Prior task-owned censes are balanced: execution routes/Sessions; controls six explicit Session opens/closes and zero routes/root residue; standalone roots `3/3`, fixture scopes `4/4`, contexts `4/4`, stdio pairs `4/4`, working directories `4/4`; lifecycle two process closes, one listener close, one stream close; CLI root/responder `1/1` and scenarios `2/2`. | OS-global resources outside fixture ownership. |
| Local timing handoff | PASS | Supplied-before and one diagnostic clean-room local-after sample are recorded below; no local sample is presented as PR-CI evidence. | `REV-CI-001` must supply current-head package timing and functional coverage. |
| PR functional coverage and terminal CI | PENDING REVIEW | The report prepares the exact package handoff but cannot prove a PR job before the PR is opened. | `REV-CI-001` (at least 61.6% functional coverage and package timing); `REV-DELIVERY-001` (terminal CI, feedback, conflicts, merge). |
| Scope and excluded surfaces | PASS | The current diff audit showed changes only under the five owned functional package directories and this validation report; restart, shared support, board persistence, baselines, production, UI, contracts, and generated outputs were untouched. | Final origin/main sync is a delivery-stage action. |

### List-only denominator

| Package | Top-level selectors (exactly as listed) |
| --- | --- |
| execution (9) | `TestAPIPetriDispatchUsageReachesDispatchList`; `TestWithServerDrainCannotReportSuccessWhileWorkIsNonTerminal`; `TestHostedFiniteRunsKeepEmptyAndTerminalSuccess`; `TestHostedContinuousRunsStayLiveWhileIdle`; `TestAPIResultAndResultsExposeTerminalInvocationData`; `TestAPIDispatchListAndDetailExposePublicCorrelation`; `TestAPIPartialResultIsAvailableBeforeTerminalCompletion`; `TestCLIInvocationIsVisibleThroughAPISessionAndWorkReads`; `TestAPIInvocationResultMatchesCLICompatibleFacts` |
| controls (7) | `TestPausedFactorySessionReturnsInvocationPausedStatus`; `TestPausedFactorySessionBuffersSubmittedWork`; `TestResumedFactorySessionDrainsBufferedWorkInOrder`; `TestPauseResumeEmitsDurableLifecycleEvents`; `TestInterruptedWorkInspectSurfacesDispatchAndStopSummary`; `TestAPIPauseResumeCancelAndTerminateFactorySession`; `TestAPIInvalidLifecycleTransitionReturnsConflict` |
| standalone (3) | `TestBTRCP0StandaloneFixtureSuccessCharacterization`; `TestBTRCP0StandaloneFixtureFailureCharacterization`; `TestBTRCP0StandaloneFixtureRunsRemainIsolated` |
| lifecycle (12) | `TestFactorySessionCreateListShowDelete`; `TestFactorySessionListMultipleSessions`; `TestFactorySessionMissingShowAndDeleteFail`; `TestAPIOpenListGetAndCloseFactorySession`; `TestAPIFactorySessionNotFoundUsesTypedError`; `TestAPIMultipleFactorySessionsRemainIsolated`; `TestFactorySessionCleanupRunsAfterEarlySubtestExit`; `TestCLIRemoteRunStartsDurableSessionOnSelectedServer`; `TestCLILocalAndRemoteRunSuccessParityThroughRootProcess`; `TestCLILocalAndRemoteRunDomainFailureParityThroughRootProcess`; `TestCLILocalAndRemoteRunCancellationParityThroughRootProcess`; `TestCLIRemoteRunTransportFailureKeepsStreamsAndExitObservable` |
| cli (2) | `TestBuildProcessRoutesEverySessionLeafThroughResolvedProductionComposition`; `TestBuildProcessRejectsDeprecatedPortBeforeSubmitDispatch` |

### Matrix-to-witness mapping

| Matrix rows | Top-level witness mapping |
| --- | --- |
| E01–E02 | execution dispatch-usage witness |
| E03–E04 | execution finite drain failure/browser-edge witness |
| E05–E08 | execution hosted finite empty/success witness |
| E09–E10 | execution hosted continuous negative-liveness witness |
| E11–E18 | execution result, bad-input, timeout, and cancellation witness |
| E19 | execution dispatch list/detail correlation witness |
| E20 | execution partial-result witness |
| E21–E22 | execution CLI visibility and API/CLI parity witnesses |
| C01–C05 | controls pause, buffer, resume, event-order, and interrupted-work witnesses |
| C06–C10 | controls public pause/resume/cancel/terminate and invalid-transition witnesses |
| S01–S03 | standalone success, typed failure, event/result, and two-store isolation witnesses |
| L01–L07 | lifecycle local CLI/API CRUD, missing, sibling-isolation, and early-cleanup witnesses |
| L08–L12 | lifecycle remote placement, parity, failure, cancellation, and transport witnesses |
| Q01–Q03 | CLI routing, default-unavailable, and deprecated-show rejection subtests |
| Q04 | CLI deprecated-submit rejection subtest |
| X01–X05 | cross-package denominator/order, cleanup, performance, auth-scope, and excluded-restart audit rows |

### Timing and irreducible-case handoff

The local-after values are one diagnostic outer-process sample from the clean
detached checkout. The package-reported value is included to distinguish Go
test execution from checkout/process overhead. The supplied-before values are
the PRD values. The raw PR-CI itemization below is from the prior current-head
run and is retained as the exact baseline for the next current-head run.

| Package | Supplied before (s) | Clean-room local after, outer (s) | Go package (s) | Retained property / PR-CI row |
| --- | ---: | ---: | ---: | --- |
| execution | 22.748 | 5.232 | 3.512 | 13 starts remain: one shared host plus drain/liveness, partial-result, CLI visibility, and parity boundaries. New-head PR-CI pending. |
| controls | 16.710 | 2.939 | 1.114 | One shared host with provider/script edge responses; busy-loop lifecycle and transition boundaries remain property-required. New-head PR-CI pending. |
| standalone | 14.089 | 3.118 | 1.499 | Three roots remain; S03 retains two independent roots/catalogs for cross-store typed not-found. New-head PR-CI pending. |
| lifecycle | 13.572 | 5.283 | 3.436 | Existing two root-built roles, listener, client-disconnect, transport-failure, and negative-liveness boundaries remain property-required. New-head PR-CI pending. |
| cli | 3.163 | 2.258 | 0.330 | One reusable root/responder with fresh execution state; Q04 reuses its fresh case working directory for the payload and remains pre-dispatch. New-head PR-CI pending. |

The clean-room local sample is diagnostic and is not a PR-CI claim. If a new
PR-CI package remains at or above three seconds, the exact witness-level
itemization below supplies the retained property and the prior raw timing row;
review must replace the timing values with the new-head artifact values.

### Prior PR-CI raw per-test itemization

Run `33356881578` on reviewed head `177dda7adfb42bdab44b202b87a5b983257c90db`
reported package values CLI `3.168s`, controls `5.167s`, execution `12.446s`,
lifecycle `12.832s`, and standalone `13.587s`. The following rows are the
artifact's exact per-test timings for every owned top-level witness. The rows
are not additive: the functional job can run tests concurrently, while the
package value includes package-level setup and teardown. Each row is retained
because its public behavior, process/store boundary, or negative-liveness
property is part of the acceptance matrix and cannot be removed to meet a
timing threshold.

| Package | Top-level witness | Matrix | Prior PR-CI seconds | Irreducible property / boundary |
| --- | --- | --- | ---: | --- |
| cli | `TestBuildProcessRoutesEverySessionLeafThroughResolvedProductionComposition` | Q01–Q03 | 1.130 | Seven ordered session HTTP requests, resolved target/path, and default-unavailable behavior through one root/responder. |
| cli | `TestBuildProcessRejectsDeprecatedPortBeforeSubmitDispatch` | Q04 | 0.020 | Deprecated submit input must reject before request dispatch with empty stdout. |
| controls | `TestPausedFactorySessionReturnsInvocationPausedStatus` | C01 | 2.350 | Paused live Session invocation must return typed `INVOCATION_PAUSED` without a fabricated result. |
| controls | `TestPausedFactorySessionBuffersSubmittedWork` | C02 | 0.030 | Paused Session must retain submitted Work buffered and undispatched. |
| controls | `TestResumedFactorySessionDrainsBufferedWorkInOrder` | C03 | 0.100 | Resume must drain two buffered Work items in submission order through dispatch events. |
| controls | `TestPauseResumeEmitsDurableLifecycleEvents` | C04 | 0.060 | Durable PAUSE then RESUME Factory Events must arrive in order with accepted outcomes. |
| controls | `TestInterruptedWorkInspectSurfacesDispatchAndStopSummary` | C05 | 0.090 | Interrupted Work must expose dispatch context and stop summary without reaching complete. |
| controls | `TestAPIPauseResumeCancelAndTerminateFactorySession` | C06–C08 | 0.060 | Public pause/resume plus cancellation and termination lifecycle transitions remain asserted on durable Sessions. |
| controls | `TestAPIInvalidLifecycleTransitionReturnsConflict` | C09–C10 | 0.030 | Terminal Session rejects invalid resume/pause controls while preserving terminal state. |
| execution | `TestAPIPetriDispatchUsageReachesDispatchList` | E01–E02 | 3.060 | Provider command edge must preserve token usage when present and omit absent metadata. |
| execution | `TestWithServerDrainCannotReportSuccessWhileWorkIsNonTerminal` | E03–E04 | 1.750 | Finite server/site drain retains listener join, non-terminal failure, and one browser-edge call. |
| execution | `TestHostedFiniteRunsKeepEmptyAndTerminalSuccess` | E05–E08 | 4.580 | Finite empty and terminal runs retain separate server/site drain and browser-edge success boundaries. |
| execution | `TestHostedContinuousRunsStayLiveWhileIdle` | E09–E10 | 3.810 | Continuous idle runs must remain live through the negative-liveness window and cancel/join cleanly. |
| execution | `TestAPIResultAndResultsExposeTerminalInvocationData` | E11–E18 | 1.420 | Success, unresolved target, timeout, bad input, wrong type, and request cancellation retain distinct public outcomes. |
| execution | `TestAPIDispatchListAndDetailExposePublicCorrelation` | E19 | 3.140 | Dispatch list/detail must preserve public IDs, Session correlation, label, and completion facts. |
| execution | `TestAPIPartialResultIsAvailableBeforeTerminalCompletion` | E20 | 3.640 | In-flight provider gate retains partial/checkpoint visibility, RUNNING state, final NOT_READY, and interrupt cleanup. |
| execution | `TestCLIInvocationIsVisibleThroughAPISessionAndWorkReads` | E21 | 4.470 | CLI-hosted execution must remain observable through API Session/Work reads with matching result/correlation. |
| execution | `TestAPIInvocationResultMatchesCLICompatibleFacts` | E22 | 8.220 | Equivalent API and CLI invocation facts require a separate cross-transport parity witness. |
| lifecycle | `TestFactorySessionCreateListShowDelete` | L01 | 0.210 | CLI create/list/show/terminate/delete must preserve identity, path, status, and deletion. |
| lifecycle | `TestFactorySessionListMultipleSessions` | L02 | 0.410 | Two CLI Sessions must remain distinct while deleting one leaves its sibling readable. |
| lifecycle | `TestFactorySessionMissingShowAndDeleteFail` | L03 | 0.230 | Missing CLI Session operations must return typed not-found without damaging a live sibling. |
| lifecycle | `TestAPIOpenListGetAndCloseFactorySession` | L04 | 0.040 | API open/list/get/close lifecycle must preserve identity and remove the closed Session. |
| lifecycle | `TestAPIFactorySessionNotFoundUsesTypedError` | L05 | 0.060 | API missing get/delete must return stable HTTP 404 typed errors. |
| lifecycle | `TestAPIMultipleFactorySessionsRemainIsolated` | L06 | 0.060 | Closing one API Session must not alter its sibling's identity/path/status. |
| lifecycle | `TestFactorySessionCleanupRunsAfterEarlySubtestExit` | L07 | 0.040 | Registered cleanup must remove a child-opened Session and residue after early return. |
| lifecycle | `TestCLIRemoteRunStartsDurableSessionOnSelectedServer` | L08 | 0.130 | Remote admission retains client-disconnect and durable Session persistence/replay boundaries. |
| lifecycle | `TestCLILocalAndRemoteRunSuccessParityThroughRootProcess` | L09 | 2.510 | Local and selected-server success require separate placement and envelope/result parity. |
| lifecycle | `TestCLILocalAndRemoteRunDomainFailureParityThroughRootProcess` | L10 | 2.200 | Local and remote domain failure must retain terminal diagnostics without false success. |
| lifecycle | `TestCLILocalAndRemoteRunCancellationParityThroughRootProcess` | L11 | 1.820 | Local and remote blocking calls retain cancellation propagation and no success result. |
| lifecycle | `TestCLIRemoteRunTransportFailureKeepsStreamsAndExitObservable` | L12 | 0.050 | Unreachable selected server must retain empty stdout, non-nil exit, and transport diagnostic. |
| standalone | `TestBTRCP0StandaloneFixtureSuccessCharacterization` | S01 | 3.600 | MCP success retains fresh invocation stdio/store scope and ordered started/updated/completed events. |
| standalone | `TestBTRCP0StandaloneFixtureFailureCharacterization` | S02 | 1.790 | MCP dependency failure retains typed detail, unavailable result, ordered failure events, and no dispatch. |
| standalone | `TestBTRCP0StandaloneFixtureRunsRemainIsolated` | S03 | 4.140 | Two independent roots/catalogs/stores are required for distinct IDs, catalog-specific results, and typed cross-read not-found. |

## Customer journey

1. A detached clean checkout was created at
   `f5c654842d9b672befb51b2c555c9e917b5dffcc`, verified clean, and removed
   after validation. The validator made no implementation edit.
2. These exact list-only commands were run once each:

   ```text
   go test ./tests/functional/sessions/execution -list '^Test'
   go test ./tests/functional/sessions/controls -list '^Test'
   go test ./tests/functional/sessions/standalone -list '^Test'
   go test ./tests/functional/sessions/lifecycle -list '^Test'
   go test ./tests/functional/sessions/cli -list '^Test'
   ```

   Every command exited `0` and produced the denominator above.
3. These exact final commands were run once each, sequentially:

   ```text
   go test -count=1 -timeout=10m ./tests/functional/sessions/execution
   go test -count=1 -timeout=10m ./tests/functional/sessions/controls
   go test -count=1 -timeout=10m ./tests/functional/sessions/standalone
   go test -count=1 -timeout=10m ./tests/functional/sessions/lifecycle
   go test -count=1 -timeout=10m ./tests/functional/sessions/cli
   ```

   Execution, controls, standalone, lifecycle, and CLI exited `0` in
   `6.274s`, `2.891s`, `3.988s`, `8.464s`, and `2.345s` outer elapsed,
   respectively. The Go package-reported durations were `3.512s`, `1.114s`,
   `1.499s`, `3.436s`, and `0.330s`.
4. The prior package gates supplied focused repeat/race evidence and detailed
   public behavior/census assertions. This clean-room run supplied the
   cross-task integration check, and the bounded CLI pass reused Q04's
   case-owned working directory for its payload without changing its
   pre-dispatch assertion.

## Cross-task integration and usability

- Documentation discoverability: this report is in
  `docs/internal/development/functional-test-optimization/` and names the
  owning gates and review handoff.
- Permission and error behavior: unchanged; bad input, typed failure,
  cancellation, timeout, missing Session, transport, and rejected-transition
  outcomes remain covered by the package witnesses.
- Persistence/reload behavior: unchanged; durable Session, Work, result,
  dispatch, and Factory Event observations remain in the execution, controls,
  standalone, and lifecycle witnesses.
- Accessibility/keyboard/responsive behavior: not applicable; no UI changed.
- Operational signals: package-owned route, Session, listener, stream, root,
  fixture-path, and Process.Execute cleanup diagnostics remain asserted.

## Findings

| ID | Severity | Reproduction | Expected | Actual | Evidence |
| --- | --- | --- | --- | --- | --- |
| None | — | — | No local loopback defect requiring correction. | All current-head validation commands passed without repair. | Clean-room commands, package gates, and prior PR-CI artifact itemization above. |

## Verdict

**PASS** for GATE-CLEAN-001 and the story-007 local validation loopback. The
prior PR-CI artifact itemization is recorded for all 33 owned witnesses; the
new-head PR functional timing/coverage result, terminal CI, conflict
resolution, feedback convergence, and merge remain review-owned and are not
claimed here.

## Delta-plan request [Required for FAIL/BLOCKED]

Not applicable: no local criterion is FAIL or BLOCKED. Review must replace the
prior itemization values with the new-head `REV-CI-001` timing/coverage result
and owns terminal CI and merge under `REV-DELIVERY-001`.
