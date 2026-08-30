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

---

## Story 003 — final rebased artifact validation

The final validation head was
`9c6d427fbf20e04db85056fb16ad223dbb91c2c8`, rebased onto
`a9f108bef55aa9115191661c4b3fc3c2e5ebf619`. During the validation run,
`origin/main` advanced to `5c997439079adf8761959897ba2fb70fdad8a1f9`; no
final push was made.

`GATE-PERF-001: PASS.` The final binary was
`.artifacts/batch-coldstart-probe/validation-9c6d427fbf20.exe`, SHA-256
`1E87BFCC6A79B5FD56BAE60D95E25A1A2C0B4736C027C66C03878BC249BE9B86`.
Using the exact command

~~~
run --work one-work.json --with-mock-workers=accept.json --no-record --quiet
~~~

in three isolated child fixture/home directories produced wall times
`538.810 ms`, `479.094 ms`, and `492.866 ms`; sorted median `492.866 ms`.
Against the pre-change median `1318.610 ms`, the reduction is `825.744 ms`
and `62.62%` (`0.3738x` the baseline). All three exited `0`, wrote exact
stdout `Batch completed successfully.\n` (30 UTF-8 bytes), wrote empty stderr,
and had no remaining packaged-Factory files. Each durable snapshot contained
`one-work` at `prompt-task:complete`, state `complete`, outcome `ACCEPTED`,
transition `process-prompt`, and session status `RUNNING`.

The final snapshot hashes for runs 1–3 were, respectively,
`877D8C340606A9132B5694C01E647164B20A09E64B9CC6AFA80FF56CEF7E4805`,
`F8A2DDAC607997D362D3C15287B2CCDE791E9E09B37248395AF95083D78DD6D6`, and
`D597533CB72D9D9422B231C64A3266845C2A9003EC5C66F93DF723365B8BA1A2`.
Fixture hashes were unchanged from story 001: Factory
`AD247CDAD555A29A73149E109B57AD3270FB1F705E3C805C9805B55493B9F978`, Work
`CA3828653379C6BA4DA77FA2C887C03EBE65A846ADFB4FDAA2E7C2083D84DCC4`, and
accepting mock workers
`69A0CE360F450CF9E71D4C9935345D1B2CF6A734A4E79DAE534E836F6218D3C3`.

`GATE-SPINE-001`, `GATE-RACE-001`, and `GATE-SERVER-001` are PASS for the
changed package and direct-consumer evidence. Normal and race-focused CLI/
process-lifecycle tests passed; direct initializer, application, Wire,
runtime-hosting, recordings, replay, and unchanged workers/mock selectors all
passed. The package-count regression caused by the standalone test file was
resolved by folding those tests into the existing CLI run-flags test file;
`make pkg-maint`, `make backend-size`, and `make pkg-file-count` then passed.

`GATE-FUNC-001: BLOCKED.` `make test-functional-fresh` was run once with the
normal environment and once with isolated `HOME`/`USERPROFILE`. The normal run
was blocked by the existing global
`C:\Users\andre\.you-agent-factory\factories\.you--full-flow.staging-owner`
(PID `6224`, backend scope
`local-0fbcdede-35ac-44d2-b02f-5888b9b8a9e1`, liveness indeterminate). The
isolated run removed that global conflict but still encountered concurrent
temporary packaged-Factory staging owners in unchanged server/session/event
tests and a Windows `Access is denied` failure removing a temporary
`you.exe` in `tests/functional/recordings/process`. No lease or unrelated
production/test-functional source was removed or repaired.

`Quality gates: BLOCKED.` Final `make test` passed with 447 Go packages (one
platform-conditional skip) and 169 workflow tests (168 pass, one skip).
Backend/package/boundary/structure/cycle/catalog/generation checks passed.
The final `make lint` reports only environment/baseline failures: UI Biome is
missing, UI Knip is missing, and deadcode reports baseline `3074` versus
current `3072`. No Wire authorship changed, so `make generate-wire` was not
applicable.

`GATE-LOOP-001: BLOCKED.` The canonical
[validation-loopback report](validation/c14-cli-batch-run-cold-start-latency-validation.md)
is present and the final binary/performance proof is complete, but the full
functional and lint gates remain blocked, and `origin/main` advanced after the
tested rebase. Final rebase, push, PR creation, CI start, and review feedback
handling remain unperformed. No browser criterion applies.

## Story 003 follow-up — serialized functional and lint environment

The prior parallel functional failures were cleared by the bounded command
`make FUNCTIONAL_DEFAULT_JOBS=1 test-functional-fresh` with isolated artifact
`HOME`/`USERPROFILE` and `YOU_NO_BROWSER_OPEN=1`. The complete unchanged
`tests/functional/...` inventory exited 0; all enumerated packages, including
packaged factories, server/session/event paths, recordings/process, and
`workers/mock`, passed without staging contention, Windows binary cleanup
failure, or source edits.

The pinned ignored UI dependencies were installed with
`cd ui; bun install --frozen-lockfile`; `make ui-lint` and `make ui-deadcode`
each exited 0. Native Windows `make lint` passed every target except the
platform-sensitive Go deadcode comparison (3072 Windows build-tag findings
versus the Linux baseline's 3074). The same `make deadcode` under WSL Go 1.25
exited 0 with the committed baseline. The shared baseline was not changed.

The current remaining handoff edges are the final rebase onto current
`origin/main`, a fresh delivered-binary probe and minimum required gates on
that head, and PR/CI start.

---

## Story 003 — current-main clean-room validation

The delivered implementation was rebased successfully onto current
`origin/main` at `905d5fe06e54f2cc18fa9d5ed4f08d72e5d85739`; the resulting head
was `035b59e2ddc461718490a60023546fe0cc81d10e`. No functional, API, Wire,
generated, or test-source changes were made during the rebase or validation.

`GATE-PERF-001: PASS.` A fresh `./cmd/factory` binary built from that head at
`.artifacts/batch-coldstart-probe/validation-035b59e2dd.exe` had SHA-256
`1E87BFCC6A79B5FD56BAE60D95E25A1A2C0B4736C027C66C03878BC249BE9B86`.
The exact command

~~~
run --work one-work.json --with-mock-workers=accept.json --no-record --quiet
~~~

ran in three isolated fresh child fixture/home directories in `128.655 ms`,
`131.264 ms`, and `174.156 ms`; sorted median `131.264 ms`. Against the
characterized `1318.610 ms` median, this is `1187.346 ms` and `90.05%` lower
(`0.0995x` the baseline). Every child exited `0`, wrote exact stdout
`Batch completed successfully.\n` (30 UTF-8 bytes), empty stderr, and no
packaged-Factory files. Each durable snapshot contained `one-work` at
`prompt-task:complete`, state `complete`, outcome `ACCEPTED`, and transition
`process-prompt`; all children had exited after the snapshot was inspected.
The snapshot hashes for runs 1–3 were
`E664C906AD6A0FD78D6746DA838CCFB57777A3AD4FD1BB08445430050CE55A4D`,
`B4614BE35137FA3FE2E1633B1CE83B7FC8579B560586D122BED9791A49127749`, and
`7A09035931FFB481C14A16BDA4FD94E9025350D6EB13F2CF413201FEA2445292`.
Fixture hashes remained unchanged: Factory
`AD247CDAD555A29A73149E109B57AD3270FB1F705E3C805C9805B55493B9F978`, Work
`CA3828653379C6BA4DA77FA2C887C03EBE65A846ADFB4FDAA2E7C2083D84DCC4`, and
accepting mock workers
`69A0CE360F450CF9E71D4C9935345D1B2CF6A734A4E79DAE534E836F6218D3C3`.

`GATE-FUNC-001: PASS.` The complete unchanged inventory exited `0` under
`FUNCTIONAL_DEFAULT_JOBS=1` with isolated `HOME`/`USERPROFILE` and
`YOU_NO_BROWSER_OPEN=1`; packaged factories, server/session/event,
recordings/process, and workers/mock all passed. This serialized run cleared
the environment-owned staging contention seen in earlier parallel attempts;
no shared lease, unrelated source, or functional test was changed.

`GATE-SPINE-001`, `GATE-RACE-001`, and `GATE-SERVER-001: PASS` for the
changed selectors and direct consumers. The normal and race-focused CLI and
process-lifecycle tests passed, as did direct initializer/application/Wire,
runtime-hosting, recordings/replay, and unchanged `workers/mock` tests. The
broader race command retains the known unrelated idle-stdin ACP cancellation
failure as an unproven edge; it is outside the changed selector and did not
change during this work.

`Quality gates: PASS with a platform qualification.` The final captured
`make test` exited `0` (447 Go packages; one platform-conditional skip; 169
workflow tests with 168 pass and one skip). Backend/package/boundary/
structure/cycle/catalog/generation checks passed. After installing the pinned
ignored UI dependencies with `bun install --frozen-lockfile`, `make ui-lint`
and `make ui-deadcode` exited `0`. Native Windows `make lint` passed every
target except the platform-sensitive deadcode comparison (`3072` Windows
build-tag findings versus the Linux baseline's `3074`); `make deadcode` under
WSL Go 1.25 passed against the committed baseline. The baseline was not
changed, and no Wire authorship changed, so `make generate-wire` was not
applicable.

The first final-probe attempt intentionally failed closed because a manual
fixture copy omitted hidden/recursive files and returned
`CURRENT_FACTORY_NOT_FOUND`; it was discarded as a procedure error. The
authoritative result above copied the complete fixture tree and passed all
behavioral checks.

`GATE-LOOP-001: READY FOR HANDOFF.` The current-main rebase, rebuilt binary,
exact three-run probe, serialized functional inventory, focused/direct/race
tests, full unit gate, and platform-qualified lint evidence are complete.
The remaining action is to push this head, open the named PR, start required
CI, and address any blocking review feedback without claiming terminal CI or
merge.
