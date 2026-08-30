# C15 Sessions root-composition and MCP characterization

## Scope and result

- Story: `functional-test-optimization-c15-sessions-root-mcp-001`
- Parent behavior: `BEH-003` — evidence proves behavior, cleanup, coverage
  neutrality, and package performance.
- Captured: `2026-08-30`
- Baseline head: `42eeee4472656b8290f798c36a5b8c871b24d7d0`
- Environment: Windows 11 Home `10.0.26200`, `windows/amd64`, Go
  `go1.25.0`, 24 logical processors.
- Dependency fidelity: local-real root, Factory Session, Recordings, API, and
  MCP stdio boundaries with controlled provider/worker edges; zero remote or
  paid calls.
- Status: **PASS — GATE-CHAR complete; stories 002–004 remain open.**

This is an additive before-state record. Story 001 changes no test behavior,
production code, public contract, generated output, UI, shared functional
support, restart package, baseline, or CI configuration. `progress.txt` and
`prd.json` are ignored local worktree scaffolding and are not part of the PR
diff.

The PRD names `docs/temp/functional-test-optimization.md` and its companion as
operator-held source-plan authority. Those files are intentionally absent from
this checkout; the checked-in PRD, the prior operator-authority disposition
recorded in the C09 migration ledger, and the C14 root ledger supply the
characterization authority used here. No implementation decision is being made
from the missing temporary files.

## GATE-CHAR evidence

The exact discovery procedures required by the task were run on the baseline
head:

```text
go test ./tests/functional/sessions/root_composition/... -list '^Test' -count=1
go test ./tests/functional/sessions/mcp/... -list '^Test' -count=1
```

Both commands exited `0`. The first reported 20 selectors in
`root_composition` and one selector in the nested `runtime_api_fixture`
package. The second reported seven selectors in `mcp`. The source audit below
maps all 29 ROOT cases and all seven MCP cases, so there are 36/36 classified
behavior rows and zero unclassified rows.

The `-list` commands prove selector presence and package compilation. They do
not prove converted sharing, terminal-event synchronization, cleanup under
the new topology, coverage, package latency, or PR CI.

## Current selector discovery

### ROOT selectors

```text
TestAutomationWorkWithoutRecordedOccupancyRestoresThroughRecordingProjection
TestSessionsEffectsRemainInertThroughRootBuildProcessConstruction
TestNamedRuntimeArtifactCollisionUsesUTCDateAndExplicitSuffix
TestRecordingFormatsRemainObservableThroughReusableRootProcess
TestDetachedOperationsFunctionalContract
TestDetachedOperationsFunctionalValidation
TestFactoryRuntimeDispatchPlanningCancellationReachesPublishedWorkerThroughPublicDurableControl
TestSessionsLifecycleAndRuntimeOpeningActivateThroughRootBuildProcessAfterLifecycle
TestP3P7CanonicalPathPreservesTerminalCleanupAndReplayIsolation
TestSessionsPackagedRootShapeMatchesCanonicalServiceLayout
TestProcessExecuteRuntimeOpeningThroughReusableRootProcess
TestRootProcessCloseAfterFailedCommandPreservesTheCommandFailure
TestRootProcessCloseAfterSuccessfulCommandReportsNoFailure
TestRootBuildProcessIsInertAndReusableAcrossFactorySessions
TestRecordedFactoryRedactsDeclaredSecretAtRecordingWriteBoundary
TestRecordedFactoryRedactsSecretStepsAcrossLifecycle
TestSeededReplayResumeMaterializesRecordedWorkOnceThroughAssembledSession
TestSessionsWorkAdmissionAndResponseStreamActivateThroughRootBuildProcessAfterLifecycle
TestRootBuildProcessRoutesProviderAndScriptWorkThroughInjectedRunnerInstances
TestRootBuildProcessRunnerFailureRoutesToFailedDispatchThroughInjectedInstance
```

Nested selector:

```text
TestRuntimeAPIPackageFixtureCleanupIsIdempotentAndPreservesFailures
```

### MCP selectors

```text
TestMCPPauseResumeAndCancelTargetCanonicalFactorySession
TestMCPControlledSessionIsIsolatedFromAnotherAPIHostByDefault
TestMCPSynchronousFactorySessionReturnsTerminalResult
TestMCPSynchronousFailureReturnsStructuredFailure
TestMCPAsyncFactorySessionCanBePolledToSuccess
TestMCPAsyncFactorySessionCanBePolledToFailure
TestMCPAsyncCancellationIsIsolatedFromAnotherAPIHostByDefault
```

## Assertion-parity ledger

The selector names below are the current source witnesses. A slash suffix is
the exact `t.Run` name. For rows whose assertion is grouped in a helper, the
top-level selector remains the executable selector and the helper name scopes
the assertion set; no assertion is inferred from a file or function name.

### ROOT-001 through ROOT-029

| Case | Exact current selector/witness | Classification for later conversion | Observable assertion retained |
| --- | --- | --- | --- |
| ROOT-001 | `TestSessionsEffectsRemainInertThroughRootBuildProcessConstruction` | Shared inert root | `BuildProcess` causes zero lifecycle, runtime-opening, Work-admission, response-stream, or API-start effects. |
| ROOT-002 | `TestAutomationWorkWithoutRecordedOccupancyRestoresThroughRecordingProjection` | Shared with controlled replay/API edge; fresh factory, home, and recording | Replayed automation Work restores occupancy and lineage exactly once. |
| ROOT-003 | `TestNamedRuntimeArtifactCollisionUsesUTCDateAndExplicitSuffix` | Shared/pure platform reservation | Same UTC date reserves distinct safe paths and gives the second artifact the explicit `-2` suffix. |
| ROOT-004 | `TestRecordingFormatsRemainObservableThroughReusableRootProcess/default recording reserves distinct dated UUID artifacts and replays` | Shared root with fresh homes, artifacts, and session state | Two default recordings are distinct dated UUID artifacts and replay to equivalent facts without changing live artifacts. |
| ROOT-005 | `TestRecordingFormatsRemainObservableThroughReusableRootProcess/explicit JSONL recording appends through root process` | Shared root with fresh recording path and session state | Explicit JSONL recording contains valid records and preserves the trailing newline. |
| ROOT-006 | `TestDetachedOperationsFunctionalContract` (`testDetachedStartsAndInvocation`) | Shared inert root; stateful owner observation remains case-local | Live, durable async, and durable sync starts, invocation, activation, normalized IDs, correlations, and results match the public detached contract. |
| ROOT-007 | `TestDetachedOperationsFunctionalContract` (`testDetachedReadsAndControls`, `testDetachedResultsAndPreparation`) | Shared inert root; stateful owner observation remains case-local | Get/list/control/result/subscribe/prepare preserve mode, session, correlation, request, and not-ready facts. |
| ROOT-008 | `TestDetachedOperationsFunctionalValidation` | Shared inert root; validation has no external effects | Nil service, invalid mode/ID/control, empty activation, negative value, and invalid request cases return typed validation errors without mutation. |
| ROOT-009 | `TestFactoryRuntimeDispatchPlanningCancellationReachesPublishedWorkerThroughPublicDurableControl` | Shared root with a fresh held-worker/provider edge and session | Durable cancellation reaches the published worker and does not report false success. |
| ROOT-010 | `TestSessionsLifecycleAndRuntimeOpeningActivateThroughRootBuildProcessAfterLifecycle` | Shared root with fresh lifecycle/runtime-opening/API edge state | Injected lifecycle precedes runtime opening and public Factory Session facts appear. |
| ROOT-011 | `TestP3P7CanonicalPathPreservesTerminalCleanupAndReplayIsolation/isolated sessions reach one terminal outcome and replay equivalent facts` | Private root process for the cross-process replay witness | Two independently activated root processes produce isolated identities, one terminal outcome each, and equivalent replayed canonical facts. |
| ROOT-012 | `TestP3P7CanonicalPathPreservesTerminalCleanupAndReplayIsolation/provider failure terminalizes as failed and is never reported as success`; `/cancellation stops the held dispatch and releases the activated process` | Private process lifecycle in the composite selector; held dispatch and shutdown release are the irreducible property | Provider failure remains FAILED with no false success; held cancellation observes release and cleanup. |
| ROOT-013 | `TestSessionsPackagedRootShapeMatchesCanonicalServiceLayout` | Shared/pure package-shape observation | The packaged Sessions root retains the canonical service layout. |
| ROOT-014 | `TestProcessExecuteRuntimeOpeningThroughReusableRootProcess/opens requested Factory Session` | Shared root with fresh factory, home, API listener, and Session | `Process.Execute` opens the requested Factory Session before completion and publishes its canonical stream identity. |
| ROOT-015 | `TestProcessExecuteRuntimeOpeningThroughReusableRootProcess/corrupt current-board recording stops opening` | Shared root with fresh corrupt recording and input state | Corrupt current-board input returns the expected diagnostic and does not open a live session. |
| ROOT-016 | `TestProcessExecuteRuntimeOpeningThroughReusableRootProcess/unavailable Factory does not register session`; `/replay loader failure stops before live activation` | Shared root with fresh unavailable/replay-loader edges | Unavailable Factory and replay-loader failure do not register or activate a live session. |
| ROOT-017 | `TestRootProcessCloseAfterFailedCommandPreservesTheCommandFailure` | Private Process close lifecycle | A command failure remains the primary error while close still runs. |
| ROOT-018 | `TestRootProcessCloseAfterSuccessfulCommandReportsNoFailure` | Private Process close lifecycle | Successful command close reports no failure, including on repeated close. |
| ROOT-019 | `TestRootBuildProcessIsInertAndReusableAcrossFactorySessions` | Shared root with fresh Session, workspace, event stream, response stream, and edge state | One inert process serves two sessions with distinct session/stream identities and matching outcomes. |
| ROOT-020 | `TestRootBuildProcessIsInertAndReusableAcrossFactorySessions/direct JavaScript transport start failure` | Shared root with a fresh direct-start failure edge | Direct JavaScript transport-start failure returns without fallback. |
| ROOT-021 | `TestRecordedFactoryRedactsDeclaredSecretAtRecordingWriteBoundary`; `TestRecordedFactoryRedactsSecretStepsAcrossLifecycle/secret step`; `/inline secret step` | Shared root with fresh recording/home/input state and controlled runner | Declared, secret-step, and inline-secret values are absent from records/replay while plain values survive. |
| ROOT-022 | `TestSeededReplayResumeMaterializesRecordedWorkOnceThroughAssembledSession/in-flight tail` | Shared root with fresh seeded replay payload, home, and Session | Unfinished seeded replay materializes one Work item and resumes at the expected initial state without redispatch. |
| ROOT-023 | `TestSeededReplayResumeMaterializesRecordedWorkOnceThroughAssembledSession/finished recording` | Shared root with fresh seeded replay payload, home, and Session | Finished seeded replay remains terminal and does not redispatch. |
| ROOT-024 | `TestSessionsWorkAdmissionAndResponseStreamActivateThroughRootBuildProcessAfterLifecycle` | Shared root with fresh API listener, Session, Work, and response stream | Work admission and response events appear through public Session reads after lifecycle opening. |
| ROOT-025 | `TestRootBuildProcessRoutesProviderAndScriptWorkThroughInjectedRunnerInstances` | Shared root with fresh controlled provider/script runner edges and Work | Provider and script Work publish the expected runner identities and outputs. |
| ROOT-026 | `TestRootBuildProcessRunnerFailureRoutesToFailedDispatchThroughInjectedInstance` | Shared root with fresh failing runner edge and Session | Injected runner failure terminalizes the dispatch as FAILED with expected facts. |
| ROOT-027 | `runtime_api_fixture/TestRuntimeAPIPackageFixtureCleanupIsIdempotentAndPreservesFailures/normal cleanup probes listener and removes root` | Private cleanup fixture: listener, Process, and temporary root are the asserted resources | Normal cleanup closes the listener, removes the root, and remains idempotent. |
| ROOT-028 | `runtime_api_fixture/TestRuntimeAPIPackageFixtureCleanupIsIdempotentAndPreservesFailures/injected execute and close failures remain visible` | Private cleanup fixture: independent execute/close failures must be joined while cleanup continues | Both failures remain visible and independent listener/root cleanup still runs. |
| ROOT-029 | `runtime_api_fixture/TestRuntimeAPIPackageFixtureCleanupIsIdempotentAndPreservesFailures/reachable listener fails the cleanup probe` | Private cleanup fixture: listener reachability is the asserted boundary | A reachable listener is reported as a cleanup failure rather than accepted as closed. |

The private ROOT rows are not broad exemptions. Their reasons are the exact
properties named in the assertion column: independent root-process replay,
Process close ownership, or listener/root cleanup and error-joining. Any later
split or shared conversion must preserve those properties or update this
ledger before claiming eligibility.

### MCP-001 through MCP-007

| Case | Exact current selector/witness | Classification for later conversion | Observable assertion retained |
| --- | --- | --- | --- |
| MCP-001 | `TestMCPPauseResumeAndCancelTargetCanonicalFactorySession` | Shared assembled host candidate with a private real stdio client/pipe pair and fresh busy-loop Session | MCP tool discovery includes the control tools; the same canonical ID reaches PAUSED, RUNNING, and CANCELED through pause/resume/cancel responses and reads. |
| MCP-002 | `TestMCPControlledSessionIsIsolatedFromAnotherAPIHostByDefault` | Shared MCP host candidate with private stdio pipes plus a required second in-process API host | The owning MCP host sees PAUSED while the separately constructed API host returns 404 for that Session ID. |
| MCP-003 | `TestMCPSynchronousFactorySessionReturnsTerminalResult` | Shared host candidate with private real stdio pipes and fresh success workflow/Session | `start_sync` returns SUCCEEDED/COMPLETED, a canonical Session ID, and a FINAL result; the subsequent read agrees. |
| MCP-004 | `TestMCPSynchronousFailureReturnsStructuredFailure` | Shared host candidate with private real stdio pipes and fresh failure workflow/Session | `start_sync` returns structured FAILED detail without a FINAL primary success, and the subsequent read remains non-FINAL. |
| MCP-005 | `TestMCPAsyncFactorySessionCanBePolledToSuccess` | Shared host candidate with private real stdio pipes and fresh async success Session | `start_async` reports RUNNING, public get/get_result observation reaches SUCCEEDED and FINAL, and the ID remains canonical. |
| MCP-006 | `TestMCPAsyncFactorySessionCanBePolledToFailure` | Shared host candidate with private real stdio pipes and fresh async failure Session | Async observation reaches FAILED and UNAVAILABLE with structured failure and no primary success payload. |
| MCP-007 | `TestMCPAsyncCancellationIsIsolatedFromAnotherAPIHostByDefault` | Shared MCP host candidate with private stdio pipes, a required second API host, and fresh busy-loop Session | The owning MCP host reaches CANCELED while the separate API host returns 404. |

The retained private MCP transport reason is common and exact: each case
asserts the real JSON-RPC-over-stdio boundary, so its input/output pipes and
stdio client must be isolated from other cases. MCP-002 and MCP-007 additionally
retain a second in-process API host because the cross-host `404` is itself an
assertion. The assembled root/MCP host is a sharing candidate; the pipe and
client are not.

## Resource and synchronization topology

Counts are source call-site counts unless explicitly labelled runtime count.
The audit intentionally does not count comments, error strings, or helper
implementations hidden in the excluded shared-support package.

| Surface | Current count | Evidence and interpretation |
| --- | ---: | --- |
| ROOT top-level selectors | 20 | `go test .../root_composition/... -list '^Test'`; the parent package reports 20. |
| ROOT nested cleanup selector | 1 top-level / 3 named subtests | `runtime_api_fixture` reports one selector; its three subtests are ROOT-027..029. |
| ROOT behavioral cases | 29 | The C14 parity ledger plus the three nested cleanup subtests map every current assertion to ROOT-001..029. |
| ROOT direct `BuildProcess*` calls | 13 | 12 `support.BuildProcess*` calls and one direct `root.BuildProcess` call across the owned subtree, including one nested-fixture call; support helpers that build internally are not silently expanded. |
| ROOT `StartFunctionalAPIServer` call sites | 3 | Lifecycle opening, dispatch cancellation, and Work/response-stream tests use the shared functional helper. |
| ROOT `StartProcessCommand` call sites | 5 | Automation recovery, Process.Execute opening, reusable-root, P3/P7, and seeded replay witnesses. |
| ROOT `RunFactoryToCompletionWithEdgesAndResponseEvents` call sites | 2 | Provider and script runner identity/failure assertions; each call hides its support-owned process setup. |
| ROOT `NewProcessAPIServer` call sites | 6 | Five parent-package observation boundaries and one runtime API package fixture boundary. |
| ROOT `os.Pipe` call sites | 0 | Root composition uses `Process.Execute` and in-process API/event boundaries; no child process or stdio pipe is asserted here. |
| ROOT fixed sleep sites | 2 | Two pre-existing `time.Sleep(25 * time.Millisecond)` sites: Work admission response observation and retained canonical dispatch observation. Story 001 adds none. |
| ROOT explicit ticker/timeout guards | 1 ticker, 3 `time.After` guards | The nested cleanup fixture polls public Session stop status at 10ms; P3/P7 and fixture shutdown use bounded failure guards. These are current evidence, not new waits. |
| ROOT public status wait call sites | 5 | Three `WaitForStatus`/`WaitForSessionTerminalStatus` calls and one `WaitForSessionStopped` path plus the nested stop-status ticker; readiness waits are recorded separately from completion claims. |
| MCP top-level selectors/cases | 7 / 7 | `go test .../mcp/... -list '^Test'`; each selector is one matrix case. |
| MCP assembled-host helper call sites/runtime hosts | 1 / 7 | `startRootRuntimeMCPServer` contains one `BuildProcessWithContext` call and is invoked once per case. |
| MCP real stdio pipe pairs | 2 per host / 14 runtime pairs | The helper creates one stdin pipe and one stdout pipe for each of seven hosts. These are the retained real transport boundary. |
| MCP stdio clients/Sessions | 7 / 7 | One client and one fresh workflow Session per case. |
| MCP secondary API hosts | 1 call site / 2 runtime hosts | MCP-002 and MCP-007 require cross-host `404` assertions. |
| MCP subscriptions | 0 | Current completion uses public status/result polling; subscribe-before-act is a later MCP conversion requirement, not a current claim. |
| MCP fixed polling sites | 3 | Three helper sites sleep `15ms`: async-success polling, async-failure polling, and lifecycle-status polling. Story 001 changes none. |
| MCP `time.After` guards | 2 | One 100ms startup backstop and one 5s stdio shutdown backstop; neither is claimed as a terminal-event proof. |
| MCP polling helpers | 3 | Two async terminal helpers and one Session-status helper; all are current witnesses to replace in story 003. |

### Cleanup and OS-boundary census

The current tests register temporary homes, factory/workflow roots, recordings,
API listeners, Processes, event/response streams, and MCP pipe endpoints at the
test or helper boundary. The cleanup assertions are ROOT-027..029 plus the
per-test `t.Cleanup`/`CleanupProcess` registrations; the source audit does not
claim a package-wide balanced census before the conversion. The package has no
child executable witness. MCP deliberately has 14 runtime `os.Pipe` pairs and
keeps them because stdio framing is asserted. The C15 lane therefore starts
with zero child-process boundaries and does not add an OS boundary in story
001.

## Available timing evidence

The PRD carries the supplied prioritization observations of
`root_composition=32.509s` and `mcp=23.950s`. They were not reclassified as a
portable local threshold. The checked-in C14 root ledger additionally records
an older root baseline median of `39.288s` and a post-C14 root result median of
`20.566s`; that comparison is retained as historical context, not claimed as
a C15 MCP measurement. No checked-in MCP before-median artifact was found.

No full unfiltered package run was added to GATE-CHAR: the declared story
procedure requires selector discovery and source/resource audit, and the
existing C14 root witness plus supplied MCP observation provide the available
before context without spending a pre-change timing sample that cannot prove
conversion behavior. Stories 002–004 own focused behavior, race, final
package, coverage, and PR timing evidence.

## Remaining gates

| Gate | Not proved by this ledger | Owner |
| --- | --- | --- |
| GATE-ROOT | Shared root conversion, fresh Session isolation, canonical terminal events, and converted cleanup | Story 002 |
| GATE-MCP | Shared MCP host, subscribe-before-act terminal observation, real stdio parity, and converted cleanup | Story 003 |
| GATE-RACE | Race safety after sharing/cancellation changes | Story 002/003 as declared |
| GATE-PR | 61.6% functional coverage and exact PR package timing | Story 004/review |
| VAL-001 | Clean-room integrated root plus MCP run and loopback report | Story 004 |
| REVIEW-CI / REVIEW-MERGE | Terminal CI, conflicts, approval, and merge | Review stage |
