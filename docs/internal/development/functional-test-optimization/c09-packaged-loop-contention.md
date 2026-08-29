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
