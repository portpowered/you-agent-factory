# C09 sessions/restart contention characterization

## Scope and result

- Story: `functional-test-optimization-c09-sessions-restart-contention-001`
- Parent behavior: `BEH-C09-RESTART` — real Factory Session restart remains
  deterministic under contention while public Work, Factory Event, replay,
  persistence, process, and cleanup behavior remain exact.
- Captured: `2026-08-28` before the C09 implementation change
- Local source head: `254bb71db2bf5ef22280c643e02d70edf5a04286`
- Retained remote failure source: `d70d09c2b9dcb4f36799557be40935dcffc78131`
- Environment for local characterization: Windows `windows/amd64`, Go
  `go1.25.0`, local-real filesystem/process/loopback boundaries, zero paid
  calls.
- Status: **PASS — GATE-CHAR-001 characterization complete; correction and
  raised-parallelism behavior remain assigned to stories 002–003.**

This is an additive before-state record. It does not change the restart tests,
the shared CLI helper, production code, public contracts, generated output,
shared functional support, or the stability-owned history boundary.

The PRD names `docs/temp/functional-test-optimization.md` as its source plan,
but that path is absent from this checkout and has no reachable file on the
current refs. `progress.txt` was also absent at iteration start. The PRD,
repository standards, retained CI artifact, and current tested source were
used as the available authorities; the missing planning log is a provenance
gap, not silently reconstructed here.

## Exact retained failure

The retained GitHub Actions run was:

| Field | Value |
| --- | --- |
| Run | [33218340928](https://github.com/portpowered/you-agent-factory/actions/runs/33218340928) |
| Job | [99006961978 — Backend Functional Coverage](https://github.com/portpowered/you-agent-factory/actions/runs/33218340928/job/99006961978) |
| Tested SHA | `d70d09c2b9dcb4f36799557be40935dcffc78131` |
| Functional runner setting | `jobs=16` |
| Failed selector | `TestBoardPersistenceCLIRestartRoundTrip` |
| Selector elapsed | `30.190s` |
| Restart package elapsed | `84.625s` |
| Final recorded observation | `board_persistence_scenarios_test.go:61: isolated daemon live session ID: ...` |

The diagnostic artifact's package summary reports only this one failure in
`github.com/portpowered/infinite-you/tests/functional/sessions/restart`. The
same job also reported failures in unrelated functional packages; those are
not C09 evidence and are not attributed to this lane.

The final observation is source-based phase evidence, not a claim that the
failure occurred on readiness. In the tested source,
`startBoardPersistenceDaemon` starts the real child, waits for `/status`, waits
for a live default Factory Session, and then logs the session ID. The test
call is attributed to `board_persistence_scenarios_test.go:61` because the
helper is marked with `t.Helper()`. Therefore the retained artifact proves
that the first daemon reached readiness and session discovery before the
reported failure/cancellation. It does not identify which later CLI call or
phase consumed the remaining shared context budget.

The artifact records these sibling outcomes:

| Selector | Result | Elapsed |
| --- | --- | ---: |
| `TestBoardPersistenceWorkerHelper` | pass | `0.00s` |
| `TestBoardPersistenceCLIRestartRoundTrip` | **fail** | `30.190s` |
| `TestBoardPersistenceCLIRestartAfterHardKillWithMissingBoardRecording` | pass | `24.140s` |
| `TestBoardPersistenceCLIRestartWithCorruptBoardRecordingFails` | pass | `16.550s` |
| `TestFactorySessionRestartRemapsLiveIDToLogicalIdentity` | pass | `5.130s` |
| `TestFactorySessionResumeDoesNotRepeatCompletedDispatch` | pass | `2.610s` |
| `TestFactorySessionHistoryIsPersistedAcrossRestart` | pass | `4.460s` |

The test selector is an owned scenario in
`board_persistence_scenarios_test.go`. The similarly named
`board_persistence_cli_test.go` is a separate excluded surface containing the
low-level `runBoardPersistenceCLI` and `submitBatchThroughCLI` helper boundary.
That file and the branch/history of open [PR #2311](https://github.com/portpowered/you-agent-factory/pull/2311), titled
`test(restart): build you CLI once per package process`, remain outside C09.
PR #2311 currently points to `c106e67a131e10d3fe62f77abf063dc3a80e735c`; no
commit from its head is an ancestor of this worktree head. C09 does not claim
that PR's build-reuse work, import its commits, or alter its helper.

## Reproducible discovery and ordinary-host characterization

These commands were run before any C09 repository edit:

```text
go test ./tests/functional/sessions/restart -list '^Test'
```

Exit status was `0`. The exact current top-level inventory is:

```text
TestBoardPersistenceWorkerHelper
TestBoardPersistenceCLIRestartRoundTrip
TestBoardPersistenceCLIRestartAfterHardKillWithMissingBoardRecording
TestBoardPersistenceCLIRestartWithCorruptBoardRecordingFails
TestFactorySessionRestartRemapsLiveIDToLogicalIdentity
TestFactorySessionResumeDoesNotRepeatCompletedDispatch
TestFactorySessionHistoryIsPersistedAcrossRestart
```

The planning-time local-real selector run was:

```text
go test -count=1 -timeout=20m -run '^TestBoardPersistenceCLIRestartRoundTrip$' ./tests/functional/sessions/restart
```

It exited `0` in `12.921s` on the ordinary local Windows host. This proves
the selector is runnable and preserves the pre-change ordinary-host witness;
it does not prove the jobs=16 failure is reproduced, the correction works, or
cleanup is stable under raised parallelism.

## Source topology before correction

The target test currently creates one cumulative CLI lifetime context before
the fresh binary build:

```go
cliContext, cancel := context.WithTimeout(t.Context(), 30*time.Second)
defer cancel()
scenario := newBoardPersistenceScenario(t, cliContext)
```

`newBoardPersistenceScenario` first calls `buildBoardPersistenceBinary`, which
does a fresh `go build -o <temporary path> ./cmd/factory`, then creates the
isolated Factory, home, release file, recording path, and real SCRIPT_WORKER
configuration. The 30-second `cliContext` therefore begins before compilation.

The same context is stored in `boardPersistenceScenario.cliContext` and is
passed to every owned CLI call across the complete round trip:

1. Daemon 1 starts from the fresh built `you` binary, submits the initial
   three-Work batch, observes the processing dispatch and Worker Session, and
   stops cleanly. The real recording must be non-empty.
2. Daemon 2 starts from the same Factory/home/recording, the three Work items
   are replayed, the interrupted dispatch is re-armed exactly once, the
   helper is released, and the processing Work completes. The test then runs
   CLI `work list`, three CLI `work show` operations, and submits one new Work
   item before stopping.
3. Daemon 3 starts from the persisted board. The test reads four exact Work
   items, runs CLI `work list`, four CLI `work show` operations, reads public
   dispatch/Worker Session history, checks for no phantom active work, and
   stops.

This is eleven CLI invocations using the one stored context: two batch
submits, two list commands, six show commands, and one Worker Session list.
The daemon child processes themselves use `t.Context()` and are not changed by
this characterization. The later correction is intentionally limited to
fresh contexts at these owned CLI call sites; it must not change the build
count, process count, helper boundary, or public assertions.

## Independent bounds and real boundaries

The current source keeps these bounds independent of the stored CLI context:

| Boundary or observation | Current bound / mechanism | What it proves |
| --- | --- | --- |
| Fresh binary build | `go build` under `t.Context()` | Actual built `you` artifact is used; one build per scenario |
| Daemon process start | `exec.CommandContext(t.Context(), ...)` | Real OS process, signal, stdio, recording, and loopback boundary |
| Dynamic address | Reserve/release `127.0.0.1:0`; reject `7437` and `7438` | Isolated port allocation and process startup |
| Daemon `/status` readiness | ticker, `http.Client` 2-second request timeout, 45-second wait | Public server readiness and exit diagnostics |
| Live Factory Session discovery | ticker, 30-second wait | Public live default Factory Session exists |
| Work state convergence | public `/work`, ticker, 30-second wait | Exact Work set and state projection |
| Dispatch convergence | public Factory Events, ticker, 30-second waits | Active/re-armed/response dispatch facts |
| Worker Session convergence | public Worker Session list, ticker, 30-second wait | Starting/running and terminal attempt observations |
| Durable snapshot/log observation | filesystem/log polling, 30-second waits | Persistence commit and recovery warning boundaries |
| Factory Event stream read | per-request 5-second context | Retained event count, decode, and ordered public stream |
| Startup failure exit | real child `done` channel, 20-second wait | Non-zero corrupt-recording process exit and diagnostics |
| Clean stop / hard kill | real signal or kill, `done` channel, 20-second wait | Exit, join, and cleanup behavior |
| CLI operations | one shared 30-second context for all eleven calls | **Contention-sensitive cumulative lifetime budget** |

The bounded polling loops are required because the parent test has no direct
readiness, child-event, or filesystem-commit channel. They observe public
HTTP, real child exit, or durable filesystem/log boundaries and report the last
observation on timeout; they are not fixed sleeps or retries.

## Preserved executable-spine witness

The target remains the following customer/system journey:

```text
fresh built you CLI
  -> dynamic loopback real daemon 1
  -> public CLI batch submit and HTTP Work/Factory Session/Factory Event reads
  -> real recording and durable session snapshot
  -> clean interruption and daemon 2 replay/re-arm
  -> public completion and new Work submission
  -> daemon 3 persistence recovery and no-phantom-dispatch check
  -> exact CLI/HTTP/event/diagnostic assertions
  -> registered process cleanup and join
```

The public assertions that must remain active during story 002 include:

- exact batch acknowledgement request ID and Work count;
- exact Work count, unique IDs, names, request IDs, state names/types, trace
  lineage, content, and parent-child relation;
- real Factory Event decoding and dispatch facts: one request, one response,
  one interruption for the old dispatch, one distinct re-armed dispatch, and
  Worker Session associations;
- active dispatch and Worker Session observations before interruption, distinct
  Worker Session identity after replay, sentinel/PASS worker output, and no
  active or starting Worker Session after the second restart;
- CLI list/show and Worker Session list results agreeing with the public HTTP
  projection;
- non-empty recording after clean stop, exact durable snapshot preservation in
  the missing-board case, structured corrupt-recording diagnostics with bytes
  preserved, and recovery warnings only for absent board history; and
- real child exit, signal/kill handling, dynamic port behavior, idempotent
  cleanup, and process joins.

The other six selectors retain their independent witnesses for live-ID logical
remapping, completed-dispatch non-repetition, persisted Factory Session/Event
history, missing state, malformed state, and process diagnostics. Story 001
records their presence; story 003 owns the complete C09-H01-H09, C09-U01-U08,
and C09-B01-B05 matrix and final package proof.

## GATE-CHAR-001 report

| Property | Evidence | Result | Not proven |
| --- | --- | --- | --- |
| Exact selector and retained failure | Run `33218340928`, job `99006961978`, artifact `functional-test-diagnostics`, tested SHA and exact selector/timings above | PASS | A new remote reproduction is not required before implementation |
| Complete current selector inventory | `go test ./tests/functional/sessions/restart -list '^Test'`, exit `0`, seven selectors listed above | PASS | Raised-parallelism scheduling |
| Before topology and last phase | Current source inspection plus artifact's line-61 live-session observation | PASS as clearly labeled source inference | Exact later CLI call/deadline that terminated the failed run |
| Owned/excluded boundary | Source paths, live PR #2311 metadata, and non-ancestor check | PASS | Future PR #2311 semantic integration |
| Ordinary-host witness | Exact focused selector command, exit `0`, `12.921s` | PASS | Correction, repeated cleanup, jobs=16 behavior, terminal CI, or merge |

Story 001 is complete because the exact failure, before topology, executable
spine, selector inventory, and exclusion boundary are independently reviewable
without changing code or importing the excluded stability work. Stories 002
and 003 must retain this baseline and record corrected focused/package/CI
evidence instead of replacing it with a timing target.

## Story 002 correction and local evidence

- Correction commit: `4f4ada4f20` (`test(restart): bound each CLI operation independently`).
- The owned scenario no longer stores or receives a cumulative `cliContext`.
  Scenario setup, including the one fresh `go build`, runs under the test
  context; each owned CLI submit, `work list`, `work show`, and Worker Session
  list operation now creates and defers cancellation of a fresh
  `context.WithTimeout(t.Context(), 30*time.Second)` immediately around that
  subprocess call.
- The low-level `runBoardPersistenceCLI` and `submitBatchThroughCLI` helper
  boundary in excluded `board_persistence_cli_test.go` is unchanged. Process
  start/stop, readiness, polling, build count, daemon count, public
  assertions, and cleanup are unchanged.

GATE-FOCUSED-001 was attempted on the corrected head with:

```text
go test -count=3 -cpu=2 -timeout=20m -run '^TestBoardPersistenceCLIRestartRoundTrip$' ./tests/functional/sessions/restart
```

The command exited non-zero after `1200.094s` when the package timeout fired.
The goroutine dump shows the test had not entered the scenario: it was blocked
in `buildBoardPersistenceBinary` at `board_persistence_process_test.go:49`
waiting for the fresh `go build`. The shared Windows host had many concurrent
Go test/build processes. No daemon, CLI operation, restart assertion, or
cleanup assertion was reached, so this is contaminated environmental evidence
and does not prove or disprove the corrected round trip. The post-change
`go test ./tests/functional/sessions/restart -list '^Test'` output enumerated
the unchanged seven selectors and printed `ok` before its wrapper was
interrupted after the result had been emitted; the full focused run itself
already compiled the edited package before reaching the build wait.

Story 002 remains open pending one successful local-real focused repeat.
Raised-parallelism CI, the full package proof, diff/ancestry audit, and clean
room validation remain story 003 evidence.
