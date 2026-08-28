# C06 Provider Sessions CLI evidence

Status: TASK-001 review-correction witnesses are now exercised on the shared
fixture; TASK-002 route unregistration/close lifecycle and TASK-003 final
clean-room validation and PR handoff remain.

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

TASK-002 remains unproven by this artifact: one shared root/API process,
repeat/race stability, final cleanup ledgers, and PR-host package timing.

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

Remaining TASK-002/TASK-003 edges are one shared root/API process, reduced
build count, repeat/race stability, clean final-artifact reconciliation,
PR-host timing, terminal CI, conflict resolution, and merge.

## TASK-002 shared-process verification (pre-review-correction record)

The following TASK-002 measurements are retained historical evidence from
before the review correction. The current source adds direct matrix witnesses,
but provider-route unregistration and close-time route assertions are still
unimplemented and must be completed in TASK-002 before this section can be
treated as final.

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
CLI/transcript/stream/error/order witnesses. It did not prove route
unregistration, a clean final checkout after the review correction, PR-host
package timing, remote provider behavior, Unix interrupt behavior on this
Windows host, terminal CI, conflict resolution, or merge.

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

Verification on the current source head:

```text
go test -run '^TestWorkerSessionsCLI$' -count=1 -timeout=10m ./tests/functional/provider_sessions/cli
PASS; all direct review-correction subtests passed

go test -count=1 -timeout=10m ./tests/functional/provider_sessions/cli
PASS; package time 10.929s; exit code 0

git diff --check
PASS
```

The current evidence still does not prove FT-R01 provider-route
unregistration/close-time route cleanup, the final clean-room artifact, PR
package timing, or review-owned CI/merge. Those are the next bounded TASK-002
and TASK-003 edges; no route-lifecycle claim is made by this correction.
