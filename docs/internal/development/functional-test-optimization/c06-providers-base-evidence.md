# C06 base-provider characterization ledger

## Status and scope

- Story: `functional-test-optimization-c06-providers-base-001`
- Gate: `PROV-CHAR-001`
- Status: **PASS with the declared retained baseline diagnostic**
- Recorded against: `67710223e327d02c0de93a6ad826c754fe5c1702`
- Scope: every Go test source directly under `tests/functional/providers/`;
  provider subpackages are excluded.
- Dependency fidelity: controlled command-runner edges plus the existing local
  real process, executable, filesystem, replay, environment, and logging edges
  already exercised by the tests.
- Contract/configuration status: no OpenAPI, CLI grammar, Factory Event,
  persisted schema, product configuration, generated output, or shared-support
  change was made.

This is the pre-migration ledger. It proves the current source inventory,
public witnesses, classification, isolation reasons, and static process
topology. It does not claim migrated parity, new-fixture cleanup, a passing
package, or CI timing improvement.

## Discovery and source reconciliation

The direct package contains 16 Go files: 11 scenario files, two OS-specific
process helper files, two helper files, and `doc.go`. The source audit found:

- 35 `func Test...` definitions when the `functionallong` files are included;
- 31 default-build top-level test identities from Go test listing;
- three grouping tests expanded into eight named behavioral subcases:
  `3 + 2 + 3` for CASE-015..017, CASE-034..035, and CASE-038..040;
- 40 pre-migration behavioral rows after expanding those groupings; and
- no missing or duplicate row relative to the PRD Section 10 matrix.

The c01 P027 inventory also records 31 default-build top-level identities.
The current `go test -list` output matches that identity set. The four
`functionallong` definitions are CASE-018, CASE-019, CASE-020, and CASE-024.
The OS-specific helper functions are support code, not additional scenario
rows. CASE-023 and CASE-033 are intentionally retained executable child
entrypoints and therefore are scenario rows.

### Discovery procedure

1. `go test ./tests/functional/providers -list '^Test'`
   - Exit: `0`
   - Observed: 31 top-level test identities; package listing completed in
     `0.038s`.
2. Direct-file source trace equivalent to the task packet’s `rg` procedure:
   `rg -n '^(func Test|\s*t\.Run\()' tests/functional/providers -g '*.go'`
   with the two OS/helper files inspected as direct files. This ripgrep build
   does not provide `--max-depth`; direct-file enumeration was used to exclude
   provider subdirectories.
   - Exit: `0`
   - Observed: 35 test definitions and the seven syntactic `t.Run` sites;
     inspection of the table-driven service runner expands those sites to the
     eight named subcases recorded above.
3. Each matched body and the c01 P027 rows were inspected. Existing assertions
   use public Work, Factory Event, Factory Session, replay, provider-edge,
   stdout/stderr, runtime-log, timeout, PID, and filesystem observations. No
   row depends only on an internal engine snapshot or an unrecorded call count,
   so no new characterization test is required for this story.

## Pre-migration witness matrix

The `Class` column is the migration decision for the later stories. A
`shareable-with-mock` row uses the controlled command/provider edge and may move
to the package-scoped explicit-session fixture. An `isolated-with-reason` row
retains its own process or child boundary. CASE-038 and CASE-039 may form one
dedicated identical-policy logging cohort, but are not part of the general
shared cohort. CASE-040 is a direct runtime-artifact boundary and constructs no
Factory application.

| Case | Source test (file:line) | Class | Given -> when -> then public witness |
| --- | --- | --- | --- |
| CASE-001 | `cli_script_executor_test.go:18` `TestScriptExecutor_Success` | shareable-with-mock | Script Work with payload and a successful controlled runner -> the session runs -> Work is `task:done` and dispatch output is `script-output-ok`. |
| CASE-002 | `cli_script_executor_test.go:28` `TestScriptExecutor_Failure` | shareable-with-mock | Script Work and runner exit 1 with stderr `script broke` -> the session runs -> Work is `task:failed` and no done Work exists. |
| CASE-003 | `cli_script_executor_test.go:39` `TestScriptExecutor_CommandCancellationIsReported` | shareable-with-mock | Script runner returns `context.Canceled` -> the session runs -> failed Work and dispatch error `execution cancelled: context canceled` are visible. |
| CASE-004 | `cli_script_executor_test.go:51` `TestScriptExecutor_MissingCommandFailsStartup` | isolated-with-reason | Worker configuration omits the command -> the isolated invocation runs -> startup reports `construct script worker` and `misconfigured`, with no dispatch. |
| CASE-005 | `cli_script_executor_test.go:77` `TestScriptExecutor_InvalidWorkstationTemplateFailsBeforeCommand` | shareable-with-mock | Workstation prompt template is invalid -> Work is scheduled -> runner calls stay zero, Work fails, and dispatch reports prompt-render failure. |
| CASE-006 | `cli_script_executor_test.go:97` `TestScriptExecutor_PreservesTokenColor` | shareable-with-mock | Seed has payload but no explicit Work or trace identity -> successful output replaces content -> done Work retains type/state and dispatch output is `new-payload`. |
| CASE-007 | `cli_script_executor_test.go:108` `TestScriptExecutor_SuccessWithColorMetadata` | shareable-with-mock | Seed has Work ID, trace ID, payload, and two tags -> successful output is applied -> IDs/tags and terminal state remain, with exact dispatch output. |
| CASE-008 | `cli_script_executor_test.go:127` `TestScriptExecutor_FailureRoutesToFailedPlace` | shareable-with-mock | Runner exits with `script-error-output` -> Work executes -> failed Work and the exact stderr-derived dispatch error are visible. |
| CASE-009 | `cli_script_executor_test.go:137` `TestScriptExecutor_ArgTemplating` | shareable-with-mock | Worker args reference input name and Work ID -> Work executes -> provider request/output contains `prd-my-feature` then `work-abc-123` in order. |
| CASE-010 | `cli_script_executor_test.go:160` `TestScriptExecutor_WorkTypeIDFromTargetPlace` | shareable-with-mock | Work targets `task` -> output transitions to done -> listed Work type remains `task` with original Work/trace IDs. |
| CASE-011 | `cli_script_executor_test.go:176` `TestScriptExecutor_ArgTemplatingWithTags` | shareable-with-mock | Args reference `env` and `team` tags -> Work executes -> dispatch output is `staging` then `infra`. |
| CASE-012 | `cli_script_executor_test.go:199` `TestScriptExecutor_RuntimeWorkstationConfigResolvesWorkingDirectoryAndEnv` | shareable-with-mock | Workstation templates working directory and command environment from tags -> Work executes -> captured request has the resolved path and exact `TEAM`/`BRANCH` values. |
| CASE-013 | `cli_script_executor_test.go:241` `TestScriptExecutor_RuntimeConfigMergePreservesCanonicalTopologyAndPromptTemplates` | shareable-with-mock | Inline and AGENTS workstation shapes conflict -> Work executes -> runtime topology/output, prompt args, worktree, environment, and dispatch output follow existing precedence. |
| CASE-014 | `cli_script_executor_test.go:313` `TestScriptExecutor_RuntimeWorkstationTimeoutRequeuesAndRetriesOnLaterTick` | shareable-with-mock | First controlled workstation attempt blocks past 10ms and the second succeeds -> scheduler ticks -> dispatches are FAILED then ACCEPTED with timeout text, one done Work, and at least two calls. |
| CASE-015 | `cli_script_executor_test.go:342` `.../SingleFileInputWithTemplateAndPayload_Completes` | shareable-with-mock | One file input has prompt template and payload -> long fallback case runs -> done Work and `template-case-ok` dispatch are visible. |
| CASE-016 | `cli_script_executor_test.go:353` `.../NoTemplateWithPayload_Completes` | shareable-with-mock | Input has payload and no prompt template -> long fallback case runs -> done Work and `payload-only-ok` dispatch are visible. |
| CASE-017 | `cli_script_executor_test.go:363` `.../NoTemplateAndNoPayload_Completes` | shareable-with-mock | Input has neither prompt template nor payload -> long fallback case runs -> done Work and `empty-input-ok` dispatch are visible. |
| CASE-018 | `cli_script_executor_timeout_long_test.go:16` `TestScriptExecutor_RuntimeWorkerTimeoutFromLoadedConfigRequeuesAndRetriesOnLaterTick` | shareable-with-mock (tagged long) | Loaded worker timeout expires the first attempt -> a later scheduler tick retries -> FAILED then ACCEPTED dispatches, timeout text, done Work, and at least two calls remain. |
| CASE-019 | `cli_template_resolution_long_test.go:16` `TestTemplateTests_ScriptWrapClaudeResolvesWorkstationExecutionTemplates` | shareable-with-mock (tagged long) | Claude worker and execution templates are configured -> the long provider route executes -> exact Claude args, empty stdin, workdir/env, and terminal Work are observed. |
| CASE-020 | `cli_template_resolution_long_test.go:42` `TestTemplateTests_ScriptWrapCodexResolvesWorkstationExecutionTemplates` | shareable-with-mock (tagged long) | Codex worker and execution templates are configured -> the long provider route executes -> exact Codex args/stdin, workdir/env, and terminal Work are observed. |
| CASE-021 | `cli_timeout_cleanup_smoke_test.go:20` `TestIntegrationSmoke_TimeoutCancelsProcessTreeAndClearsActiveExecution` | isolated-with-reason | Real test executable spawns a descendant and exceeds timeout -> isolated Factory cancels it -> child PID exits, Work fails, and one FAILED timeout dispatch remains. |
| CASE-022 | `cli_timeout_cleanup_smoke_test.go:83` `TestIntegrationSmoke_TimeoutRequeuesWorkAndSucceedsOnLaterAttempt` | isolated-with-reason | Real executable times out once and records an attempt -> isolated scheduler retries -> done Work retains IDs, FAILED then ACCEPTED dispatches remain, and the process closes. |
| CASE-023 | `cli_timeout_cleanup_smoke_test.go:124` `TestIntegrationSmoke_ProcessTreeHelper` | isolated-with-reason | Test binary receives a supported child-helper mode -> child entrypoint runs -> PID/attempt file and exit behavior match the mode, while ordinary invocation is inert. |
| CASE-024 | `cli_timeout_companion_smoke_long_test.go:25` `TestIntegrationSmoke_ScriptTimeoutCompanionRequeuesBeforeLaterCompletion` | shareable-with-mock (tagged long) | First attempt times out and retry blocks on an explicit release signal -> signal releases the second attempt -> retry follows timeout, Work completes once, dispatch order remains, and calls are at least two. |
| CASE-025 | `mock_workers_agent_test.go:17` `TestMockWorkers_AgentDefaultAcceptMovesWorkToOutputPlace` | shareable-with-mock | Default mock model worker is enabled -> Work runs -> one done Work and zero initial Work remain. |
| CASE-026 | `mock_workers_agent_test.go:35` `TestMockWorkers_AgentRejectConfigRoutesFailureWithoutLoggingCommandOutput` | shareable-with-mock | Reject config uses exit 7 and stdout/stderr -> Work runs -> failed Work and permanent refusal event are visible; log has exit 7 without stdout/stderr. |
| CASE-027 | `mock_workers_agent_test.go:65` `TestMockWorkers_AgentRejectConfigWithZeroExitCodeIsRejectedAtCustomerBoundary` | isolated-with-reason | Model reject config uses exit 0 -> isolated `you run` starts -> customer boundary rejects `exitCode` outside 1..255 and no runtime starts. |
| CASE-028 | `mock_workers_end_to_end_smoke_test.go:21` `TestMockWorkers_EndToEndSmokeRunsMixedOutcomesWithoutLiveProviderCredentials` | isolated-with-reason | Accept, reject, and script Work run with recording -> artifact is replayed in a dedicated process -> three requests/completions, exact mixed outcomes/output/failure, child side effect, empty quiet stdout, and replayed Work states are visible. |
| CASE-029 | `mock_workers_script_test.go:19` `TestMockWorkers_ScriptDefaultAcceptProducesSuccessfulScriptResult` | shareable-with-mock | Default mock script worker is enabled -> Work runs -> done Work contains `mock worker accepted`. |
| CASE-030 | `mock_workers_script_test.go:33` `TestMockWorkers_ScriptRejectConfigRoutesFailureAndLogsCommandOutput` | shareable-with-mock | Script reject uses exit 9 and configured stderr -> Work runs -> failed Work/event includes stderr; log has exit 9 without stdout/stderr. |
| CASE-031 | `mock_workers_script_test.go:64` `TestMockWorkers_ScriptRejectConfigWithZeroExitCodeStillRoutesFailure` | isolated-with-reason | Script reject config uses exit 0 -> isolated `you run` starts -> customer boundary rejects `exitCode` outside 1..255. |
| CASE-032 | `mock_workers_script_test.go:81` `TestMockWorkers_ScriptConfigExecutesCommandRunnerSideEffect` | isolated-with-reason | Mock script points to the test executable `write-file` helper -> isolated Work runs -> file has exact content, done Work has helper stdout, and the child exits. |
| CASE-033 | `mock_workers_script_test.go:160` `TestMockWorkers_ScriptHelper` | isolated-with-reason | Test binary receives `write-file` or `sleep-write-file` helper args -> child entrypoint runs -> it writes exact content/stdout, or ordinary invocation is a no-op. |
| CASE-034 | `mock_workers_service_runner_test.go:26` `.../model worker` | shareable-with-mock | Continuously hosted service uses default mock model worker -> explicit-session Work runs -> one terminal non-failed Work exists and completion log omits stdout/stderr. |
| CASE-035 | `mock_workers_service_runner_test.go:27` `.../script worker` | shareable-with-mock | Continuously hosted service uses default mock script worker -> explicit-session Work runs -> one terminal non-failed Work exists and completion log omits stdout/stderr. |
| CASE-036 | `packaged_script_runtime_test.go:23` `TestPackagedScriptRuntime_FreshInstallExecutesFactoryRelativeScript` | isolated-with-reason | Unix Factory-relative shebang script exists and is executable -> isolated fresh install runs -> complete Work text is `packaged runtime success`; Windows skips before construction. |
| CASE-037 | `packaged_script_runtime_test.go:59` `TestPackagedScriptRuntime_NonZeroExitUsesStandardFailureOutcome` | isolated-with-reason | Unix Factory-relative script writes stderr and exits 23 -> isolated fresh install runs -> Work fails and response is FAILED_EXIT_CODE/23 with empty stdout, exact stderr, and no failure type; Windows skips before construction. |
| CASE-038 | `runtime_logging_smoke_test.go:52` `.../SuccessSuppressesSystemOutputAndRecordsEnvDiagnostics` | isolated-with-reason (dedicated logging cohort) | Synthetic process env and rolling policy are configured -> successful logging session runs -> done Work, structured log suppression/policy/env, and exact replay output are observed without duplicate env diagnostics. |
| CASE-039 | `runtime_logging_smoke_test.go:82` `.../FailureSuppressesSystemOutputAndRecordsEnvDiagnostics` | isolated-with-reason (dedicated logging cohort) | Same environment runner returns stdout/stderr/23 -> failure session runs in the dedicated logging process -> failed Work, exact status/exit/output suppression, replay error, and policy are observed. |
| CASE-040 | `runtime_logging_smoke_test.go:115` `.../ExplicitTelemetryPolicyProducesStructuredArtifacts` | retained direct-artifact boundary | Explicit log/metric policy and compaction record are created -> writers close twice/once as supported -> configs, correlated fields, idempotent metrics close, and artifact JSON are exact; no Factory application is constructed. |

## Retained isolation decisions

Each row below has an individual reason that sharing the general healthy host
would weaken the property it owns. These are decisions, not post-migration
results.

| Case | Test | Exact reason sharing would weaken the property |
| --- | --- | --- |
| CASE-004 | `TestScriptExecutor_MissingCommandFailsStartup` | Proves invocation startup rejects malformed worker configuration before dispatch; an already healthy shared host changes the startup boundary. |
| CASE-021 | `TestIntegrationSmoke_TimeoutCancelsProcessTreeAndClearsActiveExecution` | Proves real descendant creation, timeout propagation, process-tree termination, and shutdown cleanup through the real executable runner. |
| CASE-022 | `TestIntegrationSmoke_TimeoutRequeuesWorkAndSucceedsOnLaterAttempt` | Proves a real child executable is terminated on timeout and a later invocation recovers using process-owned attempt state. |
| CASE-023 | `TestIntegrationSmoke_ProcessTreeHelper` | Is the child executable entrypoint for process-tree/PID/attempt behavior; it must execute in a child and remain inert in the parent package run. |
| CASE-027 | `TestMockWorkers_AgentRejectConfigWithZeroExitCodeIsRejectedAtCustomerBoundary` | Proves CLI-global mock configuration validation before runtime activation; invalid config cannot be installed on the healthy shared host. |
| CASE-028 | `TestMockWorkers_EndToEndSmokeRunsMixedOutcomesWithoutLiveProviderCredentials` | Proves incompatible recording and replay process modes, persisted-artifact recovery, and a real script child side effect. |
| CASE-031 | `TestMockWorkers_ScriptRejectConfigWithZeroExitCodeStillRoutesFailure` | Proves the malformed CLI-global script mock configuration boundary before runtime activation. |
| CASE-032 | `TestMockWorkers_ScriptConfigExecutesCommandRunnerSideEffect` | Proves configured mock-script child execution and real filesystem/stdout side effects. |
| CASE-033 | `TestMockWorkers_ScriptHelper` | Is the real child entrypoint used to write, emit, and exit; in-process sharing removes executable fidelity. |
| CASE-036 | `TestPackagedScriptRuntime_FreshInstallExecutesFactoryRelativeScript` | Proves Unix shebang permission and Factory-relative executable selection with the real exec runner. |
| CASE-037 | `TestPackagedScriptRuntime_NonZeroExitUsesStandardFailureOutcome` | Proves real shell exit-code and stderr mapping through the real exec runner. |
| CASE-038 | `.../SuccessSuppressesSystemOutputAndRecordsEnvDiagnostics` | Proves process-level environment capture, runtime-log/record destinations, and rolling policy fixed at startup; it may share one dedicated identical-policy logging process with CASE-039 but not the general cohort. |
| CASE-039 | `.../FailureSuppressesSystemOutputAndRecordsEnvDiagnostics` | Proves the same process-level environment, runtime-log/record destinations, and startup rolling policy on the failure path; it may share only the dedicated identical-policy logging cohort. |

CASE-040 is listed separately in the matrix because it is a direct platform
artifact test, not a process-sensitive Factory scenario. Its temporary log and
metrics resources are owned by the test and remain outside the shared root
topology.

## Cleanup rows to prove in later stories

These five rows are recorded now as required cleanup witnesses. They are not
claimed as current pre-migration evidence and are owned by the later fixture
stories.

| Row | Given -> when -> then | Owner | Current status |
| --- | --- | --- | --- |
| CLEAN-001 | Successful shared scenario completes -> scenario cleanup runs -> session is deleted, route removed, and temporary Factory/worktree paths are absent. | story 002 | PASS: shared topology and known-good recovery scenarios delete sessions, remove routes, and verify owned paths absent. |
| CLEAN-002 | Provider or runtime failure completes -> cleanup runs -> session/route/path census is zero and the process remains healthy. | story 002 | PASS: invalid-template, dependency-failure, timeout, cancellation, and unknown-route scenarios clean up before the next known-good session succeeds. |
| CLEAN-003 | Controlled or real timeout returns -> cleanup runs -> active execution is zero, child PID is gone, and session/route/path are removed. | story 003 | Not yet exercised; retained-process proof. |
| CLEAN-004 | Context cancellation is observed -> cleanup runs -> no active execution/session/route/path remains and the next scenario succeeds. | story 002 | PASS: shared adverse-recovery cancellation subcase is followed by a known-good session; final fixture census is empty. |
| CLEAN-005 | Helper subtest intentionally fails after acquiring process/session/route/temp resources -> parent observes non-zero child exit -> cleanup callbacks run and listener/PID/session/route/path/worktree census is empty. | story 003 | Not yet exercised; forced-assertion-failure proof. |

## Before-migration process topology

Static source tracing gives the following topology for the current fresh-process
setup. A root construction means `support.BuildProcess` directly or a helper
that constructs it. A listener start means an `APIServerStarter` is activated;
the three malformed-startup boundary rows and the record phase of CASE-028
construct a process without opening the loopback listener.

| Cohort | Root/application constructions | Loopback listener starts | Source basis |
| --- | ---: | ---: | --- |
| Default Linux | 34 | 30 | All default rows except the four long rows; CASE-036/037 execute their Unix real-executable cells. |
| Default Windows | 32 | 28 | The two shebang rows skip before application construction, leaving the same 30-to-28 listener relationship. |
| `functionallong` additions | 4 | 4 | CASE-018, CASE-019, CASE-020, and CASE-024 each add a fresh root/listener. |

The current topology has no package-scoped root, no package-scoped listener,
and no explicit per-scenario session/route registry. Most successful rows use
the implicit default Factory Session; process-sensitive rows use their own
fresh invocation or child boundary. The target counts and explicit-session
topology are owned by `PROV-MIG-002`, not this characterization story.

## Baseline diagnostic

The PRD’s retained reference observation is preserved verbatim as a diagnostic:

- Command: `go test -count=1 -timeout=10m ./tests/functional/providers`
- Reference result: exit `1`, `43.512s`
- Failure: `TestMockWorkers_EndToEndSmokeRunsMixedOutcomesWithoutLiveProviderCredentials`
- Unchanged assertion: `mock_workers_end_to_end_smoke_test.go:77` observed
  `reject-process failure reason = "unknown"`, while the assertion requires
  `permanent_bad_request`.
- Related runtime diagnostic: the Claude mock exited 13 with
  `failure_reason=non_zero_exit`, while the transition response reported
  `failure_reason=unknown`.

The same command was rerun for this characterization on `go1.25.0`
`windows/amd64` at the recorded head:

- Observed result: exit `1`, package duration `42.091s`.
- Observed failure: the same test and the same `unknown` versus
  `permanent_bad_request` assertion.
- Interpretation: the mismatch is retained unchanged and is not a
  characterization pass or a timing threshold. The local duration is
  diagnostic only; PR package CI owns the later directional timing result.

No credentials, remote provider calls, customer data, or paid calls were used.
Temporary test paths were test-owned and were managed by the existing test
cleanup. This story does not claim the stronger post-migration resource census.

## PROV-CHAR-001 evidence record

- Scope: functional characterization of the direct base-provider package,
  including default, `functionallong`, OS-specific, helper, named-subcase, and
  direct artifact cells.
- Procedure: run the discovery commands above; inspect every direct source body
  and c01 P027 row; run the declared package baseline command; compare the
  resulting identities and witnesses with the 40-row matrix.
- Artifact: this ledger at commit
  `67710223e327d02c0de93a6ad826c754fe5c1702` plus the unchanged source tests.
- Property proved: complete one-to-one current inventory, exact public witness
  baseline, retained-isolation rationale, five cleanup expectations, and
  before-migration static topology.
- Property not proved: migrated parity, explicit-session isolation, new-fixture
  cleanup under success/failure/timeout/cancellation/assertion failure,
  `functionallong` post-migration topology, three-run contamination proof,
  package pass despite the retained mismatch, PR CI timing, and remote provider
  behavior.
- Remaining gates: migration and route/session cleanup -> `PROV-MIG-002`;
  process-sensitive and forced-unwind cleanup -> `PROV-ISO-003`; repeat/package
  outcome -> `PROV-REPEAT-004`; tagged long proof -> `PROV-LONG-005`; PR timing
  -> `PR-CI-006`; integrated clean-room report -> `VAL-007`.

## PROV-MIG-002 evidence record

Story 002 migrated the eligible controlled-edge cases to the package-local
shared fixture in `tests/functional/providers/shared_process_test.go`. The
fixture builds one `root.BuildProcess` application, starts one loopback
listener, runs one continuously executing process, and exposes explicit
Factory Session open/submit/read/close/delete helpers. Every controlled
provider request is selected by an immutable cleaned `WorkDir` route; duplicate
selectors are rejected, unknown routes fail without invoking a runner, and
route registrations are removed with their session.

Verification procedures and observations:

1. Shared topology, route isolation, and adverse recovery:
   `go test -count=1 -timeout=10m ./tests/functional/providers -run
   '^TestProvidersSharedProcess(Topology|Routes|AdverseRecovery)$' -v`
   exited `0` in `2.230s`. The topology test ran two explicit sessions
   concurrently with distinct routes and outputs. The route test proved
   duplicate registration rejection, known-route dispatch, and unknown-route
   non-invocation. The recovery test proved invalid-template zero-call
   failure, dependency failure, timeout retry, cancellation, unknown-route
   failure, and known-good recovery.
2. Focused default eligible migration selector:
   `go test -count=1 -timeout=10m ./tests/functional/providers -run
   '^(TestProvidersSharedProcess(Topology|Routes|AdverseRecovery)(/.*)?|TestScriptExecutor_(Success|Failure|CommandCancellationIsReported|InvalidWorkstationTemplateFailsBeforeCommand|PreservesTokenColor|SuccessWithColorMetadata|FailureRoutesToFailedPlace|ArgTemplating|WorkTypeIDFromTargetPlace|ArgTemplatingWithTags|RuntimeWorkstationConfigResolvesWorkingDirectoryAndEnv|RuntimeConfigMergePreservesCanonicalTopologyAndPromptTemplates|RuntimeWorkstationTimeoutRequeuesAndRetriesOnLaterTick|AsyncWorkerPoolTemplateFallbackScenarios)(/.*)?|TestMockWorkers_(AgentDefaultAcceptMovesWorkToOutputPlace|AgentRejectConfigRoutesFailureWithoutLoggingCommandOutput|ScriptDefaultAcceptProducesSuccessfulScriptResult|ScriptRejectConfigRoutesFailureAndLogsCommandOutput|ServiceCommandRunnerCompletesModelAndScriptWorkers)(/.*)?)$'`
   exited `0` in `10.753s`. Public Work states, Factory Events, dispatch
   outputs/errors, controlled provider requests, timeout ordering, mock
   stdout/stderr policy, and service completion logs remained asserted by the
   migrated tests.
3. Tagged-long selector:
   `go test -json -tags=functionallong -count=1 -timeout=30m
   ./tests/functional/providers -run
   '^(TestScriptExecutor_RuntimeWorkerTimeoutFromLoadedConfigRequeuesAndRetriesOnLaterTick|TestIntegrationSmoke_ScriptTimeoutCompanionRequeuesBeforeLaterCompletion|TestTemplateTests_ScriptWrap(Claude|Codex)ResolvesWorkstationExecutionTemplates)(/.*)?$'`
   passed the loaded-worker timeout (`0.78s`) and timeout companion (`0.28s`)
   cases. Claude and Codex template cases each failed at `1.16s` with the
   same blank runtime template context and missing resolved request witness
   already reproduced against the parent/base test implementation; their
   assertions remain unchanged and are not claimed as migrated passes.
4. Full default package:
   `go test -json -count=1 -timeout=10m ./tests/functional/providers`
   exited `1` only at the retained
   `TestMockWorkers_EndToEndSmokeRunsMixedOutcomesWithoutLiveProviderCredentials`
   assertion: observed failure reason `unknown`, expected
   `permanent_bad_request`. The test and assertion are isolated and unchanged;
   the same mismatch is recorded in the baseline diagnostic above.

The shared fixture's finalizer passed its topology census: constructor count
`1`, listener starts `1`, all explicit session IDs opened by the fixture were
deleted, route count `0`, and the fixture-owned root was absent after process
shutdown. Static source tracing after migration leaves `13` default Linux
application constructions (`9` listeners) and `11` Windows constructions
(`7` listeners), with the tagged-long selector using the existing shared
fixture and adding no second construction. No provider credentials, remote
calls, customer data, or paid calls were used.

- Property proved: controlled eligible default behavior is exercised through
  one production root/process/listener with explicit-session isolation,
  immutable routing, adverse-case recovery, and success/failure/cancellation
  cleanup.
- Property not proved: the two known baseline template-resolution cases,
  retained real executable/process-tree/record-replay/environment/assertion-
  failure cells, three-run repeat cleanup, package success past the retained
  mismatch, and PR CI timing.
- Remaining gates: retained isolation -> `PROV-ISO-003`; repeat/package and
  baseline disposition -> `PROV-REPEAT-004`; tagged long parity ->
  `PROV-LONG-005`; PR timing -> `PR-CI-006`; final clean-room report ->
  `VAL-007`.
