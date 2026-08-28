# C07 Transport cleanup characterization and repair evidence

- Story: `functional-test-optimization-c07-cleanup-transport-runtime-orchestration-001`
- Parent behavior: `BEH-TRANSPORT-CLEAN` — Transport behavior remains public and
  unchanged while owned processes, ports, sessions, streams, and temporary roots
  are made observable at their actual boundaries.
- Status: `GATE-CHAR-TRANSPORT` passed on the current Windows host. The
  characterization below is the pre-repair record; the repair result and its
  evidence are recorded after the census.
- Recorded at UTC: `2026-08-28`.
- Source plan status: `docs/temp/functional-test-optimization.md` and the
  referenced `tasks/todo` plan are absent from this checkout. The checked-in
  `prd.json`/`prd.md`, current test bodies, and repository standards were used
  as the authoritative retained story packet; no scope was inferred from the
  missing files.
- Owned artifact: this document and the package-local characterization in
  `tests/functional/transport/cli/commands`.
- Read-only surfaces: `tests/functional/internal/support/**` and the c01
  eligibility inventories were not changed.

This artifact records the complete current Transport impact inventory before
cleanup repair. Test names below are the top-level identities returned by
`go test -list '^Test'`; nested identities are the runtime `=== RUN` rows from
the full Transport run. The new external-home row is a named subscenario of
the existing shared CLI test and does not remove or rename a top-level public
behavior witness.

## GATE-CHAR-TRANSPORT result

Environment:

- `go version go1.25.0 windows/amd64`
- OS/architecture: `windows/amd64`; logical CPUs: `24`
- Current branch base before this story: `34cbab9f25`
- No contracts, generated clients, Makefile, workflow, product, or shared
  support files changed.

Exact discovery procedure:

```text
go list ./tests/functional/transport/...
for each resolved package:
  go test <package> -list '^Test'
```

The discovery command resolved 15 packages and exited `0`. The complete
runtime characterization command was executed package-by-package so each
package result and runtime `=== RUN` identity could be counted:

```text
go test -count=1 -timeout=10m <each package from go list> -v
```

It exited `0`: 112 top-level tests, 271 total runtime `=== RUN` identities
(112 top-level plus 159 nested named scenarios), 111 top-level passes, one
explicit top-level skip, and zero failures. The one skip is
`TestPinnedAcpxCompletesDefaultFactoryBuilderPrompt`; its body requires
`INFINITE_YOU_RUN_ACPX_REAL_CLIENT=1`, so optional real-client absence remains
an explicit skip rather than an invented pass claim.

The focused pre-repair gap procedure was:

```text
go test -count=1 -timeout=5m ./tests/functional/transport/cli/commands \
  -run '^TestCLISharedRemoteScenarios$/^TestCLISessionListCharacterizesExternalHomeRecordingState$' -v
```

It exited `0` because the package-local test expects the public CLI operation
to fail. The observed server log identified:

```text
list recorded Factory Sessions: identify recorded session artifact
"2026/08/28/c07-external-home-malformed.json": Factory Session UUID is empty
```

The CLI returned the existing sanitized failure contract and debug cause:

```text
session list response endpointPath=/factory-sessions status=500
{"code":"CLI_COMMAND_FAILED","family":"INTERNAL_SERVER_ERROR","message":"command failed"}
debug: cause[0]=list factory sessions failed (500): failed to list factory sessions
```

The scenario creates the malformed dated artifact under a `t.TempDir` that is
outside the server's disposable Factory/home fixture, temporarily points the
production `os.UserHomeDir` inputs (`HOME` and `USERPROFILE`) at that directory,
and restores the environment through `t.Setenv`. It uses the real
`StartFunctionalAPIServer` and public CLI `session list --history-only` path;
the server's recording inventory consults the external home and fails before
returning a session list. The test log records the controlled external root
and relative artifact reference, while the public diagnostic does not leak
the malformed payload. No developer profile or shared support file is
modified.

An initial probe that only set `Command.Env` on the already-composed reusable
CLI root did not reproduce the failure: the durable-listing builder captures
its home resolver at root composition, before that invocation-local
environment is applied. That is a confirmed composition/isolation finding
for the repair story and is retained here rather than hidden by changing the
existing helper.

## Package and top-level test census

Classification vocabulary:

- `shareable`: process-free or ordinary fixture-only behavior with no owned
  lifecycle resource.
- `shareable-with-controlled-edge`: production `root.BuildProcess` /
  `Process.Execute` behavior using a controlled provider, HTTP fixture, or
  temporary Factory; serialized reuse is safe because the fixture and route
  are scoped by the scenario.
- `isolated-with-reason`: a real OS process/descendant, listener, stdio pipe,
  protocol peer, server lifecycle, signal, executable, permission, or
  environment boundary is the property under test; sharing or mocking would
  remove the evidence.

Every lifecycle-sensitive row has either an inline comment explaining that
reason or an adjacent helper whose name and body make the boundary explicit.
The package-level resource and cleanup columns apply to every listed test in
that row unless a named exception is included.

| Package | Top-level / runtime identities | Owned resources and cleanup owner | Classification and behavioral witness |
| --- | ---: | --- | --- |
| `transport/acp/realclient` | 3 / 3 | Pinned `acpx` peer, child descendants, stdio, temporary config/home; command wait and test cleanup. | `isolated-with-reason`: real ACP client/process and optional prerequisite semantics. |
| `transport/acp/stdio` | 8 / 8 | Root process, stdin/stdout pipes, ACP session/stream, wire transcript files; `t.Cleanup`, close, and process shutdown. | `isolated-with-reason`: ACP prompt/control/close and owner-readable transcript behavior. |
| `transport/cli/commands` | 14 / 56 | Shared service-mode API server, reusable root, temporary Factories/homes/files, remote Factory Sessions; harness/server cleanup and session terminate/delete callbacks. The external-home isolation witness owns a second server and dated artifact. | `shareable-with-controlled-edge` for ordinary command behavior; `isolated-with-reason` for server-backed session/recording lifecycle and the external-home scenario. |
| `transport/cli/output` | 14 / 27 | Root invocations, response streams, slow/failing writers, temporary Factory/home; per-test process/stream closure. | `shareable-with-controlled-edge` for controlled providers and streams; writer cancellation remains isolated to its invocation. |
| `transport/cli/parameters` | 18 / 24 | Root invocations, temporary Factory/working-directory assets, parameter streams; process cleanup and scenario-local files. | `shareable-with-controlled-edge`; the reusable parameter spine is serialized and uses fresh invocation inputs. |
| `transport/cli/process` | 4 / 9 | Direct root process, controlled provider command edge, ACP pipe, and process-free PID parser; `support.CleanupProcess`/pipe cleanup where applicable. | Mixed: `isolated-with-reason` for ACP cancellation; `shareable-with-controlled-edge` for provider outcomes; `shareable` for PID parsing. |
| `transport/docs` | 1 / 1 | Packaged docs lookup and temporary invocation state; root cleanup. | `shareable-with-controlled-edge`: public packaged topic alias behavior. |
| `transport/http/server` | 21 / 97 | HTTP server/listener, active Factory Sessions/streams, request contexts, temporary Factory/home, generated clients, and route fixtures; server stop, process close, listener rebind/observer checks, and `t.Cleanup`. | Mixed: `isolated-with-reason` for production loopback, bind, shutdown, active stream, and concurrency rows; `shareable-with-controlled-edge` for handler/schema/route assertions. |
| `transport/http/status` | 2 / 2 | Production API server/listener, runtime status, temporary Factory/home; server stop and root cleanup. | `isolated-with-reason`: startup/status boundary and public configuration redaction. |
| `transport/mcp/protocol` | 3 / 3 | MCP request/response stream and shutdown protocol; pipe/server cleanup. | `shareable-with-controlled-edge` for malformed/missing request mapping; `isolated-with-reason` for stdio shutdown. |
| `transport/mcp/stdio` | 7 / 9 | Root process, MCP stdin/stdout pipes, runtime/fixture server, temporary home/project root; pipe close, server shutdown, and process cleanup. | `isolated-with-reason` for stdio/runtime lifecycle; `shareable` only for the uncomposed-server constructor rejection. |
| `transport/run_scoped_server` | 14 / 20 | Local production listener, server/site runtime, Factory Sessions, HTTP peers, temporary Factory/home, and reserved ports; listener close/rebind, process close, and `t.Cleanup`. | `isolated-with-reason`: local/remote placement, exact bind, startup failure, replay, and server/site lifecycle are the behavior under test. |
| `transport/shell_completion` | 1 / 7 | Root process, temporary working/home directories, generated completion output; root cleanup and scenario files. | `shareable-with-controlled-edge`: shell script generation is asserted without starting a shell process. |
| `transport/submit` | 2 / 5 | One root process, temporary payload files, controlled HTTP server and public submit route; process/server cleanup. | `shareable-with-controlled-edge`: public submit behavior and downstream structured-output failure. |
| `transport/terminalportlock` | 0 / 0 | Cross-platform helper source has no top-level functional tests in this checkout. | No lifecycle-sensitive test exists to classify; this is an explicit empty package, not an unclassified row. |
| **Total** | **112 / 271 runtime rows** | **All package-owned resources have a paired cleanup owner in the reviewed bodies.** | **Zero unclassified lifecycle-sensitive top-level or nested scenario rows.** |

### Top-level identities by package

The following names are the complete `go test -list '^Test'` results.

#### `transport/acp/realclient` (3)

`TestRunBoundedCommandTerminatesScenarioDescendants`,
`TestRunBoundedCommandTerminatesDescendantsAfterNonZeroExit`,
`TestPinnedAcpxCompletesDefaultFactoryBuilderPrompt`

#### `transport/acp/stdio` (8)

`TestServeACP_RootBuildProcessProviderFailureTerminalizesPrompt`,
`TestServeACP_RootBuildProcessCancelTerminalizesOnlyCapturedPrompt`,
`TestServeACP_RootBuildProcessCloseStopsCapturedFactorySession`,
`TestServeACP_RootBuildProcessCloseThenLoadReplaysRetainedItemIdentities`,
`TestServeACP_RootBuildProcessCompletesOneFactoryPrompt`,
`TestServeACPWritesAWireTranscriptByDefault`,
`TestServeACPDoesNotRecordFailedOutboundFrame`,
`TestServeACPWireTranscriptIsOwnerReadableOnly`

#### `transport/cli/commands` (14)

`TestCLIDocsListsPackagedTopics`,
`TestCLIDocsEveryTopicRendersNonEmptyContent`,
`TestCLIDocsUnknownTopicReturnsActionableFailure`,
`TestCLIFactoryFlattenExpandPreservesMeaning`,
`TestCLIRunNamedFactory`,
`TestCLIRunInvalidFactoryReturnsValidationFailure`,
`TestCLIRunFactoryByPath`,
`TestCLIRunFactoryWritesPrimaryResultFromStdin`,
`TestCLIRunRejectsConflictingPositionalAndStdinInput`,
`TestCLIRunFailureWritesNoSuccessPayloadToStdout`,
`TestCLIRunCleanInvocationStdoutRemainsPipeable`,
`TestCLIRunAmbiguousPromptAndStdinFailsBeforeRuntimeStartup`,
`TestCLISharedRemoteScenarios`,
`TestCLIWorkRenderProducesDeterministicGraph`

#### `transport/cli/output` (14)

`TestCLIJSONSuccessDecodesToPublicInvocationResult`,
`TestCLIJSONFailureRemainsValidJSON`,
`TestCLIInvocationArgumentFailuresAreBadRequest`,
`TestCLIJSONContainsNoPrivateRuntimeFields`,
`TestCLIJSONOutputSelectionFailsBeforeProductActivation`,
`TestCLINDJSONEmitsDecodableResponseEventsThenInvocationResult`,
`TestCLINDJSONSequenceIsMonotonic`,
`TestCLINDJSONFailureEndsWithOneTerminalResult`,
`TestCLISlowWriterDoesNotReorderResponseEvents`,
`TestCLIWriterFailureCancelsInvocation`,
`TestCLITextStreamSurfacesIncrementalMessages`,
`TestCLITextStreamDoesNotPrintStructuredEnvelopeNoise`,
`TestCLITextStreamOperatorContinuousRunReportsStartupOutputWithoutQuiet`,
`TestCLITextStreamInterruptedRunDoesNotClaimCompletion`

#### `transport/cli/parameters` (18)

`TestRetiredSessionDispatchesCommandIsUnknown`,
`TestCLIStringBooleanAndRepeatedFlagsReachRequest`,
`TestCLIFlagAfterPositionalValueUsesDocumentedParsing`,
`TestCLIUnknownFlagFailsBeforeLifecycleStart`,
`TestCLIJSONParameterPreservesNestedObjectAndArray`,
`TestCLIInvalidJSONParameterNamesTheParameter`,
`TestCLIJSONNullAndEmptyValuesRemainDistinct`,
`TestRunKeyValueParametersReachFactoryInvocation`,
`TestRunKeyValuePreservesEqualsInValue`,
`TestRunDuplicateKeyUsesDocumentedPrecedence`,
`TestRunMalformedKeyValueFailsWithoutDispatch`,
`TestCLIParameterReusableProcessSpine`,
`TestRunAcceptsOnePositionalPrompt`,
`TestRunRejectsExtraPositionalValues`,
`TestOptionalSessionIDUsesDefaultWhenOmitted`,
`TestCLIRelativeFactoryPathResolvesFromInvocationDirectory`,
`TestCLIWorkingDirectoryDoesNotLeakIntoOutput`,
`TestCLIMissingWorkingDirectoryAssetFailsActionably`

#### `transport/cli/process` (4)

`TestACPServeCancellationPreservesContextCanceledIdentityThroughProcess`,
`TestContextCancellationPIDReadinessIgnoresPartialPublication`,
`TestCLIWorkerFailureExitCode`,
`TestCLISuccessExitCode`

#### `transport/docs` (1)

`TestDocsTopicInventory_AliasesRemainQueryableThroughPackagedSurface`

#### `transport/http/server` (21)

`TestAPIConcurrentSessionRequestsRemainIsolated`,
`TestAPICancelledRequestDoesNotCancelUnrelatedSession`,
`TestAPIJSONRequestsAndResponsesUseDocumentedContentType`,
`TestAPIUnsupportedContentTypeReturns415`,
`TestAPIMalformedJSONReturnsStructured400`,
`TestGeneratedClientStatusAndSessionRoundTrip`,
`TestGeneratedClientDecodesRepresentativeStructuredError`,
`TestGeneratedClientAndServerSchemaStayAligned`,
`TestAPIServerDiagnosticsUseProductionLoopbackStarter`,
`TestAPIServerGracefulShutdownThroughProductionLoopbackLifecycle`,
`TestListenerStopObserverReportsBoundedOpenListenerOutcomes`,
`TestAPIRoutesEveryOpenAPIOperationToNon404Handler`,
`TestAPIUnknownRouteReturnsStructuredNotFound`,
`TestAPIDashboardRoutesServeEmbeddedShellAssetAndFallback`,
`TestAPIWrongMethodReturnsDocumentedMethodError`,
`TestAPIServerPprofIsOptInThroughThePublicRunPath`,
`TestAPIServerStartsOnConfiguredListenerAndServesStatus`,
`TestAPIServerUsesPlatformStarterThroughRootProcess`,
`TestAPIServerShutdownClosesListenerAndActiveStreams`,
`TestAPIServerBindFailureUnwindsStartedLifecycleRoles`,
`TestWorkTerminalResponsePreservesOrderedTypedContentThroughPublicBoundary`

#### Remaining packages

- `transport/http/status` (2): `TestAPIStatusReportsReadyAfterStartup`,
  `TestAPIStatusDoesNotLeakInternalConfiguration`.
- `transport/mcp/protocol` (3):
  `TestMCPMalformedParametersReturnInvalidParams`,
  `TestMCPMissingFactorySessionReturnsCanonicalNotFound`,
  `TestMCPServerShutdownClosesStdioCleanly`.
- `transport/mcp/stdio` (7): `TestMCPStdioInitializeAndToolDiscovery`,
  `TestMCPUnknownToolReturnsProtocolError`,
  `TestMCPDiscoveryContainsCanonicalFactorySessionTools`,
  `TestMCPStdioRuntimeRejectsMissingHomeEnvironment`,
  `TestMCPStdioRuntimeRejectsInvalidRuntimeProjectRoot`,
  `TestMCPStdioFixtureAndRuntimePathsReachInitializer`,
  `TestMCPStdioOpenRejectsUncomposedServerAndStreams`.
- `transport/run_scoped_server` (14):
  `TestRunScopedServerAndSiteOwnNamedAndFileInvocationLifecycles`,
  `TestRunScopedServerOwnsRawJavaScriptLifecycleAfterReadiness`,
  `TestRunScopedRawJavaScriptServerReportsUnavailableWorkerSessionOwner`,
  `TestRunScopedServerUsesProductionListenerAndReportsFallback`,
  `TestRunScopedServerUsesExactListenAddress`,
  `TestRemotePlacementDispatchesThroughSelectedServer`,
  `TestRunScopedServerRejectsUnavailableExactListenAddress`,
  `TestRunScopedServerRejectsRemoteBindTargetAtCLIBoundary`,
  `TestRemotePlacementRejectsLocalHostingBeforeInitialization`,
  `TestRemotePlacementRejectsLocalOnlyServerCommand`,
  `TestRemotePlacementRejectsLocalOnlyFactoryCommand`,
  `TestRunRejectsMalformedExactListenAddress`,
  `TestRunScopedServerReportsExhaustedTerminalPortAtCLIBoundary`,
  `TestRunScopedServerOwnsReplayLifecycle`.
- `transport/shell_completion` (1):
  `TestGeneratedCompletionScriptsReachRootProcess`.
- `transport/submit` (2): `TestSubmitFamilyExecutesThroughRootBuiltProcess`,
  `TestSubmitFamilyEnqueuesWorkBeforeDownstreamStructuredOutputFailure`.

### Nested named scenarios

The full run observed 159 nested identities. Dynamic rows are recorded here by
their complete parent/name forms so the runtime count can be reconciled with
the package table:

- `transport/cli/commands`: `TestCLIDocsEveryTopicRendersNonEmptyContent/{agents,authoring-factories,packaged-factories,run,config,factory-validation,mock-workers,record-replay,guards,relationships,operations,work,sessions,metrics,orchestrators,javascript-workflows,mcp,workstations,workers,providers,serve-acp,resources,models,batch-inputs,templates}`; `TestCLIRunNamedFactory/{named_from_unrelated_working_directory,packaged_goal_summary_primary_result}`; and `TestCLISharedRemoteScenarios/{TestCLISubmitBatchInlineJSON,TestCLISubmitBatchFile,TestCLISubmitUnavailableServer,TestCLISubmitBackendErrorPreservesPublicMessage,TestCLIWorkListAndShowReflectSubmittedWork,TestCLIWorkMoveChangesState,TestCLIWorkShowMissingReturnsNotFound,TestCLIFactoryInitValidateAndShow,TestCLIFactoryReplaceCurrentChangesSessionFactory,TestCLISessionCreateListShowDelete,TestCLISessionListUsesIsolatedRecordingHome,TestCLISessionPauseBuffersAndResumeDispatches,TestCLISessionMissingIDReturnsNotFound,TestCLIWorkApprovalListAndShowExposePendingApprovalAndSafeEmptyErrors,TestCLIExplicitSessionIsolation}`. Total nested rows: 42.
- `transport/cli/output`: `TestCLIJSONContainsNoPrivateRuntimeFields/{success_stdout_stays_on_public_InvocationResponse_fields,terminal_failure_stdout_and_stderr_stay_on_public_contract_fields}`; `TestCLIJSONFailureRemainsValidJSON/{pre-terminal_failure_leaves_stdout_empty_with_one_stderr_ErrorResponse,terminal_failure_emits_failed_InvocationResponse_and_one_stderr_ErrorResponse}`; `TestCLIInvocationArgumentFailuresAreBadRequest/{unknown_argument_in_normal_JSON_mode,unknown_argument_in_quiet_mode,missing_value_before_next_invocation_flag,missing_value_for_run_flag}`; `TestCLIJSONOutputSelectionFailsBeforeProductActivation/{quiet_and_global_JSON,quiet_and_explicit_output,unsupported_explicit_output}`; `TestCLITextStreamDoesNotPrintStructuredEnvelopeNoise/{human_response-stream_lifecycle_presentation,quiet_clean_primary_result}`. Total nested rows: 13.
- `transport/cli/parameters`: `TestRunMalformedKeyValueFailsWithoutDispatch/{missing_named_value_after_key,bare_key=value_without_named_prefix}`; `TestCLIParameterReusableProcessSpine/{observer_root_parses_generic_flags,full_handler_submits_combined_signature_once}`; `TestOptionalSessionIDUsesDefaultWhenOmitted/{omitted_session_positional_targets_default_session,explicit_session_positional_overrides_default_targeting}`. Total nested rows: 6.
- `transport/cli/process`: `TestContextCancellationPIDReadinessIgnoresPartialPublication/{empty_publication,whitespace_publication,partial_publication,non_numeric_publication,complete_publication}`. Total nested rows: 5.
- `transport/http/server`: `TestGeneratedClientStatusAndSessionRoundTrip/{cancellation,deadline}`; `TestAPIServerDiagnosticsUseProductionLoopbackStarter/{disabled_by_default,enabled_by_opt-in}`; `TestAPIRoutesEveryOpenAPIOperationToNon404Handler/{previewFactory,listFactorySessions,openFactorySession,startDurableFactorySessionAsync,startDurableFactorySessionSync,closeFactorySession,getFactorySession,listHumanApprovalsBySessionId,getHumanApprovalBySessionId,approveFactorySession,listFactorySessionArtifacts,getFactorySessionArtifact,cancelFactorySession,listFactorySessionDispatches,getFactorySessionDispatch,getEventsBySessionId,getCurrentFactoryBySessionId,saveCurrentFactoryBySessionId,getCurrentFactoryWorkstationPromptTemplateContractBySessionId,validateCurrentFactoryWorkstationPromptTemplateBySessionId,interruptFactorySessionDispatch,invokeFactorySessionBySessionId,getFactorySessionPartialResult,pauseFactorySession,setFactorySessionResourceCapacity,getFactoryResponseEventsBySessionId,getFactorySessionResult,getFactorySessionResults,resumeFactorySession,retryFactorySessionDispatch,getMetrics,getMetricsCosts,listModels,invokeGenericModel,removeModel,getModel,invokeModel,pullModel,listPackagedFactories,getProviderSessionDetails,getStatus,listWorkerSessions,startWorkerSession,getWorkerSessionObservationByWorkerSessionId,cancelWorkerSession,continueWorkerSession,streamWorkerSessionEventsByTopLevelWorkerSessionId,interruptWorkerSession,pauseWorkerSession,resumeWorkerSession,terminateWorkerSession,readWorkerSessionTranscriptByWorkerSessionId,shutdownServer}`; `TestWorkTerminalResponsePreservesOrderedTypedContentThroughPublicBoundary/{terminal_success_keeps_ordered_typed_parts,terminal_failure_is_not_reported_as_success}`. Total nested rows: 76.
- `transport/mcp/stdio`: `TestMCPStdioFixtureAndRuntimePathsReachInitializer/{fixture-backed,runtime-backed}`. Total nested rows: 2.
- `transport/run_scoped_server`: `TestRunScopedServerAndSiteOwnNamedAndFileInvocationLifecycles/{named_positional_server,file_stdin_site}`; `TestRunScopedServerOwnsRawJavaScriptLifecycleAfterReadiness/{server,site}`; `TestRemotePlacementRejectsLocalHostingBeforeInitialization/{persistent_flags_before_run,persistent_flags_after_run}`. Total nested rows: 6.
- `transport/shell_completion`: `TestGeneratedCompletionScriptsReachRootProcess/{bash,zsh,powershell,factory,mode,file}`. Total nested rows: 6.
- `transport/submit`: `TestSubmitFamilyExecutesThroughRootBuiltProcess/{batch_dry-run,unary_named_session,unary_JSON_payload}`. Total nested rows: 3.

The nested total is `42 + 13 + 6 + 5 + 76 + 2 + 6 + 6 + 3 = 159`.

## CASE-T01 through CASE-T16 reconciliation

| Case | Characterized Transport witnesses | Observable result and cleanup property | Owning classification / boundary |
| --- | --- | --- | --- |
| `CASE-T01` happy | CLI named/path/stdin runs; submit; HTTP status/server; ACP prompt; MCP initialize; shell completion; docs. | Existing public output/status/events/results remain unchanged; the owning root/server/pipe/temp fixture has a paired cleanup path. | Ordinary rows use `shareable-with-controlled-edge`; server/protocol rows stay isolated. |
| `CASE-T02` malformed input | Unknown/conflicting CLI arguments, invalid Factory, malformed HTTP JSON/content type/method, malformed MCP parameters, malformed listen address. | Existing coded diagnostics/statuses remain; validation happens before dispatch where asserted, with no success payload or partial lifecycle. | Root/API/protocol functional boundaries. |
| `CASE-T03` startup failure | Missing MCP home/project root, server starter/bind failures, invalid Factory startup, unavailable remote/local placement. | Existing actionable startup failure remains; started process/listener/session roles unwind through existing cleanup. | `isolated-with-reason` where startup/role teardown is the witness. |
| `CASE-T04` port/capacity | Exact listen address occupied, exhausted terminal port, configured listener startup and rebind checks. | Existing bind/capacity diagnostics remain; no partial listener survives and rebind succeeds after cleanup. | `isolated-with-reason`, real local listener. |
| `CASE-T05` authorization/permission | ACP wire transcript owner-only read, failed outbound frame suppression, unsafe request/missing permission paths. | Owner-readable transcript and safe diagnostics remain; private payload/artifact content is not emitted. | `isolated-with-reason`, real file/stdio or protocol boundary. |
| `CASE-T06` shutdown | ACP close/cancel; HTTP graceful stop/active streams; MCP stdio shutdown; run-scoped server/site/replay lifecycle. | Stream terminates once, listener/process closes, session reaches its expected terminal/closed state, and `Serve`/`Execute` returns. | `isolated-with-reason`; real protocol/listener/process lifecycle. |
| `CASE-T07` executable selection | Optional pinned ACPX test and bounded command descendant tests; CLI process package provider edge. | Correct optional skip or executable/descendant outcome remains; no child or temporary state is claimed clean unless the body observes it. | Real-client/OS rows isolated; optional real-client was not run without its explicit flag. |
| `CASE-T08` crash/non-zero | Bounded command non-zero descendant test, CLI/provider failure, MCP/API failure paths, structured output failure. | Existing failure classification and public error routing remain; characterization identifies the failure without changing the product path. | Mixed root/process/protocol boundary. |
| `CASE-T09` timeout | Bounded listener observer, context-cancellation/process selectors, server/request deadline subcases. | Existing timeout/cancellation identity remains; owned waiters/streams/processes have bounded close/join behavior. | Isolated process/listener rows; no sleep-based assertion added. |
| `CASE-T10` cancellation | CLI context cancellation; HTTP canceled request/unrelated session; ACP cancel; MCP shutdown; run-scoped cancel/stop. | No false success payload; cancellation affects only its owner and cleanup closes the attributable boundary. | `isolated-with-reason` for real process/protocol/listener cases. |
| `CASE-T11` partial completion | Submit accepted prefix followed by downstream structured-output failure; ACP failed outbound frame; work terminal failure. | Committed prefix/public failure remains ordered and non-duplicated; no success envelope is emitted after failure. | Root/HTTP/ACP boundary assertions. |
| `CASE-T12` concurrency/isolation | Concurrent HTTP sessions, canceled request versus unrelated session, CLI explicit-session isolation, MCP/stream ownership rows. | IDs and responses remain correlated; one cancellation/close does not terminate an unrelated owner. | `isolated-with-reason` where concurrent server/session state is the property. |
| `CASE-T13` recovery/persistence | ACP close then load retained identities; run-scoped replay; CLI session lifecycle show/list/delete; packaged state lookup. | Retained identity/order or expected not-found behavior remains; old live resources are not revived by the read. | Protocol/session rows with temporary recording roots. |
| `CASE-T14` ordering | CLI NDJSON monotonic sequence/slow writer; ACP wire transcript; ordered HTTP Work terminal content; completion/output streams. | Declared order and one terminal item remain under normal and slow/failure consumption; stream closes. | Stream/protocol rows retain local-real boundaries where relevant. |
| `CASE-T15` empty/max/duplicate/idempotent | Empty docs/models/discovery, duplicate CLI key precedence, repeated session cleanup/not-found, unsupported/unknown routes, listener rebind. | Existing empty/boundary/duplicate/idempotent behavior remains. There is no Transport-local maximum-retention case in the current top-level inventory; none was invented. | Controlled root/API rows; listener idempotency isolated. |
| `CASE-T16` isolation classification | Every process/protocol/listener/executable/startup row in the package table, plus the new external-home scenario. | Each row names its real process-scoped property and cleanup owner; no mock replaces a local-real lifecycle witness; zero rows remain unclassified. | `isolated-with-reason` rows retain their boundary. |

### Confirmed gap and support ownership

The confirmed package-local gap is the CLI session-list dependency on the home
resolver used by the server's durable recording inventory. A malformed dated
artifact outside the Factory's disposable root causes the public session-list
command to fail. The new characterization is package-local and does not repair
the resolver or change the shared reusable harness.

No shared-support defect was confirmed. The initial invocation-environment
probe was a useful composition finding, not a reason to edit
`tests/functional/internal/support/**`; it is handed to the Transport repair
story with the smallest safe repair direction: preserve public session-list
behavior while making the durable-listing home explicit/isolated at the
package-owned boundary.

## GATE-TRANSPORT repair result

Story `functional-test-optimization-c07-cleanup-transport-runtime-orchestration-002`
repaired the confirmed package-local isolation gap without changing product or
shared-support code. The real server is composed with an explicit
`FactorySessionResolveHomeDirectory` edge pointing to a disposable recording
home. The public CLI still receives a different external `HOME`/
`USERPROFILE` containing the malformed dated artifact, so a successful empty
history response proves that the external home is not consulted.

Focused procedure and result:

```text
go test -count=1 -timeout=5m ./tests/functional/transport/cli/commands \
  -run '^TestCLISharedRemoteScenarios$/^TestCLISessionListUsesIsolatedRecordingHome$' -v
```

The command exited `0`. The isolated real-server/public-CLI witness returned a
`history` response with no live or recorded rows and no external artifact
reference or malformed payload. Its `t.Cleanup`/server stop path completed
without a shutdown diagnostic.

The full current-platform Transport procedure also exited `0`:

```text
go list ./tests/functional/transport/...
go test -count=1 -timeout=10m -v <each resolved package>
```

All 15 packages ran: 271 runtime `=== RUN` identities, 111 top-level passes,
one expected optional skip (`TestPinnedAcpxCompletesDefaultFactoryBuilderPrompt`),
and zero failures. Existing CLI, HTTP, ACP, MCP, stream, ordering, and
listener assertions remained active.

### Repeat, race, platform, and real-client boundaries

- `GATE-REPEAT`: the package-by-package `go test -count=3 -timeout=10m -v`
  sweep passed every package except the untouched `transport/cli/output`
  package, whose stream-start tests timed out under host contention. Its one
  bounded exact rerun exited `0` across three counts; no output-package code
  or shared support changed. The repaired `transport/cli/commands` package
  passed 168 runtime runs with 42 top-level passes.
- `GATE-RACE`: `-race -count=1` passed `acp/stdio`, the changed
  `cli/commands` package, `cli/process`, `http/server`, `mcp/stdio`, and
  `run_scoped_server`. The untouched `cli/output` rerun still timed out in
  `TestCLIWriterFailureCancelsInvocation`,
  `TestCLITextStreamSurfacesIncrementalMessages`,
  `TestCLISlowWriterDoesNotReorderResponseEvents`, and
  `TestCLITextStreamInterruptedRunDoesNotClaimCompletion`; the output showed
  no race detector report. This remains an unproven host/platform edge, not a
  repair regression.
- `GATE-PLATFORM`: the local-real platform evidence is `windows/amd64` on
  `go1.25.0`; Unix-specific execution remains owned by its supported CI job.
- `GATE-REALCLIENT`: the optional ACPX selector retained its skip semantics.
  `acpx` was unavailable on `PATH`, and Node.js was `v22.12.0` while the
  pinned client requires `22.13.0` or later, so the required-enabled scenario
  was not claimed.
- `GATE-CLEANUP`: the repaired witness has paired ownership for the external
  fixture, isolated recording home, server listener/process, CLI invocation,
  and temporary Factory. The repair adds no process start; the supplemental
  server is required to compare external and composed homes at the real
  boundary. `server.Stop`, root-process close, `t.Setenv`, and `t.TempDir`
  provide the teardown paths.

The Transport story proves the changed package and local repair at Windows
fidelity. It does not prove Unix cleanup, enabled pinned ACPX behavior,
Runtime API/Orchestration stories, clean-room validation, terminal CI, or
merge. The untouched `cli/output` repeat/race timing failures are retained as
host-contended evidence for later review rather than repaired in this story.

## Evidence limits and next story

The characterization section proves:

- all 15 currently resolved Transport packages and all 112 top-level test
  identities are enumerated;
- all 159 nested runtime identities from the full current-platform run are
  reconciled;
- public Transport behavior passes at the current local dependency fidelity;
- the malformed external-home recording dependency is observable before repair;
- process/protocol/listener/executable cases retain concrete isolated reasons;
- the new characterization has no payload leakage and all test-owned temporary
  resources are cleanup-scoped.

The repair section above additionally proves the changed CLI command package's
explicit home isolation and current-platform Transport teardown paths at the
declared local fidelity.

The Transport story does not prove:

- Unix-specific cleanup or the host-contended untouched `cli/output` repeat and
  race rows;
- optional pinned ACPX behavior with its prerequisite enabled;
- Runtime API or Orchestration packages;
- PR-level package latency, clean-room validation, terminal CI, or merge;
- actual writes to a developer's persistent profile. The characterization uses
  a disposable external-home stand-in and the production default resolver;
  the current host's prior real-home diagnostic was not reproduced without
  mutating that profile.

The next retained story is
`functional-test-optimization-c07-cleanup-transport-runtime-orchestration-003`:
characterize Runtime API and Orchestration cleanup before repair, while keeping
the Transport repair and its evidence unchanged.

## Follow-up host observation

After the package-by-package run, an aggregate
`go test -count=1 -timeout=10m ./tests/functional/transport/...` invocation
reported timeouts in the untouched `transport/cli/output` package for
`TestCLITextStreamSurfacesIncrementalMessages`,
`TestCLISlowWriterDoesNotReorderResponseEvents`, and
`TestCLITextStreamInterruptedRunDoesNotClaimCompletion`. The one permitted
bounded rerun of that package passed all 14 top-level tests, including those
three rows. No output-package code or shared support was changed; the
package-level result is retained as the primary characterization result and
the aggregate failure is recorded as compute/scheduling contamination rather
than a c07 Transport characterization regression.

## Story 003 — Runtime API and Orchestration characterization

### Scope and disposition

Story `functional-test-optimization-c07-cleanup-transport-runtime-orchestration-003`
is the pre-repair census for `tests/functional/runtime_api/**` and
`tests/functional/orchestration/**`. It records the package, top-level test,
named nested scenario, process, session, stream, route, listener, temporary
root, and discovery-ledger boundaries without changing production code, shared
support, public behavior, or the internal orchestration engine. The repair of
the confirmed discovery gap and any additional package-local cleanup probes is
owned by story 004.

The characterization ran from commit `0e0748a33023268570f13f9c2e4dbf12e8e25a0e`
on Windows `amd64` with Go `1.25.0`. It found no confirmed resource residue in
the Runtime API, JavaScript, or full Petri execution paths. It retained one
package-local discovery failure: Petri dispatch lists all eight top-level tests
but exits 1 because its empty runtime ledger is incorrectly treated as a full
execution. This is a failing pre-repair witness, not a repair in this story.

### Discovery and execution evidence

The exact package census command was:

```text
go list ./tests/functional/runtime_api/... ./tests/functional/orchestration/...
```

It resolved these eleven packages. The per-package discovery command was
`go test <package> -list '^Test'`; all packages exited 0 except the retained
Petri dispatch witness described below.

| Package | Top-level tests | Runtime `=== RUN` rows | Nested rows | `-list` |
| --- | ---: | ---: | ---: | --- |
| `runtime_api` | 25 | 43 | 18 | 0 |
| `runtime_api/factory_transformation` | 39 | 55 | 16 | 0 |
| `orchestration/javascript/composition` | 14 | 14 | 0 | 0 |
| `orchestration/javascript/contracts` | 8 | 8 | 0 | 0 |
| `orchestration/javascript/durability` | 3 | 3 | 0 | 0 |
| `orchestration/javascript/loading` | 11 | 11 | 0 | 0 |
| `orchestration/javascript/policy` | 2 | 2 | 0 | 0 |
| `orchestration/javascript/workers` | 1 | 27 | 26 | 0 |
| `orchestration/petri/cross` | 2 | 2 | 0 | 0 |
| `orchestration/petri/dispatch` | 8 | 50 | 42 | 1 (witness) |
| `orchestration/petri/guards` | 4 | 4 | 0 | 0 |
| **Total** | **117** | **219** | **102** | **10 pass, 1 witness** |

The complete package execution was:

```text
foreach package in (go list ./tests/functional/runtime_api/... ./tests/functional/orchestration/...)
    go test $package -count=1 -timeout=10m -v
```

All eleven package commands exited 0. The run retained all 117 top-level
identities, produced 219 runtime rows including 102 named nested rows, and
reported no test failures or skips. The full Petri dispatch run emitted 44
scenario rows. Its matrix contained one isolated executor-panic process and
one shared package process; each process started and stopped once, and every
scenario recorded both session-opened and session-closed.

The exact pre-repair Petri discovery witness was:

```text
go test ./tests/functional/orchestration/petri/dispatch -list '^Test'
```

The command listed all eight dispatch tests, emitted
`PETRI_DISPATCH_RUNTIME_MATRIX {"processes":[],"scenarios":[]}`, then exited
1 with:

```text
emit Petri dispatch runtime matrix: full Petri dispatch run recorded 0 scenarios, want 44
```

No process or session was started in list mode. The package's `TestMain`
emits the ledger for discovery, while `isFullPetriDispatchRun` only inspects
the empty `test.run` value and therefore applies the full-execution cardinality
assertion to an empty discovery ledger. Story 004 owns the discovery-safe
classification; full execution must continue to require all 44 rows.

The focused characterization selectors covered the public Runtime API,
factory transformation, JavaScript composition/contracts/durability/loading/
policy/shared-worker paths, and Petri cross/guards/dispatch paths for
`CASE-R01` through `CASE-R10` and `CASE-O01` through `CASE-O09`. Every focused
command used `-count=1 -timeout=10m`; every selected command exited 0 and kept
its existing public assertions. No focused selector was used as a substitute
for the complete package census.

One additional discovery observation was recorded for
`runtime_api/factory_transformation`: its package `TestMain` starts the shared
runtime fixture before `m.Run`, so `go test ... -list '^Test'` also starts and
tears down one process, listener, Factory root, and operator home. The output
included `FULL-003: controlled worker edge calls provider=0 script=0` and
`CLEAN-004: shared process, listener, Factory root, and operator home cleanup
passed`. This is measured setup topology rather than a confirmed leak; the
fixture is closed. Story 004 may optimize it only within its package-owned
cleanup scope.

### Resource and ownership census

| Surface | Opened or tracked resources | Characterized close/cleanup observation |
| --- | --- | --- |
| Runtime API shared fixture | One production root process, one real API listener, one isolated Factory root, one isolated operator home, explicit Factory Sessions, event streams, and provider/script/command routes | Session termination and deletion are registered before opening; stream closure is tracked with `sync.Once`; process execution is joined before `Process.Close`; listener refusal, route counts, ledger state, temp-root removal, and log absence are checked. |
| Runtime API factory transformation | One shared process/listener plus per-test named/default session and document/home roots | `TestMain` teardown cancels and joins execution, closes the process, probes listener refusal, removes Factory/home roots, and reports `CLEAN-004`; session closes are once-only and public. |
| JavaScript per-test fixtures | Functional API servers or root processes, real listeners where the scenario crosses HTTP, temporary Factory roots, and controlled command/provider/script edges | `t.Cleanup` closes servers/processes and removes `t.TempDir` roots; public result/event/session assertions remain in place. |
| JavaScript shared worker | One root process, one real API listener, one isolated root/home, selector routes, and tracked sessions | Routes unregister once after public session termination; the cleanup assertion checks route count zero, exactly one API start, every tracked session closed, and listener unreachability after process close. |
| Petri dispatch shared fixture | One lazy shared process, one real API listener, isolated root/home, path-keyed dispatch routes, session open/close records, and bounded concurrency slots | Every scenario registers a route before opening a public session and schedules once-only close/unregister; full TestMain closes command/process, records process stop, removes the root, and validates all opened sessions closed. The matrix does not yet expose active route count or post-removal root stat; those are repair-scope gaps for story 004. |
| Petri isolated panic path | One separately composed process for the panic executor case | Panic is routed to a failed terminal and the isolated process is recorded as started and stopped without contaminating the shared process. |

No shared-support-owned defect was found. `tests/functional/internal/support/**`
was read for ownership and used as-is; no support file changed. The only
confirmed failing witness is the Petri dispatch discovery ledger. The eager
factory-transformation discovery setup and the Petri matrix's missing active
route/root observations are recorded for repair planning, not claimed as
already repaired.

### Characterization case reconciliation

| Case family | Characterized public/runtime paths | Result before repair |
| --- | --- | --- |
| `CASE-R01` | Runtime API cleanup smoke, event replay/session projection, named invocation, structured invocation, and factory transformation readback | Existing public state and terminal assertions pass; fixture close/listener/temp checks pass. |
| `CASE-R02` | Canonical-only config rejection, policy rejection, whitespace/unsupported-operation rejection, invalid document targets and stale save versions | Typed/public rejection assertions pass with no observed dispatch residue. |
| `CASE-R03` | Functional server overrides, service-mode startup, model transport configuration, and factory transformation startup | Startup/override assertions pass; owned fixture teardown closes process/listener and isolated roots. |
| `CASE-R04` | API event/replay streams, service-mode idle lifecycle, observability, and shared fixture shutdown | Stream/session/runtime shutdown assertions pass; no package hang or listener residue observed. |
| `CASE-R05` | Canceled primary-result resolution and runtime invocation boundaries | Cancellation assertions pass; no later success or surviving tracked stream was observed. |
| `CASE-R06` | Provider-error isolation and multi-lane runtime work | Failure remains isolated and public status/event facts pass. |
| `CASE-R07` | Multiple runtime Work items, JS shared-worker concurrency, Petri capacity/serial concurrency, and concurrent result correlation | Identity/correlation assertions pass; bounded shared fixtures close their tracked resources. |
| `CASE-R08` | Canonical event replay/projection and JavaScript durability/resume | Replay/resume assertions pass without reviving the predecessor fixture. |
| `CASE-R09` | Live/replay event timeline, response/event ordering, and named stage/parallel ordering | Ordered public assertions pass; stream ownership remains explicit. |
| `CASE-R10` | Duplicate document targets, repeated cleanup paths, empty JS stages/collections, duplicate-dispatch cases, and Petri discovery | Boundary assertions pass except the pre-repair Petri `-list` ledger witness, which exits 1 before repair. |
| `CASE-O01` | JavaScript success, Petri success, shared-worker success, and multi-stage pipelines | Results/events/dispatch assertions pass; session/route/process cleanup is observed. |
| `CASE-O02` | Missing imports/inputs, syntax/type diagnostics, denied policy operations, invalid permissions | Authored/public diagnostics and no-dispatch assertions pass. |
| `CASE-O03` | Petri executor panic, worker/provider/command failures, and failure-terminal routing | Failed-terminal/public failure assertions pass; isolated panic process closes. |
| `CASE-O04` | Interrupted JavaScript durability/resume and failure boundaries | No false success or repeated completed child observed; cleanup closes. |
| `CASE-O05` | JavaScript partial failure and Petri failure routing | Documented partial/failure semantics pass; later unauthorized dispatch is absent. |
| `CASE-O06` | JavaScript parallel composition and Petri concurrent dispatch | Correlation and declared ordering pass under bounded concurrency. |
| `CASE-O07` | JavaScript checkpoint persistence and interrupted-session resume | Completed children are not repeated and final result restores. |
| `CASE-O08` | Named JavaScript stages, parallel result ordering, and Petri terminal routing | Declared order and one terminal outcome pass. |
| `CASE-O09` | Empty stage/collection, duplicate dispatch, repeated close, and Petri `go test -list` | Empty/duplicate/cleanup assertions pass; discovery retains the one failing ledger witness. |

### Complete executable identity census

The 117 top-level identities below are the planning-baseline census. Nested
identities are recorded after it so a later repair can prove parity without
silently removing, skipping, or weakening a scenario.

#### Runtime API (25)

```text
TestCleanupSmoke_BackendDashboardAndCanonicalEventsExposeOnlyCleanedFactorySurfaces
TestAPIEventReplaySmoke_PublicEventsAndSessionProjectionExposeActiveAndCompletedTimeline
TestFunctionalServerOverrideCompatibilityRegression_MockWorkersAndProviderOverride
TestInferenceEvents_ModelProviderAttemptsRecordInCanonicalHistoryAndArtifact
TestNamedJavaScriptFactoryRunResolvesInvocationInputThroughCLI
TestJavaScriptSyncExecutionResolvesStructuredInvocationInput
TestModelTransportSmoke_PullUsesConfiguredLegacyCacheWithoutNetwork
TestModelTransportSmoke_ServiceModeStartupAndDirectModelRoutesStayAligned
TestSubmitMultipleRuntimeWorkItemsCompletes
TestProviderErrorSmoke_ThrottleFailureIsolatesOtherLaneThroughPublicSession
TestRuntimeConfigAlignmentSmoke_CanonicalOnlyBoundaryStaysAlignedAcrossExecutionAndRejectsRetiredAliases
TestFunctionalAPIServer_UsesProductionRuntimeFileLoggingDefault
TestFunctionalAPIServer_RuntimeLogDirectoryIsAProcessInput
TestServiceConfigOverrideAlignment_FunctionalHTTPServerProviderCommandRunner
TestServiceModeSmoke_EmptyStartupIdleSubmissionAndPostCompletionIdleStayReachableUntilCanceled
TestObservabilitySmoke_PublicStatusSessionWorkAndEventsAlignAcrossRuntimeTransitions
TestSessionInvocationAPI_AcceptsStructuredArgsWithActiveSignature
TestAPIUnifiedEventLogSmoke_LiveRecordReplayProjectionAndDivergenceUseSameTimeline
TestWorkRootPolicySlicesRejectUnsupportedOperations
TestWorkRootPolicyServiceResolvePrimaryResultHonorsCanceledContext
TestWorkRootPolicyServicePrepareInvocationInputRejectsWhitespaceOnlyText
TestWorkRootPolicyServicePrepareInvocationInputAcceptsDirectArgs
TestWorkRootPolicyServiceResolvePrimaryResultSubmittedTerminalSuccess
TestWorkServiceApplicationSlicesExerciseFunctionalLane
TestDashboard_EngineStateSnapshot_EndToEnd
```

#### Runtime API factory transformation (39)

```text
TestCurrentFactoryEvents_InitialStructureIncludesBundledFileContent
TestCurrentFactoryPUT_DocsCreateEditRenameDeleteRoundTrip
TestCurrentFactoryPUT_DocsSaveEmitsFactoryChangeWithBundledFilesAndVersion
TestCurrentFactoryPUT_RejectsInvalidDocTargets
TestCurrentFactoryPUT_RejectsDuplicateDocTargetPaths
TestCurrentFactoryPUT_ShellEscapedBundledInlineReplayReturnsPayloadInvalid
TestCurrentFactoryEvents_ExposePortableLayoutOnInitialStructureAndFactoryChange
TestCurrentFactoryPUT_PreservesPortableLayoutThroughSaveReloadAndRuntimeExecution
TestCurrentFactoryPUT_PrunesStaleLayoutWithoutReturningEphemeralLayoutMetadata
TestCurrentFactoryPUT_AcceptsLayoutNodeMissingSize
TestCurrentFactoryPUT_AcceptsLayoutForKnownBundledDocNode
TestCurrentFactoryPUT_RejectsLayoutForUnknownBundledDocNode
TestCurrentFactoryPUT_PrePersistLayoutFailureRetainsStructuredPath
TestCurrentFactoryPUT_AcceptsPortableLayoutMultipleNodesWithSize
TestCurrentFactoryPUT_AcceptsPortableLayoutEdgeWithOneWaypoint
TestCurrentFactoryPUT_AcceptsPortableLayoutEdgeWithMultipleWaypoints
TestCurrentFactoryPUT_AcceptsPortableLayoutMultipleNodesWithoutSize
TestCurrentFactoryPUT_NonDefaultSessionImportIsolatesDefaultFactoryAndMaterializesBundledFiles
TestCurrentFactoryPUT_SaveEditableCurrentFactoryDefinitionEmitsCanonicalFactoryChangeEvent
TestCurrentFactoryPUT_FactoryChangeVersionsAdvanceOnEverySave
TestCurrentFactoryPUT_SaveDefaultFactoryDefinitionPersistsAndRunsReplacement
TestCurrentFactoryPUT_DefaultFactoryAcceptsFullCurrentFactoryReadbackDocument
TestCurrentFactoryPUT_DefaultFactoryMaterializesBundledFilesAndReturns
TestCurrentFactoryPUT_SessionScopedNamedFactoryTransformationReadbackIsIsolated
TestCurrentFactoryPUT_ReturnsMultipleTopologyValidationTargets
TestCurrentFactoryPUT_ReturnsCanonicalTopologyValidationTargets
TestCurrentFactoryPUT_RejectsTypeCountCollisionBeforePersistingDefaultFactory
TestCurrentFactoryPUT_RequiresAdvancedSaveVersion
TestFactoryTransformation_ReplaceCurrentImportMatchesCreateNamedSplitLayout
TestFactoryTransformation_CreateNamedFactoryPreservesPortableLayoutThroughActivationAndReadback
TestFactoryTransformation_UpsertNamedFactoryReplacePreservesPortableLayout
TestFactoryTransformation_CreateNamedFactoryReadbackAndWorkSurface
TestFactoryTransformation_NamedFactoryPortableFilesReadBackThroughCanonicalContract
TestFactoryTransformation_CreateNamedFactory_ReturnsBobOnFailureTarget
TestFactoryTransformation_CreateNamedFactory_ReturnsMultipleTopologyValidationTargets
TestFactoryTransformation_CreateNamedFactoryEmitsCanonicalFactoryChangeEvent
TestSessionFactoryPUT_UpsertCreateAllowsOmittedVersion
TestSessionFactoryPUT_UpsertReplaceRejectsStaleVersion
TestSessionFactoryPUT_UpsertReplaceDoesNotReturnAlreadyExists
```

#### JavaScript and Petri top-level identities (53)

```text
TestJavaScriptParallelDispatchesChildrenConcurrently
TestJavaScriptParallelPreservesDeclaredResultOrdering
TestJavaScriptParallelPartialFailureUsesDocumentedPolicy
TestJavaScriptNamedStagesExposeOrderedProgress
TestJavaScriptEmptyStageProducesDocumentedResult
TestJavaScriptPipelinePassesStageOutputToNextStage
TestJavaScriptPipelineStopsAfterStageFailure
TestJavaScriptForEachDispatchesEveryInputOnce
TestJavaScriptForEachPreservesInputResultCorrelation
TestJavaScriptForEachEmptyInputDoesNotDispatch
TestJavaScriptAgentReturnsUnaryResult
TestJavaScriptAgentFailureReturnsStableFailureRecord
TestJavaScriptNestedPipelineParallelCompositionCompletes
TestJavaScriptNestedFailureNamesChildAndStage
TestJavaScriptInvocationReceivesStringNumberBooleanObjectAndArrayInputs
TestJavaScriptMissingRequiredInputFailsBeforeChildDispatch
TestJavaScriptReturnValueMapsToPrimaryInvocationResult
TestJavaScriptStructuredArtifactsMapToPublicResult
TestJavaScriptUnsupportedReturnValueFailsWithoutPrivateVMDetails
TestJavaScriptChildProgressPublishesCanonicalResponseEvents
TestJavaScriptTerminalResultFollowsFinalResponseEvent
TestJavaScriptPhaseCheckpointLifecyclePublishesCanonicalFactoryEvents
TestJavaScriptInterruptedSessionResumesWithoutRepeatingCompletedChildren
TestJavaScriptResumeRestoresCheckpointAndFinalResult
TestJavaScriptDurabilityPersistsSnapshotsByDefault
TestJavaScriptFactoryFileRunsRelativeImportsFromFactoryRoot
TestJavaScriptFactoryMissingImportFailsActionably
TestInlineJavaScriptFactoryRunsFromCLI
TestInlineJavaScriptFactoryRunsOrderedTwoStagePipeline
TestInlineJavaScriptSyntaxErrorReturnsSourceLocation
TestTypeScriptFactoryTranspilesAndRuns
TestTypeScriptTypeOrSyntaxFailureReturnsCustomerDiagnostic
TestTypeScriptSourceMapReportsAuthoredLocation
TestNamedJavaScriptFactoryRunsThroughStandardCLI
TestNamedJavaScriptFactoryRunsThroughAPIInvocation
TestNamedJavaScriptFactoryUsesSameFactorySessionControls
TestJavaScriptDeniedChildOperationReturnsStablePolicyDiagnostic
TestJavaScriptPolicyFailureDoesNotDispatchExternalWork
TestJavaScriptSharedWorkerBehavior
TestPetriAndJavaScriptSessionsShareLifecycleControls
TestPetriAndJavaScriptSessionsExposeCompatibleStatusFacts
TestLogicalRoundTripFactoryBoundaryProvesProductiveRejectionsSurvive
TestLogicalRoundTripFactoryBoundaryStopsUnbalancedRoute
TestLogicalRoundTripFactoryBoundaryRecordReplayPreservesTerminalProjection
TestDependsOnSecondaryJoinedInput
TestPetriIndependentWorkDispatchesConcurrently
TestPetriConcurrentResultsCorrelateToOriginalWork
TestPetriConcurrentFailureDoesNotDuplicateDispatch
TestPetriExecutorPanicRoutesToFailedTerminal
TestPetriSharedDispatchSuccess
TestPetriWorkerErrorReturnsFailedTerminalOutcome
TestPetriExecutorDispatchTerminalRouting
TestPetriInvocationInputAndOutputMapping
```

### Nested scenario census and handoff

The 102 nested runtime identities are grouped by their parent top-level test:

```text
TestFunctionalServerOverrideCompatibilityRegression_MockWorkersAndProviderOverride/{StartFunctionalServerMockWorkersCompletes,ProviderOverrideIsAppliedBeforeServiceBuildForHTTPRuntime}
TestRuntimeConfigAlignmentSmoke_CanonicalOnlyBoundaryStaysAlignedAcrossExecutionAndRejectsRetiredAliases/{canonical_split_factory_stays_aligned_across_flatten_replay_and_execution,generated_factory_json_rejects_retired_worker_provider_alias,generated_factory_json_rejects_retired_workstation_resource_usage_alias,split_worker_frontmatter_rejects_retired_model_provider_alias,split_workstation_frontmatter_rejects_retired_runtime_type_alias,split_workstation_frontmatter_rejects_retired_cron_trigger_at_start_alias}
TestWorkRootPolicySlicesRejectUnsupportedOperations/{policy_admission,policy_content_staging,materialization_admission_prep,materialization_state_access,materialization_content_staging,materialization_invocation_policy,admission_materialization,admission_invocation_input,admission_primary_result,admission_state_access}
TestCurrentFactoryPUT_RejectsInvalidDocTargets/{outside_docs_root,non_canonical_path,escaping_path}
TestCurrentFactoryPUT_SessionScopedNamedFactoryTransformationReadbackIsIsolated/{alpha,beta}
TestCurrentFactoryPUT_RequiresAdvancedSaveVersion/{lower_logical_lower_physical_fails,same_logical_equal_physical_fails,lower_logical_greater_physical_fails,same_logical_greater_physical_fails,missing_version_fails,missing_logical_fails,missing_physical_fails,greater_logical_and_physical_passes,listener_cleanup_probe_classification}
TestCurrentFactoryPUT_RequiresAdvancedSaveVersion/listener_cleanup_probe_classification/{reachable_response_fails_cleanup,accepted_but_unresponsive_listener_fails_cleanup}
TestJavaScriptSharedWorkerBehavior/{spine/permission-matrix-cli-success,spine/invalid-permission-pre-dispatch-failure,spine/reverse-order,permissions/command-shaping,permissions/command-shaping/permissions-omitted,permissions/command-shaping/permissions-default,permissions/command-shaping/permissions-skip,permissions/disallowed,antigravity/model-embedded-effort,antigravity/model-embedded-effort/executorProvider=,antigravity/model-embedded-effort/executorProvider=SCRIPT_WRAP,antigravity/typed-rejection,providers/live-command-edge,providers/permission-flags,providers/permission-flags/permissions_default_dynamic,providers/permission-flags/permissions_skip_dynamic,providers/permission-flags/permissions_default_static,providers/permission-flags/permissions_skip_static,providers/permission-flags/neither,providers/invalid-permissions,providers/invalid-permissions/invalid-enum,providers/invalid-permissions/invalid-type,providers/distinct-provider-model,mock-workers/partial-passthrough,overrides/unknown-provider,isolation/concurrent-success-failure}
TestPetriIndependentWorkDispatchesConcurrently/{capacity_limited_concurrency_completes_all_work,serial_concurrency_limit_completes_all_work}
TestPetriConcurrentResultsCorrelateToOriginalWork/{single_seed_correlates_to_terminal_work,two_concurrent_seeds_each_reach_terminal_work,multi_seed_completions_remain_distinct,concurrent_execution_pool_keeps_distinct_work_identities}
TestPetriSharedDispatchSuccess/{simple_single_worker_pipeline_completes,preseeded_work_reaches_success_terminal,mixed_preseeded_and_late_submit_completes,archive_terminal_work_completes_without_refire,two_stage_pipeline_reaches_terminal,scaffolded_simple_pipeline_completes_one_task,ideation_happy_path_reaches_story_complete,dispatcher_workflow_single_idea_reaches_prd_complete,dispatcher_lifecycle_idea_reaches_archived_terminal,ideation_rejection_loop_reaches_story_complete,idea_plan_execute_review_reaches_task_complete,idea_to_prd_multiple_ideas_each_reach_terminal,config_driven_happy_path_two_stage_completes,noop_pipeline_completes_without_provider,service_simple_multiple_work_items_complete,scaffolded_multiple_work_items_complete_independently,scaffolded_two_stage_service_pipeline_completes,factory_model_maps_to_provider_invocation,work_payload_maps_into_provider_user_message,work_name_maps_into_invocation_prompt,cross_work_type_terminal_preserves_origin_trace,dispatch_events_reference_terminal_work_identity}
TestPetriWorkerErrorReturnsFailedTerminalOutcome/{mock_provider_error_routes_to_failed_terminal,provider_command_exit_routes_to_failed_terminal,rejected_worker_outcome_routes_to_failed_terminal,planner_failure_routes_idea_to_failed,executor_failure_routes_prd_to_failed,idea_to_prd_planner_command_failure_routes_idea_to_failed,idea_plan_execute_review_script_failure_routes_plan_to_failed,idea_plan_execute_review_planner_failure_routes_idea_to_failed,idea_plan_execute_review_processor_exhaustion_routes_task_to_failed}
TestPetriExecutorDispatchTerminalRouting/{provider_process_failure_without_failure_arcs_routes_to_failed,provider_nonzero_exit_without_failure_arcs_routes_to_failed,provider_failure_with_failure_arcs_routes_to_failed_not_done,provider_success_leaves_work_at_authored_done_place}
TestPetriInvocationInputAndOutputMapping/failed_terminal_preserves_origin_trace_lineage
```

The nested grouping is an identity ledger, not a source scan or meta-test. The
discovery failure is isolated to Petri `TestMain` ledger classification; the
full execution ledger is healthy. Story 004's smallest plan delta is to make
discovery mode skip runtime-cardinality assertions while retaining the full
44-row execution assertion, then add only the package-local route/root probes
needed to make the already observed cleanup ownership explicit. Repeat, race,
platform, clean-room, final lane, CI, and merge evidence remain for later
stories.
