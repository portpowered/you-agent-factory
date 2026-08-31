# Transport protocol-family characterization: MCP protocol

This ledger is the package-local evidence for
`functional-test-optimization-c15-transport-protocol-family-001`.
It freezes MCP-P-01..03 before fixture-topology changes. The opening section
is the characterization record; the review follow-up below records
post-change parity and performance evidence.

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
shutdown boundary. The comparable CI timing and irreducibility evidence is
recorded below; functional coverage, terminal CI, and merge remain
review-owned gates.

## Review follow-up: comparable timing and irreducibility

### Comparable package timing

The following is the comparable package-level denominator requested in review.
Both sides use the `seconds` package field from the same CI workflow's
`functional-timing-summary.json` on Linux X64; local Windows wall time is not
used for this comparison.

| Package | Before CI package median / sample | After CI package median / sample | Delta | Disposition |
| --- | ---: | ---: | ---: | --- |
| `mcp/protocol` | 6.598s | 5.961s | -0.637s (-9.7%) | Direct `<3s` target blocked; irreducibility alternative complete |

Before provenance is the base CI run [33337769566](https://github.com/portpowered/you-agent-factory/actions/runs/33337769566), head `42eeee4472656b8290f798c36a5b8c871b24d7d0`, Backend Functional Coverage job [99327753325](https://github.com/portpowered/you-agent-factory/actions/runs/33337769566/job/99327753325), and its [functional-test-diagnostics artifact](https://github.com/portpowered/you-agent-factory/actions/runs/33337769566/artifacts/9739638631). The `6.598s` value is the supplied admission median and is now tied to the same package timing field and CI denominator.

After provenance is the matching PR CI run [33348730534](https://github.com/portpowered/you-agent-factory/actions/runs/33348730534), head `c2c429a62dcb0e86eaca8ba87eb712ce2756387a`, Backend Functional Coverage job [99357847982](https://github.com/portpowered/you-agent-factory/actions/runs/33348730534/job/99357847982), and its [functional-test-diagnostics artifact](https://github.com/portpowered/you-agent-factory/actions/runs/33348730534/artifacts/9742979739). The artifact's `functional-tests.md` supplies the post-change top-level test timings below.

### Complete measured irreducibility table

All three MCP protocol rows are top-level tests in the CI report, so no
aggregation allocation is needed. Every row retains the exact boundary listed
in the witness map, names the owner of the irreducible cost, and records the
direct-target disposition.

| Case | Measured elapsed cost | Retained real boundary | Irreducibility reason / owner | Resulting disposition |
| --- | ---: | --- | --- | --- |
| MCP-P-01 | 1.410s PR-CI top-level | Real protocol initialize plus malformed tools/call frame and `-32602` response | Invalid-params envelope and response-ID preservation cross the protocol stream; `MCP-PROTOCOL-SHARED` | **BLOCKED** direct `<3s`; alternative complete |
| MCP-P-02 | 0.940s PR-CI top-level | Real protocol request/response for a missing Factory Session | Canonical not-found result and response decoding remain request-local; `MCP-PROTOCOL-SHARED` | **BLOCKED** direct `<3s`; alternative complete |
| MCP-P-03 | 1.320s PR-CI top-level | Isolated cancellation, serve return, stream closure, and stdout EOF | Descriptor/EOF shutdown is the genuine whole-protocol boundary; `MCP-PROTOCOL-SHUTDOWN` | **BLOCKED** direct `<3s`; alternative complete |

### GATE-PERF disposition

The post-change package sample remains above three seconds, so the direct
under-three-second branch is recorded as **BLOCKED**. The permitted alternative
is complete: all three rows have measured cost, every retained real boundary
has an explicit irreducibility reason and owner, and the bounded root-reuse
pass was already completed before this evidence follow-up. No residual row is
silently treated as an optimization opportunity or omitted from the matrix.
