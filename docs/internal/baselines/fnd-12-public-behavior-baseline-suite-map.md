# FND-12 public behavior baseline suite map

Maintainer map of **runnable** entry points that lock representative public
success and typed-failure behavior for CLI, HTTP, MCP, replay, and Factory
Visualization activation **before** Packaged Service Structure service-package
moves.

This packet captures and documents baselines only. It does **not** migrate
service packages, change provider conductor behavior, or own live
docs/models/mcp CLI-manifest generation.

## How to use

Before a PSS package move that depends on FND-12, run the captured suites:

1. `make fnd-12-behavior-baselines` — aggregator for all five surface pairs
   (CLI, HTTP, MCP, replay, visualization activation). Equivalent to running
   each per-surface `make fnd-12-*-behavior-baselines` target below.
2. `make verify-fast`
3. `make lint`

Per-surface Make targets and focused `go test` filters remain available when
debugging a single surface. No package-migration steps are required.

## Ownership note (out of lease)

Live **docs / models / mcp CLI-manifest** baselines and generation owned by
[PR #1262](https://github.com/portpowered/you-agent-factory/pull/1262)
(`make cli-manifest-generate`, `make cli-manifest-check`, authored manifest
projections) are **referenced only**. This packet must not refresh, regenerate,
or re-own those artifacts.

## Proof rules

Every listed baseline must assert an operator- or protocol-visible outcome
(exit/status, stdout/stderr or structured CLI result, HTTP status + body/error
code, MCP tool/protocol result, replay completion/divergence, visualization
view or activation failure).

Do **not** treat the following as sole proof for a surface:

- Source-file inventory or package import-graph scans
- Docs-link / docs-topology checks
- Command-tree, route-registration, or MCP discovery-artifact inventory dumps
  without a protocol-visible success or typed-failure assertion

## Surface map

| Surface | Runnable entry point | Success baseline | Typed-failure baseline | Pair status |
| --- | --- | --- | --- | --- |
| CLI | Captured Make: `make fnd-12-cli-behavior-baselines` (focused filter below). Equivalent: `go test ./pkg/transports/cli/baseline -run '^Test(RootHelpBaseline_MatchesFixture\|FailureBaseline_QuietInvalidTopologyWritesStructuredInvocationFailure)$$' -count=1`. Also included under `make test-functional` (short CLI baseline package). | `TestRootHelpBaseline_MatchesFixture` — customer-visible `you --help` stdout matches the checked-in fixture via `root.BuildProcess`. | `TestFailureBaseline_QuietInvalidTopologyWritesStructuredInvocationFailure` — invalid factory topology fails before invocation with structured `ErrorResponse` on stderr (code/family/message). | Covered (captured) |
| HTTP | Captured Make: `make fnd-12-http-behavior-baselines` (focused filter below). Equivalent: `go test ./tests/functional/runtime_api -run '^TestGeneratedAPIIntegrationSmoke_(OpenAPIGeneratedServerAndLiveRuntimeStayAligned\|SubmitWorkItemsRejectEmptyStructuredSubmission)$$' -count=1`. Broader Make wrap: `make api-smoke` (also regenerates/validates OpenAPI; heavier than the focused pair). | `TestGeneratedAPIIntegrationSmoke_OpenAPIGeneratedServerAndLiveRuntimeStayAligned` — live generated server serves a successful protocol-visible Work read/submit path. | `TestGeneratedAPIIntegrationSmoke_SubmitWorkItemsRejectEmptyStructuredSubmission` — empty structured submit returns HTTP 400. | Covered (captured) |
| MCP | Captured Make: `make fnd-12-mcp-behavior-baselines` (focused filter below). Equivalent: `go test ./pkg/transports/mcp/server -run '^Test(ServeStdioUsesSDKProtocolAndRegistersCatalog\|SDKProtocolErrors)$$' -count=1`. Broader Make wrap: `make mcp-contract-smoke` (also runs contract/discovery checks; discovery drift alone is **not** the behavioral proof). PR #1262 CLI-manifest baselines remain out of lease. | `TestServeStdioUsesSDKProtocolAndRegistersCatalog` — initialize/list/call succeed over stdio JSON-RPC with a catalog tool result. | `TestSDKProtocolErrors` — unknown tool / unsupported method return protocol-visible JSON-RPC errors. | Covered (captured) |
| Replay | Captured Make: `make fnd-12-replay-behavior-baselines` (focused filter below). Equivalent: `go test ./pkg/services/recordings/replay -run '^TestSideEffects_(InferReturnsRecordedProviderResponse\|UnmatchedRequestFailsClearly)$$' -count=1`. Optional broader (long tags): `go test -tags=<FUNCTIONAL_LONG_TAGS> ./tests/functional/replay_contracts -run 'TestReplayRegressionHarness_(LoadsArtifactAndAssertsSuccessfulReplay\|AssertsExpectedDivergence)' -count=1` | `TestSideEffects_InferReturnsRecordedProviderResponse` — matched recorded provider inference returns the recorded response. | `TestSideEffects_UnmatchedRequestFailsClearly` — unmatched replay key fails with a stable visible error (`replay provider request did not match`). | Covered (captured) |
| Visualization activation | Captured Make: `make fnd-12-visualization-behavior-baselines` (focused filter below). Equivalent: `go test ./pkg/services/factory_visualization -run '^Test(ServiceProjectsRetainedAndLiveFactoryEvents\|NewRejectsMissingDependencies)$$' -count=1` | `TestServiceProjectsRetainedAndLiveFactoryEvents` — `Start` against a valid event source projects retained-then-live events and emits observable `View`s. | `TestNewRejectsMissingDependencies` — activation construct fails with an explicit missing-dependency error (no false success). Related: `TestServiceReportsProjectionReadFailureWithoutStoppingSubscription` reports projection read failure without claiming success. | Covered (captured) |

## Gaps and follow-ups

As of this map, each surface has a **reusable success + typed-failure pair**
captured behind a dedicated Make target, plus the aggregator
`make fnd-12-behavior-baselines`. No sole-proof source scans were added.

- Story 002 (CLI): `make fnd-12-cli-behavior-baselines`
- Story 003 (HTTP): `make fnd-12-http-behavior-baselines`
- Story 004 (MCP): `make fnd-12-mcp-behavior-baselines`
- Story 005 (Replay): `make fnd-12-replay-behavior-baselines`
- Story 006 (Visualization activation): `make fnd-12-visualization-behavior-baselines`
- Story 007 (foundation gate): `make fnd-12-behavior-baselines` plus
  `make verify-fast` and `make lint` on the capturing branch — no package
  migration.
- Do not expand this map to own PR #1262 CLI-manifest generate/check artifacts.

## Non-goals (explicit)

- Migrating any `pkg/services/*` tree
- Changing provider conductor behavior
- Refreshing or re-owning PR #1262 docs/models/mcp CLI-manifest baselines
- Adding meta tests whose sole assertion is source, docs topology, or
  registration inventory
