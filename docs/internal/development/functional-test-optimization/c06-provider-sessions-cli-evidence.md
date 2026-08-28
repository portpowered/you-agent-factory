# C06 Provider Sessions CLI evidence

Status: TASK-001 baseline and explicit-identity characterization complete; the
package remains on the per-test process topology pending TASK-002.

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
| Main CLI | Help, completion, unknown input, success and provider-failure list/show/read/stream, Work attribution, attempt/timing, 8/12/20 usage, transcript answer, failure diagnosis, missing Work/session errors, recording, and cleanup. |
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
- The fleet case uses one explicit Factory Session for its three concurrent
  Works and three distinct immutable Work/provider route pairs. The gated
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
