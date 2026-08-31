# Transport protocol-family characterization: run-scoped server

This ledger is the package-local evidence for
`functional-test-optimization-c15-transport-protocol-family-001`.
It freezes the current RS-01..17 witness before any fixture topology change.
It is a characterization record, not post-change parity or performance
evidence.

## Artifact and provenance

| Field | Value |
| --- | --- |
| Package | `github.com/portpowered/infinite-you/tests/functional/transport/run_scoped_server` |
| Package ID | `F037` |
| Current HEAD | `42eeee4472656b8290f798c36a5b8c871b24d7d0` |
| Base / merge base | `origin/main` / `42eeee4472656b8290f798c36a5b8c871b24d7d0` |
| Source file | `tests/functional/transport/run_scoped_server/run_scoped_server_test.go` |
| Current source SHA-256 | `77d26a4e4966c463ce2e2d271f7cdd2350fa7e5fba46222ef9a8695ba6e7e544` |
| c01 source SHA-256 | `77d26a4e4966c463ce2e2d271f7cdd2350fa7e5fba46222ef9a8695ba6e7e544` |
| c01 classification source | `docs/internal/development/functional-test-optimization/c01-eligibility-inventory.json`, recorded commit `eef5150f277490384b460a47a0a3bcca51338e67` |
| Go / host | `go1.25.0 windows/amd64` |
| Discovery | `go test -list '^Test' -count=0 -timeout=5m ./tests/functional/transport/run_scoped_server` exited 0; 14 top-level declarations |

The supplied admission median for this package is **29.183s**. It is an
operator-supplied pre-optimization observation from the PRD and is not a
current comparable median. The one permitted current-head diagnostic was:

```text
go test -v -count=1 -timeout=5m ./tests/functional/transport/run_scoped_server -run '^TestRunScopedServerUsesExactListenAddress$'
exit 0
package-reported: 10.604s (test body 10.49s)
measured command wall: 24.660s
```

The diagnostic passed its exact warning, success, URL, listener rebind, and
cleanup assertions. The package has no package-level topology printer, so the
resource counts below are source-owned observations from this selector rather
than an invented global census: one `root.BuildProcess` process, one exact
production listener, one pre-bind free-port probe, one post-completion rebind
probe, one home, one working directory, one workflow file, zero browser calls,
and one `Process.Execute` invocation. The production listener is closed before
the rebind probe succeeds.

## Census and classification rules

The 17 matrix rows below reconcile to 14 top-level tests and six named
subtests: two named/file invocation rows, two raw-JavaScript mode rows, and two
placement-conflict rows. No row is deleted, renamed, quarantined, or skipped in
the normal run. `TestRunScopedServerUsesProductionListenerAndReportsFallback`
has its existing conditional skip only when the OS allocates port 65535; the
focused diagnostic did not take that branch. No test adds a sleep or stable
window. Existing context and HTTP timeouts are hang guards, not synchronization
evidence.

The c01 inventory records every top-level row as `isolated-with-reason` with
`propertyType=listener`, `requiredFidelity=local_real`, and
`sharingInvalidatesBecause=Sharing would reuse or close the listener under
another row and erase bind/reachability/close ownership.` Its OS-boundary audit
verdict is `ACCIDENTAL-OS` with no allowed OS property claimed. That verdict is
retained here; it is not reclassified to make a future optimization pass.

Proposed owners are package-local candidates for TASK-002. “Shared root” means
only the application graph is a reuse candidate; HOME, working directory,
Factory Session, invocation streams, edge delegate/counters, listener, and
cleanup remain fresh per case. “Isolated” is retained when a real boundary or a
multi-step identity sequence is itself the property.

## RS-01..17 exact witness map

| Case | Current test or subtest (source line) | Exact observable assertions | Current resource owner and genuine boundary | c01 record | Proposed owner |
| --- | --- | --- | --- | --- | --- |
| RS-01 | `TestRunScopedServerAndSiteOwnNamedAndFileInvocationLifecycles/named positional server` (`run_scoped_server_test.go:45`) | Named `@you/goal` plus positional prompt; stdout contains `[0] factory started`, `--- primary result ---`, and ends with `mock worker accepted`; stderr contains `dispatch `, `active at execute-goal`, and `worker `; controlled listener starts/stops exactly `1/1`; browser calls `0`. | One root process; shaped `ProviderCommandRunner`; controlled `APIServerStarter` and `BrowserOpener`; fresh HOME, working directory, installed Factory, CLI input/output, Factory Session, and listener context; `t.TempDir`/process cleanup. | `C07-F037-top-level-test-TestRunScopedServerAndSiteOwnNamedAndFileInvocationLifecycles`; `isolated-with-reason/listener`; `PSO-0075`; `ACCIDENTAL-OS`. | `RS-HOSTED-PROVIDER`: shared-root candidate with fresh invocation state and controlled edge counters. |
| RS-02 | `TestRunScopedServerAndSiteOwnNamedAndFileInvocationLifecycles/file stdin site` (`run_scoped_server_test.go:47`) | File-selected `factory.json`, stdin `site-scoped goal`, input `-`, and `--with-site`; same exact stdout/progress assertions as RS-01; controlled listener starts/stops `1/1`; browser calls `1`. | Same controlled edge set as RS-01, with fresh HOME, working directory, file-selected Factory, stdin, Session, streams, and cleanup. | `C07-F037-top-level-test-TestRunScopedServerAndSiteOwnNamedAndFileInvocationLifecycles`; `isolated-with-reason/listener`; `PSO-0075`; `ACCIDENTAL-OS`. | `RS-HOSTED-PROVIDER`: share the graph only; retain fresh site/browser invocation state. |
| RS-03 | `TestRunScopedServerOwnsRawJavaScriptLifecycleAfterReadiness/server` (`run_scoped_server_test.go:128`) | Raw JavaScript run with `--with-server`; stderr is empty; stdout contains `completed (SUCCEEDED)`; supplied handler returns dashboard `200`; controlled listener starts/stops `1/1`; browser calls `0`. | One root process; raw workflow file; controlled `APIServerStarter` with dashboard handler probe and `OnBound`; `BrowserOpener`; fresh HOME/cwd and process/listener cleanup. | `C07-F037-top-level-test-TestRunScopedServerOwnsRawJavaScriptLifecycleAfterReadiness`; `isolated-with-reason/listener`; `PSO-0076`; `ACCIDENTAL-OS`. | `RS-HOSTED-JS`: shared-root candidate with fresh workflow/cwd/HOME and listener-context state. |
| RS-04 | `TestRunScopedServerOwnsRawJavaScriptLifecycleAfterReadiness/site` (`run_scoped_server_test.go:129`) | Raw JavaScript site run; stderr empty; stdout contains `completed (SUCCEEDED)`; dashboard handler is `200`; listener starts/stops `1/1`; browser calls `1`. | Same raw-JavaScript controlled listener/browser resources as RS-03, all paths and invocation state fresh. | `C07-F037-top-level-test-TestRunScopedServerOwnsRawJavaScriptLifecycleAfterReadiness`; `isolated-with-reason/listener`; `PSO-0076`; `ACCIDENTAL-OS`. | `RS-HOSTED-JS`: share graph with RS-03 only if browser/site state stays case-keyed. |
| RS-05 | `TestRunScopedRawJavaScriptServerReportsUnavailableWorkerSessionOwner` (`run_scoped_server_test.go:182`) | Direct host runs to `completed (SUCCEEDED)` with empty stderr; an in-process HTTP probe of `/factory-sessions/~default/worker-sessions/worker-missing/events` returns HTTP `500`; decoded `ErrorResponse.Code=INTERNAL_ERROR` and `Family=InternalServerError`. | One root process; raw workflow; controlled `APIServerStarter` owns a real `httptest.Server`, client, response body, probe channel, `OnBound`, and context; fresh HOME/cwd; deliberately absent live Worker Sessions owner. The in-process HTTP probe is the local-real public handler witness. | `C07-F037-top-level-test-TestRunScopedRawJavaScriptServerReportsUnavailableWorkerSessionOwner`; `isolated-with-reason/listener`; `PSO-0077`; `ACCIDENTAL-OS`. | `RS-DIRECT-HANDLER` isolated: retain the handler probe and missing-owner composition until a case-keyed server edge proves equivalent cleanup. |
| RS-06 | `TestRunScopedServerUsesProductionListenerAndReportsFallback` (`run_scoped_server_test.go:248`) | Occupied requested legacy port; exactly one `--server is deprecated` warning; stdout contains `completed (SUCCEEDED)`; reported dashboard host is `127.0.0.1` and fallback port is greater than requested; the actual fallback port rebinds after completion. Existing terminal-port conditional skip is unchanged. | One root process; real busy TCP listener; production listener fallback; raw workflow, HOME, cwd; post-run `net.Listen` rebind probe; listener/process/temp cleanup. Genuine property: ascending fallback and listener release/rebind. | `C07-F037-top-level-test-TestRunScopedServerUsesProductionListenerAndReportsFallback`; `isolated-with-reason/listener`; `PSO-0078`; `ACCIDENTAL-OS`. | `RS-LISTENER-COHORT`: shared graph candidate, fresh occupied port, production listener, URL, and rebind per case. |
| RS-07 | `TestRunScopedServerUsesExactListenAddress` (`run_scoped_server_test.go:304`) | `--listen` takes precedence and stderr equals the exact warning; stdout contains `completed (SUCCEEDED)` and the exact `Dashboard URL: http://127.0.0.1:<port>/dashboard/ui`; requested port rebinds after completion. | One root process; free-port reservation probe; real exact listener; raw workflow, HOME, cwd; rebind probe; process/listener/temp cleanup. Genuine property: exact bind, precedence warning, release/rebind. | `C07-F037-top-level-test-TestRunScopedServerUsesExactListenAddress`; `isolated-with-reason/listener`; `PSO-0079`; `ACCIDENTAL-OS`. | `RS-LISTENER-COHORT`: shared graph candidate with a fresh exact listener and port. |
| RS-08 | `TestRemotePlacementDispatchesThroughSelectedServer` (`run_scoped_server_test.go:339`) | Stdout is exactly `remote result`, stderr empty; remote receives exactly one POST `/factory-sessions/async` and one GET `/factory-sessions/dur-sess-remote/results`; local listener starts `0`; request source is non-nil inline Factory; normalized `prompt` is `same normalized request`. | One root process; local `httptest.Server` remote endpoint and request/result counters; fresh factory file, HOME, cwd, streams, and remote HTTP cleanup. Genuine property: remote placement bypasses local runtime/listener. | `C07-F037-top-level-test-TestRemotePlacementDispatchesThroughSelectedServer`; `isolated-with-reason/listener`; `PSO-0080`; `ACCIDENTAL-OS`. | `RS-REMOTE-DISPATCH`: shared-root candidate only with fresh remote server, request counters, and input state. |
| RS-09 | `TestRunScopedServerRejectsUnavailableExactListenAddress` (`run_scoped_server_test.go:415`) | Occupied exact port makes `Process.Execute` fail with `SERVER_BIND_FAILED`; stdout has no `Dashboard URL:`; stderr contains structured JSON code `SERVER_BIND_FAILED`. | One root process; real busy TCP listener; exact production bind failure; raw workflow, HOME, cwd, input/output buffers; busy listener and process cleanup. Genuine property: bind failure before readiness and no leaked listener. | `C07-F037-top-level-test-TestRunScopedServerRejectsUnavailableExactListenAddress`; `isolated-with-reason/listener`; `PSO-0081`; `ACCIDENTAL-OS`. | `RS-LISTENER-COHORT`: shared graph candidate; retain fresh occupied port and failed-start cleanup. |
| RS-10 | `TestRunScopedServerRejectsRemoteBindTargetAtCLIBoundary` (`run_scoped_server_test.go:452`) | A remote `--server` URL used as a local bind target makes `Process.Execute` fail with `not a local bind target`; no success path is claimed. | One root process; raw workflow path, fresh HOME/cwd, CLI input/output; validation returns before local listener startup. | `C07-F037-top-level-test-TestRunScopedServerRejectsRemoteBindTargetAtCLIBoundary`; `isolated-with-reason/listener`; `PSO-0082`; `ACCIDENTAL-OS`. | `RS-PLACEMENT-VALIDATION`: shared root candidate with fresh process inputs and zero-effect counters. |
| RS-11 | `TestRemotePlacementRejectsLocalHostingBeforeInitialization/persistent flags before run` (`run_scoped_server_test.go:515`) | Exact `ErrorResponse` on stderr: code `REMOTE_LOCAL_HOSTING_CONFLICT`, family `BadRequest`, and the stable full conflict message; stdout empty; listener/browser/Factory Session effects `0`. | The parent owns one controlled effect counter across both named rows; each row gets fresh HOME/cwd, TTY flags, streams, and context. Genuine property: validation precedes initialization and all local effects. | `C07-F037-top-level-test-TestRemotePlacementRejectsLocalHostingBeforeInitialization`; `isolated-with-reason/listener`; `PSO-0083`; `ACCIDENTAL-OS`. | `RS-CONFLICT-PAIR`: retain the parent-level effect ledger; fresh per-subtest input and cleanup. |
| RS-12 | `TestRemotePlacementRejectsLocalHostingBeforeInitialization/persistent flags after run` (`run_scoped_server_test.go:519`) | Same exact `REMOTE_LOCAL_HOSTING_CONFLICT` code/family/message; stdout empty; combined listener/browser/Factory Session effects remain `0`. | Same parent effect counter, with fresh subtest HOME/cwd/TTY/streams and no acquired listener, browser, or Session. | `C07-F037-top-level-test-TestRemotePlacementRejectsLocalHostingBeforeInitialization`; `isolated-with-reason/listener`; `PSO-0083`; `ACCIDENTAL-OS`. | `RS-CONFLICT-PAIR`: same intentional parent owner; do not split away the zero-effect aggregate assertion. |
| RS-13 | `TestRemotePlacementRejectsLocalOnlyServerCommand` (`run_scoped_server_test.go:561`) | Remote `server` command fails with `supports local placement only`; controlled listener starts `0`. | One root process; controlled `APIServerStarter` start counter; fresh HOME/cwd/streams; placement validation before local listener. | `C07-F037-top-level-test-TestRemotePlacementRejectsLocalOnlyServerCommand`; `isolated-with-reason/listener`; `PSO-0084`; `ACCIDENTAL-OS`. | `RS-PLACEMENT-VALIDATION`: share graph with fresh validation inputs and effect counter. |
| RS-14 | `TestRemotePlacementRejectsLocalOnlyFactoryCommand` (`run_scoped_server_test.go:589`) | Remote `factory config validate <path>` fails with `supports local placement only` before the requested file is inspected. | One root process; fresh HOME/cwd and missing Factory path; no listener/provider/session edge acquired. | `C07-F037-top-level-test-TestRemotePlacementRejectsLocalOnlyFactoryCommand`; `isolated-with-reason/listener`; `PSO-0085`; `ACCIDENTAL-OS`. | `RS-PLACEMENT-VALIDATION`: share graph candidate; retain fresh path and no-inspection validation boundary. |
| RS-15 | `TestRunRejectsMalformedExactListenAddress` (`run_scoped_server_test.go:610`) | `--listen 127.0.0.1` fails with `invalid --listen address`; controlled listener starts `0`. | One root process; controlled listener-start counter; fresh HOME/cwd/streams; exact-address parser returns before listener/runtime. | `C07-F037-top-level-test-TestRunRejectsMalformedExactListenAddress`; `isolated-with-reason/listener`; `PSO-0086`; `ACCIDENTAL-OS`. | `RS-PLACEMENT-VALIDATION`: share graph candidate with fresh malformed input and zero-effect observation. |
| RS-16 | `TestRunScopedServerReportsExhaustedTerminalPortAtCLIBoundary` (`run_scoped_server_test.go:637`) | Cross-process terminal-port lock is acquired; real `127.0.0.1:65535` is occupied; execution fails with exhaustion text containing `through 65535`; lock and listener release cleanly. | One root process; cross-process `terminalportlock`; real terminal TCP listener; raw workflow, HOME/cwd, buffers; deferred lock/listener/process cleanup. Genuine property: maximum-port exhaustion and global ownership. | `C07-F037-top-level-test-TestRunScopedServerReportsExhaustedTerminalPortAtCLIBoundary`; `isolated-with-reason/listener`; `PSO-0087`; `ACCIDENTAL-OS`. | `RS-TERMINAL-PORT-ISOLATED`: retain isolated root and serialized lock; sharing would invalidate global port ownership. |
| RS-17 | `TestRunScopedServerOwnsReplayLifecycle` (`run_scoped_server_test.go:694`) | A named run is recorded, then replayed with server; replay stdout is non-empty and stderr empty; listener starts/stops exactly `1/1`; browser calls `0`. | One root process; controlled listener/browser edges; named Factory installation, MockWorkers config, recording file, replay file, HOME/cwd, two sequential `Process.Execute` calls, and cleanup. Genuine property: record-then-replay identity sequence plus listener lifecycle. | `C07-F037-top-level-test-TestRunScopedServerOwnsReplayLifecycle`; `isolated-with-reason/listener`; `PSO-0088`; `ACCIDENTAL-OS`. | `RS-REPLAY-SEQUENCE-ISOLATED`: retain the record/replay sequence and fresh disposable recording root; do not share its terminal state with another case. |

## Evidence boundary and handoff

The list-only run and exact-list source review prove that all 17 RS rows have a
current identity, assertion set, resource owner, c01 classification, and
proposed owner. The focused selector proves one local-real exact-listener
journey and its release/rebind behavior. It does **not** prove optimized
post-change parity for RS-01..17, package timing under PR-CI, coverage,
repeat/race behavior, the built executable, Unix behavior, or merge. Those
edges belong to GATE-RS, GATE-RACE-RS, GATE-PERF, GATE-COVERAGE, GATE-LOOP,
and GATE-PR.

The source-plan artifact named by the PRD is ignored and absent in this
worktree. The PRD/task packet supplies the matrix used here; no source-plan or
c01 canonical inventory file was edited. If the inventory owner requires
current source-body hashes after this lane's baseline, the narrow delta is to
refresh the c01 source/hash rows for this package only.

## TASK-002 post-change topology and focused evidence

Implementation commit: `c9f63c74a9` (`test: share run-scoped transport roots`).
The prior source had 14 `BuildProcessWithContext` call sites and created 16
root instances when the 17 matrix rows were executed (the two two-row table
tests each built one root per subtest). The implementation uses three lazy,
edge-compatible package cohorts when all rows are selected:

| Cohort | Rows | Root topology | Retained real boundary |
| --- | --- | --- | --- |
| controlled hosted/provider | RS-01..05, RS-17 | one shared root; fresh case context, HOME, working directory, streams, Factory state, and edge counters | RS-05 local-real handler probe; controlled server lifecycle for the other hosted rows |
| controlled placement | RS-08, RS-10..15 | one shared root; fresh case context and validation inputs | RS-08 local `httptest.Server`; zero-local-effect validation |
| production listener | RS-06, RS-07, RS-09, RS-16 | one shared root; fresh case inputs and serialized `Process.Execute` calls | production listener bind/fallback/rebind, exact bind failure, and terminal-port exhaustion |

The package fixture's final counters report one build/close per used cohort,
balanced controlled listener starts/stops, zero active controlled listeners,
and per-case listener/browser/edge-effect assertions. The package-level
fixture close is owned by `TestMain`; no root is closed between compatible
rows, while every `Process.Execute` still receives fresh invocation inputs and
a case-local context route. No assertion, skip, quarantine, timeout, sleep, or
genuine listener boundary was removed.

Focused functional evidence on commit `c9f63c74a9` (Windows local-real
functional dependency fidelity; all commands exited 0):

```text
go test -v -run '^(TestRunScopedServerAndSiteOwnNamedAndFileInvocationLifecycles|TestRunScopedServerOwnsRawJavaScriptLifecycleAfterReadiness|TestRunScopedRawJavaScriptServerReportsUnavailableWorkerSessionOwner|TestRunScopedServerOwnsReplayLifecycle)$' -count=1 -timeout=5m ./tests/functional/transport/run_scoped_server
PASS; controlled roots=1/1, listeners=6/6, active=0, browsers=2

go test -v -run '^(TestRunScopedServerUsesProductionListenerAndReportsFallback|TestRunScopedServerUsesExactListenAddress|TestRunScopedServerRejectsUnavailableExactListenAddress|TestRunScopedServerReportsExhaustedTerminalPortAtCLIBoundary)$' -count=1 -timeout=5m ./tests/functional/transport/run_scoped_server
PASS; production roots=1/1; all real bind/rebind/exhaustion assertions passed

go test -v -run '^(TestRemotePlacementDispatchesThroughSelectedServer|TestRunScopedServerRejectsRemoteBindTargetAtCLIBoundary|TestRemotePlacementRejectsLocalHostingBeforeInitialization|TestRemotePlacementRejectsLocalOnlyServerCommand|TestRemotePlacementRejectsLocalOnlyFactoryCommand|TestRunRejectsMalformedExactListenAddress)$' -count=1 -timeout=5m ./tests/functional/transport/run_scoped_server
PASS; validation roots=1/1, listeners=0/0, active=0
```

The changed shared fixture also passed the declared bounded gates:

```text
go test -v -run '^TestRunScopedServerAndSiteOwnNamedAndFileInvocationLifecycles$' -count=3 -timeout=10m ./tests/functional/transport/run_scoped_server
PASS; one controlled root, six balanced listeners, active=0

go test -race -v -run '^TestRunScopedServerAndSiteOwnNamedAndFileInvocationLifecycles$' -count=1 -timeout=10m ./tests/functional/transport/run_scoped_server
PASS; no race report; one controlled root, two balanced listeners, active=0
```

These focused runs prove RS-01..17 parity at the declared functional level,
fresh shared-fixture isolation, cleanup balance, and material root reduction.
They do not prove the comparable PR-CI under-three-second timing gate,
functional coverage, the story-005 single final full-package run, clean-room
loopback, terminal CI, or merge; those remain GATE-PERF, GATE-COVERAGE,
GATE-LOOP, and GATE-PR inputs.
