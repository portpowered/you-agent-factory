# Repository Agent Instructions

This repository is `you-agent-factory`: a Go, OpenAPI, and React system for
scheduling and orchestrating many AI workers concurrently through the `you` CLI,
backend runtime, and dashboard.

The product model is a factory: users define work types, work states,
workstations, workers, guards, resources, and orchestration policy. The runtime
turns submitted work and worker output into ordered events, then derives live
and historical world state from that event stream.

## Current Architecture

- The backend core is an event-first factory runtime. Workers and agents do not
  mutate canonical state directly; they emit outputs that re-enter the runtime
  as events.
- `FactoryService` coordinates APIs, CLI calls, session registries, persistence,
  runtime construction, and model/runtime dependencies. Per-session runtime
  state belongs to the session runtime, not the service coordinator.
- Public terminology is defined in
  `docs/architecture/data-model.md`: `Factory`, `Factory Session`, `Current
  Factory`, `Work`, `Work Request`, and `Provider Session`.
- The customer-facing model hides the internal Petri-net implementation.
  Internal packages can use tokens, places, transitions, markings, and guards;
  public docs and API surfaces should prefer customer-facing vocabulary.
- Factory events and projections are canonical for replay, dashboard state,
  session lifecycle, dispatch lifecycle, work payload lineage, and historical
  inspection.
- The frontend consumes generated OpenAPI types and event streams, derives
  client-side dashboard/editor projections, and sends user actions back through
  API requests. The backend remains authoritative for runtime state.
- OpenAPI is authored in component fragments under `api/components/` and bundled
  into `api/openapi.yaml`; generated Go and TypeScript clients are derived from
  that contract.

Read these architecture notes when the work touches their area:

- `docs/architecture/architecture.md` for backend loop, service/session
  boundaries, event stream, frontend composition, and graph-editor state flow.
- `docs/architecture/structures.md` to understand the overall interaction of components within the system.
- `docs/architecture/data-model.md` for public resource vocabulary and the
  customer/internal data-model split.

## Standards

Start with `docs/internal/standards/STANDARDS.md`, then read the relevant
standard before changing code:

- `docs/internal/standards/code/code-review-standards.md` for reviews and PR
  quality gates.
- `docs/internal/standards/code/general-backend-standards.md` for Go backend,
  state management, architecture, linting, testing, CI, and complexity.
- `docs/internal/standards/code/general-website-standards.md` for React,
  accessibility, responsive design, styling, state, performance, and tests.
- `docs/internal/standards/code/planning-standards.md` for PRDs, work stories,
  acceptance criteria, and implementation plans.

Treat files under `docs/internal/standards/` as normative. Development notes and
process maps can provide examples and file inventories, but they do not override
the standards.

## Repository Map

- `api/` contains the authored OpenAPI entrypoint, component fragments,
  reusable parameters/responses/schemas, and generated bundled contract.
- `cmd/` contains Go command entrypoints and maintenance tools, including the
  main `you` binary under `cmd/factory/`.
- `docs/` contains public docs, architecture notes, comparison docs, examples,
  and internal maintainer standards/process docs.
- `docs/reference/` is the canonical packaged `you docs <topic>` markdown.
  Edit reference topics there, not in generated or embedded copies.
- `examples/` contains example factory directories.
- `factory/` contains this repository's checked-in factory scaffold and
  factory-local docs.
- `pkg/transports/` for various API entrypoints such as CLI, MCP, REST
- `pkg/config/` contains factory config loading, persistence, mapping,
  validation entrypoints, built-in factory layout, and runtime config
  projections.
- `pkg/factory/` contains the core runtime engine, event history, projections,
  requests, validation, scheduling, subsystems, runtime support, and workstation
  config plumbing.
- `pkg/factory/sessions/` contains live and durable session state, projections,
  lifecycle gateways, response streams, execution contracts, and live
  invocation orchestration.
- `pkg/interfaces/` contains shared domain interfaces and public-ish runtime
  types used across subsystems.
- `pkg/work/` contains canonical Work content, materialization, query, graph,
  time-work, and pure invocation input/return-policy behavior.
- `pkg/models/local/` contains managed model runtime catalog, readiness,
  lifecycle, source resolution, cache, pull, and invocation support.
- `pkg/orchestrators/petri/` contains internal Petri-net primitives.
- `pkg/service/` coordinates backend service behavior across sessions,
  runtime construction, model catalog, replay, ingestion, and factory save or
  validation flows.
- `pkg/workers/` contains worker execution, inference binding/output shaping,
  provider integration, invocation-time worker capability policy, mock workers,
  worktrees, and hosted workers; `pkg/factory/packages/` contains packaged
  factory support.
- `pkg/orchestrators/javascript/preview`, `pkg/orchestrators/javascript/policy`,
  `pkg/orchestrators/javascript/result`, `pkg/orchestrators/javascript/source`,
  and `pkg/orchestrators/javascript/validation` contain JavaScript workflow
  preview, policy, result, source lookup, and validation logic.
- `tests/` contains broader functional, release, smoke, and integration tests.
- `ui/` contains the React dashboard, generated TypeScript OpenAPI types,
  feature modules, shared components, theme/styles, Storybook, and frontend
  tests.

## API And Code Generation

- Author OpenAPI changes in `api/openapi-main.yaml` and component fragments
  under `api/components/`.
- Do not hand-edit generated files:
  - `api/openapi.yaml`
  - `pkg/transports/http/generated/server.gen.go`
  - `pkg/transports/http/client/client.gen.go`
  - `ui/src/api/generated/openapi.ts`
- Run `make generate-api` after OpenAPI changes.
- For API surface changes, update the matching `pkg/transports/http` handlers,
  `pkg/transports/mapping` mappers/normalizers, generated clients, UI API adapters, and
  contract tests as applicable.
- Run `make api-smoke` for public REST contract changes when feasible.

## Documentation

- Public packaged CLI docs live in `docs/reference/` and are embedded for
  `you docs <topic>`.
- Run `make docs-reference-smoke` after changing `docs/reference/` or packaged
  docs behavior.
- Run `make readme-check` after changing README structure or linked README
  assets.
- Keep root docs as routing/index material. Avoid duplicating long customer
  guides or feature inventories outside their canonical docs.

## Verification

Choose the narrowest useful verification for the change, then broaden when the
surface area is shared or public:

- `make test` runs the short Go suite.
- `make ui-test` runs the frontend unit suite.
- `make ui-lint` runs frontend lint and UI boundary checks.
- `make lint` runs the repository lint suite.
- `make verify-fast` runs dashboard typecheck, short UI/unit tests, and short
  Go tests.
- `make verify-pr` runs the broader PR verification tier.
- `make test-full`, `make test-functional`, `make test-ui-coverage`, and
  specialty Make targets are available for higher-risk runtime, API, or UI
  changes.

When changing frontend behavior, run focused UI tests where possible and inspect
the UI in a browser for layout-sensitive work. When changing event replay,
factory sessions, dispatch lifecycle, or projections, add or update projection
and replay tests near the affected package.

## Working Rules

- Preserve user work in the tree. Do not revert unrelated changes.
- Prefer existing package boundaries and local helpers over new abstractions.
- Keep generated files in sync with their source contracts.
- Keep public resource names aligned with `docs/architecture/data-model.md`.
- Keep CLI and API behavior equivalent when shared contracts say they are
  equivalent, especially invocation behavior.
- Use `rg`/`rg --files` for code discovery.
- Add tests proportional to the risk and blast radius of the change.
