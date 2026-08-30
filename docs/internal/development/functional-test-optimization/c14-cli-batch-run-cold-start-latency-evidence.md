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
