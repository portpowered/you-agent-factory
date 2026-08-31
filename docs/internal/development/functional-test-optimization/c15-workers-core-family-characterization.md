# C15 Workers core-family characterization

## Gate and scope

This is the Story `functional-test-optimization-c15-workers-core-family-001`
characterization record. It freezes the executable behavior and process-cost
spine before Stories 002–005 make structural changes. The target is exactly the
seven packages named by the PRD:

```text
tests/functional/workers/mock
tests/functional/workers/agent
tests/functional/workers/invoke_continue
tests/functional/workers/inference/claude
tests/functional/workers/inference/codex
tests/functional/workers/script
tests/functional/workers/transports/cli/run/help
```

`A-13` is intentionally witnessed by the existing
`tests/functional/workers/concurrency` selector because the agent assertion
ledger assigns that cross-package concurrency row to the same A/I behavior
matrix. It is not included in the seven-package timing total. No production,
public contract, generated artifact, or functional assertion was changed for
this characterization story.

The PRD names `docs/temp/functional-test-optimization.md` as its source plan,
but that temporary file is absent from this checkout. The PRD explicitly gives
the customer payload as the safe non-weakening assumption. The tracked C01
inventory and the existing agent/invoke/concurrency ledgers were used only as
supporting source records; no conflict was found and no structural edit was
made.

**GATE-CHAR: PASS** at baseline commit
`42eeee4472656b8290f798c36a5b8c871b24d7d0`.

## Baseline run and package timings

The authoritative baseline is successful GitHub Actions run
[`33337769566`](https://github.com/portpowered/you-agent-factory/actions/runs/33337769566)
at the exact baseline commit above. The downloaded `functional-test-diagnostics`
artifact was read from `functional-tests.md`, `functional-timing-summary.json`,
`coverage-summary.json`, and `functional-coverage-verdict.txt`.

| Baseline fact | Observed result |
| --- | --- |
| Go / runner | Go `1.25.0`, Ubuntu `24.04.4` |
| Functional test outcomes | `1,068` pass, `0` fail, `1` skip |
| Skip disposition | `TestResumeFromRecordingChildProcess` in `tests/functional/sessions/resume_from_recording`; out of scope for this lane |
| Go coverage | `61.6%` |
| Owned skips / quarantine | `0` owned skips; no owned package quarantine |
| Target package sum | `56.460` package-seconds |

| Target package | Baseline package time (s) | Artifact status |
| --- | ---: | --- |
| `tests/functional/workers/mock` | 17.884 | pass |
| `tests/functional/workers/agent` | 8.740 | pass |
| `tests/functional/workers/invoke_continue` | 7.768 | pass |
| `tests/functional/workers/inference/claude` | 7.199 | pass |
| `tests/functional/workers/inference/codex` | 6.254 | pass |
| `tests/functional/workers/script` | 3.175 | pass |
| `tests/functional/workers/transports/cli/run/help` | 5.441 | pass |

These values are the PRD's before values. Local timing is diagnostic only; the
package timing reported by the eventual PR is authoritative.

## Current executable selector inventory

The exact command below compiled and listed every default `Test*` selector in
the seven target packages without executing a behavior row:

```text
go test ./tests/functional/workers/mock -list '^Test' -count=0
go test ./tests/functional/workers/agent -list '^Test' -count=0
go test ./tests/functional/workers/invoke_continue -list '^Test' -count=0
go test ./tests/functional/workers/inference/claude -list '^Test' -count=0
go test ./tests/functional/workers/inference/codex -list '^Test' -count=0
go test ./tests/functional/workers/script -list '^Test' -count=0
go test ./tests/functional/workers/transports/cli/run/help -list '^Test' -count=0
```

Exit status was `0`. The observed default top-level selector set was:

| Package | Listed top-level selectors |
| --- | --- |
| `mock` | `TestBuiltCLIBatchExitCodesReportSingleWorkOutcome`, `TestBuiltCLIBatchExitCodesAggregateFailureCauses`, `TestJavaScriptMockWorkersRemainFakeWhenACPProviderIsSelected`, `TestBuiltCLINamedInvocationExitCodesCharacterizeOneShot`, `TestSharedProcessWorkersMock` |
| `agent` | `TestAgentSharedProcess` |
| `invoke_continue` | `TestInvokeContinueForcedAssertionCleansOwnedResources`, `TestDWROS8ManagerInterruptsOnlyOneRemoteWorker`, `TestDWROS8ManagerInspectsTwoIsolatedRemoteWorkers`, `TestInvokeContinueSharedProcess` |
| `inference/claude` | `TestClaudeForcedAssertionFailureCleansOwnedResources`, `TestClaudeDefaultLaneSharedProcess`, `TestClaudeCommandRouterFailsClosed` |
| `inference/codex` | `TestCodexDefaultLaneSharedProcess`, `TestCodexCommandRouterFailsClosed`, `TestCodexGoldenSharedProcess`, `TestCodexWorktreeWorkstationDispatch_MaterializesCheckoutAndOmitsCLIWorktreeFlag` |
| `script` | `TestScriptCommandRouterRejectsUnknownAndDuplicateSelectors`, `TestScriptWorkerSharedSuccessSpine` |
| `transports/cli/run/help` | `TestCLIRunHelpShowsInvocationSignatureForNamedFactory`, `TestCLIRunHelpDistinguishesRequiredAndOptionalParameters`, `TestCLIRunHelpDoesNotDispatchExternalWork`, `TestCLISessionHelpPublishesRunnablePlacementExamples`, `TestCLIRunHelpCoversGenericAndExplicitFactorySelections`, `TestCLIRunHelpResetsEmptyAndInvalidSelections` |

The default selector census contains `25` top-level declarations. The tagged
`functionallong` rows are catalogued below as retained witnesses but do not
appear in this default list; the later long/clean-room gate owns their
execution.

## Root, server, build, and cleanup census

The counts below use a declared unit. `Root construction` counts the current
default-path `BuildProcess` or `StartFunctionalAPIServer` construction owned by
the target package. `API starts` counts the package's explicit API starter
counter or the equivalent shared server path. `Built CLI` counts the
test-owned compiler and target-binary executions. Forced-cleanup child
processes are listed separately because they deliberately create a second
process-scoped fixture; tagged-long setup is also separated from the default
baseline.

| Package | Default root construction | Default API starts | Test-owned build / executable runs | Cleanup owner and census witness |
| --- | --- | ---: | --- | --- |
| `mock` | `2`: shared `StartFunctionalAPIServer` plus direct ACP `BuildProcess` | `1` shared server | `1` `go build` compiler through `compiledCLIBinary.once`; `6` `runBuiltYouBinary` executions (three single-batch children, one aggregate child, two named children) | Shared fixture/session cleanup; `TestMain` temporary binary cleanup; M-19 proves no ACP command factory invocation |
| `agent` | `2`: shared fixture plus the isolated malformed-configuration probe | `1` on the shared fixture | none | `agentSharedProcessFixture.close`; the forced-cleanup child has its own one-build/one-listener report |
| `invoke_continue` | `1` package fixture | `1` package fixture | none | package `TestMain`/fixture close, session and response-stream counters; forced child report requires one build and one API start |
| `inference/claude` | `1` shared conductor fixture | `1` shared server | none | `assertSharedProcessCleanup` and `removeClaudeOwnedDirectories`; forced child separately proves one process/listener teardown |
| `inference/codex` | `1` package fixture shared by conductor, golden, and worktree rows | `1` shared server | none | package fixture close plus `cleanupCodexWorktreeScenario` for each of two local-real worktree rows |
| `script` | `1` short shared spine; `+1` tagged-long `StartFunctionalAPIServer` path | `1` short shared server; `+1` tagged-long server | none | shared process close and session/Factory directory cleanup |
| `transports/cli/run/help` | `1` reusable help fixture | `0` (read-only help) | none | help fixture close checks one root build, zero provider calls, zero active invocations, and removed root |

The default short-path fixture construction therefore has nine direct
test-process root construction paths and six API-server starts across the seven
packages. Those are topology counts, not a claim that internal runtime
goroutines or the separate forced-cleanup child processes are absent. Existing
fixture counters assert the one-build/one-server invariants at their owning
cleanup boundaries.

## OS-process census

The static site unit is one `exec.Command` or `exec.CommandContext` call
expression under the seven target package paths. `exec.LookPath` is a lookup,
not a process-start site. Site identities follow the repository's C01 naming
convention. The target subset is eight static sites: five intentional and
three accidental. The current broad checker remains unchanged at 70 sites
across 23 packages.

| Site identity | Source / enclosing identity | Property or current purpose | Static count / normal execution count | Cleanup owner | Current verdict |
| --- | --- | --- | --- | --- | --- |
| `OSSPAWN-tests-functional-workers-agent-shared-process-cleanup-test-runAgentForcedCleanupParent-01` | `agent/shared_process_cleanup_test.go:84`, `runAgentForcedCleanupParent` | Child failure and descendant cleanup are observed by the parent report | one site / one forced child | `runAgentForcedCleanupParent`; child fixture close | `INTENTIONAL-OS` — `descendant-cleanup` |
| `OSSPAWN-tests-functional-workers-invoke-continue-forced-cleanup-test-TestInvokeContinueForcedAssertionCleansOwnedResources-01` | `invoke_continue/forced_cleanup_test.go:28`, `TestInvokeContinueForcedAssertionCleansOwnedResources` | Parent observes original assertion failure and package cleanup census | one site / one forced child | forced-child fixture report | `INTENTIONAL-OS` — `descendant-cleanup` |
| `OSSPAWN-tests-functional-workers-inference-claude-conductor-forced-cleanup-test-TestClaudeForcedAssertionFailureCleansOwnedResources-01` | `inference/claude/conductor_forced_cleanup_test.go:27`, `TestClaudeForcedAssertionFailureCleansOwnedResources` | Parent observes original assertion failure and resource teardown | one site / one forced child | forced-child fixture report | `INTENTIONAL-OS` — `descendant-cleanup` |
| `OSSPAWN-tests-functional-workers-mock-compiled-cli-exit-codes-helpers-test-buildYouBinary-01` | `mock/compiled_cli_exit_codes_helpers_test.go:86`, `buildYouBinary` | Selects a test-owned delivered `./cmd/factory` executable | one site / one compiler child due to `sync.Once` | `TestMain` temporary binary directory cleanup | `INTENTIONAL-OS` — `executable-selection` |
| `OSSPAWN-tests-functional-workers-mock-compiled-cli-exit-codes-helpers-test-runBuiltYouBinary-01` | `mock/compiled_cli_exit_codes_helpers_test.go:103`, `runBuiltYouBinary` | Captures real stdout/stderr and OS exit status | one site / six binary executions | each built-CLI session plus `TestMain` | `INTENTIONAL-OS` — `exit-status` |
| `OSSPAWN-tests-functional-workers-mock-javascript-acp-test-mockACPCommandFactory-01` | `mock/javascript_acp_test.go:74`, `mockACPCommandFactory` | The returned command is unreachable when MockWorkers intercepts ACP; M-19 asserts ACP starts and provider calls are zero | one site / zero executions | `TestJavaScriptMockWorkersRemainFakeWhenACPProviderIsSelected` | `ACCIDENTAL-OS` — remove in Story 002 while retaining the zero-call assertion |
| `OSSPAWN-tests-functional-workers-inference-codex-worktree-workstation-test-cleanupCodexWorktreeScenario-01` | `inference/codex/worktree_workstation_test.go:502`, `cleanupCodexWorktreeScenario` | Direct `git worktree remove` fixture cleanup | one site / two normal cleanup commands for two scenarios | `cleanupCodexWorktreeScenario` | `ACCIDENTAL-OS` — replace with the approved repository/filesystem edge in Story 004 |
| `OSSPAWN-tests-functional-workers-inference-codex-worktree-workstation-test-runGitForCodexWorktreeFunctionalTest-01` | `inference/codex/worktree_workstation_test.go:610`, `runGitForCodexWorktreeFunctionalTest` | Direct `git init/config/commit` fixture setup | one site / eight normal setup commands for two scenarios | scenario repository temp-root cleanup | `ACCIDENTAL-OS` — replace with the approved repository/filesystem edge in Story 004 |

The normal short-path direct launches represented by the eight sites are
therefore `20`: one agent forced child, one invoke forced child, one Claude
forced child, one mock compiler, six mock binary runs, two Codex worktree
cleanup commands, and eight Codex repository setup commands. The ACP helper is
observed at zero. This is a source-control-flow count, not an unobserved OS
telemetry claim; failure-path duplicate cleanup is owned by the later cleanup
verification. The Codex `exec.LookPath` at `worktree_workstation_test.go:595`
is retained as a lookup and is not part of this total.

The read-only command
`make functional-os-boundary-check` exited `0` and reported:

```text
[agent-factory:functional-os-boundary] static OS-spawn baseline holds: observed=70 baseline=70 packages=23 intentional=62 accidental=8 decreased=0
[agent-factory:functional-os-boundary] reconciled 70 inventory OS-spawn records
```

This confirms that Story 001 made no OS-boundary edit and that the eight-site
target subset is classified before Stories 002 and 004 remove their three
accidental sites.

## Complete 82-row assertion ledger

Each row below names the exact Go test selector (including nested selector
names where applicable) and the assertion function or test-owned cleanup
probe that proves the PRD's `given/when/then` observation. A source reference
is stable-keyed by the blob hash table in the next section. Rows marked
`functionallong` remain intentional and are owned by the later long/clean-room
gate; they are not silently omitted from the matrix.

### Mock Workers: M-01–M-19

| ID | Exact executable witness | Assertion / cleanup proof | Source key |
| --- | --- | --- | --- |
| M-01 | `TestSharedProcessWorkersMock/NamedAgy` | `testNamedAgyMockPreservesDispatchMetadataAndCompletionLog` checks exact dispatch metadata, completion log, done Work, and no live provider | `mock/agy_named_mock_test.go:21` |
| M-02 | `TestSharedProcessWorkersMock/ScriptClassifier` | `testScriptWorkerClassifierRoutesWithoutModelCalls` checks classifier routing and zero model/provider calls | `mock/classifier_test.go:26` |
| M-03 | `TestSharedProcessWorkersMock/MockWorkersReplace` | `testMockWorkersReplaceOnlyNamedChildren` checks only the named child is mocked, the unmatched child makes exactly one controlled provider call, and both Work items complete | `mock/replacement_test.go:44` |
| M-04 | `TestSharedProcessWorkersMock/UnknownWorker/invalid runType in override entry` | `testUnknownWorkerOverrideFailsActionably` checks pre-dispatch diagnostic and zero provider calls | `mock/replacement_test.go:85,114` |
| M-05 | `TestSharedProcessWorkersMock/FutureFields` | `testFutureMockWorkerFieldsAreIgnoredAndDispatchBehaviorIsPreserved` checks one accepted mocked dispatch and done Work with future fields ignored | `mock/replacement_test.go:149` |
| M-06 | `TestSharedProcessWorkersMock/MockWorkerFailure` | `testMockWorkerFailureReturnsStablePublicFailure` checks reject and gate-timeout Work/dispatch details, timeout lower bound, and zero provider calls; `assertMockGateTimeoutDispatch` checks timeout evidence | `mock/replacement_test.go:176`; `mock/gate_timeout_test.go:11` |
| M-07 | `TestSharedProcessWorkersMock/{RootSelection,ServiceConfigAlignment,ExpectedArtifacts,MockUsage}` | `testMockWorkerSelectedThroughCustomerProcess`, `testServiceConfigOverrideAlignmentCustomerProcess`, `testExpectedArtifactsEnforceThroughSharedProcess`, and `testMockWorkerUsageIsVisibleAndPriceableThroughSharedProcess` preserve root selection, config edge alignment, artifact enforcement, usage, and pricing | `mock/root_build_test.go:24`; `mock/service_config_override_alignment_test.go:15`; `mock/artifact_registry_test.go:35`; `mock/usage_costs_test.go:23` |
| M-08 | `TestSharedProcessWorkersMock/{BatchDefaultSuccess,BatchVerboseSuccess,BatchAllFailedHuman,BatchMixedJSON}` | `testSharedBatchDefaultSuccess`, `testSharedBatchVerboseSuccess`, `testSharedBatchAllFailedHuman`, and `testSharedBatchMixedJSON` check primary result, Work states, deterministic failure collection, human/JSON output, and shared-session behavior | `mock/batch_invocation_shared_test.go:18,29,40,69` |
| M-09 | `TestSharedProcessWorkersMock/{BatchCircuitBreakerHuman,BatchCircuitBreakerJSON,BatchScriptFailureHuman,BatchScriptFailureJSON,NamedHumanFailure}` | The five shared invocation helpers check circuit-breaker/script/named public human and JSON error detail and retained failure causes | `mock/batch_invocation_shared_test.go:91,115,142,165,191` |
| M-10 | `TestSharedProcessWorkersMock/PlainBatchDrainCounterexamples/{empty,terminal work,continuous idle}` | `testPlainBatchDrainPreservesFiniteAndContinuousCounterexamples` checks quiet empty, one terminal success, and controlled cancellation of continuous idle without false completion | `mock/plain_batch_drain_test.go:100,135` |
| M-11 | `TestSharedProcessWorkersMock/{JavaScriptLiveCapacity,LiveCapacityIncrease,LiveCapacitySafeReduction,LiveCapacityUnsafeReduction,LiveCapacityRecording}` | Capacity helpers check wake-up, active-work preservation, unsafe-reduction refusal, ordered recording/replay, and cursor behavior | `mock/live_capacity_javascript_test.go:25`; `mock/live_capacity_test.go:53,119,156,223` |
| M-12 | `TestSharedProcessWorkersMock/{PlainBatchDrainRejectsPreActivationCancellation,PlainBatchDrainStopsAfterWorkerActivationCancellation}` | Cancellation helpers check both pre-activation and admitted-worker diagnostics with no stranded active call | `mock/plain_batch_drain_test.go:169,193` |
| M-13 | `TestSharedProcessWorkersMock/PlainBatchDrainReportsStrandedWork` | `testPlainBatchDrainReportsStrandedWork` checks canonical incomplete-drain failure, empty stdout, and safe diagnostic | `mock/plain_batch_drain_test.go:62` |
| M-14 | `TestSharedProcessWorkersMock` cleanup for each server-backed row | `session.closeAndAssertGone` plus fixture cleanup checks fresh Session deletion, route/delegate reset, zero active calls/listeners, and no runtime/resource carry-over | `mock/shared_process_test.go:53,90` |
| M-15 | `TestBuiltCLIBatchExitCodesReportSingleWorkOutcome/success quiet result exits zero` | `runCompiledBatchQuietSuccess` checks real zero exit, exact stdout, and empty stderr | `mock/batch_invocation_exit_codes_test.go:36,43`; `mock/compiled_cli_exit_codes_helpers_test.go:96` |
| M-16 | `TestBuiltCLIBatchExitCodesReportSingleWorkOutcome/{failed terminal Work exits nonzero with human detail,failed terminal Work JSON is parseable}` | `runCompiledBatchHumanFailure` and `runCompiledBatchJSONFailure` check nonzero exit, human detail, and parseable complete JSON failure | `mock/batch_invocation_exit_codes_test.go:48,53` |
| M-17 | `TestBuiltCLIBatchExitCodesAggregateFailureCauses/all submitted Work failures have a complete JSON collection` | `runCompiledBatchAllFailedJSON` checks nonzero exit and both failures exactly once in deterministic order | `mock/batch_invocation_exit_codes_test.go:64,71` |
| M-18 | `TestBuiltCLINamedInvocationExitCodesCharacterizeOneShot/{success preserves primary result,terminal failure preserves JSON detail}` | The two named one-shot helpers check primary success/zero exit and typed terminal JSON failure/nonzero exit | `mock/named_invocation_exit_codes_test.go:23,30,57` |
| M-19 | `TestJavaScriptMockWorkersRemainFakeWhenACPProviderIsSelected` | The test checks successful MockWorkers completion with ACP starts and `ProviderCommandRunner` calls both zero; `mockACPCommandFactory` is never invoked | `mock/javascript_acp_test.go:21,74` |

### Agent: A-01–A-14

| ID | Exact executable witness | Assertion / cleanup proof | Source key |
| --- | --- | --- | --- |
| A-01 | `TestAgentSharedProcess/Inert` | `fixture.assertInert` checks canonical providers before activation, zero API starts, zero route calls, and no Session/provider effect | `agent/shared_process_test.go:41`; `agent/shared_process_lifecycle_test.go:22` |
| A-02 | `TestAgentSharedProcess/Codex` | Shared scenario assertions check one controlled call, one done Work, accepted dispatch, and exact output/correlation | `agent/shared_process_test.go:65`; `agent/shared_process_assertions_test.go` |
| A-03 | `TestAgentSharedProcess/Registered` | Shared scenario assertions check registered-agent provider routing once with accepted output and done/failed counts | `agent/shared_process_test.go:65`; `agent/shared_process_assertions_test.go` |
| A-04 | `TestAgentSharedProcess/RuntimeRoot` | `assertAgentRuntimeRootPublicIdentities` checks Factory Session, Worker Session, response Run/Event, dispatch, request, and Work identity correlation | `agent/shared_process_assertions_test.go:398` |
| A-05 | `TestAgentSharedProcess/Claude` | Shared scenario route/call assertions check Claude once and Codex unused | `agent/shared_process_test.go:65`; `agent/shared_process_assertions_test.go` |
| A-06 | `TestAgentSharedProcess/Invalid/UnknownProvider` | `fixture.assertUnknownProvider` checks actionable pre-runtime failure, zero route calls, zero API starts, and no Session | `agent/shared_process_lifecycle_test.go:30` |
| A-07 | `TestAgentSharedProcess/Invalid/MalformedConfiguration` | `runAgentMalformedConfigurationProbe` builds an isolated root and checks validation failure before provider effects | `agent/shared_process_lifecycle_test.go:221` |
| A-08 | `TestAgentSharedProcess/Empty` | `fixture.runEmptyScenario` checks accepted empty Work identity followed by valid Work success in one Session | `agent/shared_process_lifecycle_test.go:112` |
| A-09 | `TestAgentSharedProcess/Minimum` | Shared scenario assertions check one Work/dispatch/attempt and exact minimum input marker | `agent/shared_process_test.go:65`; `agent/shared_process_assertions_test.go` |
| A-10 | `TestAgentSharedProcess/Failure` | Shared scenario assertions check typed failed Work/dispatch, no fallback, and closable Session | `agent/shared_process_test.go:65`; `agent/shared_process_assertions_test.go` |
| A-11 | `TestAgentSharedProcess/Timeout` | Shared scenario assertions check exhausted timeout observations, immutable route, no fallback, and zero active calls | `agent/shared_process_test.go:65`; `agent/shared_process_assertions_test.go` |
| A-12 | `TestAgentSharedProcess/Cancel` | `assertAgentCancellationResponseEvents` checks terminal cancellation response, session scope, `stream_canceled`, and zero active calls | `agent/shared_process_lifecycle_test.go:409` |
| A-13 | `TestConcurrencySharedProcess/Cancel/CC-04` | `runSessionCancellationIsolation` checks two overlapping Sessions remain correlated and the survivor completes after only one is canceled | `concurrency/shared_process_test.go:64,65` |
| A-14 | `TestAgentSharedProcess/Cleanup` | `runAgentForcedCleanupParent` and the child report check original assertion visibility, process/listener close, Session/stream deletion, route reset, zero active calls, and removed directories | `agent/shared_process_cleanup_test.go:81,84` |

### Invoke / continue: I-01–I-14

| ID | Exact executable witness | Assertion / cleanup proof | Source key |
| --- | --- | --- | --- |
| I-01 | `TestInvokeContinueSharedProcess/InvokeContinueLocal` | `runDirectWorkerSessionInvokeContinueLocal` checks exact source/successor IDs and states, output, two calls with only the second using `resume`, and request-id conflict without a third call | `invoke_continue/worker_sessions_invoke_continue_test.go:50,87` |
| I-02 | `TestInvokeContinueSharedProcess/InvokeExecutionFileFutureFields` | `runDirectWorkerSessionInvokeExecutionFileToleratesFutureFields` checks known fields, sorted warning paths without values, and omission of ignored fields from structured output | `invoke_continue/worker_sessions_invoke_continue_test.go:163` |
| I-03 | `TestInvokeContinueSharedProcess/ResumeRecordedProviderSession` | `runWSRFT015DirectWorkerSessionResumeUsesExactRecordedProviderSession` checks exact paused/resumed controls, one resume call, and unknown resume not-found | `invoke_continue/worker_sessions_invoke_continue_test.go:205` |
| I-04 | `TestInvokeContinueSharedProcess/RemoteInterrupt` | `runDirectWorkerSessionRemoteInterruptUsesExactRouteAndAdmissionSnapshots` checks route, admission/source/successor snapshots, event topics, and no local fallback | `invoke_continue/worker_sessions_invoke_continue_test.go:417` |
| I-05 | `TestInvokeContinueSharedProcess/RemoteControls/{pause,resume,cancel,terminate}` | `runDirectWorkerSessionRemoteControlsUseExactRoutesWithoutFallback` checks empty body, typed applied state/dispatch ID, and zero local calls for each control | `invoke_continue/worker_sessions_invoke_continue_test.go:460,516` |
| I-06 | `TestInvokeContinueSharedProcess/{ContinueUnknownSource,ContinueUnassociatedSource}` | The two continue helpers check not-found/continuation-invalid mapping and no extra or fallback call | `invoke_continue/worker_sessions_invoke_continue_test.go:537,558` |
| I-07 | `TestInvokeContinueSharedProcess/ContinueStaleProviderSession` | `runDirectWorkerSessionContinueStaleProviderSessionDoesNotFreshStart` checks typed failure, one exact resume attempt, and no fresh start | `invoke_continue/worker_sessions_invoke_continue_test.go:588` |
| I-08 | `TestInvokeContinueSharedProcess/{RemoteInvokeStreamFailure,RemoteInvokeCancellation}` | The stream-failure and caller-cancellation helpers check `WORKER_SESSION_STREAM_SOURCE_FAILURE` / `WORKER_SESSION_INVOKE_INTERRUPTED` and no local fallback | `invoke_continue/worker_sessions_invoke_continue_test.go:671,711` |
| I-09 | `TestInvokeContinueSharedProcess/RemoteContinueProviderFailures/{foreign-provider-session,stale-provider-session,unsupported-continuation,admission-failure}` | `runDirectWorkerSessionRemoteContinueProviderFailuresDoNotFallback` checks exact HTTP status/code/message mapping and zero local calls for all four failures | `invoke_continue/worker_sessions_invoke_continue_test.go:624,641` |
| I-10 | `TestInvokeContinueSharedProcess/UnsupportedProviderContinuation` | `runDirectWorkerSessionContinueUnsupportedProvider` checks terminal typed failure and only the initial non-resume call | `invoke_continue/worker_sessions_invoke_continue_test.go:358` |
| I-11 | `TestInvokeContinueSharedProcess/InvokeContinueLocal` controls | `assertLocalTerminalWorkerSessionControls` checks pause/resume/cancel/terminate are terminal `NOOP`/`COMPLETED` with no provider call | `invoke_continue/worker_sessions_invoke_continue_test.go:159,395` |
| I-12 | `TestDWROS8ManagerInspectsTwoIsolatedRemoteWorkers` | Public manager list/show/stream assertions check two active workers' identities, routes, correlations, order, replay, and completion remain isolated | `invoke_continue/manager_scenario_test.go:129` |
| I-13 | `TestDWROS8ManagerInterruptsOnlyOneRemoteWorker` | Public manager assertions check A cancellation/resume, B remains active, exact stream/request order, and no cross-identity effects | `invoke_continue/manager_interrupt_scenario_test.go:42` |
| I-14 | `TestInvokeContinueForcedAssertionCleansOwnedResources` | Parent/child report checks original assertion, one build/server, closed listener, deleted Session/stream/root, zero active/provider calls, and no routes | `invoke_continue/forced_cleanup_test.go:21,28` |

### Claude: C-01–C-08

| ID | Exact executable witness | Assertion / cleanup proof | Source key |
| --- | --- | --- | --- |
| C-01 | `TestClaudeDefaultLaneSharedProcess/ConcurrentDefaultScenarios/Success` | Shared Claude scenario assertions check one accepted dispatch/done Work, exact model/output/Provider Session, and ordered response Events | `inference/claude/conductor_test.go:23,29`; `inference/claude/conductor_scenarios_test.go` |
| C-02 | `functionallong: TestClaudeGoldenFullStreamTextSuccess`; `functionallong: TestClaudeGoldenToolLifecycleAndSessionIdentity` | Golden assertions check text/tool lifecycle, Provider Session identity, and sanitized records | `inference/claude/golden_success_test.go:25,40` |
| C-03 | `TestClaudeDefaultLaneSharedProcess/ConcurrentDefaultScenarios/StructuredFailure` | Shared scenario assertions check failed Work/dispatch/inference and permanent provider-failure detail | `inference/claude/conductor_test.go:23,29`; `inference/claude/conductor_scenarios_test.go` |
| C-04 | `TestClaudeDefaultLaneSharedProcess/ConcurrentDefaultScenarios/Timeout` | Shared scenario assertions check retry exhaustion, timeout closure, no invented success, and exact call count | `inference/claude/conductor_test.go:23,29`; `inference/claude/conductor_scenarios_test.go` |
| C-05 | `TestClaudeDefaultLaneSharedProcess/ConcurrentDefaultScenarios/Cancellation` | Shared scenario assertions check canceled outcome/response, context propagation, and zero active calls | `inference/claude/conductor_test.go:23,29`; `inference/claude/conductor_scenarios_test.go` |
| C-06 | `TestClaudeDefaultLaneSharedProcess/SameProcessRecoveryAfterAdverseSession/{Cancellation,FreshSuccess}` | Recovery helper checks a fresh success has no prior marker and route state resets on the same process | `inference/claude/conductor_test.go:38,51,56,61` |
| C-07 | `TestClaudeCommandRouterFailsClosed` | Router test checks unknown, duplicate, and empty selector fail closed without consuming a transcript | `inference/claude/conductor_test.go:75`; `inference/claude/conductor_router_test.go` |
| C-08 | `TestClaudeForcedAssertionFailureCleansOwnedResources/ForcedAssertion` | Parent/child report checks visible assertion failure, closed process/listener, Session/stream cleanup, zero active calls, and removed directories | `inference/claude/conductor_forced_cleanup_test.go:20,109` |

### Codex: X-01–X-09

| ID | Exact executable witness | Assertion / cleanup proof | Source key |
| --- | --- | --- | --- |
| X-01 | `TestCodexDefaultLaneSharedProcess/Success` | Shared Codex scenario assertions check accepted Work/dispatch, model args, Provider Session, ordered Events, and response Events | `inference/codex/conductor_test.go:23,31`; `inference/codex/conductor_scenarios_test.go` |
| X-02 | `TestCodexGoldenSharedProcess/Success/{TestCodexGoldenTextAndToolSuccess,TestCodexGoldenDerivesProviderSessionAndResponseEvents}` | Golden assertions check text/tool output, Provider Session, and response-event records | `inference/codex/golden_success_test.go:17,29,32` |
| X-03 | `TestCodexGoldenSharedProcess/StructuredFailure` | Golden failure helper checks failed Work/dispatch and exact authentication-failure detail | `inference/codex/golden_failure_test.go`; `inference/codex/golden_success_test.go:17` |
| X-04 | `TestCodexGoldenSharedProcess/Timeout` | Golden timeout helper checks failed Work/dispatch, exact timeout, and configured call count | `inference/codex/golden_failure_test.go`; `inference/codex/golden_success_test.go:17` |
| X-05 | `TestCodexDefaultLaneSharedProcess/Cancellation` | Shared scenario assertions check canceled terminal observations and zero active calls | `inference/codex/conductor_test.go:23,31`; `inference/codex/conductor_scenarios_test.go` |
| X-06 | `TestCodexDefaultLaneSharedProcess` and `TestCodexGoldenSharedProcess` cleanup | Shared fixture/session/stream counters and `closeCodexPackageFixture` check no stale route/provider marker and zero cleanup counters | `inference/codex/package_fixture_test.go:299`; `inference/codex/conductor_test.go`; `inference/codex/golden_success_test.go` |
| X-07 | `TestCodexCommandRouterFailsClosed` | Router test checks unknown and duplicate selectors fail closed without consuming another route | `inference/codex/conductor_test.go:44`; `inference/codex/conductor_router_test.go` |
| X-08 | `TestCodexWorktreeWorkstationDispatch_MaterializesCheckoutAndOmitsCLIWorktreeFlag/{DefaultDotWorktreesParent,ExistingClaudeWorktreesParent}` | Worktree assertions check exact checkout path, WorkDir, omitted CLI `--worktree`, Work/Event lineage, and both parent layouts | `inference/codex/worktree_workstation_test.go:75,376,414,445` |
| X-09 | `TestCodexWorktreeWorkstationDispatch_MaterializesCheckoutAndOmitsCLIWorktreeFlag` cleanup | `cleanupCodexWorktreeScenario` checks checkout removal; the two classified direct Git sites at lines 502 and 610 are the later Story 004 removal target | `inference/codex/worktree_workstation_test.go:498,502,610` |

### Script: S-01–S-09

| ID | Exact executable witness | Assertion / cleanup proof | Source key |
| --- | --- | --- | --- |
| S-01 | `TestScriptWorkerSharedSuccessSpine/PrimaryResult` | Shared spine assertions check exact output, done Work, accepted dispatch, and request/trace correlation | `script/shared_spine_test.go:40,45,233` |
| S-02 | `TestScriptWorkerSharedSuccessSpine/NoInferenceEvents` | Shared spine scenario checks no inference Events are invented for a script route | `script/shared_spine_test.go:239` |
| S-03 | `TestScriptWorkerSharedSuccessSpine/EdgeAlignment` | Shared spine command assertion checks authored command, args, WorkDir, and environment edge contract | `script/shared_spine_test.go:246` |
| S-04 | `TestScriptWorkerSharedSuccessSpine/NonZeroExit` | `assertScriptNonZeroExit` checks failed Work/dispatch and stderr detail | `script/execution_test.go:50`; `script/shared_spine_test.go:272` |
| S-05 | `TestScriptWorkerSharedSuccessSpine/FailureReachesWorkShow` | `assertScriptFailureReachesWorkShow` checks safe human/JSON failure detail without host-secret leakage | `script/execution_test.go:68` |
| S-06 | `TestScriptWorkerSharedSuccessSpine/Cancellation` | `cancelScriptSharedSessionAfterCommandStart` and cancellation assertions check accepted cancel, canceled dispatch/Event, context termination, and no false result | `script/execution_test.go:86,102` |
| S-07 | `TestScriptWorkerSharedSuccessSpine/DeclaredEnvironmentOnly` | `assertScriptSharedDeclaredEnvironment` checks declared value passes and undeclared host value is absent | `script/environment_test.go:39,100` |
| S-08 | `TestScriptWorkerSharedSuccessSpine/{MissingExecutable,WorktreePassthrough}` | Environment assertions check stable not-found failure and exact worktree WorkDir/command behavior | `script/environment_test.go:65,79` |
| S-09 | `functionallong: {TestScriptWorkerDropsResourceTokensFromArgTemplates,TestScriptWorkstationDropsResourceTokensFromPromptTemplates,TestScriptWorkerOrdersMultipleInputsByWorkstationConfigWithResources,TestScriptWorkstationOrdersMultipleInputsByWorkstationConfigWithResources}` | Long rows check resource-token removal and configured input ordering for worker and workstation templates | `script/environment_long_test.go:21,65,107,137` |

### CLI help: H-01–H-09

| ID | Exact executable witness | Assertion / cleanup proof | Source key |
| --- | --- | --- | --- |
| H-01 | `TestCLIRunHelpShowsInvocationSignatureForNamedFactory` | Checks selected identity, usage, required/optional arguments, example, and absence of generic help | `help/invocation_help_test.go:25` |
| H-02 | `TestCLIRunHelpShowsInvocationSignatureForNamedFactory` duplicate `--help`; `TestCLIRunHelpCoversGenericAndExplicitFactorySelections` repeated generic help | Both tests compare output and diagnostics byte-for-byte across repeated execution | `help/invocation_help_test.go:25,220` |
| H-03 | `TestCLIRunHelpDistinguishesRequiredAndOptionalParameters` | Checks required unbracketed parameter, optional bracketed parameters, exact choices/default/example rendering | `help/invocation_help_test.go:87` |
| H-04 | `TestCLIRunHelpDoesNotDispatchExternalWork` valid selection | Checks zero provider calls and no Work/dispatch for read-only help | `help/invocation_help_test.go:131` |
| H-05 | `TestCLIRunHelpDoesNotDispatchExternalWork` named/factory conflict | Checks stable safe CLI error, empty stdout, and zero provider calls | `help/invocation_help_test.go:131` |
| H-06 | `TestCLISessionHelpPublishesRunnablePlacementExamples` | Checks runnable remote/run examples and absence of invalid `--wait`/`--follow` examples for session/pause/resume help | `help/invocation_help_test.go:189` |
| H-07 | `TestCLIRunHelpCoversGenericAndExplicitFactorySelections` | Checks canonical generic baseline and equivalent named/explicit Factory signatures with correct selector identity | `help/invocation_help_test.go:220` |
| H-08 | `TestCLIRunHelpResetsEmptyAndInvalidSelections` empty then full selection | Checks empty help has no stale args/examples and later full help is complete on the same process | `help/invocation_help_test.go:266` |
| H-09 | `TestCLIRunHelpResetsEmptyAndInvalidSelections` missing/malformed selections | Checks safe failure, empty/no fabricated help, and zero provider calls | `help/invocation_help_test.go:266` |

The ledger contains exactly `19 + 14 + 14 + 8 + 9 + 9 + 9 = 82` rows. No row
was unmapped, and no assertion was deleted or weakened as part of this story.

## Source-hash map

The source reference in each ledger row resolves to one of the following
current-HEAD Git blob hashes. Hashes were collected with
`git hash-object -- <path>` after the read-only verification commands. This
map makes a future characterization contradiction explicit instead of allowing
a changed helper to masquerade as the same witness.

| Source path | Git blob hash |
| --- | --- |
| `tests/functional/workers/mock/agy_named_mock_test.go` | `778611c7057212e46beef7451834d2085317f627` |
| `tests/functional/workers/mock/classifier_test.go` | `2bfac999a43968d8bc308a0b7873001b21e94f18` |
| `tests/functional/workers/mock/replacement_test.go` | `ad3d588c3903730aa1e1ee5b88763ce29097827b` |
| `tests/functional/workers/mock/root_build_test.go` | `3d9012a1df8e40d86479cecda1b5e3fe44923930` |
| `tests/functional/workers/mock/service_config_override_alignment_test.go` | `df5eaec194705f36e57122bf55f4befe276c14d2` |
| `tests/functional/workers/mock/artifact_registry_test.go` | `88765a8d09d6683457b4dd0c07ee140f9f7b5339` |
| `tests/functional/workers/mock/usage_costs_test.go` | `c676ebd8e317df00403b2fe10e36dd8be052c50a` |
| `tests/functional/workers/mock/shared_process_test.go` | `7f51c10f3c8c139890f3d9a4e36bae8a753f4afd` |
| `tests/functional/workers/mock/batch_invocation_shared_test.go` | `58f8341cc60617e75dbf103c549a5a48d400b5b2` |
| `tests/functional/workers/mock/plain_batch_drain_test.go` | `1e7b3a63ea49e57cddc9c850750b8f5c4b4e1ad5` |
| `tests/functional/workers/mock/live_capacity_javascript_test.go` | `53519b757459572668ea0debc8077e249ae830c4` |
| `tests/functional/workers/mock/live_capacity_test.go` | `62606b1ad88fc15b07905359b9094de9b9776961` |
| `tests/functional/workers/mock/batch_invocation_exit_codes_test.go` | `a7294a97c2bb5fd7302f6cd85fed5192642b716e` |
| `tests/functional/workers/mock/named_invocation_exit_codes_test.go` | `072286a529e4faf67ff286ea7bec7c7d4b3a437b` |
| `tests/functional/workers/mock/javascript_acp_test.go` | `508b5e5887a3545893583c84a4ef9922126be6eb` |
| `tests/functional/workers/mock/compiled_cli_exit_codes_helpers_test.go` | `178840888d01c6e7d207014cec29225b9df7fcda` |
| `tests/functional/workers/mock/gate_timeout_test.go` | `aef94478f6e7c1ea6ad10f8b6aa8d1f2618c7a6c` |
| `tests/functional/workers/agent/shared_process_test.go` | `fbe23f6b272aa580d2895a6d0958437c68f4904c` |
| `tests/functional/workers/agent/shared_process_lifecycle_test.go` | `8b5af1d882f7b25db2733c4f75582a703dab5651` |
| `tests/functional/workers/agent/shared_process_assertions_test.go` | `0741325194b7dfab2720985bf0700f0dc6f35225` |
| `tests/functional/workers/agent/shared_process_cleanup_test.go` | `915be7e1862f283c107e898650e512edee6b9e0d` |
| `tests/functional/workers/agent/shared_process_specs_test.go` | `d3511868f0ce58e384cadc143ce0c87ff2d31d6b` |
| `tests/functional/workers/concurrency/shared_process_test.go` | `3fbae0413e0a4784eb82b94067a35f40db7bddde` |
| `tests/functional/workers/invoke_continue/worker_sessions_invoke_continue_test.go` | `a356f70a256d56edd85c2fa15044fc8dc53b5934` |
| `tests/functional/workers/invoke_continue/manager_scenario_test.go` | `4421b1f2d1fc70d1087c1f21c60e760e18c0053e` |
| `tests/functional/workers/invoke_continue/manager_interrupt_scenario_test.go` | `1cec4a01a6885c48dfbc1676e07da371e9826d0e` |
| `tests/functional/workers/invoke_continue/forced_cleanup_test.go` | `f1551c15c5732be1b9fb426bbacc5c3ec5efe14a` |
| `tests/functional/workers/invoke_continue/package_fixture_setup_test.go` | `539088fc1c41e3aa2cac4fbd8825eab06ef61994` |
| `tests/functional/workers/inference/claude/conductor_test.go` | `3e7972f8ec6c3234a7a33275fc3813b056edeefe` |
| `tests/functional/workers/inference/claude/conductor_scenarios_test.go` | `f516dda43c4a2be5ede9121c200badf4dc3a8493` |
| `tests/functional/workers/inference/claude/conductor_fixture_test.go` | `6d60b8d7c6f0f02cab04e4ee2eaea173e97cafa6` |
| `tests/functional/workers/inference/claude/golden_success_test.go` | `5816945aeda341126fd4334a28a821654cd6d89e` |
| `tests/functional/workers/inference/claude/conductor_forced_cleanup_test.go` | `aca040d41a7cf71b8ddecd313fe4b309fbeda920` |
| `tests/functional/workers/inference/claude/conductor_router_test.go` | `3ffa57f299b5c5fc73cf9115bf5fa7b0a098d176` |
| `tests/functional/workers/inference/codex/conductor_test.go` | `5bb4a5ca135603c1873fa6a2b590c5881b915af0` |
| `tests/functional/workers/inference/codex/conductor_scenarios_test.go` | `49f8359350f17df1e5fcd6cbd4eb10a4e0eae7dc` |
| `tests/functional/workers/inference/codex/package_fixture_test.go` | `3d7df335d3c400e94babb45ac1e5142b7bbaa4e9` |
| `tests/functional/workers/inference/codex/golden_success_test.go` | `b7dd08f220aa5bd6fea28ef69dccad9d1306a188` |
| `tests/functional/workers/inference/codex/golden_failure_test.go` | `98ee0916b67b03034379d08f5d61a870452a7ec4` |
| `tests/functional/workers/inference/codex/worktree_workstation_test.go` | `b82a5957be3952a4d1120239f3b52be2222c2ec4` |
| `tests/functional/workers/inference/codex/conductor_router_test.go` | `7f1ce0421cab57173ffe18a98a3b057b98809361` |
| `tests/functional/workers/script/shared_spine_test.go` | `7372fb8cd724df5dae090fdadd5ec86dfd2982ea` |
| `tests/functional/workers/script/execution_test.go` | `00f3ce287c39977631fd82ef8299aba2a907ec5d` |
| `tests/functional/workers/script/environment_test.go` | `07fa38f3b3f169c4927fe762f1bbf7a0a2e155e7` |
| `tests/functional/workers/script/environment_long_test.go` | `f0150e9ad8e6e5d3d8dd0b7c5954ee71cd0c5013` |
| `tests/functional/workers/transports/cli/run/help/package_fixture_test.go` | `52991f4147a45606f7ad446c5e1a2abe802d7402` |
| `tests/functional/workers/transports/cli/run/help/invocation_help_test.go` | `1d7f4a0f2b1b31b17c414f6e1e367e92c8c862c0` |

## Verification and remaining edges

| Evidence | Procedure and observed result | Property proved | Remaining unproven edge / owner |
| --- | --- | --- | --- |
| Baseline CI artifact | `gh run view 33337769566 ...`; `gh run download 33337769566 --name functional-test-diagnostics --dir <temp>`; artifact was complete and successful | Exact pre-change coverage, outcomes, and seven package timings | New-head timings, coverage, and CI status; Stories 002–006 |
| Selector compilation | Seven `go test <package> -list '^Test' -count=0` commands above; exit `0` and all 25 default top-level selectors listed | Current executable selector inventory has no missing default witness | Runtime parity after each structural edit; owning story gates |
| OS boundary census | `make functional-os-boundary-check`; exit `0`, observed/baseline `70/70`, `62` intentional, `8` accidental | No Story 001 OS edit; all eight target sites have identity, property, count, and cleanup owner | Removal and replacement proof for M-19 and X-09; Stories 002/004 |
| Assertion/source map | 82 PRD IDs were reconciled to selectors/helpers and 47 current-HEAD blob hashes | No unmapped row or contradictory source hash at characterization time | Post-change assertion parity and clean-room matrix; Story 006 |

No focused behavior selector was run in this story because the selector census
found no missing witness. The historical run is the dependency-faithful
behavior result for the unchanged baseline; later stories must rerun affected
functional selectors and their declared gates after making code changes.
