# c06 runtime API root characterization

## Scope and verdict

This is the story-001 characterization for
`functional-test-optimization-c06-runtime-api-root-001` at the pre-migration
base commit. It is an additive evidence artifact; no test, production,
contract, generated, shared-support, baseline, or `factory_transformation`
file was changed.

The root package has 26 executable top-level identities across its default and
`functionallong` builds: 25 default identities plus one tagged topology
identity. The default package run passed. The case map below covers every
planned `CASE-01` through `CASE-44` row, records the existing witness, and
separates the candidate shared cohort from process-scoped and no-process work.

Story-001 characterization verdict: **PASS**.

This artifact does not claim post-migration parity, one-process topology,
repeat/race stability, or new-fixture cleanup. Those properties remain owned by
stories 002–004 and the gates named below.

## Evidence identity

| Field | Value |
| --- | --- |
| Story | `functional-test-optimization-c06-runtime-api-root-001` |
| Repository | `github.com/portpowered/infinite-you` |
| Base commit | `67710223e327d02c0de93a6ad826c754fe5c1702` |
| Branch | `functional-test-optimization-c06-runtime-api-root` |
| Captured | `2026-08-28T01:00:00-07:00` |
| Go | `go1.25.0` |
| Host | Windows `10.0.26200`, `windows/amd64` |
| Dependency fidelity | Local-real production `root.BuildProcess`/`Process.Execute` and in-process HTTP, with controlled external effects |
| Remote/paid effects | None; zero remote calls and `$0` paid cost |

The PRD’s `sourcePlan` path (`docs/temp/functional-test-optimization.md`) and
the task-plan path named in `prd.md` are not present in this checkout. The
checked-in `prd.md` contains the same c06 scope, matrix, and task packet and was
used as the local plan authority. This did not block characterization.

## Commands and observed results

| Command | Exit | Observed result | Property proved / limitation |
| --- | ---: | --- | --- |
| `go test -list '^Test' ./tests/functional/runtime_api` | 0 | 25 default identities; package reported `0.057s` | Default executable inventory compiles and lists successfully. `go test -list` does not expand runtime `t.Run` subcases, so those are mapped by direct source inspection below. |
| `go test -tags=functionallong -list '^Test' ./tests/functional/runtime_api` | 0 | 26 identities; the union adds `TestEndToEndTopologyProjectionSmoke_LiveEventsAndReplayConfigMatch`; package reported `0.047s` | Tagged executable inventory is present and compiles. |
| `go test -count=1 -timeout=15m ./tests/functional/runtime_api` | 0 | `ok github.com/portpowered/infinite-you/tests/functional/runtime_api 32.329s` | Complete pre-change default root-package behavior passed through the current production-composed local HTTP fixture and controlled effects. |
| Prior source-plan diagnostic retained by the PRD | n/a | `go test -count=1 -v -timeout=15m ./tests/functional/runtime_api` passed in `32.808s` on the same dated saturated host; source-plan CI observations were approximately `10.84–10.93s` | Diagnostic prioritization only; no local duration threshold or quiet-host claim. |

The package run was executed without `-short`; existing tests that call
`SkipLongFunctional` therefore ran in the default build. No browser check is
required for this documentation-only characterization story.

## Executable identity inventory

The following is the exact output identity set from the two listing commands.
`default` means it appears in the untagged listing; `functionallong` means it
requires the `functionallong` build tag.

| # | Build | Identity | Source |
| ---: | --- | --- | --- |
| 1 | default | `TestCleanupSmoke_BackendDashboardAndCanonicalEventsExposeOnlyCleanedFactorySurfaces` | `tests/functional/runtime_api/api_cleanup_smoke_test.go:15` |
| 2 | default | `TestAPIEventReplaySmoke_PublicEventsAndSessionProjectionExposeActiveAndCompletedTimeline` | `tests/functional/runtime_api/api_event_replay_smoke_test.go:15` |
| 3 | default | `TestFunctionalServerOverrideCompatibilityRegression_MockWorkersAndProviderOverride` | `tests/functional/runtime_api/api_functional_server_override_regression_test.go:25` |
| 4 | default | `TestInferenceEvents_ModelProviderAttemptsRecordInCanonicalHistoryAndArtifact` | `tests/functional/runtime_api/api_inference_events_test.go:18` |
| 5 | default | `TestNamedJavaScriptFactoryRunResolvesInvocationInputThroughCLI` | `tests/functional/runtime_api/api_javascript_sync_structured_input_test.go:26` |
| 6 | default | `TestJavaScriptSyncExecutionResolvesStructuredInvocationInput` | `tests/functional/runtime_api/api_javascript_sync_structured_input_test.go:102` |
| 7 | default | `TestModelTransportSmoke_PullUsesConfiguredLegacyCacheWithoutNetwork` | `tests/functional/runtime_api/api_model_transport_smoke_test.go:25` |
| 8 | default | `TestModelTransportSmoke_ServiceModeStartupAndDirectModelRoutesStayAligned` | `tests/functional/runtime_api/api_model_transport_smoke_test.go:83` |
| 9 | default | `TestSubmitMultipleRuntimeWorkItemsCompletes` | `tests/functional/runtime_api/api_multi_work_dispatch_smoke_test.go:13` |
| 10 | default | `TestProviderErrorSmoke_ThrottleFailureIsolatesOtherLaneThroughPublicSession` | `tests/functional/runtime_api/api_provider_throttle_pause_observability_test.go:19` |
| 11 | default | `TestRuntimeConfigAlignmentSmoke_CanonicalOnlyBoundaryStaysAlignedAcrossExecutionAndRejectsRetiredAliases` | `tests/functional/runtime_api/api_runtime_config_alignment_smoke_test.go:28` |
| 12 | default | `TestFunctionalAPIServer_UsesProductionRuntimeFileLoggingDefault` | `tests/functional/runtime_api/api_runtime_log_policy_test.go:25` |
| 13 | default | `TestFunctionalAPIServer_RuntimeLogDirectoryIsAProcessInput` | `tests/functional/runtime_api/api_runtime_log_policy_test.go:53` |
| 14 | default | `TestServiceConfigOverrideAlignment_FunctionalHTTPServerProviderCommandRunner` | `tests/functional/runtime_api/api_service_config_override_alignment_test.go:13` |
| 15 | default | `TestSessionInvocationAPI_AcceptsStructuredArgsWithActiveSignature` | `tests/functional/runtime_api/api_session_invocation_test.go:20` |
| 16 | default | `TestAPIUnifiedEventLogSmoke_LiveRecordReplayProjectionAndDivergenceUseSameTimeline` | `tests/functional/runtime_api/api_unified_event_log_smoke_test.go:18` |
| 17 | default | `TestWorkRootPolicySlicesRejectUnsupportedOperations` | `tests/functional/runtime_api/api_work_root_policy_slices_test.go:16` |
| 18 | default | `TestWorkRootPolicyServiceResolvePrimaryResultHonorsCanceledContext` | `tests/functional/runtime_api/api_work_root_policy_slices_test.go:142` |
| 19 | default | `TestWorkRootPolicyServicePrepareInvocationInputRejectsWhitespaceOnlyText` | `tests/functional/runtime_api/api_work_root_policy_slices_test.go:157` |
| 20 | default | `TestWorkRootPolicyServicePrepareInvocationInputAcceptsDirectArgs` | `tests/functional/runtime_api/api_work_root_policy_slices_test.go:171` |
| 21 | default | `TestWorkRootPolicyServiceResolvePrimaryResultSubmittedTerminalSuccess` | `tests/functional/runtime_api/api_work_root_policy_slices_test.go:195` |
| 22 | default | `TestWorkServiceApplicationSlicesExerciseFunctionalLane` | `tests/functional/runtime_api/api_work_service_application_slices_test.go:16` |
| 23 | default | `TestDashboard_EngineStateSnapshot_EndToEnd` | `tests/functional/runtime_api/dashboard_engine_state_test.go:18` |
| 24 | default | `TestServiceModeSmoke_EmptyStartupIdleSubmissionAndPostCompletionIdleStayReachableUntilCanceled` | `tests/functional/runtime_api/api_service_mode_observability_smoke_test.go:15` |
| 25 | default | `TestObservabilitySmoke_PublicStatusSessionWorkAndEventsAlignAcrossRuntimeTransitions` | `tests/functional/runtime_api/api_service_mode_observability_smoke_test.go:46` |
| 26 | functionallong | `TestEndToEndTopologyProjectionSmoke_LiveEventsAndReplayConfigMatch` | `tests/functional/runtime_api/topology_projection_smoke_long_test.go:16` |

Support-only root files (`doc.go`, `events_test.go`, `external_support_test.go`,
`functional_server_test.go`, `helpers_test.go`, `runtime_support*.go`,
`short_helpers_test.go`, and the generated smoke helpers) contribute helpers or
package documentation but no additional top-level `Test` identity in either
listing.

## Current process, session, stream, and edge topology

The root helper path is observable in the existing source:

1. `startFunctionalServer*` and `StartFunctionalServer*` build a
   `support.FunctionalAPIServerConfig` and delegate to
   `support.StartFunctionalAPIServer`.
2. `support.StartFunctionalAPIServer` calls
   `BuildProcessWithRecordingReader`, injects `ProcessAPIServer.Start`, starts
   one asynchronous `Process.Execute`, waits for the real in-process HTTP
   listener, and registers `Stop`/`Close` cleanup.
3. Direct process cases call `support.BuildProcess(...).Execute(...)`; direct
   recording/replay cases call `support.StartFunctionalAPIServer` twice.

Direct source inspection gives 21 process/listener starts in one default
package execution: the server-override test has two subtest starts and the
record/replay test has a live plus replay start. The tagged topology test adds
two more starts (live record plus replay). This is a body-derived baseline,
not a new runtime counter; the exact call sites and process-scoped reasons are
in the tables below.

The current HTTP cohort mostly addresses the implicit `~default` Factory
Session through `support.DefaultSession...` helpers. Root-level tests do not
currently open unique explicit Factory Sessions for the candidate cohort. The
event/replay, cleanup, unified-log, and topology tests own HTTP SSE readers;
their public event assertions observe prelude/history, live order, canonical
event types, correlation, and replay identity. Controlled external effects are
provided by `edges.Edges` through provider overrides, provider/script command
runners, model asset HTTP, and process environment inputs. The package does not
instrument a shared fixture, per-scenario lane reset, or resource ledger yet.

## Top-level body and witness map

| Identity | Existing body / witness | Current topology decision |
| --- | --- | --- |
| `TestCleanupSmoke_BackendDashboardAndCanonicalEventsExposeOnlyCleanedFactorySurfaces` | Submits one Work through HTTP; asserts terminal Work, `/status`, canonical Work Request/Dispatch Request/Dispatch Response events, event-stream prelude, embedded dashboard shell, and route fallback. | `shareable-with-controlled-edge`; candidate `CASE-01`; currently one default-session server. |
| `TestAPIEventReplaySmoke_PublicEventsAndSessionProjectionExposeActiveAndCompletedTimeline` | Blocks a provider, reads the SSE prelude and live Work/dispatch events, releases dispatch, then checks active/completed Factory Session projections and one trace Work. | `shareable-with-controlled-edge`; candidate `CASE-02`; current default-session stream. |
| `TestFunctionalServerOverrideCompatibilityRegression_MockWorkersAndProviderOverride` | `StartFunctionalServerMockWorkersCompletes` proves mock-worker compatibility; `ProviderOverrideIsAppliedBeforeServiceBuildForHTTPRuntime` proves a shaped Codex runner is installed before the HTTP runtime build and is called twice. | `isolated-with-reason`; `CASE-03`/`CASE-04`; process-scoped mock-worker and pre-build edge behavior. |
| `TestInferenceEvents_ModelProviderAttemptsRecordInCanonicalHistoryAndArtifact` | Uses a two-response controlled provider, `--record`, canonical inference request/response order and correlation assertions, then stops the process and compares live inference events with the replay artifact. | `isolated-with-reason`; `CASE-05`; recording finalization is process-scoped. |
| `TestNamedJavaScriptFactoryRunResolvesInvocationInputThroughCLI` | Builds a root process directly and executes the public JSON CLI input against a named JavaScript Factory; checks exact primary text and zero provider calls. | `isolated-with-reason`; `CASE-06`; direct `Process.Execute`, named-factory HOME, working directory, and CLI/mock-worker inputs. |
| `TestJavaScriptSyncExecutionResolvesStructuredInvocationInput` | Starts an HTTP process with explicit HOME/USERPROFILE, workflow-home edge, named Factory, and mock-worker mode; checks sync `FINAL` result, exact primary text, and zero provider calls. | `isolated-with-reason`; `CASE-07`; process environment and workflow-home inputs. |
| `TestModelTransportSmoke_PullUsesConfiguredLegacyCacheWithoutNetwork` | Creates managed-cache metadata/assets, injects the cache environment and rejecting model HTTP edge, then checks `ALREADY_PRESENT`, revision/files, and zero upstream calls. | `isolated-with-reason`; `CASE-08`; environment and dependency-edge property. |
| `TestModelTransportSmoke_ServiceModeStartupAndDirectModelRoutesStayAligned` | Uses a controlled model provider to assert status, model list/detail/readiness/capability, TTS binding/audio metadata, one provider call, and unsupported-operation HTTP 400. | `shareable-with-controlled-edge`; `CASE-17`/`CASE-18`; provider and audio-file effects are controlled. |
| `TestSubmitMultipleRuntimeWorkItemsCompletes` | Submits two distinct Work IDs/traces before completion and checks both complete and the public list contains exactly two results. | `shareable-with-controlled-edge`; `CASE-09`; static provider command runner. |
| `TestProviderErrorSmoke_ThrottleFailureIsolatesOtherLaneThroughPublicSession` | Uses three deterministic Claude failures and one Codex success; checks throttled/healthy Work locations, in-flight zero, dispatch outcomes, and exact command order. | `shareable-with-controlled-edge`; `CASE-10`; pause harness and command runner. |
| `TestRuntimeConfigAlignmentSmoke_CanonicalOnlyBoundaryStaysAlignedAcrossExecutionAndRejectsRetiredAliases` | One named subtest performs canonical flatten/readback plus timeout/requeue/resource/call/event/topology execution; five named subtests load retired generated/frontmatter aliases and assert precise errors without starting runtime. | `shareable-with-controlled-edge` for execution (`CASE-11`); `no-process` for alias rejection (`CASE-12`–`CASE-16`); `isolated-with-reason` for the mixed canonical round-trip disposition (`CASE-40`) until split/cleanup ownership is handled. |
| `TestFunctionalAPIServer_UsesProductionRuntimeFileLoggingDefault` | Starts with `--runtime-log-dir`, checks one production log, runtime readiness, and whitespace Factory Session selector HTTP 404. | `isolated-with-reason`; `CASE-19`; runtime-log process input. |
| `TestFunctionalAPIServer_RuntimeLogDirectoryIsAProcessInput` | Starts with a chosen runtime log directory and checks path/root/appender fields in the flushed structured startup record. | `isolated-with-reason`; `CASE-20`; exact process input and file output. |
| `TestServiceConfigOverrideAlignment_FunctionalHTTPServerProviderCommandRunner` | Installs a shaped provider command runner before the server/process build, then checks terminal count and exactly two provider calls. | `isolated-with-reason`; `CASE-21`; composition-before-build property. |
| `TestSessionInvocationAPI_AcceptsStructuredArgsWithActiveSignature` | Posts structured args to the session invocation endpoint and checks `COMPLETED`, one text part, and exact controlled output. | `shareable-with-controlled-edge`; `CASE-39`; static provider command runner. |
| `TestAPIUnifiedEventLogSmoke_LiveRecordReplayProjectionAndDivergenceUseSameTimeline` | Upserts a stable batch, observes live SSE/canonical projections, closes the recording process, compares live/artifact IDs/order/correlation, then starts a replay server and waits for terminal status. | `isolated-with-reason`; `CASE-22`; recording finalization and replay-server body. Existing event order/count assertions also inform `CASE-44`. |
| `TestWorkRootPolicySlicesRejectUnsupportedOperations` | Ten named `t.Run` rows call unsupported policy/materialization/admission methods and check exact error substrings. | `no-process`; `CASE-23`–`CASE-30`; direct service slices and controlled content staging only. |
| `TestWorkRootPolicyServiceResolvePrimaryResultHonorsCanceledContext` | Cancels a context before primary-result resolution and asserts `context.Canceled`. | `no-process`; `CASE-31`. |
| `TestWorkRootPolicyServicePrepareInvocationInputRejectsWhitespaceOnlyText` | Prepares whitespace-only compatibility text and asserts `ErrInvalidInvocationInput`. | `no-process`; `CASE-32`. |
| `TestWorkRootPolicyServicePrepareInvocationInputAcceptsDirectArgs` | Normalizes an active-signature direct argument and asserts `structured-draft`. | `no-process`; `CASE-33`. |
| `TestWorkRootPolicyServiceResolvePrimaryResultSubmittedTerminalSuccess` | Resolves a terminal Work item from an in-memory invocation world and asserts exact terminal text. | `no-process`; `CASE-34`. |
| `TestWorkServiceApplicationSlicesExerciseFunctionalLane` | Constructs the Work runtime service directly with a controlled runtime/staging edge and checks request preparation, submission, list, primary result, staging, materialization, and cleanup. | `no-process`; `CASE-35`; no root process/listener. |
| `TestDashboard_EngineStateSnapshot_EndToEnd` | Uses a controlled provider to produce success and permanent failure, checks public Work states and provider-session data in canonical events. | `shareable-with-controlled-edge`; `CASE-36`; dashboard/world-view HTTP and provider edge. |
| `TestServiceModeSmoke_EmptyStartupIdleSubmissionAndPostCompletionIdleStayReachableUntilCanceled` | Observes empty idle, active pending dispatch, completed idle, listener reachability after completion, then explicit cancellation and `Done`. | `isolated-with-reason`; `CASE-37`; body proves process/server lifecycle and cancellation. |
| `TestObservabilitySmoke_PublicStatusSessionWorkAndEventsAlignAcrossRuntimeTransitions` | Observes public status, Factory Session, Work, and pending/completed events across idle→active→idle, then verifies the server remains alive until cancellation. | `shareable-with-controlled-edge`; `CASE-38`; blocked provider edge, with cancellation behavior retained as a later cleanup concern. |
| `TestEndToEndTopologyProjectionSmoke_LiveEventsAndReplayConfigMatch` | Tagged long test records a topology with routes/resources, closes the live process, starts a replay server, and compares public Factory/Event topology and replay config. | `isolated-with-reason`; `CASE-41`; live recording close and second replay server. |

## CASE-01 through CASE-44 witness and classification map

The classification vocabulary is the task-packet vocabulary:

- `shareable`: no external effect lane is required (none of the HTTP cohort is
  in this class before migration).
- `shareable-with-controlled-edge`: eligible for the story-002 shared process
  after explicit-session routing and per-scenario edge reset.
- `isolated-with-reason`: retain one process per case because the body proves a
  process input, lifecycle, recording-finalization, replay, CLI, or server
  override property.
- `no-process`: direct service/config logic or a cross-cutting scope invariant;
  no root process/listener is part of the witness.

| Case | Existing witness and source | Classification | Current evidence / remaining boundary |
| --- | --- | --- | --- |
| `CASE-01` | Cleanup smoke body and helpers, `api_cleanup_smoke_test.go:15-44` | `shareable-with-controlled-edge` | HTTP Work/status, canonical events, SSE prelude, dashboard shell/fallback pass in the package run. Current witness uses `~default`; explicit session and shared-fixture proof -> story 002. |
| `CASE-02` | Event replay body, `api_event_replay_smoke_test.go:15-53`, `:63-141` | `shareable-with-controlled-edge` | Blocked dispatch, historical prelude, ordered live events, active/completed projections, and one trace Work are asserted. Shared-process/session isolation -> story 002. |
| `CASE-03` | Override subtest `StartFunctionalServerMockWorkersCompletes`, `api_functional_server_override_regression_test.go:27-41` | `isolated-with-reason` | Mock-worker compatibility is a process startup override; terminal=1 and failed=0 pass. Keep isolated until story 003 records the exact override reason. |
| `CASE-04` | Override subtest `ProviderOverrideIsAppliedBeforeServiceBuildForHTTPRuntime`, `api_functional_server_override_regression_test.go:43-79` | `isolated-with-reason` | Provider command runner is configured before process/service build; non-empty trace, no failure, two Codex calls pass. |
| `CASE-05` | Inference event/artifact test, `api_inference_events_test.go:18-53` | `isolated-with-reason` | Canonical inference order/correlation and live-to-artifact event identity pass after `Stop`; recording finalization must remain isolated. |
| `CASE-06` | Named JavaScript CLI test, `api_javascript_sync_structured_input_test.go:26-75` | `isolated-with-reason` | Direct `Process.Execute`, public CLI JSON, exact primary result, and zero provider calls pass; CLI/process inputs are not shared. |
| `CASE-07` | JavaScript sync test, `api_javascript_sync_structured_input_test.go:102-148` | `isolated-with-reason` | Sync HTTP result, exact primary text, workflow-home edge, and zero calls pass; HOME/USERPROFILE/workflow state is process-scoped. |
| `CASE-08` | Model pull test, `api_model_transport_smoke_test.go:25-80` | `isolated-with-reason` | Managed cache hit metadata and zero rejecting-edge calls pass; cache environment and model HTTP edge remain isolated. |
| `CASE-09` | Multiple Work test, `api_multi_work_dispatch_smoke_test.go:13-50` | `shareable-with-controlled-edge` | Two distinct IDs/traces complete and the list has exactly two items; explicit-session routing and lane reset -> story 002. |
| `CASE-10` | Throttle test, `api_provider_throttle_pause_observability_test.go:19-88`, setup `:97-160` | `shareable-with-controlled-edge` | Three Claude failures, healthy Codex lane, in-flight zero, outcomes, and command order pass; cross-scenario runner reset -> story 002/003. |
| `CASE-11` | Canonical runtime-config execution subtest, `api_runtime_config_alignment_smoke_test.go:36-38`, execution `:65-71` and `:164-260` | `shareable-with-controlled-edge` | Canonical config, stop words, timeout/requeue, resource restoration, two provider/two script calls, Work/events/topology pass in the default run; explicit-session shared lane -> story 002. |
| `CASE-12` | Generated worker-provider alias subtest, `api_runtime_config_alignment_smoke_test.go:40-42`, `:74-78` | `no-process` | `LoadedFactory` rejects `worker.provider` with the existing `executorProvider` message; no runtime starts. |
| `CASE-13` | Generated `resource_usage` alias subtest, `api_runtime_config_alignment_smoke_test.go:44-46`, `:80-85` | `no-process` | `LoadedFactory` rejects the retired workstation alias with the existing `resources` message; no persistence/runtime mutation. |
| `CASE-14` | Split `model_provider` alias subtest, `api_runtime_config_alignment_smoke_test.go:48-50`, `:104-123` | `no-process` | Split worker load rejects the alias with the existing `modelProvider` message. |
| `CASE-15` | Split `runtime_type` alias subtest, `api_runtime_config_alignment_smoke_test.go:52-54`, `:125-135` | `no-process` | Split workstation load rejects the alias with the existing `type` message. |
| `CASE-16` | Cron `trigger_at_start` alias subtest, `api_runtime_config_alignment_smoke_test.go:56-58`, `:137-149` | `no-process` | Split cron workstation load rejects the alias with the existing `triggerAtStart` message. |
| `CASE-17` | Model service/routes test, `api_model_transport_smoke_test.go:83-173` | `shareable-with-controlled-edge` | Status/list/detail/readiness, TTS binding/audio metadata, and exactly one controlled provider call pass; explicit session/model edge routing -> story 002. |
| `CASE-18` | `assertUnsupportedModelInvocationRejected`, called by `api_model_transport_smoke_test.go:173` | `shareable-with-controlled-edge` | Unsupported `EMBED` invocation returns the existing HTTP 400 through the same controlled model server; no successful unsupported operation. |
| `CASE-19` | Runtime logging default test, `api_runtime_log_policy_test.go:25-51` | `isolated-with-reason` | One production log, readiness, and whitespace selector 404 pass; `--runtime-log-dir` is a process input. |
| `CASE-20` | Runtime log process-input test, `api_runtime_log_policy_test.go:53-86` | `isolated-with-reason` | Exact log path/root/appender fields pass; output location is process-scoped and must remain isolated. |
| `CASE-21` | Service override test, `api_service_config_override_alignment_test.go:13-39` | `isolated-with-reason` | Shaped provider runner is installed before build; terminal=1 and two calls pass. The before-build composition property cannot be inferred from a reused process. |
| `CASE-22` | Unified event-log test, `api_unified_event_log_smoke_test.go:18-40`, assertions `:106-452` | `isolated-with-reason` | Live event order/count/correlation, completed projection, artifact identity, and replay-server terminal status pass; recording close/replay requires isolation. |
| `CASE-23` | `policy admission` `t.Run`, `api_work_root_policy_slices_test.go:39-47`, parent `:16-137` | `no-process` | Exact unsupported-admission error passes in a direct Work policy slice. |
| `CASE-24` | `policy content staging` `t.Run`, `api_work_root_policy_slices_test.go:48-56` | `no-process` | Exact unsupported-content-staging error passes in a direct Work policy slice. |
| `CASE-25` | `materialization admission prep` `t.Run`, `api_work_root_policy_slices_test.go:57-65` | `no-process` | Exact unsupported-admission-prep error passes. |
| `CASE-26` | `materialization state access` `t.Run`, `api_work_root_policy_slices_test.go:66-74` | `no-process` | Exact unsupported-state-access error passes. |
| `CASE-27` | `materialization content staging` `t.Run`, `api_work_root_policy_slices_test.go:75-83` | `no-process` | Exact unsupported-content-staging error passes. |
| `CASE-28` | `materialization invocation policy` `t.Run`, `api_work_root_policy_slices_test.go:84-92` | `no-process` | Exact unsupported-invocation-policy error passes. |
| `CASE-29` | `admission materialization` `t.Run`, `api_work_root_policy_slices_test.go:93-101` | `no-process` | Exact unsupported-content-materialization error passes. |
| `CASE-30` | `admission invocation input`, `admission primary result`, and `admission state access` rows, `api_work_root_policy_slices_test.go:102-128` | `no-process` | Exact unsupported invocation-policy/state errors pass for all three admission-owned operations. |
| `CASE-31` | Canceled primary-result test, `api_work_root_policy_slices_test.go:142-152` | `no-process` | `context.Canceled` and no result pass without a root process. |
| `CASE-32` | Whitespace input test, `api_work_root_policy_slices_test.go:157-166` | `no-process` | `ErrInvalidInvocationInput` passes without a root process. |
| `CASE-33` | Direct structured-args test, `api_work_root_policy_slices_test.go:171-190` | `no-process` | Active-signature normalization retains `structured-draft`; no process. |
| `CASE-34` | Terminal primary-result test, `api_work_root_policy_slices_test.go:195-225` | `no-process` | Exact terminal text returns from in-memory state; no process. |
| `CASE-35` | Work application slice, `api_work_service_application_slices_test.go:16-130` | `no-process` | Direct Work service request, list, result, staging, materialization, and cleanup assertions pass with controlled runtime/staging; no HTTP/process boundary. |
| `CASE-36` | Dashboard engine-state test, `dashboard_engine_state_test.go:18-67` | `shareable-with-controlled-edge` | Public complete/failed Work states and provider-session event data pass; controlled provider and session-scoped HTTP conversion -> story 002. |
| `CASE-37` | Service-mode lifecycle test, `api_service_mode_observability_smoke_test.go:15-44` | `isolated-with-reason` | Server remains reachable after completion until explicit cancellation and `Done` closes afterward; cancellation/listener lifecycle is process-scoped. |
| `CASE-38` | Observability transition test, `api_service_mode_observability_smoke_test.go:46-72` | `shareable-with-controlled-edge` | Public status, Factory Session, Work, pending/completed events, idle-active-idle transitions, and listener reachability pass; explicit session/edge reset -> story 002. |
| `CASE-39` | Session invocation test, `api_session_invocation_test.go:20-40` | `shareable-with-controlled-edge` | HTTP invocation returns 200/`COMPLETED` with one exact text part; static provider runner is the controlled edge. |
| `CASE-40` | Canonical round-trip portion of the runtime-config subtest, `api_runtime_config_alignment_smoke_test.go:36-38`, `:65-68` | `isolated-with-reason` | Canonical JSON/frontmatter/resource values are checked before execution, but the current body combines that witness with a process run; retain with the isolated owner until the later split/cleanup work. |
| `CASE-41` | Tagged topology/replay test, `topology_projection_smoke_long_test.go:16-92` | `isolated-with-reason` | Live and replay public Factory/Event topology, routes, worker/model, and capacity match; two process/lifecycle bodies are intentional. |
| `CASE-42` | Cross-cutting source-scope invariant across root HTTP tests; no dedicated current test body | `no-process` | Current local API tests add no authentication header or permission expectation, and the PRD changes no authorization policy. Final clean-room scope confirmation -> `VAL-001`; this row is not claimed as a new executable witness. |
| `CASE-43` | Existing normal cleanup calls in `support.StartFunctionalAPIServer` consumers and explicit `Stop` paths; no dedicated injected-failure test in the root package | `isolated-with-reason` | Current package passes normal cleanup. An injected-failure ledger, listener probe, and zero-leak proof are not present before migration; story 003 owns that bounded enabler and must fail closed. |
| `CASE-44` | Multi-Work IDs (`api_multi_work_dispatch_smoke_test.go:23-50`), event sequence (`api_event_replay_smoke_test.go:103-121`), and unified event counts/order (`api_unified_event_log_smoke_test.go:288-452`) | `shareable-with-controlled-edge` | Existing witnesses cover distinct IDs, monotonic event sequence, and stable event counts/order. Repeated shared-fixture session/edge isolation -> story 002 and `VAL-001`. |

## Observable pre-change contract

The passing package run and direct body inspection establish the following
current witness surface:

| Surface | Existing observation |
| --- | --- |
| HTTP/API | Typed HTTP client/server payloads and direct requests assert Work submission, status, model, invocation, sync, dashboard, Factory Session selector, and error statuses/bodies. |
| Work | Public Work lists, trace/Work IDs, terminal/failed/initial locations, counts, payloads, and primary-result text are asserted. |
| Factory Events | Canonical Work Request, dispatch, inference, response, provider-session, and topology payloads are decoded; event type, order, sequence, correlation, and count assertions are present. |
| Factory Session/projections | Public runtime status, Factory Session state/progress, in-flight count, terminal/processing counts, Work projections, and dashboard/world-view outcomes are asserted. |
| SSE streams | Session Factory Event streams assert retained historical prelude before live events, canonical vocabulary, live order, and recording/replay continuation; readers are test-owned. |
| Structured logs | Runtime log tests observe production JSON startup records and exact path/root/appender fields under a supplied log directory. |
| Configuration | Canonical generated/split configuration is flattened/read and executed; retired aliases fail with exact boundary context/messages. |
| Replay artifacts | Inference/unified/topology tests compare live event identity/order/correlation with artifacts and exercise a second replay server where applicable. |
| Controlled edges | Provider overrides, provider/script command runners, model asset HTTP, environment, workflow-home, and mock-worker compatibility edges are explicitly present in source. |
| Process/cleanup | Current process construction and `Stop`/`Close` ownership are explicit in the helper path; normal package cleanup passed. No pre-migration shared fixture, resource ledger, listener-after-close probe, or injected-failure cleanup witness exists yet. |

## Story-003 implementation evidence

Story `functional-test-optimization-c06-runtime-api-root-003` preserves the
characterized witnesses while making package and scenario cleanup observable.
The eligible cohort still runs through one production-composed
`root.BuildProcess`/`Process.Execute` HTTP process; process-input, lifecycle,
recording-finalization, replay, CLI, and before-build override witnesses retain
their one-process-per-body isolation with an inline `C06-ISOLATED` reason.

### Cleanup and isolation changes

- Shared Factory Sessions are opened through the public API with unique IDs,
  tracked until a successful terminate/status/delete sequence, and released by
  an idempotent lease. Cleanup errors use `t.Errorf` and route cleanup remains
  registered independently, so a failure cannot skip lane reset.
- Shared Factory Event SSE readers register a close hook. The reader close is
  idempotent, reports a bounded shutdown timeout, and releases its ledger entry
  only after the reader goroutine has actually ended.
- Provider, provider-command, and script-command routes use cleanup tokens and
  idempotent unregister functions. Package teardown fails when any route,
  session, or stream remains active or when open/close counts differ.
- Package teardown cancels and joins `Process.Execute`, closes the production
  process once, probes the public listener until a refused request proves it is
  unreachable, verifies one process/listener start for a successful shared
  fixture, and verifies the package-owned root is absent after removal.
- The obsolete exported `FunctionalServer` wrapper was removed; CASE-37 now
  uses the existing private isolated wrapper. No production, shared-support,
  contract, generated, baseline, or transformation surface changed.

### Story-003 verification

| Command/procedure | Exit/result | Property proved | Not proved |
| --- | ---: | --- | --- |
| `go test -run 'TestRuntimeAPIPackage(CleanupLedgerAndEdgeLeasesAreIdempotent\|FixtureCleanupIsIdempotentAndPreservesFailures)$' -count=1 ./tests/functional/runtime_api` | 0; `0.035s` | Idempotent session/stream/edge release; normal listener/root cleanup; injected `Process.Execute` and process-close causes remain discoverable; reachable listeners fail the cleanup probe. | A real clean-room checkout and terminal PR CI. |
| `go test -count=1 -timeout=15m ./tests/functional/runtime_api` | 0; `27.127s` on the committed story-003 head | Full default root-package parity, shared one-process fixture teardown, isolated lifecycle witnesses, public HTTP/Work/Event/projection/stream/log/config/replay assertions. | Clean-room reproduction, PR timing, terminal CI, merge. |
| `go test -count=3 -timeout=45m ./tests/functional/runtime_api` | 0; `76.640s` on the committed story-003 head | Three successive package executions preserve behavior and reset explicit sessions, streams, and controlled edge lanes. | Clean-room reproduction and repository CI timing. |
| `go test -race -count=1 -timeout=20m ./tests/functional/runtime_api` | 0; `68.773s` on Windows `10.0.26200`, `windows/amd64`, Go `1.25.0` | No race-detector finding in the shared fixture, explicit sessions, SSE close hooks, or keyed effect routers. | Other platforms and terminal PR CI. |
| `go test -tags=functionallong -count=1 -timeout=15m ./tests/functional/runtime_api` | 0; `34.700s` on the committed story-003 head | Tagged CASE-41 live recording/replay topology witness retains its isolated two-process lifecycle. | Clean-room reproduction and merge. |
| Scoped three-dot review from `origin/main` | Passed read-only review; changed surfaces are root `tests/functional/runtime_api` files and this c06 artifact, with `factory_transformation/**`, shared support, c01 inventory, baselines, production, contracts, generated files, and unrelated paths absent. | Authorized-scope preservation. | Independent final-head validation, PR timing, terminal CI, merge. |

The local-real production-composed HTTP/package run is the highest feasible
implementation-stage runtime proof. Remote providers and paid calls remain
zero. VAL-001 still owns the fresh-checkout once/repeat/race loopback and the
final CASE-01 through CASE-44 audit; GATE-PR-FUNCTIONAL still owns PR-head
package timing and terminal CI.

## Exclusions and scope proof

The story changed only this artifact. The following remain read-only and are
explicitly outside this characterization edit:

- `tests/functional/runtime_api/factory_transformation/**`
- `tests/functional/internal/support/**`
- `docs/internal/development/functional-test-optimization/c01-eligibility-inventory.*`
- `docs/internal/baselines/**`
- stability-cleanup-owned surfaces
- production packages, contracts, generated artifacts, UI, CI, Makefile, and unrelated files

The scoped three-dot proof for this story is therefore the artifact-only path
`docs/internal/development/functional-test-optimization/c06-runtime-api-root-evidence.md`.
The later implementation stories must rerun the proof with their authorized
root-level test files, explicitly excluding the transformation subpackage.

## Remaining unproven edges and owners

| Edge | Owner |
| --- | --- |
| One package process/listener and explicit unique sessions for eligible cases | `functional-test-optimization-c06-runtime-api-root-002` |
| Per-scenario controlled-edge registration/reset and exact session/stream cleanup | `functional-test-optimization-c06-runtime-api-root-002` / `-003` |
| Isolated lifecycle, process-input, record/replay, and injected-failure cleanup proof | `functional-test-optimization-c06-runtime-api-root-003` |
| Repeat and race stability on the migrated fixture | `functional-test-optimization-c06-runtime-api-root-003` / `VAL-001` |
| Clean-room final-head proof and scope loopback | `functional-test-optimization-c06-runtime-api-root-004` / `VAL-001` |
| PR Backend Functional Coverage timing and terminal CI | `GATE-PR-FUNCTIONAL` under review ownership |
