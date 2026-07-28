# Functional Test File Checklist

## How to use this checklist

Each checkbox is one independently assignable work cell and names the expected
destination file. The listed `Test*` functions are the minimum scenarios for
that file. Existing tests that already prove a scenario should be moved and
strengthened rather than duplicated.

Every cell must:

- use only public/root-built functional boundaries;
- include customer-readable Go doc comments;
- avoid wall-clock sleeps when a deterministic edge or observation is
  available;
- test the named happy path and failure path;
- update the generated catalog metadata automatically;
- run the focused destination package and applicable boundary check.

Destination folders follow the domain tree in [`plan.md`](plan.md). Transport
owns mechanics only; domain semantics live under the matching domain even when
the test enters through CLI, HTTP, or MCP.

Priority order is Wave 1, Wave 2, then Wave 3. Cells within a section are
intentionally small enough to distribute across many agents.

## Wave 1 — transport

### CLI process contract

- [x] `tests/functional/transport/cli/process/help_and_version_test.go`
  - `TestCLIHelpListsPublicCommandFamilies` verifies root help includes the
    supported public command families without hidden/internal commands.
  - `TestCLISubcommandHelpUsesStableUsageAndExitZero` verifies representative
    nested help text and successful exit.
  - `TestCLIVersionWritesOneMachineReadableVersion` verifies version output is
    stdout-only and contains no startup noise.

- [x] `tests/functional/transport/cli/process/unknown_command_test.go`
  - `TestCLIUnknownCommandWritesActionableStderr` verifies the invalid token is
    named and suggestions are customer-safe.
  - `TestCLIUnknownCommandReturnsUsageExitCode` verifies stdout remains empty
    and the process returns the documented non-success code.

- [x] `tests/functional/transport/cli/process/stdin_test.go`
  - `TestRunReadsPromptFromStdin` verifies `you run -` consumes stdin and sends
    the exact value to the selected worker.
  - `TestSubmitBatchReadsJSONFromStdin` verifies `you submit batch -` consumes
    one canonical Work Request batch.
  - `TestCLIEmptyRequiredStdinFailsWithoutDispatch` verifies EOF/empty input is
    rejected before any external worker effect.

- [x] `tests/functional/transport/cli/process/stdout_stderr_test.go`
  - `TestCLISuccessWritesPrimaryResultOnlyToStdout` verifies success data is
    not mixed with diagnostics.
  - `TestCLIFailureWritesDiagnosticToStderr` verifies stdout does not contain a
    false primary result.
  - `TestCLIQuietModeSuppressesNonResultNoise` verifies quiet output remains
    script-safe.

- [x] `tests/functional/transport/cli/process/exit_codes_test.go`
  - `TestCLIValidationFailureExitCode` covers invalid customer input.
  - `TestCLIWorkerFailureExitCode` covers a terminal worker failure.
  - `TestCLIInterruptedExitCode` covers cancellation/interruption.
  - `TestCLISuccessExitCode` covers normal quiescence.

- [x] `tests/functional/transport/cli/process/context_cancellation_test.go`
  - `TestCLIContextCancellationStopsExternalWork` verifies the injected
    provider process is cancelled.
  - `TestCLIContextCancellationEmitsNoSuccessResult` verifies the terminal
    public outcome is interrupted rather than completed.

### CLI parameter contract

- [x] `tests/functional/transport/cli/parameters/positional_values_test.go`
  - `TestRunAcceptsOnePositionalPrompt` verifies spaces and Unicode survive.
  - `TestRunRejectsExtraPositionalValues` verifies no worker dispatch occurs.
  - `TestOptionalSessionIDUsesDefaultWhenOmitted` verifies default session
    targeting and explicit override.

- [x] `tests/functional/transport/cli/parameters/flags_test.go`
  - `TestCLIStringBooleanAndRepeatedFlagsReachRequest` verifies flag mapping at
    the external observation edge.
  - `TestCLIUnknownFlagFailsBeforeLifecycleStart` verifies stable diagnostics.
  - `TestCLIFlagAfterPositionalValueUsesDocumentedParsing` guards ordering.

- [x] `tests/functional/transport/cli/parameters/key_value_test.go`
  - `TestRunKeyValueParametersReachFactoryInvocation` covers repeated
    `key=value` inputs.
  - `TestRunKeyValuePreservesEqualsInValue` covers URLs and encoded values.
  - `TestRunDuplicateKeyUsesDocumentedPrecedence` covers duplicate input.
  - `TestRunMalformedKeyValueFailsWithoutDispatch` covers missing key/value.

- [x] `tests/functional/transport/cli/parameters/json_values_test.go`
  - `TestCLIJSONParameterPreservesNestedObjectAndArray` verifies typed payload
    mapping.
  - `TestCLIInvalidJSONParameterNamesTheParameter` verifies actionable error
    output.
  - `TestCLIJSONNullAndEmptyValuesRemainDistinct` prevents normalization loss.

- [x] `tests/functional/transport/cli/parameters/environment_precedence_test.go`
  - `TestCLIExplicitFlagOverridesEnvironmentDefault` verifies precedence.
  - `TestCLIEnvironmentOverridesGlobalConfig` verifies documented precedence.
  - `TestCLIUnsetEnvironmentFallsBackWithoutFabricatingValue` covers absence.

- [x] `tests/functional/transport/cli/parameters/working_directory_test.go`
  - `TestCLIRelativeFactoryPathResolvesFromInvocationDirectory` verifies
    customer working-directory behavior.
  - `TestCLIWorkingDirectoryDoesNotLeakIntoOutput` verifies portable output.
  - `TestCLIMissingWorkingDirectoryAssetFailsActionably` covers failure.

- [x] `tests/functional/transport/cli/parameters/working_directory_long_test.go`
  - `TestCLIProviderExecResolvesWorkdirAgainstFactoryRuntimeRoot` verifies
    provider exec resolves workdir against the Factory runtime root.

### CLI output modes and streaming

- [x] `tests/functional/transport/cli/output/json_result_test.go`
  - `TestCLIJSONSuccessDecodesToPublicInvocationResult` verifies schema and
    terminal status.
  - `TestCLIJSONFailureRemainsValidJSON` verifies structured failure output.
  - `TestCLIJSONContainsNoPrivateRuntimeFields` guards the public boundary.
  - `TestCLIJSONOutputSelectionFailsBeforeProductActivation` verifies invalid
    output selectors fail before product activation.

- [x] `tests/functional/transport/cli/output/ndjson_stream_test.go`
  - `TestCLINDJSONEmitsDecodableResponseEventsThenInvocationResult` verifies
    record order and final record type.
  - `TestCLINDJSONSequenceIsMonotonic` verifies event ordering.
  - `TestCLINDJSONFailureEndsWithOneTerminalResult` guards duplicate terminal
    records.

- [x] `tests/functional/transport/cli/output/text_stream_test.go`
  - `TestCLITextStreamSurfacesIncrementalMessages` covers streaming providers.
  - `TestCLITextStreamDoesNotPrintStructuredEnvelopeNoise` covers human mode.
  - `TestCLITextStreamInterruptedRunDoesNotClaimCompletion` covers failure.
  - `TestCLITextStreamOperatorContinuousRunReportsStartupOutputWithoutQuiet`
    covers non-quiet operator continuous startup reporting.

- [x] `tests/functional/transport/cli/output/stream_backpressure_test.go`
  - `TestCLISlowWriterDoesNotReorderResponseEvents` uses a controlled blocking
    writer.
  - `TestCLIWriterFailureCancelsInvocation` verifies output failure cleanup.

### CLI thin command wiring

These files prove command wiring and exit behavior only. Domain depth lives
under `work/`, `sessions/`, `factory/`, and `product/`.

- [x] `tests/functional/transport/cli/commands/run_wiring_test.go`
  - `TestCLIRunFactoryByPath` covers an authored Factory path.
  - `TestCLIRunNamedFactory` covers packaged/named Factory resolution.
  - `TestCLIRunInvalidFactoryReturnsValidationFailure` covers load failure.

- [x] `tests/functional/transport/cli/commands/submit_wiring_test.go`
  - `TestCLISubmitBatchInlineJSON` covers inline canonical batch input.
  - `TestCLISubmitBatchFile` covers a file path.
  - `TestCLISubmitUnavailableServer` covers connection failure and exit code.
  - `TestCLISubmitBackendErrorPreservesPublicMessage` covers error mapping.

- [x] `tests/functional/transport/cli/commands/work_wiring_test.go`
  - `TestCLIWorkListAndShowReflectSubmittedWork` covers public read models.
  - `TestCLIWorkMoveChangesState` covers manual recovery/move.
  - `TestCLIWorkShowMissingReturnsNotFound` covers error behavior.
  - `TestCLIWorkVisualizeProducesDeterministicGraph` covers visualization.

- [x] `tests/functional/transport/cli/commands/session_wiring_test.go`
  - `TestCLISessionCreateListShowDelete` covers the basic lifecycle.
  - `TestCLISessionPauseBuffersAndResumeDispatches` covers lifecycle control.
  - `TestCLISessionMissingIDReturnsNotFound` covers show/delete failure.

- [x] `tests/functional/transport/cli/commands/factory_wiring_test.go`
  - `TestCLIFactoryInitValidateAndQuery` covers generated configuration.
  - `TestCLIFactoryFlattenExpandPreservesMeaning` covers portability.
  - `TestCLIFactoryReplaceCurrentChangesSessionFactory` covers current Factory.

- [x] `tests/functional/transport/cli/commands/docs_wiring_test.go`
  - `TestCLIDocsListsPackagedTopics` covers discovery.
  - `TestCLIDocsEveryTopicRendersNonEmptyContent` iterates the packaged index.
  - `TestCLIDocsUnknownTopicReturnsActionableFailure` covers errors.

### HTTP server and protocol mechanics

- [x] `tests/functional/transport/http/server/startup_shutdown_test.go`
  - `TestAPIServerStartsOnConfiguredListenerAndServesStatus`.
  - `TestAPIServerShutdownClosesListenerAndActiveStreams`.
  - `TestAPIServerBindFailureUnwindsStartedLifecycleRoles`.

- [x] `tests/functional/transport/http/server/routing_test.go`
  - `TestAPIRoutesEveryOpenAPIOperationToNon404Handler` uses the operation
    inventory with safe requests.
  - `TestAPIUnknownRouteReturnsStructuredNotFound`.
  - `TestAPIWrongMethodReturnsDocumentedMethodError`.

- [x] `tests/functional/transport/http/server/content_negotiation_test.go`
  - `TestAPIJSONRequestsAndResponsesUseDocumentedContentType`.
  - `TestAPIUnsupportedContentTypeReturns415`.
  - `TestAPIMalformedJSONReturnsStructured400`.

- [x] `tests/functional/transport/http/server/generated_client_test.go`
  - `TestGeneratedClientStatusAndSessionRoundTrip`.
  - `TestGeneratedClientDecodesRepresentativeStructuredError`.
  - `TestGeneratedClientAndServerSchemaStayAligned`.

- [x] `tests/functional/transport/http/server/concurrent_requests_test.go`
  - `TestAPIConcurrentSessionRequestsRemainIsolated`.
  - `TestAPICancelledRequestDoesNotCancelUnrelatedSession`.

- [x] `tests/functional/transport/http/status/status_test.go`
  - `TestAPIStatusReportsReadyAfterStartup`.
  - `TestAPIStatusDoesNotLeakInternalConfiguration`.

### MCP transport

- [x] `tests/functional/transport/mcp/stdio/discovery_test.go`
  - `TestMCPStdioInitializeAndToolDiscovery`.
  - `TestMCPDiscoveryContainsCanonicalFactorySessionTools`.
  - `TestMCPUnknownToolReturnsProtocolError`.
  - `TestMCPStdioRuntimeRejectsMissingHomeEnvironment`.
  - `TestMCPStdioRuntimeRejectsInvalidRuntimeProjectRoot`.
  - `TestMCPStdioFixtureAndRuntimePathsReachInitializer`.

- [x] `tests/functional/transport/mcp/protocol/errors_test.go`
  - `TestMCPMalformedParametersReturnInvalidParams`.
  - `TestMCPMissingFactorySessionReturnsCanonicalNotFound`.
  - `TestMCPServerShutdownClosesStdioCleanly`.

## Wave 1 — workers

### Script workers

- [x] `tests/functional/workers/script/execution_test.go`
  - `TestScriptWorkerCompletesWithPublicPrimaryResult`.
  - `TestScriptWorkerNonZeroExitMapsToFailedOutcome`.
  - `TestScriptWorkerCancellationTerminatesChildProcess`.

- [x] `tests/functional/workers/script/environment_test.go`
  - `TestScriptWorkerReceivesDeclaredEnvironmentOnly`.
  - `TestScriptWorkerMissingExecutableFailsActionably`.

- [x] `tests/functional/workers/transports/cli/run/help/invocation_help_test.go`
  - `TestCLIRunHelpShowsInvocationSignatureForNamedFactory` verifies run --help
    prints invocation signature for a named Factory.
  - `TestCLIRunHelpDistinguishesRequiredAndOptionalParameters` verifies required
    vs optional parameters are visibly distinguished.
  - `TestCLIRunHelpDoesNotDispatchExternalWork` verifies help is read-only and
    does not invoke provider command execution.

- [x] `tests/functional/workers/transports/cli/run/lifecycle/lifecycle_test.go`
  - `TestCLIRunCleanInvocationCompletesWithoutDashboardStartup` verifies clean
    invocation completes without dashboard startup.
  - `TestCLIRunServerAttachedInvocationTargetsExistingFactorySession` verifies
    server-attached run targets an existing Factory Session.
  - `TestCLIRunCleanInvocationFailurePreservesPublicError` verifies clean
    invocation failure preserves the public error surface.

- [x] `tests/functional/workers/transports/cli/run/modes/output_modes_test.go`
  - `TestCLIRunSuccessPrimaryResultTextJSONAndNDJSON`.
  - `TestCLIRunFailureOmitsFalseSuccessPrimaryResult`.

- [x] `tests/functional/workers/transports/cli/run/lifecycle/lifecycle_test.go`
  - `TestCLIRunCleanInvocationCompletesWithoutDashboardStartup`.
  - `TestCLIRunCleanInvocationFailurePreservesPublicError`.
  - `TestCLIRunServerAttachedInvocationTargetsExistingFactorySession`.

### Inference workers — shared contract

- [x] `tests/functional/workers/inference/selection_test.go`
  - `TestExplicitProviderAndModelReachSelectedProviderEdge` covers selection.
  - `TestWorkerProviderOverridesGlobalDefault` covers precedence.
  - `TestUnknownProviderFailsBeforeProcessStart` covers validation.

- [x] `tests/functional/workers/inference/flags_test.go`
  - `TestProviderPermissionWorktreeAndModelFlagsMapToCommand` covers shared
    command metadata.
  - `TestUnsupportedProviderFlagReturnsCapabilityError` covers mismatch.

- [x] `tests/functional/workers/inference/failure_normalization_test.go`
  - `TestProviderNonZeroExitMapsToPublicFailure` covers generic process failure.
  - `TestProviderAuthRateLimitAndTimeoutRemainDistinct` covers error classes.
  - `TestProviderFailureRedactsPromptEnvironmentAndCredentials` covers safety.

- [x] `tests/functional/workers/inference/stream_fidelity_test.go`
  - `TestProviderFullStreamClaimsDeltasAndSnapshotsTruthfully`.
  - `TestProviderPartialStreamDoesNotFabricateMissingDeltas`.
  - `TestProviderSnapshotOnlyEmitsCompletedSnapshotsOnly`.
  - `TestProviderFinalOnlyEmitsTerminalMessageOnly`.

- [x] `tests/functional/workers/inference/process_cleanup_test.go`
  - `TestProviderTimeoutTerminatesChildProcessTree` covers cleanup.
  - `TestProviderCancellationTerminatesCompanionProcesses` covers cancellation.
  - `TestProviderSuccessWaitsForProcessAndStreamClosure` covers normal cleanup.

### Inference workers — golden-backed provider variants

- [x] `tests/functional/workers/inference/codex/golden_success_test.go`
  - `TestCodexGoldenTextAndToolSuccess` replays `codex/message-tool-success`.
  - `TestCodexGoldenDerivesProviderSessionAndResponseEvents` compares all public
    metadata goldens.

- [x] `tests/functional/workers/inference/codex/conductor_test.go`
  - `TestCodexConductorSuccessThroughRootBuildProcess` proves Codex execution through
    the product graph via `root.BuildProcess`.
  - `TestCodexCommandCancellationThroughRootBuildProcessIsCanonical` proves canonical
    cancellation through the shared process boundary.

- [x] `tests/functional/workers/inference/codex/worktree_workstation_test.go`
  - `TestCodexWorktreeWorkstationDispatch_MaterializesCheckoutAndOmitsCLIWorktreeFlag`
    proves named worktree checkout materialization and omits the CLI `--worktree`
    flag.

- [x] `tests/functional/workers/inference/codex/golden_failure_test.go`
  - `TestCodexGoldenStructuredFailure` replays a non-zero structured failure.
  - `TestCodexGoldenTimeoutHasNoFalseTerminalMessage` covers timeout.

- [x] `tests/functional/workers/inference/codex/conductor_test.go`
  - `TestCodexConductorSuccessThroughRootBuildProcess` proves successful Codex execution through the product graph.
  - `TestCodexCommandCancellationThroughRootBuildProcessIsCanonical` proves cancellation through the canonical conductor path.

- [x] `tests/functional/workers/inference/claude/golden_success_test.go`
  - `TestClaudeGoldenFullStreamTextSuccess` covers deltas and final snapshot.
  - `TestClaudeGoldenToolLifecycleAndSessionIdentity` covers tools/session.

- [x] `tests/functional/workers/inference/claude/conductor_test.go`
  - `TestClaudeConductorSuccessThroughRootBuildProcess` proves Claude execution through
    the product graph via `root.BuildProcess`.
  - `TestClaudeCommandCancellationThroughRootBuildProcessIsCanonical` proves canonical
    cancellation through the shared process boundary.

- [x] `tests/functional/workers/inference/claude/golden_failure_test.go`
  - `TestClaudeGoldenStructuredFailure` covers normalized error metadata.
  - `TestClaudeGoldenTimeoutClosesResponseStream` covers terminal closure.

- [x] `tests/functional/workers/inference/claude/conductor_test.go`
  - `TestClaudeConductorSuccessThroughRootBuildProcess` proves successful Claude execution through the product graph.
  - `TestClaudeCommandCancellationThroughRootBuildProcessIsCanonical` proves cancellation through the canonical conductor path.

- [x] `tests/functional/workers/inference/cursor/golden_success_test.go`
  - `TestCursorGoldenTextSuccessAndSessionIdentity` covers public metadata.
  - `TestCursorGoldenReadableProviderSessionDetails` covers detail lookup.

- [x] `tests/functional/workers/inference/cursor/golden_failure_test.go`
  - `TestCursorGoldenMalformedRecordReturnsStableDiagnostic` covers malformed-record diagnostics.
  - `TestCursorGoldenProcessFailureAndTimeoutRemainDistinct` covers process-failure versus timeout distinction.

- [x] `tests/functional/workers/inference/opencode/golden_test.go`
  - `TestOpenCodeGoldenStructuredSnapshotSuccess`.
  - `TestOpenCodeGoldenFinalOnlyFallback`.
  - `TestOpenCodeGoldenStructuredFailureAndTimeout`.

- [x] `tests/functional/workers/inference/gemini/golden_test.go`
  - `TestGeminiGoldenTextSuccess`.
  - `TestGeminiGoldenRateLimitAndStructuredFailure`.
  - `TestGeminiGoldenTimeout`.
  - `TestRootBuiltProcessExecutesThroughSharedSupport`.
  - `TestGeminiConductorSuccessThroughRootBuildProcess`.
  - `TestGeminiClassifierRejectsStructuredLabelThroughRootBuildProcess`.
  - `TestGeminiConductorPreservesConfiguredEnvironment`.
  - `TestGeminiConductorPreservesConfiguredSkipPermissions`.
  - `TestGeminiRejectsUnsupportedStructuredOutputBeforeProviderIO`.
  - `TestGeminiRejectsUnsupportedWorkingDirectoryBeforeProviderIO`.
  - `TestGeminiNativeFailureThroughRootBuildProcessIsSafe`.
  - `TestGeminiCommandCancellationThroughRootBuildProcessIsCanonical`.

- [x] `tests/functional/workers/inference/kiro/golden_test.go`
  - `TestKiroGoldenTextSuccess`.
  - `TestKiroGoldenAuthAndStructuredFailure`.
  - `TestKiroGoldenTimeout`.

- [x] `tests/functional/workers/inference/pi/golden_test.go`
  - `TestPiGoldenTextSuccess`.
  - `TestPiGoldenStructuredFailure`.
  - `TestPiGoldenTimeout`.

- [x] `tests/functional/workers/inference/agy/golden_test.go`
  - `TestAgyGoldenFinalOnlySuccess`.
  - `TestAgyGoldenStructuredFailure`.
  - `TestAgyGoldenTimeout`.

### Mock workers

- [x] `tests/functional/workers/mock/replacement_test.go`
  - `TestMockWorkersReplaceOnlyNamedChildren`.
  - `TestUnknownWorkerOverrideFailsActionably`.
  - `TestMockWorkerFailureReturnsStablePublicFailure`.

- [x] `tests/functional/workers/mock/service_config_override_alignment_test.go`
  - `TestServiceConfigOverrideAlignment_CustomerProcessSharesScriptAndProviderCommandRunner`.

- [x] `tests/functional/workers/mock/service_config_override_alignment_long_test.go`
  - `TestServiceConfigOverrideAlignment_CustomerProcessScriptCommandRunner`.

## Wave 1 — orchestration

### JavaScript / TypeScript loading and invocation

- [x] `tests/functional/orchestration/javascript/loading/inline_javascript_test.go`
  - `TestInlineJavaScriptFactoryRunsFromCLI` covers an inline definition.
  - `TestInlineJavaScriptSyntaxErrorReturnsSourceLocation` covers load failure.

- [x] `tests/functional/orchestration/javascript/loading/file_javascript_test.go`
  - `TestJavaScriptFactoryFileRunsRelativeImportsFromFactoryRoot` covers path
    resolution.
  - `TestJavaScriptFactoryMissingImportFailsActionably` covers load failure.

- [x] `tests/functional/orchestration/javascript/loading/named_factory_test.go`
  - `TestNamedJavaScriptFactoryRunsThroughStandardCLI` covers named resolution.
  - `TestNamedJavaScriptFactoryRunsThroughAPIInvocation` covers HTTP entry.
  - `TestNamedJavaScriptFactoryUsesSameFactorySessionControls` covers pause.

- [x] `tests/functional/orchestration/javascript/loading/typescript_test.go`
  - `TestTypeScriptFactoryTranspilesAndRuns` covers supported syntax.
  - `TestTypeScriptTypeOrSyntaxFailureReturnsCustomerDiagnostic` covers error.
  - `TestTypeScriptSourceMapReportsAuthoredLocation` covers mapped location.

### JavaScript composition primitives

- [x] `tests/functional/orchestration/javascript/composition/agent_test.go`
  - `TestJavaScriptAgentReturnsUnaryResult` covers one child dispatch.
  - `TestJavaScriptAgentFailureReturnsStableFailureRecord` covers error.

- [x] `tests/functional/orchestration/javascript/composition/pipeline_test.go`
  - `TestJavaScriptPipelinePassesStageOutputToNextStage` covers data flow.
  - `TestJavaScriptPipelineStopsAfterStageFailure` prevents later dispatch.

- [x] `tests/functional/orchestration/javascript/composition/stages_test.go`
  - `TestJavaScriptNamedStagesExposeOrderedProgress` covers stage identity.
  - `TestJavaScriptEmptyStageProducesDocumentedResult` covers edge behavior.

- [x] `tests/functional/orchestration/javascript/composition/parallel_test.go`
  - `TestJavaScriptParallelDispatchesChildrenConcurrently` observes active
    external calls without sleeps.
  - `TestJavaScriptParallelPreservesDeclaredResultOrdering` covers determinism.
  - `TestJavaScriptParallelPartialFailureUsesDocumentedPolicy` covers error.

- [x] `tests/functional/orchestration/javascript/events/javascript_test.go`

- [x] `tests/functional/orchestration/javascript/composition/for_each_test.go`
  - `TestJavaScriptForEachDispatchesEveryInputOnce` covers cardinality.
  - `TestJavaScriptForEachPreservesInputResultCorrelation` covers identity.
  - `TestJavaScriptForEachEmptyInputDoesNotDispatch` covers empty input.

- [x] `tests/functional/orchestration/javascript/composition/nested_test.go`
  - `TestJavaScriptNestedPipelineParallelCompositionCompletes` covers nesting.
  - `TestJavaScriptNestedFailureNamesChildAndStage` covers diagnostics.

### JavaScript contracts, policy, and durability

- [x] `tests/functional/orchestration/javascript/contracts/input_mapping_test.go`
  - `TestJavaScriptInvocationReceivesStringNumberBooleanObjectAndArrayInputs`
    covers typed request mapping.
  - `TestJavaScriptMissingRequiredInputFailsBeforeChildDispatch` covers error.

- [x] `tests/functional/orchestration/javascript/contracts/output_mapping_test.go`
  - `TestJavaScriptReturnValueMapsToPrimaryInvocationResult` covers output.
  - `TestJavaScriptStructuredArtifactsMapToPublicResult` covers artifacts.
  - `TestJavaScriptUnsupportedReturnValueFailsWithoutPrivateVMDetails` covers
    error safety.

- [x] `tests/functional/orchestration/javascript/contracts/response_events_test.go`
  - `TestJavaScriptChildProgressPublishesCanonicalResponseEvents` covers
    message/tool progress.
  - `TestJavaScriptTerminalResultFollowsFinalResponseEvent` covers ordering.
  - `TestJavaScriptPhaseCheckpointLifecyclePublishesCanonicalFactoryEvents` covers
    phase and checkpoint event emission.

- [x] `tests/functional/orchestration/javascript/workers/overrides_test.go`
  - `TestJavaScriptChildrenSelectDifferentProvidersAndModels` covers per-child
    overrides.
  - `TestJavaScriptMockWorkersReplaceOnlyNamedChildren` covers partial mocks.
  - `TestJavaScriptUnknownWorkerOverrideFailsActionably` covers error.

- [x] `tests/functional/orchestration/javascript/policy/denied_operations_test.go`
  - `TestJavaScriptDeniedChildOperationReturnsStablePolicyDiagnostic` covers
    policy failure.
  - `TestJavaScriptPolicyFailureDoesNotDispatchExternalWork` covers safety.

- [x] `tests/functional/orchestration/javascript/durability/resume_test.go`
  - `TestJavaScriptInterruptedSessionResumesWithoutRepeatingCompletedChildren`
    covers durable progress.
  - `TestJavaScriptResumeRestoresCheckpointAndFinalResult` covers state.
  - `TestJavaScriptCorruptCheckpointFailsActionably` covers recovery failure.

### Petri / graph orchestration

- [x] `tests/functional/orchestration/petri/dispatch/simple_run_test.go`
  - `TestPetriSingleWorkerRunCompletesAtQuiescence`.
  - `TestPetriWorkerErrorReturnsFailedTerminalOutcome`.
  - `TestPetriInvocationInputAndOutputMapping`.

- [x] `tests/functional/orchestration/petri/dispatch/concurrent_workers_test.go`
  - `TestPetriIndependentWorkDispatchesConcurrently`.
  - `TestPetriConcurrentResultsCorrelateToOriginalWork`.
  - `TestPetriConcurrentFailureDoesNotDuplicateDispatch`.

- [x] `tests/functional/orchestration/petri/cross/session_compatibility_test.go`
  - `TestPetriAndJavaScriptSessionsShareLifecycleControls`.
  - `TestPetriAndJavaScriptSessionsExposeCompatibleStatusFacts`.

## Wave 1 — automations

- [x] `tests/functional/automations/cron_root_composition_test.go`
  - `TestBuildProcessRemainsCronInertBeforeRuntimeLifecycle`.
  - `TestAutomationsCronActivatesThroughRuntimeLifecycle`.
  - `TestAutomationsCronJitterProducesStableSubmissionTiming`.
  - `TestAutomationsCronSkipsMalformedWorkstationAndFiresValid`.
- [x] `tests/functional/automations/filesystem_watcher_root_composition_test.go`
  - `TestBuildProcessRemainsFilesystemWatcherInertBeforeRuntimeLifecycle`.
  - `TestAutomationsFilesystemWatcherPreseedsThroughRuntimeLifecycle`.

- [x] `tests/functional/automations/filesystem_watcher_root_composition_test.go`
  - `TestBuildProcessRemainsFilesystemWatcherInertBeforeRuntimeLifecycle`.
  - `TestAutomationsFilesystemWatcherPreseedsThroughRuntimeLifecycle`.
- [x] `tests/functional/automations/hosted_sources_root_composition_test.go`
  - `TestBuildProcessRemainsHostedSourcesInertBeforeRuntimeLifecycle`.
  - `TestAutomationsHostedSourcesActivateThroughRuntimeLifecycle`.

- [x] `tests/functional/automations/peer_import_boundary_test.go`
  - `TestFunctionalAutomationsPackageUsesPublicProcessImportsOnly`.

- [x] `tests/functional/automations/reconciliation_root_composition_test.go`
  - `TestBuildProcessRemainsReconciliationInertBeforeExplicitRootInvocation`.
  - `TestAutomationsReconciliationAdmitsThroughPublishedRootAfterComposition`.
  - `TestAutomationsReconcileAdmitsAbsentSourceThroughPublishedRootAfterComposition`.

- [x] `tests/functional/automations/script_poller_root_composition_test.go`
  - `TestBuildProcessRemainsScriptPollerInertBeforeRuntimeLifecycle`.
  - `TestAutomationsScriptPollerAdmitsWorkThroughRuntimeLifecycle`.

## Wave 1 — workstations

- [x] `tests/functional/workstations/execution/basic_test.go`
  - `TestExecutionWorkstationDispatchesEligibleWorkOnce`.
  - `TestExecutionWorkstationFailureProjectsPublicFailedState`.

- [x] `tests/functional/workstations/execution/contention_test.go`
  - `TestEligibleWorkstationContentionChoosesOneDispatchOnly`.
  - `TestContentionMakesProgressAcrossRepeatedWork`.

- [x] `tests/functional/workstations/cron/clock_test.go`
  - `TestCronFiresAtInjectedTimeWithoutWallClockSleep`.
  - `TestCronDoesNotDoubleFireForOneScheduleBoundary`.
  - `TestCronShutdownPreventsLaterSubmission`.
  - `TestCronImplicitFailureRoutingMovesFailedCronWorkIntoFailedState`.

- [x] `tests/functional/workstations/repeater/reject_accept_test.go`
  - `TestRepeater_YieldsBetweenIterations`.
  - `TestRepeater_ResourceReleaseBetweenIterations`.
  - `TestRalphLoop_ConvergesOnReviewerAccept`.
- [x] `tests/functional/workstations/repeater/reject_accept_long_test.go`
  - `TestRepeater_RefiresOnRejectedStopsOnAccepted`.
  - `TestRepeater_GuardedLoopBreakerTerminatesRejectedRepeater`.
  - `TestRepeater_ResourceReleaseBetweenIterations_ServiceHarness`.
  - `TestWorkstationStopWords_ThroughCustomerProcess`.
  - `TestMultiOutput_WithStopWord`.
  - `TestMultiOutput_WithoutStopWord`.
  - `TestMultiOutput_NoStopWordsConfigured`.
  - `TestMultiOutput_SecondStopWord`.
  - `TestMultiOutput_OutputTokensInheritInputLineage`.
  - `TestRalphLoop_TemplateFieldsResolvePerIteration`.

- [x] `tests/functional/workstations/poller/poller_test.go`
  - `TestPollerCreatesWorkFromExternalItems`.
  - `TestPollerEmptyResultCreatesNoWork`.
  - `TestPollerRecoverableFailureRetriesWithoutDuplicates`.

- [x] `tests/functional/workstations/poller/build_process_test.go`
  - `TestScriptPollerAutomationRemainsInertThroughRootBuildProcessConstruction`.

- [x] `tests/functional/workstations/watcher/files_test.go`
  - `TestWatcherSingleFileCompletesOneWork` verifies one watched seed file
    completes exactly one Work.
  - `TestWatcherSequentialFilesAllComplete` verifies sequential multi-file
    watcher admission.
  - `TestWatcherConcurrentFilesCompleteWithoutDuplicates` verifies concurrent
    admission without duplicate Work.
  - `TestWatcherMixedOutcomesLeaveNoNonTerminalWorkLeak` verifies mixed
    success/failure settlement leaves no non-terminal Work.
  - `TestWatcherDefaultChannelSubmission`, `TestWatcherExecutionIDDirectorySubmission`,
    and `TestWatcherCombinedDefaultAndDynamicExecDirectory` verify multi-channel
    watched-file ingress paths.
  - `TestWatcherParentChildBatchFanIn` verifies PARENT_CHILD batch fan-in via
    watched-file ingress.
  - `TestWatcherExecutionFollowsCurrentFactorySwitch` verifies watched-file
    execution follows Current Factory activation.

## Wave 2 — work

- [x] `tests/functional/work/submission/batch_inputs_test.go`
  - `TestWorkBatchAcceptsInlineFileAndStdinShapes`.
  - `TestWorkBatchSelectsDefaultAndExplicitWorkTypes`.
  - `TestWorkBatchRejectsUnknownTypeWithoutPartialMutation`.
  - `TestWorkBatchDependencyOrderingNormalizesRuntimeWork`.

- [x] `tests/functional/work/submission/batch_boundary_test.go`
  - `TestWorkBatchPublicShapeStaysAlignedAcrossWatchedFileAndHTTP`.

- [x] `tests/functional/work/submission/structured_submission_test.go`
  - `TestAPISubmitWorkAcceptsHeaderOnlyStructuredSubmission`.
  - `TestAPISubmitWorkRejectsEmptyStructuredSubmission`.
  - `TestAPISubmitWorkAcceptsOrderedTextSubmission`.
  - `TestAPISubmitWorkAcceptsCanonicalContentParts`.
  - `TestAPISubmitWorkAcceptsMixedTextAndImageOnSupportedRunner`.
  - `TestAPISubmitWorkRejectsMixedTextAndImageOnUnsupportedRunner`.
  - `TestAPISubmitWorkRejectsForgedStructuredFileReference`.

- [x] `tests/functional/work/submission/legacy_unary_test.go`
  - `TestLegacyUnaryRetirementSmoke_RuntimeSubmitPathsStayBatchOnly`.

- [x] `tests/functional/work/transports/cli/submit/batch_contract/batch_contract_test.go`
  - `TestCLISubmitBatchDryRunEmitsSummaryWithoutMutation` verifies dry-run
    summary without Work upsert.
  - `TestCLISubmitBatchSuccessHumanAndJSONShapes` verifies human and JSON success
    shapes through Process.Execute.
  - `TestCLISubmitBatchInvalidJSONFailsBeforeUpsert` verifies malformed JSON fails
    before any Work mutation.
  - `TestCLISubmitBatchContractHarnessExecutesThroughRootBuildProcess` verifies
    the Work-owned CLI batch contract cell uses root.BuildProcess + edges.Edges.

- [x] `tests/functional/work/transports/cli/submit/unary_contract/unary_contract_test.go`
  - `TestCLISubmitUnaryFileAndStdinReachWork` verifies file-backed and stdin
    unary submit reaches session-scoped Work through Process.Execute.
  - `TestCLISubmitUnaryDefaultAndExplicitSessionTargeting` verifies default and
    explicit `--session` targeting through public CLI contracts.
  - `TestCLISubmitUnaryStructuredFailurePreservesPublicMessage` verifies typed
    failure preservation against a controlled public HTTP edge.
  - `TestCLISubmitUnaryContractHarnessExecutesThroughRootBuildProcess` verifies
    the Work-owned CLI unary contract cell uses root.BuildProcess + edges.Edges.

- [x] `tests/functional/work/peer_import_boundary_test.go`
  - `TestFunctionalWorkPackageUsesPublicProcessImportsOnly`.
  - `TestWorkProductionPeersReachWorkThroughPublishedSurfacesOnly`.

- [x] `tests/functional/work/root_composition/build_process_inert_test.go`
  - `TestWorkEffectsRemainInertThroughRootBuildProcessConstruction`.

- [x] `tests/functional/work/root_composition/packaged_root_shape_test.go`
  - `TestWorkPackagedRootShapeMatchesCanonicalServiceLayout`.

- [x] `tests/functional/work/root_composition/recovery_recordings_visualization_activation_test.go`
  - `TestWorkRecordingsReadActivatesThroughRootBuildProcessAfterLifecycle`.
  - `TestWorkRecoveryActivatesThroughRootBuildProcessAfterLifecycle`.
  - `TestWorkVisualizationActivatesThroughRootBuildProcessAfterLifecycle`.

- [x] `tests/functional/work/root_composition/routing_relationship_activation_test.go`
  - `TestWorkRelationshipsActivateThroughRootBuildProcessAfterLifecycle`.
  - `TestWorkRoutingActivatesThroughRootBuildProcessAfterLifecycle`.

- [x] `tests/functional/work/root_composition/submission_activation_test.go`
  - `TestWorkSubmissionAndCLISubmitActivateThroughRootBuildProcessAfterLifecycle`.

- [x] `tests/functional/work/recordings/recordings_read_test.go`
  - `TestRecordingsBackedWorkReadsUseRecordingsRootContract`.
  - `TestGetWorkFromRecordingsRootUsesRecordingsServiceRoot`.
  - `TestRecordingsBackedWorkReadsMapRichWorldState`.
  - `TestRecordingsBackedWorkReadsSurfaceTypedProjectionFailures`.

- [x] `tests/functional/work/submission/http_test.go`
  - `TestAPIPOSTSubmitAndQueryWork`.
  - `TestAPIBatchUpsertAcceptsWorksContent`.
  - `TestCLIWorkTypeNameReachesLiveAPIHandler`.
  - `TestAPISubmitBatchThenListAndGetWork`.
  - `TestAPIUpsertWorkRequestUsesCanonicalIdentity`.
  - `TestAPIUnknownWorkReturnsTypedNotFound`.

- [x] `tests/functional/work/submission/stage_and_submit_test.go`
  - `TestAPIStageAndSubmitFileCreatesExpectedWork`.

- [x] `tests/functional/work/transports/cli/submit/batch_contract/batch_contract_test.go`
  - `TestCLISubmitBatchDryRunEmitsSummaryWithoutMutation`.
  - `TestCLISubmitBatchSuccessHumanAndJSONShapes`.
  - `TestCLISubmitBatchInvalidJSONFailsBeforeUpsert`.
  - `TestCLISubmitBatchContractHarnessExecutesThroughRootBuildProcess`.

- [x] `tests/functional/work/relationships/dependencies_test.go`
  - `TestDependentWorkWaitsForPrerequisiteTargetState`.
  - `TestDependentWorkDoesNotDispatchAfterPrerequisiteFailure`.
  - `TestFanInReleasesOnlyAfterEveryPrerequisite`.
  - `TestWorkWithoutDependsOnRelationsDispatchesNormally`.

- [x] `tests/functional/work/recordings/recordings_read_test.go`
  - `TestRecordingsBackedWorkReadsUseRecordingsRootContract`.
  - `TestGetWorkFromRecordingsRootUsesRecordingsServiceRoot`.
  - `TestRecordingsBackedWorkReadsMapRichWorldState`.
  - `TestRecordingsBackedWorkReadsSurfaceTypedProjectionFailures`.

- [x] `tests/functional/work/relationships/parent_child_test.go`
  - `TestParentChildLineageSurvivesDispatchAndReplay`.
  - `TestChildFailureProjectsToDocumentedParentView`.

- [x] `tests/functional/work/routing/logical_move_test.go`
  - `TestLogicalMoveCompletesWithoutWorkerDispatch`.
  - `TestLogicalMovePreservesWorkPayloadAndLineage`.
  - `TestLogicalMoveMultipleOutputsCreatesEveryExpectedWork`.

- [x] `tests/functional/work/routing/classifier_test.go`
  - `TestClassifierRoutesEveryKnownDecision`.
  - `TestClassifierUnknownAndMalformedDecisionFailDistinctly`.
  - `TestClassifierMultiOutputPreservesPayload`.

- [x] `tests/functional/work/recovery/manual_move_test.go`
  - `TestFailedCascadeCanBeRecoveredByPublicWorkMove`.
  - `TestTerminalFailedWorkCannotBeRedispatchedIllegally`.
  - `TestAPIMoveWorkResumesRecoverableFlow`.
  - `TestAPIInvalidMoveReturnsConflictWithoutMutation`.

- [x] `tests/functional/work/recordings/recordings_read_test.go`
  - `TestRecordingsBackedWorkReadsUseRecordingsRootContract`.
  - `TestGetWorkFromRecordingsRootUsesRecordingsServiceRoot`.
  - `TestRecordingsBackedWorkReadsMapRichWorldState`.
  - `TestRecordingsBackedWorkReadsSurfaceTypedProjectionFailures`.

- [x] `tests/functional/work/visualization/dependency_graph_test.go`
  - `TestWorkVisualizeProducesDeterministicGraph`.

- [x] `tests/functional/work/recordings/recordings_read_test.go`
  - `TestRecordingsBackedWorkReadsUseRecordingsRootContract`.
  - `TestGetWorkFromRecordingsRootUsesRecordingsServiceRoot`.
  - `TestRecordingsBackedWorkReadsMapRichWorldState`.
  - `TestRecordingsBackedWorkReadsSurfaceTypedProjectionFailures`.

## Wave 2 — sessions

- [x] `tests/functional/sessions/lifecycle/crud_test.go`
  - `TestFactorySessionCreateListShowDelete`.
  - `TestFactorySessionListMultipleSessions`.
  - `TestFactorySessionMissingShowAndDeleteFail`.
  - `TestAPIOpenListGetAndCloseFactorySession`.
  - `TestAPIFactorySessionNotFoundUsesTypedError`.
  - `TestAPIMultipleFactorySessionsRemainIsolated`.

- [ ] `tests/functional/sessions/live_runtime_build_process_test.go`
  - `TestBuildProcessRoutesLiveOpenListControlAndCloseThroughFactorySessionsRoot`.

- [x] `tests/functional/sessions/controls/pause_resume_test.go`
  - `TestPausedFactorySessionBuffersSubmittedWork`.
  - `TestPausedFactorySessionReturnsInvocationPausedStatus`.
  - `TestResumedFactorySessionDrainsBufferedWorkInOrder`.
  - `TestPauseResumeEmitsDurableLifecycleEvents`.
  - `TestAPIPauseResumeCancelAndTerminateFactorySession`.
  - `TestAPIInvalidLifecycleTransitionReturnsConflict`.

- [x] `tests/functional/sessions/execution/results_dispatches_test.go`
  - `TestAPIResultAndResultsExposeTerminalInvocationData`.
  - `TestAPIDispatchListAndDetailExposePublicCorrelation`.
  - `TestAPIPartialResultIsAvailableBeforeTerminalCompletion`.

- [x] `tests/functional/sessions/execution/visibility_test.go`
  - `TestCLIInvocationIsVisibleThroughAPISessionAndWorkReads`.
  - `TestAPIInvocationResultMatchesCLICompatibleFacts`.

- [x] `tests/functional/sessions/restart/logical_identity_test.go`
  - `TestFactorySessionRestartRemapsLiveIDToLogicalIdentity`.
  - `TestFactorySessionResumeDoesNotRepeatCompletedDispatch`.
  - `TestFactorySessionHistoryRemainsReadableAfterRestart`.

- [x] `tests/functional/sessions/mcp/controls_test.go`
  - `TestMCPPauseResumeAndCancelTargetCanonicalFactorySession`.
  - `TestMCPControlledSessionIsReadableThroughAPI`.
  - `TestMCPSynchronousFactorySessionReturnsTerminalResult`.

- [x] `tests/functional/sessions/root_composition/build_process_inert_test.go`
  - `TestSessionsEffectsRemainInertThroughRootBuildProcessConstruction`.

- [x] `tests/functional/sessions/root_composition/lifecycle_runtime_opening_test.go`
  - `TestSessionsLifecycleAndRuntimeOpeningActivateThroughRootBuildProcessAfterLifecycle`.

- [x] `tests/functional/sessions/root_composition/packaged_root_shape_test.go`
  - `TestSessionsPackagedRootShapeMatchesCanonicalServiceLayout`.

- [x] `tests/functional/sessions/root_composition/peer_import_seal_test.go`
  - `TestSessionsFunctionalProofsDoNotImportOwnerPrivatePackages`.
  - `TestSessionsProductionPeersReachSessionsThroughPublicSurfacesOnly`.
  - `TestSessionsRootCompositionConstructsThroughBuildProcess`.

- [x] `tests/functional/sessions/root_composition/work_admission_response_stream_test.go`
  - `TestSessionsWorkAdmissionAndResponseStreamActivateThroughRootBuildProcessAfterLifecycle`.

- [x] `tests/functional/sessions/root_composition/work_peer_import_seal_test.go`
  - `TestFactorySessionsProductionPackagesImportWorkRootOnly`.
  - `TestFactorySessionsProductionPackagesImportWorkersOnlyThroughRoot`.
  - `TestFactorySessionsProductionPackagesImportFactoryRuntimeOnlyThroughRoot`.
  - `TestSessionsFunctionalProofsDoNotImportRetiredWorkConsumerEdges`.
  - `TestMCPSynchronousFailureReturnsStructuredFailure`.
  - `TestMCPAsyncFactorySessionCanBePolledToSuccess`.
  - `TestMCPAsyncFactorySessionCanBePolledToFailure`.
  - `TestMCPAsyncCancellationIsVisibleThroughAPI`.

## Wave 2 — factory

### Definitions

- [x] `tests/functional/factory/definitions/init_test.go`
  - `TestFactoryInitCreatesRunnablePortableScaffold`.
  - `TestFactoryInitIsIdempotent`.
  - `TestFactoryInitFailureRoutingProducesFailedWork`.

- [x] `tests/functional/factory/definitions/validation_test.go`
  - `TestFactoryValidationAcceptsMultiWorkTypeExecutableTopology`.
  - `TestFactoryValidationRejectsMissingWorkerWorkstationAndRoute`.
  - `TestFactoryValidationReportsAllActionableDefinitionErrors`.
  - `TestAPIValidateFactoryAcceptsValidAndRejectsInvalidDefinitions`.
  - `TestAPIPreviewFactoryReturnsPublicTopology`.
  - `TestAPIPreviewDoesNotStartWorkersOrSessions`.

- [x] `tests/functional/factory/definitions/defaults_test.go`
  - `TestGlobalConfigSuppliesDefaultProviderAndModel`.
  - `TestExplicitFactoryConfigOverridesGlobalDefaults`.
  - `TestOperatorGlobalDefaultsAndWorkerPresetResolveAtProviderEdge`.
  - `TestSingleDiscoveredProviderIsUsedWhenNoDefaultExists`.

- [x] `tests/functional/factory/definitions/defaults_loaded_config_long_test.go`
  - `TestLoadedFactoryConfigDrivesProviderEdgePromptAndStopToken`.

- [x] `tests/functional/factory_definitions/transports/cli/named_lifecycle/named_lifecycle_test.go`
  - `TestCLIFactoryNamedCreateListUpdateDelete` verifies create, list, update, and
    delete for a named Factory through root.BuildProcess + Process.Execute.
  - `TestCLIFactoryListReflectsCreateAndDelete` verifies list membership after
    create and removal.
  - `TestCLIFactoryDeleteMissingReturnsActionableFailure` verifies delete of a
    missing named Factory returns an actionable failure without mutation.

- [x] `tests/functional/factory_definitions/transports/cli/yaml_parity/yaml_parity_test.go`
  - `TestCLIFactoryJSONAndYAMLValidateFlattenAndRunParity` verifies validate,
    flatten, and run parity across JSON, YAML, and YML file and directory sources.
  - `TestCLIFactoryYAMLCreateAndUpdateRemainRunnableAfterCanonicalPersistence`
    verifies YAML create and JSON update remain runnable after canonical
    persistence.
  - `TestCLIFactoryRejectedAuthoredSourcesFailBeforeRuntimeExecution` verifies
    rejected authored sources fail before provider/runtime execution with source
    context in diagnostics.

- [x] `tests/functional/factory_definitions/transports/cli/validate_persist/validate_persist_test.go`
  - `TestCLIFactoryValidateRejectsInvalidDefinitionActionably`.
  - `TestCLIFactoryValidateDoesNotMutateOnFailure`.
  - `TestCLIFactoryPersistFromFileThenRunSucceeds`.

- [x] `tests/functional/factory/definitions/import_export_test.go`
  - `TestExportedFactoryCanBeImportedAndRun`.
  - `TestImportExportPreservesNestedDocsScriptsAndMetadata`.
  - `TestInvalidImportDoesNotReplaceCurrentFactory`.

- [x] `tests/functional/factory/definitions/import_export_named_factory_test.go`
  - Named Factory create/upsert/replace-current import_export scenarios
    migrated from `runtime_api-delete-06-factory-current`.

- [x] `tests/functional/factory/definitions/validation_topology_test.go`
  - Current/Named Factory topology validation targets migrated from
    `runtime_api-delete-06-factory-current`.

- [x] `tests/functional/work/transports/cli/submit/batch_contract/batch_contract_test.go`
  - `TestCLISubmitBatchDryRunEmitsSummaryWithoutMutation`.
  - `TestCLISubmitBatchSuccessHumanAndJSONShapes`.
  - `TestCLISubmitBatchInvalidJSONFailsBeforeUpsert`.
  - `TestCLISubmitBatchContractHarnessExecutesThroughRootBuildProcess`.

- [x] `tests/functional/work/transports/cli/submit/unary_contract/unary_contract_test.go`
  - `TestCLISubmitUnaryFileAndStdinReachWork`.
  - `TestCLISubmitUnaryDefaultAndExplicitSessionTargeting`.
  - `TestCLISubmitUnaryStructuredFailurePreservesPublicMessage`.

- [x] `tests/functional/workers/transports/cli/run/help/invocation_help_test.go`
  - `TestCLIRunHelpShowsInvocationSignatureForNamedFactory`.
  - `TestCLIRunHelpDistinguishesRequiredAndOptionalParameters`.
  - `TestCLIRunHelpDoesNotDispatchExternalWork`.

- [x] `tests/functional/workers/transports/cli/run/modes/output_modes_test.go`
  - `TestCLIRunSuccessPrimaryResultTextJSONAndNDJSON`.
  - `TestCLIRunFailureOmitsFalseSuccessPrimaryResult`.

- [x] `tests/functional/workers/transports/cli/run/lifecycle/lifecycle_test.go`
  - `TestCLIRunCleanInvocationCompletesWithoutDashboardStartup`.
  - `TestCLIRunCleanInvocationFailurePreservesPublicError`.
  - `TestCLIRunServerAttachedInvocationTargetsExistingFactorySession`.

### Factory visualization

- [x] `tests/functional/factory_visualization/activation_lifecycle_test.go`
  - `TestVisualizationActivatesThroughPublicRootAfterLifecycle`.

- [x] `tests/functional/factory_visualization/inert_construction_test.go`
  - `TestVisualizationRemainsInertThroughRootBuildProcessConstruction`.

- [x] `tests/functional/factory_visualization/observe_live_view_test.go`
  - `TestVisualizationObserveThroughPublicRootAfterLifecycle`.

- [x] `tests/functional/factory_visualization/peer_import_boundary_test.go`
  - `TestFunctionalProofsImportOnlyPublishedVisualizationSurfaces`.
  - `TestProductionPeersReachVisualizationThroughPublishedSurfacesOnly`.

- [x] `tests/functional/factory_visualization/response_presentation_test.go`
  - `TestVisualizationResponsePresentationThroughPublicRootAfterLifecycle`.

### Factory runtime (service-mirrored Petri depth)

- [x] `tests/functional/factory_runtime/peer_import_boundary_test.go`
  - `TestFunctionalFactoryRuntimePackageUsesPublicProcessImportsOnly`.
  - `TestProductionPeersReachFactoryRuntimeThroughPublishedSurfacesOnly`.

- [x] `tests/functional/factory_runtime/root_composition/build_process_inert_test.go`
  - `TestFactoryRuntimeEffectsRemainInertThroughRootBuildProcessConstruction`.

- [x] `tests/functional/factory_runtime/root_composition/lifecycle_activation_test.go`
  - `TestFactoryRuntimeControlObservationAndDispatchPlanActivateThroughRootBuildProcessAfterLifecycle`.

- [x] `tests/functional/factory_runtime/root_composition/packaged_root_shape_test.go`
  - `TestFactoryRuntimePackagedRootShapeMatchesCanonicalServiceLayout`.

- [x] `tests/functional/factory_runtime/root_composition/workflow_orchestration_activation_test.go`
  - `TestFactoryRuntimeJavaScriptWorkflowActivatesThroughRootBuildProcessAfterLifecycle`.
  - `TestFactoryRuntimePetriOrchestrationActivatesThroughRootBuildProcessAfterLifecycle`.

- [x] `tests/functional/factory_runtime/orchestrators/petri/guards/eligibility_test.go`
  - `TestPetriAuthoredEligibilityGuardBlocksDispatchUntilSatisfied`.
  - `TestPetriParentOrSameNameGuardReleasesExpectedWork`.
  - `TestPetriVisitOrMatchGuardFailureIsVisibleInPublicWorkState`.

- [x] `tests/functional/factory_runtime/orchestrators/petri/routing/multi_transition_test.go`
  - `TestPetriMultiStagePipelineCompletesAtPublicTerminals`.
  - `TestPetriFailureRoutesToDocumentedFailedPlace`.
  - `TestPetriMultiTransitionPreservesWorkCorrelation`.

- [x] `tests/functional/factory_runtime/orchestrators/petri/guards/eligibility_test.go`
  - `TestPetriAuthoredEligibilityGuardBlocksDispatchUntilSatisfied`.
  - `TestPetriParentOrSameNameGuardReleasesExpectedWork`.
  - `TestPetriVisitOrMatchGuardFailureIsVisibleInPublicWorkState`.

### Packaged factories

- [x] `tests/functional/factory/packaged/catalog/discovery_test.go`
  - `TestPackagedFactoryCatalogListsEveryEmbeddedFactory` compares runtime
    discovery with the embedded package inventory.
  - `TestPackagedFactoryCatalogHasUniqueStableNames` rejects collisions.
  - `TestNewEmbeddedFactoryRequiresFunctionalMatrixEntry` prevents drift.

- [x] `tests/functional/factory/packaged/catalog/override_test.go`
  - `TestLocalFactoryOverridesPackagedFactoryWithSameName` covers precedence.
  - `TestInvalidLocalOverrideDoesNotFallBackSilently` covers misconfiguration.
  - `TestUnrelatedLocalFactoryDoesNotHidePackagedFactories` covers enumeration.

- [x] `tests/functional/factory/packaged/catalog/required_inputs_test.go`
  - `TestPackagedFactoriesRejectMissingRequiredInputs` runs the package matrix.
  - `TestPackagedFactoriesNameMissingInputAndFactory` verifies diagnostics.

- [x] `tests/functional/factory/packaged/deep_research/invocation_test.go`
  - `TestPackagedDeepResearchRequiredInputCompletes` verifies dispatch sequence
    and primary result with mock workers.
  - `TestPackagedDeepResearchOptionalInputsReachWorkers` covers overrides.
  - `TestPackagedDeepResearchWorkerFailureReturnsFailedOutcome` covers failure.

- [x] `tests/functional/factory/packaged/fusion/invocation_test.go`
  - `TestPackagedFusionRequiredInputCompletes` verifies its multi-worker merge.
  - `TestPackagedFusionOptionalInputsReachWorkers` covers supported options.
  - `TestPackagedFusionPartialWorkerFailureUsesDocumentedOutcome` covers error.

- [x] `tests/functional/factory/packaged/goal/invocation_test.go`
  - `TestPackagedGoalAcceptCompletesWithSummary` covers accepted routing.
  - `TestPackagedGoalContinueRepeatsThenCompletes` covers continue repeats.
  - `TestPackagedGoalRejectRepeatsThenCompletes` covers feedback propagation.
  - `TestPackagedGoalUnknownDecisionFails` covers classifier failure.
  - `TestPackagedGoalPausedSubmissionResumes` covers session control locally.

- [x] `tests/functional/factory/packaged/quorum/invocation_test.go`
  - `TestPackagedQuorumRequiredInputCompletes` covers member dispatch and final
    result.
  - `TestPackagedQuorumOptionalMemberSettingsReachWorkers` covers overrides.
  - `TestPackagedQuorumGatesMergeUntilBothBranchesComplete` covers merge gating.
  - `TestPackagedQuorumInsufficientSuccessfulMembersFails` covers failure.

- [x] `tests/functional/factory/packaged/review/invocation_test.go`
  - `TestPackagedReviewApprovalCompletes` covers first-pass approval.
  - `TestPackagedReviewRejectionCarriesFeedback` covers retry context.
  - `TestPackagedReviewRetryExhaustionFails` covers bounded failure.

- [x] `tests/functional/factory/packaged/subagent/invocation_test.go`
  - `TestPackagedSubagentReturnsChildResult` covers basic dispatch.
  - `TestPackagedSubagentStreamsChildResponseEvents` covers observation.
  - `TestPackagedSubagentChildFailureReturnsStableFailure` covers error.

- [x] `tests/functional/factory/packaged/tts/invocation_test.go`
- [x] `tests/functional/automations/peer_import_boundary_test.go`
- [x] `tests/functional/automations/reconciliation_root_composition_test.go`
- [x] `tests/functional/automations/script_poller_root_composition_test.go`
- [x] `tests/functional/models/root_composition/build_process_inert_test.go`
- [x] `tests/functional/models/root_composition/catalog_discovery_test.go`
- [x] `tests/functional/models/root_composition/inference_invoke_test.go`
- [x] `tests/functional/models/root_composition/peer_import_seal_test.go`
- [x] `tests/functional/models/root_composition/readiness_assets_host_test.go`
- [x] `tests/functional/runtime_api/api_javascript_sync_structured_input_test.go`
- [x] `tests/functional/runtime_api/api_multi_work_dispatch_smoke_test.go`
- [x] `tests/functional/runtime_api/api_work_root_policy_slices_test.go`
- [x] `tests/functional/runtime_api/api_work_service_application_slices_test.go`
- [x] `tests/functional/factory/definition_activation/gateway_wiring_test.go`
- [x] `tests/functional/models/model_invoke/http_workcontent_coverage_test.go`
- [x] `tests/functional/models/model_list/adapter_owned_coverage_test.go`
- [x] `tests/functional/models/model_list/presentation_collaborator_coverage_test.go`
- [x] `tests/functional/workers/inference/cursor/conductor_test.go`
- [x] `tests/functional/workers/inference/opencode/conductor_test.go`
  - `TestPackagedTTSRequiredInputProducesAudioArtifactMetadata` uses a fake
    model edge.
  - `TestPackagedTTSOptionalVoiceAndFormatReachModel` covers options.
  - `TestPackagedTTSModelFailureReturnsNoFalseArtifact` covers failure.

- [x] `tests/functional/factory/packaged/cross/package_cli_api_test.go`
  - `TestPackagedFactoryInvokedByCLICanBeInspectedByAPI` owns this package
    cross-surface state check.
  - `TestPackagedFactoryCLIAndAPIPrimaryOutcomeShapesAgree` compares only
    compatible public facts.

### Current factory

- [x] `tests/functional/factory/current/read_save_test.go`
  - `TestAPIGetAndSaveCurrentFactoryWithinOneSession`.
  - `TestAPISaveCurrentFactoryValidatesBeforePersistence`.
  - `TestAPICurrentFactoriesRemainSessionScoped`.
  - Current Factory docs/save/version/session scenarios migrated from
    `runtime_api-delete-06-factory-current`.

- [x] `tests/functional/factory/current/read_save_layout_test.go`
  - Portable layout accept/reject/preserve/prune and waypoint/size variants
    migrated from `runtime_api-delete-06-factory-current`.

- [x] `tests/functional/factory/current/read_save_layout_validation_test.go`
  - `TestCurrentFactoryPUT_PrePersistLayoutFailureRetainsStructuredPath`
    migrated from `runtime_api-delete-06-factory-current`.

- [x] `tests/functional/factory/current/read_save_long_test.go`
  - `TestCurrentFactoryActivationSwitchesPersistedFactories` verifies activation
    switches persisted factories and resolves the current factory.
  - `TestCurrentFactoryLiveAPIReadsFollowActivatedFactory` verifies live API
    reads follow the activated factory.
  - `TestCurrentFactoryWatchedFileExecutionFollowsActivatedFactory` verifies
    watched-file execution follows the activated factory.

- [x] `tests/functional/factory/current/prompt_template_test.go`
  - `TestAPIPromptTemplateContractAndValidationRoundTrip`.
  - `TestAPIInvalidPromptTemplateNamesMissingVariables`.
  - `TestAPITemplateValidationDoesNotMutateCurrentFactory`.

## Wave 2 — provider sessions

- [x] `tests/functional/providers/agy/process_harness_test.go`
  - `TestAgyConductorSuccessThroughRootBuildProcess`.
  - `TestAgyNativeFailureThroughRootBuildProcessIsSafe`.
  - `TestAgyTimeoutFailureThroughRootBuildProcess`.
  - `TestAgyCommandCancellationThroughRootBuildProcessIsCanonical`.
- [x] `tests/functional/providers/codex/process_harness_test.go`
  - `TestCodexHistoricalInspectionSuccessThroughRootBuildProcess`.
  - `TestCodexHistoricalInspectionDetachedRepeatedRunsThroughRootBuildProcess`.
  - `TestCodexHistoricalInspectionMissingSessionThroughRootBuildProcess`.
  - `TestCodexHistoricalInspectionMalformedJSONLThroughRootBuildProcess`.
  - `TestCodexHistoricalInspectionContainmentRejectionThroughRootBuildProcess`.
  - `TestCodexHistoricalInspectionBoundedWalkThroughRootBuildProcess`.
  - `TestCodexHistoricalInspectionCancelledDiscoveryThroughRootBuildProcess`.

- [x] `tests/functional/providers/gemini/process_harness_test.go`
  - `TestGeminiConductorSuccessThroughRootBuildProcess`.
  - `TestGeminiConductorPreservesConfiguredEnvironment`.
  - `TestGeminiNativeFailureThroughRootBuildProcessIsSafe`.
  - `TestGeminiCommandCancellationThroughRootBuildProcessIsCanonical`.

- [x] `tests/functional/providers/kiro/process_harness_test.go`
  - `TestKiroCommandCancellationThroughRootBuildProcessIsCanonical`.

- [x] `tests/functional/providers/agy/process_harness_test.go`
  - `TestAgyConductorSuccessThroughRootBuildProcess`.
  - `TestAgyNativeFailureThroughRootBuildProcessIsSafe`.
  - `TestAgyTimeoutFailureThroughRootBuildProcess`.
  - `TestAgyCommandCancellationThroughRootBuildProcessIsCanonical`.

- [x] `tests/functional/providers/pi/process_harness_test.go`
  - `TestPiStreamingSuccessThroughRootBuildProcess`.
  - `TestPiResumeContinuityThroughRootBuildProcess`.
  - `TestPiNativeFailureThroughRootBuildProcessIsSafe`.
  - `TestPiCommandCancellationThroughRootBuildProcessIsCanonical`.

- [x] `tests/functional/providers/cursor/historical_inspection_root_test.go`
  - `TestCursorHistoricalInspectionThroughRootBuildProcess_ReturnsDeterministicNormalizedDetail`.
  - `TestCursorHistoricalInspectionThroughRootBuildProcess_PropagatesMissingAndContainmentFailures`.
  - `TestCursorHistoricalInspectionThroughRootBuildProcess_DegradesAdverseNativeDataSafely`.

- [x] `tests/functional/provider_sessions/build_process_inert_test.go`
  - `TestProviderSessionsRemainInertThroughRootBuildProcessConstruction`.

- [x] `tests/functional/provider_sessions/peer_import_boundary_test.go`
  - `TestFunctionalProviderSessionsPackageUsesPublicProcessImportsOnly`.

- [x] `tests/functional/provider_sessions/details/codex_details_test.go`
  - `TestCodexProviderSessionDetailsLoadFromGoldenMetadata` covers API detail.
  - `TestCodexProviderSessionMissingTranscriptReturnsNotFound` covers absence.
  - `TestCodexProviderSessionCorruptTranscriptReturnsSafeDiagnostic`.

- [x] `tests/functional/provider_sessions/details/cursor_details_test.go`
  - `TestCursorProviderSessionDetailsLoadFromGoldenMetadata` covers readable
    transcript data.
  - `TestCursorProviderSessionUnavailableContentRemainsInspectable` covers
    partial data.
  - `TestCursorProviderSessionMissingIDReturnsNotFound`.

- [x] `tests/functional/provider_sessions/details/http_test.go`
  - `TestAPIProviderSessionDetailsUseGoldenExpectedMetadata`.
  - `TestAPIProviderSessionRejectsRawFilesystemPathInput`.
  - `TestAPIUnsupportedProviderSessionKindReturnsTypedError`.

- [x] `tests/functional/provider_sessions/association/association_test.go`
  - `TestProviderSessionRefAssociatesWithOwningDispatchAndFactorySession`.
  - `TestAbsentProviderSessionIsNotFabricated`.
  - `TestMultipleDispatchesKeepDistinctProviderSessionRefs`.

- [x] `tests/functional/provider_sessions/association/response_exec_metadata_test.go`
  - `TestResponseExecGoldenMetadataSurvivesCLIProjection`.
  - `TestResponseExecGoldenMetadataSurvivesAPIResponseEvents`.
  - `TestResponseExecGoldenMetadataSurvivesReplay`.
    - The assertions use checked-in expected metadata, not mapper-generated
    expected values.

- [x] `tests/functional/provider_sessions/build_process_inert_test.go`
  - `TestProviderSessionsRemainInertThroughRootBuildProcessConstruction`.

- [x] `tests/functional/provider_sessions/peer_import_boundary_test.go`
  - `TestFunctionalProviderSessionsPackageUsesPublicProcessImportsOnly`.

## Wave 2 — operator settings

- [x] `tests/functional/operator_settings/servicewire/servicewire_composition_test.go`
  - `TestServiceWireCompositionRootServesDocumentAndResolutionOperations`.
  - `TestServiceFromHomePortsConstructsSettingsRoot`.
  - `TestServiceFromHomePortsRejectsMissingPorts`.
  - `TestServiceFromConfigDocumentConstructsFromDocumentPorts`.
  - `TestServiceFromConfigDocumentRejectsMissingDocumentPorts`.
  - `TestResolveFromHomeRejectsMissingFilesystemPorts`.
  - `TestRegisterDefaultsResolutionFromHomeRestoresAdapterOwnership`.
  - `TestResolveFromHomeUsesSettingsAdapterOwnershipPath`.
  - `TestResolveFromHomeFallbackPreservesAcceptedSemantics`.

## Wave 2 — events

- [x] `tests/functional/events/factory_events/order_and_cursor_test.go`
  - `TestAPIGetFactoryEventsReturnsOrderedDurableHistory`.
  - `TestAPIEventCursorReturnsOnlyNewerEvents`.
  - `TestAPIInvalidEventCursorReturnsTypedError`.
  - `TestAPISubmitWorkEmitsCanonicalTraceAwareBatchEvent`.
  - `TestFactoryEventStreamIsOrderedAndClosesAtSessionTermination`.
  - `TestFactoryEventStreamReconnectHasNoGapOrDuplicate`.

- [x] `tests/functional/events/response_events/stream_test.go`
  - `TestAPIResponseEventSSEStreamsRetainedThenLiveEvents`.
  - `TestAPIResponseEventCursorGapEmitsStreamGap`.
  - `TestAPIResponseEventSessionExpiryReturnsTypedGone`.
  - `TestAPIResponseEventStreamClosesAtDocumentedBoundary`.

- [ ] `tests/functional/events/response_events/cli_api_parity_test.go`
  - `TestCLIAndAPIResponseEventsMatchForGoalInvocation`.
  - `TestCLIAndAPIResponseEventsMatchForSubagentInvocation`.
  - `TestTerminalInvocationResultIsNotMisclassifiedAsResponseEvent`.

- [ ] `tests/functional/events/replay/record_replay_test.go`
  - `TestRecordReplayReproducesSuccessfulPublicOutcome`.
  - `TestRecordReplayReproducesFailureAndLifecycleControls`.
  - `TestReplayOfSameArtifactIsDeterministic`.

- [ ] `tests/functional/events/replay/corrupt_artifact_test.go`
  - `TestReplayCorruptArtifactReturnsActionableFailure`.
  - `TestReplayUnsupportedVersionReturnsCompatibilityFailure`.

## Wave 3 — models

- [ ] `tests/functional/models/invoke/cli_test.go`
  - `TestCLIModelInvokeUsesConfiguredFakeRuntime`.
  - `TestCLIModelInvokeMissingModelFailsActionably`.
  - `TestCLIModelReadinessTimeoutCancelsRuntime`.

- [ ] `tests/functional/models/lifecycle/http_test.go`
  - `TestAPIModelPullReadinessInvokeAndShutdown`.
  - `TestAPIModelHostFailureProducesTypedError`.
  - `TestAPIListGetAndInvokeModelWithFakeRuntime`.
  - `TestAPIPullModelReportsProgressAndReadiness`.
  - `TestAPIUnknownModelAndRuntimeFailureUseTypedErrors`.

- [ ] `tests/functional/models/cross/cli_api_test.go`
  - `TestCLIInvokedModelStatusIsVisibleThroughAPI`.
  - `TestCLIAndAPIModelFailuresShareCompatiblePublicFacts`.

## Wave 3 — guards and resources

- [ ] `tests/functional/guards/global_test.go`
  - `TestGlobalGuardBlocksBelowThresholdAndAllowsAtThreshold`.
  - `TestGlobalGuardStateIsVisibleInPublicEvents`.

- [ ] `tests/functional/guards/workstation_test.go`
  - `TestWorkstationGuardControlsRepeatedExecution`.
  - `TestWorkstationGuardFailureDoesNotConsumeResource`.

- [ ] `tests/functional/resources/concurrency_test.go`
  - `TestResourceLimitEnforcesExactConcurrentDispatchMaximum`.
  - `TestResourceReleasesAfterSuccessFailureCancelAndTimeout`.

- [ ] `tests/functional/resources/fairness_long_test.go`
  - `TestContendingWorkEventuallyMakesProgressWithoutStarvation`.
  - `TestResourceContentionDoesNotDuplicateWork`.

## Wave 3 — observability

- [x] `tests/functional/observability/verification/verify_tier_contract_test.go`
  - Verify-tier Makefile/CI contract smoke for `verify-*` tier wiring, wrapper
    scripts, and lane ordering (wrong-layer replacement for smoke catch-all).

- [ ] `tests/functional/observability/logging/redaction_test.go`
  - `TestRuntimeLogsCorrelateSessionWorkAndDispatch`.
  - `TestRuntimeLogsRedactPromptCredentialsAndEnvironment`.
  - `TestDisabledLoggingDoesNotChangeExecutionOutcome`.

- [ ] `tests/functional/observability/metrics/lifecycle_test.go`
  - `TestMetricsCountSuccessFailureRetryThrottleAndCancelOnce`.
  - `TestMetricsCarryCustomerSafeCorrelationOnly`.

## Wave 3 — product delivery

- [ ] `tests/functional/product/docs/contract_test.go`
  - `TestPackagedDocsIndexMatchesReferenceFiles`.
  - `TestDocsOutputContainsNoBrokenInternalLinks`.

- [x] `tests/functional/product/packaged_factory_guard_failure/packaged_factory_guard_failure_test.go`
  - `TestInitUnknownPackagedFactoryFailsClosedWithCatalogInventory`.

- [ ] `tests/functional/product/packaged_factory_portability/packaged_factory_portability_test.go`
  - `TestPackagedFactoryInitMaterialization_InvokesOutsideRepositoryWithBootstrapParity`.

- [ ] `tests/functional/product/dashboard/http_test.go`
  - `TestDashboardIndexStaticAssetsAndDeepLinksAreServed`.
  - `TestDashboardUnknownAssetReturns404WithoutIndexFallback`.

## Wave 3 — resilience

- [ ] `tests/functional/resilience/process/repeated_start_stop_test.go`
  - `TestProcessCanExecuteRepeatedCommandsWithoutReinjection`.
  - `TestRepeatedServerStartStopLeavesNoListenerOrGoroutineLeak`.

- [ ] `tests/functional/resilience/process/partial_startup_test.go`
  - `TestPartialInitializerFailureUnwindsStartedRolesInReverseOrder`.
  - `TestStartupFailureLeavesNoFactorySessionOrWorkerProcess`.

- [ ] `tests/functional/resilience/batch/partial_batch_test.go`
  - `TestPartialBatchSuccessAndFailureRemainIndividuallyInspectable`.
  - `TestPartialBatchRetryDoesNotDuplicateSuccessfulWork`.

- [ ] `tests/functional/resilience/platform/windows_process_test.go`
  - `TestWindowsProviderTimeoutTerminatesProcessTree`.
  - `TestWindowsPathsAndArgumentsReachProviderWithoutShellCorruption`.

- [ ] `tests/functional/resilience/platform/unix_process_test.go`
  - `TestUnixProviderTimeoutTerminatesProcessGroup`.
  - `TestUnixSignalsMapToInterruptedPublicOutcome`.

## Wave 3 — operator settings

- [x] `tests/functional/operator_settings/servicewire/servicewire_composition_test.go`
  - `TestServiceWireCompositionRootServesDocumentAndResolutionOperations`.
  - `TestServiceFromHomePortsConstructsSettingsRoot`.
  - `TestServiceFromHomePortsRejectsMissingPorts`.
  - `TestServiceFromConfigDocumentConstructsFromDocumentPorts`.
  - `TestServiceFromConfigDocumentRejectsMissingDocumentPorts`.
  - `TestResolveFromHomeUsesSettingsAdapterOwnershipPath`.
  - `TestResolveFromHomeRejectsMissingFilesystemPorts`.
  - `TestResolveFromHomeFallbackPreservesAcceptedSemantics`.
  - `TestRegisterDefaultsResolutionFromHomeRestoresAdapterOwnership`.

## Completion audit

- [ ] Every checkbox is implemented, linked to an existing equivalent test, or
  marked with an approved `wrong test layer` explanation and replacement
  evidence.
- [ ] No test remains owned by `tests/functional/runtime_api`.
- [ ] No broad `smoke` package remains a domain owner.
- [ ] Every top-level customer test has a Go doc comment.
- [ ] Every provider golden reference resolves to a tracked, sanitized
  manifest.
- [ ] The generated visualization lists every implemented file above under the
  correct domain.
- [ ] `transport`, `workers`, `orchestration`, and `workstations` appear first
  in the report and contain no undocumented scenarios.
- [x] `tests/functional/sessions/root_composition/build_process_inert_test.go`
- [x] `tests/functional/sessions/root_composition/lifecycle_runtime_opening_test.go`
- [x] `tests/functional/sessions/root_composition/packaged_root_shape_test.go`
- [x] `tests/functional/sessions/root_composition/peer_import_seal_test.go`
- [x] `tests/functional/sessions/root_composition/work_admission_response_stream_test.go`
- [x] `tests/functional/sessions/root_composition/work_peer_import_seal_test.go`
- [x] `tests/functional/providers/gemini/process_harness_test.go`
- [x] `tests/functional/providers/kiro/process_harness_test.go`
- [x] `tests/functional/providers/pi/process_harness_test.go`

- [x] `tests/functional/factory_definitions/transports/cli/named_lifecycle/named_lifecycle_test.go`

- [x] `tests/functional/factory_definitions/transports/cli/validate_persist/validate_persist_test.go`

- [x] `tests/functional/factory_runtime/orchestrators/petri/guards/eligibility_test.go`

- [x] `tests/functional/provider_sessions/build_process_inert_test.go`

- [x] `tests/functional/provider_sessions/peer_import_boundary_test.go`

- [x] `tests/functional/work/transports/cli/submit/batch_contract/batch_contract_test.go`

- [x] `tests/functional/workers/transports/cli/run/help/invocation_help_test.go`

- [x] `tests/functional/workers/transports/cli/run/modes/output_modes_test.go`
