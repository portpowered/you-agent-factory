# FSCP-01 — Factory Sessions legacy behavior matrix

## Status and evidence boundary

This is the checked-in characterization artifact for
`system-convergence-fscp-01-characterize-legacy-behavior`. It records the
current Factory Sessions entrypoints, their customer-visible behavior cells,
and the exact executable witness or explicit gap that must be used before
convergence. It does not change a Go contract, runtime, transport, persisted
format, or dependency graph.

`PASS` in this document means that the current tree contains an exact named
witness whose assertion shape covers the row. Runtime execution evidence is
recorded in the owning story procedure below when available; later stories
still own their declared semantic edges.

| Item | Identity |
| --- | --- |
| Packet | `FSCP-01` |
| Current branch | `system-convergence-fscp-01-characterize-legacy-behavior` |
| Current code HEAD audited (before documentation freeze) | `f1fc6a7a69a5260cf5059f1efae239e17ae5cbf2` |
| Governing source plan | `C:/Users/andre/work/portos/infinite-you/docs/temp/projects/system-convergence/source-plan.md` — `FSCP-01 — Characterize behavior across legacy entrypoints` |
| Source-plan SHA-256 | `058BF1A1E74CBC64DFEDB89BB83F0CBC3B805F941D489BB24BD207E00371794A` |
| Reconciliation dependency | `C:/Users/andre/work/portos/infinite-you/docs/temp/projects/system-convergence/state.md` — `SC-01` current reconciliation |
| Contract changes in this packet | None |
| Runtime tests run for this packet | Stories 002–004 owner and focused Factory Sessions, Recordings, transport, session/event, and changed-package race suites; exact commands and observed outcomes are recorded below |

### Disposition vocabulary

| Disposition | Meaning in this matrix |
| --- | --- |
| `PASS` | An exact current test name and path were reconciled, and the body is the intended semantic witness for the cell. Execution evidence is recorded when its owning story runs it. |
| `FAIL` | A runtime witness was executed and observed a current mismatch with the documented behavior. No row is marked `FAIL` by this non-runtime audit. |
| `INCONCLUSIVE` | Related witnesses exist, but the current tree does not yet prove the required comparison or complete semantic edge. |
| `UNSUPPORTED` | The current public contract intentionally rejects the operation or mode; the rejection citation is recorded. This is not a missing test. |
| `UNPROVEN` | No exact semantic witness was found for the stated edge. Symbol counts, implementation presence, and source inventory do not satisfy this disposition. |
| `NOT_COMPARABLE` | The paths have different customer intent or input/output semantics, so an identity/status equivalence claim would be invalid. |

The initial documentation audit had no `FAIL` claim. Story 002 now records
its executed functional evidence below; any later runtime failure must be
recorded against the row and its owning gate rather than inferred from this
artifact.

## Contract and ownership boundary

The current aggregate `factorysessions.Service` publishes the legacy surface
in `pkg/services/factory_sessions/service_contracts.go:89-123`. Its methods
include `StartAsync`, `StartSync`, `ResumeInterruptedSession`, `GetSession`,
durable controls, `GetResult`, dispatch and artifact reads, `ReadEvents`,
`ListSessions`, `InvokeFactorySession`, named activation, live open/list/get,
live result/preflight, response events, canonical event compatibility, and
live-change operations.

The current narrow facets are views over that root, not separately constructed
authorities:

| Current facet | Exposed behavior | Characterization rule |
| --- | --- | --- |
| `DurableExecutionService` (`execution_contract.go:831-850`) | Durable async/sync start, resume, inspect, lifecycle/dispatch controls, result, dispatch, artifact, and event reads | Record the durable behavior, but do not treat the facet as a second service graph. |
| `LiveControlService` (`service_contracts.go:146-164`) | Live open/list/get, pause/resume, close | Preserve live compatibility behavior until canonical callers replace it. |
| `LiveLifecycleControlService` (`service_contracts.go:166-178`) | Live cancel and terminate | Keep distinct from close: cancel/terminate preserve inspectability; close retires the live runtime. |
| `LiveResultService` (`service_contracts.go:180-195`) | Complete and partial live result projections | Its runtime-valued aliases are a legacy surface and remain a characterization target. |
| `TargetExecutionService` (`target_execution_contract.go:9-44`) | Target start, captured-session invoke/control/close, response events, and canonical events | Characterize target behavior separately from durable execution; it is consumed by Chat Sessions/ACP. |
| `DetachedOperations` (`assembly_contract.go:123-490`) | A value-only compatibility view that forwards by `Mode` to the one bound `Service` | It is not an independent opener. Its forwarding behavior is itself a required compatibility cell. |

The intended ownership boundary from the governing plan remains:

| Customer behavior/data | Current public owner to characterize | Internal source to identify |
| --- | --- | --- |
| Start, invoke, recover, session get/list, lifecycle, result | Factory Sessions | Definitions, Work, Runtime, Recordings, Events |
| Ephemeral response events | Factory Sessions | Process-local Events/response-stream implementation |
| Named Factory resolution | Factory Definitions | Definitions catalog and resolved immutable identity |
| Canonical Factory Events, reconnect, replay | Recordings | Canonical ledger and projection service |
| Canonical artifacts/retrieval | Recordings | Recording artifact projection/retrieval |
| Customer dispatch query/control | Factory Sessions | Recording facts plus private Runtime mechanics |
| Active scheduling and worker attempt state | Private Factory Runtime / Worker Sessions | Runtime and Worker Session observations |

No row below authorizes a future owner or deletes a current entrypoint.

## Normalized input and comparison families

Comparisons use the value-level fields below. They must not compare raw
constructors, implementation pointers, Petri markings, runtime services, or
transport-specific envelopes.

| Family | Normalized input | Observable comparison | Boundary |
| --- | --- | --- | --- |
| Live open | `OpenRequest`: trimmed folder, optional `TargetRef`, `ValidateOnly`, `InitNewFactory` | Stable live session identity, target metadata, `OPENED`/validation result, and typed failure | `OpenFactorySession`; folder discovery is a separate subfamily |
| Folder open | Folder path, target selector, validation/scaffold flags passed to `OpenFactorySessionFromFolder` | Auto-selection, picker metadata, validate-only no-open behavior, scaffold/reinitialize behavior | Not a durable start |
| Durable start | `StartRequest`: request ID, normalized `Source`, args, orchestrator/policy/runtime options, wait, event consumer | Session identity, initial status, source/policy/link projections, idempotent replay/conflict | `StartAsync` and `StartSync`; wait outcome is a request value |
| Async/sync start | Same durable start tuple; hold source/args/policy/runtime constant and vary only wait policy/observation | Identity and initial status must be compared before final wait outcome | Story 002 compares the public async/sync pair; final wait outcomes remain distinct |
| Detached start | `SessionStartRequest.Mode` plus the corresponding live or durable fields | Mode-specific result and forwarding; no extra service construction | `DetachedOperations.Start` |
| Invocation | Session ID, prepared input/source kind, request ID, timeout | Work/result identity, terminal status, failure code/message, work lineage | `InvokeFactorySession` and target invocation |
| Session control | Session ID, typed operation, request ID, reason, turn ID; dispatch ID where applicable | Accepted/no-op/rejected outcome, status, links, and isolation | Live and durable controls are separate behavior families |
| Result read | Session ID, `ResultRequest.Mode` (`final`/`partial`), `IncludeArtifacts` | Result status, availability/failure, primary result, artifact references | Final, partial, failed, running/not-ready, and timeout cells |
| Response reconnect | Session ID, `AfterSequence`, optional dispatch/kind filters | Retained prefix, live continuation, gap/expiry, terminal close, and session isolation | Ephemeral Factory Response Events |
| Canonical reconnect | Session ID plus `AfterEventID`/`AfterSequence` or `FactoryEventReconnectCursor` | Ordered durable history, cursor continuation, stale/foreign cursor error, no duplicate/gap | Recordings-owned canonical events |

`OpenResult` currently carries `SessionID`, optional `Session`, `Targets`,
`InitsNewFactory`, and `FolderPath`; it has no explicit mode field. Live mode
inference is therefore a property of the calling method, not a proof that an
open result and durable start are the same representation.

## Entry points: open, start, invoke, read, and compatibility views

| ID | Legacy entrypoint / cell | Current behavior and exact witness | Disposition | Owner / later gate |
| --- | --- | --- | --- | --- |
| E-01 | `OpenFactorySession`: selected live target opens and returns identity | `pkg/services/factory_sessions/internal/sessionservice/open_test.go:TestService_OpenFactorySession_ReturnsOpenedSessionIdentity`; public lifecycle witness `tests/functional/sessions/lifecycle/crud_test.go:TestAPIOpenListGetAndCloseFactorySession` | `PASS` | FSCP01-HERMETIC-002 |
| E-02 | Live open target discovery, auto-selection, and target metadata | `pkg/services/factory_sessions/discovery_test.go:TestDiscoverTargets_ReturnsDefaultAndNamedTargets`, `TestSelectTarget_AutoSelectsSingleTarget`, `TestSelectTarget_ReturnsNilForAmbiguousFolder`; `pkg/services/factory_sessions/internal/sessionservice/open_test.go:TestService_OpenFactorySessionFromFolder_AutoOpensSingleTarget`, `TestService_OpenFactorySessionFromFolder_ReturnsTargetPickerMetadata` | `PASS` | FSCP01-HERMETIC-002 |
| E-03 | `OpenFactorySessionFromFolder`: validate-only returns targets without opening | `pkg/services/factory_sessions/internal/sessionservice/open_test.go:TestService_OpenFactorySessionFromFolder_ValidateOnlyReturnsTargetsWithoutOpening`; control-plane edge `pkg/services/factory_sessions/internal/controlplane/open_test.go:TestOpenFromFolder_ValidateOnlyNotRunnableReturnsInitNewFactoryHint` | `PASS` | FSCP01-HERMETIC-002 |
| E-04 | Folder open scaffold/reinitialize/idempotency and propagation failures | `pkg/services/factory_sessions/internal/controlplane/open_test.go:TestOpenFromFolder_InitNewFactoryScaffoldsAndOpens`, `TestOpenFromFolder_InitNewFactoryAcceptsMissingNestedFactory`, `TestOpenFromFolder_InitNewFactoryReinitializesExistingRunnableTargetIdempotently`, `TestOpenFromFolder_InitNewFactoryPropagatesResolveFolderError`, `TestOpenFromFolder_InitNewFactoryPropagatesUnrecoverableDiscoveryError`, `TestOpenFromFolder_IdempotentInitNewFactoryPropagatesSelectTargetError`, `TestOpenFromFolder_IdempotentInitNewFactoryRequiresRunnableTarget`, `TestOpenFromFolder_IdempotentInitNewFactoryPropagatesScaffoldError`, `TestOpenFromFolder_IdempotentInitNewFactoryRequiresLiveOpener`, `TestOpenFromFolder_IdempotentInitNewFactoryPropagatesOpenForTargetError` | `PASS` | FSCP01-HERMETIC-002 |
| E-05 | Live open invalid flag combination and missing live opener | `pkg/services/factory_sessions/internal/sessionservice/open_test.go:TestService_OpenFactorySession_RejectsValidateOnlyWithInitNewFactory`; `pkg/services/factory_sessions/internal/controlplane/open_test.go:TestOpenFromFolder_InitNewFactoryRequiresLiveOpenerAfterScaffold` | `PASS` | FSCP01-HERMETIC-002 |
| E-06 | `StartAsync`: durable running/success/failure/timeout projection | `pkg/services/factory_sessions/internal/execution/fixtures/runtime_execution_test.go:TestJavaScriptRuntimeService_StartAsync_RunningBeforeCompletion`, `TestJavaScriptRuntimeService_StartAsync_SimpleWorkflowCompletesWithInspectableResult`, `TestJavaScriptRuntimeService_StartAsync_Failed`, `TestJavaScriptRuntimeService_StartAsync_TimedOut`; root-shape witness `pkg/services/factory_sessions/internal/services/durable_execution/internal/service/start_test.go:TestDurableStartAsyncReturnsPublishedSuccessShape`; public pair witness `tests/functional/sessions/lifecycle/fscp01_entrypoint_matrix_test.go:TestFSCP01StartOpenIdentityStatusModeMatrix` | `PASS` | FSCP01-HERMETIC-002 |
| E-07 | `StartSync`: terminal result and bounded wait outcome | `pkg/services/factory_sessions/internal/execution/fixtures/runtime_execution_test.go:TestJavaScriptRuntimeService_StartSync_SimpleWorkflowCompletesWithPrimaryResult`, `TestJavaScriptRuntimeService_StartSync_WaitTimeoutWithoutCancelKeepsSessionRunning`; `pkg/services/factory_sessions/internal/services/durable_execution/internal/service/start_test.go:TestDurableStartSyncReturnsPublishedSuccessShapeWithSyncOutcome`; public pair witness `tests/functional/sessions/lifecycle/fscp01_entrypoint_matrix_test.go:TestFSCP01StartOpenIdentityStatusModeMatrix` | `PASS` | FSCP01-HERMETIC-002 |
| E-08 | Supported live open and durable start identity/status/mode characterization without a cross-mode equivalence claim | Public functional witness `tests/functional/sessions/lifecycle/fscp01_entrypoint_matrix_test.go:TestFSCP01StartOpenIdentityStatusModeMatrix` compares selected live open, folder auto-open, durable async, and durable sync identities/read models. It observes live `IDLE`, durable async `RUNNING`, durable sync `SUCCEEDED` with `COMPLETED`/`FINAL`; live and durable are distinct public read unions, so no representation equivalence is claimed. | `PASS` | FSCP01-HERMETIC-002 |
| E-09 | Durable start request identity, replay, conflict, and async/sync mode distinction | `pkg/services/factory_sessions/internal/services/durable_execution/internal/service/start_idempotency_test.go:TestDurableStartAsyncIdempotentReplayReturnsStableSessionIdentity`, `TestDurableStartSyncIdempotentReplayReturnsStableSessionIdentity`, `TestDurableStartRequestIDConflictOnTupleMismatch`, `TestDurableStartRequestIDConflictOnAsyncSyncModeMismatch`; `pkg/services/factory_sessions/internal/execution/fixtures/runtime_boundary_test.go:TestJavaScriptRuntimeService_Start_ConcurrentIdempotentStarts`, `TestJavaScriptRuntimeService_Start_CrossModeRequestIDConflict` | `PASS` | FSCP01-HERMETIC-002 |
| E-10 | Durable start validation and typed missing-source/bad-source errors | `pkg/services/factory_sessions/internal/services/durable_execution/internal/service/start_test.go:TestDurableStartValidationErrorsDistinguishInvalidFields`; `pkg/services/factory_sessions/internal/execution/fixtures/runtime_boundary_test.go:TestJavaScriptRuntimeService_Start_RejectsInvalidWaitAndPolicy`, `TestJavaScriptRuntimeService_TypedFailures_MissingSessionMissingSourceBadSource`; public typed-source witness `tests/functional/sessions/lifecycle/fscp01_entrypoint_matrix_test.go:TestFSCP01InvokeLifecycleAndResultOutcomeMatrix` | `PASS` | FSCP01-HERMETIC-002 |
| E-11 | `ResumeInterruptedSession`: checkpoint reconstruction and published result shape | `pkg/services/factory_sessions/internal/services/durable_execution/internal/service/resume_test.go:TestDurableResumeInterruptedSessionReturnsPublishedSuccessShape`; `pkg/services/factory_sessions/internal/execution/fixtures/runtime_restart_resume_test.go:TestJavaScriptRuntimeService_ResumeInterruptedSession_ReconstructsFromCheckpointSummary`, `TestJavaScriptRuntimeService_ResumeInterruptedSession_RehydratesCheckpointStateForControlFlow`, `TestJavaScriptRuntimeService_ResumeInterruptedSession_PreservesLiveChildOutput` | `PASS` | FSCP01-HERMETIC-002 |
| E-12 | Resume missing/corrupt/invalid/not-eligible checkpoint and cursor failures | `pkg/services/factory_sessions/internal/execution/fixtures/runtime_restart_resume_test.go:TestJavaScriptRuntimeService_ResumeInterruptedSession_MissingCheckpointReturnsTypedFailure`, `TestJavaScriptRuntimeService_ResumeInterruptedSession_CorruptedPersistenceReturnsTypedFailure`, `TestJavaScriptRuntimeService_ResumeInterruptedSession_InvalidCheckpointSummaryReturnsTypedFailure`, `TestJavaScriptRuntimeService_ResumeInterruptedSession_NonApprovedCheckpointReturnsTypedFailure`, `TestJavaScriptRuntimeService_ResumeInterruptedSession_RejectsCheckpointDispatchNotDurablyCompleted`, `TestJavaScriptRuntimeService_ResumeInterruptedSession_RejectsRegressedEventCursor`, `TestJavaScriptRuntimeService_ResumeInterruptedSession_NonInterruptedSessionReturnsTypedFailure` | `PASS` | FSCP01-HERMETIC-002 |
| E-13 | `InvokeFactorySession`: target routing, runtime reuse, unknown/closed identity | `pkg/services/factory_sessions/internal/ondemandtarget/service_test.go:TestInvokeFactorySessionRoutesByOrchestratorKind`; `pkg/services/factory_sessions/internal/ondemandtarget/target_execution_test.go:TestInvokeFactorySessionReusesTheCachedRuntime`, `TestInvokeAndCancelAfterCloseReportSessionNotFound`, `TestInvokeFactorySessionUnknownIdentityReportsSessionNotFound` | `PASS` | FSCP01-HERMETIC-002 |
| E-14 | Invocation result, work lineage, failure/cancel/timeout visibility | `tests/functional/sessions/execution/visibility_test.go:TestCLIInvocationIsVisibleThroughAPISessionAndWorkReads`, `TestAPIInvocationResultMatchesCLICompatibleFacts`; `tests/functional/sessions/lifecycle/remote_lifecycle_test.go:TestCLILocalAndRemoteRunDomainFailureParityThroughRootProcess`, `TestCLILocalAndRemoteRunCancellationParityThroughRootProcess`; public result/control witness `tests/functional/sessions/lifecycle/fscp01_entrypoint_matrix_test.go:TestFSCP01InvokeLifecycleAndResultOutcomeMatrix` | `PASS` | FSCP01-TRANSPORT-005 |
| E-15 | Named activation and runtime-opening activation through the process root | `tests/functional/sessions/root_composition/lifecycle_runtime_opening_test.go:TestSessionsLifecycleAndRuntimeOpeningActivateThroughRootBuildProcessAfterLifecycle`; `pkg/services/factory_sessions/internal/sessionservice/definition_activation_gateway_test.go:TestDefinitionActivationGatewaySerializesActivationLock`, `TestDefinitionActivationGatewayRejectsIdleViolation` | `PASS` | FSCP01-TRANSPORT-005 |
| E-16 | Durable `GetSession`: status, source, policy, progress, result summary, artifacts, links | `pkg/services/factory_sessions/internal/execution/fixtures/start_read_test.go:TestFakeService_PublishedScenarios_GetSessionReadModels`; `pkg/services/factory_sessions/internal/execution/fixtures/inspection_test.go:TestFakeService_PublishedScenarios_AsyncStartInspectionLinksAndEventPrefix`; functional partial/read coverage `tests/functional/sessions/execution/results_dispatches_test.go:TestAPIPartialResultIsAvailableBeforeTerminalCompletion` | `PASS` | FSCP01-HERMETIC-002 |
| E-17 | Live `GetFactorySession`: projection, default identity, durable-ID rejection, fallback | `pkg/services/factory_sessions/internal/controlplane/read_test.go:TestGetLiveFactorySession_ReturnsProjectedSession`, `TestDefaultSessionSelectorResolvesConsistentRuntimeIdentity`, `TestGetLiveFactorySession_RejectsDurableIDs`, `TestListLiveFactorySessions_FallsBackWhenProjectionFails`; public not-found witness `tests/functional/sessions/lifecycle/crud_test.go:TestAPIFactorySessionNotFoundUsesTypedError` | `PASS` | FSCP01-HERMETIC-002 |
| E-18 | Durable `ListSessions`: live/persisted/all scope, filtering, deduplication | `pkg/services/factory_sessions/internal/execution/fixtures/listing_test.go:TestFakeService_PublishedScenarios_ListSessionsScopedWithDedup`, `TestApplySessionListScope_LivePersistedAndAllDedup`, `TestNormalizeListSessionsRequest_DefaultsToLiveAndRejectsUnsupportedScope`; `tests/functional/sessions/lifecycle/crud_test.go:TestFactorySessionListMultipleSessions` | `PASS` | FSCP01-HERMETIC-002 |
| E-19 | Live `ListFactorySessions`: ordering and concurrent isolation | `pkg/services/factory_sessions/internal/controlplane/read_test.go:TestListLiveFactorySessions_OrdersDefaultFirst`; `tests/functional/sessions/lifecycle/crud_test.go:TestAPIMultipleFactorySessionsRemainIsolated` | `PASS` | FSCP01-HERMETIC-002 |
| E-20 | `GetFactorySessionSyncPreflight`: logical remap, stale cursor, durable-ID rejection | `pkg/services/factory_sessions/internal/controlplane/sync_preflight_test.go:TestGetLiveFactorySessionSyncPreflight_RemappedDefaultReturnsLogicalSessionRemap`, `TestGetLiveFactorySessionSyncPreflight_StaleCursorReturnsCursorStale`, `TestGetLiveFactorySessionSyncPreflight_DurableSessionReturnsNotFound`, `TestLogicalSessionKeyID_UsesFolderAndTargetIdentity` | `PASS` | FSCP01-HERMETIC-002 |
| E-21 | `DetachedOperations.Bind`: missing owner/capability and inert binding | `pkg/services/factory_sessions/types_behavior_test.go:TestDetachedOperationsBindRejectsMissingOwner`, `TestDetachedOperationsPrepareSyncIsInertAndClonesValues` | `PASS` | FSCP01-HERMETIC-002 |
| E-22 | `DetachedOperations.Start`: live, durable async, and durable sync forwarding | `pkg/services/factory_sessions/types_behavior_test.go:TestDetachedOperationsStartForwardsLiveValues`, `TestDetachedOperationsStartForwardsDurableValues`, `TestDetachedOperationsStartForwardsSynchronousValues` | `PASS` | FSCP01-HERMETIC-002 |
| E-23 | `DetachedOperations.Invoke`, `Activate`, and `Subscribe` forwarding | `pkg/services/factory_sessions/types_behavior_test.go:TestDetachedOperationsInvokeForwardsPreparedWork`, `TestDetachedOperationsActivateAndSubscribe` | `PASS` | FSCP01-TRANSPORT-005 |
| E-24 | `DetachedOperations.Get` and `List`: mode-specific owner and all-scope behavior | `pkg/services/factory_sessions/types_behavior_test.go:TestDetachedOperationsReadListUsesModeSpecificOwners`; the method implementation explicitly maps live to `GetFactorySession`/`ListFactorySessions` and durable to `GetSession`/`ListSessions` (`assembly_contract.go:272-351`) | `PASS` | FSCP01-HERMETIC-002 |
| E-25 | `DetachedOperations.Control`: live close/cancel/terminate compatibility and durable control forwarding | `pkg/services/factory_sessions/types_behavior_test.go:TestDetachedOperationsControlUsesModeSpecificOwners`; unsupported-operation branches are explicit in `assembly_contract.go:492-550` | `PASS` | FSCP01-HERMETIC-002 |
| E-26 | `DetachedOperations.ReadResult`: live complete/partial and durable mode mapping | `pkg/services/factory_sessions/types_behavior_test.go:TestDetachedOperationsReadResultMapsDetachedValues`; `assembly_contract.go:382-444` is the exact mode branch | `PASS` | FSCP01-HERMETIC-002 |
| E-27 | `DetachedOperations.PrepareSync`: durable-only, request ID, and timeout normalization | `pkg/services/factory_sessions/types_behavior_test.go:TestDetachedOperationsPrepareSyncIsInertAndClonesValues`; `assembly_contract.go:446-464` is the contract citation for durable-only rejection and wait overlay | `PASS` | FSCP01-HERMETIC-002 |
| E-28 | Live-only/durable-only method misuse | Live reads reject durable IDs (`pkg/services/factory_sessions/internal/controlplane/read_test.go:TestGetLiveFactorySession_RejectsDurableIDs`); durable control routing rejects live IDs (`pkg/services/factory_sessions/internal/controlplane/durable_lifecycle_test.go:TestPauseDurableFactorySession_RejectsLiveSessionID`, `TestApproveDurableFactorySession_RejectsLiveSessionID`). | `UNSUPPORTED` | Preserve typed mode boundary; FSCP01-HERMETIC-002 |
| E-29 | Live change apply/recover normalization, no-op, conflict, close, and replay | `pkg/services/factory_sessions/internal/livechange/service_test.go:TestNormalizeLiveChangeRequest_CanonicalizesBodyAndRejectsMalformedInput`, `TestApplyLiveChange_ExactNoOpReturnsTypedSuccessWithoutHistory`, `TestApplyLiveChange_RequestIDConflictAndChangeIDCollisionDoNotMutate`, `TestApplyLiveChange_AdmittedApplicationFailureClosesAndReplays`, `TestRecoverLiveChange_ClosesPendingRequestAfterAppendFailure` | `PASS` | FSCP01-HERMETIC-002 |
| E-30 | Durable replay-only invocation/control/response operations before handoff | `pkg/services/factory_sessions/internal/execution/recordingreplay/service_test.go:TestServiceRejectsUnknownSessionsAndLiveOperations`; response subscription before handoff is explicitly `ErrNonLiveReplay` in `recordingreplay/service.go:115-136` | `UNSUPPORTED` | Recovery handoff semantics; FSCP01-HERMETIC-002 |

## Lifecycle and dispatch controls

| ID | Control cell | Exact witness | Disposition | Owner / later gate |
| --- | --- | --- | --- | --- |
| C-01 | Durable pause accepted, repeated pause no-op, terminal control outcome, missing session | `pkg/services/factory_sessions/internal/services/durable_execution/internal/service/control_test.go:TestDurableControlPauseAcceptedOnRunningSession`, `TestDurableControlPauseNoOpWhenAlreadyPaused`, `TestDurableControlAgainstTerminalSessionReturnsTerminalOutcome`, `TestDurableControlUnknownSessionReturnsNotFound` | `PASS` | FSCP01-HERMETIC-002 |
| C-02 | Durable resume returns running and rejects invalid lifecycle state | `pkg/services/factory_sessions/internal/execution/fixtures/lifecycle_test.go:TestJavaScriptRuntimeService_ResumePausedSessionReturnsRunning`, `TestJavaScriptRuntimeService_PauseTerminalSessionReturnsTypedControlError`; routing `pkg/services/factory_sessions/internal/controlplane/durable_lifecycle_test.go:TestResumeDurableFactorySession_RoutesDurableSessionToExecution` | `PASS` | FSCP01-HERMETIC-002 |
| C-03 | Durable cancel and terminate preserve their distinct outcomes | `pkg/services/factory_sessions/internal/execution/fixtures/lifecycle_test.go:TestJavaScriptRuntimeService_CancelRunningSessionReturnsCanceling`, `TestJavaScriptRuntimeService_TerminateRunningSessionReturnsTerminated`, `TestJavaScriptRuntimeService_CancelTerminalSessionReturnsTypedControlError`; public control `tests/functional/sessions/controls/pause_resume_test.go:TestAPIPauseResumeCancelAndTerminateFactorySession` | `PASS` | FSCP01-HERMETIC-002 |
| C-04 | Durable approve at policy boundary | `pkg/services/factory_sessions/internal/execution/fixtures/lifecycle_test.go:TestFakeService_PublishedScenarios_LifecycleControlApproveAwaitingApproval`, `TestJavaScriptRuntimeService_ApproveRunningSessionReturnsTypedControlError`; routing `pkg/services/factory_sessions/internal/controlplane/durable_lifecycle_test.go:TestApproveDurableFactorySession_RoutesDurableSessionToExecution` | `PASS` | FSCP01-HERMETIC-002 |
| C-05 | Durable retry and interrupt dispatch | `pkg/services/factory_sessions/internal/execution/fixtures/lifecycle_test.go:TestFakeService_PublishedScenarios_LifecycleControlRetryDispatchPaths`, `TestJavaScriptRuntimeService_RetryDispatchMissingDispatchReturnsNotFound`; `pkg/services/factory_sessions/internal/controlplane/durable_lifecycle_test.go:TestRetryDurableFactorySessionDispatch_PreservesDistinctFromLiveLifecycle`, `TestInterruptDurableFactorySessionDispatch_RoutesDurableSessionToExecution` | `PASS` | FSCP01-DISPATCH-004 |
| C-06 | Durable control idempotency, request conflict, and isolation | `pkg/services/factory_sessions/internal/execution/fixtures/lifecycle_test.go:TestFakeService_PublishedScenarios_LifecycleControlIdempotentReplayAndConflict`, `TestFakeService_PublishedScenarios_LifecycleControlIsolationAcrossSessions`, `TestFakeService_PublishedScenarios_LifecycleControlDeterministicAcrossServiceReload` | `PASS` | FSCP01-HERMETIC-002 |
| C-07 | Live pause/resume, typed rejection, cancellation, close, and registry retirement | `pkg/services/factory_sessions/internal/sessionservice/live_runtime_lifecycle_control_test.go:TestLiveControlCapability_OpenPauseResumePreservesLifecycleResults`, `TestLiveControlCapability_PreservesTypedRejectionAndCancellation`, `TestLiveControlCapability_CompletesLifecycleAndRetiresCanonicalSession`, `TestService_LivePauseRejectsInvalidStateWithoutRegistryMutation`, `TestService_CloseFactorySessionThroughLiveRuntimeRetiresRegistryEntry`; public control `tests/functional/sessions/controls/pause_resume_test.go:TestAPIPauseResumeCancelAndTerminateFactorySession`; public pause/resume result witness `tests/functional/sessions/lifecycle/fscp01_entrypoint_matrix_test.go:TestFSCP01InvokeLifecycleAndResultOutcomeMatrix` | `PASS` | FSCP01-HERMETIC-002 |
| C-08 | Live cancel/terminate differ from close and preserve inspectability | `pkg/services/factory_sessions/service_contracts.go:166-178` defines the distinction; public witness `tests/functional/sessions/controls/pause_resume_test.go:TestAPIPauseResumeCancelAndTerminateFactorySession`; remote parity `tests/functional/sessions/lifecycle/remote_lifecycle_test.go:TestCLILocalAndRemoteRunCancellationParityThroughRootProcess` | `PASS` | FSCP01-TRANSPORT-005 |
| C-09 | Close failure retains captured target for retry; unknown/no-runtime close is no-op; all runtimes close | `pkg/services/factory_sessions/internal/ondemandtarget/target_execution_test.go:TestCloseFactorySessionTearsDownAndEvicts`, `TestCloseFactorySessionFailureRetainsCapturedTargetForRetry`, `TestCloseFactorySessionUnknownIdentityIsNoOpSuccess`, `TestCloseTearsDownEveryTrackedRuntime`, `TestCloseWithNoOpenedRuntimesIsNoOpSuccess` | `PASS` | FSCP01-HERMETIC-002 |
| C-10 | Delete requires lifecycle safety and distinguishes missing/conflict | `tests/functional/sessions/lifecycle/crud_test.go:TestFactorySessionCreateListShowDelete`, `TestFactorySessionMissingShowAndDeleteFail`; HTTP typed failure `pkg/services/factory_sessions/transports/http/root_errors_test.go:TestHandlerFromRoot_DeleteFactorySessionConflictReturnsTypedErrorResponse` | `PASS` | FSCP01-TRANSPORT-005 |
| C-11 | Recovery after restart preserves completed dispatches and history | `tests/functional/sessions/restart/logical_identity_test.go:TestFactorySessionResumeDoesNotRepeatCompletedDispatch`, `TestFactorySessionHistoryIsPersistedAcrossRestart`; `tests/functional/sessions/resume_from_recording/resume_from_recording_test.go:TestKilledFactorySessionResumesOriginalDispatchesAfterRestart`, `TestSuccessorRecordingReplaysAgainstUnchangedCheckout` | `PASS` | FSCP01-HERMETIC-002 |
| C-12 | A cancelled/terminated/closed session cannot affect an unrelated session | `tests/functional/sessions/lifecycle/crud_test.go:TestAPIMultipleFactorySessionsRemainIsolated`; `pkg/services/factory_sessions/internal/ondemandtarget/target_execution_test.go:TestCancelInterruptsOnlyTheActiveInvocation`, `TestInvokeAndCancelIsolationBetweenMultipleStartedTargets`; response-stream isolation is C-16 | `PASS` | FSCP01-HERMETIC-002 |

## Result, timeout, and response-event cells

| ID | Result/stream cell | Exact witness | Disposition | Owner / later gate |
| --- | --- | --- | --- | --- |
| R-01 | Durable final result and terminal invocation data | `tests/functional/sessions/execution/results_dispatches_test.go:TestAPIResultAndResultsExposeTerminalInvocationData`; fixture result `pkg/services/factory_sessions/internal/execution/fixtures/start_read_test.go:TestFakeService_PublishedScenarios_ResultReadsWithStableHash`; public final-result witness `tests/functional/sessions/lifecycle/fscp01_entrypoint_matrix_test.go:TestFSCP01InvokeLifecycleAndResultOutcomeMatrix` | `PASS` | FSCP01-HERMETIC-002 |
| R-02 | Durable partial result before terminal completion | `tests/functional/sessions/execution/results_dispatches_test.go:TestAPIPartialResultIsAvailableBeforeTerminalCompletion`; result projection `pkg/services/factory_sessions/internal/execution/javascript_runtime_result_test.go:TestProjectResultRead_FailedWithPartialHonorsPartialMode`, `TestProjectResultRead_NotReadyRunningSession` | `PASS` | FSCP01-HERMETIC-002 |
| R-03 | Running final result is not ready, with typed availability | `pkg/services/factory_sessions/internal/execution/fixtures/runtime_execution_test.go:TestJavaScriptRuntimeService_StartAsync_RunningBeforeCompletion`, `TestJavaScriptRuntimeService_StartSync_WaitTimeoutWithoutCancelKeepsSessionRunning`; MCP envelope `pkg/services/factory_sessions/transports/mcp/execution_test.go:TestMockClient_GetResult_RunningFixtureReturnsTypedNotReadyEnvelope` | `PASS` | FSCP01-TRANSPORT-005 |
| R-04 | Failed session preserves partial summary and failure detail | `pkg/services/factory_sessions/transports/mcp/failure_paths_test.go:TestMockClient_GetSession_FailedFixtureReturnsDeterministicStatusWithPartialSummary`, `TestMockClient_GetResult_FailedFixtureReturnsPartialResultWithFailureDetails`; recording event mapping `pkg/services/recordings/internal/events/event_history_dispatch_lifecycle_test.go:TestFailureDetailsForResult_FailedWorkerErrorUsesStableFailureDetails` | `PASS` | FSCP01-HERMETIC-002 |
| R-05 | Sync timeout without cancellation leaves session running; cancellation-on-timeout is an explicit overlay | `pkg/services/factory_sessions/internal/execution/fixtures/runtime_boundary_test.go:TestJavaScriptRuntimeService_StartSyncWaitTimeoutReplay_PreservesFirstObservedTimedOutResponse`; `pkg/services/factory_sessions/internal/execution/fake_service_construction_start_test.go:TestFakeService_StartSync_AppliesCancelOnTimeoutOverlay`; `pkg/services/factory_sessions/internal/execution/fixtures/runtime_execution_test.go:TestJavaScriptRuntimeService_StartSync_WaitTimeoutWithoutCancelKeepsSessionRunning`; public timeout witness `tests/functional/sessions/lifecycle/fscp01_entrypoint_matrix_test.go:TestFSCP01InvokeLifecycleAndResultOutcomeMatrix` | `PASS` | FSCP01-HERMETIC-002 |
| R-06 | Live complete and partial result projection | `pkg/services/factory_sessions/internal/controlplane/result_read_test.go:TestGetLiveFactorySessionResult_ReturnsJavaScriptProjection`, `TestGetLiveFactorySessionPartialResult_ReturnsJavaScriptProjection`, `TestGetLiveFactorySessionResult_RequiresInjectedProjection`; detached mapping `pkg/services/factory_sessions/types_behavior_test.go:TestDetachedOperationsReadResultMapsDetachedValues` | `PASS` | FSCP01-HERMETIC-002 |
| R-07 | Result artifact inclusion/omission and stable artifact references | `pkg/services/factory_sessions/internal/execution/fixtures/inspection_test.go:TestFakeService_PublishedScenarios_ResultArtifactInclusion`; `pkg/services/factory_sessions/internal/execution/javascript_runtime_result_test.go:TestProjectResultRead_IncludeArtifactsShaping`; result read hash `pkg/services/factory_sessions/internal/execution/fixtures/start_read_test.go:TestProjectedResultReadHash_IsStableAcrossEquivalentReads` | `PASS` | FSCP01-CANONICAL-003 |
| R-08 | Ephemeral response events replay retained prefix, then live events, and close at documented terminal boundary | `tests/functional/events/response_events/stream_test.go:TestAPIResponseEventSSEStreamsRetainedThenLiveEvents`, `TestAPIResponseEventStreamClosesAtDocumentedBoundary`; terminal outcomes `tests/functional/events/response_events/terminal_outcomes_test.go:TestReadResponseEventStreamUntilTerminalRunOutcomes` | `PASS` | FSCP01-TRANSPORT-005 |
| R-09 | Response cursor gap, expiry, filters, and typed errors | `tests/functional/events/response_events/stream_test.go:TestAPIResponseEventCursorGapEmitsStreamGap`, `TestAPIResponseEventSessionExpiryReturnsTypedGone`; `pkg/services/factory_sessions/discovery_test.go:TestResponseStreamRootContract_TypedStaleCursorGapAndCancelFailures`; handler validation `pkg/services/factory_sessions/transports/http/handlers_response_events_test.go:TestGetFactoryResponseEventsBySessionId_RejectsInvalidAfterSequence`, `TestGetFactoryResponseEventsBySessionId_RejectsInvalidKindFilter` | `PASS` | FSCP01-TRANSPORT-005 |
| R-10 | Concurrent response subscriptions remain isolated and reconnect from cursor | `tests/functional/events/response_events/concurrent_session_isolation_test.go:TestConcurrentFactorySessionResponseEventStreamsStayIsolatedAndResumeFromCursor`; runtime replacement `tests/functional/events/response_events/session_runtime_replace_test.go:TestFactoryResponseEventSequenceSurvivesSessionRuntimeReplacement` | `PASS` | FSCP01-TRANSPORT-005 |
| R-11 | Response stream and canonical event/artifact reads are independent after reconnect/close | `tests/functional/events/factory_events/fscp01_canonical_reads_test.go:TestFSCP01CanonicalReconnectAndArtifactReadsIndependentOfResponseEvents` deliberately makes no `response-events` subscription, then reads canonical history, reconnects from event ID and sequence cursors, and reads artifacts. An explicit open-then-close response subscription remains outside this witness. | `PASS` | FSCP01-CANONICAL-003; explicit close-after-subscription remains a later edge |
| R-12 | Response stream for a recording-only session before resume handoff | `pkg/services/factory_sessions/internal/execution/recordingreplay/service_test.go:TestServiceRejectsUnknownSessionsAndLiveOperations`; `recordingreplay/service.go:115-136` returns `ErrNonLiveReplay` before handoff | `UNSUPPORTED` | Story 002 recovery characterization |

## Canonical events, artifacts, and replay

These rows deliberately separate canonical Recording reads from ephemeral
Factory Response Events. A response stream is not a substitute for canonical
history or artifact retrieval.

| ID | Canonical/artifact cell | Exact witness or contract citation | Disposition | Owner / later gate |
| --- | --- | --- | --- | --- |
| K-01 | Durable `ReadEvents` returns ordered canonical fixture events and honors reconnect cursor | `pkg/services/factory_sessions/internal/execution/fixtures/inspection_test.go:TestFakeService_PublishedScenarios_ReadEventsCanonicalAndReconnect`, `TestFakeService_PublishedScenarios_ReadEventsMissingCursorReturnsTypedError`; `pkg/services/factory_sessions/internal/execution/fake_service_lifecycle_inspection_test.go:TestFakeService_ReadEvents_ReturnsCanonicalFixtureEventsAndHonorsCursor` | `PASS` | FSCP01-CANONICAL-003 |
| K-02 | Public canonical event history is ordered, cursor reads only newer events, and invalid cursor is typed | `tests/functional/events/factory_events/order_and_cursor_test.go:TestAPIGetFactoryEventsReturnsOrderedDurableHistory`, `TestAPIEventCursorReturnsOnlyNewerEvents`, `TestAPIInvalidEventCursorReturnsTypedError` | `PASS` | FSCP01-CANONICAL-003 |
| K-03 | Canonical event stream closes at termination and reconnect has no gap/duplicate | `tests/functional/events/factory_events/order_and_cursor_test.go:TestFactoryEventStreamIsOrderedAndClosesAtSessionTermination`, `TestFactoryEventStreamReconnectHasNoGapOrDuplicate`; Recordings root cursor `pkg/services/recordings/wire/fold_behavior_preservation_test.go:TestWireFoldPreservesSubscriptionCursorOrderThroughPublishedRoot` | `PASS` | FSCP01-CANONICAL-003 |
| K-04 | Live target canonical subscription/probe owns and cancels its subscription | PR #2531 citation-only witness `pkg/services/factory_sessions/internal/sessionservice/lifecycle_test.go:TestService_ProbeFactoryEventsForSession_CancelsOwnedSubscription`; no reserved file is changed in this packet | `PASS` | FSCP01-CANONICAL-003; PR #2531 owns the file |
| K-05 | Canonical topology snapshot preserves public identity and resource evidence | `tests/functional/events/factory_events/order_and_cursor_test.go:TestCanonicalTopologySnapshotsPreservePublicIdentityAndResourceEvidence`; Recording event lifecycle `pkg/services/recordings/internal/events/event_history_session_lifecycle_test.go:TestFactoryEventHistory_RecordSessionLifecycle_EmitsReconstructableBracketSequence` | `PASS` | FSCP01-CANONICAL-003 |
| K-06 | Recording scopes remain ordered, replay-equivalent, and isolated under concurrent sessions | `pkg/services/recordings/internal/canonical_recording_lifecycle_test.go:TestRecordingScopesKeepConcurrentSessionsIsolated`, `TestRecordingScopeReplayIsEquivalentAndIsolatedUnderConcurrentAccess`; root replay `pkg/services/recordings/service_root_contract_replay_test.go:TestNeutralReplayRootContract_EquivalentReductionAndProgress`, `TestNeutralReplayRootContract_DivergenceAndTypedFailures` | `PASS` | FSCP01-CANONICAL-003 |
| K-07 | Canonical dispatch lifecycle records queue/interrupt/reconcile/artifact sequence | `pkg/services/recordings/internal/events/event_history_dispatch_lifecycle_test.go:TestFactoryEventHistory_RecordDispatchLifecycle_EmitsReconstructableQueueInterruptReconcileAndArtifactSequence`; association `pkg/services/recordings/internal/events/event_history_dispatch_worker_session_association_test.go:TestFactoryEventHistory_RecordDispatchWorkerSessionAssociation_RecordsCanonicalAssociation` | `PASS` | FSCP01-DISPATCH-004 |
| K-08 | `ListArtifacts`/`GetArtifact` return stable summaries/detail from the durable owner | `pkg/services/factory_sessions/internal/execution/fixtures/inspection_test.go:TestFakeService_PublishedScenarios_ListArtifactsStableSummaries`, `TestFakeService_PublishedScenarios_GetArtifactDetailAndUnknownError`; `pkg/services/factory_sessions/internal/execution/recordingreplay/service_test.go:TestServiceExposesRecordedArtifactsAndEmptyDispatches` | `PASS` | FSCP01-CANONICAL-003 |
| K-09 | Recording root artifact/replay export and typed validation failures | `pkg/services/recordings/replay_artifact_capability_test.go:TestRecordingReplayArtifacts_UnchangedBehaviorThroughComposedImplementation`, `TestRecordingReplayArtifacts_TypedFailures`, `TestRecordingReplayArtifacts_UnsupportedSchemaVersion`, `TestRecordingReplayArtifacts_InvalidOrder`, `TestRecordingReplayArtifacts_InvalidIntegrity`, `TestRecordingReplayArtifacts_MalformedDecode`, `TestRecordingReplayArtifacts_ExportFailureLeavesNoPartialArtifactAndRetries`; root contract `pkg/services/recordings/wire/fold_behavior_preservation_test.go:TestWireFoldPreservesPortableArtifactExportThroughPublishedRoot` | `PASS` | FSCP01-CANONICAL-003 |
| K-10 | Independent canonical artifact retrieval after response-stream closure | `tests/functional/events/factory_events/fscp01_canonical_reads_test.go:TestFSCP01CanonicalReconnectAndArtifactReadsIndependentOfResponseEvents` reads stable artifact list/detail data, safe retrieval refs, and typed missing/corrupt/foreign-session failures without subscribing to response events. | `PASS` | FSCP01-CANONICAL-003; explicit close-after-subscription remains a later edge |
| K-11 | Historical recording dispatch list/detail before durable execution handoff | `pkg/services/factory_sessions/internal/execution/recordingreplay/service_test.go:TestServiceHistoricalDispatchQueriesRemainEmptyForEveryFilter`; `recordingreplay/service.go:222-255` returns empty/not-found before handoff | `UNSUPPORTED` | Preserve existing replay behavior until a later canonical dispatch projection is specified |
| K-12 | Canonical event/artifact reads through every CLI/ACP/MCP transport | MCP canonical read witness `pkg/services/factory_sessions/transports/mcp/inspection_test.go:TestMockClient_ReadEvents_EventReconnectFixtureReturnsOrderedCanonicalEvents`; ACP child stream witness `tests/functional/sessions/chat_sessions/root_composition/acp_worker_child_events_test.go:TestACPWorkerChildStreamSurvivesRetainedReplay`; no separate CLI artifact endpoint is exposed by the current contract | `NOT_COMPARABLE` | Transport gate FSCP01-TRANSPORT-005; do not invent unsupported transport operations |

## Dispatch projection and field provenance

The current durable execution implementation takes an active snapshot in
`pkg/services/factory_sessions/internal/execution/runtime_service.go:556-614`.
`dispatchesForRead` then annotates lifecycle-event cursors and applies a
Recording completed-flush watermark in
`pkg/services/factory_sessions/internal/execution/listing.go:886-945`.
The restart/replay implementation instead returns `s.projection` until a
recording session is handed off, as shown by
`pkg/services/factory_sessions/internal/execution/recordingreplay/service.go:139-295`.
The table records that branch explicitly; it does not choose the future
FSCP-06 projection rule.

| Field/group returned by `DispatchSummary` / `DispatchDetail` | Active read source now | Terminal/replay read source now | Witness and current limit |
| --- | --- | --- | --- |
| `ID`, `Status`, `Phase`, `Label`, `Attempt`, `Retryable`, `FailureClassification`, `FailureDetail` | Runtime session snapshot; JavaScript child-record mapping at `internal/execution/service.go:595-633` | Recording projection for replay-only sessions; handed-off sessions delegate back to runtime owner | `pkg/services/factory_sessions/internal/execution/fixtures/inspection_test.go:TestFakeService_PublishedScenarios_ListDispatchesStableSummaries`, `TestFakeService_PublishedScenarios_GetDispatchDetailAndUnknownError`; public active/terminal list/detail and typed missing/foreign dispatch witness `tests/functional/sessions/execution/fscp01_dispatch_provenance_test.go:TestFSCP01DispatchReadFieldProvenanceMatrix`; replay parity remains unproven |
| `RunnerID`, `PresetID`, `ModelProvider`, `Model`, `ReasoningEffort`, `Provider` | Runtime child dispatch record (`service.go:595-626`) | Canonical event/projection facts when present; absent fields remain absent rather than being back-filled from an unrelated worker | `pkg/services/factory_sessions/internal/execution/fixtures/inspection_test.go:TestFakeService_PublishedScenarios_DispatchListIncludesProviderSessionRefs`; Recording association `pkg/services/recordings/internal/events/event_history_dispatch_worker_session_association_test.go:TestFactoryEventHistory_RecordDispatchWorkerSessionAssociationWithExecution_RetainsReplayFactsWithoutWideningPublicPayload`; public field labels and observed active/terminal values `tests/functional/sessions/execution/fscp01_dispatch_provenance_test.go:TestFSCP01DispatchReadFieldProvenanceMatrix` |
| `ProviderSessionRefs` | Runtime child record's provider-session reference (`service.go:618-625`) | Recording dispatch association/execution facts if recorded | Exact presence is covered by `TestFakeService_PublishedScenarios_DispatchListIncludesProviderSessionRefs`; public active/terminal field labeling and list/detail parity `tests/functional/sessions/execution/fscp01_dispatch_provenance_test.go:TestFSCP01DispatchReadFieldProvenanceMatrix`; source parity across all terminal paths remains `UNPROVEN` |
| `OutputArtifactIDs` and `ArtifactIDs` | Runtime child artifact reference for completed dispatch plus detail copy (`service.go:627-630`, `runtime_service.go:597-599`) | Recording canonical dispatch/result artifact IDs | `pkg/services/factory_sessions/internal/execution/fake_service_lifecycle_inspection_test.go:TestFakeService_ReadProjections_MatchFixtureDispatchesArtifactsEvents`; public terminal artifact field label `tests/functional/sessions/execution/fscp01_dispatch_provenance_test.go:TestFSCP01DispatchReadFieldProvenanceMatrix`; independent artifact retrieval is K-10 |
| `Usage`, `Warnings` | Runtime/execution projection when populated | Recording projection when the corresponding canonical facts were retained | `tests/functional/sessions/execution/dispatch_usage_test.go:TestAPIPetriDispatchUsageReachesDispatchList`; public active/terminal field labeling `tests/functional/sessions/execution/fscp01_dispatch_provenance_test.go:TestFSCP01DispatchReadFieldProvenanceMatrix`; full replay parity is `INCONCLUSIVE` |
| `ConfirmationState` | Derived at read time from lifecycle event cursor plus Recording completed-flush watermark; defaults `UNCONFIRMED` | Recording-backed read is the durability authority | `pkg/services/factory_sessions/internal/execution/fake_service_dispatch_test.go:TestFakeServiceDispatchListAndDetailDefaultToUnconfirmedTogether`, `TestLiveDispatchListAndDetailConfirmAfterCompletedFlush`; public active/terminal read labels `tests/functional/sessions/execution/fscp01_dispatch_provenance_test.go:TestFSCP01DispatchReadFieldProvenanceMatrix`; this is a read-boundary fact, not dispatch ownership |
| `StateSequence`, `StateSequenceKnown`, `StreamGenerationID` | Derived from dispatch lifecycle events by `annotateDispatchStateCursors` (`listing.go:947-980`) | Canonical cursor/projection when the event is retained | `pkg/services/factory_sessions/internal/execution/fake_service_lifecycle_inspection_test.go:TestDispatchReadPreservesLatestLifecycleCursorAndDefaultsUnconfirmed`; field-by-field terminal persistence remains `UNPROVEN` |
| `StatusTransitions` | Runtime state's `dispatchStatusTransitions` map (`runtime_service.go:600-602`) | Recording replay projection if transitions are retained | `pkg/services/factory_sessions/internal/execution/fake_service_dispatch_test.go:TestReplayDispatchProjection_DerivesInterruptedDispatchMetadata`; public active/terminal field labels `tests/functional/sessions/execution/fscp01_dispatch_provenance_test.go:TestFSCP01DispatchReadFieldProvenanceMatrix`; cross-owner parity remains FSCP01-DISPATCH-004 |
| `DispatchDetail.SessionID`, `OrchestratorKind` | Runtime session snapshot (`runtime_service.go:592-596`) | Recording session projection/handoff owner | `tests/functional/sessions/execution/results_dispatches_test.go:TestAPIDispatchListAndDetailExposePublicCorrelation`, `tests/functional/sessions/execution/fscp01_dispatch_provenance_test.go:TestFSCP01DispatchReadFieldProvenanceMatrix`; the tests prove public correlation, not complete replay source parity |
| `DispatchDetail.Petri` | Private Runtime Petri projection when populated | No independent canonical public field is established by the current replay contract | No exact field-level public witness found; `UNPROVEN`, owner FSCP01-DISPATCH-004 |
| `DispatchDetail.JavaScript` / `DispatchSummary.JavaScript` | Runtime child record and runtime dispatch projection (`service.go:615-670`, `runtime_service.go:603-608`) | Recorded JavaScript dispatch metadata when retained | `pkg/services/factory_sessions/internal/execution/fake_service_dispatch_test.go:TestProjectRuntimeExecutionRecords_LiveChildDispatch_ProjectsLifecycleArtifactsAndProviderSession`; public active/terminal field labels `tests/functional/sessions/execution/fscp01_dispatch_provenance_test.go:TestFSCP01DispatchReadFieldProvenanceMatrix`; complete restart/replay parity is `INCONCLUSIVE` |

### Dispatch conclusion

The current read is a deliberate hybrid, not one uniform source:

1. Active handed-off reads begin with the durable execution/runtime snapshot.
2. Lifecycle event cursors and Recording flush watermarks annotate the active
   result with confirmation metadata.
3. Replay-only reads begin with Recording projection values; dispatch inventory
   is empty/not-found before the explicit runtime handoff.
4. Canonical event facts may enrich or preserve terminal dispatch state, but
   the current public tests do not prove every returned field has the same
   value across active, terminal, restart, and replay paths.

Rows marked `INCONCLUSIVE` or `UNPROVEN` are intentional stop points for
FSCP01-DISPATCH-004. A convergence change must not silently turn them into a
new projection rule.

## Transport witness matrix

The transport rows identify the customer boundary that later semantic gates
must execute. Package tests prove mapping/typed errors; functional tests prove
the public boundary. Neither a handler unit test nor a symbol inventory alone
is runtime acceptance.

| ID | Transport / cells | Exact witness | Disposition | Later gate |
| --- | --- | --- | --- | --- |
| T-01 | HTTP live open, decoded input, missing folder, close | `pkg/services/factory_sessions/transports/http/session_control_test.go:TestHandlerFromRoot_OpenFactorySessionEncodesRootResult`, `TestHandlerFromRoot_OpenFactorySessionMissingFolderPathReturnsBadRequestWithoutRootCall`, `TestHandlerFromRoot_CloseFactorySessionInvokesRoot`; public lifecycle `tests/functional/sessions/lifecycle/crud_test.go:TestAPIOpenListGetAndCloseFactorySession` | `PASS` | FSCP01-TRANSPORT-005 |
| T-02 | HTTP durable async start, invalid source, typed request conflict/validation | `pkg/services/factory_sessions/transports/http/session_control_test.go:TestHandlerFromRoot_StartDurableFactorySessionAsyncInvokesRootWithDecodedRequest`, `TestHandlerFromRoot_StartDurableFactorySessionAsyncInvalidSourceReturnsBadRequestWithoutRootCall`; `pkg/services/factory_sessions/transports/http/root_errors_test.go:TestHandlerFromRoot_StartDurableFactorySessionAsyncRequestIDConflictReturnsTypedErrorResponse`, `TestHandlerFromRoot_StartDurableFactorySessionAsyncValidationErrorReturnsTypedErrorResponse` | `PASS` | FSCP01-TRANSPORT-005 |
| T-03 | HTTP session list/get and typed not-found/scope errors | `pkg/services/factory_sessions/transports/http/session_read_test.go:TestHandlerFromRoot_ListFactorySessionsDecodesScopeBeforeRootInvocation`, `TestHandlerFromRoot_ListFactorySessionsInvalidScopeReturnsBadRequestWithoutRootCall`, `TestHandlerFromRoot_GetFactorySessionEncodesRootProjectionToAPI`, `TestHandlerFromRoot_GetFactorySessionNotFoundReturnsTypedErrorResponse` | `PASS` | FSCP01-TRANSPORT-005 |
| T-04 | HTTP pause/resume/cancel/terminate/delete controls and typed conflicts | `pkg/services/factory_sessions/transports/http/session_control_test.go:TestHandlerFromRoot_PauseDurableFactorySessionEncodesRootLifecycleControl`, `TestHandlerFromRoot_PauseLiveFactorySessionEncodesRootLifecycleControl`, `TestHandlerFromRoot_LiveCancelAndTerminateUseSupportedLifecycleControls`, `TestHandlerFromRoot_LiveLifecycleControlConflictIsTyped`; `pkg/services/factory_sessions/transports/http/root_errors_test.go:TestHandlerFromRoot_PauseDurableFactorySessionControlConflictReturnsTypedLifecycleResponse`, `TestHandlerFromRoot_DeleteFactorySessionConflictReturnsTypedErrorResponse` | `PASS` | FSCP01-TRANSPORT-005 |
| T-05 | HTTP response-event SSE, cursor/filter validation, session expiry | `pkg/services/factory_sessions/transports/http/handlers_response_events_test.go:TestGetFactoryResponseEventsBySessionId_DurableSessionStreamsSSE`, `TestGetFactoryResponseEventsBySessionId_RejectsInvalidAfterSequence`, `TestGetFactoryResponseEventsBySessionId_RejectsInvalidKindFilter`, `TestGetFactoryResponseEventsBySessionId_MapsDurableSessionNotFound`, `TestGetFactoryResponseEventsBySessionId_RequiresDurableProjection`; public stream behavior R-08–R-10 | `PASS` | FSCP01-TRANSPORT-005 |
| T-06 | CLI live create/list and durable run/result/failure output | `pkg/services/factory_sessions/transports/cli/session/create_test.go:TestCreate_JSONModeEmitsOpenFactorySessionResponse`, `pkg/services/factory_sessions/transports/cli/session/list_test.go:TestList_JSONModeEmitsListFactorySessionsResponse`; `tests/functional/transport/cli/commands/run_wiring_test.go:TestCLIRunNamedFactory`, `TestCLIRunByPath`, `TestCLIRunPrimaryResultFromStdin`, `TestCLIRunFailureWritesNoSuccessPayloadToStdout`, `TestCLIRunAmbiguousPromptAndStdinFailsBeforeRuntimeStartup` | `PASS` | FSCP01-TRANSPORT-005 |
| T-07 | CLI NDJSON/text response ordering, writer failure, interruption, resume | `tests/functional/transport/cli/output/ndjson_stream_test.go:TestCLINDJSONEmitsDecodableResponseEventsThenInvocationResult`, `TestCLINDJSONFailureEndsWithOneTerminalResult`; `tests/functional/transport/cli/output/stream_backpressure_test.go:TestCLISlowWriterDoesNotReorderResponseEvents`, `TestCLIWriterFailureCancelsInvocation`; `tests/functional/transport/cli/output/text_stream_test.go:TestCLITextStreamInterruptedRunDoesNotClaimCompletion`, `TestCLITextStreamSurfacesIncrementalMessages`; resume `tests/functional/transport/cli/session_resume/resume_smoke_test.go:TestCLIResumeSmoke_InterruptedJavaScriptFactorySessionResumesThroughSharedSessionCommands` | `PASS` | FSCP01-TRANSPORT-005 |
| T-08 | ACP prompt start/reuse, failed invocation, redelivery, busy concurrency, cancel/close/replay | `tests/functional/sessions/chat_sessions/root_composition/acp_prompt_delegation_test.go:TestACPPromptDelegationStartsOneFactorySessionAndReusesItForLaterTurns`, `TestACPPromptDelegationFailedFactoryInvocationReportsAnACPError`, `TestACPPromptDelegationRedeliveredRequestMakesNoSecondFactoryDispatch`, `TestACPPromptDelegationConcurrentPromptRejectsAsBusyWithNoFactoryDispatch`; `tests/functional/transport/acp/stdio/cli_serve_acp_controls_test.go:TestServeACP_RootBuildProcessCancelTerminalizesOnlyCapturedPrompt`, `TestServeACP_RootBuildProcessCloseStopsCapturedFactorySession`, `TestServeACP_RootBuildProcessCloseThenLoadReplaysRetainedItemIdentities`; `tests/functional/transport/acp/stdio/cli_serve_acp_prompt_test.go:TestServeACP_RootBuildProcessCompletesOneFactoryPrompt` | `PASS` | FSCP01-TRANSPORT-005 |
| T-09 | ACP retained child event replay | `tests/functional/sessions/chat_sessions/root_composition/acp_worker_child_events_test.go:TestACPWorkerChildStreamSurvivesRetainedReplay` | `PASS` | FSCP01-TRANSPORT-005 |
| T-10 | MCP initialize/discovery, canonical tool inventory, malformed/unknown/missing-session errors | `tests/functional/transport/mcp/stdio/discovery_test.go:TestMCPStdioInitializeAndToolDiscovery`, `TestMCPDiscoveryContainsCanonicalFactorySessionTools`; `tests/functional/transport/mcp/protocol/errors_test.go:TestMCPMalformedParametersReturnInvalidParams`, `TestMCPUnknownToolReturnsProtocolError`, `TestMCPMissingFactorySessionReturnsCanonicalNotFound` | `PASS` | FSCP01-TRANSPORT-005 |
| T-11 | MCP start/get/result/list scope and typed result failures | `pkg/services/factory_sessions/transports/mcp/execution_bind_test.go:TestBind_FakeExecutionRootInvokedThroughCanonicalStartAsyncTool`, `TestBind_FakeExecutionRootInvokedThroughCanonicalGetSessionTool`, `TestBind_FakeExecutionRootInvokedThroughCanonicalListSessionsTool`; `pkg/services/factory_sessions/transports/mcp/execution_test.go:TestMockClient_StartAsync_RunningFixtureReturnsInProgressSession`, `TestMockClient_GetSession_RunningFixtureReturnsDeterministicStatus`, `TestMockClient_GetResult_RunningFixtureReturnsTypedNotReadyEnvelope`, `TestMockClient_StartSync_SuccessFixtureReturnsTerminalSession`, `TestMockClient_StartSync_RepeatedInvocationReturnsStableSessionIdentity`, `TestMockClient_ListSessions_ScopedPersistedAndAll`; `pkg/services/factory_sessions/transports/mcp/failure_paths_test.go:TestMockClient_StartAsync_RequestIDConflictReturnsTypedEnvelope`, `TestMockClient_GetResult_FailedFixtureReturnsPartialResultWithFailureDetails` | `PASS` | FSCP01-TRANSPORT-005 |
| T-12 | MCP dispatch/artifact/event inspection and controls | `pkg/services/factory_sessions/transports/mcp/inspection_test.go:TestMockClient_ListDispatches_DispatchInspectionFixtureReturnsStableSummaries`, `TestMockClient_ListDispatches_FiltersAndRejectsInvalidStatus`, `TestMockClient_ListArtifacts_ArtifactInspectionFixtureReturnsStableSummaries`, `TestMockClient_ReadEvents_EventReconnectFixtureReturnsOrderedCanonicalEvents`, `TestMockClient_Control_LifecycleFixtureReturnsAcceptedRejectedAndIsolatesSessions`; binding `pkg/services/factory_sessions/transports/mcp/execution_bind_test.go:TestBind_FakeRecordingsRootInvokedThroughCanonicalListDispatchesTool`, `TestBind_FakeExecutionRootInvokedThroughCanonicalControlTool` | `PASS` | FSCP01-TRANSPORT-005 |
| T-13 | Canonical event/artifact operations where a transport has no current operation | The future mode-neutral contract explicitly omits canonical event/artifact operations (`source-plan.md:234-271`); the current CLI/ACP/MCP surfaces must not be treated as equivalent to an absent endpoint. | `UNSUPPORTED` | FSCP01-TRANSPORT-005 |

## Reserved collision witnesses

The following witnesses are cited without modification. Their owning PRs are
part of the current integration state and must be refreshed/rechecked by the
owner before matrix rows are used as executed evidence.

| Owner | Reserved files | Cited cells |
| --- | --- | --- |
| PR #2531 — `recordings-legacy-sse-resolved-session-identity` | `pkg/services/factory_sessions/internal/sessionservice/lifecycle_test.go`; `pkg/services/factory_sessions/transports/http/root_binding_test.go` | K-04; HTTP root binding for session get/list (`TestHandlerFromRoot_GetFactorySessionInvokesSessionsRoot`, `TestHandlerFromRoot_ListFactorySessionsInvokesSessionsRoot`) |
| PR #2532 — `per-package-compute-reduction-c02-sessions-root-composition` | `tests/functional/sessions/root_composition/**`; `tests/functional/internal/support/process.go` | Root-process reuse, lifecycle/opening activation, work admission/response stream, cleanup, and process execution witnesses cited in E-15 and the gate declarations below |

No reserved file is edited by FSCP-01. If a reserved witness changes or is
removed before execution, the later gate must replace the citation with a
current exact witness or mark the row `UNPROVEN`; a `passes:true` routing
disposition is not acceptance evidence.

## Stale inventory versus executable evidence

The source plan's August 24, 2026 symbol inventory reports file counts such as
`StartAsync` 78, `StartSync` 62, `OpenFactorySession` 63,
`OpenFactorySessionFromFolder` 11, `GetSession` 112, `GetFactorySession` 63,
`ListSessions` 76, `ListFactorySessions` 58, `DurableExecutionService` 15,
`TargetExecutionService` 15, `runtimeopening` 42,
`applicationopening` 5, and `executionopening` 12. The current reconciliation
also recorded a later scan (September 4, 2026) of 20 `StartAsync`, 18
`StartSync`, 18 `OpenFactorySession`, 9 `DurableExecutionService`, 10
`TargetExecutionService`, 22 `runtimeopening`, 2 `applicationopening`, and 6
`executionopening` matches.

These are breadth indicators only. They do not prove identity, status,
ordering, cleanup, replay, artifact retrieval, or source provenance. Only the
named tests, contract citations, and explicit gap dispositions in this matrix
may be used as evidence.

The archived BTRC-P7 matrix at
`docs/internal/development/plans/archive/08-20/packaged-service-structure/btrc-p7-behavior-matrix.md`
is reusable background and contains useful root/HTTP/MCP/ACP/Recording
witnesses, but it is not the current FSCP acceptance artifact. Stale paths or
old ownership claims from that matrix must not be copied without current-tree
reconciliation.

## Verification declaration and remaining gates

### Story 001 procedure

The highest feasible verification is a documentation/contract audit:

```text
git rev-parse HEAD
rg -n "^type Service interface|StartAsync|StartSync|OpenFactorySession|OpenFactorySessionFromFolder|GetSession|GetFactorySession|ListSessions|ListFactorySessions|DurableExecutionService|TargetExecutionService" pkg/services/factory_sessions
rg -n --glob '*_test.go' '^func Test' pkg/services/factory_sessions/internal/controlplane pkg/services/factory_sessions/internal/sessionservice pkg/services/factory_sessions/internal/execution pkg/services/factory_sessions/transports tests/functional/sessions tests/functional/events tests/functional/transport
git ls-files --error-unmatch docs/internal/development/plans/backlog/factory-sessions-control-plane-convergence-behavior-matrix.md
```

Observed for this packet:

- HEAD resolved to `e10e38843aff30c7871b732b284976ee13ab42f1`.
- The authored root contract, owner facets, cited test paths, and named test
  declarations were inspected against the current tree.
- The matrix file is the only tracked artifact added by this story; no
  production, generated, configuration, persistence, or reserved PR-owned
  file was changed.
- This procedure proves matrix completeness, ownership classification, and
  explicit gap accounting only. It does not prove runtime identity, status,
  ordering, cleanup, canonical reads, artifacts, dispatch provenance, or any
  customer behavior.
- No browser check or paid validation applies to this documentation-only
  story.

### Story 002 procedure and observed evidence

The semantic witness is
`tests/functional/sessions/lifecycle/fscp01_entrypoint_matrix_test.go`. It
uses the package `TestMain` shared root-built process and HTTP server, creates
per-test temporary Factory folders, uses unique UUID request/session inputs,
and registers live and durable cleanup. The assertions cross the public
`/factory-sessions` open, sync-start, control, session-read, and result-read
routes; the invocation branch uses the existing CLI `Process.Execute` path.
No production or transport code changed.

Exact verification commands and observed results:

```text
go test ./tests/functional/sessions/lifecycle -run '^TestFSCP01' -count=1 -timeout 15m -v
go test ./pkg/services/factory_sessions/... -count=1 -timeout 10m
go test ./tests/functional/sessions/... -count=1 -timeout 15m
```

- The focused FSCP-01 package run passed both witnesses. It observed selected
  live and folder-auto-open sessions with UUID identities and `IDLE` status;
  durable async with a `dur-sess-*` identity and `RUNNING` status; and durable
  sync with an independent `dur-sess-*` identity, `SUCCEEDED` status,
  `COMPLETED` sync outcome, and `FINAL` result.
- The same run observed CLI invocation `COMPLETED` with the public result
  `placement parity complete`; live pause and resume both returned `ACCEPTED`
  and the read model asserted `PAUSED` then `RUNNING`; final durable result
  read returned `FINAL`; a missing workflow returned HTTP 400 with typed
  `BAD_REQUEST`; timeout without cancellation returned `TIMED_OUT` while the
  durable session remained `RUNNING`; timeout with cancellation returned
  `TIMED_OUT` with `sessionCanceledByTimeout=true` and reached `CANCELED`.
- The complete owner suite and complete Factory Sessions functional suite both
  passed. The owner suite covered the package contract, control plane,
  durable execution, runtime opening, replay, transports, and wiring; the
  functional suite covered lifecycle plus the sibling session layers.

This proves the story-002 public identity/status/mode comparison, invocation
result, live control, typed source failure, timeout overlay, session
isolation, and cleanup properties. It does not prove canonical reconnect or
artifact independence, field-by-field active/terminal dispatch provenance,
CLI/ACP/MCP mapping parity, a built artifact, or independent project
validation; those remain later gates.

### Story 003 procedure and observed evidence

The canonical-read witness is
`tests/functional/events/factory_events/fscp01_canonical_reads_test.go:TestFSCP01CanonicalReconnectAndArtifactReadsIndependentOfResponseEvents`.
It starts a completed durable session through the root-built functional API,
deliberately opens no `response-events` subscription, and reads only the
canonical session event and artifact authorities. The dispatch witness is
`tests/functional/sessions/execution/fscp01_dispatch_provenance_test.go:TestFSCP01DispatchReadFieldProvenanceMatrix`.
It uses controlled root-built active and terminal sessions, compares public
list/detail fields, joins each selected dispatch to its canonical Worker
Session association, and logs an explicit source label for every returned
field. The labels are limited to `Recording projection`, `durable execution
state`, `Runtime (transient) state`, and `UNPROVEN`.

Exact verification commands and observed results:

```text
go test ./pkg/services/recordings/... -count=1 -timeout 10m
go test ./tests/functional/events/factory_events -run '^TestFSCP01CanonicalReconnectAndArtifactReadsIndependentOfResponseEvents$' -count=1 -timeout 15m -v
go test ./tests/functional/sessions/execution -run '^TestFSCP01DispatchReadFieldProvenanceMatrix$' -count=1 -timeout 15m -v
```

- The complete Recordings owner suite passed across canonical events, replay,
  projections, artifacts, dispatch lifecycle, worker associations, and typed
  failures.
- The canonical root-built witness passed with three ordered canonical
  lifecycle events and one completed-workflow artifact. Repeated full reads
  were stable; event-ID and session-sequence reconnects returned the exact
  suffix with no duplicate or gap; the terminal boundary was unique and last;
  the artifact list/detail and safe retrieval reference were stable; unknown
  cursor, missing/corrupt/foreign artifact, and unknown-session probes returned
  typed outcomes. No response stream was opened, so this proves canonical
  reads do not require a retained/live response subscription. It does not
  prove an explicit response-subscription open/close teardown sequence.
- The dispatch root-built witness passed for active `dispatch-2` (`RUNNING`)
  and terminal `dispatch-1` (`COMPLETED`). Both had attempt `1`, a
  session-qualified canonical Worker Session association, public list/detail
  common-field equality, and a logged source label for every returned field;
  fields not independently exposed by the fixture remain `UNPROVEN`.
  Unknown and wrong-session dispatch reads returned typed `404 NOT_FOUND`
  outcomes.

This advances FSCP01-CANONICAL-003 and FSCP01-DISPATCH-004. It does not prove
explicit response-stream teardown, restart/replay field parity across every
dispatch path, transport mapping, real provider or persistence behavior, or
independent project validation.

### Story 004 procedure and observed evidence

Story 004 reconciled every current transport row, T-01 through T-13, against
its exact package or functional witness. Each row already had a current owner
test covering the documented identity, status, result, control, error, or
ordering cell; no missing transport assertion was found, so no transport test
or production file was added. T-13 remains `UNSUPPORTED` because the current
transport contracts intentionally do not expose canonical event or artifact
operations.

The integrated hermetic proof is the composition of these root-built
witnesses:

- `tests/functional/sessions/root_composition/process_reuse_inert_test.go:TestRootBuildProcessIsInertAndReusableAcrossFactorySessions` proves reusable
  process construction, two terminal outcomes, response/canonical stream
  identity, cleanup, and an injected start failure surfaced without a success
  payload after the process has handled prior sessions.
- `tests/functional/transport/http/server/concurrent_requests_test.go:TestAPIConcurrentSessionRequestsRemainIsolated`
  proves two explicit sessions remain correlated during overlapping work.
- `tests/functional/events/response_events/concurrent_session_isolation_test.go:TestConcurrentFactorySessionResponseEventStreamsStayIsolatedAndResumeFromCursor`
  proves concurrent response streams retain session identity and cursor order.
- The story-002 lifecycle witness, story-003 canonical/artifact witness, and
  story-003 dispatch-provenance witness prove the corresponding public result,
  canonical, artifact, and dispatch fields without requiring one fixture to
  expose every authority at once.
- T-01 through T-12 retain the exact HTTP, CLI, ACP, and MCP mappings listed in
  the transport matrix; T-13 records the intentional unsupported surface.

Exact verification commands and observed results:

```text
go test ./pkg/transports/http/... ./pkg/transports/cli/... ./pkg/transports/acp/... ./pkg/transports/mcp/... -count=1 -timeout 10m
go test ./tests/functional/transport/http/... ./tests/functional/transport/cli/... ./tests/functional/transport/acp/... ./tests/functional/transport/mcp/... -count=1 -timeout 15m
go test ./pkg/services/factory_sessions/... -count=1 -timeout 10m
go test ./pkg/services/recordings/... -count=1 -timeout 10m
go test ./tests/functional/sessions/... -count=1 -timeout 15m
go test ./tests/functional/events/... -count=1 -timeout 15m
go test -race ./tests/functional/events/factory_events ./tests/functional/sessions/execution ./tests/functional/sessions/lifecycle ./pkg/services/factory_sessions/... -count=1 -timeout 20m
go test -race ./pkg/services/recordings/... -count=1 -timeout 20m
go test -race ./tests/functional/transport/http/... ./tests/functional/transport/cli/... ./tests/functional/transport/acp/... ./tests/functional/transport/mcp/... ./tests/functional/events/response_events -count=1 -timeout 20m
make test
make lint
```

- All focused transport package and functional transport commands passed.
  Factory Sessions, Recordings, session, event, and all three changed-surface
  race commands also passed. `make test` passed.
- The backend-size target passed after splitting a canonical witness helper;
  the remaining Go/static lint targets passed. `make lint` could not complete
  green in this checkout because `ui-lint` cannot resolve the absent
  `ui/node_modules/@biomejs/biome/bin/biome`, `ui-deadcode` has no existing
  `knip` binary under its `--no-install` policy, and `deadcode` reports the
  repository baseline drift `3080` baseline findings versus `3078` current.
  No dependency download or baseline rewrite is in scope for this packet.
- No browser, paid, binary-artifact, real-provider, or persistence edge was
  applicable or claimed. The changed-file allowlist remains the matrix and
  unreserved functional test files only; no production, generated, public
  contract, or persisted-schema file changed.

This completes FSCP01-TRANSPORT-005 at the hermetic implementation boundary.
It does not claim clean-room validation, real artifact/process/persistence
behavior, blind customer validation, independent engineering validation, or
later convergence work.

### Later run declarations

Each semantic run must record the following before claiming a row:

| Declaration | Required value |
| --- | --- |
| Source/head | Current pushed commit and exact source-plan SHA above |
| Layer | Unit for package contract; parallel Factory Sessions functional test through `root.BuildProcess`/`Process.Execute`; integration only for a prebuilt artifact; no binary build in functional tests |
| Isolation | Per-test temporary project/config/state/cache directories, unique session/request/turn IDs, isolated ports, and no ambient user state |
| Edges | Explicit fake/mock provider/worker/clock/recording edges; real provider/network behavior is a separate integration lane |
| Concurrency | Parallel-safe sessions with no sleeps; use bounded event/status observation and cleanup on every path |
| Result | Exact command, artifact/log, observed status/outcome, property proved, and remaining unproven edge |

| Gate | Owns |
| --- | --- |
| `FSCP01-HERMETIC-002` | Stories 002 semantic start/open identity, lifecycle, invocation, result, timeout, close, and recovery proof |
| `FSCP01-CANONICAL-003` | Story 003 independent canonical event/artifact reads and replay/reconnect proof |
| `FSCP01-DISPATCH-004` | Story 003 field-by-field active/terminal dispatch source and provenance proof |
| `FSCP01-TRANSPORT-005` | Story 004 HTTP, CLI, ACP, MCP mapping and integrated public-boundary proof |
| `VAL-SC-02-LOOPBACK-001` | Independent validation of the integrated FSCP behavior lane after this packet and later stories land |

### Remaining unproven edges

1. Explicit response-stream open/close teardown followed by canonical reads is
   not covered; the story-003 witness proves independence by never opening the
   ephemeral stream.
2. Restart/replay dispatch field parity remains `INCONCLUSIVE`/`UNPROVEN`
   where the current tests do not assert each field across every handoff path.
3. Transport rows and the final hermetic integrated proof are recorded above;
   the unsupported T-13 surface remains intentionally non-comparable.
4. Independent project validation, final acceptance, and any real-provider
   artifact behavior are later gates; this packet does not claim them.
