# C14 Worker CLI run lifecycle evidence ledger

## Scope and status

- Story: `fto-c14-pkg-workers-run-lifecycle-001` — freeze the lifecycle
  behavior and cost denominator.
- Parent behavior: `BEH-001` — the Worker CLI lifecycle package keeps its
  public success, failure, cancellation, recovery, correlation, and cleanup
  witnesses while intrinsic wait is reduced.
- Captured: `2026-08-29` before any C14 lifecycle implementation change.
- Local source head: `fea2e30a499384182d2fabe7038767e3c2f9c5e5`.
- Branch: `fto-c14-pkg-workers-run-lifecycle`; `origin/main` pointed to the
  same commit at characterization time.
- Environment: Windows 11 build `10.0.26200.0`, `windows/amd64`, Go
  `go1.25.0`, PowerShell `7.6.5`, 13th Gen Intel Core i7-13700K, 24 logical
  CPUs.
- Dependency fidelity: local-real `root.BuildProcess`, filesystem, in-process
  application lifecycle, loopback HTTP, and controlled command-runner edges;
  zero paid or remote calls.
- Worktree state before characterization: clean.
- Status: **PASS — GATE-CHAR-001 is complete.** Stories 002 and 003 own
  optimized-topology, repeat, race, final-performance, clean-room, and PR
  evidence.

`prd.json` has `sourcePlan: null`, and `progress.txt` was absent at iteration
start. The PRD, repository instructions and standards, current source, and
the commands below are the available characterization authority. This ledger
is additive: it does not change lifecycle Go code, production code, shared
functional support, public contracts, generated output, configuration, or CI.

## Reproducible selector denominator

The command was:

```text
go test -list '^Test' ./tests/functional/workers/transports/cli/run/lifecycle/...
```

It exited `0` and listed exactly these seven top-level selectors:

```text
TestLifecycleListenerClosedRejectsLiveNonresponsiveListener
TestCLIRunCleanInvocationCompletesWithoutDashboardStartup
TestCLIRunToFilePreservesExactPromptAndRejectsBeforeProviderDispatch
TestCLIRunWorkerReasoningEffortOverrideReachesCodexCommand
TestCLIRunUnsupportedWorkerReasoningEffortRejectsBeforeProviderDispatch
TestCLIRunServerAttachedInvocationTargetsExistingFactorySession
TestCLIRunCleanInvocationFailurePreservesPublicError
```

The full baseline procedure was three separate invocations of the exact
package command below. A PowerShell `Stopwatch` surrounded each native command;
the test process was not reused between samples. Each sample retained its
native exit status and package-reported duration.

```text
go test ./tests/functional/workers/transports/cli/run/lifecycle/... -count=1
```

| Sample | Exit | Go package time | Stopwatch wall time |
| ---: | ---: | ---: | ---: |
| 1 | 0 | 51.857s | 54.2612272s |
| 2 | 0 | 38.106s | 40.6067739s |
| 3 | 0 | 34.087s | 36.4045186s |
| **Median** | — | **38.106s** | **40.6067739s** |

All three samples passed. The C14 comparable wall-time denominator is the
PowerShell median, `40.607s`; the Go package median is retained separately so
future runs do not confuse test-process startup with package-reported time.
The samples were collected under the untouched seven-selector `t.Parallel`
topology, so this is a package-run denominator, not the sum of selector
durations.

## Pre-change semantic assertion inventory

Every FTM row is present in the current source and was exercised by the
passing baseline. The source locations below identify the existing witness;
they are the reference that stories 002 and 003 must retain or strengthen.

| FTM | Current selector / subtest | Pre-change public witness |
| --- | --- | --- |
| FTM-01 | `TestCLIRunCleanInvocationCompletesWithoutDashboardStartup` (`lifecycle_test.go:50`) | `Process.Execute` succeeds; stdout is exactly the clean primary result; forbidden operator/dashboard chatter is absent; stderr is empty. |
| FTM-02 | `TestCLIRunToFilePreservesExactPromptAndRejectsBeforeProviderDispatch` (`lifecycle_test.go:98`) | A 30-line UTF-8 CRLF file reaches exactly one Codex command with byte-identical stdin and canonical `model_reasoning_effort="xhigh"`. |
| FTM-03 | `TestCLIRunWorkerReasoningEffortOverrideReachesCodexCommand` (`lifecycle_test.go:248`) | Mixed-case, padded `XHIGH` is normalized and reaches exactly one Codex command as `xhigh`. |
| FTM-04 | To-file subtest `file_and_positional` (`lifecycle_test.go:154-200`) | File plus positional input returns `INVOCATION_INPUT_SOURCE_CONFLICT`, names both sources, and makes zero provider calls. |
| FTM-05 | To-file subtest `file_and_supplied_stdin` (`lifecycle_test.go:154-200`) | File plus non-TTY stdin returns the stable source conflict naming `file_text` and `stdin_text`, with zero provider calls. |
| FTM-06 | `assertUnreadableFilePrompt` missing-file branch (`lifecycle_test.go:204-224`) | The missing path is named in the error and the provider call count remains zero. |
| FTM-07 | `assertUnreadableFilePrompt` directory branch (`lifecycle_test.go:226-242`) | The error requires a regular file and the provider call count remains zero. |
| FTM-08 | `TestCLIRunUnsupportedWorkerReasoningEffortRejectsBeforeProviderDispatch` (`lifecycle_test.go:289`) | Unsupported `turbo` is rejected with an actionable validation error before any provider call. |
| FTM-09 | `assertDetachedServerPrefRunCannotAttachToContinuousHost` (`lifecycle_test.go:353,540-571`) | A detached `--server` run with isolated provider edges cannot attach to the continuous host and fails without contaminating it. |
| FTM-10 | `TestCLIRunServerAttachedInvocationTargetsExistingFactorySession` (`lifecycle_test.go:325`) | Hosted readiness, default Factory Session, terminal Work, completed Worker Session, exact stdout/stderr, and one correlated ordered Factory Event dispatch are observed before listener release. |
| FTM-11 | `TestLifecycleListenerClosedRejectsLiveNonresponsiveListener` (`lifecycle_listener_test.go:86`) | A live TCP listener that accepts but does not respond is not falsely called closed, and the probe reaches it within the existing two-second bound. |
| FTM-12 | `TestCLIRunCleanInvocationFailurePreservesPublicError` (`lifecycle_test.go:464-508`) | Controlled provider exit `7` yields a non-nil execution error, empty stdout, one decodable actionable `INTERNAL_SERVER_ERROR`, and no false success. |
| FTM-13 | Adverse subtest `partial provider output` (`lifecycle_adverse_test.go:155-190`) | Incomplete Codex JSONL produces failed/rejected Work and a failed Worker Session; partial text never becomes success stdout; one dispatch and cleanup are preserved. |
| FTM-14 | Adverse subtest `server attached provider failure` (`lifecycle_adverse_test.go:192-225`) | Hosted provider exit `7` preserves failed Work/Worker Session, one failed correlated dispatch with a diagnostic, shaped CLI failure, and listener/process cleanup. |
| FTM-15 | Adverse subtest `cancellation and recovery` (`lifecycle_adverse_test.go:227-299`) | Active Work and Worker Session are observed, one cancellation reaches the provider, no success stdout appears, listener release is ordered after observation, and a fresh invocation succeeds with distinct identities. |
| FTM-16 | Adverse subtest `observation timeout releases command` (`lifecycle_adverse_test.go:301-350`) | The 250ms observation budget reports terminal phase and last observation, releases gates, cancels and joins the command, leaves stdout empty, and closes the listener/process. |
| FTM-17 | Adverse subtest `forced assertion cleanup` (`lifecycle_forced_cleanup_test.go:35-76`) | The child exits nonzero after intentional assertion abort; the parent report proves one joined canceled provider command, released gates, closed listener/process, observed session, and absent owned paths. |

The frozen pre-change reference is the complete table above. The post-change
ledger must explicitly report **`assertion inventory same or stronger`**; that
post-change property is intentionally unproven by this characterization and
belongs to GATE-SPINE-001.

## JSON profile and measured buckets

The declared full-package profile was run against the untouched source:

```text
go test -json -v ./tests/functional/workers/transports/cli/run/lifecycle/... -count=1
```

It exited `0`. The first profile reported package time `31.690s` and a
PowerShell wall time of `34.0056429s`. A filtered repeat of the same JSON
profile command was used only to retain all selector/subtest pass events and
was not substituted for a baseline sample. Its package event and selector
events were:

| Selector / subtest | JSON elapsed |
| --- | ---: |
| `TestLifecycleListenerClosedRejectsLiveNonresponsiveListener` | 2.00s |
| `TestCLIRunCleanInvocationCompletesWithoutDashboardStartup` | 10.70s |
| `TestCLIRunToFilePreservesExactPromptAndRejectsBeforeProviderDispatch` | 24.41s |
| `TestCLIRunToFilePreservesExactPromptAndRejectsBeforeProviderDispatch/file_and_positional` | 8.31s |
| `TestCLIRunToFilePreservesExactPromptAndRejectsBeforeProviderDispatch/file_and_supplied_stdin` | 3.58s |
| `TestCLIRunWorkerReasoningEffortOverrideReachesCodexCommand` | 10.71s |
| `TestCLIRunUnsupportedWorkerReasoningEffortRejectsBeforeProviderDispatch` | 10.59s |
| `TestCLIRunServerAttachedInvocationTargetsExistingFactorySession` | 21.96s |
| `TestCLIRunCleanInvocationFailurePreservesPublicError` | 43.67s |
| `.../adverse_lifecycle_matrix` | 35.51s |
| `.../partial_provider_output` | 10.35s |
| `.../server_attached_provider_failure` | 3.14s |
| `.../cancellation_and_recovery` | 19.15s |
| `.../observation_timeout_releases_command` | 2.10s |
| `.../forced_assertion_cleanup` | 0.77s |
| Full JSON-profile package event | 45.72s |

Top-level tests are parallel and their elapsed values overlap; they must not
be added to estimate package wall time. The largest observed buckets are the
adverse matrix, especially cancellation/recovery; the file-input selector,
which serially creates five application processes; and the hosted selector,
which creates a continuous host, a detached client, and an invocation-owned
host. Concurrent Windows setup contention was visible in the coordinator logs:
focused adverse readiness ranged from about `376ms` to `7.737s`, while the
full profile emitted a readiness observation as high as about `7.869s`. These
are observed timings, not a claim that every run has the same phase cost.

The focused attribution procedure was:

```text
go test -json -v -run '^TestCLIRunCleanInvocationFailurePreservesPublicError$' ./tests/functional/workers/transports/cli/run/lifecycle/... -count=1
```

It exited `0` with top-level selector time `35.81s`, adverse matrix time
`34.10s`, and these subtest times: partial output `0.87s`, hosted failure
`0.83s`, cancellation/recovery `26.04s`, observation timeout `5.04s`, and
forced cleanup `1.33s`. This confirms the adverse bucket without using a
synthetic provider or changing the tested source. No trace file was needed
after the JSON selector profile supplied attribution.

## Pre-change application and process topology

The following is an exact call-site census for one passing full-package run.
`support.BuildProcess` delegates directly to `root.BuildProcess`, and every
listed call is reached once by the passing selectors. `Process.Execute` is an
in-process application invocation, not an OS process. The controlled provider
command runners return in-process results and do not spawn provider children.

| Selector group | `root.BuildProcess` calls | `Process.Execute` invocations | API listeners | OS test-binary children | Sequential setup / retained-state note |
| --- | ---: | ---: | ---: | ---: | --- |
| `TestLifecycleListenerClosedRejectsLiveNonresponsiveListener` | 0 | 0 | 0 | 0 | One raw TCP listener is created and tested; it is not an application process. |
| Clean invocation | 1 | 1 | 0 | 0 | One isolated Factory, HOME/USERPROFILE, stream set, and command runner. |
| To-file selector and its four validation calls | 5 | 5 | 0 | 0 | Main file success, two conflict subtests, missing path, and directory are sequential and each rebuilds the root. |
| Reasoning-effort override | 1 | 1 | 0 | 0 | One isolated one-shot root and provider edge. |
| Unsupported effort | 1 | 1 | 0 | 0 | One isolated one-shot root; validation stops before provider dispatch. |
| Hosted server-attached selector | 3 | 3 | 2 | 0 | Continuous host, detached client, and hosted invocation; the continuous host and invocation listener remain distinct. |
| Clean failure plus adverse matrix | 7 | 7 | 6 | 1 | Main one-shot failure; partial, hosted failure, cancellation, recovery, timeout, and forced-cleanup child. The forced child is one `exec.Command(os.Args[0], ...)`; its provider remains in-process. |
| **Full package total** | **18** | **18** | **8** | **1** | One package test process plus one intentional forced-cleanup test-binary child. |

The coordinator’s runtime path is recorded by the existing diagnostics as:

```text
root.BuildProcess -> Process.Execute -> controlled ProviderCommandRunner/API server -> Process.Close
```

The counts above are a source-derived runtime census rather than new counters
in shared support or lifecycle code. Adding instrumentation would violate this
story’s no-Go-change scope. Story 002 must add only package-local counters or
diagnostics if final evidence needs direct runtime census output.

## Wait, poll, timeout, and cleanup inventory

No new sleep or timeout padding was introduced. The existing waits are bounded
because public HTTP projections, listener binding, asynchronous command
completion, and forced cleanup do not expose one deterministic signal for
every phase. Gates and provider start/finish channels are signal-driven.

### Lifecycle-owned waits

| Source | Mechanism and bound | Purpose / risk |
| --- | --- | --- |
| `lifecycle_coordinator_test.go:18-23` | Readiness `15s`; hosted observation `15s`; command completion `15s`; HTTP client `2s`; poll interval `10ms`; process close `5s`. | Bounds listener readiness, public session/Work observation, command join, stalled HTTP, and cleanup. |
| `lifecycle_coordinator_test.go:297-321` | `ProcessAPIServer.WaitForBaseURL(15s)`. | Waits for the invocation-owned API listener’s ready channel. |
| `lifecycle_coordinator_test.go:276-290` | `WaitCommand` timer `15s`. | Joins asynchronous `Process.Execute` after an explicit terminal gate. |
| `lifecycle_coordinator_test.go:125-133` | Cancel-and-join fallback `15s` in the package-local cancelable command. | Prevents a cancellation path from hanging indefinitely. |
| `lifecycle_coordinator_test.go:413-433` | `Process.Close` context `5s`. | Bounds process-owned resource shutdown; gates are released first. |
| `lifecycle_coordinator_test.go:476-541` | Observation timer `15s` plus `10ms` ticker; each HTTP request uses the `2s` client timeout. | Repeatedly reads public Factory Session and terminal Work until both are observable or emits last-observation diagnostics. |
| `lifecycle_adverse_test.go:25-32` | Signal wait `5s`; cleanup duration ceiling `5s`; public observation helper `10s`; timeout phase budget `250ms`. | Bounds provider start/finish, listener close, public adverse projections, cleanup, and the intentional missing-terminal-output case. |
| `lifecycle_adverse_test.go:757-765,844-855` | `5s` signal timers and `15s` command timers. | Joins provider/listener signals and canceled commands. |
| `lifecycle_listener_test.go:46-83,111-117` | Raw TCP probe context `2s`, retry timer `10ms`, accepted-listener timer `2s`. | Distinguishes a live nonresponsive listener from a refused/closed listener. |

### Shared support bounds reached by this package

These are not owned by C14 and are recorded so a reuse change does not hide
their cost or alter their contract:

| Shared source | Mechanism and bound | Reached by |
| --- | --- | --- |
| `tests/functional/internal/support/process.go:22,566-588` | Process-command cleanup waits up to `5s` after cancellation. | Every coordinator `StartCommand` cleanup and the hosted helper. |
| `tests/functional/internal/support/api_server.go:34,145-159,167-176` | API server readiness/status waits use `15s`; reusable process close uses `5s`. | Continuous server startup in the hosted selector. |
| `tests/functional/internal/support/process_api_server.go:19,117-166` | Ready/start channels have the caller’s `15s` bound; a shutdown gate has a `15s` safety ceiling. | Invocation-owned API listeners. |
| `tests/functional/internal/support/observation.go:8-60` | Immediate public observation followed by a `10ms` ticker until the caller’s `10s` deadline. | Adverse Work, Worker Session, and Factory Session state reads. |
| `tests/functional/internal/support/http_observation.go:412-` | Status polling uses a `10ms` ticker and the supplied `15s` deadline. | Continuous host service-mode readiness. |
| `tests/functional/internal/support/api_server.go:556-622` | Retained Factory Event stream read has a `15s` timer and drains the advertised retained count. | Hosted correlation and adverse dispatch assertions. |

All polling loops above have existing in-code rationale tied to eventually
consistent public projections or a real transport teardown boundary. They are
not fixed sleeps. Story 002 must preserve their diagnostic and cleanup
semantics when reducing setup or root-build work.

## Duplicate setup candidates and retained-state risks

### Actionable first reuse group

The first profile-selected candidate is the sequential, non-hosted one-shot
group:

- `TestCLIRunCleanInvocationCompletesWithoutDashboardStartup`;
- `TestCLIRunWorkerReasoningEffortOverrideReachesCodexCommand`; and
- `TestCLIRunUnsupportedWorkerReasoningEffortRejectsBeforeProviderDispatch`.

They each build one root, execute one one-shot command, use controlled provider
edges, and do not require an API listener or a durable hosted session. A
package-local routed command edge could reuse one production-composed process
while assigning each invocation a fresh Factory directory, HOME/
USERPROFILE, working directory, stdin/stdout/stderr, route key, and call-count
view. The unsupported-input row must remain before any provider dispatch.

The to-file selector is a second candidate with higher measured cost: it
currently rebuilds five roots for one main success and four validation calls.
It needs a routed provider/file edge and must prove that each invalid input
still stops before dispatch. It should not be merged into the first cohort until
route exclusivity and per-invocation file/stream isolation are explicit.

### Candidates that must remain isolated until proven otherwise

- Hosted `--with-server` and detached `--server` paths have different listener
  ownership and intentionally prove FTM-09 isolation.
- Cancellation and recovery use a blocking provider, a released shutdown gate,
  and distinct Work/Worker Session identities. Sharing these objects could
  turn cancellation into false success or reuse stale projections.
- Observation timeout relies on a live blocked command and a `250ms` phase
  budget; its command must be canceled and joined after gate release.
- Forced cleanup intentionally uses a child test process and must retain its
  assertion abort, report file, cleanup ordering, and absent-path checks.
- Failure/partial-output cases retain provider result/error state and public
  failed dispatch projections; a shared runner must not leak results or call
  counts across scenarios.

Across all cohorts, the main retained-state risks are the process-scoped
default Factory Session, route registration, mutable provider-runner results
and counters, command cancellation/done channels, API listener binding and
shutdown gates, Factory Event/Worker Session projections, and per-invocation
temporary paths. The current package’s only deliberately shared mutable HTTP
client is documented as concurrency-safe.

## GATE-CHAR-001 report

| Property | Evidence | Result | Not proven |
| --- | --- | --- | --- |
| Exact selector inventory | `go test -list '^Test' ...`, exit `0`, seven selectors listed above | PASS | Post-change inventory |
| Complete untouched denominator | Three exact `go test ... -count=1` runs, exits `0`, all durations and median retained | PASS | Final comparable median |
| Semantic assertion coverage | Explicit FTM-01 through FTM-17 table mapped to current source witnesses; full/profile/focused runs passed | PASS | Post-change `assertion inventory same or stronger` |
| Runtime application/process/child topology | Source-derived call-site census: 18 roots, 18 in-process Execute invocations, 8 API listeners, 1 intentional test-binary child, 0 provider OS children | PASS as labeled source-derived census | Direct counters on optimized topology |
| Waits and largest cost buckets | Target/shared wait inventory, JSON selector profile, and focused adverse profile | PASS | Exact per-root construction CPU attribution |
| Actionable reuse group | Three non-hosted one-shot selectors share compatible shape; hosted/adverse/forced risks are recorded | PASS | Safe reuse implementation and isolation proof |
| Environment handling | No baseline/profile attempt failed; timing variance and concurrent readiness range are retained | PASS | Hosted CI timing and raised parallelism |

GATE-CHAR-001 establishes the true local denominator, current semantic
inventory, topology, bounded waits, and a first profile-backed reuse group.
It does not prove safe fixture reuse, race freedom, final performance,
clean-room reproducibility, PR-host direction, terminal CI, or merge. Those
edges remain assigned to GATE-SPINE-001, GATE-REPEAT-001, GATE-RACE-001,
GATE-PERF-001, GATE-CLEANROOM-001, and GATE-PR-PACKAGE-001.

## Scope audit and handoff

At this story boundary the implementation diff is documentation-only and is
limited to this ledger path. No lifecycle Go or production/shared-support file
was changed. The next story may edit only
`tests/functional/workers/transports/cli/run/lifecycle/**` and append its
implementation evidence here. `prd.json` and `progress.txt` remain local
workflow scaffolding and must not enter the PR diff.

## GATE-SPINE-001 — story 002 implementation evidence

Story `fto-c14-pkg-workers-run-lifecycle-002` reused only the compatible finite
one-shot cohort. A package-local `TestMain` owns one `support.BuildProcess`
result and closes it after the package completes. Each shared invocation gets a
fresh factory directory, HOME/USERPROFILE, working directory, and captured
streams. A package-local command router keys the injected provider runner by
that working directory, rejects duplicate active routes, unbinds every route
after `Process.Execute`, and reports its registration/call topology at close.
The hosted, server-attached, cancellation, timeout, partial/failure, and forced
cleanup paths continue to use their dedicated coordinator/process fixtures.

The compatible-cohort verification was:

```text
go test ./tests/functional/workers/transports/cli/run/lifecycle/... -count=1 -parallel 1 -run '^(TestCLIRunCleanInvocationCompletesWithoutDashboardStartup|TestCLIRunToFilePreservesExactPromptAndRejectsBeforeProviderDispatch|TestCLIRunWorkerReasoningEffortOverrideReachesCodexCommand|TestCLIRunUnsupportedWorkerReasoningEffortRejectsBeforeProviderDispatch)$' -v
```

It exited `0` in `9.168s`. The package-owned topology diagnostic was:

```text
C14 lifecycle shared topology: root_builds=1 process_executions=8 process_closes=1 active_invocations=0 provider_route_registers=8 provider_route_unbinds=8 provider_routes=0 provider_calls=3
```

The complete story gate was then run without selector filtering:

```text
go test ./tests/functional/workers/transports/cli/run/lifecycle/... -count=1 -v
```

The verbose gate exited `0` in `30.074s`, and the final exact package rerun
after the helper cleanup exited `0` in `29.947s`. All seven top-level selectors
and all adverse subtests passed, including FTM-01 through FTM-17. The same
topology diagnostic
reported one shared root build, eight serialized shared executions, eight
route registrations/unregistrations, zero retained routes, one shared-process
close, and three provider calls. The source-derived full-package root-build
count is therefore reduced from 18 to 11: the eight compatible invocations now
share one root, while the hosted and adverse groups retain their 3 and 7
dedicated roots respectively. The full package still exercises 18
`Process.Execute` invocations, eight API listeners, and one intentional
forced-cleanup test-binary child.

Post-change assertion inventory: **same or stronger**. The seven public
selectors remain present; their existing output, error, provider-call,
identity, ordering, cancellation, recovery, timeout, and cleanup assertions
were retained. The shared fixture adds direct route exclusivity and cleanup
topology checks, and the full package gate passed. No production, shared
functional-support, API, generated, or public-contract file changed.

Story 002 is complete at the semantic spine. Story 003 still owns repeated
baseline/final measurement, race verification, clean-room reproduction, the
performance decision, and PR/CI handoff; this evidence does not claim the
parent's final performance floor or race result.

## Story 003 — bounded optimization and final local evidence

Story `fto-c14-pkg-workers-run-lifecycle-003` used one bounded second,
profile-guided pass after the first optimized result missed the explicit 40%
target. The pass kept the finite one-shot cohort on one package-local process,
moved the sequential partial-output and hosted-provider-failure cases onto a
second routed process, and retained dedicated ownership for the continuous
host, detached-server isolation, cancellation/recovery, observation timeout,
and forced-cleanup child. Every shared invocation has fresh HOME/USERPROFILE,
working directory, streams, provider route, and (for the hosted cohort) an
invocation-owned API server; a mutex keeps each cohort's `Process.Execute`
non-overlapping.

An exploratory attempt to reuse the cancellation/recovery process was rejected
by its own public witness: recovery observed stale Factory Session/Work state
and failed to reach readiness. That route was removed; cancellation and its
recovery remain dedicated. This is the smallest safe disposition for the
remaining stateful lifecycle edge and is part of the measured-floor rationale.

The exact final implementation commit measured below is
`4576ba78bc` (`test: reuse safe worker lifecycle cohorts`). The environment was
Windows `windows/amd64`, Go `1.25.0`, PowerShell `7.6.5`, with local-real
`root.BuildProcess`, filesystem, in-process application lifecycle, loopback
HTTP, and controlled command-runner edges. No paid or remote calls were used.
The required rebase onto `origin/main` preserved this patch at final head
`beb18375f6`; the final-head clean-room result is recorded below.

### Final comparable timing

The three samples used the exact command `go test
./tests/functional/workers/transports/cli/run/lifecycle/... -count=1` with an
outer PowerShell stopwatch, sequentially on the same committed head:

| Sample | Wall time | Exit |
| --- | ---: | ---: |
| 1 | 36.755s | 0 |
| 2 | 32.397s | 0 |
| 3 | 47.333s | 0 |
| **Median** | **36.755s** | — |

The untouched Story 001 samples were `54.261s`, `40.607s`, and `36.405s`,
median `40.607s`. The committed final median is therefore `3.852s` / `9.5%`
lower. It does not meet the 40% target; the accepted performance disposition
is a profile-backed measured floor after the one bounded pass, not a claim of
the target. The large spread is retained as expected Windows setup/scheduler
contention rather than hidden by reruns.

The committed-head JSON selector profile was run with:

```text
go test -json -v ./tests/functional/workers/transports/cli/run/lifecycle/... -count=1
```

It exited `0`, with package event `38.534s`. The relevant selector/subtest
events were: listener `2.00s`; clean `4.04s`; to-file `16.94s` (conflicts
`1.73s` and `1.89s`); reasoning override `2.11s`; unsupported effort `8.54s`;
hosted `8.93s`; clean failure plus adverse matrix `36.46s`; adverse matrix
`30.59s` (partial `3.58s`, provider failure `2.32s`, cancellation/recovery
`19.80s`, observation timeout `3.52s`, forced cleanup `1.37s`). The adverse
matrix and cancellation/recovery remain the largest serialized bucket, while
the final source-derived topology is 9 roots versus 18 untouched and 11 after
Story 002:

| Cohort | Root builds | `Process.Execute` calls | API listeners | Child binaries |
| --- | ---: | ---: | ---: | ---: |
| Finite shared cohort | 1 | 9 | 0 | 0 |
| Hosted-adverse shared partial/failure cohort | 1 | 2 | 2 | 0 |
| Dedicated hosted/cancellation/timeout/forced paths | 7 | 7 | 6 | 1 |
| **Full package** | **9** | **18** | **8** | **1** |

The direct package diagnostics were:

```text
C14 lifecycle finite one-shot shared topology: root_builds=1 process_executions=9 process_closes=1 active_invocations=0 provider_route_registers=9 provider_route_unbinds=9 provider_routes=0 provider_calls=4 api_starts=0
C14 lifecycle hosted adverse shared topology: root_builds=1 process_executions=2 process_closes=1 active_invocations=0 provider_route_registers=2 provider_route_unbinds=2 provider_routes=0 provider_calls=2 api_starts=2
```

These counters, the profile, and the preserved semantic cases provide the
measured-floor explanation: safe root-build removal is material, but the
remaining cancellation/recovery projection waits, hosted listener lifecycle,
forced child, and concurrent Windows startup contention are semantic or
dependency-fidelity costs not safely removable by this package-local route.

### Final quality gates

| Gate | Exact procedure | Result and property proved |
| --- | --- | --- |
| GATE-SPINE-001 | `go test ./tests/functional/workers/transports/cli/run/lifecycle/... -count=1 -v` on `4576ba78bc` | PASS, exit `0`; all seven selectors and FTM-01–FTM-17, output/error/state/identity/order/call-count/cancellation/recovery/timeout/cleanup witnesses pass. **assertion inventory same or stronger**. |
| GATE-REPEAT-001 | `go test ./tests/functional/workers/transports/cli/run/lifecycle/... -count=3 -run '^(TestCLIRunCleanInvocationCompletesWithoutDashboardStartup\|TestCLIRunUnsupportedWorkerReasoningEffortRejectsBeforeProviderDispatch\|TestCLIRunCleanInvocationFailurePreservesPublicError)$` | PASS, exit `0`, `100.918s`; representative success, invalid input, failure, adverse matrix, route cleanup, and recovery assertions repeat three times. |
| GATE-RACE-001 | `go test -race ./tests/functional/workers/transports/cli/run/lifecycle/... -count=1` | PASS, exit `0`, `52.034s`; no race report in shared routing, command, gate, listener, or cleanup ownership. |
| GATE-PERF-001 | Three stopwatch samples above plus committed JSON profile and one bounded second pass | PASS by the permitted measured-floor disposition; final median is recorded, target miss is explicit, and no assertion was removed or weakened. |
| GATE-CLEANROOM-001 | Fresh detached worktree at final rebased head `beb18375f6`; `go test ./tests/functional/workers/transports/cli/run/lifecycle/... -count=1`; worktree removed afterward | PASS, exit `0`, package `27.553s`; no hidden workspace state was needed. |

### GATE-CLEANROOM-001 validation report

## Environment and artifact

- Commit/build identifier: detached final rebased head `beb18375f6`.
- Environment and configuration: Windows `windows/amd64`, Go `1.25.0`,
  clean detached Git worktree, no local PRD/progress state.
- Customer entry point: public `you run` command through `root.BuildProcess` and
  `Process.Execute`.
- Real and substituted dependencies: production root/filesystem/process
  lifecycle and loopback HTTP; only external provider/API effects controlled
  through `edges.Edges`.
- Cost/call budget used: one bounded clean-room package run; zero paid/remote
  calls.

## Project criteria

| Criterion | PASS/FAIL/BLOCKED | Evidence | Unproven edge |
| --- | --- | --- | --- |
| FTM-01 through FTM-17 and seven selectors | PASS | Detached full package exit `0`; final package gate and direct topology counters above. | Terminal PR CI and merge. |
| Repeated isolation and cleanup | PASS | Three-repeat representative gate exit `0`; shared routes registered/unregistered equally and ended at zero. | Other packages/platforms. |
| Race safety | PASS | Final full-package `-race` exit `0`, no report. | Undetected defects outside this run/platform. |
| Performance/floor evidence | PASS | Three final samples, selector profile, and bounded-pass topology/floor rationale above. | Portable absolute timing and target-level 40% reduction. |
| Scoped implementation and assertion preservation | PASS | Final owned diff is lifecycle package plus this C14 ledger; explicit **assertion inventory same or stronger**. | Review-stage interpretation. |
| No prohibited production/contract/config/CI/UI change | PASS | Detached commit contains only lifecycle functional tests and prior C14 ledger; no production or generated surface changed. | None identified. |
| Reproducible clean-room behavior | PASS | Fresh detached checkout at `beb18375f6`, exact package command exit `0`, package `27.553s`, temporary worktree removed. | Terminal CI and merge. |

## Customer journey

1. A developer runs `go test
   ./tests/functional/workers/transports/cli/run/lifecycle/... -count=1`.
   The detached clean checkout reports `ok` for the lifecycle package in
   `27.553s`; the public CLI/process assertions and all adverse cleanup
   witnesses remain active.

## Cross-task integration and usability

- Documentation discoverability: this ledger is under the canonical functional
  optimization evidence directory and records commands, topology, and gates.
- Permission and error behavior: invalid input and provider failure continue to
  assert stable errors, zero-dispatch validation, shaped failure, and no false
  success.
- Persistence/reload behavior: no persisted schema or public contract changed;
  hosted Factory Session, Work, Worker Session, and Factory Event observations
  remain asserted.
- Accessibility/keyboard/responsive behavior: not applicable to this backend
  functional package.
- Operational signals: direct route/process counters, lifecycle phase
  diagnostics, provider call counts, listener close signals, and forced-cleanup
  report remain available.

## Findings

| ID | Severity | Reproduction | Expected | Actual | Evidence |
| --- | --- | --- | --- | --- | --- |
| C14-FLOOR-001 | Informational | Compare the exact three-run medians above after the bounded pass. | Safe reuse should reduce intrinsic setup while preserving all witnesses. | Median improved `9.5%`; the 40% target was not reached, but profile-selected roots were removed and all behavior/race/repeat/clean-room gates passed. | Final profile, counters, and gate table. |
| C14-ISOLATION-001 | Informational | Reuse cancellation/recovery and run its recovery invocation on the same process. | Recovery must observe a fresh session and distinct Work/Worker Session identities. | Stale lifecycle state was observed; the broader route was removed and recovery remains dedicated. | Exploratory failure recorded above; final full package and recovery witnesses pass. |

## Verdict

PASS

The clean-room validator made no repairs. The final implementation preserves the
executable spine, passes the complete semantic/repeat/race/clean-room gates,
and is ready for the PR/CI handoff. The remaining unproven edges are terminal
PR CI, review-owned package direction, conflict resolution, and merge.

## Delivery handoff inputs

The implementation-stage PR comment must contain the exact before median
`40.607s`, after median `36.755s`, the three after samples, the measured-floor
disposition, and the required PR CI run URL once CI starts. CI evidence belongs
in that PR comment and not in a commit.
