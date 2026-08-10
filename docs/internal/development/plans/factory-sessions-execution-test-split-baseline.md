# Factory Sessions execution test split — story 001 baseline

Captured: 2026-08-10 13:39 -07:00
Source baseline: `947152348b235f3dd0ede47f233947fcadddcc6a`
Scope: `pkg/services/factory_sessions/internal/execution/...`

This artifact freezes the pre-move evidence for the mechanical test split. The
two target files were not changed before capture. Every top-level test declared
in `fake_service_test.go` or `fake_service_runtime_test.go` appears exactly
once in the placement map below. Destination names are the planned focused
package-local files for stories 002–005; the map is a review aid, not a second
test inventory.

## Baseline suite

Command:

```text
go test ./pkg/services/factory_sessions/internal/execution/...
```

Output:

```text
ok  	github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution	(cached)
ok  	github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/fixtures	(cached)
ok  	github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/recordingreplay	(cached)
ok  	github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/runtimepersist	(cached)
```

## Full baseline test inventory

Command:

```text
go test -list '.*' ./pkg/services/factory_sessions/internal/execution/...
```

The complete command output is retained verbatim below, including the package
status lines.

```text
TestChildWorkerExecutor_CompletedChildRecordsItsWorkerAndOutput
TestChildWorkerExecutor_FailedChildCarriesItsProviderWithTheSessionReference
TestChildWorkerExecutor_InvocationErrorStillRecordsAFailedChild
TestChildWorkerExecutor_ScopesTheWorkersIdentityAndReleasesItAfterTheWorker
TestChildWorkerExecutor_CarriesTheAuthoredWorkerNameAndPermissionPolicy
TestNewJavaScriptExecutionServiceRequiresSyncWaitScheduler
TestWaitSyncCompletionUsesInjectedClockAndRecurringScheduler
TestJavaScriptRuntimeService_SubscribeResponseEvents_PublishesAndStreamsChildProgress
TestJavaScriptRuntimeService_SubscribeResponseEvents_RejectsMissingStore
TestJavaScriptRuntimeService_SubscribeResponseEvents_RejectsInvalidCursor
TestJavaScriptRuntimeService_SessionProgressPublisher_MapsGenericFragments
TestJavaScriptRuntimeService_SubscribeResponseEvents_RequiresRuntime
TestEnsureSessionResponseEvents_RequiresIDGenerator
TestMaterializeEventReadStream_OwnsFiniteClosedLifecycleAndDetachedHistory
TestMaterializeEventReadStream_EmptyResultStillReturnsClosedStream
TestTaggedDurableHistoryIsAuthoritativeDuringHydrationAndResave
TestProjectResultRead_ModePartialAndFinal
TestProjectResultRead_TerminalFinalAndUnavailable
TestProjectResultRead_FailedWithPartialHonorsPartialMode
TestProjectResultRead_IncludeArtifactsShaping
TestProjectResultRead_NotReadyRunningSession
TestProjectResultRead_DefaultsToFinalMode
TestFakeService_InternalLifecycleHelpers
TestFakeService_InternalStartAndProjectionHelpers
TestIsTerminalLifecycleStatus
TestEvaluateLifecycleControl_ValidTransitions
TestEvaluateLifecycleControl_InvalidAndTerminal
TestNormalizeRetryDispatchRequest_RequiresDispatchID
TestControlIdempotencyTupleHash_IsStable
TestCheckControlRequestIDReplay_Conflict
TestServiceMethods_PropagateContextCancellation
TestJavaScriptRuntimeService_StartSync_UsesInjectedClock
TestJavaScriptRuntimeService_StartSync_SimpleWorkflowCompletesWithPrimaryResult
TestJavaScriptRuntimeService_StartSync_PersistenceFailureDoesNotPublishSuccess
TestJavaScriptRuntimeService_StartAsync_PersistenceFailurePublishesFailureNotSuccess
TestJavaScriptRuntimeService_StartAsync_RunningCancelAndReads
TestJavaScriptRuntimeService_StartAsync_FailedAndTimedOut
TestJavaScriptRuntimeService_StartSync_WaitTimeoutWithoutCancelKeepsSessionRunning
TestExecutionServiceAndHelperNormalization
TestPersistenceChoiceForPolicy_DefaultDoesNotCreateProjectDurableSessions
TestPersistenceChoiceForPolicy_EnabledCreatesProjectDurableSessions
TestNormalizationAndIdempotencyHelpers
TestPrepareStartAndPersistenceHelpers
TestValidateNamedAgentPresetsRejectsUnknownPresetBeforeStart
TestJavaScriptRuntimeService_ProjectRootAloneDoesNotEnablePersistence
TestFakeService_DetailReadersAndRemainingControlWrappers
TestJavaScriptRuntimeService_ControlWrappersAndDetailReaders
TestNormalizeStartRequestAndErrorHelpers
TestRuntimeAndValidationHelperBranches
TestStartSourceRequestAndResolutionOrderBranches
TestJavaScriptRuntimeService_ReplayAndReadErrorBranches
TestPersistAndMetadataNoOpBranches
TestNormalizeStartRequestAdditionalSourceBranches
TestListingFiltersAndNormalizationBranches
TestSmallHelperBranches
TestProjectionCloneHelpers
TestProjectedLifecycleControlStatus_PrefersCanonicalBracketStatus
TestProjectedLifecycleControlStatus_FallsBackToFactoryRuntimeState
TestFactoryStateToLifecycleStatus_MapsLiveFactoryStates
TestLiveLifecycleControlResponse_BuildsTypedPauseOutcome
TestJavaScriptRuntimeService_InterruptAcceptedBeforeChildCompletion_RecordsObservedCancellation
TestJavaScriptRuntimeService_LateChildResultAfterInterrupt_SuppressesNormalRouting
TestFakeService_InterruptAcceptedBeforeCompletion_ObservableDispatchAndEventOutcomes
TestJavaScriptRuntimeService_InterruptRunningDispatch_PreservesObservedCancellationAtRecordTime
TestValidateCheckpointSummaryForResume_RejectsInvalidMetadata
TestApplyRuntimeCheckpointPartialProjection_SurfacesPartialResultWhileRunning
TestApplyRuntimeCheckpointPartialProjection_NoopsForTerminalOrEmptyCheckpoint
TestJavaScriptRuntimeService_ApplyRunningRuntimeRecord_CheckpointProjectsPartialResult
TestFinalizeInterruptedTerminalSession_PreservesPartialAndUnavailableResults
TestJavaScriptRuntimeService_PausePersistsStablePartialTerminalReadState
TestJavaScriptRuntimeService_PausePersistenceFailureKeepsRunningProjection
TestInterruptedTerminalTimestamp_PrefersSessionLifecycle
TestResumeHelperFunctions_CoverMergeCloneAndPolicyPaths
TestApplyRuntimeSuccessProjection_InvalidResultMarksFailed
TestCheckpointEventProjection_BuildsCanonicalCheckpointEvents
TestPhaseEventProjection_PreservesOrderedRunningAndTerminalPhases
TestJavaScriptRuntimeService_FactoryEventObserverDeliversOnlyUnseenEvents
TestRuntimeRecordEvents_ReconcileAppendOnlyPhaseCheckpointPhaseHistory
TestFakeService_ResumeInterruptedSession_ReturnsUnsupported
TestJavaScriptRuntimeService_ResumeInterruptedSession_PackageLocalCoverage
TestJavaScriptRuntimeServiceWriteRecordingUsesCanonicalSnapshotAndCorrelatesFailure
TestJavaScriptRuntimeService_StartSync_WorkflowFilePolicyDeniesDisallowedModel
TestNewFakeServiceRequiresExplicitClockBeforeFixtureIO
TestNewFakeServiceFromContractFixturesRequiresInjectedReader
TestLoadFakeScenariosUsesInjectedReader
TestFakeService_StartAsync_ProjectsFixtureScenarios
TestFakeService_StartAsync_IdempotentReplay
TestFakeService_StartAsync_ErrorBranches
TestFakeService_StartSync_TerminalAndTimeoutFixtures
TestFakeService_StartSync_AppliesCancelOnTimeoutOverlay
TestFakeService_StartSync_ErrorAndReplayBranches
TestFakeService_LifecycleControl_IdempotentReplayAndConflict
TestFakeService_LifecycleControls_UpdateStateAndErrors
TestFakeService_LifecycleControl_ErrorBranches
TestFakeService_ReadProjections_MatchFixtureDispatchesArtifactsEvents
TestFakeService_ReadMethods_ErrorAndFallbackBranches
TestFakeService_ReadEvents_ReturnsCanonicalFixtureEventsAndHonorsCursor
TestFakeService_ReadEvents_InvalidCursorReturnsError
TestFakeService_DerivedProjectionEvents_AreCanonicalWhenFixtureEventsMissing
TestFakeService_ListSessions_ScopedPersistedAndLive
TestFakeService_StartAsync_ConcurrentIdempotentStarts
TestFakeService_ConstructorsAndHelpers
TestFilterDispatches_PhaseStatusAndValidation
TestQueryDispatches_ReadsAndFiltersInsideExecutionOwner
TestProgressCountsFromDispatches_GroupsEveryCanonicalStatus
TestNormalizeResultRequest_DefaultsAndValidation
TestNormalizeEventReconnectRequest_RejectsNegativeSequence
TestValidateResultMatchesSessionRead
TestValidateDispatchListMatchesSessionProgress
TestValidateResultMatchesEventProjection
TestProjectionServiceMethods_PropagateContextCancellation
TestBuildCanonicalSessionEvents_RunningAndTerminalSessions
TestProjectRuntimeExecutionRecords_LiveChildDispatch_ProjectsLifecycleArtifactsAndProviderSession
TestProjectRuntimeExecutionRecords_FailedLiveChild_ProjectsFailureDetail
TestFilterEventsAfterReconnect_AfterEventIDAndSequence
TestReplaySessionProjection_TerminalSessionBracket
TestReplaySessionProjection_IdempotentOnDuplicateSequence
TestReplaySessionProjection_ReplacesArtifactStubsWithoutDuplication
TestReplaySessionProjection_FirstTerminalOutcomeWinsCompetingRace
TestReplaySessionProjection_PreservesSyncTimeoutAvailability
TestReplaySessionProjection_IgnoresUnknownEventTypes
TestReplaySessionProjection_EquivalentOrchestratorsSharePublicSessionProjection
TestReplayDispatchProjection_EquivalentOrchestratorsMatchLiveDispatchSummary
TestReplayDispatchProjection_EquivalentOrchestratorsPreserveAbsentProviderSession
TestReplaySessionProjection_EquivalentOrchestratorsRestoreArtifactsAndLatestLifecycle
TestReplaySessionProjection_EquivalentOrchestratorsPreserveResultAvailability
TestAppendDispatchInterruptedEvent_RecordsCanonicalMetadata
TestMarkDispatchInterrupted_UpdatesInspectionProjection
TestReplayDispatchProjection_DerivesInterruptedDispatchMetadata
TestFakeService_InterruptDispatch_RecordsDispatchInterruptedEvent
TestRestoreInterruptedDispatchResultSuppression_LateCompletionDoesNotReactivateRouting
TestApplyTerminalRuntimeProjection_PreservesInterruptedDispatchAndEvents
TestReplaySessionProjection_PauseResumeLifecycleEventsDeriveStatus
TestReplaySessionProjection_LegacyPauseResumeEventsDeriveStatus
TestFakeService_PauseResumeAppendsLifecycleControlEventsWithoutNoOpMutation
TestApplyInlineFactoryDeclarationPreservesWorkflowFileDefaultPolicy
TestPublishWorkerProgress_ReachesOnlyTheSessionThatStartedTheWorker
TestPublishWorkerProgress_IgnoresADispatchNoSessionOwns
TestPublishWorkerProgress_StopsOnceTheWorkerIsReleased
TestChildWorkerExecutor_ScopesTheWorkersIdentityToItsSession
TestExecutionServiceRolesNameWorkersRootContracts
TestSmokeLiveChildProviderUsesWorkersRootInferenceContracts
TestDurableExecutionConstructionUsesRootWorkflowContracts
TestNew_SelectsFakeAndKeepsInstancesIsolated
TestNewJavaScript_ForwardsClockProjectRootAndPersistence
TestNewJavaScript_ForwardsLiveChildProviderAndMode
TestNewJavaScript_AsyncRunningSupportsStatusNotReadyAndCancellation
TestNewJavaScript_AsyncCompletionPublishesTerminalResult
TestNew_RejectsUnsupportedAndIncompleteConfiguration
ok  	github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution	0.033s
TestPublishedFixtureScenarios_DocumentStableIdentity
TestPublishedFixtureScenarios_MatchExportedCatalogRows
TestLoadFixtureScenarioIdentities_ReloadIsStable
TestFakeService_PublishedScenarios_AsyncStartInspectionLinksAndEventPrefix
TestFakeService_PublishedScenarios_ResultArtifactInclusion
TestFakeService_PublishedScenarios_ListDispatchesStableSummaries
TestFakeService_PublishedScenarios_GetDispatchDetailAndUnknownError
TestFakeService_PublishedScenarios_ListArtifactsStableSummaries
TestFakeService_PublishedScenarios_GetArtifactDetailAndUnknownError
TestFakeService_InterruptDispatchRace_ObservableServiceOutcomes
TestFakeService_PublishedScenarios_ReadEventsCanonicalAndReconnect
TestFakeService_PublishedScenarios_ReadEventsMissingCursorReturnsTypedError
TestFakeService_PublishedScenarios_DispatchListIncludesProviderSessionRefs
TestFakeService_PublishedScenarios_LifecycleControlPauseResumeOutcomes
TestFakeService_PublishedScenarios_LifecycleControlCancelTerminateOutcomes
TestFakeService_PublishedScenarios_LifecycleControlApproveAwaitingApproval
TestFakeService_PublishedScenarios_LifecycleControlRetryDispatchPaths
TestFakeService_PublishedScenarios_LifecycleControlAcceptedInspectionLinks
TestFakeService_PublishedScenarios_LifecycleControlIdempotentReplayAndConflict
TestFakeService_PublishedScenarios_LifecycleControlIsolationAcrossSessions
TestFakeService_PublishedScenarios_LifecycleControlDeterministicAcrossServiceReload
TestJavaScriptRuntimeService_CancelRunningSessionReturnsCanceling
TestJavaScriptRuntimeService_TerminateRunningSessionReturnsTerminated
TestJavaScriptRuntimeService_CancelTerminalSessionReturnsTypedControlError
TestJavaScriptRuntimeService_CancelMissingSessionReturnsNotFound
TestJavaScriptRuntimeService_PauseRunningSessionReturnsPaused
TestJavaScriptRuntimeService_ResumePausedSessionReturnsRunning
TestJavaScriptRuntimeService_PauseTerminalSessionReturnsTypedControlError
TestJavaScriptRuntimeService_ApproveRunningSessionReturnsTypedControlError
TestJavaScriptRuntimeService_RetryDispatchMissingDispatchReturnsNotFound
TestJavaScriptRuntimeService_ControlIdempotentReplayAndConflict
TestBuildCanonicalRuntimeSessionEvents_ProjectsLiveProviderDispatchLifecycle
TestBuildCanonicalRuntimeSessionEvents_ProjectsFailedDispatchReconciliation
TestMapCanonicalRuntimeSessionEvents_EquivalentOrchestratorsHaveSharedPublicMeaning
TestMapCanonicalRuntimeSessionEvents_RejectsMalformedFactsWithoutPartialEvents
TestFakeService_PublishedScenarios_ListSessionsScopedWithDedup
TestNormalizeListSessionsRequest_DefaultsToLiveAndRejectsUnsupportedScope
TestApplySessionListScope_LivePersistedAndAllDedup
TestJavaScriptRuntimeService_StartAsync_IdempotentReplay
TestJavaScriptRuntimeService_StartSync_IdempotentReplay
TestJavaScriptRuntimeService_Start_ExecutionRequestIDConflict
TestJavaScriptRuntimeService_Start_ConcurrentIdempotentStarts
TestJavaScriptRuntimeService_StartAsyncReplay_PreservesFirstObservedRunningResponse
TestJavaScriptRuntimeService_StartSyncWaitTimeoutReplay_PreservesFirstObservedTimedOutResponse
TestJavaScriptRuntimeService_Start_CrossModeRequestIDConflict
TestJavaScriptRuntimeService_Start_RejectsInvalidWaitAndPolicy
TestJavaScriptRuntimeService_TypedFailures_MissingSessionMissingSourceBadSource
TestJavaScriptRuntimeService_StartSync_SimpleWorkflowCompletesWithPrimaryResult
TestJavaScriptRuntimeService_StartSyncStreamsCanonicalPhaseBeforeCompletion
TestJavaScriptRuntimeService_StartAsync_SimpleWorkflowCompletesWithInspectableResult
TestJavaScriptRuntimeService_StartAsync_RunningBeforeCompletion
TestJavaScriptRuntimeService_StartAsync_Failed
TestJavaScriptRuntimeService_StartAsync_TimedOut
TestJavaScriptRuntimeService_StartAsync_Canceled
TestJavaScriptRuntimeService_StartSync_WaitTimeoutWithoutCancelKeepsSessionRunning
TestNewExecutionService_SelectsFakeAndJavaScriptRuntimeProviders
TestJavaScriptRuntimeService_EventReplay_ReconstructsCompletedSessionProjection
TestJavaScriptRuntimeService_EventReplay_ReconstructsRunningSessionProjection
TestJavaScriptRuntimeService_EventReplay_ReconstructsSyncTimeoutProjection
TestJavaScriptRuntimeService_EventReplay_IsIdempotent
TestJavaScriptRuntimeService_EventReplay_ReconstructsAsyncCompletedSession
TestJavaScriptRuntimeService_AgentRunLiveChild_ProjectsRealDispatchInspection
TestJavaScriptRuntimeService_AgentRunFakeChild_RemainsDefaultWithoutRuntimeOverride
TestJavaScriptRuntimeService_AgentRunLiveChild_TimeoutInterruptsProviderInfer
TestJavaScriptRuntimeService_AgentRunLiveChild_StartAsyncProjectsRunningDispatchForInterrupt
TestJavaScriptRuntimeService_ParallelLiveChildFailure_ProjectsTypedFailureAndPreservesSiblings
TestJavaScriptRuntimeService_AgentRunLiveChildFailure_ProjectsFailedDispatchOnWorkflowFailure
TestJavaScriptRuntimeService_ChildExecutorModes_CoexistOnSameWorkflowSource
TestJavaScriptRuntimeService_ExplicitFakeMode_OverridesLiveServiceConfig
TestJavaScriptRuntimeService_ParallelFakeChildren_RemainsDeterministicWithoutProvider
TestJavaScriptRuntimeService_PipelineFakeChildren_RemainsDeterministicWithoutProvider
TestJavaScriptRuntimeService_LiveAndReplayEventsRemainIdenticalAcrossPhaseCheckpointPhase
TestJavaScriptRuntimeService_ProgressPrimitives_ProjectsArtifactsPhaseAndProgress
TestJavaScriptRuntimeService_AgentRunFakeChild_ProjectsDispatchAndChildArtifact
TestProjectRuntimeExecutionRecords_ProgressPrimitivesFixture
TestNewExecutionService_FakeProvider_PublishedScenarios_StillDeterministic
TestJavaScriptRuntimeService_UsesExistingFactorySessionReadSurfaces
TestPetriRuntime_MutationsPersistAndReloadThroughFactorySessionOwner
TestPersistedRuntimeSessionState_MixedTypedHistoryRoundTripsAndReplays
TestPersistedRuntimeSessionState_MixedTypedHistoryRejectsUnknownAndMalformedRecords
TestJavaScriptRuntimeService_ResumeInterruptedSession_ReconstructsFromCheckpointSummary
TestJavaScriptRuntimeService_ResumeInterruptedSession_RehydratesCheckpointStateForControlFlow
TestJavaScriptRuntimeService_ResumeInterruptedSession_PreservesLiveChildOutput
TestJavaScriptRuntimeService_ResumeInterruptedSession_ExposesReadSurfacesAndEventLineage
TestJavaScriptRuntimeService_ResumeInterruptedSession_MissingCheckpointReturnsTypedFailure
TestJavaScriptRuntimeService_ResumeInterruptedSession_CorruptedPersistenceReturnsTypedFailure
TestJavaScriptRuntimeService_ResumeInterruptedSession_InvalidCheckpointSummaryReturnsTypedFailure
TestJavaScriptRuntimeService_ResumeInterruptedSession_NonApprovedCheckpointReturnsTypedFailure
TestJavaScriptRuntimeService_ResumeInterruptedSession_RejectsCheckpointDispatchNotDurablyCompleted
TestJavaScriptRuntimeService_ResumeInterruptedSession_RejectsRegressedEventCursor
TestJavaScriptRuntimeService_ResumeInterruptedSession_NonInterruptedSessionReturnsTypedFailure
TestJavaScriptRuntimeService_NonResumedFakeChild_PreservesShippedTransportSemantics
TestJavaScriptRuntimeService_NonResumedSimpleFinal_PreservesReplayReconnectAndTerminalResult
TestJavaScriptRuntimeService_NonResumedTerminalSnapshot_OmitsCheckpointSummaryAndReloadsAcrossFreshServices
TestNewExecutionService_FakeProvider_PublishedScenarios_RemainAdditiveAfterRestartResumeLane
TestFakeService_PublishedScenarios_SyncStartTerminalAndTimeout
TestFakeService_PublishedScenarios_GetSessionReadModels
TestFakeService_PublishedScenarios_ResultReadsWithStableHash
TestFakeService_PublishedScenarios_StartIdempotentReplay
TestProjectedResultReadHash_IsStableAcrossEquivalentReads
TestMatchesDurableSessionListFilters_StatusOrchestratorAndRecoverability
TestDurableListSummaryFromSessionRead_ProjectsActionAvailability
TestFakeService_PublishedTypedFailures_StartAndReadErrors
TestFakeService_PublishedTypedFailures_LifecycleErrors
TestFakeService_MalformedRequests_DoNotMutateFixtureState
ok  	github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/fixtures	0.039s
TestReplayRecordingRestoresCompletedPublicReadModelsWithoutLiveExecution
TestReplayRecordingRestoresFailedPartialReadModelsWithoutManufacturingSuccess
TestReplayRecordingRejectsHashInconsistentResultWithoutPartialProjection
TestReplayRecordingRestoresPausedCheckpointWithoutLiveControls
TestReplayRecordingRestoresResumedHistoryAndFinalAvailability
TestServiceExposesRecordedSessionResultAndEvents
TestServiceExposesRecordedArtifactsAndEmptyDispatches
TestServiceRejectsUnknownSessionsAndLiveOperations
ok  	github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/recordingreplay	0.035s
TestNewProjectStore_ConstructsSnapshotBoundaryAndRoundTrips
TestNewProjectStore_RejectsMissingAndUnavailableRoots
TestSaveLoadBytes_RoundTripsSnapshotPayload
TestSaveLoadBytes_AcceptsCanonicalFactorySessionIdentifiers
TestSaveBytes_RejectsUnsafeSessionIdentifiers
TestStoreFailsClosedWithoutFileSystem
TestStorePropagatesInjectedFileSystemFailures
ok  	github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/runtimepersist	0.028s
```

## Baseline package coverage

Command and options to repeat after each split:

```text
go test -covermode=atomic ./pkg/services/factory_sessions/internal/execution/...
```

Output:

```text
ok  	github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution	(cached)	coverage: 79.6% of statements
ok  	github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/fixtures	(cached)	coverage: 80.0% of statements
ok  	github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/recordingreplay	(cached)	coverage: 83.1% of statements
ok  	github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/runtimepersist	(cached)	coverage: 89.8% of statements
```

Reported statement coverage:

| Package | Coverage |
| --- | ---: |
| `pkg/services/factory_sessions/internal/execution` | 79.6% |
| `pkg/services/factory_sessions/internal/execution/fixtures` | 80.0% |
| `pkg/services/factory_sessions/internal/execution/recordingreplay` | 83.1% |
| `pkg/services/factory_sessions/internal/execution/runtimepersist` | 89.8% |

## Behavior-based placement map

The source line is the declaration line in the unmodified baseline. Every
top-level test from the two target files is listed once, for a total of
120 tests.

### `fake_service_construction_start_test.go`

| Baseline source | Line | Test |
| --- | ---: | --- |
| `fake_service_runtime_test.go` | 463 | `TestFakeService_InternalStartAndProjectionHelpers` |
| `fake_service_test.go` | 50 | `TestNewFakeServiceRequiresExplicitClockBeforeFixtureIO` |
| `fake_service_test.go` | 63 | `TestNewFakeServiceFromContractFixturesRequiresInjectedReader` |
| `fake_service_test.go` | 72 | `TestLoadFakeScenariosUsesInjectedReader` |
| `fake_service_test.go` | 99 | `TestFakeService_StartAsync_ProjectsFixtureScenarios` |
| `fake_service_test.go` | 148 | `TestFakeService_StartAsync_IdempotentReplay` |
| `fake_service_test.go` | 181 | `TestFakeService_StartAsync_ErrorBranches` |
| `fake_service_test.go` | 206 | `TestFakeService_StartSync_TerminalAndTimeoutFixtures` |
| `fake_service_test.go` | 240 | `TestFakeService_StartSync_AppliesCancelOnTimeoutOverlay` |
| `fake_service_test.go` | 281 | `TestFakeService_StartSync_ErrorAndReplayBranches` |
| `fake_service_test.go` | 732 | `TestFakeService_StartAsync_ConcurrentIdempotentStarts` |
| `fake_service_test.go` | 762 | `TestFakeService_ConstructorsAndHelpers` |

### `fake_service_lifecycle_inspection_test.go`

| Baseline source | Line | Test |
| --- | ---: | --- |
| `fake_service_runtime_test.go` | 396 | `TestFakeService_InternalLifecycleHelpers` |
| `fake_service_runtime_test.go` | 1865 | `TestFakeService_DetailReadersAndRemainingControlWrappers` |
| `fake_service_test.go` | 313 | `TestFakeService_LifecycleControl_IdempotentReplayAndConflict` |
| `fake_service_test.go` | 346 | `TestFakeService_LifecycleControls_UpdateStateAndErrors` |
| `fake_service_test.go` | 398 | `TestFakeService_LifecycleControl_ErrorBranches` |
| `fake_service_test.go` | 428 | `TestFakeService_ReadProjections_MatchFixtureDispatchesArtifactsEvents` |
| `fake_service_test.go` | 465 | `TestFakeService_ReadMethods_ErrorAndFallbackBranches` |
| `fake_service_test.go` | 606 | `TestFakeService_ReadEvents_ReturnsCanonicalFixtureEventsAndHonorsCursor` |
| `fake_service_test.go` | 646 | `TestFakeService_ReadEvents_InvalidCursorReturnsError` |
| `fake_service_test.go` | 658 | `TestFakeService_DerivedProjectionEvents_AreCanonicalWhenFixtureEventsMissing` |
| `fake_service_test.go` | 674 | `TestFakeService_ListSessions_ScopedPersistedAndLive` |
| `fake_service_test.go` | 811 | `TestFilterDispatches_PhaseStatusAndValidation` |
| `fake_service_test.go` | 847 | `TestQueryDispatches_ReadsAndFiltersInsideExecutionOwner` |
| `fake_service_test.go` | 872 | `TestProgressCountsFromDispatches_GroupsEveryCanonicalStatus` |
| `fake_service_test.go` | 891 | `TestNormalizeResultRequest_DefaultsAndValidation` |
| `fake_service_test.go` | 915 | `TestNormalizeEventReconnectRequest_RejectsNegativeSequence` |
| `fake_service_test.go` | 924 | `TestValidateResultMatchesSessionRead` |
| `fake_service_test.go` | 949 | `TestValidateDispatchListMatchesSessionProgress` |
| `fake_service_test.go` | 972 | `TestValidateResultMatchesEventProjection` |
| `fake_service_test.go` | 993 | `TestProjectionServiceMethods_PropagateContextCancellation` |

### `fake_service_replay_test.go`

| Baseline source | Line | Test |
| --- | ---: | --- |
| `fake_service_test.go` | 1016 | `TestBuildCanonicalSessionEvents_RunningAndTerminalSessions` |
| `fake_service_test.go` | 1059 | `TestProjectRuntimeExecutionRecords_LiveChildDispatch_ProjectsLifecycleArtifactsAndProviderSession` |
| `fake_service_test.go` | 1122 | `TestProjectRuntimeExecutionRecords_FailedLiveChild_ProjectsFailureDetail` |
| `fake_service_test.go` | 1201 | `TestFilterEventsAfterReconnect_AfterEventIDAndSequence` |
| `fake_service_test.go` | 1291 | `TestReplaySessionProjection_TerminalSessionBracket` |
| `fake_service_test.go` | 1347 | `TestReplaySessionProjection_IdempotentOnDuplicateSequence` |
| `fake_service_test.go` | 1388 | `TestReplaySessionProjection_ReplacesArtifactStubsWithoutDuplication` |
| `fake_service_test.go` | 1428 | `TestReplaySessionProjection_FirstTerminalOutcomeWinsCompetingRace` |
| `fake_service_test.go` | 1454 | `TestReplaySessionProjection_PreservesSyncTimeoutAvailability` |
| `fake_service_test.go` | 1488 | `TestReplaySessionProjection_IgnoresUnknownEventTypes` |
| `fake_service_test.go` | 1518 | `TestReplaySessionProjection_EquivalentOrchestratorsSharePublicSessionProjection` |
| `fake_service_test.go` | 1606 | `TestReplayDispatchProjection_EquivalentOrchestratorsMatchLiveDispatchSummary` |
| `fake_service_test.go` | 1649 | `TestReplayDispatchProjection_EquivalentOrchestratorsPreserveAbsentProviderSession` |
| `fake_service_test.go` | 1677 | `TestReplaySessionProjection_EquivalentOrchestratorsRestoreArtifactsAndLatestLifecycle` |
| `fake_service_test.go` | 1743 | `TestReplaySessionProjection_EquivalentOrchestratorsPreserveResultAvailability` |

### `fake_service_dispatch_test.go`

| Baseline source | Line | Test |
| --- | ---: | --- |
| `fake_service_runtime_test.go` | 2762 | `TestFakeService_InterruptAcceptedBeforeCompletion_ObservableDispatchAndEventOutcomes` |
| `fake_service_runtime_test.go` | 3642 | `TestFakeService_ResumeInterruptedSession_ReturnsUnsupported` |
| `fake_service_test.go` | 1856 | `TestAppendDispatchInterruptedEvent_RecordsCanonicalMetadata` |
| `fake_service_test.go` | 1919 | `TestMarkDispatchInterrupted_UpdatesInspectionProjection` |
| `fake_service_test.go` | 1942 | `TestReplayDispatchProjection_DerivesInterruptedDispatchMetadata` |
| `fake_service_test.go` | 1980 | `TestFakeService_InterruptDispatch_RecordsDispatchInterruptedEvent` |
| `fake_service_test.go` | 2038 | `TestRestoreInterruptedDispatchResultSuppression_LateCompletionDoesNotReactivateRouting` |
| `fake_service_test.go` | 2106 | `TestApplyTerminalRuntimeProjection_PreservesInterruptedDispatchAndEvents` |
| `fake_service_test.go` | 2192 | `TestReplaySessionProjection_PauseResumeLifecycleEventsDeriveStatus` |
| `fake_service_test.go` | 2261 | `TestReplaySessionProjection_LegacyPauseResumeEventsDeriveStatus` |
| `fake_service_test.go` | 2349 | `TestFakeService_PauseResumeAppendsLifecycleControlEventsWithoutNoOpMutation` |

### `javascript_runtime_result_test.go`

| Baseline source | Line | Test |
| --- | ---: | --- |
| `fake_service_runtime_test.go` | 227 | `TestProjectResultRead_ModePartialAndFinal` |
| `fake_service_runtime_test.go` | 261 | `TestProjectResultRead_TerminalFinalAndUnavailable` |
| `fake_service_runtime_test.go` | 290 | `TestProjectResultRead_FailedWithPartialHonorsPartialMode` |
| `fake_service_runtime_test.go` | 310 | `TestProjectResultRead_IncludeArtifactsShaping` |
| `fake_service_runtime_test.go` | 350 | `TestProjectResultRead_NotReadyRunningSession` |
| `fake_service_runtime_test.go` | 367 | `TestProjectResultRead_DefaultsToFinalMode` |
| `fake_service_runtime_test.go` | 2260 | `TestJavaScriptRuntimeService_ReplayAndReadErrorBranches` |
| `fake_service_runtime_test.go` | 2391 | `TestListingFiltersAndNormalizationBranches` |
| `fake_service_runtime_test.go` | 2494 | `TestProjectionCloneHelpers` |
| `fake_service_runtime_test.go` | 3398 | `TestApplyRuntimeSuccessProjection_InvalidResultMarksFailed` |

### `javascript_runtime_start_test.go`

| Baseline source | Line | Test |
| --- | ---: | --- |
| `fake_service_runtime_test.go` | 776 | `TestServiceMethods_PropagateContextCancellation` |
| `fake_service_runtime_test.go` | 880 | `TestJavaScriptRuntimeService_StartSync_UsesInjectedClock` |
| `fake_service_runtime_test.go` | 911 | `TestJavaScriptRuntimeService_StartSync_SimpleWorkflowCompletesWithPrimaryResult` |
| `fake_service_runtime_test.go` | 1097 | `TestJavaScriptRuntimeService_StartAsync_RunningCancelAndReads` |
| `fake_service_runtime_test.go` | 1155 | `TestJavaScriptRuntimeService_StartAsync_FailedAndTimedOut` |
| `fake_service_runtime_test.go` | 1211 | `TestJavaScriptRuntimeService_StartSync_WaitTimeoutWithoutCancelKeepsSessionRunning` |

### `javascript_runtime_persistence_test.go`

| Baseline source | Line | Test |
| --- | ---: | --- |
| `fake_service_runtime_test.go` | 187 | `TestTaggedDurableHistoryIsAuthoritativeDuringHydrationAndResave` |
| `fake_service_runtime_test.go` | 952 | `TestJavaScriptRuntimeService_StartSync_PersistenceFailureDoesNotPublishSuccess` |
| `fake_service_runtime_test.go` | 985 | `TestJavaScriptRuntimeService_StartAsync_PersistenceFailurePublishesFailureNotSuccess` |
| `fake_service_runtime_test.go` | 1420 | `TestPersistenceChoiceForPolicy_DefaultDoesNotCreateProjectDurableSessions` |
| `fake_service_runtime_test.go` | 1450 | `TestPersistenceChoiceForPolicy_EnabledCreatesProjectDurableSessions` |
| `fake_service_runtime_test.go` | 1619 | `TestPrepareStartAndPersistenceHelpers` |
| `fake_service_runtime_test.go` | 1688 | `TestJavaScriptRuntimeService_ProjectRootAloneDoesNotEnablePersistence` |
| `fake_service_runtime_test.go` | 2318 | `TestPersistAndMetadataNoOpBranches` |

### `javascript_runtime_validation_test.go`

| Baseline source | Line | Test |
| --- | ---: | --- |
| `fake_service_runtime_test.go` | 1259 | `TestExecutionServiceAndHelperNormalization` |
| `fake_service_runtime_test.go` | 1523 | `TestNormalizationAndIdempotencyHelpers` |
| `fake_service_runtime_test.go` | 1675 | `TestValidateNamedAgentPresetsRejectsUnknownPresetBeforeStart` |
| `fake_service_runtime_test.go` | 2058 | `TestNormalizeStartRequestAndErrorHelpers` |
| `fake_service_runtime_test.go` | 2144 | `TestRuntimeAndValidationHelperBranches` |
| `fake_service_runtime_test.go` | 2225 | `TestStartSourceRequestAndResolutionOrderBranches` |
| `fake_service_runtime_test.go` | 2347 | `TestNormalizeStartRequestAdditionalSourceBranches` |
| `fake_service_runtime_test.go` | 2478 | `TestSmallHelperBranches` |
| `fake_service_runtime_test.go` | 3955 | `TestJavaScriptRuntimeService_StartSync_WorkflowFilePolicyDeniesDisallowedModel` |

### `javascript_runtime_control_test.go`

| Baseline source | Line | Test |
| --- | ---: | --- |
| `fake_service_runtime_test.go` | 664 | `TestIsTerminalLifecycleStatus` |
| `fake_service_runtime_test.go` | 697 | `TestEvaluateLifecycleControl_ValidTransitions` |
| `fake_service_runtime_test.go` | 724 | `TestEvaluateLifecycleControl_InvalidAndTerminal` |
| `fake_service_runtime_test.go` | 737 | `TestNormalizeRetryDispatchRequest_RequiresDispatchID` |
| `fake_service_runtime_test.go` | 749 | `TestControlIdempotencyTupleHash_IsStable` |
| `fake_service_runtime_test.go` | 768 | `TestCheckControlRequestIDReplay_Conflict` |
| `fake_service_runtime_test.go` | 1913 | `TestJavaScriptRuntimeService_ControlWrappersAndDetailReaders` |
| `fake_service_runtime_test.go` | 2563 | `TestProjectedLifecycleControlStatus_PrefersCanonicalBracketStatus` |
| `fake_service_runtime_test.go` | 2571 | `TestProjectedLifecycleControlStatus_FallsBackToFactoryRuntimeState` |
| `fake_service_runtime_test.go` | 2581 | `TestFactoryStateToLifecycleStatus_MapsLiveFactoryStates` |
| `fake_service_runtime_test.go` | 2595 | `TestLiveLifecycleControlResponse_BuildsTypedPauseOutcome` |
| `fake_service_runtime_test.go` | 2611 | `TestJavaScriptRuntimeService_InterruptAcceptedBeforeChildCompletion_RecordsObservedCancellation` |
| `fake_service_runtime_test.go` | 2667 | `TestJavaScriptRuntimeService_LateChildResultAfterInterrupt_SuppressesNormalRouting` |
| `fake_service_runtime_test.go` | 2874 | `TestJavaScriptRuntimeService_InterruptRunningDispatch_PreservesObservedCancellationAtRecordTime` |
| `fake_service_runtime_test.go` | 3167 | `TestJavaScriptRuntimeService_PausePersistsStablePartialTerminalReadState` |
| `fake_service_runtime_test.go` | 3274 | `TestJavaScriptRuntimeService_PausePersistenceFailureKeepsRunningProjection` |
| `fake_service_runtime_test.go` | 3310 | `TestInterruptedTerminalTimestamp_PrefersSessionLifecycle` |

### `javascript_runtime_checkpoint_test.go`

| Baseline source | Line | Test |
| --- | ---: | --- |
| `fake_service_runtime_test.go` | 2929 | `TestValidateCheckpointSummaryForResume_RejectsInvalidMetadata` |
| `fake_service_runtime_test.go` | 3010 | `TestApplyRuntimeCheckpointPartialProjection_SurfacesPartialResultWhileRunning` |
| `fake_service_runtime_test.go` | 3048 | `TestApplyRuntimeCheckpointPartialProjection_NoopsForTerminalOrEmptyCheckpoint` |
| `fake_service_runtime_test.go` | 3077 | `TestJavaScriptRuntimeService_ApplyRunningRuntimeRecord_CheckpointProjectsPartialResult` |
| `fake_service_runtime_test.go` | 3110 | `TestFinalizeInterruptedTerminalSession_PreservesPartialAndUnavailableResults` |
| `fake_service_runtime_test.go` | 3349 | `TestResumeHelperFunctions_CoverMergeCloneAndPolicyPaths` |
| `fake_service_runtime_test.go` | 3654 | `TestJavaScriptRuntimeService_ResumeInterruptedSession_PackageLocalCoverage` |

### `javascript_runtime_event_recording_test.go`

| Baseline source | Line | Test |
| --- | ---: | --- |
| `fake_service_runtime_test.go` | 3426 | `TestCheckpointEventProjection_BuildsCanonicalCheckpointEvents` |
| `fake_service_runtime_test.go` | 3486 | `TestPhaseEventProjection_PreservesOrderedRunningAndTerminalPhases` |
| `fake_service_runtime_test.go` | 3536 | `TestJavaScriptRuntimeService_FactoryEventObserverDeliversOnlyUnseenEvents` |
| `fake_service_runtime_test.go` | 3572 | `TestRuntimeRecordEvents_ReconcileAppendOnlyPhaseCheckpointPhaseHistory` |
| `fake_service_runtime_test.go` | 3920 | `TestJavaScriptRuntimeServiceWriteRecordingUsesCanonicalSnapshotAndCorrelatesFailure` |


## Shared fixture and helper ownership

The following ownership table gives each shared fixture/helper one planned
package-local owner. Helpers used only by one behavior group move with that
group; they are not copied into multiple destinations.

| Planned owner | Helpers |
| --- | --- |
| `execution_test_helpers_test.go` | `contractFixturesPath`, `newContractFakeService`, `fakeServiceTestClock`, `mustNewFakeService`, `int64Ptr`, `jsonEqual`, `canonicalTypedInternalEvent`, `assertCanonicalEventEnvelope` |
| `fake_service_construction_start_test.go` | `startAsyncByRequestID` |
| `fake_service_lifecycle_inspection_test.go` | `testFakeServiceReadSessionAndResultBranches`, `testFakeServiceReadDispatchAndArtifactBranches`, `testFakeServiceReadEventAndListingBranches`, `recordingDispatchListReader` and its `ListDispatches` method, `stubProjectionCancelAwareService` and its `GetResult` method |
| `javascript_runtime_test_helpers_test.go` | `javaScriptRuntimeServiceConfig`, `testRuntimePersistenceStoreFactory`, `mustTestRuntimePersistenceStore`, `newConfiguredJavaScriptRuntimeService`, `mustTestRecordingWriter`, `portableRecordingTestWriter.Write`, `seedRuntimeSessionWithRunningDispatch`, `applyRuntimeTerminalOutcome`, `stubCancelAwareService` methods, `durableFixedClock.Now`, `newDefaultJavaScriptRuntimeService`, `scriptedRuntimeWorkflows`, `scriptedSuccessfulRuntimeWorkflows`, `scriptedBlockingRuntimeWorkflows`, `scriptedFailedRuntimeWorkflows`, `inlineWorkflowStartRequest`, `waitUntilSessionStatus`, `decodePrimaryResultMap`, `writeSimpleFinalWorkflowProject`, `orchestrationJavaScriptFromWorkflows`, and `orchestrationJavaScriptAdapter` methods |
| `javascript_runtime_persistence_test.go` | `assertPersistenceFailureRolledBackLiveProjections`, `assertPersistenceFailureClearedInternalRuntimeState`, `runtimeRecordingStore.Save`, and `runtimeRecordingStore.Load` |
| `javascript_runtime_control_test.go` | `newJavaScriptRuntimeRunningControlState`, `testJavaScriptRuntimeServiceRunningControlWrappers`, `testJavaScriptRuntimeServiceApproveAwaitingSession`, `testJavaScriptRuntimeServiceRetryFailedDispatch` |
| `javascript_runtime_validation_test.go` | `testExecutionServiceProviders`, `testExecutionServiceSourceRequestHelpers`, `testNormalizationApproveAndSourceBranches`, `testNormalizationCanonicalAndReplayBranches`, `testNormalizationIdempotencyHashBranches`, `testNormalizeStartRequestBranches`, `testNormalizeSourceAndExecutorModeBranches`, `testControlAndValidationErrorHelpers`, `testRuntimeHookAndMarshalBranches`, `testRuntimeMetadataAndSourceValidationBranches`, `testRuntimePolicyValidationBranches`, `policyDeniedModelWorkflows` |
| `javascript_runtime_persistence_test.go` | `testExecutionServicePersistenceChoices`, `testExecutionServiceRequiredPersistenceDependencies`, `testExecutionServiceDisabledPersistence`, `testExecutionServiceInvalidPersistenceChoices`, `testApplicationPersistencePolicies` |
| `javascript_runtime_checkpoint_test.go` | `newResumeCoverageBlockingProvider` and its methods, `setupResumeCoverageWorkflowFixture`, `waitForResumeCoverageSessionStatus`, `waitForResumeCoverageDispatchStatus` |
| `javascript_runtime_event_recording_test.go` | `findDispatchInterruptedEventPayload`, `containsEventType`, `findDispatchByID`, `phaseEventStatuses`, `assertStrictCanonicalSequences` |
