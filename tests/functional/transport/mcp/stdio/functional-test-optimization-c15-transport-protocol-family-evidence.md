# Transport protocol-family characterization: MCP stdio

This ledger is the package-local evidence for
`functional-test-optimization-c15-transport-protocol-family-001`.
It freezes MCP-S-01..10 before fixture-topology changes. It is a
characterization record, not post-change parity or performance evidence.

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
