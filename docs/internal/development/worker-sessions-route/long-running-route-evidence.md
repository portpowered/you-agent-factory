# Worker Sessions route characterization

Status: GATE-BASELINE and GATE-CHARACTERIZE complete before any product correction.

## Finding

The current head does not reproduce the reported `FACTORY_UNREACHABLE` result in
the supported local fixtures. There is therefore no first failing hop to fix:
the controlled requests reached the complete route and returned the expected
observations. No production diagnostic or behavior correction was added in this
characterization story.

This bounded result does not prove behavior during the reported approximately
2.5-day daemon run. Literal long-duration verification remains the named
post-merge operator gate.

## Test identity and fidelity

- Characterization source head: `7ba45b8028` (`test(worker-sessions): characterize default route hops`).
- Base production head used for the pre-correction comparison: `905d5fe06e54f2cc18fa9d5ed4f08d72e5d85739`.
- Environment: Windows `10.0.26200`, `go1.25.0`, `amd64`.
- Dependency fidelity: local root-built production process, generated HTTP router,
  live Factory Session runtime, and a deterministic injected provider-command
  edge. No remote or paid provider was used.
- Fixture fidelity: single-step `task` Factory, default `~default` selector,
  public Work submission, public Factory Event SSE association capture, public
  REST read, and `Process.Execute` CLI read. The accumulated probe admitted
  exactly 32 sequential controlled provider calls. The resume probe used the
  supported public default-session pause/resume controls and two controlled
  calls.

## Safe behavioral witness

The operator command form exercised was:

```text
you worker-sessions list --work-id <work-id> --server <url>
you worker-sessions list --work-id <work-id> --server <url> --output json
```

The matching REST request was:

```text
GET /factory-sessions/~default/worker-sessions?workId=<work-id>
```

`<work-id>` and `<url>` above are redaction placeholders; no local paths,
credentials, payloads, provider transcript, or raw response body is retained
in this ledger. Expected identities were independently captured from the public
`DISPATCH_REQUEST` context and the public
`DISPATCH_WORKER_SESSION_ASSOCIATION` payload, joined by dispatch ID.

For the run on `7ba45b8028`, the public association samples were:

| Probe | Work ID | Dispatch / attempt ID | Worker Session ID | Factory Session | State |
| --- | --- | --- | --- | --- | --- |
| Fresh / first | `batch-request-cca44780-7fd6-4158-9dfb-0570209dbc80-route-characterization-work-01` | `c97052f0-189d-415e-baf3-cb98359f1d01` | `c97052f0-189d-415e-baf3-cb98359f1d01` | `~default` | `COMPLETED` |
| Accumulated / middle (16th) | `batch-request-0e577a59-435d-4e7d-a389-7d30db693976-route-characterization-work-16` | `683f5b66-e1ae-488b-8d86-905dfe69a595` | `683f5b66-e1ae-488b-8d86-905dfe69a595` | `~default` | `COMPLETED` |
| Accumulated / final (32nd) | `batch-request-4864d977-88b5-4728-bd63-e0e37f15bc9c-route-characterization-work-32` | `9b5cf63f-9f4a-4420-9c0a-e0565fef1fe6` | `9b5cf63f-9f4a-4420-9c0a-e0565fef1fe6` | `~default` | `COMPLETED` |
| Pause/resume / pre | `batch-request-e8738434-14b8-4d19-a933-2c916e0df362-route-characterization-pre-resume` | `6a9e8c0f-6297-4ac1-b4fb-29b0cc2f0224` | `6a9e8c0f-6297-4ac1-b4fb-29b0cc2f0224` | `~default` | `COMPLETED` |
| Pause/resume / post | `batch-request-cb881dd1-b964-4ea8-b258-6d7dce8b53b1-route-characterization-post-resume` | `c655827b-2844-4ebb-b9a6-04af5a041c36` | `c655827b-2844-4ebb-b9a6-04af5a041c36` | `~default` | `COMPLETED` |

The CLI JSON document and direct REST document were value-equal for every
sampled Work. Human CLI output visibly contained the fresh Worker Session ID.
The accumulated public event snapshot contained 32 association events and 32
dispatch responses.

## Route-hop ledger

The stage result is based on the successful public request and exact identity
comparison; no internal production instrumentation was required.

| Hop | Result | Evidence |
| --- | --- | --- |
| CLI client | reached | `Process.Execute` returned nil for the human and JSON list forms; JSON decoded as one exact observation. |
| Generated router | reached | The matching REST request returned HTTP 200 through the generated route. |
| Top-level server binding | reached | The request was served by the live server owned by the same root-built process. |
| Worker Sessions handler | reached | The Work-scoped response contained one observation and the expected Worker Session identity. |
| Work read | reached | The response was scoped to the submitted Work ID and returned the expected Work correlation. |
| Selector resolution | reached | The request used `~default` and the response reported Factory Session `~default`. |
| Session observation selection | reached | The selected observation matched the independently captured dispatch association after 32 calls and after pause/resume. |
| Worker Sessions service read | reached | `workerSessionId`, `attemptId`, `workId`, `workIds`, and terminal state matched the public association-derived expectation. |
| Response encoding | reached | REST JSON decoded successfully; CLI JSON decoded successfully; human output included the identity. |

First failing hop: none observed. A complete successful route is the honest
current-head characterization; it is not evidence that the historical daemon
failure was impossible or that multi-day uptime has been proved.

## GATE-BASELINE

Procedure:

```text
go test ./pkg/services/worker_sessions/transports/cli ./pkg/services/worker_sessions/transports/http -count=1
go test ./tests/functional/workers/transports/http -run '^TestWorkerSessionHTTPReadDuringFactoryWork$' -count=1
```

Result on the characterization head: PASS. The focused CLI package and HTTP
package passed; the existing fresh in-flight explicit-session functional route
passed.

Proved: existing Worker Sessions CLI mapping, HTTP handler behavior, and the
fresh explicit-session in-flight route remain available.

Not proved: the historical long-running daemon, the final correction, actual
binary packaging, or the complete later functional/unhappy-case matrix.

## GATE-CHARACTERIZE

Procedure: the committed tests
`TestWorkerSessionRouteCharacterization_FreshAndExactly32Dispatches` and
`TestWorkerSessionRouteCharacterization_AfterDefaultPauseResume` build through
the functional root, start the production HTTP server, use a controlled command
edge, capture public associations, and issue the exact CLI and REST reads.

Result: PASS on `7ba45b8028`. Fresh first-read, exactly 32 sequential
accumulated reads (first, middle, final), and supported default pause/resume
reads all completed with exact identity parity. The provider edge was called
exactly 32 times in the accumulated fixture and exactly twice in the
pause/resume fixture. No route error, stale observation, dispatch interference,
or sensitive diagnostic was observed.

Proved: the first faulty or conclusively successful hop in these bounded
supported fixtures is the complete public route, and the current head does not
reproduce the reported failure.

Not proved: product correction, OS executable loopback, production proxies or
infrastructure, and literal multi-day uptime.

## Remaining gates and operator handoff

- GATE-FUNCTIONAL: later correction story; complete public behavior matrix is
  not claimed here.
- GATE-LOOPBACK: later built-binary validation story; not claimed here.
- GATE-OPERATOR-LONG-RUN: after merge, run the exact customer CLI and matching
  REST request against the healthy deployment after a named long-duration
  window. Compare the returned Worker Session ID, Work ID/workIds, Factory
  Session alias, attempt ID, and state with the public dispatch association;
  stop and file a new correction lane if any hop returns
  `FACTORY_UNREACHABLE`, a non-success response, a mismatched identity, or a
  stale session. A bounded local pass must not be described as proof of the
  historical duration.

