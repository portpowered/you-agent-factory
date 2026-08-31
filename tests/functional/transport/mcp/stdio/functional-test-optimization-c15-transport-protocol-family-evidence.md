# Transport protocol-family characterization: MCP stdio

This ledger is the package-local evidence for
`functional-test-optimization-c15-transport-protocol-family-001`.
It freezes MCP-S-01..10 before fixture-topology changes. The opening section
is the characterization record; the review follow-up below records
post-change parity and performance evidence.

## Artifact and provenance

| Field | Value |
| --- | --- |
| Package | `github.com/portpowered/infinite-you/tests/functional/transport/mcp/stdio` |
| Package ID | `F036` |
| Current HEAD | `42eeee4472656b8290f798c36a5b8c871b24d7d0` |
| Base / merge base | `origin/main` / `42eeee4472656b8290f798c36a5b8c871b24d7d0` |
| Current source SHA-256 | `fae6cc0d8496f6611eb04eec7cc7ec859fdbe2b231f379f1f394ee04a6472624` for `discovery_test.go` |
| c01 source SHA-256 | `bcd6ea42cbfce14cbfcf0df757206e068cd5a12246ec56288fbf60038f0be8f3` |
| c01 classification source | `docs/internal/development/functional-test-optimization/c01-eligibility-inventory.json`, recorded commit `eef5150f277490384b460a47a0a3bcca51338e67` |
| c01 classification | S-01..S-07: `isolated-with-reason`, `propertyType=stdio`, `requiredFidelity=local_real`, OS audit `INTENTIONAL-OS`, required property `stream-file-descriptor-behavior`, `PSO-0069`..`PSO-0075`; S-08..S-10: subparts of the process-free `TestMCPStdioOpenRejectsUncomposedServerAndStreams` source row, `shareable`, no process observation |
| Go / host | `go1.25.0 windows/amd64` |
| Discovery | `go test -list '^Test' -count=0 -timeout=5m ./tests/functional/transport/mcp/stdio` exited 0; 7 top-level declarations and 2 named initializer rows (`fixture-backed`, `runtime-backed`) |

The source hash differs from the c01 record because the current file includes
the post-inventory cleanup changes from commits `db21923302` and `c2a01c8e06`.
The current identities, line-level witnesses, classification, and assertions
remain present. The c01 inventory is excluded from this lane; its owner may
refresh the source hash in a separate delta.

## Census and classification rules

The ten matrix rows reconcile to seven top-level tests, two named initializer
subtests, and three process-free constructor validation subparts. No row is
deleted, renamed, quarantined, or skipped in the normal run; the focused
selector was not run in short mode. The process-free S-08..S-10 subparts are
not root-backed and must not be counted as real-server lifecycle evidence.

The supplied admission median for this package is **15.678s**. It is an
operator-supplied pre-optimization observation from the PRD and is not a
current comparable median. The one permitted current-head diagnostic was:

```text
go test -v -count=1 -timeout=5m ./tests/functional/transport/mcp/stdio -run '^TestMCPStdioInitializeAndToolDiscovery$'
exit 0
package-reported: 5.392s (test body 5.31s)
measured command wall: 11.165s
TestMain topology: top_level_tests=7 named_initializer_rows=2 eligible_process_free_rows=1 isolated_rows=8
GATE-STDIO-ISOLATED: root_builds=1 root_closes=1; root-backed rows retain distinct Process instances; process_free_constructor=not_acquired
GATE-LIFECYCLE: invocations=1/1 sessions=1/1 contexts=1/1 streams=1/1 temporary_roots=1/1; shutdown observes cancellation and stdout EOF; pre-initialize environment failures remain session-free
GATE-CLEANUP: roots=1/1; invocations=1/1; sessions=1/1; contexts=1/1; streams=1/1; temporary_roots=1/1; child_processes=0 ports=0 routes=0 (not acquired)
```

## Witness matrix

| Row | Current identity and witness | Observable assertions | Resources / fidelity boundary | c01 record and proposed owner |
| --- | --- | --- | --- | --- |
| MCP-S-01 | `TestMCPStdioInitializeAndToolDiscovery` (`discovery_test.go:73`) | Initialize succeeds with protocol version `2024-11-05`; `tools/list` succeeds; result contains a nonempty tools array. | One composed root, real stdio reader/writer, MCP server/session, context, temporary project root, and cleanup. This is a local-real stdio handshake and discovery edge. | `C07-F036-top-level-test-TestMCPStdioInitializeAndToolDiscovery`; `isolated-with-reason/stdio`; `PSO-0069`; `INTENTIONAL-OS/stream-file-descriptor-behavior`. `MCP-STDIO-ISOLATED`: retain an isolated real stdio/root cohort. |
| MCP-S-02 | `TestMCPUnknownToolReturnsProtocolError` (`discovery_test.go:108`) | Initialized server returns a non-nil error and nil result for an unknown tool; code is `-32602`; message contains `unknown tool`. | Fresh composed root, real stdio protocol path, initialize handshake, unknown tools/call, and teardown. Protocol error must remain externally observable. | `C07-F036-top-level-test-TestMCPUnknownToolReturnsProtocolError`; `isolated-with-reason/stdio`; `PSO-0070`; `INTENTIONAL-OS/stream-file-descriptor-behavior`. `MCP-STDIO-ISOLATED`: share only case-independent graph construction; isolate session/streams and request state. |
| MCP-S-03 | `TestMCPDiscoveryContainsCanonicalFactorySessionTools` (`discovery_test.go:138`) | `tools/list` succeeds and every tool name in `mcpgenerated.PrimaryDiscovery()` is present in the response. | Fresh root/session and real stdio discovery stream; generated canonical discovery is the expected public contract. | `C07-F036-top-level-test-TestMCPDiscoveryContainsCanonicalFactorySessionTools`; `isolated-with-reason/stdio`; `PSO-0071`; `INTENTIONAL-OS/stream-file-descriptor-behavior`. `MCP-STDIO-ISOLATED`: graph may be shared only if generated discovery and request state remain independent. |
| MCP-S-04 | `TestMCPStdioRuntimeRejectsMissingHomeEnvironment` (`discovery_test.go:162`) | PATH-only environment with no HOME/USERPROFILE returns `home directory is not defined in the supplied environment`; no stdio Session exists before the failure. | Runtime-backed initializer with supplied environment, no home directory, and failure before protocol/session acquisition. Genuine boundary: initialization validation and session-free rejection. | `C07-F036-top-level-test-TestMCPStdioRuntimeRejectsMissingHomeEnvironment`; `isolated-with-reason/stdio`; `PSO-0072`; `INTENTIONAL-OS/stream-file-descriptor-behavior`. `MCP-STDIO-INIT-ERRORS`: share immutable construction only; isolate environment and failure lifecycle. |
| MCP-S-05 | `TestMCPStdioRuntimeRejectsInvalidRuntimeProjectRoot` (`discovery_test.go:182`) | Supplied invalid project root returns `factory layout not found`; no successful protocol session is exposed. | Runtime-backed initializer with invalid project-root path, supplied environment, and pre-session rejection. | `C07-F036-top-level-test-TestMCPStdioRuntimeRejectsInvalidRuntimeProjectRoot`; `isolated-with-reason/stdio`; `PSO-0073`; `INTENTIONAL-OS/stream-file-descriptor-behavior`. `MCP-STDIO-INIT-ERRORS`: keep invalid path and initializer outcome case-local. |
| MCP-S-06 | `TestMCPStdioFixtureAndRuntimePathsReachInitializer/fixture-backed` (`discovery_test.go:206`) | Fixture-backed path reaches the initializer; an incomplete MCP frame terminates with an error and stdout reaches EOF without a success response. | Fixture-backed initializer, real stdio pipes, incomplete frame, root/session/stream cleanup. This preserves the fixture path's malformed/incomplete-frame boundary. | `C07-F036-named-scenario-TestMCPStdioFixtureAndRuntimePathsReachInitializer-fixture-backed`; `isolated-with-reason/stdio`; `PSO-0074`; `INTENTIONAL-OS/stream-file-descriptor-behavior`. `MCP-STDIO-ISOLATED`: share immutable fixture setup only; isolate pipes and protocol lifecycle. |
| MCP-S-07 | `TestMCPStdioFixtureAndRuntimePathsReachInitializer/runtime-backed` (`discovery_test.go:214`) | Scaffolded runtime-backed Factory reaches the initializer; initialize succeeds with exact protocol version `2024-11-05`; cleanup completes. | Runtime-backed Factory layout, fresh environment/cwd, composed root, real stdio session, and teardown. | `C07-F036-named-scenario-TestMCPStdioFixtureAndRuntimePathsReachInitializer-runtime-backed`; `isolated-with-reason/stdio`; `PSO-0075`; `INTENTIONAL-OS/stream-file-descriptor-behavior`. `MCP-STDIO-ISOLATED`: isolated root/session/streams; case-owned runtime project. |
| MCP-S-08 | `TestMCPStdioOpenRejectsUncomposedServerAndStreams` (`discovery_test.go:231`, `server=nil`) | `mcpstdio.Open(nil, reader, buffer)` returns an error whose message identifies the missing server; no process/session is acquired. | Process-free constructor validation with nil composed server and otherwise supplied streams. No root, child process, port, or route is acquired. | `C07-F036-top-level-test-TestMCPStdioOpenRejectsUncomposedServerAndStreams`; `shareable` process-free source row; no PSO. `MCP-STDIO-PROCESS-FREE`: shareable constructor validation; do not convert it into a real-server witness. |
| MCP-S-09 | `TestMCPStdioOpenRejectsUncomposedServerAndStreams` (`discovery_test.go:231`, `reader=nil`) | Nil input stream returns an error whose message identifies missing streams; no process/session is acquired. | Process-free constructor validation; no server/root/session/child process/port/route. | `C07-F036-top-level-test-TestMCPStdioOpenRejectsUncomposedServerAndStreams`; `shareable` process-free source row; no PSO. `MCP-STDIO-PROCESS-FREE`: shareable validation only. |
| MCP-S-10 | `TestMCPStdioOpenRejectsUncomposedServerAndStreams` (`discovery_test.go:231`, `writer=nil`) | Nil output stream returns an error whose message identifies missing streams; no process/session is acquired. | Process-free constructor validation; no server/root/session/child process/port/route. | `C07-F036-top-level-test-TestMCPStdioOpenRejectsUncomposedServerAndStreams`; `shareable` process-free source row; no PSO. `MCP-STDIO-PROCESS-FREE`: shareable validation only. |

## Evidence boundary and handoff

The list-only run and source review prove that all ten MCP-S rows have current
identities, exact protocol/error/lifecycle assertions, resource owners, c01
classification, and proposed owners. The focused selector proves one
local-real initialize/discovery path, exact protocol negotiation, one
root/session/stream lifecycle, and the package's emitted topology/cleanup
counters. It does **not** prove optimized parity for all rows, malformed-frame
and environment-rejection behavior after restructuring, repeat/race behavior,
package timing under PR-CI, coverage, Unix semantics, terminal CI, or merge.
Those edges belong to GATE-MCP-STDIO, GATE-RACE-MCP-STDIO, GATE-PERF,
GATE-COVERAGE, GATE-LOOP, and GATE-PR.

The source-plan artifact named by the PRD is ignored and absent in this
worktree. The PRD/task packet supplies the matrix used here. No source-plan,
shared support, or c01 canonical inventory file was edited; the source-hash
refresh is a narrow delta request to the inventory owner.

## Story 004 optimization evidence

The MCP-S fixture now builds one package-scoped application root lazily and
serializes `Process.Execute` calls. All seven root-backed rows retain fresh
stdio pipe pairs, contexts, environments, HOME/USERPROFILE directories,
working or project roots, request IDs, and protocol state. S-08..S-10 remain
process-free constructor checks. The root-backed topology is therefore one
shared root with no isolated root; the real stdio, incomplete-frame, EOF, and
pre-initialize failure boundaries remain case-local.

Post-change source hashes:

| File | SHA-256 |
| --- | --- |
| `discovery_test.go` | `1ae0cf6775b10adc30dde5ee42fabd5324e8559fd7ca86dc18de9204bcb85179` |
| `package_fixture_test.go` | `29bee5673a03b2248dc05cccc357ed91ba8f8ec066c68b7629a075d51f6e5a38` |

List discovery ran with `go test -list '^Test' -count=0 -timeout=5m
./tests/functional/transport/mcp/stdio` and exited 0, listing seven
top-level tests, two named initializer rows, and the three process-free
constructor subparts. The focused selectors for initialize/discovery,
unknown-tool error, canonical discovery, missing-home failure, invalid-root
failure, both initializer rows, and Open validation each exited 0 with
unchanged exact assertions. Representative focused output included:

```text
go test -v -count=1 -timeout=5m ./tests/functional/transport/mcp/stdio -run '^TestMCPStdioInitializeAndToolDiscovery$'
exit 0; package 1.378s; roots=1/1; invocations=1/1; sessions=1/1; contexts=1/1; streams=1/1; temporary_roots=2/2

go test -v -count=1 -timeout=5m ./tests/functional/transport/mcp/stdio -run '^TestMCPStdioFixtureAndRuntimePathsReachInitializer$'
exit 0; package 11.518s; roots=1/1; invocations=2/2; sessions=2/2; contexts=2/2; streams=2/2; temporary_roots=4/4
```

The useful repeat procedures passed:

| Procedure | Observed topology and result |
| --- | --- |
| `go test -v -count=3 -timeout=10m ./tests/functional/transport/mcp/stdio -run '^TestMCPStdioFixtureAndRuntimePathsReachInitializer$'` | exit 0; one root built/closed; six invocations, sessions, contexts, and streams balanced; twelve temporary roots made/removed; package 25.819s. |
| `go test -v -count=3 -timeout=10m ./tests/functional/transport/mcp/stdio -run '^TestMCPStdioRuntimeRejects(MissingHomeEnvironment|InvalidRuntimeProjectRoot)$'` | exit 0; one root built/closed; six invocations balanced with zero stdio sessions/streams; twelve temporary roots made/removed; package 12.851s. |
| `go test -race -v -count=1 -timeout=15m ./tests/functional/transport/mcp/stdio -run '^TestMCPStdioFixtureAndRuntimePathsReachInitializer$'` | exit 0; no race report; one root built/closed; two invocations, sessions, contexts, and streams balanced; four temporary roots made/removed; package 68.254s under the race detector. |

The repeat and race runs prove fresh-case isolation and cleanup for the
changed shared fixture, not portable package timing, functional coverage,
built-child behavior, Unix descriptor semantics, or the single final package
run. The comparable CI timing and irreducibility evidence is recorded below;
functional coverage, terminal CI, and merge remain review-owned gates.

## Review follow-up: comparable timing and irreducibility

### Comparable package timing

The following is the comparable package-level denominator requested in review.
Both sides use the `seconds` package field from the same CI workflow's
`functional-timing-summary.json` on Linux X64; local Windows wall time is not
used for this comparison.

| Package | Before CI package median / sample | After CI package median / sample | Delta | Disposition |
| --- | ---: | ---: | ---: | --- |
| `mcp/stdio` | 15.678s | 9.292s | -6.386s (-40.7%) | Direct `<3s` target blocked; irreducibility alternative complete |

Before provenance is the base CI run [33337769566](https://github.com/portpowered/you-agent-factory/actions/runs/33337769566), head `42eeee4472656b8290f798c36a5b8c871b24d7d0`, Backend Functional Coverage job [99327753325](https://github.com/portpowered/you-agent-factory/actions/runs/33337769566/job/99327753325), and its [functional-test-diagnostics artifact](https://github.com/portpowered/you-agent-factory/actions/runs/33337769566/artifacts/9739638631). The `15.678s` value is the supplied admission median and is now tied to the same package timing field and CI denominator.

After provenance is the matching PR CI run [33348730534](https://github.com/portpowered/you-agent-factory/actions/runs/33348730534), head `c2c429a62dcb0e86eaca8ba87eb712ce2756387a`, Backend Functional Coverage job [99357847982](https://github.com/portpowered/you-agent-factory/actions/runs/33348730534/job/99357847982), and its [functional-test-diagnostics artifact](https://github.com/portpowered/you-agent-factory/actions/runs/33348730534/artifacts/9742979739). The artifact's `functional-tests.md` supplies the post-change top-level test timings below.

### Complete measured irreducibility table

The CI artifact reports top-level test elapsed cost. The two named child rows
below use focused `go test -json -count=1 -timeout=20m` measurements at the
final head because CI's report aggregates them into their parent test. The
focused commands exited `0`; the parent CI elapsed value is shown in
parentheses as the corresponding top-level report value, not as a sum across
local child runs. The three Open cases are ordinary
assertions in one top-level function, not named Go subtests, so their shared
`0.000s` top-level measurement is recorded without inventing child timings.
Every row retains the exact boundary listed in the witness map, names the owner
of the irreducible cost, and records the direct-target disposition.

| Case | Measured elapsed cost | Retained real boundary | Irreducibility reason / owner | Resulting disposition |
| --- | ---: | --- | --- | --- |
| MCP-S-01 | 1.180s PR-CI top-level | Real MCP stdio initialize and tools/list stream | Protocol handshake and discovery are local-real stream behavior; `MCP-STDIO-SHARED` | **BLOCKED** direct `<3s`; alternative complete |
| MCP-S-02 | 0.780s PR-CI top-level | Real stdio unknown-tool request and JSON-RPC error | Error envelope must cross the protocol stream; `MCP-STDIO-SHARED` | **BLOCKED** direct `<3s`; alternative complete |
| MCP-S-03 | 0.860s PR-CI top-level | Generated canonical discovery over a fresh stdio session | Canonical tool membership remains request-local; `MCP-STDIO-SHARED` | **BLOCKED** direct `<3s`; alternative complete |
| MCP-S-04 | 0.050s PR-CI top-level | Missing HOME/USERPROFILE rejection before Session acquisition | Environment validation and session-free failure cannot share case state; `MCP-STDIO-INIT-ERRORS` | **BLOCKED** direct `<3s`; alternative complete |
| MCP-S-05 | 0.980s PR-CI top-level | Invalid runtime project root rejection before successful initialize | Invalid layout path and initializer outcome remain case-local; `MCP-STDIO-INIT-ERRORS` | **BLOCKED** direct `<3s`; alternative complete |
| MCP-S-06 | 1.060s focused child (parent CI 2.090s) | Incomplete frame, stdout EOF, and fixture-backed initializer | Malformed-frame and EOF ownership require fresh pipes; `MCP-STDIO-SHARED` | **BLOCKED** direct `<3s`; alternative complete |
| MCP-S-07 | 0.890s focused child (parent CI 2.090s) | Runtime-backed Factory initializer and exact protocol version | Runtime project and successful Session lifecycle remain fresh; `MCP-STDIO-SHARED` | **BLOCKED** direct `<3s`; alternative complete |
| MCP-S-08 | 0.000s PR-CI top-level invocation | Process-free `Open` validation for nil server | No root or stream is acquired by design; `MCP-STDIO-PROCESS-FREE` | **BLOCKED** direct `<3s`; alternative complete |
| MCP-S-09 | 0.000s PR-CI top-level invocation | Process-free `Open` validation for nil reader | No root or stream is acquired by design; `MCP-STDIO-PROCESS-FREE` | **BLOCKED** direct `<3s`; alternative complete |
| MCP-S-10 | 0.000s PR-CI top-level invocation | Process-free `Open` validation for nil writer | No root or stream is acquired by design; `MCP-STDIO-PROCESS-FREE` | **BLOCKED** direct `<3s`; alternative complete |

The focused child timing procedure was:

```text
go test -json -count=1 -timeout=20m ./tests/functional/transport/mcp/stdio -run '^TestMCPStdioFixtureAndRuntimePathsReachInitializer$'
exit 0; fixture-backed=1.060s; runtime-backed=0.890s; parent=1.940s

go test -json -count=1 -timeout=20m ./tests/functional/transport/mcp/stdio -run '^TestMCPStdioOpenRejectsUncomposedServerAndStreams$'
exit 0; one top-level function, no named subtests, 0.000s
```

### GATE-PERF disposition

The post-change package sample remains above three seconds, so the direct
under-three-second branch is recorded as **BLOCKED**. The permitted alternative
is complete: all ten rows have measured cost, every retained real boundary (or
intentional process-free boundary) has an explicit irreducibility reason and
owner, and the bounded root-reuse pass was already completed before this
evidence follow-up. No residual row is silently treated as an optimization
opportunity or omitted from the matrix.
