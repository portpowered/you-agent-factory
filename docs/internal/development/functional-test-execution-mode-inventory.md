# Functional Test Execution Mode Inventory

This inventory classifies the current execution modes under
`libraries/agent-factory/tests/functional_test`. It is the migration map for
standardizing functional tests on the full worker-pool path while preserving
the existing behavioral assertions.

## Classification Rules

- `full worker pool` means the harness uses
  `testutil.WithFullWorkerPoolAndScriptWrap()`.
- `edge mock` means the test fakes the provider, provider command runner,
  command runner, or mock-worker command boundary while keeping normal runtime
  routing.
- `eligible to migrate` means a later story should try the full worker-pool
  option first and preserve the named behavior.
- `already covered` means the file has full-worker-pool coverage for the same
  behavior or intentionally validates a harness compatibility path.
- `exception` means the test currently needs a custom executor, async-only run
  loop, or legacy compatibility seam to observe timing, snapshots, or harness
  contract behavior that an edge mock alone cannot expose.

## Summary

The scan found these notable surfaces:

- Provider edge mocks with full worker pool are already common in workflow,
  config, stop-word, dependency-terminal, lifecycle, and stateless tests.
- Provider command-runner and command-runner edge mocks are already paired with
  full worker pool in provider CLI, script executor, multi-output color
  propagation, record/replay, and service override tests.
- Default harness or mock-worker tests remain in lower-level routing,
  multi-input, multichannel, feedback propagation, and narrow executor-panic
  coverage.
- Async-only or custom-executor seams remain in dashboard snapshot, dispatch
  timing, runtime-state, and trace-history coverage where the test pauses or
  inspects in-flight execution.
- The existing TODOs to migrate are in `dashboard_snapshot_test.go`,
  `dispatch_timing_test.go`, and `runtime_state_test.go`.

## Migration Targets

| File | Current mode | Classification | Behavioral preservation notes |
| --- | --- | --- | --- |
| `archive_terminal_test.go` | full worker pool + provider mock | already full worker pool | Preserve archive routing and terminal place assertions. |
| `batch_ideation_pipeline_test.go` | full worker pool + provider mock | already full worker pool | Preserve concurrency limits, provider call counts, final story/PRD routing, and provider requests loaded from worker AGENTS.md. |
| `cascading_failure_test.go` | full worker pool + provider map mock | already full worker pool | Preserve direct, transitive, and completed-child cascade behavior through provider edge failures. Dependency guards are applied to initial-place input arcs without a structured input-type declaration. |
| `code_review_test.go` | full worker pool + provider mock | already full worker pool | Preserve code-review output and terminal assertions. |
| `cold_start_test.go` | full worker pool + provider mock | already full worker pool | Preserve cold-start submission and completion behavior. |
| `concurrency_limit_test.go` | full worker pool + provider mock for ordinary resource cases; custom panic executors for panic recovery | mixed: full worker pool plus exception | Ordinary capacity, completion, provider failure, and resource-release cases now use full worker-pool provider edge mocks. The inline/async panic cases remain documented exceptions because provider or command-runner failures cannot exercise `WorkerExecutor.Execute` panic recovery while preserving the panic-derived failed-token assertion. |
| `config_driven_test.go` | full worker pool + provider mock | already full worker pool | Preserve prompt rendering, env/config, worktree, retry, timeout, and AGENTS.md behavior. |
| `conflict_resolution_test.go` | full worker pool + provider mock plus one mock worker | mostly full worker pool | Preserve conflict routing and reviewer/resolve transition assertions; review the mock-worker case for edge-boundary migration. |
| `dashboard_engine_state_test.go` | full worker pool + custom blocking executor | exception | Blocks a dispatch to compare engine snapshot and dashboard state while work is active. |
| `dashboard_inflight_test.go` | async + custom blocking executor | exception | Needs a paused in-flight dispatch to assert dashboard active-work reporting. |
| `dashboard_mixed_snapshot_test.go` | test-only subsystem snapshot seam | exception | Captures mixed dispatch/result snapshots between subsystems; not a worker execution-mode shortcut. |
| `dashboard_snapshot_test.go` | async + custom snapshot executors; one TODO | exception | Needs controlled snapshot timing for single and parallel active-work dashboard summaries. |
| `dependency_terminal_test.go` | full worker pool + provider mock | already full worker pool | Preserve dependency terminal routing and failure propagation. |
| `dependency_tracking_test.go` | full worker pool + provider mock | already full worker pool | Preserve blocked-until-satisfied and no-dependency pass-through behavior. |
| `dispatch_timing_test.go` | async + custom sleepy/channel executors; two TODOs | exception | Needs controlled duration and in-flight start-time observation. |
| `dispatcher_lifecycle_test.go` | full worker pool + provider mock | already full worker pool | Preserve batch versus service-mode lifecycle assertions. |
| `dispatcher_workflow_test.go` | full worker pool + provider command-runner mock | already full worker pool | Preserve seed-file routing, per-worker call counts, executor dispatch isolation, per-item review failure routing, and retry exhaustion through provider command requests. |
| `mock_workers_migration_test.go` | service mock-worker execution | already covered | Validates model and script workers use mock-worker command runners through service wiring. |
| `e2e_test.go` | full worker pool + provider mock; functional server smoke | already full worker pool | Preserve end-to-end transition, API, and completion assertions. |
| `executor_context_test.go` | full worker pool + provider mock for dispatch input/lineage; mock worker for feedback | mixed: full worker pool plus exception | Input color and parent lineage assertions now use provider calls through the full worker pool. Rejection feedback remains an exception because provider rejection maps to `OutcomeRejected` without `WorkResult.Feedback`, so the feedback tag cannot be produced through the provider edge. |
| `executor_failure_test.go` | full worker pool + provider/provider-command-runner mocks | already full worker pool | Preserve failure arcs, no-arc failure, and provider failure routing through command-runner exit failures. |
| `failed_immutability_test.go` | full worker pool + provider error mock | already full worker pool | Preserve failed-token immutability, reviewer failure, and duplicate-token prevention. |
| `filewatcher_flow_test.go` | full worker pool + provider/provider-command-runner mocks | already full worker pool | Preserve file watcher submission, trace, terminal place assertions, and no-token-leak coverage. |
| `filewatcher_multichannel_test.go` | full worker pool + provider mock | already full worker pool | Preserve default channel, execution ID, and dynamic execution-directory submission behavior. |
| `full_ideation_pipeline_test.go` | full worker pool + provider mock | already full worker pool | Preserve full ideation route, output content, and provider sequencing. |
| `functional_server_override_regression_test.go` | functional server mock-worker/provider override | already covered | Regression coverage for server construction with mock workers and provider overrides, not a migration target. |
| `functional_server_test.go` | functional server API harness | already covered | Covers server lifecycle and API behavior; worker-mode migration belongs in callers that submit work. |
| `generated_api_smoke_test.go` | functional server API smoke | already covered | API contract smoke, not a worker execution shortcut. |
| `idea_plan_review_execute_with_limits_test.go` | full worker pool + command-runner edge mocks | already full worker pool | Preserve command output, limits, and terminal routing. |
| `idea_to_prd_test.go` | full worker pool + provider/provider-command-runner mocks | already full worker pool | Preserve idea-to-PRD expansion, planner failure routing, and lineage assertions. |
| `init_factory_test.go` | full worker pool + provider mock | already full worker pool | Preserve factory initialization behavior. |
| `integration_smoke_test.go` | full worker pool + provider command-runner/command-runner plus functional server | already full worker pool | Preserve API, provider subprocess, script subprocess, and service smoke assertions. |
| `logical_move_test.go` | custom executors | exception | Captures payload and logical move behavior at the executor boundary. |
| `mock_workers_agent_test.go` | full worker pool + mock-worker command boundary | already full worker pool | Preserve configured model-worker mock outcomes through normal routing. |
| `mock_workers_script_test.go` | full worker pool + mock-worker command boundary | already full worker pool | Preserve configured script-worker mock outcomes through normal routing. |
| `multi_input_guard_test.go` | default harness + custom fanout executors | exception | Uses custom executors that submit child work during execution and intentionally control staggered completion; provider or command-runner edge mocks cannot create the same in-flight child-token topology. |
| `multi_output_color_propagation_test.go` | full worker pool + command-runner edge mocks | already full worker pool | Preserve output-token color/name propagation and N-to-N type matching. |
| `multi_output_test.go` | mostly full worker pool + provider mock; some mock-worker cases | mixed: eligible plus full | Preserve multi-output splitting, stop-word behavior, and terminal token assertions. |
| `multichannel_guard_test.go` | default harness + custom fanout executors | exception | Uses custom executors to submit per-channel child work and inspect guard blocking across dynamic execution directories; edge mocks cannot create the spawned work needed by the assertion. |
| `name_propagation_test.go` | full worker pool + provider mock | already full worker pool | Preserve token name propagation through worker outputs. |
| `ootb_experience_test.go` | functional server/API smoke | already covered | Covers out-of-box server behavior rather than a shortcut worker path. |
| `partial_batch_test.go` | full worker pool + provider/provider-command-runner mocks | already full worker pool | Preserve partial success/failure and provider subprocess behavior. |
| `persistence_test.go` | full worker pool + provider mock | already full worker pool | Preserve persistence and resume assertions. |
| `ralph_loop_test.go` | full worker pool + provider mock | already full worker pool | Preserve execute/review loop routing and provider sequencing. |
| `record_replay_end_to_end_test.go` | full worker pool + provider command-runner mock | already full worker pool | Preserve replay artifact dispatch/completion behavior. |
| `rejection_no_arcs_test.go` | full worker pool + provider mock | already full worker pool | Preserve rejection-to-failure, resource release, rejection-arc routing, and failure record assertions. |
| `repeater_parameterized_test.go` | mixed full worker pool/provider and mock/custom executor cases | mixed: eligible plus exception | Preserve repeater accept/reject/fail routing; keep dispatch capture only if provider calls cannot expose the parameterized input. |
| `replay_regression_harness_test.go` | full worker pool + provider mock | already full worker pool | Preserve replay regression terminal and idle-state assertions. |
| `resource_token_name_test.go` | full worker pool + provider mock | already full worker pool | Preserve resource-gated dispatch token name assertion. |
| `review_retry_exhaustion_test.go` | full worker pool + provider mock for retry/count cases; mock worker for feedback | mixed: full worker pool plus exception | Retry exhaustion and success-before-limit assertions now use provider calls through the full worker pool. Feedback propagation remains an exception because the provider edge cannot set `WorkResult.Feedback`, which is the behavior under test. |
| `runtime_state_test.go` | async/custom executor TODO plus default runtime cases | mixed: eligible plus exception | Migrate ordinary three-stage/failure/same-type cases; keep mid-execution snapshot if it still needs a blocking executor. |
| `script_executor_test.go` | full worker pool + command-runner edge mocks | already full worker pool | Preserve command args, env, working directory, output, failure, and template behavior. |
| `service_config_override_alignment_test.go` | full worker pool + provider command-runner/command-runner mocks | already full worker pool | Preserve service override alignment for model and script workers. |
| `service_harness_test.go` | mixed full worker pool/provider and harness mock/custom contract tests | already covered | Harness-contract tests intentionally validate `MockWorker` and `SetCustomExecutor`; migrate only workflow-behavior cases. |
| `stateless_collector_test.go` | full worker pool + provider mock | already full worker pool | Preserve stateless collector transition and provider prompt assertions. |
| `stateless_execution_test.go` | full worker pool + provider mock plus one dispatch recorder | mixed: eligible plus exception | Preserve runtime config lookup behavior; keep recorder only if provider calls cannot expose raw dispatch fields. |
| `stateless_integration_smoke_test.go` | full worker pool + provider mock | already full worker pool | Preserve config-driven AGENTS.md behavior across copied fixtures. |
| `timeout_cleanup_smoke_test.go` | full worker pool + real command runner | already full worker pool | Preserve subprocess cancellation and timeout cleanup assertions. |
| `trace_history_test.go` | async + custom trace executors | exception | Needs controlled trace-producing executors to assert consumed-token and output-mutation reconstruction. |
| `workflow_modification_test.go` | mostly full worker pool/provider plus mock-worker case | mixed: eligible plus full | Preserve workflow version routing and dynamic modification assertions. |
| `factory_request_batch_test.go` | full worker pool/provider plus custom checker | mixed: eligible plus exception | Preserve canonical request-batch submission; keep checker only if raw dispatch tag/relation inspection is required. |
| `workstation_stopwords_test.go` | full worker pool + provider mock | already full worker pool | Preserve stop-word precedence and routing behavior. |
| `worktree_passthrough_test.go` | full worker pool + provider command-runner mock | already full worker pool | Preserve worktree passthrough and command construction assertions. |

## Explicit TODOs

The source already marks these cases for later migration review:

- `dashboard_snapshot_test.go` parallel work item snapshot case.
- `dispatch_timing_test.go` duration and in-flight start-time cases.
- `runtime_state_test.go` three-stage runtime-state case.

## 2026-04-18 Migration Review

This review looked for existing shortcut/custom-executor tests that can move to
the full async worker-pool path as an implementation detail without changing the
behavior under test. No migrations were implemented during the review.

### Strong candidates

These tests appear migratable by replacing the custom worker executor with an
edge mock while using `testutil.WithFullWorkerPoolAndScriptWrap()`:

| File | Test | Suggested edge seam | Notes |
| --- | --- | --- | --- |
| `dashboard_inflight_test.go` | `TestDashboard_InFlightDispatches` | Blocking `workers.Provider` | Keep the in-flight snapshot assertion, but block inside `Provider.Infer` instead of `WorkerExecutor.Execute`. The scaffolded config needs a real model-worker definition or fixture so the provider path is reached. |
| `dashboard_snapshot_test.go` | `TestDashboard_SingleWorkItemSnapshot` | Snapshot-capturing `workers.Provider` | Capture `GetEngineStateSnapshot` from the provider mock after `WorkstationExecutor` and `AgentExecutor` have been entered. |
| `dashboard_snapshot_test.go` | `TestDashboard_ParallelWorkItemsSnapshot` | Barrier `workers.Provider` | Use a provider barrier to wait for all expected inferences, then assert `InFlightCount` before releasing them. |
| `dispatch_timing_test.go` | `TestDispatchTiming_HistoryRecordsDuration` | Delayed `workers.Provider` or provider command runner | The duration assertion should still hold because dispatch history duration includes the full worker-pool executor path. |
| `dispatch_timing_test.go` | `TestDispatchTiming_InFlightStartTime` | Blocking `workers.Provider` | Preserve the in-flight start-time assertion while avoiding `SetCustomExecutor`. |
| `runtime_state_test.go` | `TestRuntimeState_ThreeStagePipeline` | Delayed `workers.Provider` | The test already wants async dispatch tracking; provider delay should preserve non-zero duration assertions while exercising the full service path. |
| `runtime_state_test.go` | `TestRuntimeState_MidExecutionConsistency` | Blocking `workers.Provider` | Keep the mid-execution snapshot window, but make the block happen at the provider boundary. |
| `factory_request_batch_test.go` | `TestFactoryRequestBatch_TagsAccessibleInTokenPayload` | `MockProvider` call inspection | The `tags_test` fixture already has a model-worker `checker`; provider calls can inspect input-token tags and auto-injected request tags. |
| `repeater_parameterized_test.go` | `TestParameterizedFields_WorkingDirectoryResolvesFromTags` | `MockProvider` call inspection | Provider calls should expose the input tokens and workstation lookup references without a capturing worker executor. |
| `multi_output_test.go` | `TestMultiOutput_NoStopWordsConfigured` | `MockProvider` | With no worker stop token, `AgentExecutor` accepts successful provider responses, matching the current explicit accepted `MockWorker` outcomes. |
| `workflow_modification_test.go` | `TestWorkflowModificationRejectionLoop` | `MockProvider` with accepting/rejecting content | This should mirror the other workflow modification tests that already use full worker pool plus provider mocks, unless exact `WorkResult.Feedback` text is the behavior under test. |

### Cleanup candidates

These tests already route through a full worker-pool-like service path or are
not caught by the harness guardrail, but still have custom-executor scaffolding
that should be reviewed:

| File | Test | Cleanup note |
| --- | --- | --- |
| `dashboard_engine_state_test.go` | `TestDashboard_EngineStateSnapshot_EndToEnd` | The test passes `WithFullWorkerPoolAndScriptWrap()` and then calls `SetCustomExecutor`. In that mode the custom-executor map is not wrapped by the harness, so the blocking executor appears to be unused scaffolding. Either remove the unused custom executor or replace it with a blocking provider edge mock if the test must reliably capture `RUNNING` state. |
| `logical_move_test.go` | `TestLogicalMove_Success` | The fixture can likely express the logical workstation directly through runtime config/AGENTS.md instead of installing a `WorkstationExecutor` in the test. |
| `logical_move_test.go` | `TestLogicalMove_PreservesTokenColor` | The logical move itself can likely use real workstation loading; the follow-up model step can use `MockProvider` call inspection to verify payload preservation. |
| `runtime_state_test.go` | `TestRuntimeState_FailureRouting` | This direct-engine sync-dispatch harness could become a service-harness provider-error case if the desired contract is externally visible failure routing. Keep it lower priority if the test is intentionally covering transitioner internals. |

### Keep as exceptions for now

These cases still need fields or timing that the current provider and command
runner seams do not expose cleanly:

| File | Tests | Why keep |
| --- | --- | --- |
| `concurrency_limit_test.go` | executor panic cases | The behavior is panic recovery around `WorkerExecutor.Execute`; provider errors do not exercise that boundary. |
| `executor_context_test.go`, `review_retry_exhaustion_test.go` | rejection feedback propagation cases | `interfaces.InferenceResponse` cannot currently set `WorkResult.Feedback`, so provider-based rejection cannot prove the feedback tag behavior. |
| `multi_input_guard_test.go`, `multichannel_guard_test.go` | spawned-child guard cases | The assertions rely on `WorkResult.SpawnedWork` with parent-derived child IDs. A later migration could use generated request batches, but that would change the fanout contract being tested. |
| `stateless_execution_test.go` | `TestStatelessExecution_ThinDispatchCarriesLookupReferencesOnly` | Provider calls observe the enriched inference dispatch after `WorkstationExecutor`; this test intentionally verifies the raw thin dispatch before workstation resolution. |
| `service_harness_test.go` | mock/custom harness contract tests | These are harness behavior tests, not workflow behavior shortcuts. |

## Guardrail

`TestFunctionalTestsUseFullWorkerPoolHarnessOrDocumentException` in
`tests/functional_test/functional_harness_guardrail_test.go` scans functional
test sources for `testutil.NewServiceTestHarness(...)` calls that omit
`testutil.WithFullWorkerPoolAndScriptWrap()`. Existing shortcut cases are
allowed only through an exact-count exception map with a reason. When adding a
new functional test, prefer the full worker-pool path; when an exception is
necessary, update both the guardrail reason and this inventory's classification.

## Stability Verification

After the final review-feedback cascade assertion fix, the Agent Factory
functional test suite passed 50 consecutive runs on 2026-04-13 with:

```powershell
cd libraries/agent-factory
go test ./tests/functional_test -count=50 -timeout 35m
```

Result: `PASS` in `1207.062s`.

## Migration Order

1. Provider/workstation tests: start with provider-mock tests that lack
   `WithFullWorkerPoolAndScriptWrap()` and preserve output, stop-word,
   history, and terminal-place assertions.
2. Script and command-runner tests: migrate command-runner edge mocks that do
   not yet use full worker pool.
3. Mock-worker tests: move configured outcomes to the mock-worker subprocess
   boundary where possible and preserve spawned work, feedback, failure, and
   output-token assertions.
4. Lifecycle/observability/resource tests: migrate ordinary behavior cases and
   document narrow exceptions for blocking, timing, and snapshot inspection.

## Verification Command

Refresh this inventory after major harness changes with:

```powershell
rg -n "NewServiceTestHarness\\(|StartFunctionalServer\\(|WithFullWorkerPoolAndScriptWrap\\(|WithRunAsync\\(|SetCustomExecutor\\(|WithProvider\\(|WithProviderCommandRunner\\(|WithCommandRunner\\(|\\.MockWorker\\(|MockWorkersConfig" libraries/agent-factory/tests/functional_test -g "*.go"
```
