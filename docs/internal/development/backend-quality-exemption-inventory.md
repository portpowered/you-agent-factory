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
**72** packages. Of **37** directive-owning package directories,
**18** are in both systems and **19** are directive only; the remaining
**54** baseline packages are coverage-baseline only.

| Owning package | Active directives | Files | Quality-system status |
| --- | ---: | ---: | --- |
| `pkg/transports/http` | 7 | 3 | directive + coverage baseline |
| `pkg/transports/http/providersessioncursor` | 1 | 1 | directive only |
| `pkg/transports/http/servertests` | 1 | 1 | directive only |
| `pkg/transports/http/workstationprojection` | 3 | 1 | directive only |
| `pkg/transports/mapping/factorysession` | 8 | 6 | directive only |
| `pkg/transports/cli` | 2 | 1 | directive only |
| `pkg/transports/cli/cliinputs` | 5 | 3 | directive only |
| `pkg/transports/cli/config` | 2 | 1 | directive + coverage baseline |
| `pkg/transports/cli/init` | 2 | 1 | directive only |
| `pkg/transports/cli/mcp` | 3 | 3 | directive + coverage baseline |
| `pkg/transports/cli/run` | 2 | 1 | directive + coverage baseline |
| `pkg/transports/cli/submit` | 1 | 1 | directive only |
| `pkg/transports/cli/work` | 1 | 1 | directive only |
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
| `pkg/factory/sessions/execution` | 26 | 8 | directive only |
| `pkg/factory/sessions/execution/fixtures` | 5 | 3 | directive only |
| `pkg/factory/contracts` | 3 | 2 | directive + coverage baseline |
| `pkg/platform/cursors` | 10 | 5 | directive + coverage baseline |
| `pkg/transports/mcp/factorysession` | 9 | 4 | directive + coverage baseline |
| `pkg/factory/replay` | 2 | 2 | directive + coverage baseline |
| `pkg/factory/replay/configtests` | 3 | 3 | directive only |
| `pkg/runtimehost` | 5 | 3 | directive + coverage baseline |
| `pkg/service` | 29 | 9 | directive + coverage baseline |
| `pkg/service/runtimelogtests` | 2 | 1 | directive only |
| `pkg/workers/executor` | 7 | 3 | directive + coverage baseline |
| `pkg/workers/provider` | 2 | 2 | directive only |
| `tests/functional/runtime_api/factory_transformation` | 1 | 1 | directive only |

### Coverage-baseline-only packages

These entries are retained verbatim after whitespace normalization; they have no active
directive occurrence in the focused scan:

- `pkg/transports/http/apitypes`
- `pkg/transports/mapping`
- `pkg/transports/mapping/factorydefinition`
- `pkg/transports/mapping/optional`
- `pkg/transports/cli/clidiag`
- `pkg/transports/cli/dashboardrender`
- `pkg/transports/cli/default`
- `pkg/transports/cli/models`
- `pkg/transports/cli/session`
- `pkg/transports/cli/sessionexecution`
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
- `pkg/workers/hosted`
- `pkg/workers/hosted/linear`
- `pkg/invocations`
- `pkg/models/local`
- `pkg/models/assets`
- `pkg/platform/logging`
- `pkg/transports/mcp/server`
- `pkg/models/host`
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

### `pkg/transports/http`

Owner: `pkg/transports/http` package maintainers. Status: **directive + coverage baseline**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/transports/http/server_factory_sessions_test.go:1` | `pkgmaintcheck:ignore-file-lines` | `pkg/transports/http/server_factory_sessions_test.go` | T | T gate |
| `pkg/transports/http/server_factory_sessions_test.go:2` | `backendsizecheck:ignore-file` | `pkg/transports/http/server_factory_sessions_test.go` | T | T gate |
| `pkg/transports/http/server_submit_work_test.go:125` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestSubmitWork_AcceptsCanonicalContent` | T | T gate |
| `pkg/transports/http/server_work_query_test.go:1` | `backendsizecheck:ignore-file` | `pkg/transports/http/server_work_query_test.go` | T | T gate |
| `pkg/transports/http/server_work_query_test.go:2` | `pkgmaintcheck:ignore-file-lines` | `pkg/transports/http/server_work_query_test.go` | T | T gate |
| `pkg/transports/http/server_work_query_test.go:722` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestListWork_ReturnsRuntimeRelationsWithSourceToTargetDirection` | T | T gate |
| `pkg/transports/http/server_work_query_test.go:1054` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestUpsertWorkRequest_MapsWorkTypeNameAndRelationsToRuntime` | T | T gate |

### `pkg/transports/http/providersessioncursor`

Owner: `pkg/transports/http/providersessioncursor` package maintainers. Status: **directive only**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/transports/http/providersessioncursor/detail_test.go:15` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestLoadDetails_ReadsReadableSessionFromConfiguredRoot` | T | T gate |

### `pkg/transports/http/servertests`

Owner: `pkg/transports/http/servertests` package maintainers. Status: **directive only**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/transports/http/servertests/server_durable_session_interrupt_dispatch_test.go:262` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestInterruptFactorySessionDispatch_LateResultAfterInterruptSuppressedFromNormalRouting` | T | T gate |

### `pkg/transports/http/workstationprojection`

Owner: `pkg/transports/http/workstationprojection` package maintainers. Status: **directive only**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/transports/http/workstationprojection/projection_test.go:128` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestBuildFactoryWorldWorkstationRequestProjectionSlice_PreservesPendingDispatchWithoutInferenceFallback` | T | T gate |
| `pkg/transports/http/workstationprojection/projection_test.go:676` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `assertCompletedProjectionRequest` | T | T gate |
| `pkg/transports/http/workstationprojection/projection_test.go:848` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `assertCompletedScriptProjection` | T | T gate |

### `pkg/transports/mapping/factorysession`

Owner: `pkg/transports/mapping/factorysession` package maintainers. Status: **directive only**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/transports/mapping/factorysession/factory_session_execution_test.go:224` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestSyncStartResponseToAPI_MapsTerminalAndTimeoutFixtures` | T | T gate |
| `pkg/transports/mapping/factorysession/factory_session_fake_consumer_test.go:13` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestFakeServiceConsumer_ProjectsFixtureThroughApisurfaceMappers` | T | T gate |
| `pkg/transports/mapping/factorysession/factory_session_lifecycle.go:344` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `lifecycleTimestampsToAPI` | R | R gate |
| `pkg/transports/mapping/factorysession/factory_session_mapper.go:60` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `SessionReadResultFromAPI` | R | R gate |
| `pkg/transports/mapping/factorysession/factory_session_mapper.go:226` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `DispatchDetailFromAPI` | R | R gate |
| `pkg/transports/mapping/factorysession/factory_session_mapper.go:407` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `DurableSessionListSummaryFromAPI` | R | R gate |
| `pkg/transports/mapping/factorysession/factory_session_mapper_test.go:684` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `assertListSummaryFieldsPreserved` | T | T gate |
| `pkg/transports/mapping/factorysession/factory_session_projection_test.go:350` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestProjectionResponses_TrimAndOmitOptionalFields` | T | T gate |

### `pkg/transports/cli`

Owner: `pkg/transports/cli` package maintainers. Status: **directive only**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/transports/cli/root_run_test.go:1` | `backendsizecheck:ignore-file` | `pkg/transports/cli/root_run_test.go` | T | T gate |
| `pkg/transports/cli/root_run_test.go:2` | `pkgmaintcheck:ignore-file-lines` | `pkg/transports/cli/root_run_test.go` | T | T gate |

### `pkg/transports/cli/cliinputs`

Owner: `pkg/transports/cli/cliinputs` package maintainers. Status: **directive only**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/transports/cli/cliinputs/parser_parity_test.go:25` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestProductionParserParity_RepresentativeCommands` | T | T gate |
| `pkg/transports/cli/cliinputs/parser_parity_test.go:128` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `productionParserParityRunFlagCases` | T | T gate |
| `pkg/transports/cli/cliinputs/parser_parity_test.go:207` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `productionParserParitySubmitCases` | T | T gate |
| `pkg/transports/cli/cliinputs/synthetic_args_relationships_test.go:143` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `assertSyntheticArgumentRecord` | T | T gate |
| `pkg/transports/cli/cliinputs/synthetic_flags_test.go:437` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `assertSyntheticFlagRecord` | T | T gate |

### `pkg/transports/cli/config`

Owner: `pkg/transports/cli/config` package maintainers. Status: **directive + coverage baseline**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/transports/cli/config/config_test.go:228` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `assertDeterministicExpandedRuntimeConfig` | T | T gate |
| `pkg/transports/cli/config/config_test.go:868` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `assertExistingSplitDefinitionsPreserved` | T | T gate |

### `pkg/transports/cli/init`

Owner: `pkg/transports/cli/init` package maintainers. Status: **directive only**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/transports/cli/init/init_test.go:401` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `assertRalphRuntimeConfig` | T | T gate |
| `pkg/transports/cli/init/init_test.go:732` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `assertInitScaffoldFilesCanonical` | T | T gate |

### `pkg/transports/cli/mcp`

Owner: `pkg/transports/cli/mcp` package maintainers. Status: **directive + coverage baseline**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/transports/cli/mcp/serve_runtime_resume_smoke_test.go:24` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestRunServe_RuntimeResumeSmoke_InterruptedSessionResumesThroughMCPControl` | T | T gate |
| `pkg/transports/cli/mcp/serve_runtime_smoke_test.go:26` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestRunServe_RuntimeSmoke_DiscoveryAsyncPollAndResult` | T | T gate |
| `pkg/transports/cli/mcp/serve_smoke_test.go:29` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestRunServe_InstallSmoke_DiscoveryValidateAsyncPoll` | T | T gate |

### `pkg/transports/cli/run`

Owner: `pkg/transports/cli/run` package maintainers. Status: **directive + coverage baseline**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/transports/cli/run/run_invocation_test.go:1` | `backendsizecheck:ignore-file` | `pkg/transports/cli/run/run_invocation_test.go` | T | T gate |
| `pkg/transports/cli/run/run_invocation_test.go:2` | `pkgmaintcheck:ignore-file-lines` | `pkg/transports/cli/run/run_invocation_test.go` | T | T gate |

### `pkg/transports/cli/submit`

Owner: `pkg/transports/cli/submit` package maintainers. Status: **directive only**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/transports/cli/submit/submit_test.go:203` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestSubmit_JSONPayloadPostsWorkTypeName` | T | T gate |

### `pkg/transports/cli/work`

Owner: `pkg/transports/cli/work` package maintainers. Status: **directive only**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/transports/cli/work/list_test.go:675` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestList_JSONOutputPreservesGeneratedResponseShape` | T | T gate |

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

### `pkg/factory/sessions/execution`

Owner: `pkg/factory/sessions/execution` package maintainers. Status: **directive only**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/factory/sessions/execution/control.go:411` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `applyRuntimeExtendedLifecycleControl` | R | R gate |
| `pkg/factory/sessions/execution/control.go:412` | `backendsizecheck:ignore-function` | `applyRuntimeExtendedLifecycleControl` | R | R gate |
| `pkg/factory/sessions/execution/control.go:413` | `pkgmaintcheck:ignore-function-lines` | `applyRuntimeExtendedLifecycleControl` | R | R gate |
| `pkg/factory/sessions/execution/control.go:519` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `applyRuntimeAcceptedLifecycleControl` | R | R gate |
| `pkg/factory/sessions/execution/fake_fixture.go:93` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `parseFakeScenarioFromFixture` | R | R gate |
| `pkg/factory/sessions/execution/fake_fixture.go:222` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `sessionReadFromFixtureMap` | R | R gate |
| `pkg/factory/sessions/execution/fake_service.go:449` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `applyLifecycleControl` | R | R gate |
| `pkg/factory/sessions/execution/fake_service.go:548` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `mutateSessionForControl` | R | R gate |
| `pkg/factory/sessions/execution/fake_service_runtime_test.go:1` | `backendsizecheck:ignore-file` | `pkg/factory/sessions/execution/fake_service_runtime_test.go` | T | T gate |
| `pkg/factory/sessions/execution/fake_service_runtime_test.go:2` | `pkgmaintcheck:ignore-file-lines` | `pkg/factory/sessions/execution/fake_service_runtime_test.go` | T | T gate |
| `pkg/factory/sessions/execution/fake_service_runtime_test.go:234` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestFakeService_InternalLifecycleHelpers` | T | T gate |
| `pkg/factory/sessions/execution/fake_service_runtime_test.go:2274` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestJavaScriptRuntimeService_LateChildResultAfterInterrupt_SuppressesNormalRouting` | T | T gate |
| `pkg/factory/sessions/execution/fake_service_runtime_test.go:2368` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestFakeService_InterruptAcceptedBeforeCompletion_ObservableDispatchAndEventOutcomes` | T | T gate |
| `pkg/factory/sessions/execution/fake_service_runtime_test.go:2668` | `pkgmaintcheck:ignore-function-lines` | `TestJavaScriptRuntimeService_PausePersistsStablePartialTerminalReadState` | T | T gate |
| `pkg/factory/sessions/execution/fake_service_runtime_test.go:2669` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestJavaScriptRuntimeService_PausePersistsStablePartialTerminalReadState` | T | T gate |
| `pkg/factory/sessions/execution/fake_service_test.go:1` | `backendsizecheck:ignore-file` | `pkg/factory/sessions/execution/fake_service_test.go` | T | T gate |
| `pkg/factory/sessions/execution/fake_service_test.go:2` | `pkgmaintcheck:ignore-file-lines` | `pkg/factory/sessions/execution/fake_service_test.go` | T | T gate |
| `pkg/factory/sessions/execution/fake_service_test.go:1007` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestProjectRuntimeExecutionRecords_FailedLiveChild_ProjectsFailureDetail` | T | T gate |
| `pkg/factory/sessions/execution/fake_service_test.go:1832` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestFakeService_InterruptDispatch_RecordsDispatchInterruptedEvent` | T | T gate |
| `pkg/factory/sessions/execution/fake_service_test.go:2195` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestFakeService_PauseResumeAppendsLifecycleControlEventsWithoutNoOpMutation` | T | T gate |
| `pkg/factory/sessions/execution/lifecycle.go:102` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `EvaluateLifecycleControl` | R | R gate |
| `pkg/factory/sessions/execution/listing.go:90` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `MatchesDurableSessionListFilters` | R | R gate |
| `pkg/factory/sessions/execution/listing.go:251` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `matchesLifecycleTimeFilters` | R | R gate |
| `pkg/factory/sessions/execution/resume.go:1` | `backendsizecheck:ignore-file` | `pkg/factory/sessions/execution/resume.go` | F | F gate |
| `pkg/factory/sessions/execution/resume.go:2` | `pkgmaintcheck:ignore-file-lines` | `pkg/factory/sessions/execution/resume.go` | F | F gate |
| `pkg/factory/sessions/execution/resume.go:184` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `validateDurableResumeFacts` | R | R gate |

### `pkg/factory/sessions/execution/fixtures`

Owner: `pkg/factory/sessions/execution/fixtures` package maintainers. Status: **directive only**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/factory/sessions/execution/fixtures/inspection_test.go:237` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestFakeService_InterruptDispatchRace_ObservableServiceOutcomes` | T | T gate |
| `pkg/factory/sessions/execution/fixtures/runtime_live_child_test.go:371` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestJavaScriptRuntimeService_AgentRunLiveChildFailure_ProjectsFailedDispatchOnWorkflowFailure` | T | T gate |
| `pkg/factory/sessions/execution/fixtures/runtime_live_child_test.go:701` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `assertParallelFailureFailedDispatch` | T | T gate |
| `pkg/factory/sessions/execution/fixtures/runtime_restart_resume_test.go:1` | `backendsizecheck:ignore-file` | `pkg/factory/sessions/execution/fixtures/runtime_restart_resume_test.go` | T | T gate |
| `pkg/factory/sessions/execution/fixtures/runtime_restart_resume_test.go:2` | `pkgmaintcheck:ignore-file-lines` | `pkg/factory/sessions/execution/fixtures/runtime_restart_resume_test.go` | T | T gate |

### `pkg/factory/contracts`

Owner: `pkg/factory/contracts` package maintainers. Status: **directive + coverage baseline**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/factory/contracts/interfaces_contract_test.go:1` | `backendsizecheck:ignore-file` | `pkg/factory/contracts/interfaces_contract_test.go` | T | T gate |
| `pkg/factory/contracts/interfaces_contract_test.go:2` | `pkgmaintcheck:ignore-file-lines` | `pkg/factory/contracts/interfaces_contract_test.go` | T | T gate |
| `pkg/factory/contracts/work_runtime_test.go:812` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestCloneToken_PreserveNilAndEmptyValues` | T | T gate |

### `pkg/platform/cursors`

Owner: `pkg/platform/cursors` package maintainers. Status: **directive + coverage baseline**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/platform/cursors/protobuf_decoder.go:102` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `extractProtobufFields` | P | P gate |
| `pkg/platform/cursors/redacted_reasoning_decoder.go:16` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `decodeRedactedReasoning` | P | P gate |
| `pkg/platform/cursors/store_blob_decode.go:14` | `backendsizecheck:ignore-function` | `decodeBlobEntryValue` | P | P gate |
| `pkg/platform/cursors/store_blob_decode.go:15` | `pkgmaintcheck:ignore-function-lines` | `decodeBlobEntryValue` | P | P gate |
| `pkg/platform/cursors/store_blob_decode.go:16` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `decodeBlobEntryValue` | P | P gate |
| `pkg/platform/cursors/store_parse.go:122` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `parseTextMessageFormat` | P | P gate |
| `pkg/platform/cursors/store_parse.go:222` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `parseComposerFromData` | P | P gate |
| `pkg/platform/cursors/store_parse.go:439` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `messageUnknownContentText` | P | P gate |
| `pkg/platform/cursors/store_query.go:8` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `QueryBlobsTable` | P | P gate |
| `pkg/platform/cursors/store_query.go:111` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `QueryMetaTable` | P | P gate |

### `pkg/transports/mcp/factorysession`

Owner: `pkg/transports/mcp/factorysession` package maintainers. Status: **directive + coverage baseline**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/transports/mcp/factorysession/execution_test.go:45` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestMockClient_GetSession_RunningFixtureReturnsDeterministicStatus` | T | T gate |
| `pkg/transports/mcp/factorysession/execution_test.go:135` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestMockClient_AsyncPolling_ObservesCompletedFixtureThroughStatusAndResult` | T | T gate |
| `pkg/transports/mcp/factorysession/execution_test.go:279` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestMockClient_StartSync_SuccessFixtureReturnsTerminalSession` | T | T gate |
| `pkg/transports/mcp/factorysession/execution_test.go:336` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestMockClient_GetResult_TerminalSessionReturnsDeterministicResult` | T | T gate |
| `pkg/transports/mcp/factorysession/failure_paths_test.go:52` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestMockClient_GetResult_FailedFixtureReturnsPartialResultWithFailureDetails` | T | T gate |
| `pkg/transports/mcp/factorysession/inspection.go:157` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `invokeLifecycleControl` | R | R gate |
| `pkg/transports/mcp/factorysession/inspection_test.go:216` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestMockClient_ListArtifacts_ArtifactInspectionFixtureReturnsStableSummaries` | T | T gate |
| `pkg/transports/mcp/factorysession/inspection_test.go:306` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestMockClient_ReadEvents_EventReconnectFixtureReturnsOrderedCanonicalEvents` | T | T gate |
| `pkg/transports/mcp/factorysession/inspection_test.go:363` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestMockClient_Control_LifecycleFixtureReturnsAcceptedRejectedAndIsolatesSessions` | T | T gate |

### `pkg/factory/replay`

Owner: `pkg/factory/replay` package maintainers. Status: **directive + coverage baseline**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/factory/replay/event_artifact_test.go:287` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `assertMergedGeneratedWorkstations` | T | T gate |
| `pkg/factory/replay/event_log_test.go:459` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `assertReducedCompletionSafeDiagnostics` | T | T gate |

### `pkg/factory/replay/configtests`

Owner: `pkg/factory/replay/configtests` package maintainers. Status: **directive only**.

| Source | Directive rule | Target | Reason | Evidence |
| --- | --- | --- | --- | --- |
| `pkg/factory/replay/configtests/effective_config_generated_test.go:63` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `assertEmbeddedGeneratedFactory` | T | T gate |
| `pkg/factory/replay/configtests/effective_config_test.go:335` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestGeneratedFactoryFromLoadedConfig_EmitsCanonicalPublicWorkstationKind` | T | T gate |
| `pkg/factory/replay/configtests/generated_factory_test.go:19` | `pkgmaintcheck:ignore-cyclomatic-complexity` | `TestGeneratedFactoryFromLoadedConfig_EmbedsSplitRuntimeDefinitionsInGeneratedFactory` | T | T gate |

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

## External ownership overlay

> Ownership snapshot time: `2026-07-13T07:18:59Z` (UTC)
>
> Local source base: `dcda359c40703b01c3d1e0b32bbd03a547f9313f` (`main`)
>
> Inventory document head before this overlay: `619ae00f4f88e781f08c02bed81f411be60aaf16`

This overlay is dispatch-time coordination, not permanent code ownership. **Externally
owned** means that the file is inside an active or reserved dynamic-cleanup work item, or
is present in an open pull request at the exact head recorded below. No duplicate exemption
cleanup should be assigned there. **Unowned at snapshot** means only that none of those
file sets collided at the timestamp above; refresh the open-PR and active-worktree sets
before dispatching a later cleanup.

The authoritative dynamic-cleanup definitions were the planner artifacts
`docs/temp/dynamic-workflows-cleanup/005-root-wire-foundation.json` through
`008-runtime-shim-removal.json`, with request IDs
`dynamic-workflows-cleanup-005-root-wire-foundation`,
`dynamic-workflows-cleanup-006-domain-package-convergence`,
`dynamic-workflows-cleanup-007-session-service-convergence`, and
`dynamic-workflows-cleanup-008-runtime-shim-removal`. Their reservation sets were applied
as follows:

| Batch/work item | Exact reservation used for this overlay |
| --- | --- |
| Batch 005 — `create-root-process-owner` | Active head `4516ea35aeff5cdde5e578b9000b8054db6bb834`; changed files: `pkg/transports/cli/root.go`, `pkg/transports/cli/root_run_test.go`, `pkg/transports/cli/root_work.go`, `pkg/config/operatorconfig/environment_resolution.go`, `pkg/root/input.go`, `pkg/root/root.go`, `pkg/root/root_test.go`. |
| Batch 005 — `create-wire-application-graph` | Active head `8015a87fb4594a1659ab21c6a7720bf324ea7a85`; changed files: `pkg/wire/doc.go`, `pkg/wire/graph.go`, `pkg/wire/graph_test.go`. The dependency-blocked initializer item had no implementation file set yet. |
| Batch 006 — `converge-transport-family` | Every detailed directive file under `pkg/transports/http/**`, `pkg/transports/mapping/**`, `pkg/transports/cli/**`, and `pkg/transports/mcp/**`. |
| Batch 006 — `converge-model-worker-families` | Every detailed directive file under `pkg/workers/**`; the other named source families have no directive file in this snapshot. |
| Batch 006 — `converge-work-family` | The named source roots (`pkg/invocations`, `pkg/materialize`, `pkg/timework`, `pkg/workcontent`, `pkg/workgraph`, and `pkg/workquery`) have no directive file in this snapshot. |
| Batch 006 — `converge-factory-orchestrator-families` | Every detailed directive file under `pkg/factory/sessions/execution/**`; the other explicitly named moved roots have no directive file in this snapshot. Core event-first `pkg/factory/**` files are not reserved by this work item. |
| Batch 006 — `converge-platform-and-interfaces` | Every detailed directive file under `pkg/factory/contracts/**`, `pkg/platform/cursors/**`, `pkg/factory/replay/**`, and `pkg/platform/replay/**`, matching the work item's interfaces, cursor-storage, Factory replay, and replay-infrastructure moves. |
| Batch 007 — `move-session-state-to-factorysessions`, `split-runtime-build-ownership`, and `narrow-factory-service-facade` | Every detailed directive file under `pkg/factory/sessions/execution/**` and `pkg/service/**`. |
| Batch 008 — `retire-legacy-composition-entrypoints` and `delete-host-composition-shims` | Every detailed directive file under `pkg/runtimehost/**` and `pkg/service/**`. Other Batch 008 deletion items have no additional directive file in this snapshot. |

The overlay covers all **177** active occurrences across all **93** handwritten files:
**141** occurrences in **69** externally owned files and **36** occurrences in **24**
files unowned at snapshot. The group rows below apply to every detailed directive row for
every file in the named path set; the two file exceptions add the named PR collision.

| Detailed file set | Files | Directives | Snapshot ownership |
| --- | ---: | ---: | --- |
| `pkg/api/**` | 6 | 12 | **Externally owned:** Batch 006 transport-family convergence |
| `pkg/apisurface/**` | 6 | 8 | **Externally owned:** Batch 006 transport-family convergence |
| `pkg/cli/**` | 12 | 18 | **Externally owned:** Batch 006 transport-family convergence; `pkg/cli/root_run_test.go` also collides with Batch 005 `create-root-process-owner` |
| `pkg/factory/sessions/execution/**` | 11 | 31 | **Externally owned:** Batch 006 factory/orchestrator convergence and Batch 007 session/service convergence |
| `pkg/factory/contracts/**` | 2 | 3 | **Externally owned:** Batch 006 platform/interfaces convergence |
| `pkg/platform/cursors/**` | 5 | 10 | **Externally owned:** Batch 006 platform/interfaces convergence |
| `pkg/transports/mcp/**` | 4 | 9 | **Externally owned:** Batch 006 transport-family convergence |
| `pkg/factory/replay/**` | 5 | 5 | **Externally owned:** Batch 006 platform/interfaces convergence |
| `pkg/runtimehost/**` | 3 | 5 | **Externally owned:** Batch 008 runtime-shim removal; `pkg/runtimehost/runtime_sessions.go` also collides with PR #1062 |
| `pkg/service/**` | 10 | 31 | **Externally owned:** Batch 007 session/service convergence and Batch 008 runtime-shim removal |
| `pkg/workers/**` | 5 | 9 | **Externally owned:** Batch 006 model/worker convergence; `pkg/workers/provider/recording_provider_test.go` also collides with PR #1001 |

Every remaining detailed file is explicitly **unowned at snapshot**:

| File | Package owner | Covered directives | Reason |
| --- | --- | --- | --- |
| `pkg/config/config_mapper.go` | `pkg/config` | file size + file lines | F |
| `pkg/config/factory_config_mapping.go` | `pkg/config` | file size + file lines | F |
| `pkg/config/factory_config_mapping_internal.go` | `pkg/config` | file size + file lines | F |
| `pkg/config/layout.go` | `pkg/config` | file size + file lines | F |
| `pkg/config/openapitests/openapi_factory_test.go` | `pkg/config/openapitests` | cyclomatic complexity | T |
| `pkg/factory/events/event_history.go` | `pkg/factory/events` | cyclomatic complexity | R |
| `pkg/factory/events/event_history_dispatch_lifecycle.go` | `pkg/factory/events` | cyclomatic complexity | R |
| `pkg/factory/events/event_history_lineage_test.go` | `pkg/factory/events` | cyclomatic complexity | T |
| `pkg/factory/ingest/filewatcher_test.go` | `pkg/factory/ingest` | cyclomatic complexity | T |
| `pkg/factory/projections/world_state.go` | `pkg/factory/projections` | file size + file lines + cyclomatic complexity | F, R |
| `pkg/factory/projections/world_state_dispatch.go` | `pkg/factory/projections` | cyclomatic complexity | R |
| `pkg/factory/projections/projectiontests/dispatch_lifecycle_event_replay_test.go` | `pkg/factory/projections/projectiontests` | cyclomatic complexity | T |
| `pkg/factory/projections/projectiontests/simple_dashboard_projection_test.go` | `pkg/factory/projections/projectiontests` | 2 cyclomatic-complexity directives | T |
| `pkg/factory/projections/projectiontests/world_state_support_test.go` | `pkg/factory/projections/projectiontests` | 2 cyclomatic-complexity directives | T |
| `pkg/factory/projections/projectiontests/world_state_test.go` | `pkg/factory/projections/projectiontests` | cyclomatic complexity | T |
| `pkg/factory/requests/work_request_submit_test.go` | `pkg/factory/requests` | cyclomatic complexity | T |
| `pkg/factory/requests/work_request_test.go` | `pkg/factory/requests` | cyclomatic complexity | T |
| `pkg/factory/runtime/factory_event_history_test.go` | `pkg/factory/runtime` | 3 cyclomatic-complexity directives | T |
| `pkg/factory/runtime/factory_modes_test.go` | `pkg/factory/runtime` | 2 cyclomatic-complexity directives | T |
| `pkg/factory/runtime/factory_runtime_test_helpers_test.go` | `pkg/factory/runtime` | cyclomatic complexity | T |
| `pkg/factory/subsystems/history_transitioner_pipeline_test.go` | `pkg/factory/subsystems` | cyclomatic complexity | T |
| `pkg/factory/subsystems/dispatchertests/dispatcher_test.go` | `pkg/factory/subsystems/dispatchertests` | cyclomatic complexity | T |
| `pkg/factory/validation/validation_test.go` | `pkg/factory/validation` | file size + file lines | T |
| `tests/functional/runtime_api/factory_transformation/api_current_factory_put_layout_test.go` | `tests/functional/runtime_api/factory_transformation` | function size | T |

### Open pull-request collision snapshot

The open-PR set was fetched at `2026-07-13T07:18:59Z` UTC. PR conversation inspection
found no PR for this inventory branch at that time. These are the exact heads and file lists
used for collision evaluation; only PRs #1001 and #1062 intersect active directive files,
but all files were compared when selecting the cohort.

| PR | Head revision | Files used for comparison |
| --- | --- | --- |
| #1001 `provider-failure-dashboard-details` | `ad2e805a360c1f6aaef11b0a9087c38cd36174ca` | `pkg/workers/provider/recording_provider_test.go`; `ui/src/api/dashboard/types.ts`; `ui/src/api/factory-sessions/normalize-durable-inspection.test.ts`; `ui/src/features/current-selection/base/messages/shell/current-selection-dispatch-history.ts`; `ui/src/features/current-selection/dispatch-selection/components/dispatch-history/selected-work-dispatch-history-card.tsx`; `ui/src/features/current-selection/dispatch-selection/components/dispatch-history/selected-work-dispatch-history.test.tsx`; `ui/src/features/current-selection/work-selection/components/execution/terminal-work-summary-detail.test.tsx`; `ui/src/features/current-selection/work-selection/components/execution/terminal-work-summary-detail.tsx`; `ui/src/features/current-selection/work-selection/components/inference-attempt/inference-attempt-metadata-details.test.tsx`; `ui/src/features/current-selection/work-selection/components/inference-attempt/inference-attempt-metadata-details.tsx`; `ui/src/features/current-selection/work-selection/components/work-item/work-item-card.stories.tsx`; `ui/src/features/factory-session-detail/components/dispatch-detail/dispatch-detail-content.test.tsx`; `ui/src/features/factory-session-detail/components/dispatch-detail/dispatch-detail-content.tsx`; `ui/src/features/factory-session-detail/components/factory-session-detail-panel.failure.test.tsx`; `ui/src/features/factory-session-detail/components/live-provider-inspection/factory-session-detail-panel.failed-bridged-child-inspection.test.tsx`; `ui/src/features/factory-session-detail/components/stories/factory-session-detail-panel.live-provider-story-definitions.stories.shared.tsx`; `ui/src/features/factory-session-detail/lib/factory-session-detail-panel.story-definitions.stories.shared.tsx`; `ui/src/features/factory-session-detail/messages/factory-session-detail.ts`; `ui/src/features/timeline/state/timeline/cloneTimelineSnapshot.ts`; `ui/src/features/timeline/state/timeline/projectWorkstationRequests.ts`; `ui/src/features/timeline/state/timeline/replayCompletion.ts`; `ui/src/features/timeline/state/timeline/replayWorldState.ts`; `ui/src/features/timeline/state/timeline/replayWorldStateInference.test.ts`; `ui/src/features/timeline/state/timeline/replayWorldStateSupport.ts`; `ui/src/features/timeline/state/timeline/types.ts`; `ui/src/testing/factory-session-live-provider-inspection-fixtures.ts` |
| #1037 `session-repair-exact-checkpoint-identity` | `94066d9ae60a9f609b690076a2a2cd69a61626cb` | `pkg/transports/cli/root.go`; `pkg/transports/cli/run/failure_baseline_quiet_leak_test.go`; `pkg/transports/cli/terminalpolicy/policy.go`; `pkg/transports/cli/terminalpolicy/policy_test.go`; `pkg/workers/process/command_test.go`; `ui/src/features/dashboard/hooks/preflight/use-dashboard-checkpoint-preflight.identity-race.test.tsx`; `ui/src/features/dashboard/hooks/preflight/use-dashboard-checkpoint-preflight.test.tsx`; `ui/src/features/dashboard/hooks/preflight/use-dashboard-checkpoint-preflight.ts`; `ui/src/features/dashboard/hooks/useDashboardSnapshot.test.tsx`; `ui/src/features/dashboard/lib/dashboard-session-lifecycle.test.ts`; `ui/src/features/dashboard/lib/preflight/resolve-dashboard-checkpoint-preflight.test.ts`; `ui/src/features/dashboard/lib/preflight/resolve-dashboard-checkpoint-preflight.ts`; `ui/src/features/timeline/lib/stream-derived-cache-identity.test.ts`; `ui/src/features/timeline/lib/stream-derived-cache-identity.ts`; `ui/src/features/timeline/public/index.ts`; `ui/src/features/timeline/state/checkpoint-persistence/deletePersistedTimelineCheckpoint.test.ts`; `ui/src/features/timeline/state/checkpoint-persistence/deletePersistedTimelineCheckpoint.ts`; `ui/src/features/timeline/state/checkpoint-persistence/timelineCheckpointPersistence.multi-session.test.ts`; `ui/src/features/timeline/state/checkpoint-persistence/timelineCheckpointPersistence.test.ts`; `ui/src/features/timeline/state/timelineCheckpointPersistence.ts` |
| #1040 `you-goal-b07-stream-program-gate` | `41ad64d6d527d7bb6c09131f94e71c879318f9f3` | `docs/internal/development/plans/you-goal/api-cli-response-stream-parity.md`; `docs/internal/development/plans/you-goal/goal-response-stream-integration.md`; `docs/internal/development/plans/you-goal/stream-responses-final-audit.md`; `docs/internal/development/plans/you-goal/subagent-response-stream-integration.md`; `docs/internal/processes/api-relevant-files.md`; `docs/internal/processes/invocation-relevant-files.md`; `pkg/transports/cli/root.go`; `pkg/packagedfactories/subagent/materialize_test.go`; `pkg/workers/service/hosted_poller_test.go`; `tests/functional/smoke/cli_named_goal_response_stream_smoke_test.go`; `tests/functional/smoke/cli_named_goal_routing_smoke_test.go`; `tests/functional/smoke/cli_named_response_stream_api_parity_smoke_test.go`; `tests/functional/smoke/cli_named_subagent_response_stream_smoke_test.go` |
| #1062 `fix-sessions` | `76db760b53e0cb0a15b5da8485d4a74f851eb93d` | `factory/workstations/review/AGENTS.md`; `pkg/runtimehost/model_catalog_test.go`; `pkg/runtimehost/runtime_sessions.go` |
| #1064 `stream-b03-sse-contract` | `be3d3136a5135be080597e488184b054a082a840` | `api/codegen_config/client.yaml`; `api/codegen_config/server.yaml`; `api/components/parameters/ResponseEventAfterSequence.yaml`; `api/components/parameters/ResponseEventDispatchID.yaml`; `api/components/parameters/ResponseEventKind.yaml`; `api/components/responses/ResponseEventBadRequest.yaml`; `api/components/responses/ResponseEventSessionNotFound.yaml`; `api/components/responses/ResponseEventStreamExpired.yaml`; `api/components/schemas/api/ErrorFamily.yaml`; `api/components/schemas/api/ErrorResponse.yaml`; `api/openapi-main.yaml`; `api/openapi.yaml`; `contracts/testdata/baseline/rest-operations.json`; `docs/internal/processes/api-relevant-files.md`; `pkg/api/contracttests/generated_contract_common_test.go`; `pkg/api/contracttests/openapi_contract_authoring_test.go`; `pkg/api/contracttests/openapi_contract_response_events_test.go`; `pkg/api/contracttests/openapi_contract_surface_test.go`; `pkg/api/generated/server.gen.go`; `pkg/config/openapitests/parity_inventory_test.go`; `pkg/generatedclient/client.gen.go`; `ui/src/api/generated/openapi.ts` |
| #1066 `stream-b03-provider-adapter-kernel` | `f680f5737f131888137969574b2721f462f40b67` | `pkg/factory/sessions/responseevents/draft.go`; `pkg/factory/sessions/responseevents/draft_test.go`; `pkg/workers/provider/adapter/contract.go`; `pkg/workers/provider/adapter/contract_test.go`; `pkg/workers/provider/adapter/orchestration.go`; `pkg/workers/provider/adapter/orchestration_test.go`; `pkg/workers/provider/adapter/registry.go`; `pkg/workers/provider/adapter/registry_test.go`; `pkg/workers/provider/adapter/testkit/conformance.go`; `pkg/workers/provider/adapter/testkit/final_only_conformance.go`; `pkg/workers/provider/adapter/testkit/final_only_test.go`; `pkg/workers/provider/adapter/testkit/full_stream_test.go` |

Active website-session work was also excluded from cohort selection. PR #1037's exact set
is in the open-PR table; the other active sets were:

| Active website-session head | Exact files used for comparison |
| --- | --- |
| `session-repair-explicit-hydration-state` at `2ca9f77dfb7f76c574cd19087f4fd6a2025bfcc7` | `ui/src/App.session-stream.test.tsx`; `ui/src/features/dashboard/hooks/useDashboardSnapshot.test.tsx`; `ui/src/features/dashboard/hooks/useDashboardSnapshot.ts`; `ui/src/features/dashboard/hooks/useDashboardWorldView.test.tsx`; `ui/src/features/dashboard/hooks/useDashboardWorldView.ts`; `ui/src/features/dashboard/lib/dashboard-world-view.test.ts`; `ui/src/features/dashboard/lib/dashboard-world-view.ts`; `ui/src/features/dashboard/lib/synchronization/dashboard-synchronization-state.test.ts`; `ui/src/features/dashboard/lib/synchronization/dashboard-synchronization-state.ts` |
| `session-repair-work-outcome-materializer` at `3eb9f246691014147dd7d28cc760f84ca915c587` | `ui/src/features/work-outcome/lib/materialized-work-outcome.test.ts`; `ui/src/features/work-outcome/lib/materialized-work-outcome.ts` |

None of the active website-session files intersects the selected cohort.

## Ranked first burn-down cohort

This cohort is a later implementation assignment, not a change made by this inventory.
All five files were unowned at the ownership snapshot, are handwritten test files, and are
outside `cmd/factory`, root/wire composition, Factory Session packages and adapters,
response-stream packages and adapters, active website-session work, and every open-PR file
set above. They avoid the event-history and world-state projection files whose replay and
projection responsibilities carry wider consolidation risk.

| Rank | File and owner | Covered directive/target | Low-collision rationale | Required removal evidence and focused preservation check |
| ---: | --- | --- | --- | --- |
| 1 | `pkg/factory/requests/work_request_submit_test.go` — `pkg/factory/requests` | cyclomatic complexity on `TestWorkRequestFromSubmitRequests_PreservesCanonicalBatchContract` | One test-only batch-normalization scenario in a narrow domain package; no active ownership collision. | Split table setup/assertions while preserving canonical batch request, relation, and trace assertions; run `go test ./pkg/factory/requests` and both quality checkers. |
| 2 | `pkg/factory/requests/work_request_test.go` — `pkg/factory/requests` | cyclomatic complexity on `TestNormalizeWorkRequest_IndependentWorkItemsShareRequestAndTrace` | Same narrow package and test-fixture class as rank 1, permitting one bounded review without touching runtime composition. | Split independent-work-item fixtures/assertions with equivalent request/trace identity coverage; run `go test ./pkg/factory/requests` and both quality checkers. |
| 3 | `pkg/factory/ingest/filewatcher_test.go` — `pkg/factory/ingest` | cyclomatic complexity on `TestFileWatcher_JSONFactoryRequestBatchAcceptsParentChildByWorkName` | Isolated ingest behavior test; no composition, session, stream, website, batch, or PR collision. | Extract fixture and assertion helpers while preserving observable parent/child-by-work-name ingestion; run the named test in `go test ./pkg/factory/ingest` and both quality checkers. |
| 4 | `pkg/factory/subsystems/dispatchertests/dispatcher_test.go` — `pkg/factory/subsystems/dispatchertests` | cyclomatic complexity on `assertSingleTransitionDispatchResult` | Test-only assertion helper in an existing dispatcher test package; production dispatch logic remains untouched. | Split the assertion by observable dispatch/result facets with equivalent callers and outcomes; run `go test ./pkg/factory/subsystems/dispatchertests` and both quality checkers. |
| 5 | `pkg/factory/validation/validation_test.go` — `pkg/factory/validation` | file-size and file-line directives | Test-only file in the validation owner; ranked last because removing two file-level directives requires a larger scenario split than the four function cases above. | Split scenarios by validation responsibility while preserving all acceptance/rejection assertions; run `go test ./pkg/factory/validation` and both quality checkers. |

A dispatching maintainer must refresh the batch worktrees, website-session worktrees, and
open-PR file lists immediately before starting this cohort. Any new collision makes the
affected file externally owned and requires selecting the next ranked unowned candidate;
it does not authorize duplicate cleanup.

## Non-handwritten and checker-owned matches

### Generated, vendored, and fixture-data exclusions

The focused scan found **0** directive matches in excluded generated, vendored, or
`testdata` source; this revision has no `vendor/` directory. These are not handwritten debt. The checker policy in
`internal/backendsizecheck/policy.go` excludes `pkg/transports/http/generated`, `pkg/transports/http/client`,
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
