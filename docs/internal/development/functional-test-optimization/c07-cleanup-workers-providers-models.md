# C07 Workers, Providers, Provider Sessions, and Models characterization ledger

Status: CHAR-001 complete for story functional-test-optimization-c07-cleanup-workers-providers-models-001.

This is a current-head discovery and characterization record. It does not claim
the cleanup repairs, repeat/race evidence, integrated clean-room validation, or
PR-CI evidence owned by later stories. No production, shared-support, public
contract, or excluded-surface file was changed for this story.

## Authority and scope

- Repository: you-agent-factory
- Branch: functional-test-optimization-c07-cleanup-workers-providers-models
- Exact head and origin/main at discovery:
  34cbab9f253208328c59b82eb8a3f17b76c09d15
- PRD story: functional-test-optimization-c07-cleanup-workers-providers-models-001
- Owned patterns:
  - ./tests/functional/workers/...
  - ./tests/functional/providers/...
  - ./tests/functional/provider_sessions/cli
  - ./tests/functional/models/root_composition
- Source-plan reference: docs/temp/functional-test-optimization.md was not
  present in this checkout. The PRD and repository standards are therefore the
  authoritative scope for this characterization, and the missing source plan
  is recorded rather than reconstructed.
- Paid/remote validation: 0 calls, $0.

The package profile in the tables below is an ownership classification, not a
claim that every test in a package acquires every resource in that profile.
The exact test identity appendix is the denominator; the resource matrix
records the representative owner, witness, cleanup path, and remaining gap.

## DISC-001 exact executable denominator

Discovery used the local real Go toolchain:

    go version
    go list ./tests/functional/workers/... ./tests/functional/providers/... ./tests/functional/provider_sessions/cli ./tests/functional/models/root_composition
    go test -list '^Test' <each resolved package separately>

Environment: go1.25.0 windows/amd64, Windows build 10.0.26200.0, x64,
24 processors. The default-tag denominator is 26 resolved packages and 225
tests. Four resolved packages have zero default-tag tests and remain in the
denominator. Adding -tags=functionallong exposes 15 additional tests, for
240 visible tests; those tests are not silently counted as default tests.

| Family | Resolved packages | Default tests | functionallong additions | Profile |
| --- | ---: | ---: | ---: | --- |
| Workers | 13 | 85 | 9 | W-* |
| Providers | 11 | 77 | 5 | P-* |
| Provider Sessions CLI | 1 | 9 | 0 | S-CLI |
| Models root composition | 1 | 54 | 1 | M-ROOT |
| **Total** | **26** | **225** | **15** | **240 tagged-visible** |

### Package denominator and owner profile

| Package | Default tests | Profile | Owned resource boundary and cleanup authority |
| --- | ---: | --- | --- |
| tests/functional/workers/agent | 4 | W-ROOT | Root-built Workers/Providers behavior; package/test cleanup and root lifecycle helpers. |
| tests/functional/workers/concurrency | 1 | W-SESSION | Concurrent Factory Sessions and worker routes; session close and fixture cleanup. |
| tests/functional/workers/inference | 31 | W-ROOT/W-OS | Root, provider process, stream, temp workspace, and process-tree paths; t.Cleanup/CleanupProcess and platform helpers. |
| tests/functional/workers/inference/agy | 2 | W-ROOT | Controlled AGY golden edge; no live credential acquisition in the default tests. |
| tests/functional/workers/inference/claude | 4 | W-ROOT | Controlled Claude golden/root edge; long stream/tool cases are tag-gated. |
| tests/functional/workers/inference/codex | 4 | W-ROOT/W-OS | Root, Codex command/process, worktree, and session paths; test cleanup helpers. |
| tests/functional/workers/invoke_continue | 14 | W-SESSION/W-HTTP | Direct/remote Worker Session invocation, continuation, stream, and HTTP resources. |
| tests/functional/workers/mock | 5 | W-ROOT | Mock/CLI worker outcomes and controlled command side effects; long alignment case is tag-gated. |
| tests/functional/workers/script | 2 | W-ROOT | Script worker process, workspace, prompt, and output paths; long resource-template cases are tag-gated. |
| tests/functional/workers/transports/cli/run/help | 4 | W-CLI | Built CLI process and temp invocation artifacts; CLI process/test cleanup. |
| tests/functional/workers/transports/cli/run/lifecycle | 6 | W-CLI/W-HTTP | Built CLI, attached server/session, packaged staging, and lifecycle resources. |
| tests/functional/workers/transports/cli/run/modes | 2 | W-CLI | Built CLI output modes and temporary invocation resources. |
| tests/functional/workers/transports/http | 6 | W-HTTP/W-SESSION | HTTP server, remote Worker Session, stream, and cancellation resources. |
| tests/functional/providers | 35 | P-ROOT/P-OS | Shared provider fixture, root/API, routes, sessions, command processes, temp paths, and teardown assertions. |
| tests/functional/providers/acp | 25 | P-ACP | ACP stdio process/connection, prompt, stream, session, and shutdown ownership. |
| tests/functional/providers/agy | 10 | P-AGY | Controlled AGY command/process and temp paths; live smoke is explicitly gated/skipped here. |
| tests/functional/providers/claude | 2 | P-ROOT | Controlled Claude stream/process edge through root composition. |
| tests/functional/providers/codex | 2 | P-ROOT/P-OS | Codex command/process, root, worktree, and history resources. |
| tests/functional/providers/contract | 0 | 0 | Resolved package with no default-tag tests. |
| tests/functional/providers/discovery | 2 | P-ROOT | Root-built provider/model discovery and temporary fixture path. |
| tests/functional/providers/mock_workers | 0 | 0 | Resolved package with no default-tag tests. |
| tests/functional/providers/observability | 0 | 0 | Resolved package with no default-tag tests. |
| tests/functional/providers/permission | 1 | P-ROOT | Public permission selection/bypass boundary through root composition. |
| tests/functional/providers/script | 0 | 0 | Resolved package with no default-tag tests. |
| tests/functional/provider_sessions/cli | 9 | S-CLI | CLI, HTTP/stream, Worker Session, recording/replay, and temporary fixture resources. |
| tests/functional/models/root_composition | 54 | M-ROOT | Root/API, model host, HTTP/listener, cache/assets, temp paths, and runtime readiness state. |

### Exact default-tag test identities

The following is the exact default-tag output of go test -list '^Test', grouped
by the 26 resolved package paths. The package counts above and this appendix
are the DISC-001 denominator; a zero-test package is listed explicitly.

#### tests/functional/workers/agent (4)

- TestAgentRunnerDeliveryRemainsInertThroughRootBuildProcessConstruction
- TestBuildProcessExecutesModelWorkerThroughConvergedWorkersService
- TestBuildProcessResolvesRegisteredAgentThroughProviders
- TestBuildProcessExecutesProviderAttemptThroughRuntimeRoot

#### tests/functional/workers/concurrency (1)

- TestFactoryRuntimeConcurrentSessionsShareWorkersWithoutCancellationLeakage

#### tests/functional/workers/inference (31)

- TestProviderNonZeroExitMapsToPublicFailure
- TestProviderMissingCompletionEvidenceMapsToPublicFailure
- TestProviderTaskCompletePartialOutputDoesNotAdvanceWork
- TestProviderAuthRateLimitAndTimeoutRemainDistinct
- TestProviderFailureRedactsPromptEnvironmentAndCredentials
- TestProviderPermissionWorktreeAndModelFlagsMapToCommand
- TestUnsupportedProviderFlagReturnsCapabilityError
- TestWSRFT003ProviderNeutralLifecycleWorksWithoutProviderSession
- TestWSRFT006PortableWorkerRecordingParity
- TestWSRFT006PortableExportSelectsRootBuiltWorkerSession
- TestProcessGoneReleasesSameRouteAdmissionThroughRootProcess
- TestProcessGoneReconciliationThroughRootProcess
- TestProviderTimeoutTerminatesChildProcessTree
- TestProcessTreeHelper
- TestProviderSuccessWaitsForProcessAndStreamClosure
- TestWSRFT004DurableOpeningGatesProviderHandoff
- TestWSRFT005CompletedWorkerReplayParity
- TestWSRFT008PostHandoffRecordingLossPreservesExecutionTruth
- TestWSRFT007WorkerRecordingHealthMatrix
- TestWSRFT009CanonicalRestartRecoversWorkerHistory
- TestSharedInferenceCommandRouterRejectsUnknownSelector
- TestExplicitProviderAndModelReachSelectedProviderEdge
- TestWorkerProviderOverridesGlobalDefault
- TestUnknownProviderFailsBeforeProcessStart
- TestWorkersWireRejectsInvalidInferenceRunner
- TestProviderFullStreamClaimsDeltasAndSnapshotsTruthfully
- TestProviderPartialStreamDoesNotFabricateMissingDeltas
- TestDetachedStructuredResultReachesDispatchResponse
- TestWSRFT011WorkerSessionCursorResumeAcrossRestart
- TestWSRFT012WorkerSessionFollowAndProviderReferenceParity
- TestWSRFT010WorkerSessionIDHTTPHistory

#### tests/functional/workers/inference/agy (2)

- TestAgyGoldenFinalOnlySuccess
- TestAgyGoldenTimeout

#### tests/functional/workers/inference/claude (4)

- TestClaudeGoldenStructuredFailure
- TestClaudeGoldenTimeoutClosesResponseStream
- TestClaudeConductorSuccessThroughRootBuildProcess
- TestClaudeCommandCancellationThroughRootBuildProcessIsCanonical

#### tests/functional/workers/inference/codex (4)

- TestCodexDefaultLaneSharedProcess
- TestCodexCommandRouterFailsClosed
- TestCodexGoldenSharedProcess
- TestCodexWorktreeWorkstationDispatch_MaterializesCheckoutAndOmitsCLIWorktreeFlag

#### tests/functional/workers/invoke_continue (14)

- TestDWROS8ManagerInterruptsOnlyOneRemoteWorker
- TestDWROS8ManagerInspectsTwoIsolatedRemoteWorkers
- TestDirectWorkerSessionContinueUnsupportedProviderDoesNotFreshStartThroughRootProcess
- TestDirectWorkerSessionInvokeContinueLocalPreservesSessionAndLineage
- TestDirectWorkerSessionInvokeExecutionFileToleratesFutureFields
- TestWSRFT015DirectWorkerSessionResumeUsesExactRecordedProviderSession
- TestDirectWorkerSessionRemoteInterruptUsesExactRouteAndAdmissionSnapshots
- TestDirectWorkerSessionRemoteControlsUseExactRoutesWithoutFallback
- TestDirectWorkerSessionContinueUnknownSourceReturnsNotFoundWithoutProviderCall
- TestDirectWorkerSessionContinueUnassociatedSourceRejectsWithoutProviderContinuation
- TestDirectWorkerSessionContinueStaleProviderSessionDoesNotFreshStart
- TestDirectWorkerSessionRemoteContinueProviderFailuresDoNotFallback
- TestDirectWorkerSessionRemoteInvokeStreamSourceFailureThroughRootProcess
- TestDirectWorkerSessionRemoteInvokeCallerCancellationThroughRootProcess

#### tests/functional/workers/mock (5)

- TestBuiltCLIBatchExitCodesReportSingleWorkOutcome
- TestBuiltCLIBatchExitCodesAggregateFailureCauses
- TestJavaScriptMockWorkersRemainFakeWhenACPProviderIsSelected
- TestBuiltCLINamedInvocationExitCodesCharacterizeOneShot
- TestSharedProcessWorkersMock

#### tests/functional/workers/script (2)

- TestScriptWorkerSharedSuccessSpine
- TestScriptCommandRouterRejectsUnknownAndDuplicateSelectors

#### tests/functional/workers/transports/cli/run/help (4)

- TestCLIRunHelpShowsInvocationSignatureForNamedFactory
- TestCLIRunHelpDistinguishesRequiredAndOptionalParameters
- TestCLIRunHelpDoesNotDispatchExternalWork
- TestCLISessionHelpPublishesRunnablePlacementExamples

#### tests/functional/workers/transports/cli/run/lifecycle (6)

- TestCLIRunCleanInvocationCompletesWithoutDashboardStartup
- TestCLIRunToFilePreservesExactPromptAndRejectsBeforeProviderDispatch
- TestCLIRunWorkerReasoningEffortOverrideReachesCodexCommand
- TestCLIRunUnsupportedWorkerReasoningEffortRejectsBeforeProviderDispatch
- TestCLIRunServerAttachedInvocationTargetsExistingFactorySession
- TestCLIRunCleanInvocationFailurePreservesPublicError

#### tests/functional/workers/transports/cli/run/modes (2)

- TestCLIRunSuccessPrimaryResultTextJSONAndNDJSON
- TestCLIRunFailureOmitsFalseSuccessPrimaryResult

#### tests/functional/workers/transports/http (6)

- TestWorkerSessionRemoteInvokeObserveContinueUsesServerAfterDisconnect
- TestWorkerSessionHTTPDisconnectKeepsAdmittedWorkerAlive
- TestWorkerSessionHTTPShutdownJoinsAdmittedWorker
- TestWorkerSessionHTTPInterruptRejectsUnassociatedActiveSource
- TestWorkerSessionHTTPControlCancelConvergesTerminalSnapshot
- TestWorkerSessionHTTPReadDuringFactoryWork

#### tests/functional/providers (35)

- TestProvidersSharedProcessTopology
- TestProvidersSharedProcessRoutes
- TestProvidersSharedProcessAdverseRecovery
- TestScriptExecutor_Success
- TestScriptExecutor_Failure
- TestScriptExecutor_CommandCancellationIsReported
- TestScriptExecutor_MissingCommandFailsStartup
- TestScriptExecutor_InvalidWorkstationTemplateFailsBeforeCommand
- TestScriptExecutor_PreservesTokenColor
- TestScriptExecutor_SuccessWithColorMetadata
- TestScriptExecutor_FailureRoutesToFailedPlace
- TestScriptExecutor_ArgTemplating
- TestScriptExecutor_WorkTypeIDFromTargetPlace
- TestScriptExecutor_ArgTemplatingWithTags
- TestScriptExecutor_RuntimeWorkstationConfigResolvesWorkingDirectoryAndEnv
- TestScriptExecutor_RuntimeConfigMergePreservesCanonicalTopologyAndPromptTemplates
- TestScriptExecutor_RuntimeWorkstationTimeoutRequeuesAndRetriesOnLaterTick
- TestScriptExecutor_AsyncWorkerPoolTemplateFallbackScenarios
- TestIntegrationSmoke_TimeoutCancelsProcessTreeAndClearsActiveExecution
- TestIntegrationSmoke_TimeoutRequeuesWorkAndSucceedsOnLaterAttempt
- TestIntegrationSmoke_ProcessTreeHelper
- TestProviders_ForcedAssertionFailureCleansOwnedResources
- TestMockWorkers_AgentDefaultAcceptMovesWorkToOutputPlace
- TestMockWorkers_AgentRejectConfigRoutesFailureWithoutLoggingCommandOutput
- TestMockWorkers_AgentRejectConfigWithZeroExitCodeIsRejectedAtCustomerBoundary
- TestMockWorkers_EndToEndSmokeRunsMixedOutcomesWithoutLiveProviderCredentials
- TestMockWorkers_ScriptDefaultAcceptProducesSuccessfulScriptResult
- TestMockWorkers_ScriptRejectConfigRoutesFailureAndLogsCommandOutput
- TestMockWorkers_ScriptRejectConfigWithZeroExitCodeStillRoutesFailure
- TestMockWorkers_ScriptConfigExecutesCommandRunnerSideEffect
- TestMockWorkers_ScriptHelper
- TestMockWorkers_ServiceCommandRunnerCompletesModelAndScriptWorkers
- TestPackagedScriptRuntime_FreshInstallExecutesFactoryRelativeScript
- TestPackagedScriptRuntime_NonZeroExitUsesStandardFailureOutcome
- TestRuntimeLoggingSmoke_SuccessAndFailureRespectOutputEnvAndRollingPolicies

#### tests/functional/providers/acp (25)

- TestACPCommandStartFailureMapsToDependencyFailure
- TestACPFailureRedactsConfiguredSecretsFromStderr
- TestACPProtocolFailuresMapToStableWorkerFailureClasses
- TestUnavailableACPExecutableFailsBeforeStartWithMissingExecutableClass
- TestFactoryRunRetriesACPProviderByResumingExactSession
- TestRootConstructionDoesNotStartACPProcess
- TestUnknownExecutorProviderFailsBeforeACPProcessStart
- TestACPAgentHelperProcess
- TestProvidersACPSerializesConcurrentPromptsOnOneStdioConnection
- TestProvidersACPRestartsAfterCrashWithoutReplayingUncertainPrompt
- TestProvidersACPRetiresDisconnectedConnectionBeforeReuse
- TestProvidersACPRetainsOneOSProcessAndConnectionAcrossExecutions
- TestProvidersACPRejectsIncompatibleProtocolVersionAtStdioBoundary
- TestProvidersShutdownCancelsActivePromptAndJoinsACPProcess
- TestPinnedACPSDKGoldenManifestIsCompleteAndParseable
- TestACPGoldenRPCPeerProcess
- TestPackagedACPProfilesUseSharedConformanceBehavior
- TestPackagedACPUnexpectedCommand
- TestPackagedSpawnRunsPlannerChildrenAndMergerThroughPersistentACPStdio
- TestPackagedTournamentRunsCompetitorsAndJudgeThroughPersistentACPStdio
- TestYouRunMapsGoldenSessionAndConfigRPCFailuresToTerminalWork
- TestYouRunUsesPinnedACPWireGoldensAndProjectsTerminalOutput
- TestYouRunMapsSkipPermissionsToSDKGoldenPermissionSelection
- TestYouRunReturnsUnsupportedFilesystemAndTerminalRPCsAtTheACPBoundary
- TestACPSharedProcess

#### tests/functional/providers/agy (10)

- TestAgyMultimodalGoldenThroughRootBuildProcess
- TestAgyClipQAGoldenPassThroughRootBuildProcess
- TestAgyStructuredJSONGoldenThroughRootBuildProcess
- TestAgyMissingFileRefusalFailsWorkThroughRootBuildProcess
- TestAgyLiveSmoke
- TestAgyConductorSuccessThroughRootBuildProcess
- TestAgyNativeFailureThroughRootBuildProcessIsSafe
- TestAgyTimeoutFailureThroughRootBuildProcess
- TestAgyCommandCancellationThroughRootBuildProcessIsCanonical
- TestAgyProductionReviewRolesThroughRootBuildProcess

#### tests/functional/providers/claude (2)

- TestClaudeHaikuStreamJSONGoldens
- TestClaudeStreamJSONCommandThroughRootBuildProcess

#### tests/functional/providers/codex (2)

- TestCodexHistoricalInspectionCancelledDiscoveryThroughRootBuildProcess
- TestCodexSharedTrustedWorkAndHistory

#### tests/functional/providers/contract (0)

No default-tag tests.

#### tests/functional/providers/discovery (2)

- TestProvidersListThroughRootBuildProcess
- TestPackagedACPProjectionRejectsInvalidRuntimeBindings

#### tests/functional/providers/mock_workers (0)

No default-tag tests.

#### tests/functional/providers/observability (0)

No default-tag tests.

#### tests/functional/providers/permission (1)

- TestProviderPermissionBypassFunctionalContract

#### tests/functional/providers/script (0)

No default-tag tests.

#### tests/functional/provider_sessions/cli (9)

- TestFactoryTargetReadinessCharacterization
- TestWorkerSessionsStreamAbortReturnsTypedDiagnosticThroughRootProcess
- TestBuiltWorkerSessionsStreamAbortExitsNonZero
- TestBuiltWorkerSessionsStreamCancellationExits130
- TestWorkerSessionsReplayOnlyRedirectsWellFormedNDJSON
- TestWSRFT001OpeningRecordPrecedesProviderOutput
- TestWSRFT002LiveAndReplayCorrelationRemainStable
- TestWorkerSessionsCLI
- TestWorkerSessionsFleetListCLIConcurrent

#### tests/functional/models/root_composition (54)

- TestModelsASRDirectCLIEndToEndThroughRootBuildProcess
- TestModelsEffectsRemainInertThroughRootBuildProcessConstruction
- TestModelsCatalogCLIProjectsFactoryDiscoveryThroughRootBuildProcess
- TestGenericModelContractsRemainDetachedAtApplicationRoot
- TestModelsCatalogDiscoveryActivatesThroughRootBuildProcessAfterLifecycle
- TestModelsRootCompositionModelScenarios
- TestModelsCatalogDiscoveryProjectsWorkerCapabilitiesAndFactoryPrecedence
- TestModelsCatalogDiscoveryMapsUnknownDetailThroughHTTP
- TestModelsCatalogDiscoveryMapsUnsupportedOperationThroughHTTP
- TestModelsCatalogReadinessFailureKeepsPublicUnavailableTaxonomy
- TestModelsCatalogReadinessCancellationReturnsPublicFailure
- TestModelsInvokeReadinessDependencyFailureIsUnavailableAfterCatalogSuccess
- TestModelsInvokeCatalogDependencyCancellationIsSafeThroughProcess
- TestModelsInvokeCatalogRequestCancellationStopsReadiness
- TestModelsInvokeReadinessCancellationAfterCatalogSuccessIsSafe
- TestModelsInvokeReadinessCancellationAfterSuccessfulObservationIsSafe
- TestModelsCatalogProjectsBuiltInsThroughRootBuildProcess
- TestModelsCatalogReadinessFailureStaysUnavailableThroughHTTP
- TestModelsCatalogProjectsCustomModelThroughRootBuildProcess
- TestModelsLocalRemoveMissingCacheRendersCodedDiagnostic
- TestModelsLocalRemoveMissingCacheMatchesHTTPDiagnostic
- TestModelsLocalInspectUnknownRendersCodedDiagnostic
- TestModelsLocalInspectUnknownMatchesHTTPDiagnostic
- TestDeliveredCLIArtifactReachesProtocolFixture
- TestDeliveredEmbedCLIArtifactReachesProtocolFixture
- TestDeliveredEmbedHTTPArtifactReachesProtocolFixture
- TestModelsGenericCLIProcessPublishesSingleOutputToStdoutOnly
- TestModelsGenericCLIProcessRollsBackMappedOutputsThroughEdges
- TestModelsGenericCLIProcessCancellationStopsReadinessAndPublishesNothing
- TestModelsGenericCLIProcessTimeoutStopsReadinessAndPublishesNothing
- TestModelsGenericCLIProcessRedactsCrashedHostDetails
- TestModelsJoinedBuiltinInvokeWithoutFactoryDeclaration
- TestModelsGenericHTTPInvocationReachesJoinedRootThroughProcess
- TestModelsNamedBuiltinRouteUsesEffectiveDefinitionWithoutWorker
- TestLocalAICLIConformanceMatrixRunsThroughRootBuildProcess
- TestLocalAIFailureDiagnosticsReachHTTPAndCLI
- TestLocalAIHTTPConformanceMatrixRunsThroughRootBuildProcess
- TestOmniProtocolTransportRoundTripsThroughNetworkDialer
- TestModelsPullToReadySurvivesProcessReconstruction
- TestModelsReadinessAssetsHostEffectsRemainInertThroughRootBuildProcess
- TestModelsReadinessAssetsHostActivateThroughRootBuildProcessAfterLifecycle
- TestModelsSharedProcessEligibleScenarios
- TestModelsPublicRemoveMissingCacheRendersServerDiagnostic
- TestModelsOmniTextInputReachesPinnedCodecThroughRootBuildProcess
- TestModelsPublicRemoveMissingCachePreservesJSONDiagnostic
- TestModelsOmniFileInputsPreserveDetectedTypesAndImageOrderThroughRootBuildProcess
- TestModelsPublicPullWorkflowProvesTruthfulTerminalState
- TestModelsPublicRemoveWorkflowProvesReclamationAndInUseRefusal
- TestModelsEmbedRootCompositionBehavior
- TestModelsEmbedCacheMissThenHitAvoidsNetworkThroughRootBuildProcess
- TestModelsEmbedHTTPParityUsesTheSameFixtureThroughRootBuildProcess
- TestModelsOmniVideoCapabilityAndCancellationThroughRootBuildProcess
- TestModelsOmniCancellationReleasesHostAcrossRootProcesses
- TestModelsDirectTTSAliasEndToEndThroughRootBuildProcess

## Build-tag, platform, and skip facts

The default discovery was repeated with -tags=functionallong. The following
15 names are additional tagged tests, not hidden default tests:

| Package | functionallong-only tests |
| --- | --- |
| workers/inference | TestProviderCancellationTerminatesCompanionProcesses |
| workers/inference/claude | TestClaudeGoldenFullStreamTextSuccess; TestClaudeGoldenToolLifecycleAndSessionIdentity |
| workers/mock | TestServiceConfigOverrideAlignment_CustomerProcessScriptCommandRunner |
| workers/script | TestScriptWorkerDropsResourceTokensFromArgTemplates; TestScriptWorkstationDropsResourceTokensFromPromptTemplates; TestScriptWorkerOrdersMultipleInputsByWorkstationConfigWithResources; TestScriptWorkstationOrdersMultipleInputsByWorkstationConfigWithResources; TestWorkerPublicContractSmoke_CanonicalWorkerExecutesAndKeepsRuntimeOnlyFieldsPrivate |
| providers | TestScriptExecutor_RuntimeWorkerTimeoutFromLoadedConfigRequeuesAndRetriesOnLaterTick; TestTemplateTests_ScriptWrapClaudeResolvesWorkstationExecutionTemplates; TestTemplateTests_ScriptWrapCodexResolvesWorkstationExecutionTemplates; TestIntegrationSmoke_ScriptTimeoutCompanionRequeuesBeforeLaterCompletion |
| models/root_composition | TestRealLocalInference_OMNIVOICEModelInvokeAndDirectAPIProduceAudio; TestRecordedRealLocalModelEventDiagnostics |

Default-tag ignored-file facts relevant to cleanup/platform fidelity:

- workers/inference: process_cleanup_long_test.go,
  process_cleanup_process_unix_test.go.
- workers/inference/claude: golden_common_long_test.go,
  golden_success_test.go.
- workers/mock: service_config_override_alignment_long_test.go.
- workers/script: environment_long_test.go, execution_long_test.go,
  helpers_long_test.go.
- providers: cli_script_executor_timeout_long_test.go,
  cli_template_resolution_long_test.go,
  cli_timeout_cleanup_process_unix_test.go,
  cli_timeout_companion_smoke_long_test.go, helpers_long_test.go.
- providers/acp: shared_process_status_unix_test.go.
- models/root_composition: api_model_local_inference_long_test.go.

Observed platform/environment boundaries:

- Provider Sessions stream-abort test records
  os.Interrupt is not supported for child processes on Windows and skips.
- AGY live smoke is disabled unless YOU_AGY_LIVE_SMOKE=1; the live AGY
  executable is not a default local dependency.
- Long tests use support.SkipLongFunctional and/or explicit environment
  gates; their omission from default counts is intentional.
- Process-gone and Unix process-tree cells are unavailable on this Windows
  host where their test code requires Unix semantics.
- Provider Codex and packaged-script tests retain their existing executable,
  symlink, shebang, and PATH capability skips.

## Resource ownership ledger

The seven resource classes are application process, provider/command
subprocess, explicit Factory Session, stream, listener, temporary filesystem,
and runtime state. A static call-site census was used to locate owners; its
counts are source occurrences, not execution counts. Execution counts below
are reported only when the selected test or fixture emits an observable
counter.

| Profile | Acquired identity/count in representative characterization | Cleanup owner/result | Public behavior witness | Gap retained for later story |
| --- | --- | --- | --- | --- |
| W-ROOT | Root/process/session identities are acquired through the existing Workers root/test helpers; individual counts are not emitted by the selected tests. Controlled command and t.TempDir identities are test-owned. | Existing t.Cleanup, support.CleanupProcess, session close, and deferred stop/close paths run; selected public probes pass. No package-wide exact residue report exists yet. | Work/Event/Worker Session result, provider routing, typed failure, worktree, and output assertions. | Add a package-local exact process/session/path census before any repair; exercise assertion-failure cleanup in story 002. |
| W-SESSION | Concurrent/direct/remote cases acquire Factory Session, provider-route, stream, and sometimes HTTP identities; exact per-case counts are not emitted by the selected probes. | Existing session/stream/route close helpers and HTTP cleanup; selected cancellation/concurrency probes pass. | Session identity, continuation, route isolation, stream terminal state, and no-fallback assertions. | Exact route/stream/process report and forced-assertion child are not yet package-local. |
| W-CLI | Built CLI process and temporary invocation/package-staging identities; counts depend on the test's subprocess path and are not emitted uniformly. | Existing command wait/cleanup and test temp cleanup. | CLI help, output mode, exit code, attached-session, and public error assertions. | Full clean-room process/staging census is story 002/005 evidence. |
| W-HTTP | Root/API or HTTP server, remote Worker Session, stream, and temporary request identities; no exact aggregate emitted by the selected worker probe. | Existing server stop, session/stream close, and test cleanup. | HTTP observe/continue, disconnect survival, interrupt/cancel, and history assertions. | Listener and stream identity/count needs exact package-local reporting. |
| P-ROOT | Existing Providers shared fixture owns one root/API lifecycle boundary per package run, with provider routes/sessions and controlled command identities per case. | The fixture cleanup closes process/API, closes sessions, removes owned paths, and asserts one root/listener topology and zero active routes. Selected success/failure/cancel probes pass. | Provider output, typed failure, route, Work, Event, permission, and retry assertions. | Current full package retains one baseline assertion mismatch; no assertions were weakened. |
| P-ACP | ACP tests acquire a stdio child/connection and prompt/session identities where applicable; exact per-test process counts are not emitted. | ACP shutdown/cancel/restart/retire paths join/close process and connection; selected package suite passes. | ACP protocol failure classes, redaction, serialization, restart, no-replay, and cancellation assertions. | Unix status and repeat/race cleanup remain later gates. |
| P-AGY | Controlled AGY command/process and temporary output identities in default tests; live executable is not acquired. | Existing process/temp cleanup and explicit live-smoke skip. | Golden success/failure/timeout/cancellation and root mapping assertions. | Live provider and exact PID census are intentionally unproven. |
| P-OS | Provider/worker subprocess trees, worktrees, and platform-specific process identities. Exact Windows counts are not portable to Unix process-tree cases. | Existing process-tree cleanup and OS-specific helpers. | Timeout, crash, non-zero, worktree, and companion-process assertions where supported. | Unix-only child-tree proof remains CI-owned. |
| S-CLI | The selected session probe emitted root-builds=1 api-host-starts=1 cli-builds=0; CLI/session/stream identities are created by the session fixture and command cases. | Fixture reports active-provider-routes=0; selected normal and abort probes pass; the Windows interrupt case skips at its documented platform boundary. | CLI session output, Worker Session stream/replay, opening order, correlation, typed abort, and exit-code assertions. | Exact per-case stream/listener/session report and Unix interrupt execution remain later gates. |
| M-ROOT | The three-case representative run emitted root_builds=2 api_servers=1 httptest_servers=3 localai_starts=0 tcp_listeners=0 factory_roots=3 temp_dirs=4 host_starts=3 asset_http_calls=0 host_http_calls=0 local_http_calls=2; shared fixture counters were all zero. | Model characterization TestMain records host/cache/path counters; selected activation, redaction, and cancellation probes pass. | Catalog/readiness, redacted host failure, cancellation/no-publication, and root composition assertions. | Full package is affected by an operator-local cache state; exact per-case lease/stream report remains story 004/005. |
| 0 | No test identity or owned resource is acquired by the zero-test package. | Not applicable. | Discovery fact only. | Keep package in denominator and do not invent a cleanup result. |

### Resource-class coverage by representative outcome

| Family | Normal probe | Failure probe | Cancellation probe | Cleanup result and gap |
| --- | --- | --- | --- | --- |
| Workers | TestScriptWorkerSharedSuccessSpine passed all nine scenario subtests. The shared fixture and scenario Work/session identities are owned by the existing worker helpers; exact root/PID counts are not emitted. | TestProviderNonZeroExitMapsToPublicFailure passed with the existing public non-zero failure witness; controlled provider process identity is owned by the inference helper. | TestDirectWorkerSessionRemoteInvokeCallerCancellationThroughRootProcess passed with the existing caller-cancellation witness; route, stream, session, and process identities are helper-owned. | Public probes pass and helper cleanup runs. Exact seven-class zero-residue evidence and forced assertion failure are retained as story 002 gaps. |
| Providers | TestScriptExecutor_Success passed with the existing provider fixture and successful script witness. | TestScriptExecutor_Failure passed with the existing failure routing witness. | TestScriptExecutor_CommandCancellationIsReported passed with the existing cancellation witness. | Shared fixture cleanup is an existing exact route/root/listener assertion; per-case process/session/path counts are not emitted. The current mixed mock-work failure is retained below. |
| Provider Sessions CLI | TestWorkerSessionsCLI passed its normal CLI/session matrix; fixture output recorded one root build, one API host, zero CLI builds, and zero active routes at teardown. | TestWorkerSessionsStreamAbortReturnsTypedDiagnosticThroughRootProcess passed its typed abort witness. | TestBuiltWorkerSessionsStreamCancellationExits130 reached the documented Windows skip because os.Interrupt is unsupported for child processes on this platform. | Session fixture cleanup passed; the skipped Unix cancellation execution and exact per-case stream/listener report remain unproven. |
| Models | TestModelsReadinessAssetsHostActivateThroughRootBuildProcessAfterLifecycle passed; the aggregate three-case ledger recorded the exact counts above. | TestModelsGenericCLIProcessRedactsCrashedHostDetails passed with the existing redaction witness. | TestModelsGenericCLIProcessCancellationStopsReadinessAndPublishesNothing passed with no-publication/cancellation witness. | Model TestMain emitted host/path/root counters and selected cleanup completed. Full package cache-state diagnostic and exact per-case lease report remain below/later. |

### Existing forced-assertion witness audit

tests/functional/providers/cli_timeout_cleanup_smoke_test.go contains the
package-local TestProviders_ForcedAssertionFailureCleansOwnedResources child
process characterization. It intentionally calls t.Fatal after acquiring
process/session/route/path resources; the parent verifies a non-zero child,
joined process, closed listener, one opened/deleted session, zero active
routes, and absent owned paths. This is a passing pre-repair witness for the
Providers family.

No analogous package-local forced-assertion child census was found in the
Workers, Provider Sessions CLI, or Models root-composition packages. That is a
characterized evidence gap, not a demonstrated production defect. It is
retained for the owning repair stories; no shared support or production code
was changed to manufacture a result in this discovery story.

## Representative probes

All commands used -count=1 and -timeout=30m unless stated otherwise. They
cross the local root/process/session boundaries and use the package's
controlled command/protocol edges; no paid or remote provider was used.

### Workers

    go test -count=1 -timeout=30m -run '^(TestScriptWorkerSharedSuccessSpine|TestProviderNonZeroExitMapsToPublicFailure|TestDirectWorkerSessionRemoteInvokeCallerCancellationThroughRootProcess)$' ./tests/functional/workers/...

Result: exit 0. The script success spine's nine child scenarios and the
selected provider failure and caller-cancellation witnesses passed. Packages
without one of the selectors reported no tests to run. This proves the
representative public outcome paths and their existing helper cleanup run; it
does not prove an exact seven-class residue census.

### Providers

    go test -count=1 -timeout=30m -run '^(TestScriptExecutor_Success|TestScriptExecutor_Failure|TestScriptExecutor_CommandCancellationIsReported)$' ./tests/functional/providers/...

Result: exit 0. The normal, failure, and command-cancellation witnesses
passed. The package's shared fixture cleanup remains the authority for root,
listener, route, and owned-path cleanup; per-case counts are a later evidence
addition.

### Provider Sessions CLI

    go test -count=1 -timeout=30m -run '^(TestWorkerSessionsCLI|TestWorkerSessionsStreamAbortReturnsTypedDiagnosticThroughRootProcess|TestBuiltWorkerSessionsStreamCancellationExits130)$' ./tests/functional/provider_sessions/cli

Result: exit 0. CLI and typed stream-abort witnesses passed; the built CLI
cancellation case recorded the exact Windows unsupported-interrupt skip. The
fixture emitted:

    C06 TASK-002 topology: root-builds=1 api-host-starts=1 cli-builds=0
    C06 TASK-002 cleanup: active-provider-routes=0

### Models

    go test -count=1 -timeout=30m -run '^(TestModelsReadinessAssetsHostActivateThroughRootBuildProcessAfterLifecycle|TestModelsGenericCLIProcessRedactsCrashedHostDetails|TestModelsGenericCLIProcessCancellationStopsReadinessAndPublishesNothing)$' ./tests/functional/models/root_composition

Result: exit 0. The lifecycle activation, crashed-host redaction, and
cancellation/no-publication witnesses passed. The characterization ledger
emitted:

    CHAR-001 ledger root_builds=2 api_servers=1 httptest_servers=3 localai_starts=0 tcp_listeners=0 factory_roots=3 temp_dirs=4 host_starts=3 asset_http_calls=0 host_http_calls=0 local_http_calls=2
    C06-002 shared_models_root_builds=0 shared_models_api_starts=0 shared_models_session_opens=0 shared_models_session_closes=0

## Current-head full-suite characterization

These are pre-repair diagnostics, not reasons to change assertions in this
story.

| Command | Result | Exact retained diagnostic | Disposition |
| --- | --- | --- | --- |
| go test -count=1 -timeout=30m ./tests/functional/workers/... | Exit 1 | The Workers lifecycle package encountered operator-local packaged-factory staging-owner contention. The same attached-session selector passed when rerun in isolation. | ENV-001: no repair; shared/operator state is outside this story. |
| go test -count=1 -timeout=30m ./tests/functional/providers/... | Exit 1 | TestMockWorkers_EndToEndSmokeRunsMixedOutcomesWithoutLiveProviderCredentials at tests/functional/providers/mock_workers_end_to_end_smoke_test.go:80 observed failure reason "unknown" where the existing witness expects "neutral terminal refusal". | BASE-001: retain exact assertion; production/runtime classification is outside this story and no assertion was weakened. |
| go test -count=1 -timeout=30m ./tests/functional/provider_sessions/cli | Exit 0 | Package passed; documented Windows interrupt skip remains. | Pass for story characterization. |
| go test -count=1 -timeout=30m ./tests/functional/models/root_composition | Exit 1 | TestModelsCatalogCLIProjectsFactoryDiscoveryThroughRootBuildProcess at tests/functional/models/root_composition/catalog_cli_test.go:79 observed an operator-local built-in model in INSTALLED/READY state where the baseline expects NOT_INSTALLED/MISSING. | ENV-002: operator cache/state diagnostic; no cache deletion or production change. |

The full Workers and Models runs also emitted noisy local fixture counters;
they are not substituted for clean-room evidence. The source and test
artifacts retain the exact ownership paths and the focused probes above supply
the stable current-head behavioral characterization.

## Excluded owners and structured deltas

| ID | Boundary | Evidence | Required disposition |
| --- | --- | --- | --- |
| ENV-001 | Operator-local packaged-factory staging owner affected one Workers lifecycle full-suite run. | Focused attached-session selector passed; no source assertion identified as failing in isolation. | Do not edit operator state, shared support, or production code in story 001. Recheck only under the later package/clean-room gate. |
| BASE-001 | Providers mixed mock-worker terminal failure classification. | Exact current-head test failure at line 80; focused normal/failure/cancel probes pass. | Preserve the public assertion and report to the owning Providers/runtime review; story 003 may diagnose but may not weaken it. |
| ENV-002 | Models catalog sees pre-existing operator-local ready/installed cache state. | Exact current-head failure at line 79; focused lifecycle/failure/cancellation probes pass. | Do not remove cache or change model readiness behavior in story 001. Story 004 must separate cache state from teardown. |
| PLATFORM-001 | Windows cannot execute Unix interrupt/process-tree cells. | Exact documented skip messages and ignored Unix files above. | Leave platform behavior unchanged; Unix/race evidence belongs to package CI. |
| GAP-001 | Workers, Provider Sessions, and Models lack an explicit forced-assertion cleanup child census. | Static package-local audit found the Providers witness only. | Retain this characterization before repair; add only within the owning later story if still justified. |

No shared-support, production, C01 inventory, remote-provider, paid-validation,
or excluded-surface files were changed. No test assertion, skip, timeout,
fixture policy, or public behavior was changed.

## Evidence boundary and handoff

This ledger proves:

- the exact current-head package/test denominator and zero-test packages;
- the 15 functionallong additions and relevant ignored/platform facts;
- representative normal, failure, and cancellation public witnesses for each
  family, with the acquired identity/count when the fixture emits it;
- the current resource owner and cleanup mechanism for all seven classes;
- the existing Providers forced-assertion witness and the missing witness gap
  for the other three families; and
- the exact baseline/environment diagnostics that later stories must not hide
  by weakening assertions or editing excluded owners.

It does not prove all outcome rows, three-run repeat, race instrumentation,
cross-family clean-room convergence, terminal CI, or merged delivery. Those
edges remain assigned to WORKERS-003, PROVIDERS-004, MODELS-005,
REPEAT-007, RACE-008, VAL-013, and PR-CI-012 as listed in the PRD.
