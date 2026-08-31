# Validation report: C15 Session-family functional-test optimization

## Environment and artifact

- Commit/build identifier: `81ec3a7e408fe1dd523e564ca03a20d509fc2ce0`.
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
| Exact 33-test denominator | PASS | In a clean detached checkout, `go test <package> -list '^Test'` exited `0` for every owned package and listed 9 execution, 7 controls, 3 standalone, 12 lifecycle, and 2 CLI top-level tests. | Future test additions after this artifact. |
| Complete clean-room behavior run | PASS | Exactly one `go test -count=1 -timeout=10m <package>` run per package exited `0`; no test failure, skip, or quarantine token was observed. | Hosted CI timing and coverage. |
| Nested behavior matrix | PASS | The complete E01–E22, C01–C10, S01–S03, L01–L12, Q01–Q04, and X01–X05 ledger remains mapped to the unchanged top-level witnesses below; stories 002–006 supplied focused behavior and cleanup evidence. | Restart and authorization remain outside this lane. |
| Process/topology reduction | PASS | Eligible setup was consolidated: execution `21 -> 13` retained starts, controls `7 -> 1` API-host start, standalone `4 -> 3` root builds, lifecycle retained its property-required two roles/one listener, and CLI uses one root/responder. | PR-host package timing is review-owned. |
| Zero-residue cleanup | PASS | All clean-room package runs passed package-owned cleanup assertions. Prior task-owned censes are balanced: execution routes/Sessions; controls six explicit Session opens/closes and zero routes/root residue; standalone roots `3/3`, fixture scopes `4/4`, contexts `4/4`, stdio pairs `4/4`, working directories `4/4`; lifecycle two process closes, one listener close, one stream close; CLI root/responder `1/1` and scenarios `2/2`. | OS-global resources outside fixture ownership. |
| Local timing handoff | PASS | Supplied-before and one diagnostic clean-room local-after sample are recorded below; no local sample is presented as PR-CI evidence. | `REV-CI-001` must supply current-head package timing and functional coverage. |
| PR functional coverage and terminal CI | PENDING REVIEW | The report prepares the exact package handoff but cannot prove a PR job before the PR is opened. | `REV-CI-001` (at least 61.6% functional coverage and package timing); `REV-DELIVERY-001` (terminal CI, feedback, conflicts, merge). |
| Scope and excluded surfaces | PASS | The three-dot audit against the local `origin/main` showed only the 13 intended files under the five owned functional packages; restart, shared support, board persistence, baselines, production, UI, contracts, and generated outputs were untouched. | Final origin/main sync is a delivery-stage action. |

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
the PRD values; PR-CI is deliberately left for review.

| Package | Supplied before (s) | Clean-room local after, outer (s) | Go package (s) | Retained property / PR-CI row |
| --- | ---: | ---: | ---: | --- |
| execution | 22.748 | 13.256 | 8.003 | 13 starts remain: one shared host plus drain/liveness, partial-result, CLI visibility, and parity boundaries. `REV-CI-001` pending. |
| controls | 16.710 | 9.054 | 2.617 | One shared host; busy-loop lifecycle and transition boundaries remain property-required. `REV-CI-001` pending. |
| standalone | 14.089 | 32.427 | 24.494 | Three roots remain; S03 retains two independent roots/catalogs for cross-store typed not-found. `REV-CI-001` pending. |
| lifecycle | 13.572 | 25.455 | 19.795 | Existing two root-built roles, listener, client-disconnect, transport-failure, and negative-liveness boundaries remain property-required. `REV-CI-001` pending. |
| cli | 3.163 | 6.364 | 0.711 | One reusable root/responder with fresh execution state; deprecated input remains pre-dispatch. `REV-CI-001` pending. |

No package is locally claimed below the three-second PR-CI requirement. If a
PR-CI package remains at or above three seconds, review must itemize its exact
irreducible tests and timing contribution using the retained properties above.

## Customer journey

1. A detached clean checkout was created at `81ec3a7e40`, verified clean, and
   removed after validation. The validator made no implementation edit.
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
   `13.256s`, `9.054s`, `32.427s`, `25.455s`, and `6.364s` outer elapsed,
   respectively.
4. The prior package gates supplied focused repeat/race evidence and detailed
   public behavior/census assertions; the clean-room run supplied the
   cross-task integration check without re-running those repeat gates.

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
| None | — | — | No local loopback defect requiring correction. | All local validation commands passed without repair. | Clean-room commands and package gates above. |

## Verdict

**PASS** for GATE-CLEAN-001 and the story-007 local validation loopback. The
review-owned PR functional timing/coverage result, terminal CI, conflict
resolution, feedback convergence, and merge remain unproven and are not
claimed here.

## Delta-plan request [Required for FAIL/BLOCKED]

Not applicable: no local criterion is FAIL or BLOCKED. Review must complete the
`REV-CI-001` timing/coverage comment and owns terminal CI and merge under
`REV-DELIVERY-001`.
