# Cleanup Analyzer Report: Test Harness Baseline Prune

Date: 2026-04-18

## Scope

Deadcode baseline cleanup for Agent Factory test harnesses, test helpers, and
dormant functional or stress scaffolding. The sweep intentionally excluded the
remaining production-side factory findings.

## Starting Deadcode Output

Starting point from `origin/main:libraries/agent-factory/docs/development/deadcode-baseline.txt`:

```text
pkg/cli/init/init_test.go:105:6: unreachable func: writeCLITestFile
pkg/cli/init/init_test.go:115:6: unreachable func: readCLITestFile
pkg/factory/options.go:182:6: unreachable func: WithChannelBuffer
pkg/factory/options.go:201:6: unreachable func: WithTracer
pkg/factory/options.go:246:6: unreachable func: WithWorkRequestRecorder
pkg/factory/subsystems/history_transitioner_pipeline_test.go:47:31: unreachable func: mockMetricsRecorder.RecordDispatch
pkg/factory/subsystems/history_transitioner_pipeline_test.go:48:31: unreachable func: mockMetricsRecorder.RecordRepeat
pkg/factory/subsystems/history_transitioner_pipeline_test.go:49:31: unreachable func: mockMetricsRecorder.RecordCompletion
pkg/factory/subsystems/history_transitioner_pipeline_test.go:52:31: unreachable func: mockMetricsRecorder.RecordFailure
pkg/testutil/mock_provider.go:27:6: unreachable func: WithDefaultResponse
pkg/testutil/provider_error_smoke_harness.go:51:6: unreachable func: WithProviderErrorSmokeWorkerName
pkg/testutil/provider_error_smoke_harness.go:58:6: unreachable func: WithProviderErrorSmokePromptBody
pkg/testutil/provider_error_smoke_harness.go:149:37: unreachable func: ProviderErrorSmokeHarness.SubmitWork
pkg/testutil/replay_harness.go:36:6: unreachable func: WithReplayHarnessDir
pkg/testutil/service_harness.go:79:6: unreachable func: WithWorkstationLoader
pkg/testutil/service_harness.go:153:6: unreachable func: WithLogger
pkg/workers/inference_provider_test.go:1637:6: unreachable func: countValidProcessEnvNames
tests/functional_test/dispatcher_workflow_test.go:240:6: unreachable func: dispatcherWorkflowConfig
tests/functional_test/functional_server_test.go:186:29: unreachable func: FunctionalServer.GetDashboardUI
tests/functional_test/functional_server_test.go:429:6: unreachable func: traceExistsInMarking
tests/functional_test/integration_smoke_test.go:30:6: unreachable func: generatedDashboardReadModel
tests/functional_test/integration_smoke_test.go:1090:6: unreachable func: assertCompletedDispatchOutcomesBySession
tests/functional_test/integration_smoke_test.go:1115:6: unreachable func: assertDashboardProviderSessionIDs
tests/functional_test/runtime_state_test.go:478:29: unreachable func: acceptingExecutor.Execute
tests/functional_test/script_executor_test.go:70:30: unreachable func: blockingCommandRunner.Run
tests/functional_test/testhelpers_test.go:61:25: unreachable func: phaseExecutor.Execute
tests/functional_test/testhelpers_test.go:70:25: unreachable func: phaseExecutor.totalCalls
tests/stress/barrier_limits_test.go:384:42: unreachable func: duplicateWorkIDSpawnerExecutor.Execute
tests/stress/livelock_test.go:327:32: unreachable func: chainSpawnerExecutor.callCount
tests/stress/livelock_test.go:333:32: unreachable func: chainSpawnerExecutor.Execute
tests/stress/poison_token_test.go:705:29: unreachable func: panickingExecutor.Execute
tests/stress/recursive_work_test.go:234:36: unreachable func: unlimitedSpawnerExecutor.callCount
tests/stress/recursive_work_test.go:240:36: unreachable func: unlimitedSpawnerExecutor.Execute
tests/stress/recursive_work_test.go:289:27: unreachable func: DepthLimitGuard.Evaluate
tests/stress/resource_exhaustion_test.go:459:30: unreachable func: failEveryNExecutor.Execute
```

The starting baseline contained 35 findings. This cleanup removed 32
test-harness and test-helper findings and retained 3 out-of-scope factory
findings.

## Caller Inventories

Commands and final expected results:

```bash
rg -n "WithDefaultResponse|WithReplayHarnessDir|WithWorkstationLoader|WithLogger" libraries/agent-factory/pkg/testutil libraries/agent-factory/tests -g "*.go"
```

Expected result: 0 matches. The pruned shared `pkg/testutil` option helpers are
absent from active testutil and test code.

```bash
rg -n "WithProviderErrorSmokeWorkerName|WithProviderErrorSmokePromptBody|ProviderErrorSmokeHarness\.SubmitWork" libraries/agent-factory/pkg/testutil libraries/agent-factory/tests -g "*.go"
```

Expected result: 0 matches. Provider-error smoke tests now use seed work for
startup setup and `ServiceTestHarness.SubmitWorkRequest` for late submissions.

```bash
rg -n "writeCLITestFile|readCLITestFile|mockMetricsRecorder|countValidProcessEnvNames" libraries/agent-factory -g "*.go"
```

Expected result: 21 matches in
`libraries/agent-factory/pkg/cli/config/config_test.go`. Those matches are the
active package-local CLI config helper definitions and callers that were
intentionally retained; the dead CLI init helpers, subsystem recorder scaffold,
and inference provider helper are absent.

```bash
rg -n "dispatcherWorkflowConfig|GetDashboardUI|traceExistsInMarking|generatedDashboardReadModel|assertCompletedDispatchOutcomesBySession|assertDashboardProviderSessionIDs" libraries/agent-factory/tests/functional_test -g "*.go"
```

Expected result: 0 matches. Functional tests no longer carry obsolete raw
dashboard or assertion helpers.

```bash
rg -n "acceptingExecutor|blockingCommandRunner|phaseExecutor" libraries/agent-factory/tests/functional_test -g "*.go"
```

Expected result: 0 matches. Dormant functional executor scaffolding is absent.

```bash
rg -n "duplicateWorkIDSpawnerExecutor|chainSpawnerExecutor|panickingExecutor|unlimitedSpawnerExecutor|DepthLimitGuard|failEveryNExecutor" libraries/agent-factory/tests/stress -g "*.go"
```

Expected result: 0 matches. Dormant stress executors, the custom guard, and
their skipped scenarios were removed together.

```bash
rg -n "WithDefaultResponse|WithReplayHarnessDir|WithWorkstationLoader|WithLogger|WithProviderErrorSmokeWorkerName|WithProviderErrorSmokePromptBody|ProviderErrorSmokeHarness\.SubmitWork|writeCLITestFile|readCLITestFile|mockMetricsRecorder|countValidProcessEnvNames|dispatcherWorkflowConfig|GetDashboardUI|traceExistsInMarking|generatedDashboardReadModel|assertCompletedDispatchOutcomesBySession|assertDashboardProviderSessionIDs|acceptingExecutor|blockingCommandRunner|phaseExecutor|duplicateWorkIDSpawnerExecutor|chainSpawnerExecutor|panickingExecutor|unlimitedSpawnerExecutor|DepthLimitGuard|failEveryNExecutor" libraries/agent-factory/docs/development/deadcode-baseline.txt
```

Expected result: 0 matches. The pruned test harness and helper findings are not
in the accepted deadcode baseline.

## Removed Symbols

- `writeCLITestFile`
- `readCLITestFile`
- `mockMetricsRecorder.RecordDispatch`
- `mockMetricsRecorder.RecordRepeat`
- `mockMetricsRecorder.RecordCompletion`
- `mockMetricsRecorder.RecordFailure`
- `WithDefaultResponse`
- `WithProviderErrorSmokeWorkerName`
- `WithProviderErrorSmokePromptBody`
- `ProviderErrorSmokeHarness.SubmitWork`
- `WithReplayHarnessDir`
- `WithWorkstationLoader`
- `WithLogger` from `pkg/testutil`
- `countValidProcessEnvNames`
- `dispatcherWorkflowConfig`
- `FunctionalServer.GetDashboardUI`
- `traceExistsInMarking`
- `generatedDashboardReadModel`
- `assertCompletedDispatchOutcomesBySession`
- `assertDashboardProviderSessionIDs`
- `acceptingExecutor.Execute`
- `blockingCommandRunner.Run`
- `phaseExecutor.Execute`
- `phaseExecutor.totalCalls`
- `duplicateWorkIDSpawnerExecutor.Execute`
- `chainSpawnerExecutor.callCount`
- `chainSpawnerExecutor.Execute`
- `panickingExecutor.Execute` from `tests/stress`
- `unlimitedSpawnerExecutor.callCount`
- `unlimitedSpawnerExecutor.Execute`
- `DepthLimitGuard.Evaluate`
- `failEveryNExecutor.Execute`

## Final Deadcode Output

Final output from `libraries/agent-factory/bin/deadcode-current.txt` after
`cd libraries/agent-factory && make lint`:

```text
pkg/factory/options.go:182:6: unreachable func: WithChannelBuffer
pkg/factory/options.go:201:6: unreachable func: WithTracer
pkg/factory/options.go:246:6: unreachable func: WithWorkRequestRecorder
```

The final baseline contains 3 findings. They are pre-existing production-side
factory findings outside the test-harness scope of this cleanup.

## Retained Exceptions

No test harness, local test helper, functional dashboard helper, or stress
executor findings were retained as exceptions.

Retained out-of-scope baseline findings:

- `WithChannelBuffer`
- `WithTracer`
- `WithWorkRequestRecorder`

## Validation

Commands run:

```bash
cd libraries/agent-factory && go test ./pkg/testutil ./tests/functional_test -run "TestReplay|TestWorkDispatchContractSmoke|TestServiceHarness" -count=1
cd libraries/agent-factory && go test ./pkg/testutil ./tests/functional_test -run "TestProviderErrorSmoke" -count=1
cd libraries/agent-factory && go test ./pkg/cli/init ./pkg/factory/subsystems ./pkg/workers -count=1
cd libraries/agent-factory && go test ./tests/functional_test -run '^$' -count=1
cd libraries/agent-factory && go test ./tests/functional_test ./tests/stress -run '^$' -count=1
cd libraries/agent-factory && make lint
```

Results:

- Focused replay and service harness tests passed.
- Focused provider-error smoke tests passed.
- CLI init, subsystem pipeline, and worker package tests passed.
- Functional test compile pass passed.
- Functional and stress compile pass passed.
- `make lint` passed with `[agent-factory:deadcode] baseline matches`.
