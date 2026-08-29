# C09 packaged-loop contention characterization

## Scope and status

This document records the pre-correction characterization for
`functional-test-optimization-c09-packaged-loop-contention-001`. The owned
implementation surface is the production-composed packaged-loop functional
package under `tests/functional/factory/packaged/loop/`; no product runtime,
public contract, shared functional support, workflow, or generated file is
changed by this story.

The PRD names `docs/temp/functional-test-optimization.md` as its source plan,
but that file is absent from this checkout and from the reachable Git history.
The PRD, current package, and repository standards are the available
authorities for this characterization. No historical worktree changes were
imported.

## Exact contention failure

The failed run was inspected before the package was structurally corrected.

| Field | Observed value |
| --- | --- |
| Run | [33218340928](https://github.com/portpowered/you-agent-factory/actions/runs/33218340928) |
| Job | [Backend Functional Coverage / 99006961978](https://github.com/portpowered/you-agent-factory/actions/runs/33218340928/job/99006961978) |
| Head | `d70d09c2b9dcb4f36799557be40935dcffc78131` |
| Runner and parallelism | `ubuntu-latest`, four logical CPUs, `jobs=16` |
| Functional step | `2026-08-28T22:52:54.8023298Z` through `2026-08-28T22:59:02.9784362Z` |
| Functional step wall time | `6m08.12s` in `resource-usage.txt` |
| Package timing | `github.com/portpowered/infinite-you/tests/functional/factory/packaged/loop` — `6.723s`, `outcome=fail` |
| Exact failing selector | `factory/packaged/loop/invocation_test.go::TestPackagedLoop` (source line 23 on the failed head) |
| Child failure | `shared_fixture_test.go:288: Factory Session "fe78814e-6aff-47fb-aab7-a55c16fa1f22" remains open` |
| Runtime diagnostic | `2026-08-28T22:54:33.454Z` |
| Diagnostic artifact | [functional-test-diagnostics/9704386108](https://github.com/portpowered/you-agent-factory/actions/runs/33218340928/artifacts/9704386108) |

The durable report identifies `TestPackagedLoop` as the only failed top-level
test in this package. The package report does not preserve a more specific
first missed phase: the aggregate reason is the open-session teardown result.
The same job also reported unrelated failures in
`tests/functional/providers/acp`, `tests/functional/sessions/restart`, and
`tests/functional/workers/transports/cli/run/lifecycle`; those packages are
outside this lane and are not used as evidence for the loop correction.

The resource artifact records `user=1184.39s`, `system=187.83s`, `372%` CPU,
`792988 kB` maximum resident set, and exit status `0` for the measurement
wrapper. These values establish the oversubscribed runner context; they are
not a latency threshold.

## Current executable spine

The package builds one process and one continuous loopback API host, then
executes the customer path through the production `root.BuildProcess` and
`Process.Execute` composition. External provider execution is controlled only
at `serviceedges.Edges.ProviderCommandRunner`; the loop scheduler clock,
submission recorder, and API server are also injected edges owned by this
package.

The current flow is:

1. `newLoopSharedFixture` creates a test root, isolated home/work directories,
   `support.NewProcessAPIServer`, a provider command router, a fake clock at
   `2026-07-29T20:00:00Z`, and a submission channel.
2. `support.BuildProcessWithContext` constructs the reusable production process
   with `APIServerStarter`, `Clock`, `ProviderCommandRunner`, and
   `SubmissionRecorder` edges.
3. `support.InstallPackagedFactoryWithProcess` installs `@you/loop`; a public
   `you run --continuously --with-server --no-record` command is started through
   `Process.Execute`.
4. Each scenario copies the installed Factory into a private `HOME`, registers
   its Factory/work paths with the command-runner router, opens a non-default
   Factory Session through `POST /factory-sessions`, and invokes the loop
   through `POST /factory-sessions/{id}/invocations`.
5. Public scheduled-execution Work is observed through the injected
   `FactorySubmissionRecord` callback. The valid loop also reads public Factory
   Events to establish dispatch completion.

### Existing public assertion inventory

`TestPackagedLoopUsesInvocationDurationAndSkipsOverlap` currently asserts:

- a 20 ms invocation response succeeds with terminal status `TIMED_OUT` for
  the intentionally long-lived controller;
- trigger-at-start emits one `scheduled-execution` Work in `init` with
  `SCHEDULED`, sequence `1`, and nominal/actual timestamps equal to the fake
  start;
- the controlled runner starts execution 1;
- after the scheduler is ready and fake time advances one minute, exactly one
  execution remains in flight and sequence `2` is `skipped` with
  `SKIPPED_OVERLAP` and exact minute-1 timestamps;
- releasing the runner and observing a completed public dispatch allows the
  scheduler to re-arm; advancing another minute emits sequence `3` in `init`
  with `SCHEDULED` and exact minute-2 timestamps.

`TestPackagedLoopRejectsInvalidDurationBeforeWorkAdmission` currently asserts
that `every=tomorrow` returns HTTP 400 and emits no submission to the Work
recorder.

The characterization added by this story retains those assertions and adds
the six defined duration boundary cases below. Accepted values additionally
observe trigger-at-start `scheduled-execution` Work in `init`/`SCHEDULED` with
sequence 1 and exact fake-start timestamps. Rejected values assert HTTP 400
and no Work admission.

## Wait topology before correction

The following are the package's current observation and lifecycle waits. They
are documented here as characterization; this story does not replace them.

| Location | Current observation | Bound or behavior |
| --- | --- | --- |
| `newLoopSharedFixture` | API URL and runtime status readiness | `api.WaitForURL`; `support.WaitForStatus` with the package's 15 s ceiling |
| `invokeLoop` | Public invocation response | Product request has `timeoutMillis=20`; no test sleep is involved |
| valid-loop child | Provider dispatch starts | `runner.started` with a 1 s `time.After` bound |
| valid-loop child | Scheduler has registered/re-armed its timer | `schedulerTimers` signal with a 1 s `time.After` bound |
| valid-loop children | Work submission | `waitForLoopSubmission` with a 2 s `time.After` bound |
| valid-loop child | Public dispatch response exists | 2 s wall-clock deadline plus `runtime.Gosched` observation |
| scenario cleanup | Session stopped before deletion | shared support's bounded `WaitForSessionStopped` observation |
| scenario cleanup | Session deletion and root absence | HTTP `GET` 404 plus `os.Stat` after `RemoveAll` |
| fixture cleanup | Listener is no longer serving | One HTTP `/status` probe with a 1 s client timeout after process close |

The scheduler-specific signal is necessary to distinguish gocron's timer from
unrelated fake-clock waiters. The current code still uses bounded diagnostic
opportunities around that signal; the contention-hermetic replacement belongs
to story `...-002`.

## Resource ownership and cleanup order

| Owner | Acquired resource | Current release/observation |
| --- | --- | --- |
| shared fixture | Test root, isolated home/work, packaged install | `t.TempDir` plus fixture `RemoveAll` after process close |
| shared fixture | One root-built process | `support.CleanupProcess` |
| shared fixture | Loopback API listener | process close, then `/status` rejection probe |
| each scenario | Copied Factory, scenario root/home/work | scenario `RemoveAll` and fixture-root cleanup |
| each scenario | Provider-router registrations | scenario unregister cleanup |
| each scenario | Explicit non-default Factory Session | terminate, wait for stopped, delete, then public `GET` 404 |
| loop runtime | Scheduler/timer and controller goroutines | session termination and process close |
| valid scenario | Controlled blocking runner | explicit release in the happy path or context cancellation during teardown |
| shared fixture | Submission channel and event reads | process/session lifecycle bounds the producers; observations are read synchronously |

Cleanup is registered in LIFO order. The fixture registers its post-process
probe before `support.CleanupProcess`, so process close runs before the fixture
assertion. A scenario registers router unregistration before `scenario.open`
registers session cleanup, so session cleanup runs before route removal.

The pre-correction cleanup path is not complete on every failure: `close`
calls `support.CloseFactorySessionAt`, whose fatal observation can stop the
remaining delete/root/ledger steps. The CI symptom proves an open session was
left visible to the fixture ledger, but the aggregate report does not prove
which internal stop observation first missed its bound. Idempotent continuation
and error aggregation are explicitly deferred to story `...-002`.

## Complete C09 case matrix

This is the review matrix from the PRD. Story `...-001` owns B01-B06; the
remaining rows are preserved for the named follow-on story rather than being
silently treated as characterized by this change.

| ID | Kind | Given / when / then | Owning story |
| --- | --- | --- | --- |
| C09-H01 | happy | Installed loop, explicit session, `every=1m`, trigger-at-start, controlled provider; invoke with a 20 ms deadline; HTTP succeeds with `TIMED_OUT` while scheduling remains active. | `...-002` |
| C09-H02 | happy | Valid loop is admitted at fake start; trigger-at-start fires; one `scheduled-execution` Work is `init`/`SCHEDULED`, sequence 1, exact start tags, and one dispatch starts. | `...-002` |
| C09-H03 | happy/recovery | Sequence 2 is skipped while execution 1 is active; execution 1 completes; scheduler re-arms and fake time reaches minute 2; sequence 3 is `init`/`SCHEDULED` with exact minute-2 tags. | `...-002` |
| C09-U01 | unhappy/bad input | Explicit session with `every=tomorrow`; post invocation; HTTP 400 and no Work. | `...-002` |
| C09-U02 | unhappy/dependency blocked | Provider execution 1 is held; minute 1 arrives; no second provider call and sequence 2 is terminal `SKIPPED_OVERLAP`. | `...-002` |
| C09-U03 | unhappy/dependency timeout | A required readiness/dispatch signal never arrives; the diagnostic context expires; exact phase/last observation is reported and cleanup still runs. | `...-002` |
| C09-U04 | unhappy/partial completion | Setup acquires only some resources, then fails; cleanup attempts every acquired resource once and retains all cleanup errors. | `...-002` |
| C09-C01 | concurrency/overlap | Execution 1 is active at the next nominal minute; capacity 1 is occupied; sequence 2 is `SKIPPED_OVERLAP`, provider calls remain 1, and no duplicate dispatch exists. | `...-002` |
| C09-C02 | concurrency/contention | CPU is oversubscribed and goroutines are delayed; touched valid selector repeats under contention; phase signals preserve sequence 1/2/3 without a one-second scheduling opportunity. | `...-002` |
| C09-X01 | cancellation | Caller deadline expires while controller remains active; HTTP returns; terminal response is `TIMED_OUT` and scheduling continues for public witnesses. | `...-002` |
| C09-X02 | cancellation/cleanup | Controlled provider execution is blocked when test/session cancellation begins; release or context cancellation unblocks it before teardown and no owned goroutine/process remains. | `...-002` |
| C09-T01 | timing | Fake start is `2026-07-29T20:00:00Z`, interval one minute; trigger-at-start and two advances occur after readiness; tags are exactly start, +1m, +2m with sequence 1/2/3. | `...-002` |
| C09-T02 | timing/contention | Scheduler registration/dispatch is delayed by contention; test coordinates phases; fake time advances only after scheduler-specific readiness and the bound remains diagnostic. | `...-002` |
| C09-B01 | boundary/minimum | `every=1s` through the public Factory Session boundary; request is admitted and trigger-at-start Work is observable. | `...-001` |
| C09-B02 | boundary/maximum | `every=168h` through the public Factory Session boundary; request is admitted and trigger-at-start Work is observable. | `...-001` |
| C09-B03 | boundary/empty | `every=""` through the public Factory Session boundary; HTTP 400 before Work admission. | `...-001` |
| C09-B04 | boundary/below minimum | `every=999ms`; HTTP 400 before Work admission. | `...-001` |
| C09-B05 | boundary/above maximum | `every=168h1s`; HTTP 400 before Work admission. | `...-001` |
| C09-B06 | boundary/malformed | `every=tomorrow`; HTTP 400 before Work admission. | `...-001` |
| C09-B07 | boundary/ordering | Three scheduler decisions occur across overlap; exactly ordered sequence 1/2/3 records and no duplicate/reordering. | `...-002` |
| C09-B08 | boundary/idempotency | Release/close/cancel is requested from success and deferred cleanup paths; each owned resource reaches cleanup once without panic/double close. | `...-002` |
| C09-CL01 | cleanup/normal | Valid scenario completes assertions; runner, routes, session, timers, listener, process, streams, and roots release; session GET is 404. | `...-002` |
| C09-CL02 | cleanup/validation error | Invalid duration returns 400; cleanup runs with no Work and all scenario resources released. | `...-002` |
| C09-CL03 | cleanup/assertion failure | Package-local cleanup-core probe injects an early failure; blocked runner, session, timers, listener, streams, registrations, and roots are cleaned and all failures diagnosed. | `...-002` |
| C09-N01 | not applicable/authorization | Local functional API has no authorization mode; no authorization case is invented. | `...-003` |
| C09-N02 | not applicable/persistence | Scenarios use `--no-record` and live scheduling; no persistence/restart claim is made while public live Factory Events remain covered. | `...-003` |

## Baseline evidence boundary

The CI artifact proves the exact high-contention failure identity, package
timing, environment, and open-session symptom. It does not prove the corrected
behavior, the first missed scheduler phase, portable latency thresholds, or
other functional packages. The local discovery, focused boundary cases, and
one package run recorded below are the only story-owned local evidence.

### Story-owned local evidence

| Procedure | Result | Property proved | Remaining edge |
| --- | --- | --- | --- |
| `go test -list . ./tests/functional/factory/packaged/loop` | Go test result `ok`, exit 0; discovered `TestPackagedLoop` | Current package test spine and named top-level entry point | High-contention determinism; story `...-002` |
| Boundary group within `TestPackagedLoop` | Included in the one package run; six nested cases passed | Public 1s/168h admission with trigger Work and empty/999ms/168h1s/tomorrow rejection with no Work | Corrected cleanup/contention; stories `...-002`/`...-003` |
| `go test -count=1 -timeout=10m ./tests/functional/factory/packaged/loop` | Go test result `ok …/packaged/loop 2.988s`; the Windows PTY wrapper remained idle after emitting the result and was interrupted, with no test failure output | Current local-real package behavior before structural correction | Corrected behavior and GitHub suite contention |
| `git diff --check` | Exit 0 | No whitespace errors in the characterized diff | Markdown rule coverage |
| `go run ./cmd/markdown-linter docs/internal/development/functional-test-optimization/c09-packaged-loop-contention.md` | No diagnostics emitted; the wrapper remained idle under concurrent local Go workloads and was interrupted after a bounded wait (exit 1) | No claim made because the linter did not complete | Review-stage/documentation lint rerun |

No CI result or downloaded diagnostic artifact is committed. CI contention,
final package promotion, diff audit, and clean-room loopback remain owned by
the later gates `CI-CONTENTION-01`, `PACKAGE-FINAL-C09-01`, `DIFF-C09-01`, and
`LOOPBACK-C09-01`.

## Story 002 correction and evidence

The package-owned correction keeps the production-composed executable spine
unchanged and replaces readiness guesses with phase observation. Each
`loopPhaseObserver` wait creates a named, phase-local context and records the
phase and last observation before failing with the `LOOP-CONTENTION-01`
diagnostic. The 15 s ceiling remains only for production process startup and
loopback binding; it is not reused for scenario phases or cleanup. The
provider and scheduler readiness phases use 3 s budgets, public Work and
dispatch observations use 2 s budgets, and the invocation client has a 2 s
transport ceiling around the product's 20 ms response deadline. These bounds
are based on the former 1 s readiness opportunities and 2 s public projection
observation window, with margin for the production-composed Windows process;
they are diagnostic ceilings rather than synchronization.

| Former wait | Corrected observation |
| --- | --- |
| Provider start with `time.After(1s)` | Controlled runner `started` signal under `loopProviderPhaseBudget=3s`; the last observation includes provider call count. |
| Scheduler timer channel with `time.After(1s)` | Monotonic scheduler-specific registration count plus a non-blocking notification channel under `loopSchedulerPhaseBudget=3s`; each advance waits for a registration strictly newer than its captured cursor. |
| Work submission with `time.After(2s)` | Submission channel under `loopWorkPhaseBudget=2s`; every discarded record updates the last public Work observation. |
| Dispatch wall deadline plus `runtime.Gosched` | Public retained Factory Event/dispatch projection through the existing bounded `support.WaitForObservation` under `loopDispatchPhaseBudget=2s`, recording event/dispatch/completion counts. |
| Invocation request without a client bound | Request context derived from the test context and the scoped `loopInvocationRequestBudget=2s` transport ceiling. |

The public projection poll remains intentional: the dispatch completion
observation is asynchronously committed and has no package-visible signal that
covers the public Factory Event projection. `support.WaitForObservation`
observes immediately and at a bounded interval; it does not create a fake
runtime edge or turn a missing dispatch into success.

Cleanup now uses a package-local LIFO stack. The controlled runner's release
is guarded by `sync.Once`; session termination, stopped-status observation,
deletion, and the final public 404 check each run under their own bounded
context beneath one `loopSessionCleanupBudget=5s` total. Cleanup continues to
the next step after an error. Route unregistration and scenario-root removal
still run after session cleanup. The scenario cleanup stack executes every
registered action once, joins all errors, and returns the same joined error on
repeated calls. Fixture cleanup also asserts one process/listener close, no
remaining provider registrations, a completed `Process.Execute` command, and
the rejected post-close `/status` probe.

`TestLoopCleanupResourceMatrix` now exercises the real package composition in
five sequential subtests: invalid-duration validation after session
acquisition, a timed-out invocation with a blocked provider and scheduler
timer, an injected early assertion with live Factory Event and Response Event
streams, a stalled stream-header acquisition, and partial acquisition before
session opening. The matrix counts each cleanup action, observes provider-route
unregister calls, asserts public Factory Session absence, checks scheduler timer
stop counts against registrations, retains both injected cleanup errors, waits
for the blocked runner and stream readers to finish, and leaves fixture
listener/process/root checks to the shared fixture cleanup. The runner-release
unit test registers release and cancellation before launching `Run`, then
asserts the goroutine exits on both its normal and cleanup paths.

The stream opener now derives its long-lived SSE cancellation context from the
calling test context and uses a cloned default transport with
`ResponseHeaderTimeout=loopStreamOpenBudget` only for response-header
acquisition. It does not set `http.Client.Timeout`, so a successfully opened
body remains available until scenario cleanup. Non-success diagnostics cancel
the request before reading at most `loopStreamErrorBodyLimit` bytes and close
the response/client transport. The `stream-open-failure` matrix row uses a
local handler that withholds headers, asserts the opener returns a
`context.DeadlineExceeded`, and then verifies session, runner, routes, and
scenario-root cleanup still run.

### Story-owned corrected-run evidence

| Procedure | Result | Property proved | Remaining edge |
| --- | --- | --- | --- |
| `GOMAXPROCS=2 go test -count=3 -run '^TestPackagedLoop/TestPackagedLoopUsesInvocationDurationAndSkipsOverlap$' -timeout=10m ./tests/functional/factory/packaged/loop` | Go test emitted `ok …/packaged/loop 7.475s` for all three repetitions. The Windows command wrapper remained idle after emitting the result and was stopped after a bounded wait; no test failure output was present. | Repeated valid selector preserves `TIMED_OUT`, sequence 1 `SCHEDULED`, sequence 2 `SKIPPED_OVERLAP`, one provider call during overlap, public dispatch completion, scheduler re-arm, and sequence 3 `SCHEDULED` under representative local contention. | GitHub-hosted suite contention and portable latency thresholds. |
| `GOMAXPROCS=2 go test -count=1 -run '^TestPackagedLoop/(TestPackagedLoopRejectsInvalidDurationBeforeWorkAdmission|TestPackagedLoopDurationBoundaries/minimum_1s)$' -timeout=10m ./tests/functional/factory/packaged/loop` | Go test emitted `ok …/packaged/loop 1.335s`; no failure output. | Invalid duration remains HTTP 400 with no Work, and the minimum accepted duration remains public `init`/`SCHEDULED` Work with exact start tags. | The remaining boundary cases are covered by the complete package run. |
| `go test -count=1 -run '^Test(LoopCleanupStackRunsAllActionsOnceAndRetainsFailures|BlockingLoopRunnerReleaseIsIdempotent)$' ./tests/functional/factory/packaged/loop` | Focused cleanup tests emitted `ok …/packaged/loop` in approximately 0.06 s. | Partial cleanup failure does not suppress later LIFO actions, both injected errors remain discoverable, repeated cleanup does not rerun actions, and repeated runner release unblocks the blocked runner without a close panic. | OS-wide resources outside this package's ownership. |
| `GOMAXPROCS=2 go test -count=1 -run '^TestPackagedLoop$' -timeout=10m ./tests/functional/factory/packaged/loop` | Go test emitted `ok …/packaged/loop 2.671s`. The complete run's fixture assertions require one process/API host, eight selected scenario sessions, public session absence, removed roots, and zero provider-route registrations; no failure output was present. | Complete current package behavior and lifecycle accounting after the correction, directionally below the characterized 2.988 s package result. | Final-head promotion, PR CI, and clean-room validation remain story `...-003`. |
| `go test -count=1 -run '^(TestLoopCleanupResourceMatrix|TestBlockingLoopRunnerReleaseIsIdempotent|TestLoopCleanupStackRunsAllActionsOnceAndRetainsFailures)$' -timeout=10m ./tests/functional/factory/packaged/loop` | Go test emitted `ok …/packaged/loop 1.914s` with all four resource-matrix subtests and cleanup-core tests passing. | Production-composed validation, blocked/canceled execution, early assertion with live streams, partial acquisition, idempotent release, public session absence, exact route/timer cleanup counts, and joined cleanup failures are observed without resource remainder. | This local evidence does not prove GitHub-hosted suite contention. |
| `go test -race -count=1 -run '^TestLoopCleanupResourceMatrix$' -timeout=15m ./tests/functional/factory/packaged/loop` | Go test emitted `ok …/packaged/loop 7.709s`; no race diagnostic or failure output was present. | The new stream, blocked-runner, timer-counter, route-counter, and fixture cleanup synchronization is race-clean in the package-owned matrix. | Other package tests and hosted contention remain outside this focused race run. |
| `go test -count=1 -run '^TestLoopCleanupResourceMatrix$/stream-open-failure$' -timeout=10m ./tests/functional/factory/packaged/loop` | Go test emitted `ok …/packaged/loop 3.780s`; the stalled header request returned a deadline error and the scenario cleanup assertions passed. | Stream header acquisition is bounded independently of the long-lived SSE body, and an open failure still reaches production-composed session, runner, route, and root cleanup. | Non-success stream status diagnostics and hosted contention remain outside this focused selector. |
| `go test -count=1 -run '^TestLoopCleanupResourceMatrix$' -timeout=15m ./tests/functional/factory/packaged/loop` after the stream-open correction | Go test emitted `ok …/packaged/loop 3.871s` with all five cleanup rows passing and no failure output. | The complete package-owned resource matrix remains green after bounding stream acquisition. | The final pushed-head package and current-head CI remain promotion evidence. |
| `GOMAXPROCS=2 go test -count=3 -run '^TestPackagedLoop/TestPackagedLoopUsesInvocationDurationAndSkipsOverlap$' -timeout=15m ./tests/functional/factory/packaged/loop` | Go test emitted `ok …/packaged/loop 5.250s` for all three repetitions with no failure output. | The corrected phase budgets preserve valid-loop invocation, Work, overlap, dispatch, re-arm, timing, and terminal witnesses under representative local contention. | GitHub-hosted suite contention and portable latency thresholds. |
| `go test -count=1 -timeout=15m ./tests/functional/factory/packaged/loop` | Go test emitted `ok …/packaged/loop 5.518s` with no failure output. | The complete current package, including the resource cleanup matrix, passed once through the production-composed path. | Final pushed-head CI and clean-room validation remain story `...-003`. |
| `go test -count=1 -timeout=10m ./tests/functional/factory/packaged/loop` after the required `origin/main` rebase | Go test emitted `ok …/packaged/loop 103.738s` with no failure output. The host was visibly contended during this run; the wall time is recorded as contaminated local timing, not a threshold. | The final rebased code head passed the complete production-composed package, including the resource cleanup matrix. | Hosted contention and portable latency thresholds remain review-owned. |
| `git diff --check` | Exit 0 after formatting and correction. | No whitespace errors in the implementation diff. | Final three-dot ownership audit remains story `...-003`. |

The package-level timing is directional evidence only. The host was running
other Go workloads and the Windows PTY wrapper did not exit promptly after
printing successful Go results, so no quiet-host threshold or wrapper exit
latency claim is made. The corrected wait topology removes the fixed
one-second readiness opportunities while retaining bounded diagnostics. Full
Hosted contention remains review-owned; the final local package run is
recorded in the promotion evidence below.

## Story 003 promotion evidence

### PACKAGE-FINAL-C09-01

| Procedure | Result | Property proved | Remaining edge |
| --- | --- | --- | --- |
| `go test -count=1 -timeout=10m ./tests/functional/factory/packaged/loop` at source head `da43bff2b982e57104af03ff8624c6b3af431fd2` | Go test emitted `ok github.com/portpowered/infinite-you/tests/functional/factory/packaged/loop 4.277s` with no failure output. The Windows command wrapper produced no further output during a bounded completion check and was interrupted; the tool reported wrapper exit 1. The interruption is recorded separately from the successful Go test result and is not treated as a package assertion failure. | The complete local-real, root-built packaged-loop suite executed once and emitted its passing package result, covering the public loop, Work, Factory Event, dispatch, overlap, duration, timing, ordering, terminal, and lifecycle assertions. | The command-wrapper shutdown behavior and GitHub-hosted suite contention remain review evidence; no wrapper-latency threshold is claimed. |

Environment: `go1.25.0 windows/amd64`, local production composition, controlled
provider command-runner edge, and the package's fake clock/submission edges.
The test source was unchanged by the evidence update; the final delivery audit
must verify that the owned package paths remain byte-identical after base sync.

### DIFF-C09-01

The pre-push three-dot audit at the tested source head reported exactly these
owned paths and no generated, shared-support, workflow, or historical-worktree
files:

- `docs/internal/development/functional-test-optimization/c09-packaged-loop-contention.md`
- `tests/functional/factory/packaged/loop/cleanup_test.go`
- `tests/functional/factory/packaged/loop/invocation_test.go`
- `tests/functional/factory/packaged/loop/resource_cleanup_test.go`
- `tests/functional/factory/packaged/loop/shared_fixture_test.go`

`git diff --check origin/main...HEAD` emitted no diagnostics. The final audit
and current-head PR identity belong to the handoff record; CI-run evidence is
intentionally excluded from this document and must be placed in a PR comment.
