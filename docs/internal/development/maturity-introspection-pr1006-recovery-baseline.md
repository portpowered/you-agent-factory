# PR #1006 Factory Session Introspection Recovery Baseline

This baseline records the evidence used to recover the existing
`maturity-introspection-parity` pull request (#1006). PR #1006 remains the
durable review lane. Recovery work must update that PR rather than open a
second maturity-introspection implementation.

Evidence was captured in UTC on 2026-07-12. The reviewed feature head was
`548470ba448dd8f6416ea6d3fc4667e0d146093e`; its merge base with current main
was `dae9bb725d0a42e4d3334870631d9b83bbf160f8`. At capture time the feature
branch was 9 commits ahead of and 110 commits behind `origin/main`.

## Windows failure classification

PR run `29196823322`, job `86661184838`, ran `make test` on Windows Server
2025 and failed at 2026-07-12 14:49 UTC. The same required job passed on
current-main commit `4b1b08aa801524bf301e201d57f07c10099042a9` in run
`29201329976`, job `86673388336`, at 2026-07-12 17:13 UTC.

The failing head predates main commit `ba054cf229419ccf80fcd780f58b4485a69980d8`
(`make Windows short suite blocking and portable`). That commit corrects the
observed portability families at their owning boundaries. Later main commits
`19b7d3e40`, `7014131e6`, `81c132a81`, and `6214ece8c` reconcile the named
factory path and test-layout changes implicated by the CLI/config failures.
None of the Windows-owning files below are changed by the PR #1006 feature
diff, except unrelated additions in `pkg/transports/cli` session-inspection files.
Therefore the failures are current-main drift, not introspection regressions.

| Observable failure in PR job | Concrete assertion/output | Current-main correction |
| --- | --- | --- |
| JSON/path spelling | `TestRunInvokeCallsRealBackendAndReturnsAudioContent` compared a JSON-escaped Windows path with raw path bytes. | Decode the response and compare the typed file value (`ba054cf22`). |
| GNU Make diagnostics | Boundary/file-count tests expected `*** [target]`; Windows Make includes `Makefile:<line>:`. | Match the stable `target] Error` diagnostic (`ba054cf22`). |
| User-home isolation | Named-factory, default-provider, and session-path tests set `HOME`, but Windows resolves through `USERPROFILE`. | Isolate both environment variables and normalize logical keys (`ba054cf22` and named-factory follow-ups). |
| Windows quoting and separators | Startup diagnostics, prompt errors, content URLs, tool arguments, and working-directory assertions assumed POSIX slash/quote spelling. | Compare parsed/normalized values or use platform-aware quoting/path conversion (`ba054cf22`). |
| POSIX file modes | Portable bundled-file tests required `0644`/`0755`, which Windows filesystems do not represent. | Assert portable content/behavior and platform-supported executable semantics (`ba054cf22`; obsolete layout tests were removed by `7014131e6`). |
| Process and locked-file cleanup | Timeout/cancellation and cache-release tests observed Windows process/handle timing. | Use the Windows process boundary and deterministic cleanup behavior (`ba054cf22`). |
| Make executable quoting | Verification smokes passed raw `C:\...\make.exe` and UI script paths through Bash, which stripped separators. | Quote environment-provided executables and invoke scripts through Bash (`ba054cf22`). |
| Direct shell-script execution | Go tests attempted to execute `.sh` files as Win32 applications. | Route repository scripts through the configured Bash executable (`ba054cf22`). |
| Shared factory state | Factory-save/runtime and packaged-goal cases inherited global home/factory data or stale on-disk versions. | Isolate Windows home roots and use current named-factory/save fixtures (`ba054cf22` plus the named-factory follow-ups). |

The complete set of unique failing test labels is retained below so every CI
failure can be mapped to the table rather than treated as an unexplained suite
failure:

- JSON/path spelling: `TestRunInvokeCallsRealBackendAndReturnsAudioContent`,
  `TestFilesystemPathToContentURL`, `TestMaterializeContentURL_LocalFileOK`,
  `TestMaterializeContentURL_LocalMissing`,
  `TestResolveDispatchContentURL_RelativeFileURLJoinsWorkingDirectory`, and
  `TestURLMaterializationCorpus_CasesMatchExpectedOutcomes` (`local_file_ok`).
- GNU Make diagnostics: `TestMakePkgBoundaryTargetFailsForUnapprovedRootPackageFamily`,
  `TestMakeLintPathFailsForUnapprovedRootPackageFamily`, and
  `TestMakeLintPathFailsForOversizedOwnedPackage`.
- Home, named-factory, and registry isolation:
  `TestRunCommand_NamedFlagPrefersProjectFactoryOverGlobal`,
  `TestRunCommand_NamedFactoryResolutionMetadataFlowsForBuiltInGoal`,
  `TestRunCommand_RepeatedBuiltInGoalRunReusesMaterializedCopy`,
  `TestRootCommand_DefaultProviderFlagResolvesSymbolicDefaultFromFile`,
  `TestDefaultNamedFactoryRoots`, `TestExpandFolderHome_ExpandsTildePrefix`,
  `TestResolveSessionFolder_ExpandsTildeBeforeStat`,
  `TestLogicalSessionKeyID_DefaultTargetUsesStableKey`,
  `TestLogicalSessionKeyID_NamedTargetIncludesFactoryName`, and
  `TestRegistry_FindByLogicalSessionKeyID_ReturnsMatchingSession`.
- Path quoting, separator, and safe-diagnostic behavior:
  `TestRun_VerboseStartupDiagnosticsReportResolvedRuntimeMetadata`,
  `TestMaterializedPackagedGoalFactory_SplitRolePromptRegressionFailsWhenPromptMissing`
  (planner, executor, checker, summarizer),
  `TestMaterializedPackagedGoalFactory_SplitRolePromptRegressionFailsWhenPromptMiswired`
  (planner, executor, checker, summarizer),
  `TestFormatAgentRunError_AbsolutePathArgumentOmitsPath`,
  `TestPolicyToolExecutor_FailureDiagnosticsExcludeAbsolutePaths`,
  `TestToolRelativePathFromArguments_OmitsUnsafePaths`,
  `TestCodexContentDispatch_MixedContentEmitsOrderedImageArgs`,
  `TestCodexProviderBehavior_BuildArgs_MaterializesLocalFileURLWithoutCopy`,
  `TestScriptWrapProvider_Infer_CodexBatchLocalAndRemoteImageURLs`,
  `TestScriptWrapProvider_Infer_CodexImageContentEmitsOrderedImageArgs`, and
  `TestWorkstationExecutor_ResolvesTemplatedWorkingDirectoryFromSessionContext`.
- Portable layout and file-mode behavior:
  `TestMaterializePortableBundledFiles_RestoresExplicitMakefileWithMode`,
  `TestPortableBundledFiles_ExpandRestoresExplicitMakefileFromManifest`, and
  `TestPersistNamedFactory_StripsSupportedBundledFileInlineContentFromFactoryJSON`.
- Process, provider, cache, and worker portability:
  `TestDispatchCache_ReusesURLAndCleansUpOnRelease`,
  `TestExecCommandRunner_LogsCancelCleanupForceKillSuccess`,
  `TestScriptExecutor_TimeoutStopsProcessBeforeItCanFinish`,
  `TestMockWorkerCommandRunner_RunNextUsesExecRunnerWhenNextMissing`,
  `TestScriptPollerCommandRequest_ResolvesWorkstationEnvAndWorkerTimeout`,
  `TestInferenceProgressPublishingCommandRunner_CursorPublishesDiagnosticsAndLaterValidEventsInOrder`,
  `TestInferenceProgressPublishingCommandRunner_MapsFailureCancelAndTruncation`,
  `TestInferenceProgressPublishingCommandRunner_MapsUnknownAndMalformedCodexEventsToBoundedDiagnostics`,
  `TestInferenceProgressPublishingCommandRunner_NormalizesCodexStructuredEvents`,
  `TestInferenceProgressPublishingCommandRunner_PublishesOrderedFragments`,
  `TestInferenceProgressPublishingCommandRunner_WithoutPublisherPreservesExecBehavior`,
  and `TestScriptWrapProvider_Infer_CursorErrorFlaggedSuccessPublishesOnlyCanonicalFailure`.
- Verification-command portability:
  `TestVerifyFastCommandSmoke_UsesOnlyShortOwnedSuites`,
  `TestVerifyFastCommandSmoke_FailureReportsOwnedSuiteAndRerunCommand`,
  `TestVerifyPRCommandSmoke_UsesRequiredLanesOnce`,
  `TestVerifyPRCommandSmoke_FailureReportsExactLaneRerun`,
  `TestUICoverageCommandSmoke_RunsPackageCoverageThenReplayCheck`,
  `TestUIPackageCoverageCommandSmoke_InvokesPackageOwnedCoverageScript`,
  `TestVerifyCompatibilityAliasSmoke_RedirectsToCanonicalPRTier`,
  `TestBackendVerificationLaneScriptSmoke_UsesCanonicalOwnedCommandAndCapturesLog`,
  `TestBackendVerificationLaneScriptSmoke_PreservesFailureExitAndLog`,
  `TestConcurrentUIVerificationLanesScriptSmoke_RunsBothOwnedLanesConcurrently`,
  `TestConcurrentUIVerificationLanesScriptSmoke_FailureReportsExactLaneRerun`,
  `TestShardedUICoverageScriptSmoke_RunsAllShardsThenMerge`,
  `TestShardedUICoverageScriptSmoke_CleansStaleVitestReportBlobsBeforeShards`,
  `TestShardedUICoverageScriptSmoke_FailureReportsExactShardRerun`,
  `TestVerifyExtendedCommandSmoke_UsesOnlyExplicitLongSuitesAfterPRTier`,
  `TestLongTestsCommandSmoke_FailureReportsExactSpecialtyLaneRerun`,
  `TestVerifyPRInferenceCommandSmoke_RunsSingleNamedRegressionOnly`, and
  `TestVerifyPRInferenceCommandSmoke_StaysOutsideRequiredPRAndExtendedTiers`.
- Shared factory/save fixture isolation:
  `TestBuildReplacementFactoryRuntime_ServiceModeStaysRunningUntilCanceled`,
  `TestCurrentFactoryEvents_ExposePortableLayoutOnInitialStructureAndFactoryChange`,
  all `TestCurrentFactoryPUT_*` cases reported by the job,
  `TestFactoryService_CreateNamedFactory_MaterializesSupportedPortableBundledFiles`,
  all reported `TestFactoryService_SaveCurrentFactory*` and
  `TestFactoryService_SaveFactoryForSession*` cases,
  `TestFactoryTransformation_ReplaceCurrentImportMatchesCreateNamedSplitLayout`,
  `TestFactoryTransformation_UpsertNamedFactoryReplacePreservesPortableLayout`,
  `TestSessionFactoryPUT_UpsertReplaceDoesNotReturnAlreadyExists`,
  `TestGeneratedAPIIntegrationSmoke_SubmitWorkItemsAcceptMixedTextAndImageSubmissionOnSupportedRunner`,
  and `TestSessionInvocationAPI_PackagedGoalBlockedReturnsBlockedStatusDetails` /
  `TestSessionInvocationAPI_PackagedGoalNeedsHumanReturnsNeedsHumanStatusDetails`.
- Packaged-factory fixture isolation:
  `TestPackagedGoalBuiltInTopologyScaffold_PrimaryResultIsExecutionSummaryNotReviewLabel`,
  all reported `TestPackagedGoalBuiltInTopology_*` lifecycle/review cases, and
  `TestPackagedTTSInvocationPrimaryResult_ReturnsMetadataNotRawAudio`.

## Canonical introspection behavior already present

PR #1006 adds one event-derived Factory Session read model owned by
`pkg/factorysessionexecution`; it does not add a transport-owned source of
truth. Ordered session events and runtime records project:

- current lifecycle and orchestrator phase plus ordered phase summaries;
- the latest checkpoint reference;
- canonical Dispatch summaries and counts for queued, running, completed,
  failed, canceled, timed out, skipped, and interrupted outcomes;
- artifact references and Dispatch-to-artifact lineage;
- session/Dispatch usage, policy, budget, source, and lifecycle facts; and
- explicit not-ready, partial, final, failed-with-partial, and unavailable
  result states.

The REST handlers and generated OpenAPI contract map that domain projection.
CLI JSON and human output read the same session/result/Dispatch queries. MCP
uses the same `factorysessionexecution.QueryDispatches` filter boundary. The
dashboard consumes the generated contract and explicitly loads required
Dispatch and partial/final-result reads; supplemental transport failures reject
to its error state rather than becoming successful missing data.

The latest PR conversation review explicitly clears the earlier canonical
Dispatch filtering, status-group counts, dashboard error-state, and browser
coverage findings on head `548470ba4`. The only remaining blocking comment is
the Windows check, which the current-main evidence above attributes to drift.

## Recovery boundary

The next recovery step is to incorporate current main into the durable PR lane,
preserving the 52-file introspection feature diff and the mainline Windows
portability fixes. Do not rebuild the model, open a replacement PR, introduce a
parallel projection, or absorb adjacent Batch 004 maturity work unless a
current-main commit is required to reconcile the existing feature.
