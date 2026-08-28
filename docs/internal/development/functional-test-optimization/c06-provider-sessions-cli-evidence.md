# C06 Provider Sessions CLI evidence

Status: TASK-001 and TASK-002 review-correction witnesses are exercised on the
shared fixture, including deterministic provider-route close. TASK-003 final
clean-room validation is complete; the final implementation handoff is recorded
below and review owns the PR-host timing verdict, terminal CI, conflicts, and
merge.

## Baseline identity

- Source commit: `67710223e327d02c0de93a6ad826c754fe5c1702`
- Branch: `functional-test-optimization-c06-provider-sessions-cli`
- Environment: `go version go1.25.0 windows/amd64`; Windows 11 build `26200`; `amd64`; Intel Core i7-13700K; 24 logical CPUs.
- Cost and network: controlled local fixtures only; 0 remote calls; `$0` paid cost.
- Worktree status before baseline: clean.

## GATE-BASE-001

Commands run on the unchanged source commit:

```text
go test ./tests/functional/provider_sessions/cli -list '^Test'
go test -count=1 -timeout=10m ./tests/functional/provider_sessions/cli
```

The test-list command emitted exactly these eight top-level tests:

1. `TestWorkerSessionsStreamAbortReturnsTypedDiagnosticThroughRootProcess`
2. `TestBuiltWorkerSessionsStreamAbortExitsNonZero`
3. `TestBuiltWorkerSessionsStreamCancellationExits130`
4. `TestWorkerSessionsCLI`
5. `TestWorkerSessionsReplayOnlyRedirectsWellFormedNDJSON`
6. `TestWSRFT001OpeningRecordPrecedesProviderOutput`
7. `TestWSRFT002LiveAndReplayCorrelationRemainStable`
8. `TestWorkerSessionsFleetListCLIConcurrent`

Observed retained diagnostic result: `PASS`, exit code `0`. The package test
reported `18.410s`; the measured PowerShell command wall time was
`00:00:21.1555642`. This is one diagnostic sample, not a portable performance
threshold.

## Baseline topology

| Expensive operation | Source call sites | Executed baseline count | Witnesses |
| --- | --- | ---: | --- |
| Root application construction | `worker_sessions_cli_test.go:59`; `worker_sessions_cli_test.go:393`; `worker_sessions_fleet_list_test.go:34`; `stream_abort_test.go:29` | 6 | Main CLI (1), replay fixture for redirect/WSRFT001/WSRFT002 (3), fleet (1), root abort (1) |
| Production API-host start | `worker_sessions_cli_test.go:58`; `worker_sessions_cli_test.go:392`; `worker_sessions_fleet_list_test.go:33` | 5 | Main CLI (1), replay fixture (3), fleet (1) |
| CLI build | `worker_sessions_cli_test.go:945` through `buildWorkerSessionsCLIBinary` | 3 | Built abort, built cancellation, replay redirection |

The abort stream uses an isolated `httptest.Server`; it is not a production
API-host fixture and has no Factory Session state. The two built-process abort
rows intentionally retain real executable, exit-status, redirection, and
signal witnesses.

## Public witness map

| Test | Retained observable witness |
| --- | --- |
| Root stream abort | `Process.Execute` returns typed `WORKER_SESSION_STREAM_CLOSED`; one RECORD remains on stdout; no terminal or replay summary is synthesized. |
| Built stream abort | Built `you` exits non-zero; one RECORD and one safe diagnostic remain separated across stdout/stderr. |
| Built stream cancellation | On supported non-Windows hosts, the built child retains the RECORD and exits 130 after `os.Interrupt`; Windows keeps the explicit skip. |
| Main CLI | Help/completion/unknown-input no-mutation, explicit empty-list isolation, duplicate-route fail-closed setup, success/provider-failure/recovery list/show/read/stream, repeated read/replay idempotence, Work attribution, attempt/timing, 8/12/20 usage, transcript answer, failure diagnosis, missing Work/session no-side-effect errors, recording, and cleanup. |
| Replay redirection | Built CLI writes valid NDJSON to redirected stdout, strictly increasing positions, complete replay summary, and verbose diagnostics only to stderr. |
| WSRFT001 | Opening lifecycle record precedes provider binding/output, positions increase, and exactly one terminal record is last. |
| WSRFT002 | Live and replay Worker Session identity, Work attribution, opening timestamp, and attempt correlation agree. |
| Fleet | Three gated observations are visible in RUNNING and COMPLETED states; fleet CLI/HTTP attribution and deterministic order agree; `--limit 1` returns one. |

## TASK-001 identity denominator

The production-hosted cases to characterize before process sharing are the main
CLI success/failure cases, the three replay/ordering cases, and the fleet case.
Each must target a unique non-default Factory Session and unique explicit
Provider Session, Work, request, route, and test-owned path identities. The
abort-only HTTP fixtures have no Factory Session and will receive distinct
case-local provider/worker stream identities where their public payloads name
them. Missing/invalid, empty, duplicate, repeated-read, failure, abrupt-close,
and cleanup witnesses remain in scope; unsupported authorization, timeout, and
maximum claims remain out of scope.

At this identity-only checkpoint, TASK-002 remained unproven by this artifact:
one shared root/API process, repeat/race stability, final cleanup ledgers, and
PR-host package timing.

## TASK-001 post-identity verification

Command run after the identity-only changes:

```text
go test -count=1 -timeout=10m ./tests/functional/provider_sessions/cli
```

Observed retained diagnostic result: `PASS`, exit code `0`. The package test
reported `17.642s`; the measured PowerShell command wall time was
`00:00:19.7453131`. This is a diagnostic sample, not a TASK-002 performance
claim. On Windows, the Unix interrupt case retains its existing explicit skip.

The identity topology now proves:

- The main success and provider-failure cases open separate non-default Factory
  Sessions. Their Work IDs, request names, provider-session IDs, route markers,
  rollout paths, and list/show/read/stream selectors are distinct; the fleet
  parity assertion maps each Work back to its owning Factory Session.
- Redirect/replay and both WSRF ordering cases each open a separate
  non-default Factory Session, use distinct Work/request/provider identities,
  route provider output by the immutable Work marker, and verify the scoped
  list contains exactly one matching Factory Session, Provider Session, and
  Work.
- The fleet case uses three distinct explicit Factory Sessions for its three
  concurrent Works and three distinct immutable Work/provider route pairs. The gated
  runner records every route and rejects missing, duplicate, or unexpected
  routes without relying on arrival order.
- Root/built abort fixtures use distinct case-local Worker Session and Provider
  Session identities and preserve their typed diagnostic, non-zero, redirection,
  and exit-130 witnesses where the host supports interruption.
- Deferred cleanup closes every explicit Factory Session and verifies that its
  detail endpoint is gone and no same-folder live-list entry remains. The
  current service may retain an empty tombstone in the list, so raw ID absence
  is not asserted as a product fact.

No production, shared-support, contract, generated, or out-of-package files
were changed. The platform command edge does not expose private execution
metadata, so the package-local workstation renders the already public Work
name into the controlled provider prompt for immutable routing. Current stream
event frames omit the optional `factorySessionId` field; selector-scoped list,
show, read, and replay assertions prove the requested session boundary while
that frame-level field remains an explicitly unproven implementation edge.

At this identity-only checkpoint, the remaining TASK-002/TASK-003 edges were
one shared root/API process, reduced build count, repeat/race stability,
clean final-artifact reconciliation, PR-host timing, terminal CI, conflict
resolution, and merge.

## TASK-002 shared-process verification (pre-review-correction record)

The following TASK-002 measurements are retained historical evidence from
before the review correction. At that checkpoint the current source had direct
matrix witnesses, but provider-route unregistration and close-time route
assertions were not yet implemented. The route-lifecycle correction is recorded
in the current section below.

The delivered TASK-002 source commit is the working-tree head after the
identity characterization commit. The package now owns one lazy shared
production-composed `root.BuildProcess` fixture, one API-host starter, one
immutable Work-marker provider route ledger, one package-cached built `you`
artifact, and fresh child processes for executable/redirection/exit/signal
rows. Each hosted case still creates a separate case Factory directory and
explicit Factory Session; its cleanup closes the session, verifies the public
absence boundary, and removes the case path before the next case.

Verbose topology ledger from the full package run:

```text
C06 TASK-002 topology: root-builds=1 api-host-starts=1 cli-builds=1
```

Required TASK-002 commands:

```text
go test -count=1 -timeout=10m ./tests/functional/provider_sessions/cli
```

Observed: `PASS`, exit code `0`; package time `8.159s`; measured PowerShell
wall time `00:00:10.1117087`. The verbose confirmation run also passed all
seven selected runtime tests on this Windows host and emitted the topology
ledger above. The package's `-list '^Test'` denominator remains the eight-test
baseline; the Windows-only interrupt test is explicitly skipped.

```text
go test -count=3 -timeout=30m ./tests/functional/provider_sessions/cli
```

Observed: `PASS`, exit code `0`; package time `17.148s`. This gate initially
exposed a real repeat defect when the fleet synchronization channel was closed
after the first repetition; the bounded fix made only that synchronization
gate resettable per repetition, after which the complete three-run gate passed.

```text
go test -race -count=1 -timeout=10m -run '^TestWorkerSessionsFleetListCLIConcurrent$' ./tests/functional/provider_sessions/cli
```

Observed: `PASS`, exit code `0`; package time `3.648s`; no race report. The
fleet gate retained concurrent RUNNING observations before release and
terminal CLI/HTTP parity after release.

TASK-002 historically proved the complete package once, repeated identity
isolation, the focused fleet race path, one root/API topology, one cached CLI
build, fresh real child processes for the three OS-boundary rows, route
attribution without arrival-order assignment, explicit session cleanup,
case-directory cleanup, shared-listener shutdown, and preserved
CLI/transcript/stream/error/order witnesses. That historical run did not prove
route unregistration, a clean final checkout after the review correction,
PR-host package timing, remote provider behavior, Unix interrupt behavior on
this Windows host, terminal CI, conflict resolution, or merge.

## TASK-003 clean-room validation (pre-review-correction record)

This historical validation ran before the review correction and is not final
evidence for the current head. It remains useful as the prior clean-room
record; TASK-003 must rerun the declared gates after TASK-002 closes the route
lifecycle gap.

The clean-room validation checkout was a detached worktree at source commit
`152ce11fccce39257bb9efef3add74902fe1b19e`. It was clean before and after the
runtime probes. Environment: `go version go1.25.0 windows/amd64`; Windows 11
build `26200`; `amd64`; local sanitized fixtures only; 0 remote calls and
`$0` paid cost.

Required clean-room commands and observed results:

```text
go test -count=1 -timeout=10m ./tests/functional/provider_sessions/cli
PASS; package time 8.365s; exit code 0

go test -count=3 -timeout=30m ./tests/functional/provider_sessions/cli
PASS; package time 19.053s; exit code 0

go test -race -count=1 -timeout=10m -run '^TestWorkerSessionsFleetListCLIConcurrent$' ./tests/functional/provider_sessions/cli
PASS; package time 4.337s; exit code 0; no race report
```

The verbose clean-room package run passed all supported tests and emitted:

```text
C06 TASK-002 topology: root-builds=1 api-host-starts=1 cli-builds=1
```

The eight-test denominator remained intact. The built cancellation test was
explicitly skipped because `os.Interrupt` is unsupported for child processes
on Windows; the Unix-only interrupt witness remains assigned to the Unix
PR/platform gate. The runtime assertions proved the CLI/transcript/stream/
error/order witnesses, explicit identity isolation, route attribution,
repeatability, fleet race safety, and deterministic child/session/path/listener
cleanup. `git diff --check` passed and the owned-path audit against the
characterization base found only the six package/evidence paths permitted by
GATE-SCOPE.

Local diagnostic timing improved from the baseline package result of `18.410s`
to `8.365s` in that historical clean checkout. This is directional local evidence only;
the Backend Functional Coverage package row and its PR-host directional verdict
remain review-owned. The canonical loopback report is emitted at
`validation/functional-test-optimization-c06-provider-sessions-cli.md`; that
directory is excluded from the PR diff by the repository validation-artifact
convention.

## Review-correction verification

Source head: `52cae4b6f` (`test: add provider sessions CLI matrix witnesses`).
This bounded correction remains package-owned test code; no production,
shared-support, contract, generated, or out-of-package files changed.

The main CLI test now executes direct witnesses for FT-B04 duplicate-route
registration before Work/session admission, FT-H01 and FT-U01 no-mutation help
and unknown input, FT-B01 explicit empty-list isolation, FT-R02 failure-to-next-
success recovery, and FT-B05 repeated show/read/replay identity and history.
Missing Work/show/read cases additionally snapshot public Factory Session/Work/
Worker identities and provider-edge call count, require empty stdout, and
assert no side effect. The fleet remains three distinct explicit Factory
Sessions with ascending/stable CLI and HTTP order, and replay asserts complete
Worker Session history.

For FT-B01, the public session-scoped Worker Session HTTP resource requires a
Work ID. The supported no-Work CLI list invocation therefore uses explicit
`--session` against the top-level collection and asserts a non-nil empty
`sessions` array; no unsupported endpoint contract is invented.

Verification on the prior review-correction source head:

```text
go test -run '^TestWorkerSessionsCLI$' -count=1 -timeout=10m ./tests/functional/provider_sessions/cli
PASS; all direct review-correction subtests passed

go test -count=1 -timeout=10m ./tests/functional/provider_sessions/cli
PASS; package time 10.929s; exit code 0

git diff --check
PASS
```

The route-lifecycle gap identified in review was still open at this historical
checkpoint. The current TASK-002 correction below closes that gap; the final
clean-room artifact, PR package timing, and review-owned CI/merge remain
TASK-003 or review edges.

## TASK-002 route-lifecycle correction

Source head: `3c03b90368` (`test: close provider session CLI routes deterministically`).
The source-only correction was tested immediately before this commit with no
source changes between the test run and commit.

The controlled provider edge now separates immutable route definitions from
active case registrations. Registration fails closed for unknown or duplicate
keys; each registration has an idempotent close handle; and unregister refuses
to remove a route while its command call is active. Hosted cases register only
their own Work-marker routes, close them after provider activity completes, and
retain cleanup close handling for assertion-failure paths. Case cleanup checks
the active route keys, and TestMain fails the package if active calls or route
registrations survive shared-process shutdown.

The direct route-lifecycle witnesses are:

- Main success, failure, and recovery routes close after their respective
  hosted observations; duplicate registration remains fail-closed.
- Replay and WSRF cases close their route after provider execution and before
  final replay inspection.
- Fleet routes remain available through gated RUNNING/COMPLETED and CLI/HTTP
  parity assertions, then all three close before case cleanup returns.
- The root-process abrupt-stream-close witness verifies that no provider route
  remains active at that boundary; built abort/cancellation cases use isolated
  HTTP servers and do not register provider routes.

Required TASK-002 commands on the current source:

```text
go test -v -count=1 -timeout=10m ./tests/functional/provider_sessions/cli
PASS; package time 11.443s; exit code 0
C06 TASK-002 topology: root-builds=1 api-host-starts=1 cli-builds=1
C06 TASK-002 cleanup: active-provider-routes=0

go test -count=3 -timeout=30m ./tests/functional/provider_sessions/cli
PASS; package time 24.298s; exit code 0

go test -race -count=1 -timeout=10m -run '^TestWorkerSessionsFleetListCLIConcurrent$' ./tests/functional/provider_sessions/cli
PASS; package time 12.721s; exit code 0; no race report
```

Focused route-boundary confirmation also passed:
`TestWorkerSessionsCLI` in `4.267s` and
`TestWorkerSessionsStreamAbortReturnsTypedDiagnosticThroughRootProcess` in
`1.136s`. `git diff --check` passed before the source commit. These runs prove
the package-level behavior, one-root/API/cached-build topology, repeat/race
cleanup, and route lifecycle on the local Windows host. They do not prove a
fresh final checkout, PR-host package timing, remote provider behavior, the
Unix interrupt edge unavailable on Windows, terminal CI, conflict resolution,
or merge.

## TASK-003 final clean-room validation

The final validation used a newly created detached clean worktree after syncing
the branch onto the current `origin/main`. The validated delivered source head
was `b0402f64ee3529ab3ab3dc96ee0b091cf1851b9a`; the evidence reconciliation
commit that follows changes this markdown only and does not alter the package
implementation or its runtime dependencies.

Environment and fidelity matched the declared gate:
`go version go1.25.0 windows/amd64`; Windows 11 build `26200`; `amd64`; production
`root.BuildProcess`/API/service composition; controlled local provider and HTTP
fixtures; real built CLI children for executable, redirection, exit-code, and
signal witnesses; 0 remote calls and `$0` paid cost.

Required final-head commands and results:

```text
go test -count=1 -timeout=10m ./tests/functional/provider_sessions/cli
PASS; package time 8.508s; exit code 0

go test -count=3 -timeout=30m ./tests/functional/provider_sessions/cli
PASS; package time 21.109s; exit code 0

go test -race -count=1 -timeout=10m -run '^TestWorkerSessionsFleetListCLIConcurrent$' ./tests/functional/provider_sessions/cli
PASS; package time 4.348s; exit code 0; no race report

go test -v -count=1 -timeout=10m ./tests/functional/provider_sessions/cli
PASS; package time 9.301s; exit code 0
C06 TASK-002 topology: root-builds=1 api-host-starts=1 cli-builds=1
C06 TASK-002 cleanup: active-provider-routes=0
```

The verbose run retained the Windows-only skip with the exact diagnostic
`os.Interrupt is not supported for child processes on Windows`; the Unix
interrupt edge remains assigned to the Unix PR/platform gate. All supported
tests passed, including the direct matrix witnesses, repeated identity/history
reads, fleet order/parity checks, route lifecycle, and deterministic cleanup.

The local diagnostic package result improved directionally from the baseline
`18.410s` to `8.508s`. This is a local sample, not a portable threshold; the
review-owned Backend Functional Coverage package row supplies the PR-host
directional verdict. `git diff --check` and the allowed-path audit against
`origin/main` `bb37276f74e9e904e637931c2c13a37a67b54ca8` passed, with only the
six permitted C06 package/evidence paths changed. No source, support,
generated, contract, baseline, UI, CI, or unrelated functional-test path was
modified.

This validation promotes TASK-003 for review handoff. The final PR head is
the evidence-updated branch head pushed to PR #2382; CI start, the PR-host
package timing, terminal required checks, conflict resolution, and merge are
review-owned and are not claimed by this artifact.

## Review correction: repeated-read stabilization and size split

Source head: `f043b66c53` (`test: stabilize provider session CLI coverage`).
The replay/history assertions now live in `worker_sessions_cli_replay_test.go`,
the main CLI setup matrix is extracted from the top-level test, and the
backend-size limits are satisfied without removing any direct witness. FT-B05
polls the public `confirmationState` until it reaches `CONFIRMED` before
comparing repeated `show`, `read`, and replay output. This accommodates the
one-time confirmation transition observed by the public read path while
retaining byte-for-byte output and full identity/history assertions after the
state settles.

The review failure was reproduced on Linux with coverage instrumentation:
the first `show` returned `UNCONFIRMED`, the second returned `CONFIRMED`, and
all other serialized fields were identical. After the bounded stabilization,
the same coverage-style parent test passed ten consecutive repetitions.

```text
make backend-size
PASS; all owned Go backend files are within file limit 1000 and function limit 100

go test -v -count=1 -timeout=10m ./tests/functional/provider_sessions/cli
PASS; package time 9.548s; root-builds=1 api-host-starts=1 cli-builds=1; active-provider-routes=0

go test -count=3 -timeout=30m ./tests/functional/provider_sessions/cli
PASS; package time 22.291s

go test -race -count=1 -timeout=10m -run '^TestWorkerSessionsFleetListCLIConcurrent$' ./tests/functional/provider_sessions/cli
PASS; package time 3.872s; no race report

go test -covermode=count -coverpkg=./pkg/... -coverprofile=/tmp/c06-parent-repeat-final.out -count=10 -run '^TestWorkerSessionsCLI$' ./tests/functional/provider_sessions/cli
PASS; Linux/amd64; package time 5.733s
```

The Linux command is a coverage-style repeated run of the review-failing
parent/subtest; the full Backend Functional Coverage lane and PR-host timing
remain CI/review evidence. Local Windows retains the documented skip for the
Unix-only child interrupt witness. The new source remains entirely within
`tests/functional/provider_sessions/cli/**`; the tracked evidence artifact is
the only other changed path.
