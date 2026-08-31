# Transport protocol-family characterization: MCP protocol

This ledger is the package-local evidence for
`functional-test-optimization-c15-transport-protocol-family-001`.
It freezes MCP-P-01..03 before fixture-topology changes. It is a
characterization record, not post-change parity or performance evidence.

## Artifact and provenance

| Field | Value |
| --- | --- |
| Package | `github.com/portpowered/infinite-you/tests/functional/transport/mcp/protocol` |
| Package ID | `F035` |
| Current HEAD | `42eeee4472656b8290f798c36a5b8c871b24d7d0` |
| Base / merge base | `origin/main` / `42eeee4472656b8290f798c36a5b8c871b24d7d0` |
| Current source SHA-256 | `f9bbf4a1782df9d235ac517f2a33c6e544eaece7a3e74de0d2bf71bfc41c1e3a` for `errors_test.go` |
| c01 source SHA-256 | `70a0e81c3021887726cc59f449f4b29b554024422ab3367c1ed78529fadc7dab` |
| c01 classification source | `docs/internal/development/functional-test-optimization/c01-eligibility-inventory.json`, recorded commit `eef5150f277490384b460a47a0a3bcca51338e67` |
| c01 classification | P-01 and P-02: `shareable-with-mock`, process-free protocol/error assertions; P-03: `isolated-with-reason`, `propertyType=stdio`, `requiredFidelity=local_real`, OS audit `INTENTIONAL-OS`, required property `stream-file-descriptor-behavior`, `PSO-0068` |
| Go / host | `go1.25.0 windows/amd64` |
| Discovery | `go test -list '^Test' -count=0 -timeout=5m ./tests/functional/transport/mcp/protocol` exited 0; 3 top-level declarations |

The source hash differs from the c01 record because the current file includes
the post-inventory protocol-root changes from commits `476b7f5068` and
`33f88d4efd`. The current identities, line-level witnesses, classification,
and assertions remain present. The c01 inventory is excluded from this lane;
its owner may refresh the source hash in a separate delta.

## Census and classification rules

The three matrix rows reconcile to three top-level tests. No row is deleted,
renamed, quarantined, or skipped in the normal run; the focused selector was
not run in short mode. P-01 and P-02 are shareable protocol/error witnesses;
P-03 retains the real stdio shutdown boundary and its isolated root/stream
ownership.

The supplied admission median for this package is **6.598s**. It is an
operator-supplied pre-optimization observation from the PRD and is not a
current comparable median. The one permitted current-head diagnostic was:

```text
go test -v -count=1 -timeout=5m ./tests/functional/transport/mcp/protocol -run '^TestMCPMalformedParametersReturnInvalidParams$'
exit 0
package-reported: 1.921s (test body 1.79s)
measured command wall: 7.584s
GATE-PROTOCOL topology: roots shared=1/1 isolated=0/0; invocations=1/1; contexts=1; streams=1/1; temporary_roots=1/1; child_processes=0 ports=0 routes=0 (not acquired)
```

## Witness matrix

| Row | Current identity and witness | Observable assertions | Resources / fidelity boundary | c01 record and proposed owner |
| --- | --- | --- | --- | --- |
| MCP-P-01 | `TestMCPMalformedParametersReturnInvalidParams` (`errors_test.go:108`) | Initialize handshake succeeds; malformed `tools/call` with empty parameters returns a response preserving request ID `1`, with a non-nil JSON-RPC error and code `-32602`. | One protocol-root cohort, real stdio reader/writer, initialize and malformed request frames, context, temporary root, and cleanup. The public invalid-params envelope is the owned edge. | `C07-F035-top-level-test-TestMCPMalformedParametersReturnInvalidParams`; `shareable-with-mock`; no PSO. `MCP-PROTOCOL-SHARED`: share process-free/root-independent protocol setup with P-02 while keeping request frames and response assertions case-local. |
| MCP-P-02 | `TestMCPMissingFactorySessionReturnsCanonicalNotFound` (`errors_test.go:125`) | Initialize succeeds; missing-session `tools/call` preserves the response ID; no JSON-RPC error is returned; result is non-nil with `isError=false` and exactly one text item; embedded error code is `factory_session.session.not_found`, session ID is `dur-sess-missing-999`, and `retryable=false`. | One protocol root/stdio stream cohort may be shared with P-01; missing-session request/response and exact canonical error payload remain case-local. | `C07-F035-top-level-test-TestMCPMissingFactorySessionReturnsCanonicalNotFound`; `shareable-with-mock`; no PSO. `MCP-PROTOCOL-SHARED`: shared graph candidate with isolated request state and response decoding. |
| MCP-P-03 | `TestMCPServerShutdownClosesStdioCleanly` (`errors_test.go:168`) | Initialize/connection shutdown returns cleanly; stdout reaches EOF without a hang; cancellation/close is observed; the isolated root, streams, temporary root, and invocation all balance. | One isolated real stdio root, context cancellation, server/stream shutdown, temporary project root, and cleanup. Genuine boundary: descriptor/EOF behavior and lifecycle closure. | `C07-F035-top-level-test-TestMCPServerShutdownClosesStdioCleanly`; `isolated-with-reason/stdio`; `PSO-0068`; `INTENTIONAL-OS/stream-file-descriptor-behavior`. `MCP-PROTOCOL-SHUTDOWN`: retain a separate root/streams/close cohort; sharing would blur EOF and shutdown ownership. |

## Evidence boundary and handoff

The list-only run and source review prove that all three MCP-P rows have
current identities, exact error/lifecycle assertions, resource owners, c01
classification, and proposed owners. The focused selector proves one
protocol-local malformed-parameter response, request-ID preservation, exact
`-32602` classification, and the package's emitted topology counters. It does
**not** prove optimized parity for the missing-session or shutdown rows,
repeat/race behavior, package timing under PR-CI, coverage, Unix semantics,
terminal CI, or merge. Those edges belong to GATE-MCP-PROTOCOL,
GATE-RACE-MCP-PROTOCOL, GATE-PERF, GATE-COVERAGE, GATE-LOOP, and GATE-PR.

The source-plan artifact named by the PRD is ignored and absent in this
worktree. The PRD/task packet supplies the matrix used here. No source-plan,
shared support, or c01 canonical inventory file was edited; the source-hash
refresh is a narrow delta request to the inventory owner.

## Story 004 optimization evidence

P-01 and P-02 continue to use one serialized package-scoped application root
with fresh request frames, stdio streams, contexts, HOME/USERPROFILE
directories, working roots, and response decoding per invocation. P-03 retains
its separate root because cancellation, serve return, descriptor closure, and
stdout EOF are the genuine whole-protocol lifecycle witness. The protocol
topology is therefore one shared root plus one isolated shutdown root, with all
temporary roots and streams balanced.

The post-change `errors_test.go` SHA-256 is
`d1dfb7d46631cfe062adb8b980643244e5a662d100bc4d058a84a42f45b8de2b`.
List discovery ran with `go test -list '^Test' -count=0 -timeout=5m
./tests/functional/transport/mcp/protocol` and exited 0, listing all three
top-level tests. Focused P-01, P-02, and P-03 selectors each exited 0 and
preserved exact request IDs, invalid-params/error envelopes, missing-session
payloads, cancellation, clean serve return, and stdout EOF.

The useful repeat and race procedures passed:

| Procedure | Observed topology and result |
| --- | --- |
| `go test -v -count=3 -timeout=10m ./tests/functional/transport/mcp/protocol -run '^TestMCP(MalformedParametersReturnInvalidParams|MissingFactorySessionReturnsCanonicalNotFound)$'` | exit 0; one shared root built/closed; six invocations, streams, and temporary roots balanced; six home roots made/removed; package 25.353s. |
| `go test -v -count=3 -timeout=10m ./tests/functional/transport/mcp/protocol -run '^TestMCPServerShutdownClosesStdioCleanly$'` | exit 0; three isolated roots built/closed; three invocations, contexts, streams, working roots, and home roots balanced; package 12.822s. |
| `go test -race -v -count=1 -timeout=15m ./tests/functional/transport/mcp/protocol -run '^TestMCP(MalformedParametersReturnInvalidParams|MissingFactorySessionReturnsCanonicalNotFound)$'` | exit 0; no race report; one shared root built/closed; two invocations and streams balanced; two working and home roots removed; package 67.596s under the race detector. |

These procedures prove changed shared-root isolation and the retained local-real
shutdown boundary, not portable package timing, functional coverage,
built-child behavior, remote compatibility, or the final package run. Story
005 owns the final execution and GATE-PERF/GATE-COVERAGE/GATE-LOOP.
