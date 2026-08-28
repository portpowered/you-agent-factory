# C07 sessions, Work, and Chat root-composition characterization

## Scope and result

- Story: `functional-test-optimization-c07-cleanup-sessions-work-chat-001`
- Parent behavior: `BEH-001` — public lifecycle, routing, Chat Session,
  event, persistence, ordering, and replay outcomes remain equivalent while
  owned resources are characterized before cleanup correction.
- Captured: `2026-08-28T10:23:13-07:00`
- Current commit: `34cbab9f253208328c59b82eb8a3f17b76c09d15`
- Environment: Windows `windows/amd64`, Go `go1.25.0`
- Dependency fidelity: local-real root/session composition with controlled
  worker/provider edges; no remote or paid calls.
- Status: **PASS — characterization complete; cleanup gaps remain assigned to
  stories 002–005 and the named gates below.**

This is an additive audit artifact. Story 001 makes no cleanup correction and
does not change production code, public contracts, generated output, shared
support, the C01 inventory, or process-isolation policy.

The PRD names `docs/temp/functional-test-optimization.md` as its source plan,
but that path is absent from this checkout and has no reachable history on the
current refs. `progress.txt` was also absent at iteration start. The PRD and
current source were therefore the available authorities for this record; the
missing planning artifacts are retained as provenance gaps, not silently
reconstructed.

## Reproducible discovery and characterization

The exact discovery commands were run before any repository edit:

```text
go test -list '^Test' ./tests/functional/sessions/lifecycle
go test -list '^Test' ./tests/functional/work/routing
go test -list '^Test' ./tests/functional/sessions/chat_sessions/root_composition
```

The commands exited `0`. The lifecycle package's `TestMain` emitted its
existing shutdown diagnostic while listing; it reported two root-built roles,
one HTTP listener start, a closed listener, and a removed fixture root.

The final top-level denominator is exactly the planning baseline:

| Package | Top-level tests | Result |
| --- | ---: | --- |
| `./tests/functional/sessions/lifecycle` | 12 | exact list below |
| `./tests/functional/work/routing` | 1 | exact list below; it owns nested scenario cases |
| `./tests/functional/sessions/chat_sessions/root_composition` | 25 | exact list below; two are peer-process entrypoints |
| **Total** | **38** | **matches the PRD baseline** |

### Lifecycle denominator (12)

```text
TestFactorySessionCreateListShowDelete
TestFactorySessionListMultipleSessions
TestFactorySessionMissingShowAndDeleteFail
TestAPIOpenListGetAndCloseFactorySession
TestAPIFactorySessionNotFoundUsesTypedError
TestAPIMultipleFactorySessionsRemainIsolated
TestFactorySessionCleanupRunsAfterEarlySubtestExit
TestCLIRemoteRunStartsDurableSessionOnSelectedServer
TestCLILocalAndRemoteRunSuccessParityThroughRootProcess
TestCLILocalAndRemoteRunDomainFailureParityThroughRootProcess
TestCLILocalAndRemoteRunCancellationParityThroughRootProcess
TestCLIRemoteRunTransportFailureKeepsStreamsAndExitObservable
```

### Work-routing denominator (1 top-level)

```text
TestSharedProcessWorkRouting
```

Its current nested scenario inventory is eleven groups:

```text
LogicalMove/CompletesWithoutWorkerDispatch
LogicalMove/PreservesWorkPayloadAndLineage
LogicalMove/MultipleOutputsCreatesEveryExpectedWork
ClassifierSuccess/RoutesEveryKnownDecision
ClassifierFanout/PreservesPayload
RoutingGuard/SelectorFailureClosesSession
ClassifierFailure/UnknownAndMalformedDecision
ClassifierFailure/ReworkFailureTerminatesWithoutCompletion
ClassifierRejection/RoutesToFailedTerminal
ClassifierRejection/RecordsDispatchFeedback
ClassifierRejection/ReleasesResourcesForSubsequentWork
```

`ClassifierSuccess/RoutesEveryKnownDecision` has four nested decision cases;
`ClassifierFailure/UnknownAndMalformedDecision` has three nested malformed or
unknown cases. Those nested cases are behavior witnesses, not additional
top-level denominator entries.

### Chat root-composition denominator (25)

```text
TestACPSessionAnswersEachTurnWithThatTurnsOwnResult
TestPackagedFactoriesCompleteOneACPPromptTurn
TestPackagedPlanParallelCompletesOneACPPromptTurn
TestFactoryBuilderGreetsOnAVagueFirstACPTurn
TestPackagedJavaScriptFactoryCompletesOneACPPromptTurn
TestPackagedJavaScriptFactoryWithStructuredResultStreamsItsResult
TestACPPromptDelegationStartsOneFactorySessionAndReusesItForLaterTurns
TestACPPromptDelegationFailedFactoryInvocationReportsAnACPError
TestACPPromptDelegationUnresolvableFactoryTargetFailsSafelyAndTerminalizes
TestACPPromptDelegationRedeliveredRequestMakesNoSecondFactoryDispatch
TestACPPromptDelegationConcurrentPromptRejectsAsBusyWithNoFactoryDispatch
TestACPServerReachesCanonicalChatSessionsAuthorityThroughRootBuildProcess
TestACPServerWithNoAuthoredAgentProfileOffersEveryInstalledFactory
TestACPServerAuthoredAllowedTargetsStillRestrictsCatalog
TestACPServerSetConfigOptionSelectsAnotherInstalledFactory
TestACPServerAuthoredAllowedTargetsAreOfferedInAuthoredOrder
TestACPServerUnrestrictedTargetsAreOfferedCurrentFirstThenSorted
TestACPServeCommandStreamsThroughRootBuildProcessWithoutDuplicateFinalText
TestACPServeCommandStreamsUsageUpdateThroughRootBuildProcess
TestACPStreamUsagePeerProcess
TestOneACPWorkerDeliversEveryUpdateAsChildContent
TestTwoACPWorkersKeepChildStreamsAttributed
TestACPWorkerChildStreamSurvivesRetainedReplay
TestACPWorkerChildPeerProcess
TestJavaScriptFactoryChildrenAreVisibleAsWorkers
```

## Current characterization results

The package-level characterization commands were:

```text
go test -count=1 -timeout=10m ./tests/functional/sessions/lifecycle
go test -count=1 -timeout=10m ./tests/functional/work/routing
go test -count=1 -timeout=10m ./tests/functional/sessions/chat_sessions/root_composition
```

All three exited `0`:

| Package | Observed package result |
| --- | --- |
| lifecycle | `ok`, `3.052s` |
| routing | `ok`, `1.248s` |
| chat root composition | `ok`, `17.092s` |

The focused characterization commands were also run:

```text
go test -count=1 -timeout=10m -v -run '^TestFactorySessionCleanupRunsAfterEarlySubtestExit$' ./tests/functional/sessions/lifecycle
go test -count=1 -timeout=10m -v -run '^TestSharedProcessWorkRouting$' ./tests/functional/work/routing
go test -count=1 -timeout=10m -v -run '^(TestACPServerReachesCanonicalChatSessionsAuthorityThroughRootBuildProcess|TestACPPromptDelegationUnresolvableFactoryTargetFailsSafelyAndTerminalizes|TestACPPromptDelegationConcurrentPromptRejectsAsBusyWithNoFactoryDispatch|TestACPServeCommandStreamsThroughRootBuildProcessWithoutDuplicateFinalText|TestACPWorkerChildStreamSurvivesRetainedReplay)$' ./tests/functional/sessions/chat_sessions/root_composition
```

The focused commands exited `0`: lifecycle `0.991s`, routing `1.543s`, and
chat `2.772s`. The routing output showed all eleven groups passing, including
four known decisions and three malformed/unknown cases. The chat output showed
the canonical session, resolver failure/recovery, busy prompt, stream, and
retained replay witnesses passing.

Observed logs that are part of the current characterization, not failures:

- lifecycle and routing log the existing `logicaltarget/discovery.go` probe
  diagnostic when a transient child directory is examined before its authored
  Factory root is written; the public test still passes;
- the lifecycle early-exit setup logs the existing preseed `README.md` path
  depth warning;
- the unresolvable ACP witness logs one existing `acp wire transcript
  unavailable` warning while asserting its bounded public error.

No shared-support defect was localized in this story. No file under
`tests/functional/internal/support/**` was edited.

## CASE-X01 through CASE-X05 characterization

| Case | Existing witness and observation | Current census result | Remaining edge |
| --- | --- | --- | --- |
| `CASE-X01` early exit | `TestFactorySessionCleanupRunsAfterEarlySubtestExit` opens a public Factory Session, registers `registerLifecycleSessionCleanup`, returns before explicit body cleanup, and the parent observes the session as typed not-found through the shared HTTP server. | One registered callback reached the supported terminate/delete path; public session absence was observed. The current helper does not count callback invocations, so this is not yet a numeric exactly-once proof. | Per-resource close counts and all resource classes -> stories 002–004 / `GATE-PKG-001`. |
| `CASE-X02` concurrency | `TestSharedProcessWorkRouting` schedules independent scenario groups through four goroutines. Each scenario has a unique root, Factory, route selector, and explicit Factory Session. Chat's two-worker and JavaScript-child witnesses preserve distinct child identities; lifecycle's two-session witnesses preserve a sibling after one closes. | Routing passed with no cross-attribution or cross-close observation. Lifecycle and chat do not yet share a common acquisition ledger or prove every concurrent teardown interleaving. | Count-three and supported race matrix -> `GATE-REPEAT-001`, `GATE-RACE-001`. |
| `CASE-X03` recovery | Focused lifecycle cancellation/transport, routing failure/rejection, and chat resolver-failure, busy-prompt, stream, and retained-replay witnesses all passed. | Existing failure/cancellation recovery remained publicly usable. The current three-package baseline has no complete common timeout/interrupted-stream census; those paths remain explicitly unproven rather than inferred from normal close. | Timeout, interruption, active-call, reader/writer, and late-event proof -> stories 002–004. |
| `CASE-X04` empty/zero residue | Lifecycle `TestMain` reports `roles=2`, `http-listener-starts=1`, `listener-closed=true`, `fixture-root-removed=true`. Routing package cleanup checks the shared listener, active controlled calls, explicit session absence, per-scenario roots, and its ledger. Chat `TestMain` closes package cohorts/homes and each ACP harness closes invocation pipes/context. | Existing public absence and package-root cleanup probes are present for lifecycle/routing. Chat has close owners but no complete zero-valued session/stream/turn/path/runtime census yet. | Full zero-residue census -> stories 002–004 and `VAL-001`. |
| `CASE-X05` denominator/maximum | The three exact `go test -list` outputs total 38 top-level tests; routing bounds concurrent scenario workers at four and retains all nested behavior cases. | Exact denominator matches the planning baseline; no synthetic load ceiling was introduced. | Final post-correction denominator and clean-room result -> story 005 / `VAL-001`. |

## Acquisition and close-owner vocabulary

The audit uses these terms consistently in later cleanup stories:

| Term | Meaning in this lane |
| --- | --- |
| Acquisition | The first operation that creates or makes a resource live: root construction, listener bind, Factory Session open, route registration, command start, pipe creation, stream/subscription creation, or temporary path creation. |
| Owning scope | The smallest test, scenario, cohort, or package `TestMain` that remains responsible for the resource until terminal close. |
| Supported close | The public or existing narrow lifecycle operation for the resource, such as Factory Session close/delete, command stop/join, process `Close`, pipe close, route unregister, or reader/subscription close. |
| Explicit isolated fallback | Closing the owning process when the current public contract cannot close a terminal activation or stream independently; this is valid only when the reason and absence probe are recorded. |
| Public absence | A customer-facing read proves a closed session/listener/resource is no longer addressable, such as HTTP 404 or an unreachable `/status`. |
| Physical absence | Filesystem/process/path/listener probes prove the resource no longer exists after its owner closes. |
| Active-call absence | The controlled provider/worker edge reports no in-flight call after cancellation or owner close. |
| Census | A per-resource record of identity, owner, acquisition count, terminal-close count, public absence, physical absence, and any remaining residue. |

## Resource ownership inventory

### Factory Session lifecycle package

| Resource | Acquisition | Current owner and close path | Current observation / gap |
| --- | --- | --- | --- |
| Shared server root process | `startSharedLifecycleServer` builds a root process with role `server`; `Process.Execute` runs in a goroutine. | Package `TestMain` calls `fixture.stop`: cancel, wait for `Execute`, then `Process.Close`. | Role and listener shutdown are reported by `LIFECYCLE-005`; no process PID/goroutine census. |
| Shared client root process | `startSharedLifecycleServer` builds role `client`. | Package `TestMain` calls `closeLifecycleProcess(fixture.client.process)` after client invocations have joined. | Exactly two root-built roles are reported; no per-invocation process census because `Process.Execute` is reused. |
| HTTP listener/port | `lifecycleHTTPServer.start` is passed as `APIServerStarter` and binds once. | Server process close; `waitClosed` and a post-close `/status` probe in `fixture.stop`. | One start, closed listener, and unreachable probe pass in current runs. |
| Per-test Factory Session | CLI `session create`, HTTP `POST /factory-sessions`, or remote execution admission. | `registerLifecycleSessionCleanup` registers `t.Cleanup`; cleanup terminates then DELETEs. Explicit body close can call the returned marker when the test already closed the session. | Early exit proves public absence. No acquisition/close counter or complete session list census yet. |
| CLI invocation and response streams | `lifecycleClientProcess.startCLI` uses `support.StartProcessCommand`; output may be a captured buffer or readiness writer. | Caller waits/accepts command; context cancellation and `command.Stop` join. Shared client mutex remains held until `Done`. | Cancellation/transport parity passed; stream close counts remain uninstrumented. |
| Temporary home, Factory, working directory | `os.MkdirTemp`/`t.TempDir`; shared paths are under `rootDir`. | Per-test `t.TempDir`; package `fixture.stop` removes the shared root after process close. | Shared root removal passes; per-test path census is implicit in testing cleanup. |
| HTTP response bodies | Public helper requests and probes create `http.Response` bodies. | Each helper defers `Body.Close`. | Code inspection and passing requests show close ownership; no leak metric. |

### Work-routing package

| Resource | Acquisition | Current owner and close path | Current observation / gap |
| --- | --- | --- | --- |
| Shared root process and service listener | `newWorkRoutingPackageFixture` builds one root; its API starter records a process start and starts `ProcessAPIServer`; `startWorkRoutingProcess` runs the service command. | Package `TestMain` stops the command, calls root `Process.Close`, waits for `apiStopped`, probes `/status`, and removes `fixture.rootDir`. | Existing ledger requires exactly one process start; package close checks active calls, listener reachability, and root removal. |
| Scenario root and Factory directory | `fixture.newScenario` creates a unique temp root and copies a named fixture. | `workRoutingScenario.close` closes the Factory Session, asserts HTTP absence, removes the scenario root, then records the ledger close. | Unique absolute paths and exactly-once ledger close are checked; worktree/path subresources are not separately counted. |
| Factory Session | `workRoutingScenario.open` calls public `OpenFactorySessionAt` and registers the returned ID. | Scenario `t.Cleanup` calls `scenario.close`, which uses public `CloseFactorySessionAt`. | Ledger checks non-default unique IDs, closed count, public absence, and root removal. |
| Provider route | `fixture.provider.register` adds selectors for the scenario. | `t.Cleanup` from `newScenario` calls `provider.unregister`. | Selector registration is synchronized; route count/close timing is not emitted in a final census. |
| Controlled provider call | Scenario runner increments its active count around the injected command. | Runtime completion/cancellation decrements it; package cleanup asserts active count zero. | Active-call zero is checked only at package shutdown, not after every scenario/failure path. |
| Factory Event/Work reads and response bodies | Scenario `observe` reads public session, Work, and Factory Event endpoints. | HTTP helpers defer response body close; scenario cleanup closes session after observations. | Ordered public behavior passes; reader/stream identity is not separately ledgered. |
| Temporary package home/host/factory paths | `MkdirTemp` and fixture copy during package setup. | Package close removes `fixture.rootDir`; scenario close removes each scenario root. | Shared and scenario physical root probes pass. |

### Chat root-composition package

| Resource | Acquisition | Current owner and close path | Current observation / gap |
| --- | --- | --- | --- |
| Catalog cohort process/home | `catalogCohortForTest` creates one home, seeds installed packaged Factories, and builds one root. | Package `TestMain` calls `closeCatalogCohort`, closes the process, and removes the home. | Catalog-only tests share immutable profile/catalog state; no process/active runtime census is emitted. |
| Controlled cohort process/home | `controlledACPCohortForTest` creates one fixed profile/root for non-activation witnesses. | Package `TestMain` closes the process and removes the home. | Package ownership is explicit; retained runtime absence is inferred from successful close and path removal. |
| Isolated activation process/home | `newControlledACPCohort`, `newPackagedFactoryCohort`, or a test-specific builder creates a fresh root where the current ACP close contract cannot release a terminal activation. | `closeProcessCleanly` or a `t.Cleanup` closes the owning process before `t.TempDir` removes its home. | Isolation reason is documented inline. This is the required current-contract fallback; no public per-activation close exists. |
| ACP stdin/stdout pipes | `startServeACPProcess` creates two `os.Pipe` pairs and starts `Process.Execute` in a goroutine. | Cleanup cancels the invocation, closes stdin, waits up to five seconds for `Execute`, then closes all pipe ends. | Normal stream and cleanup witnesses pass; interrupted-stream reader/writer termination is not fully counted yet. |
| ACP session/turn/update stream | `session/new`/`session/prompt` and response/update reads use the real ACP server; retained replay reads child updates. | Invocation cleanup closes the process/pipe owner; current public ACP close is not used for every terminal Factory activation. | Public ordering, attribution, busy/error, and replay behavior passes; Chat Session, turn, subscription, and cursor counters are absent. |
| Controlled provider/peer subprocess | Usage and child-event fixtures re-exec the test binary through `exec.Command(os.Args[0], ...)` as an ACP peer. | Production ACP provider owns peer protocol lifetime; enclosing process cleanup closes the root and pipe boundary. | One peer start is asserted in usage/child fixtures; no package-wide child PID census. |
| Packaged Factory/Work/Factory Session runtime | Prompt fixtures seed real packaged Factory files and activate them through the root ACP path. | Scenario process close is the current supported owner when the terminal activation has no narrower close; `t.TempDir` removes home/cwd afterward. | Existing prompt results and child attribution pass; runtime/worktree/retained-session zero residue needs story 004's census. |
| Temporary home/workdir/Factory files | `t.TempDir`, `os.MkdirTemp`, and fixture writers create paths. | `t.Cleanup`, process-close ordering, and testing cleanup remove them. | Cleanup order is explicitly documented; no post-cleanup path enumeration is emitted. |

## Process-isolation classification

The classification is based on the resource boundary the test body actually
needs, not on the presence of `t.Run` or a helper name.

| Cohort | Tests | Classification and inline reason |
| --- | --- | --- |
| Lifecycle shared host/client | All 12 lifecycle tests | Shared process. The public CLI/API lifecycle witnesses use one server listener and one serialized reusable client process; each created Factory Session and response command is the scenario boundary. Rebuilding a root per test would change the already-migrated topology and is not needed for these public session observations. |
| Work shared host | `TestSharedProcessWorkRouting` and its eleven groups | Shared process with unique explicit sessions. Four scenario workers run concurrently against one service-mode root and controlled command edge; unique scenario roots, route selectors, Factory Sessions, and request identities keep attribution independent. |
| Chat immutable catalog | `TestACPServerWithNoAuthoredAgentProfileOffersEveryInstalledFactory`, `TestACPServerSetConfigOptionSelectsAnotherInstalledFactory`, `TestACPServerUnrestrictedTargetsAreOfferedCurrentFirstThenSorted` | Shared catalog cohort. These tests read/switch immutable installed catalog state without activating a terminal Factory runtime that another test must release. |
| Chat fixed controlled cohort | `TestACPServerReachesCanonicalChatSessionsAuthorityThroughRootBuildProcess` | Shared controlled cohort. The test opens independent ACP connection/session identities against one fixed profile and observes canonical root composition. |
| Chat authored catalog profiles | `TestACPServerAuthoredAllowedTargetsStillRestrictsCatalog`, `TestACPServerAuthoredAllowedTargetsAreOfferedInAuthoredOrder` | Isolated cohort per profile. Each test changes persisted allowlist/profile inputs; sharing would make the profile and target order mutable across tests. |
| Chat activation-owning cohorts | `TestACPSessionAnswersEachTurnWithThatTurnsOwnResult`, the two prompt-delegation success/failure tests, redelivery, busy prompt, stream-composition, all packaged prompt tests, the three worker-child behavior tests, and `TestJavaScriptFactoryChildrenAreVisibleAsWorkers` | Isolated process/root with reason. The current production ACP activation binds a terminalized Factory under process-scoped `~default` state and has no narrower public release/reopen operation. Process close is therefore the narrowest supported owner that prevents stale activation from contaminating a later test. |
| Chat resolver-specific root | `TestACPPromptDelegationUnresolvableFactoryTargetFailsSafelyAndTerminalizes` | Isolated process/root. The test removes an installed Factory, clears home variables, and corrupts Operator Settings after admission; those destructive resolver inputs cannot be shared. |
| Chat peer entrypoints | `TestACPStreamUsagePeerProcess` and `TestACPWorkerChildPeerProcess` | Self-reexec helper entrypoints. Under an ordinary package run the environment gate is absent and each returns immediately; the actual peer subprocess is started and owned by the corresponding usage/worker fixture. |

No process-isolation classification was changed in story 001. The table freezes
the current topology and its reasons for later correction review.

## Shared-support and excluded-surface audit

- `tests/functional/internal/support/**`: read-only; no defect localized and no
  diff.
- C01 eligibility inventory: read-only; no diff.
- Production, public contract, generated, UI, restart, remote-provider, and
  paid-validation surfaces: no diff and no claim.
- The existing production discovery diagnostics observed during fixture setup
  are not converted into a shared-support defect without a minimal reproducer
  and owner. They did not alter the public result or cleanup outcome in this
  characterization run.

## Evidence boundaries and handoff

This story proves:

- the exact 38-test top-level denominator remains intact;
- current public lifecycle, Work routing, ACP session/stream, ordering,
  failure, concurrency, and replay witnesses pass on the current topology;
- current acquisition and close owners, process-isolation reasons, and known
  census gaps are recorded before correction;
- the early-exit public absence path, shared routing concurrency, and focused
  recovery witnesses execute without changing their original outcomes.

This story does not prove corrected zero residue, repeat-safe teardown, race
cleanliness, clean-room integration, a portable timing threshold, terminal CI,
remote provider behavior, or crash/restart cleanup. Those edges remain owned by
stories 002–005 and `GATE-PKG-001`, `GATE-REPEAT-001`, `GATE-RACE-001`,
`VAL-001`, `REV-CI-001`, `EXISTING-PROVIDER-INTEGRATION-RELEASE`, and
`EXCLUDED-STABILITY-RESTART` as applicable.

The next implementation slice may add package-local ledgers and supported
close/absence probes using this vocabulary. It must preserve the 38-test
denominator, keep shared support read-only, and treat any missing public close
capability as an explicit isolated-with-reason boundary rather than inventing a
new ACP contract.

## VAL-001 clean-room validation

Story `functional-test-optimization-c07-cleanup-sessions-work-chat-005` ran the
final integrated proof from detached clean worktrees at
`cbd175dd74b98776c8fb284ff35975df41d44a6f`. Discovery retained the exact
12 + 1 + 25 = 38 top-level denominator. Count-one and count-three runs passed
for all three packages, the supported lifecycle/routing/chat race paths passed
(routing required one bounded rerun after a transient first attempt), and
`make test-lane-audit` reported the expected ownership counts.

The final verbose clean-room censuses were zero-residue: lifecycle closed
2/2 processes, 30/30 invocation streams, 12/12 sessions, and removed 12/12
temporary Factory paths; routing closed 16/16 sessions, 139/139 readers,
18/18 routes, and removed 16/16 scenario roots; chat closed 24/24 processes,
58/58 connections, 58/58 response streams, 56/56 pipes, 29/29 sessions,
30/30 turns, 34/34 active calls, 4/4 peer processes, and removed 97/97 paths
with zero violations. The full report, exact commands, timing caveats, and
remaining review-owned edges are in
[`validation/c07-cleanup-sessions-work-chat.md`](validation/c07-cleanup-sessions-work-chat.md).
