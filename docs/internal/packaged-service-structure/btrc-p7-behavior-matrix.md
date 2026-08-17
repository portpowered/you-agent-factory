# BTRC P7 public behavior matrix

This is the frozen P7-A baseline required by the P7 "Build first" step in
`docs/internal/packaged-service-structure/build-time-runtime-composition-plan.md`:

> Freeze a behavior matrix keyed by customer operation and observable outcome:
> activation, Work admission, direct/child dispatch, terminal result, response
> stream, replay, cancellation, and cleanup. Record the existing assertion and
> the owning package for every cell.

It exists because P7 is a proof packet, not a seam replacement. Later P7 slices
strengthen assertions; they need a written record of what was already asserted,
by whom, and where the tree had no direct guard at all, so a "strengthened"
assertion can be distinguished from a newly invented one.

**This document is not an acceptance test.** No P7 slice may cite this file, or
any scan of it, in place of a behavioral assertion. Every row points at a Go
test that runs a real operation.

## Method

- The caller set is the plan's P7 caller-by-caller migration table (seven
  rows). Each caller gets one section below; each section's rows are that
  caller's observable scenarios from the same table.
- A cell records the **existing assertion** (`path:TestName`) and the **owning
  package** that must keep it passing.
- Guards are located by word-boundary search for `func <TestName>` across
  `*_test.go`. A plan-named guard that is absent, or that lives at a different
  path than the plan states, is recorded in *Reconciliation findings* rather
  than silently substituted.
- Reconciliation base: `7018644b3`, the merge-base of this branch and
  `origin/main` (BTRC P6, #2046).

## Status vocabulary

| Status | Meaning |
| --- | --- |
| **Guarded** | A direct guard exists in the tree and runs a real operation. |
| **Closed by P7-A** | The cell had no direct guard at the named boundary; this slice added one. |
| **Closed by P7-B** | Same, closed by the P7-B slice. |
| **Closed by P7-C** | Same, closed by the P7-C slice. |
| **P7-D** | The cell is deliberately deferred to the final authored P7 slice. |

## 1. `cmd/factory/main.go:runProcess` and `root.BuildProcess`

Canonical path: `Process.Execute` plus the Initializer lifecycle.

| Operation | Observable outcome | Existing assertion | Owning package | Status |
| --- | --- | --- | --- | --- |
| Activation | One inert graph is built per invocation and reused across isolated executions | `pkg/root/root_test.go:TestBuildProcessReusesCanonicalRootsAcrossTwoIsolatedExecutions`, `tests/functional/sessions/root_composition/process_reuse_inert_test.go:TestRootBuildProcessIsInertAndReusableAcrossFactorySessions` | `pkg/root`, `pkg/wire` | Guarded |
| Activation failure | Construction failure starts no external lifecycle | `pkg/root/root_test.go:TestBuildProcessConstructionFailureDoesNotStartExternalLifecycle` | `pkg/root` | Guarded |
| Invocation success | Command output reaches the customer's own stdout | `pkg/root/root_test.go:TestProcessRoutesHelpAndExplicitCommandsToSuppliedStreams` | `pkg/initializer/application` | Guarded |
| Invocation success | The **entrypoint** returns exit 0 and prints to the real process stdout | `cmd/factory/main_lifecycle_test.go:TestRunProcessCompletesCanonicalLifecycleAndMapsExitCode` | `cmd/factory` | **Closed by P7-A** |
| Domain failure | Argument and domain failures return a diagnostic, never success | `pkg/root/root_test.go:TestProcessInvalidArgumentsReturnsDiagnostic`, `cmd/factory/main_lifecycle_test.go:TestRunProcessCompletesCanonicalLifecycleAndMapsExitCode` | `pkg/root`, `cmd/factory` | **Closed by P7-A** |
| Cancellation | A declared cancel exit code is selected per command path; an ordinary failure wins over a canceled process context | `cmd/factory/main_test.go:TestProcessExitCodePreservesDeclaredLifecycleContract` | `cmd/factory` | Guarded |
| Cleanup | `Close` is deterministic and idempotent on a built process | `pkg/root/events_lifecycle_composition_test.go:TestBuildProcessClosesDeterministicallyAndIdempotently` | `pkg/root`, `pkg/wire` | Guarded |
| Cleanup / close-error joining | After a **failed command**, `Close` still succeeds, so `errors.Join` carries exactly the command failure and never a cancellation classification | `tests/functional/sessions/root_composition/process_lifecycle_close_test.go:TestRootProcessCloseAfterFailedCommandPreservesTheCommandFailure`, `...:TestRootProcessCloseAfterSuccessfulCommandReportsNoFailure` | `pkg/root`, `pkg/initializer/lifecycle` | **Closed by P7-A** |
| Cleanup unwind (exactly once) | Start failure stops only the components that actually started, in reverse order, exactly once each; the component whose Start failed is never stopped; no later component starts; the primary never runs; every acquired resource closes exactly once in reverse order; the primary start failure is retained beside each cleanup failure | `pkg/initializer/lifecycle/manager_test.go:TestManagerUnwindsStartFailureAndJoinsCleanupErrors` | `pkg/initializer/lifecycle` | **Closed by P7-C** |
| Cleanup continuation (composed lifecycle) | The lifecycle Wire actually composes reaches Providers then Events exactly once per `Close`, and an earlier owner's teardown failure neither skips nor masks the later one | `pkg/wire/session_runtime_providers_test.go:TestProvideApplicationProcessLifecycle_ClosesProvidersAndEventsExactlyOnceAfterAFailure`, `...:TestProcessCloseContinuesThroughEveryLifecycleOwnerAfterFailure` | `pkg/wire` | **Closed by P7-C** |
| Activation failure | An already-opened Runtime is closed exactly once when the Visualization sink cannot be resolved, and the runtime adapter is never reached | `tests/functional/sessions/root_composition/application_opening_failure_test.go:TestApplicationOpeningClosesRuntimeWhenVisualizationSinkIsUnavailable` | `pkg/services/factory_sessions` | Guarded |
| Activation failure (owner tier) | Every other opening-failure branch -- adapter failure and lifecycle-planning failure -- also closes the opened runtime exactly once and preserves the primary error | `pkg/services/factory_sessions/internal/applicationopening/service_test.go:TestOpenApplicationClosesOpenedResourcesOnBindingFailure`, `...:TestOpenApplicationClosesOpenedResourcesWhenLifecyclePlanningFails` | `pkg/services/factory_sessions` | Guarded |

## 2. HTTP generated server and SSE response-event routes

Canonical path: Wire-built owner handlers, Sessions ephemeral response events,
Recordings canonical reads.

| Operation | Observable outcome | Existing assertion | Owning package | Status |
| --- | --- | --- | --- | --- |
| Content negotiation | Documented request/response content types; 415 on an unsupported type | `tests/functional/transport/http/server/content_negotiation_test.go:TestAPIJSONRequestsAndResponsesUseDocumentedContentType`, `...:TestAPIUnsupportedContentTypeReturns415` | `pkg/transports/http` | Guarded |
| Malformed input | Malformed JSON returns a structured 400; unknown route returns structured 404; wrong method returns the documented method error | `tests/functional/transport/http/server/content_negotiation_test.go:TestAPIMalformedJSONReturnsStructured400`, `tests/functional/transport/http/server/routing_test.go:TestAPIUnknownRouteReturnsStructuredNotFound`, `...:TestAPIWrongMethodReturnsDocumentedMethodError` | `pkg/transports/http` | Guarded |
| Concurrent requests | Concurrent sessions stay isolated; a canceled request does not cancel an unrelated session | `tests/functional/transport/http/server/concurrent_requests_test.go:TestAPIConcurrentSessionRequestsRemainIsolated`, `...:TestAPICancelledRequestDoesNotCancelUnrelatedSession` | `pkg/transports/http`, `pkg/services/factory_sessions` | Guarded |
| Terminal envelope | The response-event payload union covers every documented variant | `pkg/transports/http/contracttests/openapi_contract_response_events_test.go:TestOpenAPIContract_FactoryResponseEventPayloadUnionCoversAllVariants` | `pkg/transports/http` | Guarded |
| Response stream | Retained-then-live SSE ordering; documented stream close boundary | `tests/functional/events/response_events/stream_test.go:TestAPIResponseEventSSEStreamsRetainedThenLiveEvents`, `...:TestAPIResponseEventStreamClosesAtDocumentedBoundary` | `pkg/services/factory_sessions` | Guarded |
| Reconnect gap | A cursor gap emits the typed stream-gap record; an expired session returns typed Gone | `tests/functional/events/response_events/stream_test.go:TestAPIResponseEventCursorGapEmitsStreamGap`, `...:TestAPIResponseEventSessionExpiryReturnsTypedGone` | `pkg/services/factory_sessions` | Guarded |
| Reconnect resume | Reopening a live session's stream from an acknowledged cursor replays exactly the retained suffix, in order, with no duplicate of the acknowledged prefix | `tests/functional/events/response_events/concurrent_session_isolation_test.go:TestConcurrentFactorySessionResponseEventStreamsStayIsolatedAndResumeFromCursor` | `pkg/services/factory_sessions` | **Closed by P7-B** |
| Concurrent response streams | Two Factory Sessions streaming at once each decode their own typed payload unions in published order; neither stream carries the other session's identity, event ids, dispatch identity, or message text | `tests/functional/events/response_events/concurrent_session_isolation_test.go:TestConcurrentFactorySessionResponseEventStreamsStayIsolatedAndResumeFromCursor` | `pkg/services/factory_sessions` | **Closed by P7-B** |
| Active-stream close | Server shutdown closes the listener and every active stream; bind failure unwinds started lifecycle roles | `tests/functional/transport/http/server/startup_shutdown_test.go:TestAPIServerShutdownClosesListenerAndActiveStreams`, `...:TestAPIServerBindFailureUnwindsStartedLifecycleRoles` | `pkg/transports/http`, `pkg/initializer` | Guarded |
| Terminal result | Response-event stream read to a terminal run outcome | `tests/functional/events/response_events/terminal_outcomes_test.go:TestReadResponseEventStreamUntilTerminalRunOutcomes` | `pkg/services/factory_sessions` | Guarded |
| Work admission | Work admission and response stream activate after runtime lifecycle through `root.BuildProcess` | `tests/functional/sessions/root_composition/work_admission_response_stream_test.go:TestSessionsWorkAdmissionAndResponseStreamActivateThroughRootBuildProcessAfterLifecycle` | `pkg/services/factory_sessions`, `pkg/services/work` | Guarded |

## 3. MCP stdio and service tools

Canonical path: Wire-built raw tool registry and Initializer stdio intent.

| Operation | Observable outcome | Existing assertion | Owning package | Status |
| --- | --- | --- | --- | --- |
| Discovery | Initialize plus tool discovery lists the canonical Factory Session tools | `tests/functional/transport/mcp/stdio/discovery_test.go:TestMCPStdioInitializeAndToolDiscovery`, `...:TestMCPDiscoveryContainsCanonicalFactorySessionTools` | `pkg/transports/mcp` | Guarded |
| Malformed arguments | Malformed parameters return `invalid params`; an unknown tool returns a protocol error | `tests/functional/transport/mcp/protocol/errors_test.go:TestMCPMalformedParametersReturnInvalidParams`, `tests/functional/transport/mcp/stdio/discovery_test.go:TestMCPUnknownToolReturnsProtocolError` | `pkg/transports/mcp` | Guarded |
| Async / not-ready polling | Async polling observes the terminal result | `pkg/services/factory_sessions/transports/mcp/execution_test.go:TestMockClient_RuntimeService_AsyncPollingObservesTerminalResult` | `pkg/services/factory_sessions` | Guarded |
| Typed failure | A missing Factory Session returns the canonical not-found | `tests/functional/transport/mcp/protocol/errors_test.go:TestMCPMissingFactorySessionReturnsCanonicalNotFound` | `pkg/transports/mcp` | Guarded |
| Shutdown | Server shutdown closes stdio cleanly | `tests/functional/transport/mcp/protocol/errors_test.go:TestMCPServerShutdownClosesStdioCleanly` | `pkg/transports/mcp` | Guarded |
| Activation | Runtime rejects a missing home environment and an invalid project root before composing | `tests/functional/transport/mcp/stdio/discovery_test.go:TestMCPStdioRuntimeRejectsMissingHomeEnvironment`, `...:TestMCPStdioRuntimeRejectsInvalidRuntimeProjectRoot` | `pkg/transports/mcp`, `pkg/initializer` | Guarded |

## 4. ACP stdio, Chat Sessions, and CLI ACP delegation

Canonical path: Chat Sessions conversation/control context, Sessions
invocation, Providers/Settings configuration, ACP response bridge.

| Operation | Observable outcome | Existing assertion | Owning package | Status |
| --- | --- | --- | --- | --- |
| Redelivery idempotency | A redelivered prompt makes no second Factory dispatch | `tests/functional/chat_sessions/root_composition/acp_prompt_delegation_test.go:TestACPPromptDelegationRedeliveredRequestMakesNoSecondFactoryDispatch` | `pkg/services/chat_sessions` | Guarded |
| Busy rejection | A concurrent prompt is rejected as busy with no Factory dispatch | `tests/functional/chat_sessions/root_composition/acp_prompt_delegation_test.go:TestACPPromptDelegationConcurrentPromptRejectsAsBusyWithNoFactoryDispatch` | `pkg/services/chat_sessions` | Guarded |
| Cancellation | Cancel terminalizes only the captured prompt | `tests/functional/transport/acp/stdio/cli_serve_acp_controls_test.go:TestServeACP_RootBuildProcessCancelTerminalizesOnlyCapturedPrompt` | `pkg/transports/acp` | Guarded |
| Close | Close stops the captured Factory Session | `tests/functional/transport/acp/stdio/cli_serve_acp_controls_test.go:TestServeACP_RootBuildProcessCloseStopsCapturedFactorySession` | `pkg/transports/acp` | Guarded |
| Child event identity | The worker child stream survives retained replay without crossing streams | `tests/functional/chat_sessions/root_composition/acp_worker_child_events_test.go:TestACPWorkerChildStreamSurvivesRetainedReplay` | `pkg/services/chat_sessions`, `pkg/services/recordings` | Guarded |

## 5. `root.BuildStatelessWorkers` and detached agent-run

Canonical path: `workers.Service.Execute` with injected edges; no Factory
Runtime and no Factory Session are opened.

| Operation | Observable outcome | Existing assertion | Owning package | Status |
| --- | --- | --- | --- | --- |
| Direct execution | A detached script attempt is accepted with its exact output, at the **public** root boundary, and opens no Runtime or Session | `tests/functional/workers/agent/stateless_root_test.go:TestBuildStatelessWorkersExecutesDetachedAttemptThroughRoot` | `pkg/root`, `pkg/services/workers` | **Closed by P7-A** |
| Cancellation / resource release | A canceled detached attempt terminalizes as `context.Canceled`, never reaches the external command effect, and opens no Runtime or Session | `tests/functional/workers/agent/stateless_root_test.go:TestBuildStatelessWorkersReleasesDetachedAttemptOnCancellation` | `pkg/root`, `pkg/services/workers` | **Closed by P7-A** |
| Direct execution (owner tier) | The same composition proven from inside `pkg/root`, including prompt-template and model-recording contracts | `pkg/root/root_submit_family_compatibility_test.go:TestBuildStatelessWorkersExecutesDetachedAttemptThroughRoot`, `...:TestBuildStatelessWorkersExecutesProviderAttemptThroughRoot` | `pkg/root` | Guarded |
| Goal decision envelope | A detached agent run preserves the goal decision envelope | `pkg/services/workers/wire/stateless_execute_agent_test.go:TestNewServiceExecuteDetachedAgentRunPreservesGoalDecisionEnvelope` | `pkg/services/workers` | Guarded |
| Last provider turn | The last runner turn stays authoritative, including when the harness fails | `pkg/services/workers/internal/services/workstations/executor/agentrun/detached_test.go:TestExecuteDetachedKeepsLastRunnerTurnAuthoritative`, `...:TestExecuteDetachedReturnsLastRunnerTurnWithHarnessError` | `pkg/services/workers` | Guarded |
| Timeout / typed failure | A deadline reached inside the agent loop leaves `ExecuteDetached` classifiable as an agent-run timeout with the last turn intact | `pkg/services/workers/internal/services/workstations/executor/agentrun/detached_test.go:TestExecuteDetachedTimeoutSurfacesAgentRunTimeoutClass` | `pkg/services/workers` | **Closed by P7-A** |
| Resource release (production worktree) | A canceled canonical stateless Workers run releases the production worktree | `pkg/wire/session_runtime_providers_test.go:TestCanonicalStatelessWorkersReleasesProductionWorktreeAfterCancellation` | `pkg/wire` | Guarded |
| Activation ordering | Detached Workers execution happens before Runtime opening | `pkg/wire/session_runtime_providers_test.go:TestBuildStatelessWorkersExecutesBeforeRuntimeOpening` | `pkg/wire` | Guarded |

## 6. Runtime direct/child dispatch and Worker Sessions

Canonical path: Runtime active-attempt contract, Workers `ExecuteResult`,
Worker Sessions identity.

| Operation | Observable outcome | Existing assertion | Owning package | Status |
| --- | --- | --- | --- | --- |
| Child dispatch | Concurrent accept of a scheduler-originated dispatch result resolves exactly once | `pkg/services/factory_runtime/internal/services/orchestration/runtime/dispatch_worker_sessions_idempotency_test.go:TestFactoryImpl_ConcurrentAcceptDispatchResultResolvesExactlyOnce` | `pkg/services/factory_runtime` | Guarded |
| Direct vs child dispatch | Both preserve an identical terminal-outcome mapping | `pkg/services/factory_runtime/internal/services/orchestration/runtime/dispatch_worker_sessions_terminal_semantics_test.go:TestFactoryImpl_DirectAndChildDispatchPreserveIdenticalTerminalOutcomeMapping` | `pkg/services/factory_runtime` | Guarded |
| Direct vs child dispatch, duplicate completion | Duplicate terminal delivery for one in-flight dispatch resolves to exactly one RETIRED acceptance with every other concurrent caller DUPLICATE_IDEMPOTENT, records one Worker Session association and no duplicate canonical response, leaves nothing in flight, and maps to the identical terminal outcome from **both** origins | `pkg/services/factory_runtime/internal/services/orchestration/runtime/dispatch_workers_root_boundary_test.go:TestFactoryImpl_ConcurrentDuplicateCompletionResolvesExactlyOnceForDirectAndChildDispatch` | `pkg/services/factory_runtime` | **Closed by P7-C** |
| Completion vs acceptance race | A Worker Session terminal callback racing an explicit Runtime-root acceptance retires exactly once, with the loser DUPLICATE_IDEMPOTENT and one recorded association and response | `pkg/services/factory_runtime/internal/services/orchestration/runtime/dispatch_worker_sessions_idempotency_test.go:TestFactoryImpl_WorkerSessionCompletionRacesExplicitAcceptanceAndCanonicalIdempotency` | `pkg/services/factory_runtime` | Guarded |
| Response observation | An observed in-flight response is excluded from the in-flight projection and does not terminate early | `pkg/services/factory_runtime/internal/rootobservation/project_test.go:TestProject_ExcludesObservedDispatchResponseFromInFlightProjection`, `.../terminationtests/termination_test.go:TestTerminationCheck_DoesNotTerminateWhileObservedResponseAwaitsRetirement` | `pkg/services/factory_runtime` | Guarded |
| Worker Sessions identity | A dispatch is control-addressable before start and records its association before Workers handoff | `pkg/services/factory_runtime/internal/services/orchestration/runtime/dispatch_worker_sessions_cutover_test.go:TestStartThroughWorkerSessions_AssociationIsControlAddressableBeforeStart`, `...:TestFactoryImpl_PlanDispatchRecordsWorkerSessionAssociationBeforeWorkersHandoff` | `pkg/services/factory_runtime`, `pkg/services/worker_sessions` | Guarded |
| Replay | Historical attempts list chronologically and the terminal stream replays from an atomic snapshot | `pkg/services/factory_runtime/internal/services/orchestration/runtime/dispatch_worker_sessions_cutover_test.go:TestRecordedWorkerSessionObservation_ListsHistoricalAttemptsInChronologicalOrder`, `...:TestRecordedWorkerSessionObservationStreamUsesAtomicSnapshotAndPreservesHistory` | `pkg/services/factory_runtime` | Guarded |

## 7. Work and Recordings public reads

Canonical path: Work owns content policy; Recordings owns
history/projection/replay.

| Operation | Observable outcome | Existing assertion | Owning package | Status |
| --- | --- | --- | --- | --- |
| Replay equivalence | Projection queries are equivalent for retained and replayed canonical facts | `pkg/services/recordings/internal/projection_query_contract_test.go:TestProjectionQueries_AreEquivalentForRetainedAndReplayedCanonicalFacts` | `pkg/services/recordings` | Guarded |
| Scope isolation | Recording-scope queries stay isolated across concurrent scopes and concurrent sessions | `pkg/services/recordings/internal/projection_query_contract_test.go:TestRecordingScopeQueriesRemainIsolatedAcrossConcurrentScopes`, `pkg/services/recordings/internal/canonical_recording_lifecycle_test.go:TestRecordingScopesKeepConcurrentSessionsIsolated` | `pkg/services/recordings` | Guarded |
| Replay equivalence under concurrent access | Several concurrent canonical replays of two distinct finalized scopes each complete with exactly their own scope's retained world state, never another scope's | `pkg/services/recordings/internal/canonical_recording_lifecycle_test.go:TestRecordingScopeReplayIsEquivalentAndIsolatedUnderConcurrentAccess` | `pkg/services/recordings` | **Closed by P7-C** |
| Multi-part Work content | A terminal result with two differently typed ordered parts (text then JSON) keeps both discriminated types and their order through the public boundary, and a failure terminal outcome is never reported as success | `tests/functional/transport/http/server/work_terminal_response_test.go:TestWorkTerminalResponsePreservesOrderedTypedContentThroughPublicBoundary` | `pkg/services/work` | **Closed by P7-B** |

## 8. CLI and API invocation parity

Canonical path: shared invocation contract over the same root-built process.

| Operation | Observable outcome | Existing assertion | Owning package | Status |
| --- | --- | --- | --- | --- |
| Invocation success | Local and remote `run` succeed identically | `tests/functional/sessions/lifecycle/remote_lifecycle_test.go:TestCLILocalAndRemoteRunSuccessParityThroughRootProcess` | `pkg/services/factory_sessions`, `pkg/transports/cli` | Guarded |
| Domain failure | Local and remote `run` fail identically | `tests/functional/sessions/lifecycle/remote_lifecycle_test.go:TestCLILocalAndRemoteRunDomainFailureParityThroughRootProcess` | `pkg/services/factory_sessions`, `pkg/transports/cli` | Guarded |
| Cancellation | Local and remote `run` cancel identically, and cancellation emits no success result | `tests/functional/sessions/lifecycle/remote_lifecycle_test.go:TestCLILocalAndRemoteRunCancellationParityThroughRootProcess`, `tests/functional/transport/cli/process/context_cancellation_test.go:TestCLIContextCancellationEmitsNoSuccessResult` | `pkg/services/factory_sessions`, `pkg/transports/cli` | Guarded |
| Terminal result | An NDJSON failure ends with exactly one terminal result; success writes the primary result only to stdout | `tests/functional/transport/cli/output/ndjson_stream_test.go:TestCLINDJSONFailureEndsWithOneTerminalResult`, `tests/functional/transport/cli/process/stdout_stderr_test.go:TestCLISuccessWritesPrimaryResultOnlyToStdout` | `pkg/transports/cli` | Guarded |
| Response stream | A slow writer does not reorder response events; a writer failure cancels the invocation | `tests/functional/transport/cli/output/stream_backpressure_test.go:TestCLISlowWriterDoesNotReorderResponseEvents`, `...:TestCLIWriterFailureCancelsInvocation` | `pkg/transports/cli` | Guarded |

## Reconciliation findings

The plan's guard table was authored before P5 and P6 merged. Four named guards
did not resolve at the path the plan states. Each is recorded here rather than
silently re-pointed at a lookalike.

| Plan-named guard | Finding | Disposition |
| --- | --- | --- |
| `tests/functional/workers/agent/stateless_root_test.go:TestBuildStatelessWorkersExecutesDetachedAttemptThroughRoot` | The name existed only as an in-package test at `pkg/root/root_submit_family_compatibility_test.go:410`. No test outside `pkg/root` drove `root.BuildStatelessWorkers` as a customer would, so the "detached execution stays outside Runtime/Session opening" promise had no public guard. | **Closed by P7-A** at the named path. The `pkg/root` owner-tier test is retained; the two assert different boundaries and neither replaces the other. |
| `pkg/services/workers/internal/services/workstations/executor/agentrun/executor_test.go:TestAgentRunExecutor_TimeoutSurfacesAgentRunTimeoutClass` | Neither the file nor the test exists. The nearest coverage, `agentrun/failure_test.go:TestFailureClassForError_RawDeadlineRemainsAgentRunTimeout`, tests the class mapping as a pure function and never runs a detached agent loop. | **Closed by P7-A** as `agentrun/detached_test.go:TestExecuteDetachedTimeoutSurfacesAgentRunTimeoutClass`, which drives `ExecuteDetached` and then asserts the class, metadata, and diagnostics of the error it actually returns. No new file was added to `agentrun` (the package is at 14 Go files against a limit of 15). |
| `.../runtime/dispatch_worker_sessions_idempotency_test.go:TestFactoryImpl_WorkerSessionCompletionRacesExplicitAcceptanceAndCanonicalReplay` | Absent **under that exact name**. P7-A recorded the cell as unguarded; **P7-C corrects that**: the same file already carries `TestFactoryImpl_WorkerSessionCompletionRacesExplicitAcceptanceAndCanonicalIdempotency`, which holds a Worker Session terminal callback and an explicit Runtime-root acceptance behind one barrier and asserts exactly one RETIRED, one DUPLICATE_IDEMPOTENT, one recorded association and one recorded response. Its own doc comment defers the *canonical replay* half to the Recordings owner package, and that half had no concurrent-access guard. | **Closed by P7-C** in two halves: the Runtime race is the existing `...AndCanonicalIdempotency` guard, extended by the new direct-vs-child duplicate-completion guard; the canonical replay half is closed at its owner by `pkg/services/recordings/internal/canonical_recording_lifecycle_test.go:TestRecordingScopeReplayIsEquivalentAndIsolatedUnderConcurrentAccess`. No duplicate of the existing Runtime guard was added under the plan's name. |
| `tests/functional/transport/http/server/work_terminal_response_test.go:TestWorkTerminalResponsePreservesOrderedTypedContentThroughPublicBoundary` | Absent, and the plan already marks it *planned*. | **Closed by P7-B** at the named path. |
| `tests/functional/sessions/root_composition/p3_p7_behavior_matrix_test.go:TestP3P7CanonicalPathPreservesTerminalCleanupAndReplayIsolation` | Absent, and the plan already marks it *planned*. | **P7-D** owns this cell. |

One further gap was found by walking the caller table rather than the guard
table: **`cmd/factory/main.go:runProcess` had no direct guard of any kind.**
`cmd/factory/main_test.go` replaces `runProcess` outright to test `main`, and
tests `processExitCode` as a pure function; neither one builds, executes, or
closes a process. The composed lifecycle the entrypoint performs — build the
canonical process, execute against the real process streams, close, join the
close result into the command result, map to an exit code — was unasserted.
**Closed by P7-A** by `cmd/factory/main_lifecycle_test.go` and
`tests/functional/sessions/root_composition/process_lifecycle_close_test.go`.

## Gap closure delivered by P7-A

| Cell | New or extended guard |
| --- | --- |
| Entrypoint lifecycle: success, domain failure, exit-code mapping, stream delivery, no state carried between invocations | `cmd/factory/main_lifecycle_test.go:TestRunProcessCompletesCanonicalLifecycleAndMapsExitCode`, `...:TestRunProcessReusesNoStateAcrossSequentialInvocations` |
| Close-error joining after a failed and after a successful command | `tests/functional/sessions/root_composition/process_lifecycle_close_test.go:TestRootProcessCloseAfterFailedCommandPreservesTheCommandFailure`, `...:TestRootProcessCloseAfterSuccessfulCommandReportsNoFailure` |
| Public detached-worker caller, including cancellation release | `tests/functional/workers/agent/stateless_root_test.go:TestBuildStatelessWorkersExecutesDetachedAttemptThroughRoot`, `...:TestBuildStatelessWorkersReleasesDetachedAttemptOnCancellation` |
| Detached agent-run typed timeout | `pkg/services/workers/internal/services/workstations/executor/agentrun/detached_test.go:TestExecuteDetachedTimeoutSurfacesAgentRunTimeoutClass` |

Every guard above replaces external effects only through `edges.Edges` (script
command runner, Sessions/Runtime opening ports observed for their call counts)
or through the process's own `Args`/`Env`/`WorkingDirectory`/stream inputs. No
guard sleeps, pads a timeout, or synchronizes on wall-clock time. P7-A makes no
production change and no deletion.

## Gap closure delivered by P7-B

| Cell | New or extended guard |
| --- | --- |
| Public multi-part Work terminal response, success and failure | `tests/functional/transport/http/server/work_terminal_response_test.go:TestWorkTerminalResponsePreservesOrderedTypedContentThroughPublicBoundary` |
| Concurrent-session response-event isolation, typed ordered payloads, cursor resume | `tests/functional/events/response_events/concurrent_session_isolation_test.go:TestConcurrentFactorySessionResponseEventStreamsStayIsolatedAndResumeFromCursor` |

Two P7-B findings are recorded rather than papered over:

- **CLI/API invocation parity needed no new guard.** Section 8's success,
  domain-failure, and cancellation rows already assert local-versus-remote
  `run` parity through `support.BuildProcess`, so P7-B verified the three cells
  against the plan's wording instead of adding a duplicate parity test.
- **Ordered multi-part terminal content reaches the public read through
  customer-submitted content, not through worker output.** Worker output
  materialization produces exactly one text part
  (`work.ProposedOutputFromLegacyWorkResult`), and a recorded decision-envelope
  output Work is re-materialized under a fresh identity, which breaks
  submitted-Work lineage. The guard therefore submits the ordered typed parts
  through `POST /factory-sessions/{id}/work` and authors
  `workPropagation: {mode: PRESERVE_INPUT}`, which is the one production path
  that carries customer content onto the terminal token. The public invocation
  route is text-only by contract (`work.ResolveAPITextInputContent`), so it
  cannot carry this case. No production compatibility path was added.

## Gap closure delivered by P7-C

| Cell | New or strengthened guard |
| --- | --- |
| Activation-failure unwind is exactly once, in reverse order, and never touches a component that did not start | `pkg/initializer/lifecycle/manager_test.go:TestManagerUnwindsStartFailureAndJoinsCleanupErrors` (strengthened) |
| The lifecycle Wire composes reaches both resource-owning closers exactly once and does not short-circuit on the first failure | `pkg/wire/session_runtime_providers_test.go:TestProvideApplicationProcessLifecycle_ClosesProvidersAndEventsExactlyOnceAfterAFailure` |
| Concurrent duplicate completion resolves exactly once with identical terminal mapping for **direct** and **child** dispatch | `pkg/services/factory_runtime/internal/services/orchestration/runtime/dispatch_workers_root_boundary_test.go:TestFactoryImpl_ConcurrentDuplicateCompletionResolvesExactlyOnceForDirectAndChildDispatch` |
| Canonical replay equivalence holds under concurrent cross-scope access | `pkg/services/recordings/internal/canonical_recording_lifecycle_test.go:TestRecordingScopeReplayIsEquivalentAndIsolatedUnderConcurrentAccess` |

Three P7-C findings are recorded rather than papered over:

- **P7-A's "absent" verdict on the completion-race cell was too strong.** The
  guard exists under the suffix `...AndCanonicalIdempotency`, not
  `...AndCanonicalReplay`. P7-C corrects the reconciliation row instead of
  adding a second Runtime test under the plan's name, and closes the *replay*
  half at its actual owner, Recordings.
- **The pre-existing concurrent-acceptance guard covers only the child
  origin.** `TestFactoryImpl_ConcurrentAcceptDispatchResultResolvesExactlyOnce`
  and `...AndCanonicalIdempotency` both drive `SubmitWorkRequest` + `Run`, which
  is the scheduler-originated (child) dispatch. The Runtime-root `PlanDispatch`
  (direct) origin had no duplicate-completion race guard at all. That is the
  gap the new direct-vs-child guard closes.
- **The direct origin records no canonical workstation response when an
  external acceptance wins the race.** Probed, not assumed: the scripted ledger
  shows `RecordDispatchWorkerSessionAssociation` for both origins but
  `RecordWorkstationResponse` only for the child origin. The guard therefore
  asserts one association and *no duplicate* response, rather than asserting a
  response count that only one origin produces.

Every P7-C guard was falsified once before being trusted: appending a component
to the started set before `Start` succeeds (double-stop), returning early from
the composed `Close` on the first failure, classifying duplicate retirement as
RETIRED, and crossing the two replay scopes each produced the expected failure.

## Cells deliberately left to later slices

- **P7-D** — the cross-packet corpus test, the full `-race`/`-count>=2` rerun,
  and deletion-register closure.
