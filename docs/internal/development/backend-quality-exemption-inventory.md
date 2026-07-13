# Backend quality exemption and coverage inventory

> Snapshot revision: `dcda359c40703b01c3d1e0b32bbd03a547f9313f`  
> Scan time: `2026-07-13T07:06:21Z` (UTC)  
> Scope: source and documentation inventory only; no checker policy, directive, threshold, baseline, or product behavior is changed.

## Snapshot method

This inventory recognizes exactly the spellings implemented by the two checkers:
`backendsizecheck:ignore-file`, `backendsizecheck:ignore-function`,
`pkgmaintcheck:ignore-file-lines`, `pkgmaintcheck:ignore-function-lines`, and
`pkgmaintcheck:ignore-cyclomatic-complexity`.

The focused source scan was run from the repository root at the revision above:

```sh
roots=(cmd internal pkg tests)
if test -d vendor; then roots+=(vendor); fi
rg -n --glob '*.go' '// (backendsizecheck:ignore-(file|function)|pkgmaintcheck:ignore-(file-lines|function-lines|cyclomatic-complexity))' "${roots[@]}"
```

The command intentionally includes checker tests and generated/fixture paths, plus `vendor`
when that root exists, so matches can be classified instead of silently omitted. A match is active handwritten
debt only when it is outside `cmd/backendsizecheck`, `cmd/pkgmaintcheck`, generated output,
`testdata`, and `vendor`. Targets below are the file for file directives and the next Go
function declaration for function-scoped directives, matching Go doc-comment attachment.

Snapshot reconciliation:

- Focused matches: **185**.
- Active handwritten occurrences: **177** across **93** files and **37** package directories.
- Checker-owned fixture strings: **8**.
- Generated, vendored, or `testdata` directive matches: **0**.
- Active rule totals: `27` backendsize file, `5` backendsize function, `26` maintainability file-lines, `5` maintainability function-lines, and `114` cyclomatic-complexity.

## Classification and removal evidence

Every active row names its owner as the maintainers of the package directory in its section
and uses exactly one reason class. Removal is permitted only when that class's objective
evidence is present:

- **F — production file responsibility:** the maintained file owns too many co-located responsibilities. Remove the directive only after a bounded responsibility split leaves each owned file within both checker limits, package behavior tests remain green, and both checkers pass.
- **R — production function responsibility:** one production function keeps a branchy or long atomic seam together. Remove the directive only after behavior-preserving extraction makes the target pass its applicable line/complexity checks, focused package behavior tests remain green, and both checkers pass.
- **T — test scenario or fixture:** a contract scenario, assertion helper, or consolidated test file exceeds a limit to keep observable evidence together. Remove the directive only after a fixture/scenario split preserves equivalent observable assertions, the focused test package passes, and both checkers pass.
- **P — ported integration seam:** MIT-ported Cursor storage decoding or probing retains upstream-shaped branching. Remove the directive only after the compatibility/integration seam is retired or a bounded decoder/prober responsibility is extracted with equivalent fixture-backed behavior, followed by package tests and both checkers passing.

The reason class is not an instruction to refactor immediately; it is the auditable gate for a
later removal. Line numbers are revision-stamped source locations, not stable identifiers.

## Package reconciliation summary

The coverage baseline is parsed like `gocoveragecheck`: trim surrounding whitespace, ignore
blank and `#` comment lines, and deduplicate the remaining full import paths. It contains
**71** packages. Of **37** directive-owning package directories,
**18** are in both systems and **19** are directive only; the remaining
**53** baseline packages are coverage-baseline only.

| Owning package | Active directives | Files | Quality-system status |
| --- | ---: | ---: | --- |
| `pkg/api` | 7 | 3 | directive + coverage baseline |
| `pkg/api/providersessioncursor` | 1 | 1 | directive only |
| `pkg/api/servertests` | 1 | 1 | directive only |
| `pkg/api/workstationprojection` | 3 | 1 | directive only |
| `pkg/apisurface/factorysession` | 8 | 6 | directive only |
| `pkg/cli` | 2 | 1 | directive only |
| `pkg/cli/cliinputs` | 5 | 3 | directive only |
| `pkg/cli/config` | 2 | 1 | directive + coverage baseline |
| `pkg/cli/init` | 2 | 1 | directive only |
| `pkg/cli/mcp` | 3 | 3 | directive + coverage baseline |
| `pkg/cli/run` | 2 | 1 | directive + coverage baseline |
| `pkg/cli/submit` | 1 | 1 | directive only |
| `pkg/cli/work` | 1 | 1 | directive only |
| `pkg/config` | 8 | 4 | directive + coverage baseline |
| `pkg/config/openapitests` | 1 | 1 | directive only |
| `pkg/factory/events` | 3 | 3 | directive + coverage baseline |
| `pkg/factory/ingest` | 1 | 1 | directive + coverage baseline |
| `pkg/factory/projections` | 4 | 2 | directive + coverage baseline |
| `pkg/factory/projections/projectiontests` | 6 | 4 | directive only |
| `pkg/factory/requests` | 2 | 2 | directive + coverage baseline |
| `pkg/factory/runtime` | 6 | 3 | directive only |
| `pkg/factory/subsystems` | 1 | 1 | directive + coverage baseline |
| `pkg/factory/subsystems/dispatchertests` | 1 | 1 | directive only |
| `pkg/factory/validation` | 2 | 1 | directive + coverage baseline |
| `pkg/factorysessionexecution` | 26 | 8 | directive only |
| `pkg/factorysessionexecution/fixtures` | 5 | 3 | directive only |
| `pkg/interfaces` | 3 | 2 | directive + coverage baseline |
| `pkg/internal/cursorstorage` | 10 | 5 | directive + coverage baseline |
| `pkg/mcp/factorysession` | 9 | 4 | directive + coverage baseline |
| `pkg/replay` | 2 | 2 | directive + coverage baseline |
| `pkg/replay/configtests` | 3 | 3 | directive only |
| `pkg/runtimehost` | 5 | 3 | directive + coverage baseline |
| `pkg/service` | 29 | 9 | directive + coverage baseline |
| `pkg/service/runtimelogtests` | 2 | 1 | directive only |
| `pkg/workers/executor` | 7 | 3 | directive + coverage baseline |
| `pkg/workers/provider` | 2 | 2 | directive only |
| `tests/functional/runtime_api/factory_transformation` | 1 | 1 | directive only |

### Coverage-baseline-only packages

These entries are retained verbatim after whitespace normalization; they have no active
directive occurrence in the focused scan:

- `pkg/api/apitypes`
- `pkg/apisurface`
- `pkg/apisurface/optional`
- `pkg/cli/clidiag`
- `pkg/cli/dashboardrender`
- `pkg/cli/default`
- `pkg/cli/models`
- `pkg/cli/session`
- `pkg/cli/sessionexecution`
- `pkg/config/factoryrun`
- `pkg/config/inboxgitkeep`
- `pkg/config/load`
- `pkg/config/mockworkers`
- `pkg/config/operatordefaultsruntime`
- `pkg/config/retiredboundary`
- `pkg/factory`
- `pkg/factory/runtime/buffers`
- `pkg/factory/service`
- `pkg/factory/state`
- `pkg/factory/throttle`
- `pkg/factory/token_transformer`
- `pkg/factorysessionexecution/runtimepersist`
- `pkg/factorysessions`
- `pkg/factorysessions/controlplane`
- `pkg/factorysessions/dataplane`
- `pkg/factorysessions/service`
- `pkg/factorysessions/stream`
- `pkg/hostedworkers`
- `pkg/hostedworkers/linear`
- `pkg/invocations`
- `pkg/localmodels`
- `pkg/localmodels/assets`
- `pkg/logging`
- `pkg/mcp/server`
- `pkg/modelhost`
- `pkg/orchestrators/javascript/policy`
- `pkg/orchestrators/javascript/preview`
- `pkg/orchestrators/javascript/result`
- `pkg/orchestrators/javascript/runtime`
- `pkg/orchestrators/javascript/source`
- `pkg/orchestrators/javascript/store`
- `pkg/orchestrators/javascript/validation`
- `pkg/packagedfactories/goal`
- `pkg/packagedfactories/tts`
- `pkg/service/factorysave`
- `pkg/service/runtimebuild`
- `pkg/timework`
- `pkg/workcontent`
- `pkg/workcontent/materialize`
- `pkg/workers`
- `pkg/workers/mockworker`
- `pkg/workers/process`
- `pkg/workers/prompting`

## Active handwritten directive detail

Each section's named owner is that package directory's maintainers. `Evidence` refers to
the objective removal condition defined for the row's single reason class above.

### `pkg/api`

Owner: `pkg/api` package maintainers. Status: **directive + coverage baseline**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/api/server_factory_sessions_test.go:1` | `pkgmaintcheck:ignore-file-lines` | `pkg/api/server_factory_sessions_test.go` | T | T gate |
| `pkg/api/server_factory_sessions_test.go:2` | `backendsizecheck:ignore-file` | `pkg/api/server_factory_sessions_test.go` | T | T gate |
| `pkg/api/server_submit_work_test.go:125` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestSubmitWork_AcceptsCanonicalContent` | T | T gate |
| `pkg/api/server_work_query_test.go:1` | `backendsizecheck:ignore-file` | `pkg/api/server_work_query_test.go` | T | T gate |
| `pkg/api/server_work_query_test.go:2` | `pkgmaintcheck:ignore-file-lines` | `pkg/api/server_work_query_test.go` | T | T gate |
| `pkg/api/server_work_query_test.go:722` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestListWork_ReturnsRuntimeRelationsWithSourceToTargetDirection` | T | T gate |
| `pkg/api/server_work_query_test.go:1054` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestUpsertWorkRequest_MapsWorkTypeNameAndRelationsToRuntime` | T | T gate |

### `pkg/api/providersessioncursor`

Owner: `pkg/api/providersessioncursor` package maintainers. Status: **directive only**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/api/providersessioncursor/detail_test.go:15` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestLoadDetails_ReadsReadableSessionFromConfiguredRoot` | T | T gate |

### `pkg/api/servertests`

Owner: `pkg/api/servertests` package maintainers. Status: **directive only**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/api/servertests/server_durable_session_interrupt_dispatch_test.go:262` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestInterruptFactorySessionDispatch_LateResultAfterInterruptSuppressedFromNormalRouting` | T | T gate |

### `pkg/api/workstationprojection`

Owner: `pkg/api/workstationprojection` package maintainers. Status: **directive only**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/api/workstationprojection/projection_test.go:128` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestBuildFactoryWorldWorkstationRequestProjectionSlice_PreservesPendingDispatchWithoutInferenceFallback` | T | T gate |
| `pkg/api/workstationprojection/projection_test.go:676` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `assertCompletedProjectionRequest` | T | T gate |
| `pkg/api/workstationprojection/projection_test.go:848` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `assertCompletedScriptProjection` | T | T gate |

### `pkg/apisurface/factorysession`

Owner: `pkg/apisurface/factorysession` package maintainers. Status: **directive only**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/apisurface/factorysession/factory_session_execution_test.go:224` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestSyncStartResponseToAPI_MapsTerminalAndTimeoutFixtures` | T | T gate |
| `pkg/apisurface/factorysession/factory_session_fake_consumer_test.go:13` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestFakeServiceConsumer_ProjectsFixtureThroughApisurfaceMappers` | T | T gate |
| `pkg/apisurface/factorysession/factory_session_lifecycle.go:344` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `lifecycleTimestampsToAPI` | R | R gate |
| `pkg/apisurface/factorysession/factory_session_mapper.go:60` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `SessionReadResultFromAPI` | R | R gate |
| `pkg/apisurface/factorysession/factory_session_mapper.go:226` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `DispatchDetailFromAPI` | R | R gate |
| `pkg/apisurface/factorysession/factory_session_mapper.go:407` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `DurableSessionListSummaryFromAPI` | R | R gate |
| `pkg/apisurface/factorysession/factory_session_mapper_test.go:684` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `assertListSummaryFieldsPreserved` | T | T gate |
| `pkg/apisurface/factorysession/factory_session_projection_test.go:350` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestProjectionResponses_TrimAndOmitOptionalFields` | T | T gate |

### `pkg/cli`

Owner: `pkg/cli` package maintainers. Status: **directive only**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/cli/root_run_test.go:1` | `backendsizecheck:ignore-file` | `pkg/cli/root_run_test.go` | T | T gate |
| `pkg/cli/root_run_test.go:2` | `pkgmaintcheck:ignore-file-lines` | `pkg/cli/root_run_test.go` | T | T gate |

### `pkg/cli/cliinputs`

Owner: `pkg/cli/cliinputs` package maintainers. Status: **directive only**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/cli/cliinputs/parser_parity_test.go:25` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestProductionParserParity_RepresentativeCommands` | T | T gate |
| `pkg/cli/cliinputs/parser_parity_test.go:128` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `productionParserParityRunFlagCases` | T | T gate |
| `pkg/cli/cliinputs/parser_parity_test.go:207` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `productionParserParitySubmitCases` | T | T gate |
| `pkg/cli/cliinputs/synthetic_args_relationships_test.go:143` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `assertSyntheticArgumentRecord` | T | T gate |
| `pkg/cli/cliinputs/synthetic_flags_test.go:437` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `assertSyntheticFlagRecord` | T | T gate |

### `pkg/cli/config`

Owner: `pkg/cli/config` package maintainers. Status: **directive + coverage baseline**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/cli/config/config_test.go:228` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `assertDeterministicExpandedRuntimeConfig` | T | T gate |
| `pkg/cli/config/config_test.go:868` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `assertExistingSplitDefinitionsPreserved` | T | T gate |

### `pkg/cli/init`

Owner: `pkg/cli/init` package maintainers. Status: **directive only**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/cli/init/init_test.go:401` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `assertRalphRuntimeConfig` | T | T gate |
| `pkg/cli/init/init_test.go:732` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `assertInitScaffoldFilesCanonical` | T | T gate |

### `pkg/cli/mcp`

Owner: `pkg/cli/mcp` package maintainers. Status: **directive + coverage baseline**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/cli/mcp/serve_runtime_resume_smoke_test.go:24` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestRunServe_RuntimeResumeSmoke_InterruptedSessionResumesThroughMCPControl` | T | T gate |
| `pkg/cli/mcp/serve_runtime_smoke_test.go:26` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestRunServe_RuntimeSmoke_DiscoveryAsyncPollAndResult` | T | T gate |
| `pkg/cli/mcp/serve_smoke_test.go:29` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestRunServe_InstallSmoke_DiscoveryValidateAsyncPoll` | T | T gate |

### `pkg/cli/run`

Owner: `pkg/cli/run` package maintainers. Status: **directive + coverage baseline**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/cli/run/run_invocation_test.go:1` | `backendsizecheck:ignore-file` | `pkg/cli/run/run_invocation_test.go` | T | T gate |
| `pkg/cli/run/run_invocation_test.go:2` | `pkgmaintcheck:ignore-file-lines` | `pkg/cli/run/run_invocation_test.go` | T | T gate |

### `pkg/cli/submit`

Owner: `pkg/cli/submit` package maintainers. Status: **directive only**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/cli/submit/submit_test.go:203` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestSubmit_JSONPayloadPostsWorkTypeName` | T | T gate |

### `pkg/cli/work`

Owner: `pkg/cli/work` package maintainers. Status: **directive only**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/cli/work/list_test.go:675` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestList_JSONOutputPreservesGeneratedResponseShape` | T | T gate |

### `pkg/config`

Owner: `pkg/config` package maintainers. Status: **directive + coverage baseline**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/config/config_mapper.go:1` | `backendsizecheck:ignore-file` | `pkg/config/config_mapper.go` | F | F gate |
| `pkg/config/config_mapper.go:2` | `pkgmaintcheck:ignore-file-lines` | `pkg/config/config_mapper.go` | F | F gate |
| `pkg/config/factory_config_mapping.go:1` | `backendsizecheck:ignore-file` | `pkg/config/factory_config_mapping.go` | F | F gate |
| `pkg/config/factory_config_mapping.go:2` | `pkgmaintcheck:ignore-file-lines` | `pkg/config/factory_config_mapping.go` | F | F gate |
| `pkg/config/factory_config_mapping_internal.go:1` | `backendsizecheck:ignore-file` | `pkg/config/factory_config_mapping_internal.go` | F | F gate |
| `pkg/config/factory_config_mapping_internal.go:2` | `pkgmaintcheck:ignore-file-lines` | `pkg/config/factory_config_mapping_internal.go` | F | F gate |
| `pkg/config/layout.go:1` | `backendsizecheck:ignore-file` | `pkg/config/layout.go` | F | F gate |
| `pkg/config/layout.go:2` | `pkgmaintcheck:ignore-file-lines` | `pkg/config/layout.go` | F | F gate |

### `pkg/config/openapitests`

Owner: `pkg/config/openapitests` package maintainers. Status: **directive only**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/config/openapitests/openapi_factory_test.go:96` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestFactoryConfigFromOpenAPIJSON_MapsPortableLayoutContract` | T | T gate |

### `pkg/factory/events`

Owner: `pkg/factory/events` package maintainers. Status: **directive + coverage baseline**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/factory/events/event_history.go:593` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `factoryEventPayload` | R | R gate |
| `pkg/factory/events/event_history_dispatch_lifecycle.go:96` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `RecordDispatchQueued` | R | R gate |
| `pkg/factory/events/event_history_lineage_test.go:348` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `assertEventHistoryResponseLineage` | T | T gate |

### `pkg/factory/ingest`

Owner: `pkg/factory/ingest` package maintainers. Status: **directive + coverage baseline**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/factory/ingest/filewatcher_test.go:396` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestFileWatcher_JSONFactoryRequestBatchAcceptsParentChildByWorkName` | T | T gate |

### `pkg/factory/projections`

Owner: `pkg/factory/projections` package maintainers. Status: **directive + coverage baseline**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/factory/projections/world_state.go:1` | `backendsizecheck:ignore-file` | `pkg/factory/projections/world_state.go` | F | F gate |
| `pkg/factory/projections/world_state.go:2` | `pkgmaintcheck:ignore-file-lines` | `pkg/factory/projections/world_state.go` | F | F gate |
| `pkg/factory/projections/world_state.go:96` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `apply` | R | R gate |
| `pkg/factory/projections/world_state_dispatch.go:759` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `mergeJavaScriptDispatchState` | R | R gate |

### `pkg/factory/projections/projectiontests`

Owner: `pkg/factory/projections/projectiontests` package maintainers. Status: **directive only**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/factory/projections/projectiontests/dispatch_lifecycle_event_replay_test.go:278` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `assertJavaScriptDispatchLifecycleReplay` | T | T gate |
| `pkg/factory/projections/projectiontests/simple_dashboard_projection_test.go:12` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestBuildSimpleDashboardProjection_RecordsWorkMoveHistory` | T | T gate |
| `pkg/factory/projections/projectiontests/simple_dashboard_projection_test.go:251` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `assertTerminalProjection` | T | T gate |
| `pkg/factory/projections/projectiontests/world_state_support_test.go:610` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `assignGeneratedProjectionPayload` | T | T gate |
| `pkg/factory/projections/projectiontests/world_state_support_test.go:686` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `assignGeneratedProjectionSessionLifecyclePayload` | T | T gate |
| `pkg/factory/projections/projectiontests/world_state_test.go:313` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestReconstructFactoryWorldState_PreservesSafeInferenceAttemptDiagnostics` | T | T gate |

### `pkg/factory/requests`

Owner: `pkg/factory/requests` package maintainers. Status: **directive + coverage baseline**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/factory/requests/work_request_submit_test.go:32` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestWorkRequestFromSubmitRequests_PreservesCanonicalBatchContract` | T | T gate |
| `pkg/factory/requests/work_request_test.go:64` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestNormalizeWorkRequest_IndependentWorkItemsShareRequestAndTrace` | T | T gate |

### `pkg/factory/runtime`

Owner: `pkg/factory/runtime` package maintainers. Status: **directive only**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/factory/runtime/factory_event_history_test.go:499` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `assertOrderedEventPayloads` | T | T gate |
| `pkg/factory/runtime/factory_event_history_test.go:619` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `assertBatchRequestReplayEvents` | T | T gate |
| `pkg/factory/runtime/factory_event_history_test.go:702` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `assertGeneratedBatchEvents` | T | T gate |
| `pkg/factory/runtime/factory_modes_test.go:21` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestFactoryEventHistory_SubscribeReplaysHistoryThenStreamsLiveEvents` | T | T gate |
| `pkg/factory/runtime/factory_modes_test.go:475` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestGetEngineStateSnapshot_AggregatesRuntimeLifecycleUptimeAndTopology` | T | T gate |
| `pkg/factory/runtime/factory_runtime_test_helpers_test.go:421` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `assertSafeBoundaryRequestView` | T | T gate |

### `pkg/factory/subsystems`

Owner: `pkg/factory/subsystems` package maintainers. Status: **directive + coverage baseline**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/factory/subsystems/history_transitioner_pipeline_test.go:429` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestHistoryTransitionerPipeline_CodexWindowsExitCode4294967295RequeuesAndPreservesRetryableProviderMetadata` | T | T gate |

### `pkg/factory/subsystems/dispatchertests`

Owner: `pkg/factory/subsystems/dispatchertests` package maintainers. Status: **directive only**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/factory/subsystems/dispatchertests/dispatcher_test.go:644` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `assertSingleTransitionDispatchResult` | T | T gate |

### `pkg/factory/validation`

Owner: `pkg/factory/validation` package maintainers. Status: **directive + coverage baseline**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/factory/validation/validation_test.go:1` | `backendsizecheck:ignore-file` | `pkg/factory/validation/validation_test.go` | T | T gate |
| `pkg/factory/validation/validation_test.go:2` | `pkgmaintcheck:ignore-file-lines` | `pkg/factory/validation/validation_test.go` | T | T gate |

### `pkg/factorysessionexecution`

Owner: `pkg/factorysessionexecution` package maintainers. Status: **directive only**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/factorysessionexecution/control.go:411` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `applyRuntimeExtendedLifecycleControl` | R | R gate |
| `pkg/factorysessionexecution/control.go:412` | `backendsizecheck:ignore-function` | `applyRuntimeExtendedLifecycleControl` | R | R gate |
| `pkg/factorysessionexecution/control.go:413` | `pkgmaintcheck:ignore-function-lines` | `applyRuntimeExtendedLifecycleControl` | R | R gate |
| `pkg/factorysessionexecution/control.go:519` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `applyRuntimeAcceptedLifecycleControl` | R | R gate |
| `pkg/factorysessionexecution/fake_fixture.go:93` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `parseFakeScenarioFromFixture` | R | R gate |
| `pkg/factorysessionexecution/fake_fixture.go:222` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `sessionReadFromFixtureMap` | R | R gate |
| `pkg/factorysessionexecution/fake_service.go:449` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `applyLifecycleControl` | R | R gate |
| `pkg/factorysessionexecution/fake_service.go:548` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `mutateSessionForControl` | R | R gate |
| `pkg/factorysessionexecution/fake_service_runtime_test.go:1` | `backendsizecheck:ignore-file` | `pkg/factorysessionexecution/fake_service_runtime_test.go` | T | T gate |
| `pkg/factorysessionexecution/fake_service_runtime_test.go:2` | `pkgmaintcheck:ignore-file-lines` | `pkg/factorysessionexecution/fake_service_runtime_test.go` | T | T gate |
| `pkg/factorysessionexecution/fake_service_runtime_test.go:234` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestFakeService_InternalLifecycleHelpers` | T | T gate |
| `pkg/factorysessionexecution/fake_service_runtime_test.go:2274` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestJavaScriptRuntimeService_LateChildResultAfterInterrupt_SuppressesNormalRouting` | T | T gate |
| `pkg/factorysessionexecution/fake_service_runtime_test.go:2368` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestFakeService_InterruptAcceptedBeforeCompletion_ObservableDispatchAndEventOutcomes` | T | T gate |
| `pkg/factorysessionexecution/fake_service_runtime_test.go:2668` | `pkgmaintcheck:ignore-function-lines` | `TestJavaScriptRuntimeService_PausePersistsStablePartialTerminalReadState` | T | T gate |
| `pkg/factorysessionexecution/fake_service_runtime_test.go:2669` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestJavaScriptRuntimeService_PausePersistsStablePartialTerminalReadState` | T | T gate |
| `pkg/factorysessionexecution/fake_service_test.go:1` | `backendsizecheck:ignore-file` | `pkg/factorysessionexecution/fake_service_test.go` | T | T gate |
| `pkg/factorysessionexecution/fake_service_test.go:2` | `pkgmaintcheck:ignore-file-lines` | `pkg/factorysessionexecution/fake_service_test.go` | T | T gate |
| `pkg/factorysessionexecution/fake_service_test.go:1007` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestProjectRuntimeExecutionRecords_FailedLiveChild_ProjectsFailureDetail` | T | T gate |
| `pkg/factorysessionexecution/fake_service_test.go:1832` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestFakeService_InterruptDispatch_RecordsDispatchInterruptedEvent` | T | T gate |
| `pkg/factorysessionexecution/fake_service_test.go:2195` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestFakeService_PauseResumeAppendsLifecycleControlEventsWithoutNoOpMutation` | T | T gate |
| `pkg/factorysessionexecution/lifecycle.go:102` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `EvaluateLifecycleControl` | R | R gate |
| `pkg/factorysessionexecution/listing.go:90` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `MatchesDurableSessionListFilters` | R | R gate |
| `pkg/factorysessionexecution/listing.go:251` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `matchesLifecycleTimeFilters` | R | R gate |
| `pkg/factorysessionexecution/resume.go:1` | `backendsizecheck:ignore-file` | `pkg/factorysessionexecution/resume.go` | F | F gate |
| `pkg/factorysessionexecution/resume.go:2` | `pkgmaintcheck:ignore-file-lines` | `pkg/factorysessionexecution/resume.go` | F | F gate |
| `pkg/factorysessionexecution/resume.go:184` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `validateDurableResumeFacts` | R | R gate |

### `pkg/factorysessionexecution/fixtures`

Owner: `pkg/factorysessionexecution/fixtures` package maintainers. Status: **directive only**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/factorysessionexecution/fixtures/inspection_test.go:237` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestFakeService_InterruptDispatchRace_ObservableServiceOutcomes` | T | T gate |
| `pkg/factorysessionexecution/fixtures/runtime_live_child_test.go:371` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestJavaScriptRuntimeService_AgentRunLiveChildFailure_ProjectsFailedDispatchOnWorkflowFailure` | T | T gate |
| `pkg/factorysessionexecution/fixtures/runtime_live_child_test.go:701` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `assertParallelFailureFailedDispatch` | T | T gate |
| `pkg/factorysessionexecution/fixtures/runtime_restart_resume_test.go:1` | `backendsizecheck:ignore-file` | `pkg/factorysessionexecution/fixtures/runtime_restart_resume_test.go` | T | T gate |
| `pkg/factorysessionexecution/fixtures/runtime_restart_resume_test.go:2` | `pkgmaintcheck:ignore-file-lines` | `pkg/factorysessionexecution/fixtures/runtime_restart_resume_test.go` | T | T gate |

### `pkg/interfaces`

Owner: `pkg/interfaces` package maintainers. Status: **directive + coverage baseline**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/interfaces/interfaces_contract_test.go:1` | `backendsizecheck:ignore-file` | `pkg/interfaces/interfaces_contract_test.go` | T | T gate |
| `pkg/interfaces/interfaces_contract_test.go:2` | `pkgmaintcheck:ignore-file-lines` | `pkg/interfaces/interfaces_contract_test.go` | T | T gate |
| `pkg/interfaces/work_runtime_test.go:812` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestCloneToken_PreserveNilAndEmptyValues` | T | T gate |

### `pkg/internal/cursorstorage`

Owner: `pkg/internal/cursorstorage` package maintainers. Status: **directive + coverage baseline**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/internal/cursorstorage/protobuf_decoder.go:102` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `extractProtobufFields` | P | P gate |
| `pkg/internal/cursorstorage/redacted_reasoning_decoder.go:16` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `decodeRedactedReasoning` | P | P gate |
| `pkg/internal/cursorstorage/store_blob_decode.go:14` | `backendsizecheck:ignore-function` | `decodeBlobEntryValue` | P | P gate |
| `pkg/internal/cursorstorage/store_blob_decode.go:15` | `pkgmaintcheck:ignore-function-lines` | `decodeBlobEntryValue` | P | P gate |
| `pkg/internal/cursorstorage/store_blob_decode.go:16` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `decodeBlobEntryValue` | P | P gate |
| `pkg/internal/cursorstorage/store_parse.go:122` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `parseTextMessageFormat` | P | P gate |
| `pkg/internal/cursorstorage/store_parse.go:222` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `parseComposerFromData` | P | P gate |
| `pkg/internal/cursorstorage/store_parse.go:439` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `messageUnknownContentText` | P | P gate |
| `pkg/internal/cursorstorage/store_query.go:8` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `QueryBlobsTable` | P | P gate |
| `pkg/internal/cursorstorage/store_query.go:111` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `QueryMetaTable` | P | P gate |

### `pkg/mcp/factorysession`

Owner: `pkg/mcp/factorysession` package maintainers. Status: **directive + coverage baseline**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/mcp/factorysession/execution_test.go:45` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestMockClient_GetSession_RunningFixtureReturnsDeterministicStatus` | T | T gate |
| `pkg/mcp/factorysession/execution_test.go:135` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestMockClient_AsyncPolling_ObservesCompletedFixtureThroughStatusAndResult` | T | T gate |
| `pkg/mcp/factorysession/execution_test.go:279` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestMockClient_StartSync_SuccessFixtureReturnsTerminalSession` | T | T gate |
| `pkg/mcp/factorysession/execution_test.go:336` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestMockClient_GetResult_TerminalSessionReturnsDeterministicResult` | T | T gate |
| `pkg/mcp/factorysession/failure_paths_test.go:52` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestMockClient_GetResult_FailedFixtureReturnsPartialResultWithFailureDetails` | T | T gate |
| `pkg/mcp/factorysession/inspection.go:157` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `invokeLifecycleControl` | R | R gate |
| `pkg/mcp/factorysession/inspection_test.go:216` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestMockClient_ListArtifacts_ArtifactInspectionFixtureReturnsStableSummaries` | T | T gate |
| `pkg/mcp/factorysession/inspection_test.go:306` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestMockClient_ReadEvents_EventReconnectFixtureReturnsOrderedCanonicalEvents` | T | T gate |
| `pkg/mcp/factorysession/inspection_test.go:363` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestMockClient_Control_LifecycleFixtureReturnsAcceptedRejectedAndIsolatesSessions` | T | T gate |

### `pkg/replay`

Owner: `pkg/replay` package maintainers. Status: **directive + coverage baseline**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/replay/event_artifact_test.go:287` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `assertMergedGeneratedWorkstations` | T | T gate |
| `pkg/replay/event_log_test.go:459` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `assertReducedCompletionSafeDiagnostics` | T | T gate |

### `pkg/replay/configtests`

Owner: `pkg/replay/configtests` package maintainers. Status: **directive only**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/replay/configtests/effective_config_generated_test.go:63` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `assertEmbeddedGeneratedFactory` | T | T gate |
| `pkg/replay/configtests/effective_config_test.go:335` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestGeneratedFactoryFromLoadedConfig_EmitsCanonicalPublicWorkstationKind` | T | T gate |
| `pkg/replay/configtests/generated_factory_test.go:19` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestGeneratedFactoryFromLoadedConfig_EmbedsSplitRuntimeDefinitionsInGeneratedFactory` | T | T gate |

### `pkg/runtimehost`

Owner: `pkg/runtimehost` package maintainers. Status: **directive + coverage baseline**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/runtimehost/definitions.go:1` | `backendsizecheck:ignore-file` | `pkg/runtimehost/definitions.go` | F | F gate |
| `pkg/runtimehost/host.go:1` | `backendsizecheck:ignore-file` | `pkg/runtimehost/host.go` | F | F gate |
| `pkg/runtimehost/host.go:2` | `pkgmaintcheck:ignore-file-lines` | `pkg/runtimehost/host.go` | F | F gate |
| `pkg/runtimehost/runtime_sessions.go:1` | `backendsizecheck:ignore-file` | `pkg/runtimehost/runtime_sessions.go` | F | F gate |
| `pkg/runtimehost/runtime_sessions.go:2` | `pkgmaintcheck:ignore-file-lines` | `pkg/runtimehost/runtime_sessions.go` | F | F gate |

### `pkg/service`

Owner: `pkg/service` package maintainers. Status: **directive + coverage baseline**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/service/factory_build.go:1` | `backendsizecheck:ignore-file` | `pkg/service/factory_build.go` | F | F gate |
| `pkg/service/factory_build.go:2` | `pkgmaintcheck:ignore-file-lines` | `pkg/service/factory_build.go` | F | F gate |
| `pkg/service/factory_dashboard_test.go:96` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestFactoryService_Run_APIServerStarterReceivesWorkingAPISurface` | T | T gate |
| `pkg/service/factory_runtime_mode_test.go:1` | `backendsizecheck:ignore-file` | `pkg/service/factory_runtime_mode_test.go` | T | T gate |
| `pkg/service/factory_runtime_mode_test.go:2` | `pkgmaintcheck:ignore-file-lines` | `pkg/service/factory_runtime_mode_test.go` | T | T gate |
| `pkg/service/factory_runtime_mode_test.go:56` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestBuildFactoryService_ServiceModeAcceptsLateSubmissionAfterIdleStartup` | T | T gate |
| `pkg/service/factory_runtime_mode_test.go:235` | `pkgmaintcheck:ignore-function-lines` | `TestBuildFactoryService_ServiceModeRuntimeMetricsCaptureDispatchOutcomes` | T | T gate |
| `pkg/service/factory_runtime_mode_test.go:345` | `pkgmaintcheck:ignore-function-lines` | `TestBuildFactoryService_ServiceModeRuntimeMetricsCaptureProviderAndScriptDiagnostics` | T | T gate |
| `pkg/service/factory_runtime_mode_test.go:346` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestBuildFactoryService_ServiceModeRuntimeMetricsCaptureProviderAndScriptDiagnostics` | T | T gate |
| `pkg/service/factory_runtime_mode_test.go:850` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestFactoryService_RunPreservesSnapshotAndFactoryEventObservability` | T | T gate |
| `pkg/service/factory_runtime_mode_test.go:1862` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestFactoryService_ModelMethodsDelegateToModelService` | T | T gate |
| `pkg/service/factory_runtime_mode_test.go:2010` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestFactoryService_LifecycleMethodsDelegateToCoordinator` | T | T gate |
| `pkg/service/factory_test.go:1` | `pkgmaintcheck:ignore-file-lines` | `pkg/service/factory_test.go` | T | T gate |
| `pkg/service/factory_test.go:2` | `backendsizecheck:ignore-file` | `pkg/service/factory_test.go` | T | T gate |
| `pkg/service/factory_test.go:1719` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestFactoryService_CreateNamedFactory_MaterializesSupportedPortableBundledFiles` | T | T gate |
| `pkg/service/factory_test_helpers_test.go:1` | `backendsizecheck:ignore-file` | `pkg/service/factory_test_helpers_test.go` | T | T gate |
| `pkg/service/factory_test_helpers_test.go:2` | `pkgmaintcheck:ignore-file-lines` | `pkg/service/factory_test_helpers_test.go` | T | T gate |
| `pkg/service/poller_watcher_test.go:1` | `pkgmaintcheck:ignore-file-lines` | `pkg/service/poller_watcher_test.go` | T | T gate |
| `pkg/service/poller_watcher_test.go:2` | `backendsizecheck:ignore-file` | `pkg/service/poller_watcher_test.go` | T | T gate |
| `pkg/service/runtime_session_runtime_test.go:1` | `backendsizecheck:ignore-file` | `pkg/service/runtime_session_runtime_test.go` | T | T gate |
| `pkg/service/runtime_session_runtime_test.go:2` | `pkgmaintcheck:ignore-file-lines` | `pkg/service/runtime_session_runtime_test.go` | T | T gate |
| `pkg/service/runtime_sessions.go:1` | `backendsizecheck:ignore-file` | `pkg/service/runtime_sessions.go` | F | F gate |
| `pkg/service/runtime_sessions.go:2` | `pkgmaintcheck:ignore-file-lines` | `pkg/service/runtime_sessions.go` | F | F gate |
| `pkg/service/testmain_test.go:1` | `pkgmaintcheck:ignore-file-lines` | `pkg/service/testmain_test.go` | T | T gate |
| `pkg/service/testmain_test.go:2` | `backendsizecheck:ignore-file` | `pkg/service/testmain_test.go` | T | T gate |
| `pkg/service/testmain_test.go:839` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `assertReplayArtifactStoresCanonicalEvents` | T | T gate |
| `pkg/service/testmain_test.go:897` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestBuildFactoryService_RecordModeStreamsReadableArtifactBeforeShutdown` | T | T gate |
| `pkg/service/testmain_test.go:1609` | `backendsizecheck:ignore-function` | `TestLoadWorkersFromConfig_ModelInvokeContractExecutesAcrossLocalAndCloudWorkers` | T | T gate |
| `pkg/service/testmain_test.go:1645` | `backendsizecheck:ignore-function` | `TestLoadWorkersFromConfig_ModelInvokeWorkstationExecutesThroughLocalManagedRuntimeEdge` | T | T gate |

### `pkg/service/runtimelogtests`

Owner: `pkg/service/runtimelogtests` package maintainers. Status: **directive only**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/service/runtimelogtests/factory_runtime_log_test.go:23` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestFactoryService_RunWritesStructuredRuntimeLogFile` | T | T gate |
| `pkg/service/runtimelogtests/factory_runtime_log_test.go:357` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestFactoryService_RunWritesCommandRunnerEventsWithOutputsToRuntimeLog` | T | T gate |

### `pkg/workers/executor`

Owner: `pkg/workers/executor` package maintainers. Status: **directive + coverage baseline**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/workers/executor/agent_inference_test.go:112` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestAgentExecutor_InferenceRequestUsesCanonicalWorkDispatchPayload` | T | T gate |
| `pkg/workers/executor/script_test.go:542` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `assertSharedRunnerRequest` | T | T gate |
| `pkg/workers/executor/script_test.go:876` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `assertScriptRequestEvent` | T | T gate |
| `pkg/workers/executor/script_test.go:913` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `assertScriptResponsePayload` | T | T gate |
| `pkg/workers/executor/workstation_executor_test.go:1` | `backendsizecheck:ignore-file` | `pkg/workers/executor/workstation_executor_test.go` | T | T gate |
| `pkg/workers/executor/workstation_executor_test.go:2` | `pkgmaintcheck:ignore-file-lines` | `pkg/workers/executor/workstation_executor_test.go` | T | T gate |
| `pkg/workers/executor/workstation_executor_test.go:186` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestWorkstationExecutor_ModelWorkstationUsesCanonicalWorkstationRuntimeFields` | T | T gate |

### `pkg/workers/provider`

Owner: `pkg/workers/provider` package maintainers. Status: **directive only**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/workers/provider/inference_provider_error_normalization_test.go:958` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `assertNormalizedProviderFailure` | T | T gate |
| `pkg/workers/provider/recording_provider_test.go:466` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestRecordingProvider_Infer_MissingInnerProviderEmitsMisconfiguredFailureEvent` | T | T gate |

### `tests/functional/runtime_api/factory_transformation`

Owner: `tests/functional/runtime_api/factory_transformation` package maintainers. Status: **directive only**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `tests/functional/runtime_api/factory_transformation/api_current_factory_put_layout_test.go:84` | `backendsizecheck:ignore-function` | `TestCurrentFactoryPUT_PreservesPortableLayoutThroughSaveReloadAndRuntimeExecution` | T | T gate |

## Non-handwritten and checker-owned matches

### Generated, vendored, and fixture-data exclusions

The focused scan found **0** directive matches in excluded generated, vendored, or
`testdata` source; this revision has no `vendor/` directory. These are not handwritten debt. The checker policy in
`internal/backendsizecheck/policy.go` excludes `pkg/api/generated`, `pkg/generatedclient`,
any `pkg/**/testdata` or `tests/**/testdata` directory, and all of `vendor` because those
trees are generated output, test inputs, or third-party code rather than maintained backend
source. A zero-match section is retained so future refreshes cannot conflate an exclusion
with an unexamined path.

### Checker-owned fixture strings

These **8** matches are string literals used to verify directive parsing. They are not comments
attached to repository production or test declarations and are not active exemptions:

| Source | Fixture spelling |
| --- | --- |
| `cmd/backendsizecheck/main_test.go:127` | `backendsizecheck:ignore-file` |
| `cmd/backendsizecheck/main_test.go:141` | `backendsizecheck:ignore-function` |
| `cmd/pkgmaintcheck/main_test.go:120` | `pkgmaintcheck:ignore-file-lines` |
| `cmd/pkgmaintcheck/main_test.go:123` | `pkgmaintcheck:ignore-function-lines` |
| `cmd/pkgmaintcheck/main_test.go:124` | `pkgmaintcheck:ignore-cyclomatic-complexity` |
| `cmd/pkgmaintcheck/main_test.go:138` | `pkgmaintcheck:ignore-function-lines` |
| `cmd/pkgmaintcheck/main_test.go:139` | `pkgmaintcheck:ignore-cyclomatic-complexity` |
| `cmd/pkgmaintcheck/main_test.go:207` | `pkgmaintcheck:ignore-file-lines` |

## Refresh procedure

This document is a snapshot, not a generated registry. To refresh it:

1. Record `git rev-parse HEAD` and a UTC scan timestamp, then rerun the exact focused scan above.
2. Separate checker fixture strings and policy-excluded paths before counting active handwritten comments.
3. Resolve each function-scoped comment to its attached Go declaration and revalidate its owner, reason class, and objective removal evidence.
4. Parse `docs/internal/development/go-coverage-package-baseline.txt` with whitespace trimming, blank/comment removal, and deduplication; recompute all three package sets and totals.
5. Replace this revision-stamped snapshot and run `go run ./cmd/backendsizecheck` and `go run ./cmd/pkgmaintcheck ./pkg`. Do not add a source-scanning inventory test or change a checker/baseline merely to match the document.
