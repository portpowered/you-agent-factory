# C14 sessions/restart characterization evidence

## GATE-CHAR

- Story: `fto-c14-pkg-sessions-restart-001`
- Parent behavior: `BEH-C14-RESTART` — every restart, replay, persistence,
  failure, process, and cleanup witness remains while redundant intrinsic work
  is removed.
- Status: **PASS for pre-change characterization**.
- Captured: `2026-08-29` before any edit under
  `tests/functional/sessions/restart/**`.
- Tested head: `fea2e30a499384182d2fabe7038767e3c2f9c5e5`.
- Dependency fidelity: `local_real` Go build, real Windows processes,
  loopback, filesystem, recording, and controlled pre-existing provider edges.
- Cost: zero paid or remote-provider calls.

This is the story-001 before-state record. It does not claim the shared binary,
post-change behavior, final performance, race behavior, clean-room result, or
hosted PR result. No test implementation, production, shared-support,
contract, generated, workflow, or CI file was changed for this story.

## Exact command and environment

Selector discovery:

```text
go test ./tests/functional/sessions/restart/... -list '^Test'
```

Exit status: `0`.

The required three-run procedure was executed from the repository root in
PowerShell exactly as follows:

```powershell
$results = [System.Collections.Generic.List[object]]::new()
1..3 | ForEach-Object {
  $timer = [System.Diagnostics.Stopwatch]::StartNew()
  go test ./tests/functional/sessions/restart/... -count=1
  $exitCode = $LASTEXITCODE
  $timer.Stop()
  $result = [pscustomobject]@{ Run = $_; Seconds = $timer.Elapsed.TotalSeconds; ExitCode = $exitCode }
  [void]$results.Add($result)
  if ($exitCode -ne 0) { throw "restart baseline/final run $_ failed with exit $exitCode" }
}
$ordered = $results | Sort-Object Seconds
$ordered
$medianSeconds = $ordered[1].Seconds
```

Environment:

| Field | Value |
| --- | --- |
| Go | `go1.25.0` |
| OS/architecture | `windows/amd64` |
| CGO | enabled (`CGO_ENABLED=1`) |
| Package command | `go test ./tests/functional/sessions/restart/... -count=1` |
| Source head | `fea2e30a499384182d2fabe7038767e3c2f9c5e5` |
| Paid/remote dependency | none |
| Source changes before measurement | none |

The three PowerShell result objects all exited `0`. The console table rendered
seconds to two decimal places; the retained middle result was
`medianSeconds=44.1461053`.

| Run | Outer stopwatch seconds | Exit | Package-reported elapsed | Use |
| ---: | ---: | ---: | ---: | --- |
| 1 | `44.15` | `0` | `42.223s` | successful comparable baseline |
| 2 | `49.59` | `0` | `46.881s` | successful comparable baseline |
| 3 | `39.12` | `0` | `36.872s` | successful comparable baseline |
| **Median** | **`44.1461053s`** | — | — | **authoritative pre-change median** |

The sorted order was run 3, run 1, run 2. There were no failed or excluded
runs in the prescribed baseline set. A preceding output-capturing diagnostic
set also passed (`40.573s`, `34.079s`, `57.944s`, median `40.573s`); it is
retained as diagnostic context only and is not used as the authoritative
denominator because the exact procedure above was subsequently run.

The JSON event profile was then run once with:

```text
go test -json ./tests/functional/sessions/restart/... -count=1
```

It exited `0`; all seven selectors passed and the package event reported
`23.397s`. The JSON selector observations were:

| Selector | Result | Elapsed |
| --- | --- | ---: |
| `TestBoardPersistenceWorkerHelper` | pass | `0.00s` |
| `TestBoardPersistenceCLIRestartRoundTrip` | pass | `9.06s` |
| `TestBoardPersistenceCLIRestartAfterHardKillWithMissingBoardRecording` | pass | `5.49s` |
| `TestBoardPersistenceCLIRestartWithCorruptBoardRecordingFails` | pass | `3.91s` |
| `TestFactorySessionRestartRemapsLiveIDToLogicalIdentity` | pass | `2.02s` |
| `TestFactorySessionResumeDoesNotRepeatCompletedDispatch` | pass | `1.01s` |
| `TestFactorySessionHistoryIsPersistedAcrossRestart` | pass | `1.85s` |

## Exact selector and assertion denominator

The package has ten Go source files, seven top-level `Test*` declarations, and
148 lexical `t.Fatal*`/`t.Error*`/`t.Fail*` failure-call sites. The lexical
count is a source denominator, not a claim that every call is a distinct
product assertion: shared helpers are invoked by multiple selectors. The
following table identifies every top-level selector and its observable
witnesses.

| Selector and source | Current observable witness | Case rows |
| --- | --- | --- |
| `TestBoardPersistenceWorkerHelper` (`board_persistence_scenarios_test.go:24`) | Helper-only environment guard; non-empty release path; sentinel output; release-file observation; unexpected filesystem errors fail. It is launched by the real `SCRIPT_WORKER` configuration and does not construct package setup. | `CASE-B05`, `CASE-U05`, `CASE-U06` |
| `TestBoardPersistenceCLIRestartRoundTrip` (`board_persistence_scenarios_test.go:56`) | Real built `you` CLI; three private daemon generations; exact three-then-four Work board; public Work/Factory Event/Worker Session observations; clean stop; replay/re-arm; release-file completion; CLI list/show parity; no phantom active attempt; cleanup. | `CASE-H01`–`CASE-H06`, `CASE-U03`–`CASE-U06`, `CASE-U08`, `CASE-R01`, `CASE-B02`–`CASE-B06` |
| `TestBoardPersistenceCLIRestartAfterHardKillWithMissingBoardRecording` (`board_persistence_scenarios_test.go:68`) | Real batch submission; complete processing; durable snapshot capture; recording removal; hard kill; exact empty recovered Work; byte-for-byte snapshot preservation; recovery warning fragments; cleanup. | `CASE-H01`, `CASE-H02`, `CASE-H05`, `CASE-U01`, `CASE-U03`–`CASE-U06`, `CASE-R01`, `CASE-B01`, `CASE-B03`, `CASE-B04` |
| `TestBoardPersistenceCLIRestartWithCorruptBoardRecordingFails` (`board_persistence_scenarios_test.go:138`) | Present corrupt recording; non-zero real daemon exit; exact structured corruption code/message/path; operator guidance fragments; no absence-recovery claim; corrupt bytes preserved. | `CASE-U02`–`CASE-U04`, `CASE-U06`, `CASE-U08` |
| `TestFactorySessionRestartRemapsLiveIDToLogicalIdentity` (`logical_identity_test.go:32`) | Two production-composed server generations; distinct live session ID and stream generation; stable logical key/backend scope; sync-preflight remap; checkpoint not reusable. | `CASE-H07`, `CASE-U04`, `CASE-U06` |
| `TestFactorySessionResumeDoesNotRepeatCompletedDispatch` (`logical_identity_test.go:114`) | Interrupted two-dispatch session; completed first dispatch; interrupted/running second dispatch; accepted public resume; terminal progress `2/2/0`; lifecycle continuity; exactly two dispatches; stable IDs; provider call count exactly three. | `CASE-H08`, `CASE-U04`–`CASE-U06`, `CASE-B02`, `CASE-B04` |
| `TestFactorySessionHistoryIsPersistedAcrossRestart` (`logical_identity_test.go:219`) | Interrupted durable session before restart; two dispatch summaries; non-empty event prefix; stale identity preflight; restarted session/lifecycle/progress; dispatch ID/kind/status parity; ordered event ID/type prefix continuity. | `CASE-H07`, `CASE-H09`, `CASE-U04`–`CASE-U06`, `CASE-B02`, `CASE-B04` |

The assertion helpers that carry the denominator are:

- `assertBoardList` and `assertBoardWork` in
  `board_persistence_cli_assertions_test.go:66,346`: count, duplicate and
  unexpected Work IDs; name, Work/request identity, state name/type, trace
  lineage, content shape/text, worker sentinel/PASS output, and parent-child
  relation.
- `readBoardDispatchStates` and the four dispatch wait helpers in
  `board_persistence_cli_assertions_test.go:147-323`: public Factory Event
  decoding and exactly-once request/response/interruption/reconciliation and
  Worker Session association facts, including one distinct re-armed dispatch.
- `assertBoardCLIListAndShows` and
  `assertBoardCLIWorkerSessionsForWork` in
  `board_persistence_cli_assertions_test.go:42,366`: CLI/HTTP parity,
  historical Worker Session presence, and terminal state after restart.
- The board failure assertions in
  `board_persistence_scenarios_test.go:89-189`: empty recovery, snapshot byte
  preservation, diagnostic fragments and code/path, corrupt-byte preservation,
  and absence-vs-corruption distinction.
- `requireLiveStreamIdentity`, `assertDurableProgressCounts`,
  `requireDispatchSummary`, `assertDispatchSummaryParity`, and
  `assertFactoryEventHistoryContinuity` in `logical_identity_test.go:374-781`:
  logical/live identity, checkpoint policy, progress, lifecycle timestamps,
  dispatch identity/status/kind, provider cancellation, and event-prefix
  continuity.

## CASE map

This map is the pre-change assertion-to-case denominator. A row marked
“not an independent selector” is intentionally a boundary or negative-path
condition named by the plan but not directly injected by the current source;
it remains unproven for the later story rather than being treated as a pass.

| Case | Pre-change selector/source witness | Observable assertion or characterization |
| --- | --- | --- |
| `CASE-H01` | RoundTrip and Missing; `startBoardPersistenceDaemon` (`board_persistence_process_test.go:56`) | `/status` becomes ready and a live default Factory Session is found before the scenario proceeds; cleanup is registered by `startBoardPersistenceDaemonProcess`. |
| `CASE-H02` | RoundTrip and Missing; `submitBoardPersistenceBatchThroughCLI`, `assertBoardList` | Batch acknowledgement has exact request ID/count; three Work items have exact IDs, names, state names/types, lineage, content and parent-child relation. |
| `CASE-H03` | RoundTrip; `runBoardPersistenceInitialGeneration` | One active dispatch and starting/running Worker Session are observed before clean stop; recording exists and is non-empty afterward. |
| `CASE-H04` | RoundTrip; `runBoardPersistenceRecoveryGeneration` and `waitForBoardRearmedDispatch` | Three Work items replay; old dispatch has one interruption; exactly one distinct active dispatch and Worker Session re-arm is observed. |
| `CASE-H05` | RoundTrip; release write, `waitForBoardDispatchResponse`, `assertBoardWork` | Re-armed dispatch has one response and no active dispatch; processing Work reaches complete with the sentinel/PASS worker output while the other Work remains exact. |
| `CASE-H06` | RoundTrip; `runBoardPersistenceSecondRestart` | One new Work item is accepted, four exact Work items persist, CLI list/show agrees with HTTP, and no active/starting Worker Session or phantom dispatch remains. |
| `CASE-H07` | Remap and History; `requireLiveStreamIdentity`, sync-preflight checks | Live ID/generation remap to the same logical key/backend scope; stale checkpoint is not reusable. |
| `CASE-H08` | Resume; `startInterruptedResumableSession`, resume assertions, provider call count | Completed child is not repeated; interrupted child resumes; dispatch count/IDs and provider calls remain exactly as asserted. |
| `CASE-H09` | History; before/after durable reads, dispatch summaries, event lists | Session/lifecycle/progress, two dispatches, and the ordered event prefix remain inspectable and stable after server restart. |
| `CASE-U01` | Missing; `TestBoardPersistenceCLIRestartAfterHardKillWithMissingBoardRecording` | Missing recording yields exactly empty Work, preserves snapshot bytes, and emits all four recovery-warning distinctions. |
| `CASE-U02` | Corrupt; `TestBoardPersistenceCLIRestartWithCorruptBoardRecordingFails` | Present malformed recording exits non-zero with exact structured guidance, preserves bytes, and does not claim recoverable absence. |
| `CASE-U03` | Board process helpers; `waitForBoardDaemonReady`, `waitForBoardPersistenceDaemonExit` | Early daemon exit fails with original stdout/stderr and exit diagnostics; corrupt startup is observed through the real child. |
| `CASE-U04` | All board polling and logical wait helpers | Existing bounded readiness, Work, event, Worker Session, snapshot, log, dispatch, durable-status, provider-block, and child-exit observations retain phase/last-observation failure diagnostics where implemented. |
| `CASE-U05` | Board `exec.CommandContext`/daemon context and logical blocking provider `Execute` | Child/provider cancellation remains tied to the parent context; cleanup joins the real child or observes `ctx.Err()`. |
| `CASE-U06` | Board daemon `stop`, `kill`, `cleanup`; support server test cleanup | Stop/kill/cleanup are guarded against duplicate signals, wait for child join, and retain private listener/process/temp ownership. |
| `CASE-U07` | GATE-SCOPE only | Not applicable: the isolated loopback has no authorization contract and no permission behavior is invented. |
| `CASE-U08` | `newBoardPersistenceScenario` setup and `t.TempDir` ownership | Setup errors fail the selector, but the current source has no injected “fail after allocation” selector; direct partial-setup failure proof is intentionally unresolved for story 002. |
| `CASE-R01` | Three independent `newBoardPersistenceScenario` calls and per-test `t.TempDir` resources | Current source has no shared mutable board state across selectors; the package run passed after each scenario’s cleanup path. Immutable build sharing is the only candidate that does not share this state. |
| `CASE-B01` | Missing; `waitForBoardStates(..., map[string]string{})` and result length check | Recovered public Work is exactly empty, not partial or stale. |
| `CASE-B02` | Board `assertBoardList`/`readBoardDispatchStates`; logical dispatch/event helpers | Duplicate/unexpected Work IDs fail; public dispatch request/response/interruption and event-prefix identity/order facts remain counted and checked. |
| `CASE-B03` | `reserveBoardPersistenceAddress`, board process helpers | Ephemeral loopback is reserved/released; ports 7437 and 7438 are rejected; real daemon process binds and later joins. |
| `CASE-B04` | Board expected maps and logical two-dispatch assertions | Current board counts are three then four Work items and logical workflows have two dispatches; no new capacity limit is asserted. |
| `CASE-B05` | `boardPersistenceWorkerConfig`, helper environment guard, helper selector | Normal package setup and helper-child execution are separate roles; helper emits sentinel and waits only for release, with no recursive package setup. The current normal package still performs three explicit builds. |
| `CASE-B06` | Package command plus three `buildBoardPersistenceBinary` call sites | One Go test package process runs all seven selectors, but the pre-change explicit CLI build count is three; daemon/server counts are characterized below and remain unchanged candidates until story 002 evidence. |

## Before-state topology and profile

Counts below are source-backed invocation counts for one `-count=1` package
process. “Factory fixture roots” are private directories created by support;
“production composition roots” are daemon or in-process server generations;
the built CLI client invocations are counted separately from those roots.

| Selector | Factory fixture roots | Explicit `go build ./cmd/factory` | Real daemon starts | In-process server starts | Built CLI subprocesses | SCRIPT_WORKER helper children | Lifecycle boundary |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| `TestBoardPersistenceWorkerHelper` | 0 | 0 | 0 | 0 | 0 | 0 from the normal package process | Helper-only path; the child role is exercised by board scenarios. |
| `TestBoardPersistenceCLIRestartRoundTrip` | 1 | 1 | 3 | 0 | 12 | 2 | Three real daemon generations and three clean stops. |
| `TestBoardPersistenceCLIRestartAfterHardKillWithMissingBoardRecording` | 1 | 1 | 2 | 0 | 1 | 1 | One hard-killed daemon, one cleanup kill. |
| `TestBoardPersistenceCLIRestartWithCorruptBoardRecordingFails` | 1 | 1 | 1 | 0 | 0 | 0 | One early-exit daemon and one exit observation. |
| `TestFactorySessionRestartRemapsLiveIDToLogicalIdentity` | 1 | 0 | 0 | 2 | 0 | 0 | Two production-composed server generations. |
| `TestFactorySessionResumeDoesNotRepeatCompletedDispatch` | 1 | 0 | 0 | 1 | 0 | 0 | One server with interrupt/resume lifecycle. |
| `TestFactorySessionHistoryIsPersistedAcrossRestart` | 1 | 0 | 0 | 2 | 0 | 0 | Two server generations over the same durable store. |
| **Total** | **6** | **3** | **6** | **5** | **13** | **3** | One Go test package process per command. |

The six board daemon starts and five in-process server starts are eleven
production composition generations. The three explicit builds are the
material redundant setup: each board scenario builds the same `./cmd/factory`
artifact independently. The thirteen built-CLI calls are twelve owned
round-trip operations (two batch submits, two list commands, seven show
commands, and one Worker Session list) plus one missing-recording submit. The
three helper children are two round-trip attempts and one missing-recording
attempt. The corrupt-recording scenario starts no helper because startup
fails before dispatch.

The board fixture allocates a private Factory directory, HOME directory,
release-file directory, recording path, and dynamic port per scenario. The
logical selectors allocate private Factory/session state and retain their
existing `UseMockWorkers` or `ProviderOverride` controlled edges. No mutable
scenario state is currently shared.

## Wait and timeout inventory

The package uses bounded observation because real daemon/process/filesystem
boundaries do not expose a test-owned readiness or commit channel. These are
polls over public HTTP, a real child exit, or durable files/logs; they are not
new fixed sleeps. Existing waits are recorded here so story 002 does not
silently remove a witness or add timeout padding.

| Observation/helper | Calls in one package run | Bound and cadence | Classification and evidence |
| --- | ---: | --- | --- |
| `waitForBoardDaemonReady` | 5 | 45s; 25ms ticker; 2s HTTP client timeout | Required process-bound `/status` readiness; reports early child exit and captured streams. |
| `waitForBoardSessionID` | 5 | 30s; 25ms ticker | Required public live-session discovery after each successful daemon start. |
| `waitForBoardStates` | 7 | 30s; 25ms ticker | Required public Work projection convergence: four round-trip observations and three missing-recording observations. |
| `waitForBoardActiveDispatch` | 1 | 30s; 25ms ticker | Required public event-derived active-dispatch witness. |
| `waitForBoardRearmedDispatch` | 1 | 30s; 25ms ticker | Required exactly-once interruption/distinct re-arm witness. |
| `waitForBoardDispatchResponse` | 1 | 30s; 25ms ticker | Required response/no-active-dispatch witness. |
| `waitForBoardWorkerObservation` | 2 | 30s; 25ms ticker | Required starting/running Worker Session identity witness. |
| `waitForBoardDispatchStates` | 1 normal call | 30s; 25ms ticker | Required final public event read; also used for timeout diagnostics. |
| `waitForBoardPersistenceSnapshot` | 1 | 30s; 25ms ticker | Required filesystem commit observation before destructive missing-recording setup. |
| `waitForBoardPersistenceLogMessage` | 1 | 30s; 25ms ticker | Required operator-warning observation after missing-board recovery. |
| `waitForBoardPersistenceDaemonExit` | 1 | 20s timer over `done` | Required corrupt-startup exit observation. |
| Board `stop`/`kill` joins | 5 | 20s timer over `done` | Required real process teardown: three clean stops and two hard-kill/cleanup joins. |
| `TestBoardPersistenceWorkerHelper` release observation | 3 child instances | 10ms ticker until release file | Required real SCRIPT_WORKER hold/release boundary; parent observes the public Work/dispatch result. |
| `readBoardEvents` request | Nested in event polls | 5s request context | Required retained Factory Event stream decode/count boundary. |
| `waitForFactorySessionDispatchStatus` | 4 | 5s; existing 10ms sleeps | Required logical dispatch status convergence; two calls each in resume and history. |
| `waitForDurableFactorySessionStatus` | 2 | 8s and 5s; existing 15ms sleeps | Required durable lifecycle convergence for resume and history. |
| `waitForExecuteBlocked` | 2 | 5s timer over provider channel | Required controlled provider cancellation setup. |
| `waitForCanceledExecute` | 2 | 5s; existing 5ms sleeps | Required provider context-cancellation observation. |
| `listFactorySessionEvents` | 2 | 20s request context | Required before/after retained event-prefix read in history. |

The three `time.Sleep` loops in `logical_identity_test.go` are pre-existing
bounded polling implementation (`10ms`, `15ms`, and `5ms`) with no new sleep
introduced by this story. The board polling helpers carry in-code reasons why
public/process/filesystem observation is required. There is no safe
characterization basis for replacing these waits with a sleep, removing a
process boundary, or increasing a timeout. Support-managed startup/cleanup
wait internals are not instrumented by this package and are marked unresolved;
they are outside the owned first candidate.

## Redundant-permutation analysis and selected candidate

The three board selectors independently call the same
`newBoardPersistenceScenario`, which in turn calls the same
`buildBoardPersistenceBinary` and `boardPersistenceFactoryConfig`. The
scenario-specific Factory directory, HOME, release file, recording, port,
daemon lifecycle, and helper state are materially different and must remain
private:

- The round trip needs three daemon generations, an interrupted recording,
  replay/re-arm, and a post-recovery Work submission.
- The missing-recording case removes only board history after a valid durable
  snapshot and requires a distinct hard-kill/reopen boundary.
- The corrupt-recording case must fail startup on a present malformed artifact
  and preserve its bytes; sharing any live or persisted scenario state would
  invalidate that distinction.

The first safe optimization candidate is therefore **share one immutable
package-owned built `you` binary across the three board scenarios**, with a
helper-child recursion guard keyed by the existing SCRIPT_WORKER environment
role. The candidate removes two explicit `go build` operations without sharing
Factory, HOME, recording, release, port, process, session, provider, or event
state. The helper child must perform only its sentinel/release behavior and
zero package builds. Package teardown must remove the binary after all
scenarios and helper children have joined.

No second optimization is selected at characterization time. Fixed polling is
not classified removable because each current board poll observes a real
boundary and has actionable diagnostics; logical provider/server state is
already isolated and uses existing controlled exceptions. A further pass is
authorized only if the post-candidate profile is non-improving and identifies
one owned, profile-backed reduction without weakening a case row.

## C09 preservation check

The merged C09 context correction is present in the current source:

- `runBoardPersistenceCLIWithFreshContext` in
  `board_persistence_cli_assertions_test.go:19-28` creates a new
  `context.WithTimeout(t.Context(), 30*time.Second)` and defers cancellation
  for each CLI operation.
- `submitBoardPersistenceBatchThroughCLI` in
  `board_persistence_cli_assertions_test.go:30-40` does the same for each
  submit.
- The round-trip call sites perform twelve independent operations: two batch
  submits, two `work list` calls, seven `work show` calls, and one Worker
  Session list. The missing-recording selector adds one independent submit.
- `runBoardPersistenceCLI` in `board_persistence_cli_test.go:13-22` consumes
  the supplied context; no scenario stores a cumulative CLI context.
- Daemon startup/build contexts remain `t.Context()` and are independent from
  CLI-operation contexts.

Therefore the profile records **13 fresh per-operation CLI contexts** and
proposes no cumulative deadline. The shared-binary candidate must retain this
ownership exactly.

## Evidence boundary and next gates

This gate proves the current seven-selector denominator, source hashes, public
assertion/case map, C09 context ownership, before-state topology, wait
inventory, and a safe first optimization candidate on local-real boundaries.
It does not prove:

| Unproven edge | Owning gate |
| --- | --- |
| One guarded shared binary, helper recursion prevention, and scenario/package cleanup | `GATE-SPINE` / story `...-002` |
| Complete all-selector preservation after the fixture change | `GATE-PACKAGE` / story `...-003` |
| Supported race behavior | `GATE-RACE` / story `...-003` |
| Final three-run performance median or measured-floor explanation | `GATE-PERF` / story `...-003` |
| Clean-room exact-head reproducibility | `GATE-LOOPBACK` / story `...-003` |
| Hosted PR behavior and terminal CI | `GATE-PR-CI` / review stage |
| Support-managed internal wait counts | Later bounded package profiling if material |

GATE-CHAR is complete for story 001. The historical sections below retain the
earlier review attempts, while the final exact-head section at the end of this
ledger is authoritative for stories 002 and 003 after the latest rebase.

## SHA-256 source register

These hashes bind the characterization to the exact pre-edit restart package
source. They are source evidence, not generated artifacts.

| File | SHA-256 |
| --- | --- |
| `board_persistence_cli_assertions_test.go` | `2221f8625d91d36ad377d9c940a5dece3e54b6e56a5143660927bf79e85a14c1` |
| `board_persistence_cli_observation_test.go` | `a4907d203858888c221f3cfb445748d3023398172e84e6059725f1cfdd04a691` |
| `board_persistence_cli_test.go` | `b2bcaa6739298cd971df969722d4e9e3ffc3adc61e0f4bde6bc518808c56937d` |
| `board_persistence_fixtures_test.go` | `a98cc7d77e5fe5529418304be31e4a5ce89b8eb54d21a1fa2a4d16d82269b19d` |
| `board_persistence_polling_test.go` | `5daeef99a2f58919b4934b816295f6f223171c67e9ab6337002aad304765a127` |
| `board_persistence_process_nonwindows_test.go` | `d2a71d0e2228aadf9da8556e1abaf7014af535eff6e18039198fb5bc17000286` |
| `board_persistence_process_test.go` | `477dd4c5f5d86c87840b3976252a19201f9baae55b265c2354e6c3929cd325bb` |
| `board_persistence_process_windows_test.go` | `d7169e285158e5824190a14341b392f3f931d178d8234645f43c8ded4e54761d` |
| `board_persistence_scenarios_test.go` | `c6229528fdb011010951cf3ca40103b3b89d466be4bc7de5c6f192786001fa7f` |
| `logical_identity_test.go` | `7b6569b537f7c0fdc85c03911700de5d143f7ac113a14757f2669376cdeac65c` |

## GATE-SPINE

- Story: `fto-c14-pkg-sessions-restart-002`
- Parent behavior: `BEH-C14-RESTART` — every restart, replay, persistence,
  failure, process, and cleanup witness remains while redundant intrinsic work
  is removed.
- Status: **PASS for the focused one-build restart spine and direct partial-
  setup cleanup proof**.
- Tested head: `977d6809f7f676bfc4d2d443a3e0cbeeb38ba4d2`, based on current
  `origin/main` `47285a02c116394b0bdf3078cdd59909dc12e825`.
- Dependency fidelity: `local_real` built CLI, real Windows daemon processes,
  loopback, filesystem, recording, and existing controlled provider edges.
- Cost: zero paid or remote-provider calls.

The focused procedure was:

```text
go test ./tests/functional/sessions/restart/... -count=1 -run '^TestBoardPersistenceCLIRestartRoundTrip$' -v
```

It exited `0`; `TestBoardPersistenceCLIRestartRoundTrip` passed in `25.89s`
and the package reported `ok ... 25.977s`. The test log showed one immutable
package path and `builds=1`, followed by three live daemon session IDs and the
initial active dispatch witness. The round-trip therefore crossed the real
submit, readiness, Worker Session, clean-stop, replay/re-arm, release-file,
response, persistence, and second-restart boundaries with the shared binary.

The direct CASE-U08 procedure was also run:

```text
go test ./tests/functional/sessions/restart/... -run '^TestBoardPersistenceSharedBinaryPartialSetupFailure$' -count=1 -v
```

It exited `0` in `0.095s`. The parent test launched an isolated copy of the
package test binary with a test-only failure immediately after `MkdirTemp`,
observed the child’s injected diagnostic and nonzero exit, decoded the reported
temporary directory and partial artifact, and verified that both paths were
absent after child exit. The scenario-start marker also remained absent, proving
that setup failure prevented scenario execution while `TestMain` still cleaned
the package-scoped partial state.

The implementation is limited to
`tests/functional/sessions/restart/board_persistence_process_test.go` and
`tests/functional/sessions/restart/board_persistence_scenarios_test.go`, plus
the test-only environment constants in
`tests/functional/sessions/restart/board_persistence_fixtures_test.go`:

- `sync.Once` creates one package-scoped temporary build directory and one
  `go build ./cmd/factory` artifact for the normal package process.
- The existing `YOU_BOARD_PERSISTENCE_HELPER=1` role is rejected by the build
  helper, and the helper selector asserts its own process-local build count is
  zero.
- `TestMain` removes the package build directory after all test scenarios and
  child-process joins, including partial-build failure cleanup.
- The partial-setup probe creates a deterministic partial artifact only in its
  child-process fault mode and observes its removal without selecting any board
  scenario.
- No existing waits, CLI contexts, assertions, public contracts, production
  code, shared support, or scenario-owned mutable resources changed. The
  reviewer-requested child-process observation has a bounded test-only timer
  with kill-and-join diagnostics; it does not add timeout padding to a product
  observation.

## GATE-REPEAT

- Story: `fto-c14-pkg-sessions-restart-002`
- Status: **PASS for repeated fixture use and cleanup**.
- Tested head: `977d6809f7f676bfc4d2d443a3e0cbeeb38ba4d2`.

The declared repeat procedure was run once:

```text
go test ./tests/functional/sessions/restart/... -count=3
```

It exited `0` with package elapsed `230.766s`. All eight selectors passed in
each of the three repetitions. The board logs reused one path and reported
`builds=1` in every normal package process; the `SCRIPT_WORKER` helper-role
assertion observed zero package builds; and the direct partial-setup probe
passed in each repetition. No failed or excluded repetition occurred in this
exact-head run.
Each repetition retained six real daemon starts, thirteen built CLI
subprocesses, and three helper children, for totals of eighteen daemon starts,
thirty-nine built CLI subprocesses, and nine helper children across the run.
Each scenario retained private Factory/home/recording/release/port state. The
package `TestMain` cleanup ran only after the repeated daemon and helper
processes had joined.

This gate proves the structural one-build fixture, helper recursion guard,
scenario isolation, direct partial-setup cleanup, and package teardown on the
exact current implementation head. It does not prove the final three-run
performance median, supported race result, clean-room loopback, terminal PR CI,
or merge; those remain owned by story `fto-c14-pkg-sessions-restart-003` and
review.

## GATE-PACKAGE

- Story: `fto-c14-pkg-sessions-restart-003`
- Status: **PASS for the complete final package behavior on the current
  implementation head**.
- Tested head: `15f8c2fd7bd4ab1d9aed55930ec2619086dc41c8`.
- Dependency fidelity: `local_real` built CLI, real Windows daemon processes,
  loopback, filesystem, recording, and the existing controlled provider edges.
- Cost: zero paid or remote-provider calls.

The exact final package procedure was run three times from PowerShell with the
same command required by the PRD:

```text
go test ./tests/functional/sessions/restart/... -count=1
```

All three runs exited `0` and exercised the eight selectors discovered by:

```text
go test ./tests/functional/sessions/restart/... -list '^Test'
```

The final package results were:

| Run | Outer stopwatch seconds | Package-reported elapsed | Exit | Observable result |
| ---: | ---: | ---: | ---: | --- |
| 1 | `60.00` | `51.955s` | `0` | Eight selectors passed; package/scenario cleanup completed. |
| 2 | `36.20` | `31.716s` | `0` | Eight selectors passed; package/scenario cleanup completed. |
| 3 | `40.13` | `35.566s` | `0` | Eight selectors passed; package/scenario cleanup completed. |

The one-build log observations from `GATE-SPINE` and `GATE-REPEAT` remain
applicable to this implementation: one package-scoped CLI build, zero helper
builds, six real board-daemon starts, five production-composed server
generations, thirteen built-CLI subprocesses, three helper children, and one
test-binary partial-setup probe child per normal package run. The three board
scenarios retain private Factory, HOME, recording, release-file, port, process,
and expected-state ownership. The pre-change seven-selector, 148 lexical
failure-call-site assertion denominator recorded by `GATE-CHAR` remains intact;
the added setup probe only adds direct cleanup assertions and removes no case
row or existing witness.

This gate proves complete local package behavior and topology on the final
implementation source. It does not prove a quiet-host timing bound, terminal
hosted CI, or merge.

## GATE-RACE

- Story: `fto-c14-pkg-sessions-restart-003`
- Status: **PASS**.
- Tested head: `15f8c2fd7bd4ab1d9aed55930ec2619086dc41c8` implementation source.
- Dependency fidelity: `local_real` Windows/amd64 race-enabled test process.

The supported race procedure was:

```text
go test -race ./tests/functional/sessions/restart/... -count=1
```

It exited `0` on `2026-08-29` with Go `go1.25.0`, `GOOS=windows`,
`GOARCH=amd64`, and `CGO_ENABLED=1`; the package reported `ok` in `63.771s`.
No race report, test failure, helper recursion, orphan process, or cleanup
failure was emitted. This proves the exercised package fixture and runtime
test paths are race-clean on this supported platform, not all possible
concurrency behavior or unsupported platforms.

## GATE-PERF

- Story: `fto-c14-pkg-sessions-restart-003`
- Status: **PASS via the permitted profile-backed measured-floor explanation;
  the local timing sample is below the 40% target and host-variable**.
- Baseline: authoritative unchanged-source median `44.1461053s` from
  `GATE-CHAR`.
- Final samples: `60.00s`, `36.20s`, and `40.13s`, all exit `0` under the exact
  package command; the sorted median is `40.1260758s`.
- Calculated change: `9.10%` reduction, so the explicit 40% timing branch is
  not claimed.

The final timing spread is materially wider than the first two samples on this
shared Windows host. The samples are retained exactly and are used only for the
required comparable median; topology and dependency fidelity remain the
primary bounded optimization evidence. No unrelated process was terminated or
modified.

The measured floor explanation is bounded by the post-change topology and
source-backed wait inventory:

1. The only redundant package setup identified by `GATE-CHAR` was three
   identical `go build ./cmd/factory` operations. The implemented bounded pass
   reduces that count to exactly one normal-process build, asserts zero helper
   builds, and keeps its artifact package-scoped until `TestMain` cleanup.
2. The final package still requires six real daemon generations, five
   production-composed server generations, thirteen built-CLI invocations,
   three helper-child release boundaries, one direct setup-failure child,
   private mutable scenario state, and every existing public HTTP, Factory
   Event, Worker Session, filesystem, process, cancellation, malformed-state,
   missing-state, remap, resume, and history observation. Removing any of
   those would remove or lower the fidelity of a CASE row.
3. The existing bounded polls observe public/process/filesystem/log
   boundaries and carry failure diagnostics. The source profile found no
   owned fixed sleep or timeout padding safe to remove, and no second
   profile-backed reduction can be admitted without changing a witness or
   broadening into shared support/production code. A standalone diagnostic
   `go build ./cmd/factory` measured `16.4055s`, corroborating that build reuse
   is the material owned setup reduction; it is not used as a package latency
   claim.

Therefore the package has completed its one authorized owned optimization pass
and retains the measured real-edge floor rather than inventing a second,
unprofiled optimization. The PR package result and hosted-runner timing remain
the primary delivery verdict for review. This gate does not claim portable
wall-clock behavior or terminal PR CI.

## GATE-SCOPE

- Story: `fto-c14-pkg-sessions-restart-003`
- Status: **PASS after the reviewer-requested rebase and scope audit**.
- Tested head: `15f8c2fd7bd4ab1d9aed55930ec2619086dc41c8`.

The current three-dot path set is limited to the three owned restart test files
and the two C14 evidence documents. `git diff --check` passes, and
`git merge-base --is-ancestor origin/main HEAD` exits `0`. The source scan found
the same fresh 30-second per-operation CLI contexts in
`board_persistence_cli_assertions_test.go`; no cumulative CLI context, new
sleep, timeout increase, generated file, public contract, production file,
shared support, workflow, or CI configuration is present.

## Evidence boundary and next gates

`GATE-PACKAGE`, `GATE-RACE`, and the bounded `GATE-PERF` explanation prove the
complete local behavior, supported race path, and owned topology decision on
the current implementation source. `GATE-SCOPE` is complete after the rebase;
`GATE-LOOPBACK` remains the final clean-room gate, and `GATE-PR-CI` remains
review-owned for terminal hosted CI and merge.

## GATE-SCOPE (final)

- Story: `fto-c14-pkg-sessions-restart-003`
- Status: **PASS** for the rebased implementation scope.
- Rebased implementation head: `15f8c2fd7bd4ab1d9aed55930ec2619086dc41c8`
  on current `origin/main` `1552d3e3ef114eeb68b1828d46db7d686ad1ef33`.

The final three-dot audit after synchronization reported only these owned
paths: the three restart test files and the two C14 evidence files under
`docs/internal/development/functional-test-optimization/`. `git diff --check`
passed. The final source scan confirms the C09 fresh per-operation
30-second CLI contexts are unchanged; no cumulative deadline, new sleep,
timeout increase, assertion loss, public contract, generated output,
production code, shared support, workflow, or CI configuration entered the
lane. `git rebase origin/main` completed without conflict before the clean-room
proof.

This gate proves authority, ancestry, owned scope, C09 preservation, and
assertion parity on the final local source. It does not prove terminal hosted
CI or merge.

## GATE-LOOPBACK

- Story: `fto-c14-pkg-sessions-restart-003`
- Status: **PASS for the local clean-room loopback**.
- Dependency fidelity: `clean local_real` detached worktree at
  `15f8c2fd7bd4ab1d9aed55930ec2619086dc41c8`; no implementation repair was
  made. The final pushed report refresh is documentation-only and does not
  alter the validated implementation tree.
- Report: `c14-pkg-sessions-restart-validation-loopback.md`.

The exact procedure was:

```text
git worktree add --detach <temporary-path> 15f8c2fd7bd4ab1d9aed55930ec2619086dc41c8
go test ./tests/functional/sessions/restart/... -count=1
git worktree remove <temporary-path>
```

The detached worktree was clean before removal. The package command exited `0`
and reported `32.456s`; all eight selectors passed, including the direct
partial-setup cleanup probe, real CLI,
daemon/server, restart, persistence, malformed/missing state, remap, resume,
history, cancellation, and cleanup witnesses. The canonical report records
the criteria, customer journey, evidence boundary, host-timing finding, and
review-owned edges without changing implementation.

This gate proves exact-head clean-room reproducibility and local integrated
behavior. It does not prove terminal PR CI, conflict-free merge, or future host
behavior.

## GATE-PR-CI

- Story: `fto-c14-pkg-sessions-restart-003`
- Status: **PENDING implementation handoff / review**.

The implementation handoff must push the final head, open or update the named
PR, confirm required CI has started on that exact head, and comment the
authoritative before median `44.1461053s`, final local median `40.1260758s`, and
the PR CI run URL. Review owns terminal CI, any later conflict request, and
merge; no CI result is committed to this branch.

## Historical review follow-up: exact rebased head `c9e3e727d5f6`

- Story: `fto-c14-pkg-sessions-restart-002` and
  `fto-c14-pkg-sessions-restart-003`
- Historical status: **BLOCKED pending exact post-rebase real-process package
  gates; superseded by the exact-head completion below**.
- Base: current fetched `origin/main` `47285a02c116394b0bdf3078cdd59909dc12e825`;
  `git merge-base --is-ancestor origin/main HEAD` exited `0` and the three-dot
  scope remains the three restart test files plus these two C14 documents.
- Code correction: the direct CASE-U08 child probe now starts the test binary,
  waits on one result channel with the test-only 20-second bound, kills on
  expiry, and joins before reporting phase/output diagnostics. The nonzero
  status, injected diagnostic, no-scenario marker, and partial-directory/
  artifact cleanup assertions remain unchanged.

The direct exact-head CASE-U08 procedure passed:

```text
go test ./tests/functional/sessions/restart/... -run '^TestBoardPersistenceSharedBinaryPartialSetupFailure$' -count=1 -v
```

It exited `0` in `0.108s` on Windows/amd64. The child reported the injected
failure, exited nonzero, wrote the allocated directory and artifact paths, and
the parent observed both paths plus the scenario marker absent after child
exit.

The required focused real-process procedure was attempted twice on this exact
head:

```text
go test ./tests/functional/sessions/restart/... -run '^TestBoardPersistenceCLIRestartRoundTrip$' -count=1 -v
```

Both attempts exited `1` only after the existing `10m0s` Go test bound, before
the scenario reached a daemon or any behavior assertion. The timeout stack was
inside `buildBoardPersistenceBinary` waiting for its unchanged
`exec.CommandContext(..., "go", "build", "-o", path, "./cmd/factory")` child.
The shared Windows host had dozens of unrelated Go test/build processes; a
separate real `go build ./cmd/factory` diagnostic produced a valid
`75,951,616`-byte artifact but its Go command also failed to return under that
host load. No package or scenario assertion failed, and no test timeout or
sleep was changed.

Consequently, the historical `GATE-SPINE`, `GATE-REPEAT`, `GATE-PACKAGE`,
`GATE-RACE`, `GATE-PERF`, and `GATE-LOOPBACK` observations above are retained
as prior-head evidence but do not satisfy the exact current-head gates. The
implementation was then blocked on a successful exact package build; the next
run was required to repeat the complete package, supported race, clean-room
loopback, and final PR handoff evidence without using a build substitute.

## Story 002 exact-head completion

- Story: `fto-c14-pkg-sessions-restart-002`.
- Status: **PASS**.
- Tested head: `977d6809f7f676bfc4d2d443a3e0cbeeb38ba4d2`, based on current
  `origin/main` `47285a02c116394b0bdf3078cdd59909dc12e825`.
- Dependency fidelity: `local_real` Windows/amd64 built CLI, daemon,
  filesystem, loopback, child-process, and existing controlled provider edges.
- Cost: zero paid or remote-provider calls.

The exact-head GATE-SPINE procedure passed:

```text
go test ./tests/functional/sessions/restart/... -run '^TestBoardPersistenceCLIRestartRoundTrip$' -count=1 -v
```

The command exited `0`; the representative real restart journey passed in
`25.89s`, with package elapsed `25.977s`. Its output showed one immutable
package-scoped CLI path (`builds=1`) and three isolated daemon session IDs. The
submit, readiness, Worker Session, clean-stop, replay/re-arm, release-file,
response, persistence, and second-restart witnesses all remained active.

The exact-head GATE-REPEAT procedure also passed:

```text
go test ./tests/functional/sessions/restart/... -count=3 -v
```

The command exited `0` with package elapsed `230.766s`. All eight selectors
passed in all three repetitions. Each normal package process reported one
shared-binary build, the helper branch observed zero package builds, and the
direct partial-setup probe passed with nonzero child status, injected
diagnostics, no scenario marker, and removal of the partial directory and
artifact. Scenario Factory, HOME, recording, release, port, process, and
expected-state ownership remained private; package teardown followed all child
and daemon joins.

These gates complete story 002's shared-binary, helper-recursion,
partial-setup, assertion-preservation, repeated-use, and cleanup criteria.
Story 003 still owns the final complete-package, race, performance, clean-room,
and PR handoff gates.

## Current exact-head validation after the latest rebase

- Behavior lane: `BEH-C14-RESTART`.
- Story: `fto-c14-pkg-sessions-restart-003`, with the exact shared-binary and
  partial-setup witnesses from story 002 rechecked as affected prerequisites.
- Implementation/test head: `cc08b6377fe1008c38b20330255887f7cd713633`.
- Base: `origin/main` `a9f108bef55aa9115191661c4b3fc3c2e5ebf619`.
- Environment: Windows/amd64, Go `go1.25.0`, `CGO_ENABLED=1`.
- Dependency fidelity: `local_real` built CLI, real daemon/server processes,
  loopback, filesystem, recording, child-process, and existing controlled
  provider edges.
- Cost: zero paid or remote-provider calls.

This section supersedes the earlier exact-head sections that reference
`977d6809`, `15f8c2fd`, `c9e3e727`, `1f6cd451`, `8a1f3edb`, or an earlier `origin/main`. The rebase
completed without conflict, and no implementation or shared-support repair was
made while collecting the evidence below.

### GATE-SPINE and CASE-U08

- Status: **PASS**.
- Focused procedure:

  ```text
  go test ./tests/functional/sessions/restart/... -count=1 -run '^TestBoardPersistenceCLIRestartRoundTrip$' -v
  ```

  Exit `0`; the real restart journey passed in `10.33s`, with package elapsed
  `10.397s`. Its output showed one package-scoped immutable CLI path with
  `builds=1`, three isolated daemon session IDs, and the initial active
  dispatch witness. The public submit, readiness, Worker Session, clean-stop,
  replay/re-arm, release-file, response, persistence, and second-restart
  boundaries all remained active.
- Direct failure-path procedure:

  ```text
  go test ./tests/functional/sessions/restart/... -run '^TestBoardPersistenceSharedBinaryPartialSetupFailure$' -count=1 -v
  ```

  Exit `0`; package elapsed `0.083s`. The child injected failure after
  temporary-root allocation, exited nonzero with the diagnostic, reported the
  allocated directory and partial artifact, never created the scenario marker,
  and was followed by assertions that both paths were removed.

### GATE-REPEAT

- Status: **PASS**.
- Procedure:

  ```text
  go test ./tests/functional/sessions/restart/... -count=3 -v
  ```

  Exit `0`; package elapsed `81.243s`. All eight selectors passed in all three
  repetitions. Each normal process reused one package build (`builds=1`), the
  helper path observed zero package builds, and the partial-setup child kept
  its nonzero status, diagnostic, no-scenario marker, and cleanup assertions.
  Scenario Factory, HOME, recording, release, port, process, and expected-state
  resources remained private, and package cleanup followed all joins.

### GATE-PACKAGE

- Status: **PASS**.
- Procedure:

  ```text
  go test ./tests/functional/sessions/restart/... -count=1
  ```

  Exit `0`; package elapsed `21.108s`. The direct package proof, helper selector, all
  three board persistence/restart selectors, logical remap, resume, and
  history selectors passed. The package log showed one normal-process CLI
  build and no helper recursion. The pre-change seven-selector and 148
  lexical failure-call-site denominator remains intact; the added probe only
  strengthens setup-cleanup coverage.

### GATE-RACE

- Status: **PASS** on the supported local platform.
- Procedure:

  ```text
  go test -race ./tests/functional/sessions/restart/... -count=1 -v
  ```

  Exit `0`; package elapsed `37.867s` on Windows/amd64 with Go `go1.25.0` and
  `CGO_ENABLED=1`. All eight selectors passed, including the bounded partial
  setup probe. No race report, helper recursion, orphan process, or cleanup
  failure was emitted.

### GATE-PERF

- Status: **PASS via the permitted profile-backed measured-floor explanation**;
  the local timing direction is host-variable and does not meet the 40% branch.
- Authoritative unchanged-source baseline: `44.1461053s`.
- Final isolated procedure: three sequential successful PowerShell stopwatch
  runs of `go test ./tests/functional/sessions/restart/... -count=1`, each
  with exit-code enforcement and captured package output.
- Final outer samples: `35.25s`, `33.85s`, and `31.82s`; sorted median
  `33.85s`, exit `0` for every run. The median is a `23.32%` reduction from the
  baseline, so no 40% reduction is claimed.
- The bounded optimization remains profile-backed: the source and runtime
  topology reduce three identical package CLI builds to exactly one normal
  process build, zero helper builds, and package teardown after all child and
  daemon joins. The remaining six daemon generations, five in-process server
  generations, thirteen built-CLI invocations, private scenario state, public
  observations, and bounded diagnostics are required witnesses. Existing
  polling waits are real HTTP/process/filesystem/log observations; no new sleep,
  timeout padding, cumulative CLI context, assertion deletion, or second
  unprofiled optimization was introduced.
- The three samples are retained despite shared-host variability. This is the
  permitted one-pass measured-floor explanation; the PR package result remains
  the primary delivery verdict and no unrelated process was modified.

### GATE-SCOPE

- Status: **PASS**.
- `git merge-base --is-ancestor origin/main HEAD` exited `0`.
- `git diff --check` passed.
- The three-dot diff contains only the three owned restart test files and the
  two C14 evidence documents. The source audit retains C09's fresh
  30-second context for every CLI operation and finds no cumulative deadline,
  new sleep, timeout increase, public contract, generated file, production
  file, shared-support file, workflow, CI configuration, or assertion loss.

### GATE-LOOPBACK

- Status: **PASS** for the clean local-real loopback.
- A detached clean worktree at implementation/test head
  `cc08b6377fe1008c38b20330255887f7cd713633` ran:

  ```text
  go test ./tests/functional/sessions/restart/... -count=1
  ```

  The command exited `0` with package elapsed `19.859s`; the temporary
  worktree was removed after the run. No implementation repair occurred. The
  complete package crossed the real CLI, daemon/server, restart, persistence,
  malformed/missing state, remap, resume, history, cancellation, and cleanup
  witnesses from a clean checkout.
- The canonical report is
  `c14-pkg-sessions-restart-validation-loopback.md`. It uses only PASS, FAIL,
  and BLOCKED statuses and leaves review-owned terminal CI and merge explicitly
  unproven.

### Repository quality gate

- `make test` exited `0` in `202.29s` on the immediately preceding
  code-equivalent rebased head. The latest ancestry-only rebase changed no
  owned implementation or evidence path. The unitlane report recorded 169
  tests, 168 passed, 0 failed, and 1 skipped; the skip was an existing Windows
  privilege/platform condition. No failure was caused by the restart-package
  diff.

### Current evidence boundary

The current exact-head local evidence proves the complete restart package,
bounded failure-path cleanup, repeated fixture isolation, supported race
behavior, the authorized measured-floor decision, current-main ancestry, and
clean-room reproducibility. It does not prove terminal hosted CI or merge;
those remain review-owned. The implementation handoff must push this final
head, confirm CI has started on that exact head, and post the before/after
medians plus the CI run URL in the PR conversation without committing CI-run
evidence.

## Post-review rebase and blocker refresh

Captured: `2026-08-30` after rebasing onto `origin/main` at
`5c997439079adf8761959897ba2fb70fdad8a1f9`. The exact tested source head was
`b876663d3dd01358733c507b62c98ec9c205c6ad`; the refresh is documentation-only
relative to the restart implementation.

The review-requested local gates were rerun after the rebase:

| Gate | Command/procedure | Result | Property proved |
| --- | --- | --- | --- |
| `GATE-PACKAGE` | `go test ./tests/functional/sessions/restart/... -count=1` | Exit `0`; package `38.044s` | Complete restart package and all eight selectors remain active. |
| `GATE-RACE` | `go test -race ./tests/functional/sessions/restart/... -count=1` | Exit `0`; package `57.796s` on Windows/amd64, Go `1.25.0`, `CGO_ENABLED=1` | Supported race behavior remains clean. |
| `GATE-LOOPBACK` | Detached clean worktree at the exact head, then `go test ./tests/functional/sessions/restart/... -count=1` | Exit `0`; package `40.880s`; temporary worktree removed | The exact rebased source reproduces the package proof without repair. |
| `GATE-REPOSITORY` | `make test` | Exit `0`; 169 Node subtests, 168 passed, 0 failed, 1 expected skip | The mandated repository quality target passes on the rebased head. |
| `GATE-ACP-DIAGNOSTIC` | `go test ./tests/functional/providers/acp -run '^TestACPSharedProcess$' -count=1 -timeout=90s` | Exit `0`; package `65.944s` | The prior hosted ACP timeout is not reproducible on this local host; no ACP or shared-support path was changed. |

The final post-rebase timing samples were `41.0144s`, `34.2973s`, and
`60.2404s`, all exit `0`; the sorted median is `41.0144s`, a `7.09%`
reduction from the unchanged-source baseline `44.1461053s`. The result does
not claim the 40% branch. The permitted one-build profile-backed measured-floor
explanation remains the applicable bounded optimization decision: one normal
package build, zero helper builds, private scenario state, and all existing
restart witnesses are retained.

The prior hosted run's failure was in untouched
`tests/functional/providers/acp`, where
`TestACPSharedProcess/javascript-acp-run` observed 15 rather than 16 peer
exits before its 20-second bound. The focused test passed on the current local
head and the restart diff does not alter ACP behavior, so no speculative
unowned fix entered this lane. The next required evidence boundary is the
matching-head hosted CI run after the final push.

## Current-main rebase correction and exact-head rerun

Captured: `2026-08-30` after the review-requested refresh of `origin/main`.
The branch rebased cleanly onto `origin/main`
`905d5fe06e54f2cc18fa9d5ed4f08d72e5d85739`; the exact tested head is
`65db217b6363aa02588a9a0a8c5aba1970294b8c`. The rebase changed no restart
implementation behavior and introduced no conflict. This section supersedes
the preceding stale-main handoff section for exact-head local evidence.

The refreshed ancestry and owned-scope checks passed:

```text
git merge-base --is-ancestor origin/main HEAD
exit 0
git diff --name-status origin/main...HEAD
five owned paths: three restart test files and two C14 evidence documents
```

The review-requested exact-head gates passed on Windows/amd64 with Go
`go1.25.0` and `CGO_ENABLED=1`:

| Gate | Command/procedure | Result | Property proved |
| --- | --- | --- | --- |
| `GATE-PACKAGE` | `go test ./tests/functional/sessions/restart/... -count=1` | Exit `0`; Go package `14.640s`, outer process `16.817s` | The complete restart package and all existing selectors pass on the current-main descendant. |
| `GATE-RACE` | `go test -race ./tests/functional/sessions/restart/... -count=1` | Exit `0`; Go package `32.123s`, outer process `35.137s` | Supported Windows/amd64 race behavior remains clean. |
| `GATE-LOOPBACK` | Detached clean worktree at `65db217b…`, then `go test ./tests/functional/sessions/restart/... -count=1` | Exit `0`; Go package `14.377s`, outer process `36.225s`; temporary worktree removed | The exact rebased artifact reproduces the package proof without repair. |

The exact-head final performance procedure ran three sequential successful
outer stopwatch samples of the package command:
`16.7596s`, `19.0249s`, and `17.4717s`, each exit `0`. The sorted median is
`17.4717s` versus the unchanged-source baseline `44.1461053s`, a `60.42%`
reduction. This is a measurement refresh only; no additional optimization
pass was made. The one-build topology remains one normal-process package CLI
build, zero helper builds, private scenario state, and joined package cleanup.

The clean-room path was confirmed absent after teardown
(`LOOPBACK_PATH_REMOVED=True`). These local gates do not claim terminal hosted
CI or merge; the next handoff step is to push this exact head, confirm the new
required CI run has started, and post the refreshed before/after medians and
CI URL in the PR conversation.
