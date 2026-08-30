# C14 CLI batch-run cold-start characterization

Status: CHAR-001 PASS for the retained characterization story. Stories 002
and 003 own production deferral, repeat/race evidence, the post-change
threshold, clean-room validation, and PR delivery.

## Scope and evidence boundary

- Parent behavior: BEH-001 — Fast compatible batch cold start.
- Story: cli-batch-run-cold-start-latency-001.
- Frozen source head: 47285a02c116394b0bdf3078cdd59909dc12e825.
- The worktree was clean at the frozen head before these characterization
  tests and this ledger were added. prd.json contained no operatorAmendment;
  progress.txt was absent at iteration start.
- This story changed no production code, public/internal contracts, API or
  generated files, functional-test source, Factory Runtime transitioner, or
  pricing file. The package witnesses were added to existing package test
  files; no new pkg/wire file was created.
- Dependency fidelity is local-real for the compiled CLI probe and controlled
  for package witnesses. The accepting mock uses the fixture's local
  SCRIPT_WORKER echo command; paid or remote provider calls: 0.
- This ledger is the characterization denominator. It does not claim the
  optimized path or the 50% performance threshold.

## Environment and executable inventory

Measurements were collected on 2026-08-29 from this worktree:

~~~
Go: go1.25.0 windows/amd64
OS: Microsoft Windows NT 10.0.26200.0
PowerShell: 7.6.5
Logical processors: 24
GOMOD: C:\Users\andre\work\portos\infinite-you\.claude\worktrees\cli-batch-run-cold-start-latency\go.mod
GOCACHE: C:\Users\andre\AppData\Local\go-build
GOMAXPROCS: unset
CI: unset
~~~

The pre-change executable was built once with:

~~~
go build -o .artifacts/batch-coldstart-probe/you.exe ./cmd/factory
~~~

Binary SHA-256:

~~~
3DE0EB21647D4CC04B47E1C9894DBB45511247AFA27A82E1F934AB1EFBCA7FCC
~~~

Fixture SHA-256 inventory:

~~~
AD247CDAD555A29A73149E109B57AD3270FB1F705E3C805C9805B55493B9F978  fixture/factory/factory.json
CA3828653379C6BA4DA77FA2C887C03EBE65A846ADFB4FDAA2E7C2083D84DCC4  fixture/one-work.json
69A0CE360F450CF9E71D4C9935345D1B2CF6A734A4E79DAE534E836F6218D3C3  fixture/accept.json
5AE553B7C80C602644EA46DB813C9C59368B26DCF3AF2286026A5D57026D55BF  fixture/factory/workers/mock-worker/AGENTS.md
A585E435784D074BE950077390EDD12795C0DCA07DC3158C9723133251AA1835  fixture/factory/workstations/process-prompt/AGENTS.md
~~~

The fixture contains one prompt-task Work type, initial init, terminal
complete, failed failed, one mock-worker, one process-prompt Model
Workstation, and one one-work request. The accepting mock selects the
mock-worker/process-prompt pair and the local worker command is echo.

## Exact compiled-CLI baseline

After resolving the built executable to an absolute $probeBinary, the
working directory was the fixture directory and each invocation was a fresh
child process with an isolated HOME/USERPROFILE and
YOU_NO_BROWSER_OPEN=1. The exact argument vector was:

~~~
run --work one-work.json --with-mock-workers=accept.json --no-record --quiet
~~~

The three invocations were started sequentially as three independent OS
processes. Wall time was a monotonic PowerShell Stopwatch around each child;
stdout and stderr were redirected and compared byte-for-byte. Results:

| Run | Wall time | Exit | Stdout | Stderr | Process after wait |
| ---: | ---: | ---: | --- | --- | --- |
| 1 | 1630.593 ms | 0 | Batch completed successfully.\n (30 bytes) | empty | HasExited=True |
| 2 | 1165.540 ms | 0 | Batch completed successfully.\n (30 bytes) | empty | HasExited=True |
| 3 | 1318.610 ms | 0 | Batch completed successfully.\n (30 bytes) | empty | HasExited=True |

Sorted wall times are 1165.540 ms, 1318.610 ms, and 1630.593 ms; the
pre-change median is therefore 1318.610 ms (1.318610 s). All three
children returned exit code zero, and no child process or listener was
observed after each process completed. The batch request carried Port=0, so
the host path did not request a listener.

The durable snapshot written by the current runtime after the probe is
.artifacts/batch-coldstart-probe/fixture/.you-agent-factory/durable-sessions/~default.json
(SHA-256
B64F91171FF736F4B3A246E44C35BE431CA7C848DC1C4AA22C37890DF3D25B27). Its
petri_token_summary contains exactly the configured Work identity:

~~~
tokenId=one-work
workId=one-work
workTypeId=prompt-task
placeId=prompt-task:complete
state=complete
outcome=ACCEPTED
transitionId=process-prompt
~~~

This is the direct terminal-Work witness for the completed probe sequence.
The durable session envelope itself retains its existing RUNNING session
status while the Work token is terminal; this ledger does not reinterpret that
session-level value as a terminal-session claim.

An earlier harness attempt used PowerShell's reserved $HOME variable and
therefore did not isolate the child environment. It failed during unrelated
packaged-factory staging with CURRENT_FACTORY_INVALID; it is excluded from
the baseline median and retained here as a harness-contamination note. No
global user-home or shared staging state was changed.

## Phase attribution

The diagnostic profile was collected with the profile path expanded before
the command was passed to Go:

~~~
go test -json ./tests/functional/sessions/root_composition -run ^TestRootBuildProcessIsInertAndReusableAcrossFactorySessions$ -count=1 -timeout=5m -cpuprofile=.artifacts/batch-coldstart-probe/baseline-root-composition.prof
go tool pprof -top -cum -nodecount=25 .artifacts/batch-coldstart-probe/baseline-root-composition.prof
~~~

The focused root-composition test passed (go test package time about
4.104 s; the selected test reported about 3.89 s). The profile is controlled
package integration evidence, not an exact compiled-CLI timing sample. It had
3.83 s of CPU samples over about 4.02 s profile duration. The largest
cumulative attributable chain was:

~~~
Process.Execute                                      2.28 s cumulative (59.53%)
  CLI run command / runFactoryWithOptions             2.20 s cumulative
  Initializer.InitializeSystem                        2.17 s cumulative (56.66%)
    system-initialization workflow                    2.17 s cumulative
      EnsurePackagedFactories                         2.15 s cumulative (56.14%)
        createManagedPackagedFactory                  1.66 s cumulative (43.34%)
~~~

runtime.cgocall was the largest raw sample (3.25 s, 84.86%) on this Windows
host. It is retained as host/runtime overhead, not treated as a removable
product phase. The profile-backed removable seam selected for story 002 is
the system-initialization packaged-Factory installation performed before this
custom batch executes.

Phase coverage at this story's diagnostic resolution:

| Phase | Evidence | Current disposition |
| --- | --- | --- |
| root.BuildProcess | Root-composition test and Process.Execute caller chain | Construction-specific leaf is below this profile's useful resolution; no second graph was observed. |
| CLI construction/parsing | Process.Execute → CLI run-command chain | Present in the caller chain; no independent leaf bucket. |
| Factory Session preparation/opening | Existing runtime-opening package witnesses; not a distinct top-25 profile bucket | Below resolution in this diagnostic profile. |
| Models/provider catalog or scope | Existing injected-owner tests; no top-25 owner bucket | Below resolution in this diagnostic profile. |
| Durable-state opening | Existing persistence/opening witnesses; no top-25 owner bucket | Below resolution in this diagnostic profile. |
| Lifecycle activation | Initializer.InitializeSystem and existing process-lifecycle order witnesses | Explicitly characterized; startup contribution is included in the initialization chain. |
| Batch Work execution | Exact compiled probe output/exit plus persisted terminal token | Observable completion is proven; execution leaf is not separately timed by this profile. |
| Close | Exact child exit/process observation plus package cleanup witnesses | Close leaf is below profile resolution; package close counts/order are explicit below. |

The profile is sufficient to attribute the packaged-installation seam and to
prevent story 002 from selecting an unmeasured unrelated constructor. It does
not prove optimized performance, lazy concurrency safety, or full functional
behavior.

## Package characterization witnesses

The following package-local tests were added without changing production
behavior:

- pkg/wire/runtime_inputs_test.go — batch and hosted request values remain
  RuntimeModeBatch; batch carries Port=0, AutoPort=false, Pprof=false, and
  mock-worker/work-file values, while hosted input carries the explicit
  127.0.0.1:8123, auto-port, and pprof values.
- pkg/wire/system_initialization_composition_test.go — the current system
  initialization operation passes the exact home directory, invokes its owner
  exactly once, and preserves a controlled failure identity.
- pkg/services/factory_sessions/internal/processlifecycle/batch_server_characterization_test.go
  — batch roles are runtime sidecar, workers sidecar, and application
  transport; the hosted shape adds Factory visualization sidecar. Successful
  start/stop call counts are one each, batch listener starts/stops are zero,
  hosted readiness/listener starts/stops are one, and the runtime close is
  exactly once. Worker-start and hosted-transport-start failures preserve the
  sentinel, do not stop an unstarted transport, unwind prior roles in reverse,
  and close the runtime resource once. An invalid plan activates nothing.

The focused characterization command was:

~~~
go test ./pkg/wire ./pkg/services/factory_sessions/internal/processlifecycle -run TestBatchColdStartCharacterization -count=1 -timeout=5m
~~~

It exited 0; both packages passed. Existing direct host-owner tests also
passed for the actual readiness/close boundary:

~~~
go test ./pkg/services/factory_sessions/internal/runtimehosting -run 'TestServiceRun(HostsAPICompletesStartupAndWaitsForRuntime|DoesNotReportReadinessWhenRuntimeStartupFails|ReadinessFailureTransitionsRuntimeStartupFailure|CancellationStopsRuntimeBeforeTransport)$' -count=1 -timeout=5m
~~~

That command exited 0 and passed the hosted readiness, startup-failure,
readiness-failure, and cancellation ordering witnesses. Existing
Factory Session/process lifecycle tests remain the direct references for
recording flush, replay/opening, worker cleanup, and reverse shutdown.

## Authority, blockers, and remaining edges

The selected seam is within lane authority: the existing
SystemInitializationOperation is injected into Initializer, and its
profile-backed packaged-installation work is the first bounded candidate for
story 002. Story 002 must preserve the one wire.InjectBundle graph and the
existing RunIntent, batch/server, recording, replay, transition, and cleanup
contracts.

The following edges are intentionally unproven and delegated to later
stories:

- optimized delivered-binary median and exact before/after arithmetic
  (GATE-PERF-001);
- first-demand concurrency, cancellation, retry, capacity, repeated
  Process.Execute, and exactly-once lazy close (GATE-RACE-001);
- recording/replay determinism and complete unchanged functional inventory
  (GATE-SERVER-001, GATE-FUNC-001);
- clean rebased delivered artifact and validation loopback (GATE-LOOP-001);
- required CI, review, and merge (GATE-PR).

No browser criterion applies to this backend/CLI story. No production
optimization or threshold success claim is made by CHAR-001.

## Story 002 — profile-selected batch system-bootstrap deferral

Status: the selected optimization is implemented and has bounded behavioral,
repeatability, race-selector, and delivered-binary evidence. Final clean
rebase, validation loopback, pull request publication, and review handoff are
story 003 work.

### Implementation boundary

`pkg/transports/cli/root_factory.go` now skips the existing system-initialization
operation only for the profile-selected finite batch shape: a Work file,
mock workers, no recording, no replay/resume, no server/site/listener,
non-continuous execution, no bootstrap, and no named Factory. Explicit
`--dir`, `--factory`, or `--named` flag provenance retains initialization even
when the parsed selection has already populated the same fields. Real workers,
recording/replay, hosted execution, continuous execution, bootstrap, named
Factories, and explicit Factory selections remain on the existing startup
path. The change reuses the existing `RunConfig`, `StartupPreparation`,
`CommandFactory`, and single `wire.InjectBundle` graph; it does not add a
second graph or a public configuration mode.

The changed-path tests are in
`pkg/transports/cli/batch_cold_start_deferral_test.go`. They table-test the
deferral boundary, assert that the deferred initializer is not called for the
exact command shape, assert that demanded initialization preserves a sentinel
failure and exactly-one call, and cover explicit Factory path/directory,
recording, replay, server, continuous, real-worker, bootstrap, named-Factory,
and listener exclusions. The existing process-lifecycle characterization
remains in
`pkg/services/factory_sessions/internal/processlifecycle/batch_server_characterization_test.go`.

### Acceptance evidence

`GATE-PERF-001: PASS for the profile-selected exact batch path.` The final
compiled binary was
`.artifacts/batch-coldstart-probe/post-optimized-b08fbd550e.exe`, SHA-256
`DED26C02B440D3F64009CB00EC823C3B01B1A765761419AA83FE0F8FA31F064B`. Three
fresh child processes used isolated `HOME`/`USERPROFILE` directories and the
exact command below from a copied fixture:

~~~
run --work one-work.json --with-mock-workers=accept.json --no-record --quiet
~~~

The post-change wall times were 221.779 ms, 213.539 ms, and 223.867 ms;
sorted median 221.779 ms. Against the characterized pre-change median of
1318.610 ms, this is 1096.831 ms and 83.18% lower. All three processes exited
0 with exact stdout `Batch completed successfully.\n` (30 UTF-8 bytes) and
empty stderr. Each persisted snapshot contained `one-work` at
`prompt-task:complete`, state `complete`, outcome `ACCEPTED`; each snapshot
reported session status `RUNNING`, matching the existing CLI contract. The
run-1 snapshot artifact was
`.artifacts/batch-coldstart-probe/post-optimized-b08fbd550e-final2/run-1/.you-agent-factory/durable-sessions/~default.json`,
SHA-256
`1EA613ABD8FAC9D73D3388A7F95CC315FBD9921079D7CB08F087E27D2562A6A6`. Each
child had exited after wait, and each isolated packaged-Factory directory had
zero files. Fixture hashes were unchanged: factory
`AD247CDAD555A29A73149E109B57AD3270FB1F705E3C805C9805B55493B9F978`, Work
`CA3828653379C6BA4DA77FA2C887C03EBE65A846ADFB4FDAA2E7C2083D84DCC4`, mock
workers `69A0CE360F450CF9E71D4C9935345D1B2CF6A734A4E79DAE534E836F6218D3C3`.

`GATE-SPINE-001: PASS for the selected CLI and existing lifecycle boundary.`
The following final focused package run exited 0:

~~~
go test ./pkg/transports/cli ./pkg/services/factory_sessions/internal/processlifecycle -count=1 -timeout=10m
~~~

The exact command path, deferred initializer call count, demanded failure
identity, startup role order, reverse unwind, readiness/listener, and
exactly-once close witnesses all passed. `make pkg-maint` also exited 0 after
the test-shape refactor, with the repository limits of 1000 lines per file,
100 lines per function, and cyclomatic complexity 15 or less.

`GATE-RACE-001: PASS for changed selectors; broad lane remains qualified.`
The CLI deferral selectors passed the repeated race run (five repetitions),
and the process-lifecycle characterization passed under the race detector.
A broader race command was also run:

~~~
go test -race ./pkg/transports/cli ./pkg/transports/cli/run ./pkg/initializer/process ./pkg/initializer/application -count=1 -timeout=10m
~~~

That broader run was not green because the unrelated existing
`TestServeACPCommand_CancellationClosesStdinToUnblockRealServerMidRead`
failed with `Execute() did not return after cancellation while stdin remained
open and idle`; a direct selector rerun reproduced the same failure. This
batch change does not claim the first-demand concurrency, cancellation, retry,
capacity, repeated `Process.Execute`, or lazy-close edges from that failure.

`GATE-SERVER-001: PASS for existing direct preservation witnesses.` Existing
recording/replay, runtime-hosting readiness/failure/cancellation, and process
lifecycle tests passed in the focused and unit lanes. The demanded server
sentinel test proves the predicate does not bypass required initialization;
the full hosted/recording/replay inventory remains an unchanged-path risk to
be looped in story 003.

`GATE-FUNC-001: PASS for the final unit lane; functional clean-room result is
contaminated.` `make test` exited 0 with 168 tests passed, one
platform-conditional skip, and zero failures. The exact fresh functional
command `make test-functional-fresh` could not provide clean-room evidence:
the shared packaged-Factory staging owner
`C:\Users\andre\.you-agent-factory\factories\.you--full-flow.staging-owner`
was held by pid 6224 for backend/full scope
`local-0fbcdede-35ac-44d2-b02f-5888b9b8a1`, with liveness indeterminate. The
run reported installation contention, `process API server starter was never
invoked`, context cancellations, and unrelated MCP/session/provider/model/
recording failures. No shared-host cleanup or unrelated production repair was
performed. `make lint` was attempted; UI lint/deadcode could not run because
the environment lacks the Biome and Knip binaries, while the targeted
`make pkg-maint` gate passed after the maintainability fixes. Full lint is not
claimed.

`GATE-LOOP-001: NOT YET PROVEN.` The final source tree has not yet been
rebased in a clean room, run through the complete story-003 validation
loopback, pushed, or published as a pull request. Those are the remaining
acceptance edges; no browser criterion applies.
