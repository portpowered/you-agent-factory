# C14 providers cluster characterization ledger

## Story status and scope

- Story: `fto-c14-pkg-providers-cluster-001` — freeze provider package behavior,
  topology, and intrinsic baseline.
- Status: **PASS for the declared read-only characterization scope, with
  pre-change diagnostic failures retained**.
- Recorded: `2026-08-29` against local `HEAD`
  `fea2e30a499384182d2fabe7038767e3c2f9c5e5`.
- Scope: direct `tests/functional/providers/*.go` and
  `tests/functional/providers/acp/*.go` sources, their existing local-real
  process/stdio/filesystem edges, controlled provider edges, and this ledger.
- No test source, production source, shared support, generated output,
  workflow, Makefile, baseline, contract, or provider-subpackage file was
  changed for this story.
- No operator amendment is present in `prd.json`.

The ledger freezes the current executable spine before structural edits:
`root.BuildProcess` -> `Process.Execute` -> public Factory Session/Work/Event
observations -> controlled provider or retained local-real process/stdio edge
-> deterministic cleanup. It does not claim post-change parity, performance
improvement, clean-room behavior, or CI behavior.

## CHAR-001 procedure and environment

The six baseline samples were run as six separate uncached invocations before
any C14 source edit. The commands were:

```text
go test ./tests/functional/providers -count=1 -timeout=15m
go test ./tests/functional/providers/acp -count=1 -timeout=15m
```

Each command was invoked three times in package order, with wall time measured
around the complete `go test` process and the package elapsed value taken from
the terminal Go test event. A failing invocation still contributes a diagnostic
sample; it is never presented as a behavioral pass.

| Property | Value |
| --- | --- |
| OS | Microsoft Windows 11 Home `10.0.26200` |
| Architecture | `windows/amd64`, x64 |
| CPU | 13th Gen Intel(R) Core(TM) i7-13700K |
| Logical processors | `24` |
| Go | `go1.25.0 windows/amd64` |
| GOMOD | `C:\Users\andre\work\portos\infinite-you\.claude\worktrees\fto-c14-pkg-providers-cluster\go.mod` |
| GOCACHE | `C:\Users\andre\AppData\Local\go-build` |
| GOAMD64 | `v1` |
| GOMAXPROCS / GOFLAGS | unset / unset |
| CGO_ENABLED | `1` |
| Commit | `fea2e30a499384182d2fabe7038767e3c2f9c5e5` |

### Three-run samples

| Package | Sample | Wall seconds | Package seconds | Exit | Terminal observation |
| --- | ---: | ---: | ---: | ---: | --- |
| `providers` | 1 | 68.590 | 38.576 | 1 | Pre-change failure; expected diagnostic/error output was emitted. |
| `providers` | 2 | 76.743 | 72.237 | 1 | Same unchanged package failure class. |
| `providers` | 3 | 58.380 | 52.835 | 1 | Same unchanged package failure class. |
| `providers/acp` | 1 | 152.990 | 147.409 | 1 | Contention/noise-sensitive failure set; retained as diagnostic. |
| `providers/acp` | 2 | 146.612 | 140.804 | 0 | Complete package passed. |
| `providers/acp` | 3 | 124.644 | 119.932 | 0 | Complete package passed. |

The pre-change medians are:

| Denominator | Wall median | Package median |
| --- | ---: | ---: |
| Base `providers` | **68.590s** | **52.835s** |
| ACP `providers/acp` | **146.612s** | **140.804s** |
| Sum of package medians | **215.202s** | **193.639s** |

The base samples all exited 1 at the unchanged mixed mock-worker/replay
assertion identified by the profile below. The first ACP sample exited 1 with
contention-sensitive failures in packaged, redaction, unsupported-capability,
start-failure, and persistent-reuse cells; samples two and three exited 0.
These observations are retained as pre-change diagnostics and are not used to
weaken an assertion, select a timing denominator, or claim package parity.

The hosted estimate from run `33264193119` remains labeled a
**contention-inflated estimate** (approximately 33.3s base and 25.5s ACP). It
is not a denominator for this lane and is not mixed with the isolated local
samples above.

## Source and top-level inventory

The exact pre-edit listing commands exited 0 and reported 35 base and 26 ACP
top-level identities:

```text
go test -list '^Test' ./tests/functional/providers
=> 35 identities; package listing reported ok in 0.065s

go test -list '^Test' ./tests/functional/providers/acp
=> 26 identities; package listing reported ok in 0.079s
```

The identities, including helper entrypoints that must remain inert during
ordinary package discovery, were:

### Base package: 35 identities

```text
TestProvidersSharedProcessTopology
TestProvidersSharedProcessRoutes
TestProvidersSharedProcessAdverseRecovery
TestScriptExecutor_Success
TestScriptExecutor_Failure
TestScriptExecutor_CommandCancellationIsReported
TestScriptExecutor_MissingCommandFailsStartup
TestScriptExecutor_InvalidWorkstationTemplateFailsBeforeCommand
TestScriptExecutor_PreservesTokenColor
TestScriptExecutor_SuccessWithColorMetadata
TestScriptExecutor_FailureRoutesToFailedPlace
TestScriptExecutor_ArgTemplating
TestScriptExecutor_WorkTypeIDFromTargetPlace
TestScriptExecutor_ArgTemplatingWithTags
TestScriptExecutor_RuntimeWorkstationConfigResolvesWorkingDirectoryAndEnv
TestScriptExecutor_RuntimeConfigMergePreservesCanonicalTopologyAndPromptTemplates
TestScriptExecutor_RuntimeWorkstationTimeoutRequeuesAndRetriesOnLaterTick
TestScriptExecutor_AsyncWorkerPoolTemplateFallbackScenarios
TestIntegrationSmoke_TimeoutCancelsProcessTreeAndClearsActiveExecution
TestIntegrationSmoke_TimeoutRequeuesWorkAndSucceedsOnLaterAttempt
TestIntegrationSmoke_ProcessTreeHelper
TestProviders_ForcedAssertionFailureCleansOwnedResources
TestIntegrationSmoke_ScriptTimeoutCompanionRequeuesBeforeLaterCompletion
TestMockWorkers_AgentDefaultAcceptMovesWorkToOutputPlace
TestMockWorkers_AgentRejectConfigRoutesFailureWithoutLoggingCommandOutput
TestMockWorkers_AgentRejectConfigWithZeroExitCodeIsRejectedAtCustomerBoundary
TestMockWorkers_EndToEndSmokeRunsMixedOutcomesWithoutLiveProviderCredentials
TestMockWorkers_ScriptDefaultAcceptProducesSuccessfulScriptResult
TestMockWorkers_ScriptRejectConfigRoutesFailureAndLogsCommandOutput
TestMockWorkers_ScriptRejectConfigWithZeroExitCodeStillRoutesFailure
TestMockWorkers_ScriptConfigExecutesCommandRunnerSideEffect
TestMockWorkers_ScriptHelper
TestMockWorkers_ServiceCommandRunnerCompletesModelAndScriptWorkers
TestPackagedScriptRuntime_FreshInstallExecutesFactoryRelativeScript
TestPackagedScriptRuntime_NonZeroExitUsesStandardFailureOutcome
TestRuntimeLoggingSmoke_SuccessAndFailureRespectOutputEnvAndRollingPolicies
```

The three grouping tests expand to the six adverse shared cases, three input
fallback cases, two service-runner cases, and three logging cases. The four
`functionallong` test definitions are not in the default build, but remain in
the source and are part of the C14 B-018..B-020/B-024 inventory.

### ACP package: 26 identities

```text
TestACPCommandStartFailureMapsToDependencyFailure
TestACPFailureRedactsConfiguredSecretsFromStderr
TestACPProtocolFailuresMapToStableWorkerFailureClasses
TestUnavailableACPExecutableFailsBeforeStartWithMissingExecutableClass
TestFactoryRunRetriesACPProviderByResumingExactSession
TestRootConstructionDoesNotStartACPProcess
TestUnknownExecutorProviderFailsBeforeACPProcessStart
TestACPAgentHelperProcess
TestProvidersACPSerializesConcurrentPromptsOnOneStdioConnection
TestProvidersACPRestartsAfterCrashWithoutReplayingUncertainPrompt
TestProvidersACPRetiresDisconnectedConnectionBeforeReuse
TestProvidersACPRetainsOneOSProcessAndConnectionAcrossExecutions
TestProvidersACPRejectsIncompatibleProtocolVersionAtStdioBoundary
TestProvidersShutdownCancelsActivePromptAndJoinsACPProcess
TestACPFixtureContractRejectsInvalidInvocationScopedData
TestPinnedACPSDKGoldenManifestIsCompleteAndParseable
TestACPGoldenRPCPeerProcess
TestPackagedACPProfilesUseSharedConformanceBehavior
TestPackagedACPUnexpectedCommand
TestPackagedSpawnRunsPlannerChildrenAndMergerThroughPersistentACPStdio
TestPackagedTournamentRunsCompetitorsAndJudgeThroughPersistentACPStdio
TestYouRunMapsGoldenSessionAndConfigRPCFailuresToTerminalWork
TestYouRunUsesPinnedACPWireGoldensAndProjectsTerminalOutput
TestYouRunMapsSkipPermissionsToSDKGoldenPermissionSelection
TestYouRunReturnsUnsupportedFilesystemAndTerminalRPCsAtTheACPBoundary
TestACPSharedProcess
```

The ACP profile recorded 88 pass events, including the current nested profile,
catalog, protocol, permission, and golden expansions. Its longest parent
records were `TestACPSharedProcess` (64.870s), packaged spawn (10.600s),
packaged tournament (10.520s), shutdown (10.460s), serialization (10.370s),
reuse (10.360s), and incompatible negotiation (10.100s).

## Assertion and behavior reconciliation

The C14 matrix is the PRD Section 10 matrix. Every row is a preservation
requirement; the C14 implementation stories do not authorize deleting,
skipping, narrowing, or replacing a distinct assertion. The current C06 base
ledger and C06/C10 ACP ledger were reconciled against the current source and
the matrix below. A contiguous ID range in this ledger includes every ID in
the range.

### Base B-* rows (53 total)

| Matrix IDs | Current source owner / expansion | Frozen observable categories |
| --- | --- | --- |
| B-001..B-014 | `cli_script_executor_test.go`; individual script cases plus the shared timeout case | Work state, dispatch output/error, cancellation, bad input, zero-call template failure, IDs/tags, argument order, type identity, WorkDir/env, merge precedence, timeout/retry order. |
| B-015..B-017 | `TestScriptExecutor_AsyncWorkerPoolTemplateFallbackScenarios/{SingleFileInputWithTemplateAndPayload_Completes,NoTemplateWithPayload_Completes,NoTemplateAndNoPayload_Completes}` | Three exact input-shape fallback outputs and terminal Work states. |
| B-018..B-020 | `cli_timeout_long_test.go` / `cli_template_resolution_long_test.go` under `functionallong` | Loaded timeout retry; exact Claude/Codex args, stdin, WorkDir/env, and terminal Work. |
| B-021..B-023 | `cli_timeout_cleanup_smoke_test.go` and its process helper | Real child/descendant PID, timeout cancellation, retry-owned attempt state, exit status, and inert helper discovery. |
| B-024 | `cli_timeout_companion_smoke_long_test.go` | Timeout then explicit release, retry order, exact completion, and minimum call count. |
| B-025..B-027 | `mock_workers_agent_test.go` | Default mock model success, configured refusal/public event/log suppression, and customer-boundary exit-code validation. |
| B-028 | `mock_workers_end_to_end_smoke_test.go` | Three recorded requests/completions, mixed outcomes, real script side effect, quiet stdout, and replayed Work states. Its unchanged refusal-class mismatch is the base diagnostic. |
| B-029..B-033 | `mock_workers_script_test.go` and script helper | Default script result, stderr failure/log policy, invalid exit-code boundary, real file/stdout child side effect, and inert helper mode. |
| B-034..B-035 | `mock_workers_service_runner_test.go/{model worker,script worker}` | Explicit-session hosted model/script completion and omission of command output from completion logs. |
| B-036..B-037 | `packaged_script_runtime_test.go` | Unix shebang/factory-relative success and non-zero exit/stderr mapping; Windows skip-before-construction behavior. |
| B-038..B-040 | `runtime_logging_smoke_test.go/{success,failure,telemetry}` | Work/event outcome, output suppression, startup environment/rolling policy, replay, correlation, idempotent close, and artifact JSON. |
| B-H01 | `TestProvidersSharedProcessTopology` | Two explicit sessions on one root/listener, distinct immutable routes, concurrent isolation, exact outputs, and zero routes after cleanup. |
| B-H02 | `TestProvidersSharedProcessRoutes` | Duplicate registration rejection, known-route invocation, unknown-route failure, and no cross-call. |
| B-H03..B-H08 | `TestProvidersSharedProcessAdverseRecovery/{invalid_template,dependency_failure,timeout,cancellation,unknown_route,known_good_after_adverse_cases}` | Shared zero-call failure, dependency error, timeout retry, cancellation, unknown route, known-good recovery, and route census. |
| B-C01 | Shared `Scenario.close` plus topology test | Session deletion, route removal, Factory/worktree path removal, listener/root ownership, and final census. |
| B-C02 | Shared adverse recovery and next known-good case | Failure cleanup leaves zero resources while the shared process remains usable. |
| B-C03 | Real timeout cleanup tests and shared timeout case | Active execution zero, child PID termination where applicable, and session/route/path removal. |
| B-C04 | Shared cancellation/adverse recovery | Cancellation cleanup completes before known-good recovery; no active owned resource remains. |
| B-C05 | `TestProviders_ForcedAssertionFailureCleansOwnedResources` | Intentional child assertion failure still joins `Process.Execute`, closes listener/session/routes, and removes owned paths. |

This reconciles all `B-001..B-040`, `B-H01..B-H08`, and `B-C01..B-C05`
rows. The C06 CASE-001..CASE-040 table remains the detailed given/when/then
source record for B-001..B-040; the C14 H/C rows are the current merged
shared-spine and cleanup additions.

### ACP-* rows (56 total)

| Matrix IDs | Current source owner / expansion | Frozen observable categories |
| --- | --- | --- |
| ACP-001..ACP-017 | `acp_error_test.go`, `acp_provider_events_test.go`, `basic_factory_run_test.go`, `btrc_p0_characterization_test.go`, and `eligible_shared_behavior_test.go` / shared-process expansions | ACP selection and legacy compatibility, Provider Session, JavaScript/mixed routing, response/Worker/Factory event order, partial failure, auth, model option, canonical resource, BTRC success/failure projections, cancellation, and generic protocol classification. |
| ACP-018..ACP-022 | `eligible_shared_behavior_test.go` catalog negative subtests | Missing/noncanonical name, TCP rejection, empty command, missing delete name, exact CLI errors, and absent settings. |
| ACP-023..ACP-024 | `TestACPSharedProcess` catalog persistence/idempotency scenarios | Add/list/delete persistence, init idempotency, default/custom entry retention, and absence of policy fields. |
| ACP-025 | `daemon_concurrency_test.go` | Two prompts remain blocked until release, serialize over one real stdio connection/peer, then both complete. |
| ACP-026..ACP-027 | `daemon_crash_recovery_test.go` | Uncertain-prompt crash replacement, no replay, live disconnect retirement, replacement success, and exact two-start counts. |
| ACP-028..ACP-030 | `daemon_reuse_test.go`, `daemon_shutdown_test.go` | One-process/one-connection sequential reuse, incompatible negotiation, cancellation, process join, and cleanup. |
| ACP-031 | `golden_fixture_test.go` | Pinned manifest identity, checksums, parseability, uniqueness, and counts. |
| ACP-032..ACP-037 | `acp_error_test.go`, `basic_factory_run_test.go` | OS command-start dependency failure, secret redaction, incompatible/generic protocol classes, unavailable executable, and zero-start boundaries. |
| ACP-038..ACP-041 | `packaged_conformance_test.go` and unexpected-command helper | Exactly 20 named profile fixtures, ACP v1 conformance, one allowlisted executable projection, and no ambient command launch. |
| ACP-042..ACP-043 | `packaged_spawn_test.go`, `packaged_tournament_test.go` | One persistent real stdio peer across four-agent spawn and three-agent tournament workflows, with merged/champion output. |
| ACP-044..ACP-050 | `run_failure_diagnostics_test.go`, `run_parameters_content_test.go`, `run_permissions_test.go`, `run_unsupported_capabilities_test.go` | Pinned session/config failures, exact sanitized wire transcript/content, permission reject/allow selection, and unsupported filesystem/terminal RPC responses. |
| ACP-051..ACP-052 | Functional and golden child helpers | Ordinary discovery remains inert; parent owns all protocol/wire assertions. |
| ACP-053..ACP-054 | Event/Worker Session owners and manifest checks | Response/Factory sequence monotonicity, one terminal publication, source-key/name uniqueness, and separation from replay. |
| ACP-055 | Scope disposition, no current test | No public ACP provider-attempt deadline exists; synchronous `wait.timeoutMillis` semantics are not reclassified as a provider timeout. No contract or timeout test is invented. |
| ACP-056 | Every shared/isolated owner and teardown helper | Sessions, listener, peer, process, connection, route, and temporary path cleanup on early failure. |

ACP-009 remains intentionally owned by the existing Workers mock gate under
`tests/functional/workers/mock`; this lane neither moves nor weakens it.

### F-* fixture/cross-cutting rows (18 total)

| Matrix IDs | Current source owner / status | Frozen observable categories |
| --- | --- | --- |
| F-001..F-002 | `fixture_test.go` valid functional/golden child subtests | Valid invocation-scoped fixture selects the real functional or pinned golden peer. |
| F-003..F-012 | `TestACPFixtureContractRejectsInvalidInvocationScopedData` malformed subtests | Invalid base64url/JSON, missing or unknown kind/mode, kind/mode mismatch, relative path, empty marker, and unknown JSON field all exit before JSON-RPC/filesystem side effects. |
| F-013..F-014 | `fixture_test.go` round-trip and secret subtests | Exact fixture round trip and production token absence from child arguments/output/artifacts. |
| F-015..F-018 | Cross-cutting later ACP optimization gates | Concurrent invocation isolation, audited capacity/parallelism, deterministic failure cleanup, and repeat/recovery with no stale child/process/connection/session/path state. These are preserved as explicit later proof obligations, not falsely claimed by characterization. |

The three tables reconcile the complete B/ACP/F inventory: 53 + 56 + 18 =
127 matrix rows. No row is silently reclassified as a pass by the baseline
diagnostics, and the future parity work remains owned by stories 002 and 003.

## Topology and wait inventory

### Base package

The merged C06 shared spine is present in `helpers_test.go`: one package-shared
`root.BuildProcess`, one injected loopback listener, one continuously executing
`Process.Execute`, explicit Factory Session tracking, immutable cleaned WorkDir
routes, and final route/session/path census. The shared fixture itself observes
one construction and one listener start and requires zero routes after cleanup.

The C06 source audit for the current merged topology records 13 default Linux
application constructions with 9 listeners and 11 default Windows
constructions with 7 listeners. The two Unix packaged-script rows skip before
construction on Windows. The tagged-long B-018..B-020/B-024 cases use the
existing shared fixture and add no second package-shared construction. Fresh
roots/listeners remain for malformed startup, real executable/process-tree,
record/replay, packaged Unix executable, logging-policy, and forced-cleanup
witnesses where sharing would erase the property under test.

The static wait scan found only existing synchronization and behavior budgets:

- loopback readiness and runtime status (`WaitForBaseURL`, `WaitForStatus`);
- public session terminal observation (`WaitForSessionTerminalStatus`);
- bounded `time.After(FixtureTimeout)` and context close bounds during shared
  fixture finalization;
- real process-tree/attempt helper sleeps that intentionally outlive the
  1500ms factory timeout, plus the 25ms PID-file observation interval;
- the explicit child `sleep-write-file` behavior case; and
- the existing 10ms runtime-log synchronization in the dedicated logging
  witness.

These are existing source-level waits, not new padding. The process-tree
sleep is the child behavior that proves cancellation, and the cleanup/status
polls have in-code reasons tied to real process/listener or filesystem
observation. No C14 characterization edit added a sleep, timeout, or poll.

### ACP package

The C06/C10 merged topology records 15 root constructions: one shared
`TestACPSharedProcess` root plus retained isolated roots for process,
connection, negotiation, crash, shutdown, command, golden-wire, permission,
packaged, and bidirectional-RPC witnesses. The package-owned peer accounting
is 17 shared peer starts plus 21 retained isolated peer starts, 38 accounted
starts total. The total is not an optimization target by itself: isolated
starts are witnesses for real process/stdio identity and replacement.

Zero-start boundaries include root construction, unknown provider, unavailable
executable, command-start rejection, legacy SCRIPT_WRAP routing, mock-worker
routing, catalog/assets, and inert helper discovery. Two-start boundaries are
the retry/session-continuation, crash replacement, and disconnect replacement
cases. The profile emitted repeated root/process/peer/connection/route markers;
it did not replace the C06 source-and-counter audit with a host-wide process
claim.

The ACP wait scan found existing deterministic observations and bounded cleanup
guards:

- `WaitForURL`, `WaitForStatus`, `WaitForSessionTerminalStatus`, and
  `WaitForObservation` for public readiness, terminal state, marker files, and
  peer exits;
- 10ms marker/release polling in the child peer and 25ms daemon process
  observation where a real child must signal a filesystem boundary;
- 3s/5s/20s cancellation or cleanup bounds for daemon/listener/process join;
  and
- explicit release files for serialization, retry, disconnect, and packaged
  conformance cases.

The comments in the existing ACP sources identify these polls as real child
readiness, release, or cleanup edges. There is no public ACP provider-attempt
deadline; `wait.timeoutMillis` remains a synchronous wait budget. No new
timeout contract or test padding is authorized.

## JSON profile evidence

Each package received one pre-edit JSON execution profile with the exact
`-count=1` and `-timeout=15m` package command:

| Package | Command | Wall | Exit | Go test events | Slowest useful records |
| --- | --- | ---: | ---: | --- | --- |
| Base | `go test -json ./tests/functional/providers -count=1 -timeout=15m` | 39.183s | 1 | 46 pass, 1 fail | Runtime logging 5.340s; real timeout cleanup 3.800s; timeout retry 3.790s; mixed mock/replay failure 2.750s. |
| ACP | `go test -json ./tests/functional/providers/acp -count=1 -timeout=15m` | 102.216s | 0 | 88 pass | Shared process 64.870s; packaged spawn 10.600s; tournament 10.520s; shutdown 10.460s; serialization 10.370s; reuse 10.360s; negotiation 10.100s. |

The base profile's single failure was
`TestMockWorkers_EndToEndSmokeRunsMixedOutcomesWithoutLiveProviderCredentials`.
The runtime emitted the unchanged refusal-class mismatch (`unknown` observed
versus `permanent_bad_request` expected) after the test's public recording and
replay assertions. This is diagnostic evidence of the untouched base, not a
reason to alter B-028.

The ACP profile's process-oriented output included repeated `root`, `process`,
`peer`, `connection`, `fixture`, and `route` markers. It emitted no source
output marker for `sleep`, `poll`, `wait`, `timeout`, or `cleanup`; the static
wait inventory above is therefore the authoritative accounting for existing
wait code, and absence of a profile output marker is not treated as proof that
all waits are free.

## RACE-003 pre-change disposition

Both full-package race commands were run before any C14 source edit:

| Package | Command | Wall | Package | Exit | Observation |
| --- | --- | ---: | ---: | ---: | --- |
| Base | `go test -race ./tests/functional/providers -count=1 -timeout=15m` | 120.654s | 73.145s | 1 | Same unchanged mixed mock/replay assertion; no `WARNING: DATA RACE`. |
| ACP | `go test -race ./tests/functional/providers/acp -count=1 -timeout=15m` | 191.666s | 182.297s | 0 | Full package passed; no race report. |

The exact full-package race commands are supported pre-change requirements.
Any later change to a concurrent helper must rerun the relevant full-package
command; a failure caused by a changed helper is not waived by the pre-change
base diagnostic. The base failure is retained as an environmental/runtime
diagnostic and does not prove a race.

## Story-001 acceptance disposition

| Criterion | Disposition | Evidence |
| --- | --- | --- |
| Three isolated samples, medians, environment, summed median | **PASS** | Six separate `-count=1` invocations and tables above. |
| Current test/expanded-subcase inventory reconciles to B/ACP/F | **PASS** | Exact 35/26 listings, 53/56/18 row mapping, and C06/C10 reconciliation above. |
| Root/process/listener/peer/wait/profile findings recorded | **PASS** | C06/C10 source-and-counter topology plus one JSON profile per package and wait inventory. |
| Full-package race support recorded | **PASS with diagnostic** | Full base/ACP `-race` command results above. |
| Environmental/noisy failures retained honestly | **PASS** | Base repeated mismatch and ACP first-run contention are explicitly diagnostic; no assertion was changed. |

This story establishes the pre-change denominator and freezes the assertion and
isolation inventory. It does not mark the later same-or-stronger parity or
performance criteria as passing.

## Remaining unproven edges

| Edge | Owning gate |
| --- | --- |
| Base post-change parity, cleanup, and intrinsic work reduction | `BASE-002` |
| ACP post-change protocol/process parity, cleanup, and intrinsic work reduction | `ACP-003` |
| Changed concurrent helper race/repeat evidence | `RACE-003` |
| Final per-package and summed medians, 40-percent target or measured floor | `PERF-004` |
| Clean-room integrated validation | `CLEAN-004` |
| Hosted PR package result and delivery comment | `PR-CI-004` |
| Remote provider behavior | Intentionally outside scope |
