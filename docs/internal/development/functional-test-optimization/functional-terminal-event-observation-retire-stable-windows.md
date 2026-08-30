# Functional terminal event observation and stable-window retirement

## Story 001 — frozen baseline ledger

Status: **PASS for functional-terminal-event-observation-retire-stable-windows-001**.
This is the pre-change evidence artifact only. It does not claim observer
correctness, migration completeness, after-change performance, clean-checkout
validation, or PR CI.

## Scope and authority

- PRD: prd.json, project Retire functional terminal-status stable windows through canonical event observation.
- Behavior lane: BEH-TERM-OBS — functional scenarios synchronize completion on the canonical terminal Factory Event while preserving observable values.
- Source-plan authority: null; no operatorAmendment was present.
- Story scope: census, ownership handoffs, assertion inventory, build-tag map, and the exact 17-package before baseline. No helper, caller, production, API, generated, UI, or excluded-lane implementation was changed.
- Dependency fidelity: the packages' existing root-built functional wiring and controlled local edges; this ledger reads the existing source and records the package commands exactly as run.
- Cost/network: local fixtures only, no paid calls.

## Baseline identity

| Field | Observed value |
| --- | --- |
| Branch | functional-terminal-event-observation-retire-stable-windows |
| Exact source head | a44ed015421b1ed42b919f1178531f38fd5087b5 |
| origin/main at census | a44ed015421b1ed42b919f1178531f38fd5087b5 |
| Rebase procedure | git fetch origin main --prune; git rebase origin/main |
| Rebase result | Current branch ... is up to date |
| Worktree before baseline | clean except ignored scaffolding (prd.json, prd.md; progress.txt was absent) |
| Go | go version go1.25.0 windows/amd64 |
| Host | Windows 11 Home build 26200, amd64, 24 logical processors |
| Timing capture | PowerShell Measure-Command, wall values rounded to the nearest millisecond by the capture script |

The head is the post-rebase implementation head, not the stale planning-head
reference in the PRD's historical notes.

## Census procedure and result

The family census command was:

    rg -n 'WaitForTerminalStatus|terminalObservationStableWindow|RunFactoryToCompletionWithEdgesAndWorkStable|RunFactoryToCompletionWithEdgesAndObservationsStable' tests/functional

The source has 58 external direct support.WaitForTerminalStatus callsites
and one internal support call at
tests/functional/internal/support/process_factory.go:409 (59 direct helper
call hits in total). The declaration is at http_observation.go:86; its 300 ms
state is at http_observation.go:92 and its 10 ms status ticker is at
http_observation.go:419. The stable family has 21 WorkStable callers, two
owned ObservationsStable callers, and one excluded
ObservationsStableBeforeClose caller.

The following tables group only identical test/function ownership; every line
number is one census hit and appears once.

### Shared support definitions and internal mode uses

| Exact location | Symbol/use | Build | Disposition |
| --- | --- | --- | --- |
| tests/functional/internal/support/http_observation.go:86 | WaitForTerminalStatus declaration | default | owned shared support |
| tests/functional/internal/support/http_observation.go:92 | stableWindow = 300 * time.Millisecond | default | legacy implementation to retire in Story 003 |
| tests/functional/internal/support/http_observation.go:419 | time.NewTicker(10 * time.Millisecond) in waitForStatusAt | default | legacy implementation to retire in Story 003; other WaitForStatus callers are not silently reclassified |
| tests/functional/internal/support/process_factory.go:66-70 | terminalObservationMode; correlated and stable-window modes | default | owned shared support |
| tests/functional/internal/support/process_factory.go:134 | RunFactoryToCompletionWithEdgesAndWorkStable declaration | default | owned wrapper |
| tests/functional/internal/support/process_factory.go:146 | stable-window mode passed by WorkStable wrapper | default | owned wrapper path |
| tests/functional/internal/support/process_factory.go:167 | RunFactoryToCompletionWithEdgesAndObservationsStableBeforeClose declaration | default | excluded caller compatibility handoff |
| tests/functional/internal/support/process_factory.go:182 | stable-window mode passed by BeforeClose wrapper | default | excluded wrapper path |
| tests/functional/internal/support/process_factory.go:408-409 | stable-mode branch and internal WaitForTerminalStatus call | default | owned shared support path |
| tests/functional/internal/support/process_factory_functionallong.go:16,28 | RunFactoryToCompletionWithEdgesAndObservationsStable declaration and stable-mode use | functionallong | owned wrapper |

### External direct helper callsites

| Exact path and line(s) | Owning test/function(s), in line order | Build | Family | Disposition |
| --- | --- | --- | --- | --- |
| tests/functional/events/factory_events/order_and_cursor_test.go:46,87,132,230,343 | TestAPIGetFactoryEventsReturnsOrderedDurableHistory; TestAPIEventCursorReturnsOnlyNewerEvents; TestAPIInvalidEventCursorReturnsTypedError; TestAPISubmitWorkEmitsCanonicalTraceAwareBatchEvent; TestFactoryEventStreamReconnectHasNoGapOrDuplicate | default | direct | OWNED |
| tests/functional/events/response_events/stream_test.go:254,264,367,407 | TestAPIResponseEventCursorGapEmitsStreamGap (two waits); TestAPIResponseEventSSEStreamsRetainedThenLiveEvents (two waits) | default | direct | OWNED |
| tests/functional/factory/definitions/defaults_test.go:429 | runFactoryWithOperatorHome (called by global-default, explicit-override, and single-discovered-provider tests) | default | direct | OWNED |
| tests/functional/factory/replay_contracts/canonical_event_admission_replay_test.go:40 | TestCanonicalRecordReplayPreservesAdmittedFacts | default | direct | OWNED |
| tests/functional/factory/replay_contracts/composed_record_replay_test.go:38,59 | TestComposedRecordReplayUsesRootBuildProcessAndExecute (record and replay) | default | direct | OWNED |
| tests/functional/factory/replay_contracts/work_snapshot_projection_test.go:37 | TestComposedWorkSnapshotReaderProjectsCanonicalState | default | direct | OWNED |
| tests/functional/factory/visualization/runtime_metrics/replay_priced_cost_test.go:52 | TestReplayPricedUsageReachesPublicCosts | default | direct | OWNED |
| tests/functional/factory/visualization/runtime_metrics/replay_unpriced_cost_test.go:46 | TestReplayOperatorPriceTableIsReversibleInPublicCosts | default | direct | OWNED |
| tests/functional/factory/visualization/runtime_metrics/runtime_artifact_flow_test.go:58 | TestRuntimeMetricsAndArtifactsThroughRootProcess | default | direct | OWNED |
| tests/functional/internal/support/process_factory.go:409 | runFactoryToCompletionWithHome internal completion branch | default | direct | OWNED shared support |
| tests/functional/internal/support/work_location_test.go:127 | TestCountWorkAtCustomerState_SupportBackedScenarioReachesTaskCompleteWithoutPetriHelpers | default | direct | OWNED |
| tests/functional/operator_settings/root_composition/operator_config_activation_test.go:80 | TestOperatorConfigLoadAndDefaultsResolutionActivateThroughRootBuildProcessAfterLifecycle | default | direct | OWNED |
| tests/functional/recordings/root_composition/event_artifact_projection_activation_test.go:50 | TestRecordingsEventArtifactProjectionSurfacesActivateThroughRootBuildProcessAfterLifecycle | default | direct | OWNED |
| tests/functional/recordings/root_composition/portable_transport_activation_test.go:58 | TestRecordingsPortableBuildValidateAndTransportsActivateThroughRootBuildProcessAfterLifecycle | default | direct | OWNED |
| tests/functional/recordings/root_composition/record_replay_lifecycle_activation_test.go:45,71,98 | TestRecordReplayLifecycleActivatesThroughRootBuildProcessAfterLifecycle (record, replay, resume) | default | direct | OWNED |
| tests/functional/replay_contracts/replay_factory_only_serialization_smoke_long_test.go:50,68 | TestReplayFactoryOnlySerializationSmoke_RecordReplayUsesRunStartedFactoryPayload (record and replay) | functionallong | direct | OWNED |
| tests/functional/replay_contracts/replay_process_test.go:30 | observeReplayThroughRoot | functionallong | direct | OWNED |
| tests/functional/replay_contracts/replay_record_end_to_end_long_test.go:263,367 | TestRecordReplayEndToEnd_FactoryRequestBatchAndWorkerGeneratedBatchReplayDeterministically; TestRecordReplayEndToEnd_ProviderCommandDiagnosticsPersistRedactedEnv | functionallong | direct | OWNED |
| tests/functional/replay_contracts/replay_regression_harness_long_test.go:41,166 | TestReplayRegressionHarness_LoadsArtifactAndAssertsSuccessfulReplay; recordReplayHarnessFixtureArtifact | functionallong | direct | OWNED |
| tests/functional/replay_contracts/replay_runtime_config_smoke_long_test.go:45,63 | TestReplayRuntimeConfigSmoke_CanonicalWorkstationsDriveDispatchAndReplay (record and replay) | functionallong | direct | OWNED |
| tests/functional/replay_contracts/replay_thin_event_dual_dispatch_smoke_test.go:111 | runThinEventDualDispatchSmoke | default | direct | OWNED |
| tests/functional/replay_contracts/replay_work_dispatch_contract_smoke_long_test.go:202 | runRecordedWorkDispatchContractSmoke | functionallong | direct | OWNED |
| tests/functional/runtime_api/api_inference_events_test.go:51 | TestInferenceEvents_ModelProviderAttemptsRecordInCanonicalHistoryAndArtifact | default | direct | OWNED |
| tests/functional/runtime_api/api_unified_event_log_smoke_test.go:42 | TestAPIUnifiedEventLogSmoke_LiveRecordReplayProjectionAndDivergenceUseSameTimeline | default | direct | OWNED |
| tests/functional/work/recovery/manual_move_test.go:83 | TestFailedCascadeCanBeRecoveredByPublicWorkMove | default | direct | OWNED |
| tests/functional/work/relationships/parent_child_test.go:97,205 | TestParentChildLineageSurvivesDispatchAndReplay; TestChildFailureProjectsToDocumentedParentView | default | direct | OWNED |
| tests/functional/work/root_composition/recovery_recordings_visualization_activation_test.go:137 | TestWorkRecordingsReadActivatesThroughRootBuildProcessAfterLifecycle | default | direct | OWNED |
| tests/functional/work/root_composition/routing_relationship_activation_test.go:226 | parent-child subtest in TestWorkRoutingAndRelationshipsActivateThroughRootBuildProcessAfterLifecycle | default | direct | OWNED |
| tests/functional/work/submission/batch_inputs_long_test.go:68,78 | TestLegacyUnaryRetirementReplaySubmitsCanonicalBatchWorkRequests (record and replay) | functionallong | direct | OWNED |
| tests/functional/work/submission/legacy_unary_test.go:68 | assertLegacyUnaryDirectSubmitAndPut | default | direct | OWNED |
| tests/functional/workflow/config_driven_retry_loop_breaker_test.go:101 | TestConfigDrivenRetryLoopBreaker_TerminatesAfterMaxRetries | default | direct | OWNED |
| tests/functional/workflow/review_retry_exhaustion_long_test.go:35,71 | TestReviewRetryLoopBreaker_TerminatesAfterMaxRetries; TestReviewRetryLoopBreaker_FeedbackPropagated | functionallong | direct | OWNED |

These rows contain 48 external owned direct hits plus the one internal support
hit, matching 49 owned direct paths. The ten external direct hits below are
explicit handoffs and are not migration scope.

### Excluded external direct callsites

| Exact path and line(s) | Owning test/function(s) | Build | Disposition and reason |
| --- | --- | --- | --- |
| tests/functional/provider_sessions/association/response_exec_metadata_test.go:206,262 | observeResponseExecCodexGoldenReplay; recordResponseExecCodexGoldenRun | default | EXCLUDED — provider-session live lane |
| tests/functional/providers/cli_timeout_cleanup_smoke_test.go:72,123 | timeout cleanup smoke tests | default | EXCLUDED — providers live lane |
| tests/functional/providers/mock_workers_end_to_end_smoke_test.go:87 | TestMockWorkers_EndToEndSmokeRunsMixedOutcomesWithoutLiveProviderCredentials | default | EXCLUDED — providers live lane |
| tests/functional/providers/mock_workers_script_test.go:109 | TestMockWorkers_ScriptConfigExecutesCommandRunnerSideEffect | default | EXCLUDED — providers live lane |
| tests/functional/providers/packaged_script_runtime_test.go:39,77 | packaged script runtime tests | default | EXCLUDED — providers live lane |
| tests/functional/providers/runtime_logging_smoke_test.go:318 | runRuntimeLoggingSmoke | default | EXCLUDED — providers live lane |
| tests/functional/workers/script/execution_long_test.go:64 | TestWorkerPublicContractSmoke_CanonicalWorkerExecutesAndKeepsRuntimeOnlyFieldsPrivate | functionallong | EXCLUDED — workers live lane |

### External stable-family callers

| Exact path and line(s) | Owning test/function(s) | Build | Family | Disposition |
| --- | --- | --- | --- | --- |
| tests/functional/workstations/watcher/files_test.go:44,106,173,246,313,365,414 | TestWatcherSingleFileCompletesOneWork; TestWatcherSequentialFilesAllComplete; TestWatcherConcurrentFilesCompleteWithoutDuplicates; TestWatcherMixedOutcomesLeaveNoNonTerminalWorkLeak; TestWatcherDefaultChannelSubmission; TestWatcherExecutionIDDirectorySubmission; TestWatcherCombinedDefaultAndDynamicExecDirectory | functionallong | WorkStable | OWNED |
| tests/functional/workstations/watcher/files_test.go:460 | TestWatcherParentChildBatchFanIn | functionallong | ObservationsStable | OWNED |
| tests/functional/workstations/repeater/reject_accept_test.go:27,50,70 | TestRepeater_YieldsBetweenIterations; TestRepeater_ResourceReleaseBetweenIterations; TestRalphLoop_ConvergesOnReviewerAccept | default | WorkStable | OWNED |
| tests/functional/workstations/repeater/reject_accept_long_test.go:34 | TestRepeater_GuardedLoopBreakerTerminatesRejectedRepeater | functionallong | ObservationsStable | OWNED |
| tests/functional/workstations/repeater/reject_accept_long_test.go:56,80,162,188,205,224,243,267,302 | TestRepeater_RefiresOnRejectedStopsOnAccepted; TestRepeater_ResourceReleaseBetweenIterations_ServiceHarness; TestWorkstationStopWords_ThroughCustomerProcess (subtests); TestMultiOutput_WithStopWord; TestMultiOutput_WithoutStopWord; TestMultiOutput_NoStopWordsConfigured; TestMultiOutput_SecondStopWord; TestMultiOutput_OutputTokensInheritInputLineage; TestRalphLoop_TemplateFieldsResolvePerIteration | functionallong | WorkStable | OWNED |
| tests/functional/workflow/config_driven_retry_loop_breaker_test.go:127 | TestConfigDrivenRetryLoopBreaker_SucceedsBeforeLimit | default | WorkStable | OWNED |
| tests/functional/workflow/review_retry_exhaustion_long_test.go:106 | TestReviewRetryLoopBreaker_SucceedsBeforeLimit | functionallong | WorkStable | OWNED |
| tests/functional/providers/acp/packaged_conformance_test.go:83 | TestPackagedACPConformance... | default | ObservationsStableBeforeClose | EXCLUDED — providers live lane; preserves its before-close hook |

The stable table contains 21 owned WorkStable hits, two owned
ObservationsStable hits (repeater and watcher), and one excluded
ObservationsStableBeforeClose hit. Thus the external owned total is 71
completion invocations (48 direct plus 23 stable); the support implementation
call and wrapper definitions are separately listed above.

## Vanished JavaScript audit locations

The four audit-time locations are absent from current content under
tests/functional/orchestration/javascript (rg returned zero family hits).
History inspection found their removal in
31affad3e162c8d968b7375b5a8a80149037ca11, dated
2026-08-29T12:32:30-07:00, subject test: observe JavaScript fixture teardown
at real edges:

| Historical label | Old assertion | Current disposition |
| --- | --- | --- |
| composition:282 | WaitForStatus(... RuntimeStatus != "") | deleted; replaced by the API server's WaitForBaseURL readiness boundary |
| contracts:304 | same runtime-status readiness poll | deleted; replaced by WaitForBaseURL |
| durability:200 | same runtime-status readiness poll | deleted; replaced by WaitForBaseURL |
| loading:405 | same runtime-status readiness poll | deleted; replaced by WaitForBaseURL |

No migration or new work was invented for these absent sites. The nearby policy
fixture has the same real-edge treatment in the same commit but is not one of
the four PRD audit locations.

## Package build-tag map

The map was obtained by counting .go files in each exact planning-head
directory and inspecting the file-level functionallong build constraint.
Each baseline command used -tags=functionallong, so the command exercised
the default and tagged files together.

| Exact package | Go files | Default files | functionallong files |
| --- | ---: | ---: | ---: |
| ./tests/functional/internal/support | 58 | 55 | 3 |
| ./tests/functional/events/factory_events | 3 | 3 | 0 |
| ./tests/functional/events/response_events | 6 | 6 | 0 |
| ./tests/functional/factory/definitions | 16 | 15 | 1 |
| ./tests/functional/factory/replay_contracts | 7 | 7 | 0 |
| ./tests/functional/factory/visualization/runtime_metrics | 9 | 9 | 0 |
| ./tests/functional/operator_settings/root_composition | 8 | 8 | 0 |
| ./tests/functional/recordings/root_composition | 5 | 5 | 0 |
| ./tests/functional/replay_contracts | 14 | 5 | 9 |
| ./tests/functional/runtime_api | 29 | 27 | 2 |
| ./tests/functional/work/recovery | 2 | 2 | 0 |
| ./tests/functional/work/relationships | 7 | 7 | 0 |
| ./tests/functional/work/root_composition | 5 | 5 | 0 |
| ./tests/functional/work/submission | 9 | 7 | 2 |
| ./tests/functional/workflow | 3 | 2 | 1 |
| ./tests/functional/workstations/repeater | 2 | 1 | 1 |
| ./tests/functional/workstations/watcher | 1 | 0 | 1 |

## Assertion inventory for post-migration comparison

The inventory below records the exact stable values and public fields asserted
after the completion call. Dynamic identities are recorded as equality or
presence invariants rather than replaced with guessed values.

| Owner and callsite(s) | Exact pre-change assertions |
| --- | --- |
| events/factory_events/order_and_cursor_test.go:46,87,132,230,343 | Retained history length >=4, ascending order, second read same length/order; event-ID and sequence cursors return exactly the suffix after the acknowledged event; invalid cursors return BAD_REQUEST, no history/SSE body, and recovery CURSOR_STALE for ~default with both omit flags true; submitted WORK_REQUEST has the submitted request ID, type FACTORY_REQUEST_BATCH, one work, name and both trace IDs trace-request; reconnect excludes the acknowledged event and has exact suffix/order/sequence with no duplicate. |
| events/response_events/stream_test.go:254,264,367,407 | Tight-retention history has >=2 retained response events and first sequence >1; stale cursor is 1 or 0 by first available sequence, gap metadata matches, and catch-up equals retained order; retained-then-live has >=2 retained frames, every live sequence > maxRetainedSequence, ascending/SSE-ID matching, and provider runner call count 2. |
| factory/definitions/defaults_test.go:429 | Global defaults, factory override, and single-discovered-provider cases each assert task:complete=1, task:failed=0, runner calls 1; provider commands are CODEX, CLAUDE, and discovered codex; models are operator-default-model, factory-authored-model, or the configured discovered selection. |
| factory/replay_contracts/*.go:37-59 | Canonical live/artifact histories are non-empty; artifact terminal sequence equals len(events)-1; IDs, sequence, types, public comparable payload JSON, and session scope ~default match; artifact contains RUN_RESPONSE and SESSION_COMPLETED; malformed WORK_REQUEST payload [] is rejected with zero provider calls and source artifact preserved. Composed record/replay asserts task:complete=1, public dispatch response, non-empty artifact, replay provider calls 0, unchanged artifact, and unchanged write-call count. Snapshot reader is non-nil, Items and Admissions are non-empty, task is complete, and repeated snapshot sizes are unchanged. |
| factory/visualization/runtime_metrics/replay_priced_cost_test.go:52 | Terminal category 1, failed 0; human/CLI/API report PRICED, USD, $21.25, priced amount 21.25, source BUILT_IN, total tokens 3,000,000; input/output/total tokens 1,000,000/2,000,000/3,000,000; one line/worker/provider/session row with CODEX/gpt-5-codex, worker session cost-replay-priced-worker-session; operator override is PRICED, $42.00, amount 42, source OPERATOR_SUPPLIED; provider/script calls 0. |
| factory/visualization/runtime_metrics/replay_unpriced_cost_test.go:46 | Terminal 1, failed 0; before configuration human/API/CLI is UNPRICED, unknown/no $0.00, Claude claude-sonnet-4-6, input/output/total 1200/300/1500; configured result is PRICED, $0.01, amount 0.0081, source OPERATOR_SUPPLIED; removal reverts to UNPRICED; API and CLI facts match and external calls remain 0. |
| factory/visualization/runtime_metrics/runtime_artifact_flow_test.go:58 | Exactly one active regular artifact with YYYY/MM/DD/<time>-runtime-metrics-*.log; protected and failed retention counts are positive, failed path is reported, expired artifact is removed, unrelated/outside contents are exact, symlink is preserved when supported; positive provider.input_tokens and provider.output_tokens records contain non-empty session/runtime/dispatch/worker-session/timestamp fields; named reservation paths are session.jsonl and collision session-2.jsonl, original content/mode preserved. |
| internal/support/work_location_test.go:127 | Public support-backed scenario asserts task:complete=1, task:init=0, and the listed work ID is at task:complete. |
| operator_settings/root_composition/operator_config_activation_test.go:80 | BuildProcess filesystem/temp-file effects are 0; after lifecycle ReadFile calls >0, runner calls 1, command CODEX, and args contain --model flag-override-model. The same file's public init path preserves provider claude, model operator-updated-model, and the exact success stdout sentence. |
| recordings/root_composition/*.go:45-98 | Event/artifact activation requires dispatch request/response events, default/global terminal category 1, global FactoryState RUNNING, durable session SUCCEEDED with non-empty ID, artifact list/detail identity, non-empty label/hash, CLI record path equality, malformed portable ErrInvalidPortableArtifact, MCP BAD_REQUEST, and MCP replay not-found recording.replay.not_found; record/replay/resume each preserve task:complete=1, dispatch response, session identity, and successor events. |
| replay_contracts/replay_factory_only_serialization_smoke_long_test.go:50,68 | Record/replay places are task:complete=1, task:init=0, task:failed=0; artifact contains factory payload and no effectiveConfig, __replayEffectiveConfig, or runtimeWorkerConfig; work-type states are init/processing/complete/failed, resource slot capacity 1, workers exec-worker and finish-worker are AGENT_WORKER with stop token COMPLETE, and executor/finisher workstation/resource shape is preserved. |
| replay_contracts/replay_record_end_to_end_long_test.go:263,367 | CLI record/replay stdout is empty where specified; dispatch request/response counts are >0; raw script secret is absent and env is redacted; replay places task:done=1, init/failed 0; drift warning is the exact documented sentence; generated request ID is non-empty with generated-request- prefix, generated works 2, relations 1, source prefix worker-output:, parent lineage request-replay-external-batch,work-external-fanout, relation generated-beta→generated-alpha requiring complete, and generated IDs alpha/beta; provider env receives raw secret before recording, artifact does not. |
| replay_contracts/replay_regression_harness_long_test.go:41,166 | Fixture has dispatch request/response counts >0; successful replay is task:complete=1 and init/processing/failed 0; divergence contains exact category dispatch_mismatch, mutated tick, and expected event ID. |
| replay_contracts/replay_runtime_config_smoke_long_test.go:45,63 | Record/replay places are task complete 1, init/processing/failed 0; dispatch request and response counts are exactly 2; provider calls exactly 2 and each prompt contains Do the work.; artifact omits legacy workstation_configs, API factory has exactly 2 workstations including step-one. |
| replay_contracts/replay_thin_event_dual_dispatch_smoke_test.go:111 | Model task complete 1, script-task done 1, both failure states 0; worker-a, worker-b, and script runner calls each 1; model/script request-response lifecycle, shared trace/request IDs, and omission of retired raw fields are preserved in live and artifact histories. |
| replay_contracts/replay_work_dispatch_contract_smoke_long_test.go:202 | Each canonical/legacy/recorded dispatch asserts command echo, exact request/work/type/trace/chaining IDs, previous trace list containing the same trace, exact work name/title/tags, resolved /tmp/<branch> work directory, BRANCH and TEAM environment values, task:done=1, init/failed 0, runner calls 1, and event/artifact dispatch/script correlation. |
| runtime_api/api_inference_events_test.go:51 | Dispatch→inference request→inference response→dispatch response ordering and shared dispatch ID; attempt 1; non-empty matching inference request ID/prompt; successful response text Step one done. COMPLETE; provider session session-inference-events; non-negative duration; raw context dispatch ID present, payload dispatch/transition IDs absent; live inference events recorded with same type. |
| runtime_api/api_unified_event_log_smoke_test.go:42 | Upsert request/trace IDs are exact fixture IDs; two completed works retain fixture trace and complete; canonical event subsequence is RUN_REQUEST, INITIAL_STRUCTURE_REQUEST, WORK_REQUEST, RELATIONSHIP_CHANGE_REQUEST, DISPATCH_REQUEST, INFERENCE_REQUEST, INFERENCE_RESPONSE, DISPATCH_RESPONSE, FACTORY_STATE_RESPONSE, RUN_RESPONSE; relationship is review depends on draft complete; four dispatches and four each inference request/response; Factory and RUN response state COMPLETED; only session lifecycle trailers follow RUN_RESPONSE; live/artifact type, tick, dispatch ID, and work IDs match. |
| work/recovery/manual_move_test.go:83 | Initial parent/child failed states; move responses HTTP 200; both completed Work count 2, each state complete, and both IDs are listed at task:complete. |
| work/relationships/parent_child_test.go:97,205 | Exact request/work IDs and names remain linked by PARENT_CHILD; child listing/show carries parent ID; event history has WORK_REQUEST and RELATIONSHIP_CHANGE_REQUEST with correct source/target/type; reconstructed history length/order facts match; child failure and parent projection both are failed with types story/story-set. |
| work/root_composition/recovery_recordings_visualization_activation_test.go:137 | Work list includes exact activation work name, work type task, state complete. |
| work/root_composition/routing_relationship_activation_test.go:226 | Parent-child public listing, event relation, and child dispatch preserve the exact parent/child IDs and relation type. |
| work/submission/batch_inputs_long_test.go:68,78 | PUT response request ID request-retired-unary-replay, one work ID work-retired-unary-replay; recorded source external-submit, work count 1, relations 0; replay emits the same request/work/type task. |
| work/submission/legacy_unary_test.go:68 | Direct POST trace is non-empty; idempotent PUT preserves the first trace ID; completed work IDs and canonical FACTORY_REQUEST_BATCH event/request/work/type facts remain exact. |
| workflow/config_driven_retry_loop_breaker_test.go:101,127 | Max retry refusal: task failed 1, init/complete 0, provider calls 1, one FAILED process response; success-before-limit: complete 1, init/failed 0. |
| workflow/review_retry_exhaustion_long_test.go:35,71,106 | Exhaustion: swe/reviewer calls 3 each, failed 1, init/in-review/complete 0, exact rejected outputs add unit tests, tests incomplete, coverage too low; success: swe/reviewer calls 2 each, complete 1, failed/init 0. |
| workstations/repeater/reject_accept*.go stable callers | Guarded breaker: task failed 1, init/complete 0, route executor-loop-breaker; reject/accept cases preserve exact executor/finisher call counts (1, 2, 3, or 4 as configured), completion/failure state maps, stop-word matrices, multi-output plan/task states, input trace lineage on plan/task, exact resolved Ralph working directory and PROJECT=ralph-loop-fixture, and no unexpected non-terminal Work. |
| workstations/watcher/files_test.go stable callers | Single/sequential/concurrent/default/execution-ID/combined cases assert processor calls and terminal counts 1, 3, 5, 1, 1, 2 as named, failed 0, listed Work counts matching seed counts, complete counts matching seed counts, init/processing 0, non-empty unique IDs, and TERMINAL state type. Mixed case asserts terminal 3, failed 2, listed 5, complete 3, failed 2, init/processing 0, and exact complete/failed state names. Parent-child fan-in asserts story complete 1, story failed 1, story-set failed 1, waiting/init/complete 0, runner calls 3, and recorded parent-failure ordering. |

## Before timing evidence — 51 exact invocations

For every row, the exact command was run as a separate PowerShell
Measure-Command invocation:

    go test -tags=functionallong -count=1 <package>

Wall values are the captured Measure-Command values rounded to milliseconds;
the package-reported value is the go test output. Rows marked FAIL retain the
actual exit code and diagnostic signature below.

| # | Exact command | Result / exit | Go package time | Wall |
| ---: | --- | --- | ---: | ---: |
| 1 | go test -tags=functionallong -count=1 ./tests/functional/internal/support | PASS / 0 | 5.706s | 18.120s |
| 2 | go test -tags=functionallong -count=1 ./tests/functional/internal/support | PASS / 0 | 6.308s | 8.165s |
| 3 | go test -tags=functionallong -count=1 ./tests/functional/internal/support | PASS / 0 | 7.752s | 9.918s |
| 4 | go test -tags=functionallong -count=1 ./tests/functional/events/factory_events | PASS / 0 | 5.500s | 7.944s |
| 5 | go test -tags=functionallong -count=1 ./tests/functional/events/factory_events | PASS / 0 | 5.214s | 7.319s |
| 6 | go test -tags=functionallong -count=1 ./tests/functional/events/factory_events | PASS / 0 | 4.957s | 6.924s |
| 7 | go test -tags=functionallong -count=1 ./tests/functional/events/response_events | PASS / 0 | 1.798s | 3.949s |
| 8 | go test -tags=functionallong -count=1 ./tests/functional/events/response_events | PASS / 0 | 2.536s | 4.805s |
| 9 | go test -tags=functionallong -count=1 ./tests/functional/events/response_events | PASS / 0 | 2.177s | 5.307s |
| 10 | go test -tags=functionallong -count=1 ./tests/functional/factory/definitions | PASS / 0 | 20.201s | 22.311s |
| 11 | go test -tags=functionallong -count=1 ./tests/functional/factory/definitions | PASS / 0 | 18.068s | 19.867s |
| 12 | go test -tags=functionallong -count=1 ./tests/functional/factory/definitions | PASS / 0 | 20.534s | 22.567s |
| 13 | go test -tags=functionallong -count=1 ./tests/functional/factory/replay_contracts | PASS / 0 | 10.861s | 12.827s |
| 14 | go test -tags=functionallong -count=1 ./tests/functional/factory/replay_contracts | PASS / 0 | 10.580s | 12.520s |
| 15 | go test -tags=functionallong -count=1 ./tests/functional/factory/replay_contracts | PASS / 0 | 11.480s | 13.276s |
| 16 | go test -tags=functionallong -count=1 ./tests/functional/factory/visualization/runtime_metrics | PASS / 0 | 10.239s | 12.289s |
| 17 | go test -tags=functionallong -count=1 ./tests/functional/factory/visualization/runtime_metrics | PASS / 0 | 11.255s | 13.124s |
| 18 | go test -tags=functionallong -count=1 ./tests/functional/factory/visualization/runtime_metrics | PASS / 0 | 12.249s | 14.339s |
| 19 | go test -tags=functionallong -count=1 ./tests/functional/operator_settings/root_composition | PASS / 0 | 1.848s | 4.495s |
| 20 | go test -tags=functionallong -count=1 ./tests/functional/operator_settings/root_composition | PASS / 0 | 1.707s | 3.894s |
| 21 | go test -tags=functionallong -count=1 ./tests/functional/operator_settings/root_composition | PASS / 0 | 1.462s | 3.573s |
| 22 | go test -tags=functionallong -count=1 ./tests/functional/recordings/root_composition | FAIL / 1 | 3.643s | 5.786s |
| 23 | go test -tags=functionallong -count=1 ./tests/functional/recordings/root_composition | FAIL / 1 | 3.744s | 5.789s |
| 24 | go test -tags=functionallong -count=1 ./tests/functional/recordings/root_composition | FAIL / 1 | 3.659s | 5.620s |
| 25 | go test -tags=functionallong -count=1 ./tests/functional/replay_contracts | FAIL / 1 | 54.018s | 56.566s |
| 26 | go test -tags=functionallong -count=1 ./tests/functional/replay_contracts | FAIL / 1 | 56.765s | 59.117s |
| 27 | go test -tags=functionallong -count=1 ./tests/functional/replay_contracts | FAIL / 1 | 55.852s | 57.861s |
| 28 | go test -tags=functionallong -count=1 ./tests/functional/runtime_api | FAIL / 1 | 42.721s | 45.229s |
| 29 | go test -tags=functionallong -count=1 ./tests/functional/runtime_api | FAIL / 1 | 41.369s | 43.402s |
| 30 | go test -tags=functionallong -count=1 ./tests/functional/runtime_api | FAIL / 1 | 40.570s | 42.540s |
| 31 | go test -tags=functionallong -count=1 ./tests/functional/work/recovery | PASS / 0 | 4.244s | 6.206s |
| 32 | go test -tags=functionallong -count=1 ./tests/functional/work/recovery | PASS / 0 | 4.313s | 6.142s |
| 33 | go test -tags=functionallong -count=1 ./tests/functional/work/recovery | PASS / 0 | 4.366s | 6.286s |
| 34 | go test -tags=functionallong -count=1 ./tests/functional/work/relationships | PASS / 0 | 3.071s | 5.401s |
| 35 | go test -tags=functionallong -count=1 ./tests/functional/work/relationships | PASS / 0 | 2.926s | 4.791s |
| 36 | go test -tags=functionallong -count=1 ./tests/functional/work/relationships | PASS / 0 | 2.853s | 4.700s |
| 37 | go test -tags=functionallong -count=1 ./tests/functional/work/root_composition | PASS / 0 | 2.207s | 4.154s |
| 38 | go test -tags=functionallong -count=1 ./tests/functional/work/root_composition | PASS / 0 | 2.129s | 3.925s |
| 39 | go test -tags=functionallong -count=1 ./tests/functional/work/root_composition | PASS / 0 | 2.151s | 4.016s |
| 40 | go test -tags=functionallong -count=1 ./tests/functional/work/submission | PASS / 0 | 20.681s | 22.720s |
| 41 | go test -tags=functionallong -count=1 ./tests/functional/work/submission | PASS / 0 | 21.284s | 23.326s |
| 42 | go test -tags=functionallong -count=1 ./tests/functional/work/submission | PASS / 0 | 20.736s | 22.790s |
| 43 | go test -tags=functionallong -count=1 ./tests/functional/workflow | PASS / 0 | 7.457s | 9.436s |
| 44 | go test -tags=functionallong -count=1 ./tests/functional/workflow | PASS / 0 | 7.577s | 9.545s |
| 45 | go test -tags=functionallong -count=1 ./tests/functional/workflow | PASS / 0 | 7.706s | 9.650s |
| 46 | go test -tags=functionallong -count=1 ./tests/functional/workstations/repeater | FAIL / 1 | 31.329s | 33.408s |
| 47 | go test -tags=functionallong -count=1 ./tests/functional/workstations/repeater | FAIL / 1 | 30.434s | 32.412s |
| 48 | go test -tags=functionallong -count=1 ./tests/functional/workstations/repeater | FAIL / 1 | 29.795s | 31.908s |
| 49 | go test -tags=functionallong -count=1 ./tests/functional/workstations/watcher | PASS / 0 | 12.784s | 14.877s |
| 50 | go test -tags=functionallong -count=1 ./tests/functional/workstations/watcher | PASS / 0 | 13.060s | 15.256s |
| 51 | go test -tags=functionallong -count=1 ./tests/functional/workstations/watcher | PASS / 0 | 13.320s | 15.391s |

### Package medians and unchanged failure signatures

| Package | Three wall values | Median wall | Outcome |
| --- | --- | ---: | --- |
| internal/support | 18.120s, 8.165s, 9.918s | 9.918s | all pass |
| events/factory_events | 7.944s, 7.319s, 6.924s | 7.319s | all pass |
| events/response_events | 3.949s, 4.805s, 5.307s | 4.805s | all pass |
| factory/definitions | 22.311s, 19.867s, 22.567s | 22.311s | all pass |
| factory/replay_contracts | 12.827s, 12.520s, 13.276s | 12.827s | all pass |
| factory/visualization/runtime_metrics | 12.289s, 13.124s, 14.339s | 13.124s | all pass |
| operator_settings/root_composition | 4.495s, 3.894s, 3.573s | 3.894s | all pass |
| recordings/root_composition | 5.786s, 5.789s, 5.620s | 5.786s | all three fail |
| replay_contracts | 56.566s, 59.117s, 57.861s | 57.861s | all three fail |
| runtime_api | 45.229s, 43.402s, 42.540s | 43.402s | all three fail |
| work/recovery | 6.206s, 6.142s, 6.286s | 6.206s | all pass |
| work/relationships | 5.401s, 4.791s, 4.700s | 4.791s | all pass |
| work/root_composition | 4.154s, 3.925s, 4.016s | 4.016s | all pass |
| work/submission | 22.720s, 23.326s, 22.790s | 22.790s | all pass |
| workflow | 9.436s, 9.545s, 9.650s | 9.545s | all pass |
| workstations/repeater | 33.408s, 32.412s, 31.908s | 32.412s | all three fail |
| workstations/watcher | 14.877s, 15.256s, 15.391s | 15.256s | all pass |

The 51 commands produced 39 passes and 12 failures. The four failing package
families are unchanged baseline diagnostics, not Story 001 code regressions:

- recordings/root_composition: all runs fail before the scenario because the shared global packaged-factory staging owner
  C:\\Users\\andre\\.you-agent-factory\\factories\\.you--full-flow.staging-owner
  reports indeterminate-contention with owner PID 6224 and unverified
  liveness. No shared staging target was removed or altered.
- replay_contracts: all runs retain packaged-install contention plus existing
  fixture/assertion failures, including process API starter/replay input
  failure, the factory-only worker-type mismatch, generated relation count
  mismatch, expected divergence mismatch, and a nil replay workstation map
  assertion.
- runtime_api: all runs retain existing logical-target failures for the
  workers and workstations fixture directories and dashboard world-view
  world-view-failed/permanent_bad_request diagnostics.
- workstations/repeater: all runs retain existing processor execution
  failures and the Windows separator assertion where the expected Ralph path
  uses \\ralph\\ralph-loop and the observed path uses \\ralph/ralph-loop.

The timings are directional characterization only. The host was not treated as
quiet or low-variance, and no absolute threshold was imposed. These runs prove
the exact pre-change denominator and runnable package set; they do not prove
the observer, migration, after timings, or clean checkout.

## Story 003 — final census and after timing evidence

### Review reconciliation

The review feedback exposed two distinct support boundaries that the original
literal family audit had conflated. First, excluded provider, Provider
Session, Worker, and ACP tests still compile against the old
`WaitForTerminalStatus` and stable-wrapper names. Those names remain only for
the unchanged excluded callers and delegate to event-driven support without a
stable window. Second, `WaitForSessionTerminalStatus` is a pre-existing
session-scoped status-projection contract used by live/shared-session tests;
those sessions can finish their current Work while remaining live and do not
publish the standalone process `RUN_RESPONSE` boundary. It therefore retains
its status semantics through bounded adaptive retries, with no fixed poll
interval or stable window. It is outside the direct `WaitForTerminalStatus`
family audited here. No excluded file is changed.

The central process helper now establishes the terminal observer while the
root-built API listener is held before invocation, waits for the canonical
post-cursor `RUN_RESPONSE` before taking terminal snapshots or running capture
callbacks, and uses an explicit Work admission count for continuous
watcher/repeater callers. A delayed-admission regression proves that a
terminal projection of the currently listed Work cannot end the wait before a
known later admission.

The observer's synchronous SSE open uses a bounded response-header policy,
cancellation, response-body close, and idle-connection cleanup. A
delayed-header test proves that an accepted connection cannot strand the test
before `Wait` is reachable. The generic status helper now uses bounded adaptive
retries for readiness/stop predicates and for the separate live/shared-session
status contract; the owned standalone terminal completion path does not call
it.

The production-only deadcode checker cannot see callers in external functional
test packages. The observer and Work-observation support entrypoints are
therefore deliberately function-valued test-harness seams, with the raw Work
stream/projection read kept local to that seam so unrelated legacy support
functions do not become falsely live. This is an owned design exception; the
shared deadcode baseline is unchanged.

### Final family census

The final census was rerun after the review fixes with:

    rg -n 'WaitForTerminalStatus|terminalObservationStableWindow|RunFactoryToCompletionWithEdgesAndWorkStable|RunFactoryToCompletionWithEdgesAndObservationsStable' tests/functional

The owned result is zero direct `WaitForTerminalStatus` callers, zero owned
`terminalObservationStableWindow` mode references, zero owned stable-only
completion wrappers, and zero owned stable-wrapper callers. The separate
session-scoped status helper remains an explicit non-family compatibility
boundary for live/shared-session callers and has no fixed interval or stable
window. The final result retains these explicit compatibility handoffs:

| Current hit | Owner/disposition |
| --- | --- |
| `tests/functional/providers/cli_timeout_cleanup_smoke_test.go:72,123` | Providers live lane; excluded and unchanged |
| `tests/functional/providers/mock_workers_end_to_end_smoke_test.go:87` | Providers live lane; excluded and unchanged |
| `tests/functional/providers/mock_workers_script_test.go:109` | Providers live lane; excluded and unchanged |
| `tests/functional/providers/packaged_script_runtime_test.go:39,77` | Providers live lane; excluded and unchanged |
| `tests/functional/providers/runtime_logging_smoke_test.go:318` | Providers live lane; excluded and unchanged |
| `tests/functional/provider_sessions/association/response_exec_metadata_test.go:206,262` | Provider Sessions live lane; excluded and unchanged |
| `tests/functional/workers/script/execution_long_test.go:64` | Workers live lane; excluded and unchanged |
| `tests/functional/providers/acp/packaged_conformance_test.go:83` | ACP live lane's before-close wrapper; excluded and unchanged |
| `tests/functional/internal/support/http_observation.go` | `WaitForTerminalStatus` is the compatibility symbol required to compile excluded direct callers; it delegates to event-driven session observation and implements no stable window. `WaitForSessionTerminalStatus` separately preserves the live/shared-session status projection contract with bounded adaptive retries. |
| `tests/functional/internal/support/process_factory.go:151,155` | Compatibility wrapper required by the excluded ACP caller; it waits on canonical event observation and implements no stable window |

The two unrelated `stableWindow` constants remain at
`tests/functional/workstations/watcher/files_test.go:676` and
`tests/functional/factory/current/helpers_long_test.go:248`; they are watcher
debounce/readiness behavior outside this terminal-observation family. No file
under the excluded provider, provider-session, worker, or ACP directories
changed. The literal symbol-absence criterion is satisfied for the audited
owned family; the retained names above are documented delegated boundaries,
not an unrecorded PASS claim for the excluded lanes. The live/shared-session
status contract is intentionally not presented as a post-cursor
`RUN_RESPONSE` journey: its existing callers need to observe a live host's
status projection.
The four audit-time JavaScript locations remain absent as established by the
Story 001 history reconciliation.

### After timing evidence — 51 exact invocations

Each row below is three independent invocations of:

    go test -tags=functionallong -count=1 <package>

The tuple format is `outcome/exit, package time, Measure-Command wall time`;
wall times are rounded to milliseconds. The median is the median wall time.

| # | Package | Run 1 | Run 2 | Run 3 | Median wall |
| ---: | --- | --- | --- | --- | ---: |
| 1 | `internal/support` | PASS/0, 8.348s, 10.967s | PASS/0, 7.925s, 10.319s | PASS/0, 8.060s, 10.336s | 10.336s |
| 2 | `events/factory_events` | PASS/0, 5.238s, 7.270s | PASS/0, 6.078s, 8.027s | PASS/0, 4.569s, 6.452s | 7.270s |
| 3 | `events/response_events` | PASS/0, 3.099s, 5.184s | PASS/0, 5.558s, 8.403s | PASS/0, 3.844s, 9.373s | 8.403s |
| 4 | `factory/definitions` | PASS/0, 84.677s, 87.209s | PASS/0, 23.367s, 25.547s | PASS/0, 22.556s, 24.624s | 25.547s |
| 5 | `factory/replay_contracts` | PASS/0, 10.363s, 12.563s | PASS/0, 10.029s, 12.131s | PASS/0, 9.933s, 11.838s | 12.131s |
| 6 | `factory/visualization/runtime_metrics` | PASS/0, 11.413s, 13.511s | PASS/0, 10.511s, 12.368s | PASS/0, 10.520s, 12.411s | 12.411s |
| 7 | `operator_settings/root_composition` | PASS/0, 1.124s, 2.892s | PASS/0, 1.265s, 3.022s | PASS/0, 1.352s, 3.209s | 3.022s |
| 8 | `recordings/root_composition` | FAIL/1, 3.024s, 5.192s | FAIL/1, 2.959s, 4.779s | FAIL/1, 3.027s, 5.080s | 5.080s |
| 9 | `replay_contracts` | FAIL/1, 50.643s, 52.589s | FAIL/1, 50.535s, 52.531s | FAIL/1, 54.842s, 56.770s | 52.589s |
| 10 | `runtime_api` | FAIL/1, 44.933s, 47.247s | FAIL/1, 43.303s, 45.502s | FAIL/1, 40.844s, 42.952s | 45.502s |
| 11 | `work/recovery` | PASS/0, 2.881s, 4.826s | PASS/0, 3.037s, 5.242s | PASS/0, 3.624s, 5.935s | 5.242s |
| 12 | `work/relationships` | PASS/0, 2.930s, 5.019s | PASS/0, 3.008s, 5.060s | PASS/0, 3.211s, 5.443s | 5.060s |
| 13 | `work/root_composition` | PASS/0, 1.978s, 4.309s | PASS/0, 2.300s, 4.790s | PASS/0, 2.551s, 4.642s | 4.642s |
| 14 | `work/submission` | PASS/0, 18.897s, 20.900s | PASS/0, 19.778s, 22.938s | PASS/0, 27.441s, 29.396s | 22.938s |
| 15 | `workflow` | PASS/0, 6.055s, 8.212s | PASS/0, 6.634s, 8.872s | PASS/0, 5.801s, 7.898s | 8.212s |
| 16 | `workstations/repeater` | FAIL/1, 23.690s, 25.704s | FAIL/1, 19.577s, 21.353s | FAIL/1, 18.646s, 20.301s | 21.353s |
| 17 | `workstations/watcher` | PASS/0, 9.359s, 11.179s | PASS/0, 8.459s, 10.205s | PASS/0, 8.128s, 9.900s | 10.205s |

The after sweep is 39 PASS and 12 FAIL, with the same four failing package
families and baseline signatures recorded above: global `@you/full-flow`
staging-owner contention in recordings, packaged/fixture and assertion
failures in replay, logical-target/fixture failures in runtime API, and
processor/path failures in repeater. The implementation did not remove or
alter shared global state. Compared with the before medians, 10 of 17 package
medians improved and the sum of package medians decreased from 276.263s to
259.943s (5.9% directionally lower); repeater decreased from 32.412s to
21.353s and watcher from 15.256s to 10.205s. This is the bounded optimization
pass for this lane: fixed status polling and stable-window padding were removed
from owned completion paths, with no unrelated optimization or threshold
claim made on this saturated Windows host.

### Story 003 evidence conclusion

The final census, exact assertion inventory above, 51 after invocations, and
`go vet ./...` (exit 0, no findings) prove the owned migration and preserve
the existing passing behavior. The 12 failures are unchanged diagnostic
baseline signatures, not implementation regressions. Clean-checkout loopback,
final delivery, and CI remain Story 004/review gates.

## Story 001 evidence conclusion

The ledger proves the story's four acceptance criteria:

1. The rebased head, exact family definitions/mode uses, every external hit,
   support internal hit, build tag, owner, and excluded handoff are listed once.
2. The four vanished JavaScript sites are reconciled against current content and
   history with zero current hits and no invented migration.
3. Every lane-owned completion caller is mapped to its exact post-wait asserted
   fields/values and invariants for comparison.
4. All 51 exact -tags=functionallong -count=1 invocations have raw outcome,
   package time, rounded wall time, and package medians; unchanged environmental
   failures are retained as diagnostic evidence.

Remaining gates are deliberately open: OBS-SPINE, OBS-UNIT, MIG-PKG,
PERF-001, VET-001, and VAL-001.
