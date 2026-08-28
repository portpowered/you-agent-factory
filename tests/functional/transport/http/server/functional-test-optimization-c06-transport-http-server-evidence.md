# C06 HTTP-server functional characterization evidence

This artifact freezes the HTTP-server behavior and process/listener topology
before the shared-process migration and records the staged migration evidence.
Stories 001 through 004 update it within the package-owned surface.

## Scope and source note

- Owned surface: `tests/functional/transport/http/server/**`.
- Baseline head: `67710223e` (`origin/main` at characterization time).
- No production code, public contract, shared functional support, global
  inventory, or CI file was changed.
- The PRD references `docs/temp/functional-test-optimization.md`, but that file
  is absent from this checkout. The matching task and acceptance excerpt in
  `prd.md` (the coverage assessment and TASK-001 sections) supplied enough
  authority to complete this bounded characterization. No missing-plan content
  was reconstructed and no scope was expanded. Restoring or explicitly
  re-authorizing that source plan is the smallest follow-up before interpreting
  any later scope dispute.

## GATE-CHAR

Required procedure:

```text
go test -count=1 -timeout=10m -json ./tests/functional/transport/http/server
```

Observed on 2026-08-28 on the shared Windows workstation:

| Run | Head / state | Exit | Wall time | JSON pass events | JSON fail events |
| --- | --- | ---: | ---: | ---: | ---: |
| Pre-change baseline | `67710223e`, before characterization assertions | 0 | 31.903 s | 98 | 0 |
| Post-characterization check | same production topology, after the two bounded witness additions below | 0 | 44.291 s | 98 | 0 |

The PRD's retained diagnostic observations are a 40.107-second focused local
run and supplied CI observations of approximately 14.45/14.55 seconds. They
are prioritization data, not local thresholds; the host was compute-saturated.

The package currently enumerates 21 top-level tests with `go test -list .`,
not the stale count of 20 in the PRD's general prose. The exhaustive route
test expands to 70 OpenAPI operation subtests plus its shutdown subcase. All
top-level tests and meaningful subcases passed in both GATE-CHAR runs.

This gate proves the current public behavior, the package-local classification,
and the pre-migration topology below. It does not prove shared-process reuse,
explicit-session migration of every stateful case, repeat/race safety, final
resource-ledger assertions, CI timing, or the clean-room validation.

## Bounded characterization assertions added before migration

These additions observe runtime behavior at the existing public HTTP boundary;
they do not inspect source topology or replace an existing assertion.

- `content_negotiation_test.go` now proves a valid opened session is non-default,
  terminates and deletes it, and is then absent through typed `GET`; rejected
  unsupported-media-type and malformed-JSON requests preserve the live-session
  ID set.
- `work_terminal_response_test.go` now proves the terminal success/failure
  Work IDs have correlated public `DISPATCH_RESPONSE` Factory Events in the
  default session with `ACCEPTED`/`FAILED` outcomes. The current runtime does
  not emit a `WORK_STATE_CHANGE` event for this path; Work terminal state is
  instead observed through status and Work detail. Story 003 owns migrating
  this witness to explicit sessions and preserving the observed event shape.

## Story 003 migration evidence

The terminal Work success and provider-failure cells now use the package-owned
shared root-built process. Each cell creates a unique non-default Factory
Session, registers a selector-scoped controlled command runner, submits Work
through the session-scoped HTTP API, reads the session-scoped Work list/detail
and Factory Event history, and lets scenario cleanup prove session absence,
zero active calls, zero registrations, zero response streams, and temporary
Factory-root removal.

Required story procedure, observed on 2026-08-28 on the shared Windows
workstation:

```text
go test -count=1 -timeout=10m ./tests/functional/transport/http/server -run 'Test(GeneratedClient|WorkTerminal)'
go test -count=1 -timeout=10m ./tests/functional/transport/http/server
```

Both commands exited 0. The focused run took 3.831 s; the assembled package
gate took 22.943 s. The focused run proves generated-client recovery plus
terminal success/failure Work/Event/session lineage, ordered typed content,
controlled provider failure classification, and route invocation. The
assembled gate additionally proves those cells coexist with the remaining
package cases and package fixture cleanup. The shared fixture ledger asserted
exactly one shared API process start; the two pre-migration terminal process
constructions now contribute no additional process starts. Local real-listener
lifecycle, repeat/race safety, and CI contention timing remain later-story
edges.

## Top-level test and public-witness inventory

Every top-level test observed by `go test -list .` is listed. The CASE matrix
below supplies the complete CASE-01 through CASE-25 mapping.

| Top-level test | CASE mapping | Public witness / meaningful subcases | Initial classification |
| --- | --- | --- | --- |
| `TestAPIConcurrentSessionRequestsRemainIsolated` | CASE-13 | Concurrent HTTP identity/status requests for two sessions | Shareable with controlled edge |
| `TestAPICancelledRequestDoesNotCancelUnrelatedSession` | CASE-14 | Canceled invocation and unrelated session identity/status | Shareable with controlled edge |
| `TestAPIJSONRequestsAndResponsesUseDocumentedContentType` | CASE-07 | Documented JSON request/response media types and explicit session lifecycle | Shareable |
| `TestAPIUnsupportedContentTypeReturns415` | CASE-05 | Structured 415, `UNSUPPORTED_MEDIA_TYPE`, no session admission | Shareable |
| `TestAPIMalformedJSONReturnsStructured400` | CASE-06 | Structured 400, `BAD_REQUEST`, no session admission | Shareable |
| `TestGeneratedClientStatusAndSessionRoundTrip` | CASE-08, CASE-09, CASE-11 | Generated-client typed status/session, deadline, cancellation, and stream behavior | Shareable with controlled edge |
| `TestGeneratedClientDecodesRepresentativeStructuredError` | CASE-10 | Generated-client typed 404 `NOT_FOUND` response | Shareable |
| `TestGeneratedClientAndServerSchemaStayAligned` | CASE-08, CASE-12 | Generated-client Work/status schema, terminal status, stale-cursor recovery | Shareable with controlled edge |
| `TestAPIServerDiagnosticsUseProductionLoopbackStarter` | CASE-21 | Real loopback diagnostics disabled/enabled subtests | Isolated with reason |
| `TestAPIServerGracefulShutdownThroughProductionLoopbackLifecycle` | CASE-22 | Independent stop caller, serve completion, stream closure, listener observer | Isolated with reason |
| `TestListenerStopObserverReportsBoundedOpenListenerOutcomes` | CASE-24 | Real open listener deadline vs canceled observer result | No application process; isolated listener fixture |
| `TestAPIRoutesEveryOpenAPIOperationToNon404Handler` | CASE-01 | 70 OpenAPI operations and final `shutdownServer` subcase | Isolated as a whole because shutdown is destructive |
| `TestAPIUnknownRouteReturnsStructuredNotFound` | CASE-02 | Structured JSON 404 and typed `NOT_FOUND` | Shareable |
| `TestAPIDashboardRoutesServeEmbeddedShellAssetAndFallback` | CASE-03 | Shell, asset, fallback, and incomplete-query responses | Shareable |
| `TestAPIWrongMethodReturnsDocumentedMethodError` | CASE-04 | Structured JSON 405 `METHOD_NOT_ALLOWED`, distinct from 404 | Shareable |
| `TestAPIServerPprofIsOptInThroughThePublicRunPath` | CASE-17, CASE-18 | Diagnostics-disabled and diagnostics-enabled subtests | Isolated with reason |
| `TestAPIServerStartsOnConfiguredListenerAndServesStatus` | CASE-19 | Configured startup path and populated `/status` | Isolated with reason |
| `TestAPIServerUsesPlatformStarterThroughRootProcess` | CASE-21 | Root-built process, real platform starter, status/pprof, listener close | Isolated with reason |
| `TestAPIServerShutdownClosesListenerAndActiveStreams` | CASE-20 | Active invocation termination, process join, listener refusal/rebind | Isolated with reason |
| `TestAPIServerBindFailureUnwindsStartedLifecycleRoles` | CASE-23 | Exact rejected ports, structured bind failure, no readiness effects, rebind | Isolated with reason |
| `TestWorkTerminalResponsePreservesOrderedTypedContentThroughPublicBoundary` | CASE-15, CASE-16 | Controlled success/failure, status, list/detail typed content/order, Event correlation | Shareable with controlled edge |

## CASE-01 through CASE-25 matrix

“Covered” means the named current test passed and the listed public witness was
observed. “Partial” records a property explicitly deferred to a later story;
it is still mapped so migration cannot silently lose it.

| CASE | Existing test/subcase | Public witness preserved at characterization | Classification / status |
| --- | --- | --- | --- |
| CASE-01 | `TestAPIRoutesEveryOpenAPIOperationToNon404Handler`; 70 operation subtests and final shutdown | Each published operation receives its safe request and reaches its intended non-404 or typed domain-not-found handler; final `shutdownServer` returns 202 and stops its server | Shareable route portion; isolated shutdown portion — covered |
| CASE-02 | `TestAPIUnknownRouteReturnsStructuredNotFound` | HTTP 404, JSON content type, `NOT_FOUND` family/code, non-empty message, server remains usable | Shareable — covered |
| CASE-03 | `TestAPIDashboardRoutesServeEmbeddedShellAssetAndFallback` | Shell and asset return 200 with existing body markers; client fallback returns 200/root element; incomplete provider-session query returns 400 | Shareable — covered |
| CASE-04 | `TestAPIWrongMethodReturnsDocumentedMethodError` | POST `/status` returns structured JSON 405 `METHOD_NOT_ALLOWED`, distinct from unknown-route 404 | Shareable — covered |
| CASE-05 | `TestAPIUnsupportedContentTypeReturns415` | `text/plain` request returns structured 415 `UNSUPPORTED_MEDIA_TYPE`; live Factory Session IDs do not change | Shareable — covered |
| CASE-06 | `TestAPIMalformedJSONReturnsStructured400` | Malformed `application/json` returns structured 400 `BAD_REQUEST`, not 415; live Factory Session IDs do not change | Shareable — covered |
| CASE-07 | `TestAPIJSONRequestsAndResponsesUseDocumentedContentType` | Valid documented JSON opens a unique non-default session with documented response type; terminate/delete/typed GET proves absence | Shareable — covered |
| CASE-08 | `TestGeneratedClientStatusAndSessionRoundTrip`; `TestGeneratedClientAndServerSchemaStayAligned`; Work terminal tests | Caller-owned generated-client HTTP transport is used; typed status/events/session responses and terminal Work observation remain available | Shareable / controlled edge — covered on shared process with explicit sessions |
| CASE-09 | `TestGeneratedClientStatusAndSessionRoundTrip/deadline` | Outstanding generated-client response ends with `context.DeadlineExceeded` and no typed success | Shareable — covered; direct active-stream counter deferred |
| CASE-10 | `TestGeneratedClientDecodesRepresentativeStructuredError` | Missing session produces generated-client typed 404, `NOT_FOUND`, `RESPONSE_EVENT_SESSION_NOT_FOUND`, and message | Shareable — covered |
| CASE-11 | `TestGeneratedClientStatusAndSessionRoundTrip` cancellation subcases | In-flight and pre-canceled generated-client calls return cancellation without a typed response | Shareable — covered; direct call/stream ledger deferred |
| CASE-12 | `TestGeneratedClientAndServerSchemaStayAligned` recovery subcase | Completed Work/status is typed; stale cursor recovery returns `CURSOR_STALE` and retry omits the stale cursor | Controlled edge — covered on shared process with explicit session |
| CASE-13 | `TestAPIConcurrentSessionRequestsRemainIsolated` | Overlapping requests return the correct session identities and non-empty state for two sessions | Controlled edge — covered in separate processes; one shared-process proof deferred |
| CASE-14 | `TestAPICancelledRequestDoesNotCancelUnrelatedSession` | Canceling one blocking request does not change the unrelated session identity/status | Controlled edge — covered in separate processes; shared-session cleanup deferred |
| CASE-15 | `TestWorkTerminalResponsePreservesOrderedTypedContentThroughPublicBoundary/terminal success keeps ordered typed parts` | One terminal/zero failed; list/detail retain text then JSON type, payload, and order; correlated `DISPATCH_RESPONSE` is `ACCEPTED` in a unique explicit session | Controlled edge — covered on shared process |
| CASE-16 | `TestWorkTerminalResponsePreservesOrderedTypedContentThroughPublicBoundary/terminal failure is not reported as success` | One failed/zero terminal; Work is FAILED; content remains readable/ordered; correlated `DISPATCH_RESPONSE` is `FAILED` in a unique explicit session | Controlled edge — covered on shared process |
| CASE-17 | `TestAPIServerPprofIsOptInThroughThePublicRunPath` default subcase | Pprof/debug probes are absent or typed 404; status/runtime and metrics remain available; process stops | Isolated diagnostics mode — covered |
| CASE-18 | Same test enabled subcase | Index, heap, named/CPU/delta/trace/text profiles and invalid/unknown probes preserve status/header/body/error witnesses; process stops | Isolated diagnostics mode — covered |
| CASE-19 | `TestAPIServerStartsOnConfiguredListenerAndServesStatus` | Public startup returns loopback URL and populated `/status`; the support transport is `httptest`, so exact OS-port ownership is separately covered by CASE-21 | Isolated startup/configuration — covered with this boundary note |
| CASE-20 | `TestAPIServerShutdownClosesListenerAndActiveStreams` | Public stop joins process, closes active request/stream, refuses listener probes, and permits rebind | Isolated shutdown — covered |
| CASE-21 | `TestAPIServerUsesPlatformStarterThroughRootProcess`; production diagnostics subtests | Root-built process reaches real platform loopback listener; status and opt-in diagnostics work; listener closes | Isolated real-listener fidelity — covered |
| CASE-22 | `TestAPIServerGracefulShutdownThroughProductionLoopbackLifecycle` | Independent public stop prints confirmation; serve command returns; response stream closes; listener-stop observer succeeds | Isolated graceful shutdown — covered |
| CASE-23 | `TestAPIServerBindFailureUnwindsStartedLifecycleRoles` | Requested/fallback addresses are rejected exactly; structured `SERVER_BIND_FAILED`; no readiness/browser effects; requested port rebinds and `/status` is unreachable | Isolated bind failure — covered; direct process-close assertion deferred |
| CASE-24 | `TestListenerStopObserverReportsBoundedOpenListenerOutcomes` | Open real listener distinguishes `DeadlineExceeded` from `Canceled`; deferred listener, connections, and accept goroutine cleanup | Isolated listener observer, no application process — covered |
| CASE-25 | Package `t.Cleanup`, `server.Stop`, `support.CleanupProcess`, and existing session teardown paths | Existing tests release their process/server fixtures through test cleanup; no package-wide runtime ledger, zero-count assertion, or three-repeat proof exists yet | Cross-cutting — partial; GATE-CLEAN/GATE-REPEAT owned by Stories 002–004 |

## Pre-migration runtime topology

The counts below are reviewer evidence from the existing runtime fixture call
graph expanded by top-level/subtest execution. They are not a source-scanning
meta-test. The support helper creates one root-built `Process` and one
`ProcessAPIServer` per invocation; direct lifecycle tests create the process
and production starter separately.

### Root-built process constructions

| Runtime group | Process constructions | Why it is counted this way | Future class |
| --- | ---: | --- | --- |
| Routing: unknown route, dashboard, wrong method | 3 | One `StartFunctionalAPIServer` per test | Shareable |
| Content negotiation: valid, unsupported, malformed | 3 | One per test; valid case now proves explicit-session removal | Shareable |
| Generated client | 3 | Status/session, structured error, schema/recovery tests | Shareable or controlled edge |
| Concurrent/cancellation | 2 | One per top-level test with controlled blocking runner | Shareable with controlled edge |
| Terminal Work | 2 | One per terminal subtest through the helper, success and provider failure | Shareable with controlled edge |
| Exhaustive routing inventory | 1 | The final `shutdownServer` operation terminates the process | Isolated |
| Pprof default/opt-in | 2 | Mutually exclusive process startup flags | Isolated |
| Configured-listener test | 1 | Startup argument/configuration witness | Isolated |
| Platform-starter real listener | 1 | OS listener, Serve, and close fidelity | Isolated |
| Active-stream shutdown | 1 | The test stops its server and observes active request teardown | Isolated |
| Production diagnostics | 2 | One real server for each diagnostics mode | Isolated |
| Production graceful shutdown | 2 | One real server plus an independent root-built stop caller | Isolated |
| Bind failure | 1 | Deterministic failing starter and lifecycle unwind | Isolated |
| **Total** | **24** | **13 eligible candidates + 11 isolated process instances; zero unclassified** | |

The listener observer has no application process. The 13 eligible candidates
are the routing, negotiation, generated-client, concurrency, cancellation, and
terminal-Work groups; the final split between plain sharing and controlled edge
routing is owned by Stories 002 and 003.

### Listener constructions and lifecycle outcomes

| Listener category | Successful binds | Rejected attempts | Close/release observation |
| --- | ---: | ---: | --- |
| API listeners: 18 `httptest` support servers plus 4 production loopback servers | 22 | 0 | Existing server/process cleanup paths; final direct ledger is deferred |
| Auxiliary reservations, rebind probes, and observer fixture | 9 | 0 | Immediate close or observer `defer` cleanup in the owning test |
| Bind-failure starter | 0 | 2 (`127.0.0.1:65534`, fallback `127.0.0.1:65535`) | Requested address is rebound after failure; direct process-close assertion remains deferred |
| **Total listener operations** | **31** | **2** | **All successful auxiliary binds have explicit release; API/process ledger finalization is deferred** |

No process or listener construction is unclassified. These counts are a
pre-change inventory, not a target or a post-migration result.

## Cleanup and remaining evidence ownership

Already observed:

- Existing `StartFunctionalAPIServer` cells register process/server cleanup;
  direct real-listener cells use `support.CleanupProcess` or command stop paths.
- The valid negotiation case now proves explicit session termination, deletion,
  and typed absence. Rejected negotiation requests prove no live-session-set
  mutation.
- Terminal success/failure cases prove correlated public Factory Event outcome,
  Work state, and ordered typed content.

Not yet proved by this staged migration and intentionally left for later
stories:

- local real-listener and process lifecycle cleanup ledgers, including the
  bind-failure process path;
- package-wide three-repeat isolation, race execution, final post-migration
  topology, and PR CI package timing;
- clean-room VAL-001 assembly.

The smallest next step is Story 004: retain and directly prove the isolated
local listener/process lifecycle witnesses, then run the repeat, race, CI, and
clean-room validation gates.
