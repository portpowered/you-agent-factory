# Transport protocol-family characterization: run-scoped server

This ledger is the package-local evidence for
`functional-test-optimization-c15-transport-protocol-family-001`.
It freezes the current RS-01..17 witness before any fixture topology change.
The opening section is the characterization record; the review follow-up below
records post-change parity and performance evidence.

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
The comparable CI timing and irreducibility evidence is recorded below;
functional coverage, terminal CI, and merge remain review-owned gates.

## Review follow-up: comparable timing and irreducibility

### Comparable package timing

The following is the comparable package-level denominator requested in review.
Both sides use the `seconds` package field from the same CI workflow's
`functional-timing-summary.json` on Linux X64; local Windows wall time is not
used for this comparison.

| Package | Before CI package median / sample | After CI package median / sample | Delta | Disposition |
| --- | ---: | ---: | ---: | --- |
| `run_scoped_server` | 29.183s | 21.854s | -7.329s (-25.1%) | Direct `<3s` target blocked; irreducibility alternative complete |

Before provenance is the base CI run [33337769566](https://github.com/portpowered/you-agent-factory/actions/runs/33337769566), head `42eeee4472656b8290f798c36a5b8c871b24d7d0`, Backend Functional Coverage job [99327753325](https://github.com/portpowered/you-agent-factory/actions/runs/33337769566/job/99327753325), and its [functional-test-diagnostics artifact](https://github.com/portpowered/you-agent-factory/actions/runs/33337769566/artifacts/9739638631). The `29.183s` value is the supplied admission median and is now tied to the same package timing field and CI denominator.

After provenance is the matching PR CI run [33348730534](https://github.com/portpowered/you-agent-factory/actions/runs/33348730534), head `c2c429a62dcb0e86eaca8ba87eb712ce2756387a`, Backend Functional Coverage job [99357847982](https://github.com/portpowered/you-agent-factory/actions/runs/33348730534/job/99357847982), and its [functional-test-diagnostics artifact](https://github.com/portpowered/you-agent-factory/actions/runs/33348730534/artifacts/9742979739). The artifact's `functional-tests.md` supplies the post-change top-level test timings below.

### Complete measured irreducibility table

The CI artifact reports top-level test elapsed cost. The seven named child
rows below use focused `go test -json -count=1 -timeout=20m` measurements at
the final head because CI's report aggregates those children into their parent
test. The focused commands all exited `0`; parent CI elapsed values are shown
in parentheses as the corresponding top-level report value, not as a sum across
local child runs. Every row retains the exact boundary listed in the witness
map, names the owner of the irreducible cost, and records the direct-target
disposition.

| Case | Measured elapsed cost | Retained real boundary | Irreducibility reason / owner | Resulting disposition |
| --- | ---: | --- | --- | --- |
| RS-01 | 2.510s focused child (parent CI 5.180s) | Controlled listener, provider edge, Factory Session, and fresh CLI streams | Hosted provider invocation and listener effect stay case-local; `RS-HOSTED-PROVIDER` | **BLOCKED** direct `<3s`; alternative complete |
| RS-02 | 7.990s focused child (parent CI 5.180s) | Controlled listener/browser, site input, Factory Session, and fresh stdin | Site/browser and file/stdin state cannot cross a case; `RS-HOSTED-PROVIDER` | **BLOCKED** direct `<3s`; alternative complete |
| RS-03 | 1.680s focused child (parent CI 2.120s) | Controlled dashboard handler/listener and raw JavaScript readiness | Listener context and dashboard probe stay case-keyed; `RS-HOSTED-JS` | **BLOCKED** direct `<3s`; alternative complete |
| RS-04 | 1.540s focused child (parent CI 2.120s) | Controlled dashboard handler/listener and browser site path | Browser/site lifecycle is case-local; `RS-HOSTED-JS` | **BLOCKED** direct `<3s`; alternative complete |
| RS-05 | 1.270s PR-CI top-level | Local-real HTTP handler probe for an unavailable Worker Session owner | Missing-owner handler composition remains isolated; `RS-DIRECT-HANDLER` | **BLOCKED** direct `<3s`; alternative complete |
| RS-06 | 1.630s PR-CI top-level | Busy TCP port, production fallback bind, and post-run rebind | Port occupancy and listener release are real OS effects; `RS-LISTENER-COHORT` | **BLOCKED** direct `<3s`; alternative complete |
| RS-07 | 1.380s PR-CI top-level | Exact production bind and post-run rebind | Requested-port identity and release cannot be shared; `RS-LISTENER-COHORT` | **BLOCKED** direct `<3s`; alternative complete |
| RS-08 | 0.330s PR-CI top-level | Local remote-placement HTTP endpoint and zero local listener effect | Remote request/result ownership is case-local; `RS-REMOTE-DISPATCH` | **BLOCKED** direct `<3s`; alternative complete |
| RS-09 | 1.010s PR-CI top-level | Busy exact port and production bind failure | Bind failure and no-readiness cleanup are real listener boundaries; `RS-LISTENER-COHORT` | **BLOCKED** direct `<3s`; alternative complete |
| RS-10 | 0.110s PR-CI top-level | CLI placement validation with zero listener effect | Invalid remote bind target must fail before initialization; `RS-PLACEMENT-VALIDATION` | **BLOCKED** direct `<3s`; alternative complete |
| RS-11 | 0.040s focused child (parent CI 0.050s) | Pre-initialization conflict and aggregate zero-effect counters | Both flag placements must retain the same parent-owned zero-effect proof; `RS-CONFLICT-PAIR` | **BLOCKED** direct `<3s`; alternative complete |
| RS-12 | 0.040s focused child (parent CI 0.050s) | Pre-initialization conflict and aggregate zero-effect counters | Both flag placements must retain the same parent-owned zero-effect proof; `RS-CONFLICT-PAIR` | **BLOCKED** direct `<3s`; alternative complete |
| RS-13 | 0.020s PR-CI top-level | Remote local-only server validation and zero listener effect | Local-only command rejection precedes listener startup; `RS-PLACEMENT-VALIDATION` | **BLOCKED** direct `<3s`; alternative complete |
| RS-14 | 0.040s PR-CI top-level | Remote local-only Factory validation before file inspection | Placement rejection and missing-path state stay case-local; `RS-PLACEMENT-VALIDATION` | **BLOCKED** direct `<3s`; alternative complete |
| RS-15 | 0.040s PR-CI top-level | Malformed exact-address parser and zero listener effect | Parser failure precedes runtime/listener acquisition; `RS-PLACEMENT-VALIDATION` | **BLOCKED** direct `<3s`; alternative complete |
| RS-16 | 1.150s PR-CI top-level | Cross-process terminal-port lock, port 65535 listener, and release | Global terminal-port ownership cannot be shared; `RS-TERMINAL-PORT-ISOLATED` | **BLOCKED** direct `<3s`; alternative complete |
| RS-17 | 4.020s PR-CI top-level | Record-then-replay sequence, recording files, listener, and rebind | Replay identity and terminal state are a multi-step sequence; `RS-REPLAY-SEQUENCE-ISOLATED` | **BLOCKED** direct `<3s`; alternative complete |

Focused child timing procedures at the final head were:

```text
go test -json -count=1 -timeout=20m ./tests/functional/transport/run_scoped_server -run '^TestRunScopedServerAndSiteOwnNamedAndFileInvocationLifecycles$'
exit 0; named_positional_server=2.510s; file_stdin_site=7.990s; parent=10.490s

go test -json -count=1 -timeout=20m ./tests/functional/transport/run_scoped_server -run '^TestRunScopedServerOwnsRawJavaScriptLifecycleAfterReadiness$'
exit 0; server=1.680s; site=1.540s; parent=3.230s

go test -json -count=1 -timeout=20m ./tests/functional/transport/run_scoped_server -run '^TestRemotePlacementRejectsLocalHostingBeforeInitialization$'
exit 0; persistent_flags_before_run=0.040s; persistent_flags_after_run=0.040s; parent=0.190s
```

### GATE-PERF disposition

The post-change package sample remains above three seconds, so the direct
under-three-second branch is recorded as **BLOCKED**. The permitted alternative
is complete: all 17 rows have measured cost, every retained real boundary has
an explicit irreducibility reason and owner, and the bounded root-reuse pass
was already completed before this evidence follow-up. No residual row is
silently treated as an optimization opportunity or omitted from the matrix.
