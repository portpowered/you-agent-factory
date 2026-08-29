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

GATE-CHAR is complete for story 001. Stories 002 and 003 remain incomplete;
this ledger must be extended with their focused, repeat, package, race,
performance, scope, clean-room, and PR handoff evidence rather than replacing
the before-state record.

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
