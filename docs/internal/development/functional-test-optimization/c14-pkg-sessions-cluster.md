# C14 Sessions functional-test cluster characterization

## Scope and result

- Story: `fto-c14-pkg-sessions-cluster-001`
- Parent behavior: `BEH-001` — the owned Sessions functional packages retain
  their public behavior while later stories remove measured intrinsic setup and
  waiting.
- Captured: `2026-08-29`
- Baseline head: `fea2e30a499384182d2fabe7038767e3c2f9c5e5`
- Environment: Windows `windows/amd64`, Go `go1.25.0`, 24 logical processors.
- Dependency fidelity: local-real root, API, ACP, filesystem, and process
  boundaries with controlled worker/provider edges; zero remote or paid calls.
- Status: **PASS — characterization complete; optimization remains assigned to
  stories 002–004 and final reconciliation to story 005.**

This is an additive before-state record. Story 001 changes no test behavior,
production code, public contract, generated output, UI, shared functional
support, restart package, or CI configuration. `progress.txt` was absent at
iteration start and is local ignored scaffolding; it is not part of the
tracked evidence or PR diff.

## Reproducible discovery and command shape

The package commands were run sequentially on the same checkout and host so
that one package did not contend with another package's sample:

```text
go test ./tests/functional/sessions/root_composition/... -count=1
go test ./tests/functional/sessions/chat_sessions/root_composition/... -count=1
go test ./tests/functional/sessions/execution/... -count=1
```

Each command was run three times with a PowerShell `Diagnostics.Stopwatch`; all
nine samples exited `0`. The exact selector discovery commands were:

```text
go test ./tests/functional/sessions/root_composition/... -list '^Test' -count=1
go test ./tests/functional/sessions/chat_sessions/root_composition/... -list '^Test' -count=1
go test ./tests/functional/sessions/execution/... -list '^Test' -count=1
```

Discovery exited `0` for every package. The owned denominator is 26 ROOT,
25 CHAT, and 9 EXEC top-level selectors, for 60 selectors. The ROOT `...`
command also runs the nested `runtime_api_fixture` package, whose one
top-level cleanup selector has three subcases mapped to ROOT-027 through
ROOT-029.

## Three-sample unchanged baseline

| Package command | Sample 1 | Sample 2 | Sample 3 | Median | Exit codes |
| --- | ---: | ---: | ---: | ---: | --- |
| `./tests/functional/sessions/root_composition/...` | 44.998s | 39.288s | 35.766s | **39.288s** | 0, 0, 0 |
| `./tests/functional/sessions/chat_sessions/root_composition/...` | 55.551s | 32.238s | 62.003s | **55.551s** | 0, 0, 0 |
| `./tests/functional/sessions/execution/...` | 34.896s | 40.043s | 32.919s | **34.896s** | 0, 0, 0 |

These medians are the pre-change denominators. The spread is retained as
observed host variance; no quiet-host or absolute-latency claim is made.

## JSON selector profile

The selector profile used `go test -json "$package/..." -count=1` and read the
Go test event `Elapsed` value for each terminal selector. The first profile
pass was captured through an interactive terminal and wrapped long event
lines; a second diagnostic capture, with stdout/stderr read without terminal
formatting, was required to preserve the complete rows below. Both captures
exited `0`; the clean second capture is the ledger artifact and no code or
test behavior changed between them.
The clean captured package results were `root_composition=73.013s`,
`chat_sessions/root_composition=58.380s`, and `execution=82.688s`, all exit
`0`. These profile runs were slower than the isolated baseline samples under
host contention and are diagnostic selector data, not replacement timing
denominators.

### ROOT selectors

| Selector | Result | Elapsed |
| --- | --- | ---: |
| `TestAutomationWorkWithoutRecordedOccupancyRestoresThroughRecordingProjection` | PASS | 1.45s |
| `TestSessionsEffectsRemainInertThroughRootBuildProcessConstruction` | PASS | 0.32s |
| `TestNamedRuntimeArtifactCollisionUsesUTCDateAndExplicitSuffix` | PASS | 0.18s |
| `TestDefaultRecordingUsesDistinctDatedUUIDArtifactsAndReplaysThroughRootProcess` | PASS | 23.39s |
| `TestExplicitJSONLRecordingUsesAppendStorageThroughRootProcess` | PASS | 16.68s |
| `TestDetachedOperationsFunctionalContract` | PASS | 0.06s |
| `TestDetachedOperationsFunctionalValidation` | PASS | 0.00s |
| `TestFactoryRuntimeDispatchPlanningCancellationReachesPublishedWorkerThroughPublicDurableControl` | PASS | 0.66s |
| `TestSessionsLifecycleAndRuntimeOpeningActivateThroughRootBuildProcessAfterLifecycle` | PASS | 16.67s |
| `TestP3P7CanonicalPathPreservesTerminalCleanupAndReplayIsolation` | PASS | 5.94s |
| `TestSessionsPackagedRootShapeMatchesCanonicalServiceLayout` | PASS | 0.00s |
| `TestProcessExecuteOpensRequestedFactorySessionThroughRoot` | PASS | 16.58s |
| `TestProcessExecuteCorruptCurrentBoardRecordingStopsOpening` | PASS | 16.54s |
| `TestProcessExecuteUnavailableFactoryDoesNotRegisterSession` | PASS | 15.80s |
| `TestProcessExecuteReplayLoaderFailureStopsBeforeLiveActivation` | PASS | 0.63s |
| `TestRootProcessCloseAfterFailedCommandPreservesTheCommandFailure` | PASS | 16.34s |
| `TestRootProcessCloseAfterSuccessfulCommandReportsNoFailure` | PASS | 0.85s |
| `TestRootBuildProcessIsInertAndReusableAcrossFactorySessions` | PASS | 18.92s |
| `TestRootProcessReportsDirectJavaScriptTransportStartFailure` | PASS | 15.68s |
| `TestRecordedFactoryRedactsDeclaredSecretAtRecordingWriteBoundary` | PASS | 4.15s |
| `TestRecordedFactoryRedactsSecretStepAndPreservesPlainStepAcrossLifecycle` | PASS | 5.71s |
| `TestRecordedFactoryRedactsInlineSecretStepAndPreservesPlainStepAcrossLifecycle` | PASS | 24.25s |
| `TestSeededReplayResumeMaterializesRecordedWorkOnceThroughAssembledSession` | PASS | 7.25s |
| `TestSessionsWorkAdmissionAndResponseStreamActivateThroughRootBuildProcessAfterLifecycle` | PASS | 16.73s |
| `TestRootBuildProcessRoutesProviderAndScriptWorkThroughInjectedRunnerInstances` | PASS | 16.86s |
| `TestRootBuildProcessRunnerFailureRoutesToFailedDispatchThroughInjectedInstance` | PASS | 15.96s |

The nested ROOT cleanup package profile was:

| Selector | Result | Elapsed |
| --- | --- | ---: |
| `TestRuntimeAPIPackageFixtureCleanupIsIdempotentAndPreservesFailures` | PASS | 0.00s |
| `.../normal_cleanup_probes_listener_and_removes_root` | PASS | 0.04s |
| `.../injected_execute_and_close_failures_remain_visible` | PASS | 0.02s |
| `.../reachable_listener_fails_the_cleanup_probe` | PASS | 0.00s |

### CHAT selectors

| Selector | Result | Elapsed |
| --- | --- | ---: |
| `TestACPSessionAnswersEachTurnWithThatTurnsOwnResult` | PASS | 1.67s |
| `TestPackagedFactoriesCompleteOneACPPromptTurn` | PASS | 7.22s |
| `TestPackagedPlanParallelCompletesOneACPPromptTurn` | PASS | 3.19s |
| `TestFactoryBuilderGreetsOnAVagueFirstACPTurn` | PASS | 3.34s |
| `TestPackagedJavaScriptFactoryCompletesOneACPPromptTurn` | PASS | 3.06s |
| `TestPackagedJavaScriptFactoryWithStructuredResultStreamsItsResult` | PASS | 3.48s |
| `TestACPPromptDelegationStartsOneFactorySessionAndReusesItForLaterTurns` | PASS | 0.96s |
| `TestACPPromptDelegationFailedFactoryInvocationReportsAnACPError` | PASS | 0.94s |
| `TestACPPromptDelegationUnresolvableFactoryTargetFailsSafelyAndTerminalizes` | PASS | 0.57s |
| `TestACPPromptDelegationRedeliveredRequestMakesNoSecondFactoryDispatch` | PASS | 1.16s |
| `TestACPPromptDelegationConcurrentPromptRejectsAsBusyWithNoFactoryDispatch` | PASS | 0.85s |
| `TestACPServerReachesCanonicalChatSessionsAuthorityThroughRootBuildProcess` | PASS | 0.29s |
| `TestACPServerWithNoAuthoredAgentProfileOffersEveryInstalledFactory` | PASS | 0.30s |
| `TestACPServerAuthoredAllowedTargetsStillRestrictsCatalog` | PASS | 0.32s |
| `TestACPServerSetConfigOptionSelectsAnotherInstalledFactory` | PASS | 0.10s |
| `TestACPServerAuthoredAllowedTargetsAreOfferedInAuthoredOrder` | PASS | 0.30s |
| `TestACPServerUnrestrictedTargetsAreOfferedCurrentFirstThenSorted` | PASS | 0.04s |
| `TestACPServeCommandStreamsThroughRootBuildProcessWithoutDuplicateFinalText` | PASS | 3.09s |
| `TestACPServeCommandStreamsUsageUpdateThroughRootBuildProcess` | PASS | 3.65s |
| `TestACPStreamUsagePeerProcess` | PASS | 0.00s |
| `TestOneACPWorkerDeliversEveryUpdateAsChildContent` | PASS | 4.17s |
| `TestTwoACPWorkersKeepChildStreamsAttributed` | PASS | 6.11s |
| `TestACPWorkerChildStreamSurvivesRetainedReplay` | PASS | 6.00s |
| `TestACPWorkerChildPeerProcess` | PASS | 0.00s |
| `TestJavaScriptFactoryChildrenAreVisibleAsWorkers` | PASS | 7.14s |

### EXEC selectors

| Selector | Result | Elapsed |
| --- | --- | ---: |
| `TestAPIPetriDispatchUsageReachesDispatchList` | PASS | 3.75s |
| `TestWithServerDrainCannotReportSuccessWhileWorkIsNonTerminal` | PASS | 1.49s |
| `TestHostedFiniteRunsKeepEmptyAndTerminalSuccess` | PASS | 1.64s |
| `TestHostedContinuousRunsStayLiveWhileIdle` | PASS | 1.72s |
| `TestAPIResultAndResultsExposeTerminalInvocationData` | PASS | 50.68s |
| `TestAPIDispatchListAndDetailExposePublicCorrelation` | PASS | 5.50s |
| `TestAPIPartialResultIsAvailableBeforeTerminalCompletion` | PASS | 6.52s |
| `TestCLIInvocationIsVisibleThroughAPISessionAndWorkReads` | PASS | 3.27s |
| `TestAPIInvocationResultMatchesCLICompatibleFacts` | PASS | 8.04s |

## Setup, process, and wait profile

The source scan below is a current-head call-site inventory, not a claim that
each call site executes exactly once. Several calls are behind the existing
functional support entry points (`StartFunctionalAPIServer`,
`RunFactoryToCompletion...`, and the Chat `buildChatProcess` cohort helper).
The planning packet's 31 direct-build reference is retained as planning
context; this current executable call-site scan found 25 explicit
`support.BuildProcess*`/`root.BuildProcess` calls in the three owned trees,
excluding diagnostic strings and comments.

| Owned package | Explicit BuildProcess call sites | Direct `StartProcessCommand` call sites | Existing fixed sleeps | Existing ticker / timeout waits | Runtime observations |
| --- | ---: | ---: | --- | --- | --- |
| ROOT | 19 (15 support, 4 root) | 3 | 2 × 25ms | 1 ticker; 3 `time.After` guards; public status/Factory Event waits | Nested fixture counter asserts `Process.Execute` starts exactly once; root-process lifecycle and close assertions remain active. |
| CHAT | 1 shared `BuildProcessWithContext` boundary in `buildChatProcess` | 0 | none | 2 `time.After` cleanup/termination guards; ACP pipes and stream reads are event-driven | Existing package census: processes 24/24, connections 58/58, response streams 58/58, pipes 56/56, sessions 29/29, turns 30/30, active calls 34/34, peers 4/4, paths 97/97, listeners 0/0, violations 0. |
| EXEC | 5 support | 5 | 1 × 5ms | 2 tickers; 2 `time.After` guards; bounded public observation helpers | Hosted mode edge counters assert one listener start and one listener stop per scenario; no package-wide process census exists yet. |

The three fixed sleeps are pre-existing synchronization in
`work_admission_response_stream_test.go`, `p3_p7_behavior_matrix_test.go`,
and `execution/helpers_test.go`. Story 001 adds no sleep and changes no wait
helper. Existing comments justify the bounded polling or timeout guards where
the public projection or process boundary exposes no deterministic event.

The source inventory also shows why later optimization must be profile-backed:
ROOT's largest selector costs are default recording/replay, secret redaction,
reusable-root, Process.Execute opening/close, and public lifecycle/work paths;
CHAT's largest costs are packaged Factory and child-stream cohorts; EXEC's
largest cost is the terminal invocation matrix. These are candidates only;
they are not changed by characterization.

### Repeated compatible setup observed

| Package | Repeated or shared setup observed before edits | Characterization decision |
| --- | --- | --- |
| ROOT | Default recording invokes two live runs and two replay runs; several public API scenarios obtain roots through `StartFunctionalAPIServer`; provider/script routing uses two `RunFactoryToCompletion...` calls; secret-redaction cases build live and replay processes. | No reuse was introduced. The later ROOT story must prove matching home, Factory, edge, session, stream, and cleanup ownership before sharing any of these paths. |
| CHAT | Immutable catalog tests already share one catalog cohort; the fixed controlled cohort is shared where profile state is unchanged; activation-owning prompts, authored profiles, and interrupted streams use isolated processes/homes. | Existing cohort classifications are frozen. Later CHAT work may extend only immutable or fixed-profile cohorts and must retain the process-fallback reason for terminal activations. |
| EXEC | Public API result subcases repeatedly create isolated Factory/API fixtures; hosted drain modes create one process/listener per mode; visibility and CLI/API parity each own their process boundary. | No reuse was introduced. Later EXEC work must preserve the API/CLI boundary and the per-scenario listener, cancellation, and result ownership. |

## Assertion parity ledger

Every planned case has a current passing witness. The witness names below are
the exact selector or nested selector present before optimization. A later
scenario consolidation may rename a selector only after it records a direct
old-to-new mapping with equal or stronger assertions.

### ROOT-001 through ROOT-029

| Case | Current passing witness | Observable property retained |
| --- | --- | --- |
| ROOT-001 | `TestSessionsEffectsRemainInertThroughRootBuildProcessConstruction` | Build has no lifecycle, opening, admission, response-stream, or API-start effect. |
| ROOT-002 | `TestAutomationWorkWithoutRecordedOccupancyRestoresThroughRecordingProjection` | Replayed automation Work restores occupancy and lineage once. |
| ROOT-003 | `TestNamedRuntimeArtifactCollisionUsesUTCDateAndExplicitSuffix` | UTC date and collision suffix produce distinct safe paths. |
| ROOT-004 | `TestRecordingFormatsRemainObservableThroughReusableRootProcess` / `default recording reserves distinct dated UUID artifacts and replays` | Two default recordings are distinct dated UUID artifacts and replay to equivalent facts. |
| ROOT-005 | `TestRecordingFormatsRemainObservableThroughReusableRootProcess` / `explicit JSONL recording appends through root process` | Explicit JSONL recording appends valid records and preserves the trailing newline. |
| ROOT-006 | `TestDetachedOperationsFunctionalContract` / `live_*`, `durable_*` subcases | Live, durable async, and durable sync starts, invocation, and activation normalize and return expected results. |
| ROOT-007 | `TestDetachedOperationsFunctionalContract` / `live_*`, `durable_*` subcases | Get, list, controls, result, subscription, and preparation preserve mode and correlation facts. |
| ROOT-008 | `TestDetachedOperationsFunctionalValidation` | Nil services, invalid modes/IDs, empty activation, negative values, and invalid controls retain typed validation errors without mutation. |
| ROOT-009 | `TestFactoryRuntimeDispatchPlanningCancellationReachesPublishedWorkerThroughPublicDurableControl` | Durable cancellation reaches the held worker and does not report false success. |
| ROOT-010 | `TestSessionsLifecycleAndRuntimeOpeningActivateThroughRootBuildProcessAfterLifecycle` | Injected lifecycle precedes runtime opening and public session facts appear. |
| ROOT-011 | `TestP3P7CanonicalPathPreservesTerminalCleanupAndReplayIsolation` / `isolated_sessions_reach_one_terminal_outcome_and_replay_equivalent_facts` | Two canonical sessions remain isolated and replay-equivalent. |
| ROOT-012 | `TestP3P7CanonicalPathPreservesTerminalCleanupAndReplayIsolation` / `provider_failure...`; `cancellation...` | Provider failure stays failed and held-dispatch cancellation releases the process. |
| ROOT-013 | `TestSessionsPackagedRootShapeMatchesCanonicalServiceLayout` | Shipped Sessions root package layout remains canonical. |
| ROOT-014 | `TestProcessExecuteRuntimeOpeningThroughReusableRootProcess` / `opens requested Factory Session` | `Process.Execute` opens the requested Factory Session before command completion. |
| ROOT-015 | `TestProcessExecuteRuntimeOpeningThroughReusableRootProcess` / `corrupt current-board recording stops opening` | Corrupt current-board recording stops opening with the expected diagnostic. |
| ROOT-016 | `TestProcessExecuteRuntimeOpeningThroughReusableRootProcess` / `unavailable Factory does not register session`; `/replay loader failure stops before live activation` | Unavailable Factory and replay-loader failures do not register or activate a live session. |
| ROOT-017 | `TestRootProcessCloseAfterFailedCommandPreservesTheCommandFailure` | Command failure remains primary while close completes. |
| ROOT-018 | `TestRootProcessCloseAfterSuccessfulCommandReportsNoFailure` | Successful command close reports no failure. |
| ROOT-019 | `TestRootBuildProcessIsInertAndReusableAcrossFactorySessions` | One inert process serves two sessions with distinct session/stream identities. |
| ROOT-020 | `TestRootBuildProcessIsInertAndReusableAcrossFactorySessions` / `direct JavaScript transport start failure` | Injected JavaScript transport-start failure returns directly without fallback. |
| ROOT-021 | `TestRecordedFactoryRedactsDeclaredSecretAtRecordingWriteBoundary`; `TestRecordedFactoryRedactsSecretStepsAcrossLifecycle` / `secret step`, `inline secret step` | Declared, step, and inline secrets stay redacted while plain values survive recording/replay. |
| ROOT-022 | `TestSeededReplayResumeMaterializesRecordedWorkOnceThroughAssembledSession` / `in-flight_tail` | In-flight seeded replay materializes Work once and completes through the assembled session. |
| ROOT-023 | `TestSeededReplayResumeMaterializesRecordedWorkOnceThroughAssembledSession` / `finished_recording` | Finished seeded replay remains terminal and does not redispatch. |
| ROOT-024 | `TestSessionsWorkAdmissionAndResponseStreamActivateThroughRootBuildProcessAfterLifecycle` | Work admission and response events appear through public Sessions reads. |
| ROOT-025 | `TestRootBuildProcessRoutesProviderAndScriptWorkThroughInjectedRunnerInstances` | Provider and script Work publish the expected runner identity and output. |
| ROOT-026 | `TestRootBuildProcessRunnerFailureRoutesToFailedDispatchThroughInjectedInstance` | Injected runner failure remains terminally failed with the expected dispatch facts. |
| ROOT-027 | `TestRuntimeAPIPackageFixtureCleanupIsIdempotentAndPreservesFailures` / `normal_cleanup_probes_listener_and_removes_root` | Normal cleanup is idempotent, closes the listener, and removes the fixture root. |
| ROOT-028 | `TestRuntimeAPIPackageFixtureCleanupIsIdempotentAndPreservesFailures` / `injected_execute_and_close_failures_remain_visible` | Execute and close failures remain visible while independent cleanup continues. |
| ROOT-029 | `TestRuntimeAPIPackageFixtureCleanupIsIdempotentAndPreservesFailures` / `reachable_listener_fails_the_cleanup_probe` | A reachable listener is reported rather than treated as clean shutdown. |

### CHAT-001 through CHAT-025

| Case | Current passing witness | Observable property retained |
| --- | --- | --- |
| CHAT-001 | `TestACPSessionAnswersEachTurnWithThatTurnsOwnResult` | Three ACP turns return their own Worker and assistant result without earlier-turn text. |
| CHAT-002 | `TestPackagedFactoriesCompleteOneACPPromptTurn` / `goal` | `@you/goal` returns its expected result through one ACP prompt. |
| CHAT-003 | `TestPackagedFactoriesCompleteOneACPPromptTurn` / `classify` | `@you/classify` preserves classification and answer calls. |
| CHAT-004 | `TestPackagedFactoriesCompleteOneACPPromptTurn` / `loop` | `@you/loop` returns without an infinite wait when no timeout is named. |
| CHAT-005 | `TestPackagedPlanParallelCompletesOneACPPromptTurn` | Parallel packaged children complete and return the expected result. |
| CHAT-006 | `TestFactoryBuilderGreetsOnAVagueFirstACPTurn` | A vague first turn returns the Factory Builder greeting. |
| CHAT-007 | `TestPackagedJavaScriptFactoryCompletesOneACPPromptTurn` | Packaged JavaScript Factory text streams through ACP. |
| CHAT-008 | `TestPackagedJavaScriptFactoryWithStructuredResultStreamsItsResult` | Structured JavaScript output renders through the ACP result contract. |
| CHAT-009 | `TestACPPromptDelegationStartsOneFactorySessionAndReusesItForLaterTurns` | First target activation creates one Factory Session and later turn reuses it. |
| CHAT-010 | `TestACPPromptDelegationFailedFactoryInvocationReportsAnACPError`; `TestACPPromptDelegationUnresolvableFactoryTargetFailsSafelyAndTerminalizes` | Provider and resolver failures return bounded ACP errors and terminalize safely. |
| CHAT-011 | `TestACPPromptDelegationRedeliveredRequestMakesNoSecondFactoryDispatch`; `TestACPPromptDelegationConcurrentPromptRejectsAsBusyWithNoFactoryDispatch` | Redelivery does not dispatch twice and concurrent prompt returns busy without cross-dispatch. |
| CHAT-012 | `TestACPServerReachesCanonicalChatSessionsAuthorityThroughRootBuildProcess` | Two ACP connections receive unique Chat Session identities and the default target. |
| CHAT-013 | `TestACPServerWithNoAuthoredAgentProfileOffersEveryInstalledFactory` | An absent profile lists every installed Factory with Factory Builder current. |
| CHAT-014 | `TestACPServerAuthoredAllowedTargetsStillRestrictsCatalog`; `TestACPServerAuthoredAllowedTargetsAreOfferedInAuthoredOrder` | Authored allowlists restrict and preserve authored target order. |
| CHAT-015 | `TestACPServerSetConfigOptionSelectsAnotherInstalledFactory` | `session/set_config_option` selects another installed Factory. |
| CHAT-016 | `TestACPServerUnrestrictedTargetsAreOfferedCurrentFirstThenSorted` | Unrestricted target listing puts current first and sorts the rest. |
| CHAT-017 | `TestACPServeCommandStreamsThroughRootBuildProcessWithoutDuplicateFinalText` | Real ACP command updates arrive once and final text is not duplicated. |
| CHAT-018 | `TestACPServeCommandStreamsUsageUpdateThroughRootBuildProcess` | Provider usage update reaches the ACP client with expected values. |
| CHAT-019 | `TestACPStreamUsagePeerProcess` | Usage peer exchanges its ACP frames and exits cleanly. |
| CHAT-020 | `TestOneACPWorkerDeliversEveryUpdateAsChildContent` | Every Worker update is child content with one Worker identity, not top-level assistant output. |
| CHAT-021 | `TestTwoACPWorkersKeepChildStreamsAttributed` | Concurrent Worker child streams remain attributed to the correct Worker. |
| CHAT-022 | `TestACPWorkerChildStreamSurvivesRetainedReplay` | Retained replay preserves ordered child content exactly once. |
| CHAT-023 | `TestACPWorkerChildPeerProcess` | Worker peer emits expected child updates and terminal result before clean exit. |
| CHAT-024 | `TestJavaScriptFactoryChildrenAreVisibleAsWorkers` | JavaScript-created children appear as Workers with correct attribution. |
| CHAT-025 | Package `TestMain` cleanup census emitted by `acp_server_composition_test.go` | Cohort, session, turn, call, peer, pipe, path, stream, and process cleanup balances with zero violations. |

### EXEC-001 through EXEC-015

| Case | Current passing witness | Observable property retained |
| --- | --- | --- |
| EXEC-001 | `TestAPIPetriDispatchUsageReachesDispatchList` / `provider_token_metadata_is_exposed` | Dispatch list exposes exact input, output, and total token usage. |
| EXEC-002 | `TestAPIPetriDispatchUsageReachesDispatchList` / `missing_provider_token_metadata_stays_absent` | Missing provider token metadata remains absent rather than zero-filled. |
| EXEC-003 | `TestWithServerDrainCannotReportSuccessWhileWorkIsNonTerminal` / `server`, `site` | Non-terminal Work causes finite hosted drain to fail only after listener/runtime join, with no false success. |
| EXEC-004 | `TestHostedFiniteRunsKeepEmptyAndTerminalSuccess` / `empty/server`, `empty/site`, `terminal_work/server`, `terminal_work/site` | Empty and terminal Work succeed in both modes and site opens its browser edge once. |
| EXEC-005 | `TestHostedContinuousRunsStayLiveWhileIdle` / `server`, `site` | Continuous server and site modes stay live while idle and join after deterministic cancellation. |
| EXEC-006 | `TestAPIResultAndResultsExposeTerminalInvocationData` / `successfulInvocationExposesPrimaryResultOnInvocationAndWorkReads` | Completed invocation and Work reads expose the exact primary result. |
| EXEC-007 | `TestAPIResultAndResultsExposeTerminalInvocationData` / `unresolvedPrimaryResultReturnsFailedTerminalStatus` | Unresolved explicit output is failed and has no fabricated primary result. |
| EXEC-008 | `TestAPIResultAndResultsExposeTerminalInvocationData` / `timeoutReturnsTimedOutTerminalStatus` | Blocking invocation timeout is `TIMED_OUT`, result is absent, and runner cancellation is observed. |
| EXEC-009 | `TestAPIResultAndResultsExposeTerminalInvocationData` / `rejectsWhitespaceOnlyTextWithTypedPublicError`, `rejectsArgsWithoutActiveSignatureWithTypedPublicError`, `rejectsInvalidStructuredArgValueShapeWithTypedPublicError` | Text, signature, and structured-argument validation retains typed public errors without Work. |
| EXEC-010 | `TestAPIResultAndResultsExposeTerminalInvocationData` / `canceledRequestContextStopsInFlightInvocation` | Canceled request returns cancellation and stops in-flight execution without false success. |
| EXEC-011 | `TestAPIResultAndResultsExposeTerminalInvocationData` / `durableResultsReadExposesFinalPrimaryResult` | Durable later result read exposes the final primary result. |
| EXEC-012 | `TestAPIDispatchListAndDetailExposePublicCorrelation` | Dispatch list/detail, Work, Worker, and session public correlation agrees. |
| EXEC-013 | `TestAPIPartialResultIsAvailableBeforeTerminalCompletion` | Partial result is visible while running, then terminal completion remains correct. |
| EXEC-014 | `TestCLIInvocationIsVisibleThroughAPISessionAndWorkReads` | In-flight and terminal CLI invocation facts become visible through public session/Work reads. |
| EXEC-015 | `TestAPIInvocationResultMatchesCLICompatibleFacts` | Equivalent API and CLI invocation status, Work, result, and correlation facts agree. |

## Story 002 ROOT optimization result

Story: `fto-c14-pkg-sessions-cluster-002`
Status: **PASS — ROOT-only optimization complete; CHAT and EXEC remain owned by
stories 003 and 004.**

The optimized subtree removes eight compatible root-fixture constructions:

- default and explicit recording cases now share one immutable process;
- the four `Process.Execute` opening cases share one process and fresh scenario
  inputs;
- the direct JavaScript startup-failure witness is a subtest of the existing
  reusable-process fixture;
- secret-step, automation replay/resume, and seeded replay subcases each reuse
  one process while retaining per-case homes, Factory directories, recording
  payloads, and API listeners.

Independent lifecycle, failure, cancellation, replay-isolation, and cleanup
boundaries remain separate where their assertions require them. Independent
top-level ROOT fixtures run in parallel behind a package-local eight-slot
semaphore; the bound prevents Windows packaged-installation/listener startup
contention from becoming a false readiness failure. No new sleep or timeout
padding was added.

### Final ROOT package evidence

The exact command was run sequentially three times with a PowerShell
`Diagnostics.Stopwatch`; every sample exited `0`:

```text
go test ./tests/functional/sessions/root_composition/... -count=1
```

| Sample 1 | Sample 2 | Sample 3 | Median | Baseline median | Improvement |
| ---: | ---: | ---: | ---: | ---: | ---: |
| 17.797s | 21.319s | 20.566s | **20.566s** | 39.288s | **47.6%** |

All three broad final gates passed; their package results were
`root_composition=14.620s`, `17.794s`, and `17.312s`, with nested
`runtime_api_fixture=0.073s`, `0.062s`, and `0.057s`. Earlier
uncapped/high-contention scheduling produced readiness failures; those attempts
were not converted into passes. The final bounded schedule passed, while an
unrelated long-running local `you` process remained on the shared host, so the
numbers are directional same-host evidence rather than an absolute latency
claim.

Changed-witness repeatability passed with exit `0`:

```text
go test ./tests/functional/sessions/root_composition -run '^(TestAutomationWorkWithoutRecordedOccupancyRestoresThroughRecordingProjection|TestRecordingFormatsRemainObservableThroughReusableRootProcess|TestProcessExecuteRuntimeOpeningThroughReusableRootProcess|TestRootBuildProcessIsInertAndReusableAcrossFactorySessions|TestRecordedFactoryRedactsSecretStepsAcrossLifecycle|TestSeededReplayResumeMaterializesRecordedWorkOnceThroughAssembledSession)$' -count=10
```

The repeat command passed with package elapsed `146.066s`.

The same selector set passed supported race execution with exit `0`:

```text
go test -race ./tests/functional/sessions/root_composition -run '^(TestAutomationWorkWithoutRecordedOccupancyRestoresThroughRecordingProjection|TestRecordingFormatsRemainObservableThroughReusableRootProcess|TestProcessExecuteRuntimeOpeningThroughReusableRootProcess|TestRootBuildProcessIsInertAndReusableAcrossFactorySessions|TestRecordedFactoryRedactsSecretStepsAcrossLifecycle|TestSeededReplayResumeMaterializesRecordedWorkOnceThroughAssembledSession)$' -count=1
```

The supported race command passed with package elapsed `20.119s`.

The assertion-parity table above records the direct old-to-new selector mapping
for ROOT-001 through ROOT-029. The final full gate and changed-witness checks
observed the original success, bad-input, provider-failure, cancellation,
replay, redaction, cleanup, and runner-routing outcomes; no assertion was
deleted or weakened.

## Story 003 Chat Sessions root-composition optimization result

Story: `fto-c14-pkg-sessions-cluster-003`
Status: **PASS — Chat root-composition optimization complete; EXEC and final
cluster reconciliation remain owned by stories 004 and 005.**

The optimized Chat fixtures keep one immutable published-catalog/profile input
cohort and one fresh root process, working directory, ACP connection, and
session lifetime per activation-owning scenario. The shared profile offers all
published packaged Factories and the three custom child-event Factories once;
each scenario selects its target with the real
`session/set_config_option` request before its first prompt. Independent ACP
activations run in parallel behind a four-slot package-local semaphore because
the production packaged-installation reconciler still touches the
filesystem-backed catalog during runtime opening. The semaphore kept that
host contention bounded without changing production code or adding waits. The
re-exec'd child peer uses a command-scoped test flag, so parallel fixtures do
not mutate process-wide peer environment state.

The retained-runtime boundary remains explicit: sharing a Chat process across
completed on-demand activations was characterized and failed with
`dependency_unavailable`, so process ownership was not weakened. The shared
home contains only fixture catalog/profile inputs and runtime-managed
installation evidence; scenario working roots, runner state, process state,
ACP streams, and sessions remain private. The existing CHAT-001 through
CHAT-025 mapping above is unchanged, with no assertion deleted, skipped, or
weakened and no new sleep or timeout padding.

### Final Chat package evidence

The exact PRD command was run sequentially three times; every sample exited
`0`:

```text
go test ./tests/functional/sessions/chat_sessions/root_composition/... -count=1
```

| Sample 1 | Sample 2 | Sample 3 | Median | Baseline median | Improvement |
| ---: | ---: | ---: | ---: | ---: | ---: |
| 17.137s | 16.850s | 19.060s | **17.137s** | 55.551s | **69.2%** |

The spread is retained as same-host variance; the median remains more than
40% below the unchanged baseline. The package result for the broad pre-measure
was 55.551s. These samples were collected after the final setup refinement,
which moves the identical operator-default environment initialization into the
one-time shared fixture cohort.

The exact package gate passed with exit `0` on all three samples. The
bounded changed-witness repeat command passed with exit `0` in `96.085s`:

```text
go test ./tests/functional/sessions/chat_sessions/root_composition -run '^(TestPackagedFactoriesCompleteOneACPPromptTurn|TestPackagedPlanParallelCompletesOneACPPromptTurn|TestFactoryBuilderGreetsOnAVagueFirstACPTurn|TestPackagedJavaScriptFactoryCompletesOneACPPromptTurn|TestPackagedJavaScriptFactoryWithStructuredResultStreamsItsResult|TestOneACPWorkerDeliversEveryUpdateAsChildContent|TestTwoACPWorkersKeepChildStreamsAttributed|TestACPWorkerChildStreamSurvivesRetainedReplay|TestJavaScriptFactoryChildrenAreVisibleAsWorkers|TestACPPromptDelegationStartsOneFactorySessionAndReusesItForLaterTurns|TestACPPromptDelegationFailedFactoryInvocationReportsAnACPError|TestACPPromptDelegationRedeliveredRequestMakesNoSecondFactoryDispatch|TestACPPromptDelegationConcurrentPromptRejectsAsBusyWithNoFactoryDispatch)$' -count=10
```

The same changed-witness selector set passed supported race execution with
exit `0` in `35.943s`:

```text
go test -race ./tests/functional/sessions/chat_sessions/root_composition -run '^(TestPackagedFactoriesCompleteOneACPPromptTurn|TestPackagedPlanParallelCompletesOneACPPromptTurn|TestFactoryBuilderGreetsOnAVagueFirstACPTurn|TestPackagedJavaScriptFactoryCompletesOneACPPromptTurn|TestPackagedJavaScriptFactoryWithStructuredResultStreamsItsResult|TestOneACPWorkerDeliversEveryUpdateAsChildContent|TestTwoACPWorkersKeepChildStreamsAttributed|TestACPWorkerChildStreamSurvivesRetainedReplay|TestJavaScriptFactoryChildrenAreVisibleAsWorkers|TestACPPromptDelegationStartsOneFactorySessionAndReusesItForLaterTurns|TestACPPromptDelegationFailedFactoryInvocationReportsAnACPError|TestACPPromptDelegationRedeliveredRequestMakesNoSecondFactoryDispatch|TestACPPromptDelegationConcurrentPromptRejectsAsBusyWithNoFactoryDispatch)$' -count=1
```

The final broad gate's cleanup census balanced `24/24` processes, `58/58`
connections, `58/58` response streams, `56/56` pipes, `29/29` sessions,
`30/30` turns, `34/34` calls, `4/4` peers, and `91/91` paths, with zero
listeners and zero violations. The repeated and race runs also completed
without a census violation. Runtime logs included bounded
`packaged_installation` active-contention observations from the shared
filesystem catalog; all retries completed successfully and no test outcome or
cleanup count was lost.

No production, public contract, generated, UI, shared-support, restart, or
other Sessions package files changed. Remote ACP/provider behavior, hosted CI,
and merge remain unproven and are owned by the final handoff.

## Failure, privacy, and evidence boundaries

- All baseline and profile commands exited `0`; the planned environmentally
  failed-package branch was not triggered. No failure was suppressed or
  converted into a pass.
- Controlled provider failures, timeouts, cancellations, partial results,
  replay recovery, redaction, concurrency, and cleanup failures were exercised
  by the mapped selectors and remained observable in passing assertions.
- Existing diagnostic logs include injected failure and cancellation paths by
  design; no raw provider payload, credential, or absolute temporary path is
  retained in this ledger.
- No browser, remote ACP, remote provider, or paid validation was authorized
  or attempted. UI accessibility, keyboard, responsive, and localization
  behavior are not applicable to this test-only story.

## Evidence boundaries and handoff

This story proves the unchanged current-head baseline, selector cost, explicit
setup/wait topology, existing package-local resource observations, and a
complete ROOT/CHAT/EXEC assertion denominator before structural edits. It does
not prove optimization, post-change medians, race safety after changes, the
final clean-room cluster, hosted PR performance, terminal CI, remote provider
behavior, or merge. Those edges remain owned by stories 002–005 and
`GATE-ROOT-001`, `GATE-CHAT-001`, `GATE-EXEC-001`, `GATE-RACE-001`,
`GATE-PR-001`, and `VAL-001`.
