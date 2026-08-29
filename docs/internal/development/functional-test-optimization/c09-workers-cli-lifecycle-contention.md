# C09 Worker CLI lifecycle contention characterization ledger

Status: CHAR-C09-01 complete for story
functional-test-optimization-c09-workers-cli-lifecycle-contention-001. The
later story status and verification sections are appended below.

This is a current-head discovery and characterization record. It freezes the
existing Worker CLI lifecycle contract and the cited jobs=16 failure before a
synchronization repair. It does not claim the coordinator change, adverse-path
cleanup proof, raised-parallelism CI proof, or clean-room loopback owned by the
later stories.

## Authority and scope

- Repository: `you-agent-factory`
- Branch: `functional-test-optimization-c09-workers-cli-lifecycle-contention`
- Exact current head and `origin/main` at discovery:
  `bdb65eac0115e3d81a3dd55aba5365600e190e20`
- PRD story: `functional-test-optimization-c09-workers-cli-lifecycle-contention-001`
- Owned implementation surface inspected:
  `tests/functional/workers/transports/cli/run/lifecycle/**`
- Owned evidence surface added by this story:
  `docs/internal/development/functional-test-optimization/`
- No production, shared-support, public-contract, workflow, or excluded test
  surface was changed for this story.
- The PRD's source-plan reference was not present in this checkout. The PRD,
  repository standards, current source, and cited CI artifact are therefore the
  authorities for this characterization; the missing source plan is recorded,
  not reconstructed.
- Paid/remote validation calls made by this story: 0. Existing public CI
  artifact access was read-only.

## CHAR-C09-01 cited jobs=16 failure

The cited [CI run 33218340928](https://github.com/portpowered/you-agent-factory/actions/runs/33218340928)
and [Backend Functional Coverage job 99006961978](https://github.com/portpowered/you-agent-factory/actions/runs/33218340928/job/99006961978)
are the source of the following exact observations.

| Field | Observed value |
| --- | --- |
| Workflow | `CI`, `workflow_dispatch`, `merge-full`, `short=false` |
| Raised configuration | `jobs=16` |
| Run head | `d70d09c2b9dcb4f36799557be40935dcffc78131` |
| Discovery denominator | 156 packages, 1,046 tests |
| Failing package | `tests/functional/workers/transports/cli/run/lifecycle` |
| Failing selector | `TestCLIRunCleanInvocationFailurePreservesPublicError` |
| Selector elapsed | 4.120s |
| Package elapsed | 14.334s |
| Failure location | `lifecycle_test.go:484` in the artifact's checked-out source |
| Exact failure | `decode ErrorResponse: invalid character 'i' looking for beginning of value` |
| Failure phase | after `Process.Execute`, while decoding the expected JSON stderr response |
| Concurrent lifecycle witness | `TestCLIRunServerAttachedInvocationTargetsExistingFactorySession` passed in 8.480s |

The artifact's failure log shows non-JSON initialization text before the JSON
`RUN_INVOCATION_FAILED` response. The prefix reports packaged-factory staging
contention (`.staging-owner`, `outcome=active-contention`) during system
bootstrap. The intended shaped provider failure was therefore not reached on
that failing attempt; this is an initialization/contention contamination of the
public error stream, not evidence that the provider-failure contract itself is
wrong.

The job's diagnostic artifact also records `make functional-test-viz` resource
usage of 6:08.12 wall time, 372% CPU, and 792,988 kB maximum resident set size.
Those are job-level observations, not a lifecycle-package performance
baseline. Other packages in the same job failed independently (including
packaged-loop, providers/ACP, and sessions/restart); they are outside this
lane and were not repaired here.

## Current executable denominator

Procedure run at the current head:

    go test -list '^Test' ./tests/functional/workers/transports/cli/run/lifecycle

Observed package result: `ok`, six top-level tests listed, command/package
enumeration completed in 0.129s. The exact six-selector denominator is:

| Selector | Current public behavior witness | Main resources and cleanup owner |
| --- | --- | --- |
| `TestCLIRunCleanInvocationCompletesWithoutDashboardStartup` | `Process.Execute` succeeds; stdout is exactly `deterministic workers lifecycle primary COMPLETE`; stderr is empty; operator chatter is absent. | Root-built process, temp factory, injected success command runner; `support.CleanupProcess`, `t.TempDir`, and test cleanup. |
| `TestCLIRunToFilePreservesExactPromptAndRejectsBeforeProviderDispatch` | A 30-line UTF-8/CRLF file reaches one Codex command with exact stdin and canonical `xhigh`; file/positional, file/stdin, missing-file, and directory cases reject before provider dispatch. | Root-built processes, temp factory/file fixture, file reader/stat edges, shaped runner; each process uses `support.CleanupProcess`, with `t.TempDir` and subtest cleanup. |
| `TestCLIRunWorkerReasoningEffortOverrideReachesCodexCommand` | The public override is normalized to `--config model_reasoning_effort="xhigh"` and reaches exactly one injected Codex call. | Root-built process, temp factory, shaped runner; `support.CleanupProcess` and test cleanup. |
| `TestCLIRunUnsupportedWorkerReasoningEffortRejectsBeforeProviderDispatch` | Unsupported `turbo` input returns an actionable validation error and makes zero provider calls. | Root-built process, temp factory, shaped runner; `support.CleanupProcess` and test cleanup. |
| `TestCLIRunServerAttachedInvocationTargetsExistingFactorySession` | The detached `--server` negative control cannot reuse the continuous host; the `--with-server` invocation observes the hosted Factory Session, terminal Work, one completed Worker Session, and one correlated public dispatch through retained Factory Events before the invocation-owned API listener is released, with exact stdout/stderr. | Continuous root/API host, detached negative-control process, invocation-owned root/API process, gated runner, two lifecycle gates, HTTP polling, goroutine/ticker; host stop, `support.CleanupProcess`, `t.Cleanup`, `sync.Once`, and ticker/body closes. |
| `TestCLIRunCleanInvocationFailurePreservesPublicError` | The injected exit-7 provider failure returns a non-nil `Process.Execute` error, empty stdout, exactly one JSON `ErrorResponse`, actionable code/message, and `INTERNAL_SERVER_ERROR` family. | Root-built process, temp factory, shaped failing runner; `support.CleanupProcess` and test cleanup. |

All six top-level tests call `t.Parallel()`. The file-backed test has two
named conflict subtests (`file and positional`, `file and supplied stdin`) and
two unreadable-source branches (missing path and directory path); those are
subcases, not additional top-level selectors. The six tests contain ten
current `support.CleanupProcess` registrations, covering the primary process,
conflict processes, the directory-path process, the detached negative-control
process, and the hosted process.

## Public lifecycle observations

The current tests use the public CLI/process boundary and public generated HTTP
responses rather than internal state mutation. The following is the frozen
behavioral spine.

1. The clean, file, override, unsupported-input, server-attached, and failure
   paths all construct a root process through `support.BuildProcess` and drive
   it with public `Process.Execute` inputs. Provider effects are injected at
   the `ProviderCommandRunner` edge. No live provider credential or network
   call is part of these characterizations.
2. The successful server-attached path reads
   `/factory-sessions/~default` while the invocation is still active and
   requires a non-empty Factory Session identity. It then reads the public
   session-scoped `/work` projection and requires terminal Work containing the
   exact server-attached primary result.
3. The server test's `sessionObservedGate` releases the shaped worker only
   after the hosted Factory Session is readable. Its `workObservedGate` holds
   invocation-owned API shutdown until terminal Work, Worker Session, and
   retained Factory Event correlation are read through public endpoints. The
   retained events require one dispatch request, one Worker Session association,
   and one dispatch response in that order, with the same dispatch and Worker
   Session identities. This proves the current public ordering witness; it is a
   characterization extension, not the C09 package-wide coordinator repair.
4. The failure and invalid-input paths assert output/error identity and
   provider-call count. In particular, malformed source combinations,
   unreadable file sources, unsupported effort, and the deterministic provider
   error do not silently dispatch a provider.

The live Petri server-attached path does not use the durable-only
`/factory-sessions/{session_id}/dispatches` list: the public route correctly
returns not-found for a live Petri session. Dispatch identity and completion
are therefore characterized through the public retained Factory Event stream,
while Worker Session identity/state is characterized through the public
`/worker-sessions?workId=` projection. No internal marking or service state is
used by the lifecycle test.

The remaining current lifecycle measurement gaps are:

| Public edge | Current evidence boundary | Status before story 002 |
| --- | --- | --- |
| Readiness latency | `WaitForBaseURL(5 * time.Second)` is the current bounded readiness observation; no phase timestamp is emitted. The remote server-attached selector completed in 8.480s overall. | Exact phase latency unproven; the bound and whole-selector timing are recorded for story 002. |
| Teardown latency | The hosted listener is held until Work is observed and process cleanup is registered, but the current selector emits no separate shutdown/cleanup timestamp. | Exact phase latency unproven; cleanup ownership is inventoried above. |

These timing gaps are part of the safe repair target. They are not converted
into passes by treating a whole-selector duration as a phase measurement.

## Adverse-path characterization

The current six-selector contract covers the applicable malformed-input,
duplicate-source, unsupported-capability, deterministic provider-failure,
empty-success-output, and server-attached negative-control observations:

- source conflicts identify both conflicting source labels and make zero
  provider calls;
- missing and non-regular file sources retain their path/shape diagnostics and
  make zero provider calls;
- unsupported reasoning effort retains an actionable validation error and makes
  zero provider calls;
- a shaped non-zero provider result retains empty primary stdout and one
  public `ErrorResponse` with non-empty code/message and
  `INTERNAL_SERVER_ERROR` family;
- the detached `--server` preference negative control cannot attach to the
  continuous host with isolated provider edges;
- successful paths retain exact primary stdout, empty stderr, provider-call
  identity, prompt bytes, and normalized command arguments.

This is characterization only. It does not prove cancellation, timeout,
partial bootstrap recovery, repeated same-process execution, forced cleanup,
zero-residue process/goroutine census, or jobs 12/16 repeat stability. Those
are explicit later-story gates.

## Local verification and environment boundary

The focused list command above passed at the current head. A clean isolated
HOME run of the clean selector also passed:

    go test -count=1 -timeout=15m -run '^TestCLIRunCleanInvocationCompletesWithoutDashboardStartup$' -v ./tests/functional/workers/transports/cli/run/lifecycle

Observed result: `PASS`; selector 45.18s, package 45.346s. The first isolated
bootstrap populated a temporary `.you-agent-factory` tree, so this is a
behavior witness, not a stable latency baseline.

After the public correlation assertions were added, one isolated-HOME server
selector also passed:

    go test -count=1 -timeout=15m -run '^TestCLIRunServerAttachedInvocationTargetsExistingFactorySession$' -v ./tests/functional/workers/transports/cli/run/lifecycle

Observed result: `PASS`; package 5.863s. The expected detached negative-control
failure emitted controlled provider-failure logs; the attached invocation
retained exact primary stdout and empty stderr and passed the public Worker
Session/Event correlation assertions. The remote artifact's 8.480s selector
timing remains the authoritative non-local witness for the cited jobs=16 run.

A concurrent package/focused attempt using the operator's normal HOME was
contaminated by an existing long-lived `you run --continuously --with-server`
process and shared `.staging-owner` entries. The affected attempts reported
`outcome=active-contention` during packaged-factory installation; those failures
are consistent with the cited remote failure phase and are not attributed to
the lifecycle assertions. A server-attached selector in a fresh isolated-HOME
attempt additionally reported that its process API starter was never invoked
and observed no session; this is recorded as local environment/test-harness
failure, not a passing witness. The authoritative existing CI artifact remains
the server-attached pass evidence.

No package-wide coordinator, timeout inflation, serialization, workflow, or
shared-support change was made in this story. The next safe implementation step
is the package-local coordinator in story 002, using the exact six-selector
denominator and the remaining phase-latency measurements above.

## Story 002 lifecycle synchronization

Status: `LIFECYCLE-C09-01` complete for story
`functional-test-optimization-c09-workers-cli-lifecycle-contention-002`.

The package now has one package-local `lifecycleCoordinator` in
`tests/functional/workers/transports/cli/run/lifecycle/lifecycle_coordinator_test.go`.
All six top-level selectors in `lifecycle_test.go`, including the file-backed
subcases and detached server negative control, construct the production-shaped
process through `support.BuildProcess`, prepare public inputs through the
coordinator, and close through its bounded lifecycle owner. The coordinator
also gives every direct invocation an isolated `HOME`/`USERPROFILE`, removing
the shared operator-home staging collision from local package execution without
changing production or shared functional support.

The server-attached witness now has this observable order:

    inputs -> Process.Execute -> API listener readiness -> public Factory Session
    -> provider release -> terminal Work -> public Worker Session/Event/dispatch
    correlation -> listener release -> command completion -> Process.Close

Readiness is driven by `ProcessAPIServer`'s ready signal. Active and terminal
state are read through public HTTP projections, with a 10ms retry cadence only
for eventual projection visibility. Provider, listener, and correlation gates
are idempotent and are released by coordinator cleanup on every assertion or
phase failure. The phase guards are local safety ceilings (15s for readiness,
observation, and command completion; 5s for close), not a package-wide timeout
or a success-by-sleep mechanism. HTTP reads also have a 2s client deadline so
one stalled projection cannot defeat the observation deadline.

The coordinator emits diagnostics containing the failed phase, elapsed time,
last public observation, topology, and the ordered transition ledger. A verbose
server-attached run recorded readiness after `1.8069629s`, total hosted-process
teardown after `2.0662366s`, and this transition sequence:

    inputs, execute, readiness wait, readiness ready, active observation,
    provider release, terminal Work, public correlation, listener release,
    command done, process close

The provider-failure selector inspects stdout/stderr only after
`Process.Execute` returns its terminal error and the coordinator records the
command-done phase. It continues to require empty success stdout and exactly
one parseable public `ErrorResponse`; no assertion or serialization was removed.

## Story 002 verification

The following commands were run against the changed package with controlled
provider effects and real root/process/session/API wiring:

    go test -run '^$' ./tests/functional/workers/transports/cli/run/lifecycle

Observed result: `ok` (compile-only, cached after the source compiled) and
`git diff --check` passed.

    go test -count=1 -timeout=15m -run '^(TestCLIRunCleanInvocationCompletesWithoutDashboardStartup|TestCLIRunServerAttachedInvocationTargetsExistingFactorySession|TestCLIRunCleanInvocationFailurePreservesPublicError)$' ./tests/functional/workers/transports/cli/run/lifecycle

Observed result: `ok`; all three representative success/server/failure
selectors passed in `7.438s`.

    go test -count=3 -timeout=15m -run '^(TestCLIRunCleanInvocationCompletesWithoutDashboardStartup|TestCLIRunServerAttachedInvocationTargetsExistingFactorySession|TestCLIRunCleanInvocationFailurePreservesPublicError)$' ./tests/functional/workers/transports/cli/run/lifecycle

Observed result: `ok`; all nine bounded repeat executions passed in `26.044s`.
The verbose server-attached run also passed in `7.902s` and emitted the phase
ledger above. The focused commands prove unchanged output/error/provider-call
behavior and deterministic representative ordering; they do not prove the
adverse cancellation/timeout matrix, race instrumentation, full-package
coexistence, raised-parallelism CI, or clean-room loopback owned by stories
003-004.

## Story 003 adverse lifecycle and cleanup extension

Status: `ADVERSE-C09-01` and `CLEANUP-C09-01` are partially evidenced for
`functional-test-optimization-c09-workers-cli-lifecycle-contention-003`.
The package-local adverse matrix is implemented, but the live-runtime
cancellation projection described below remains unproven and keeps the story
open. No production, shared-support, public-contract, workflow, or excluded
surface was changed.

The adverse cases extend the existing failure selector so the package retains
its six top-level selector denominator:

- A partial Codex JSONL stream reaches the real provider boundary, produces a
  failed/rejected Work and failed Worker Session, retains one correlated
  dispatch request without a fabricated `ACCEPTED` response, emits no success
  stdout, and closes after the public failure is inspected.
- A shaped exit-7 provider failure retains the public ErrorResponse, failed
  Work/Worker Session, one correlated failed dispatch response with an error,
  one provider call, and a closed listener/process.
- An intentionally absent terminal observation expires its local 250ms phase
  budget, reports the terminal phase and last observation, releases every
  tracked gate, cancels and joins the command, emits no stdout, and closes the
  listener/process.
- A forced child assertion after acquiring a factory, artifact, root process,
  listener, provider, command, gate, and Factory Session writes a parent report
  proving the expected non-zero child exit, process close, command join,
  provider start/finish/cancel, gate/listener close, session observation, and
  removal of the artifact and factory paths.
- A cancellation followed by recovery uses the supported live Factory Session
  `cancel` route. The provider is canceled exactly once, the public Factory
  Session reaches `FINISHED`, `/status` reports `COMPLETED`/`FINISHED` with no
  active Work categories, stdout remains empty, the command stays blocked until
  the listener gate is released, and a fresh recovery invocation succeeds with
  distinct Work and Worker Session identities.

The cancellation witness also found a bounded runtime projection edge that the
test does not conceal: after the accepted live-session cancel and provider join,
the public Work remains `PROCESSING`, its correlated Worker Session remains
`RUNNING` with `OPERATOR_CANCELED`, and the retained dispatch has a request but
no response. The command then returns a cancellation-compatible error and
`Process.Close` closes the listener. The direct top-level Worker Session
control route is not a valid substitute: it is not attached to this
runtime-owned attempt and did not cancel the injected provider in the same
experiment. Therefore LIFE-U07's terminal Work/Worker Session/dispatch proof
is explicitly unproven, although no successful terminal result is fabricated.
The smallest plan delta is a runtime-owned terminalization path (or a
supported turn-scoped live-session control carrying the active Worker Session
correlation); that belongs to the excluded production/runtime ownership and
was not silently broadened into this test-only lane.

## Story 003 verification

The following commands used local-real root/process/session/listener wiring and
deterministic provider edges:

    go test -count=1 -timeout=5m ./tests/functional/workers/transports/cli/run/lifecycle -run '^TestCLIRunCleanInvocationFailurePreservesPublicError$'

Observed result: `ok` in `28.401s`; the original public failure and all five
adverse subcases passed once.

    go test -race -count=1 -timeout=10m ./tests/functional/workers/transports/cli/run/lifecycle -run '^TestCLIRunCleanInvocationFailurePreservesPublicError$'

Observed result: `ok` in `65.796s`; the touched adverse coordination path had
no race report.

    go test -count=3 -timeout=15m ./tests/functional/workers/transports/cli/run/lifecycle -run '^TestCLIRunCleanInvocationFailurePreservesPublicError$'

Observed result: `ok` in `93.970s`; all three original-failure plus adverse
matrix executions passed. These commands prove the implemented partial,
dependency-failure, timeout, forced-cleanup, no-success cancellation, provider
call, recovery, and listener/process cleanup witnesses. They do not prove the
unresolved cancellation terminal projection, full six-selector package
coexistence, raised-parallelism CI, or clean-room loopback.
