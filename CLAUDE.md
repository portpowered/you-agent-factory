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
- Product behavior is split across Factory Definitions, Factory Runtime,
  Factory Sessions, Recordings, Work, Workers, Providers, Models, Automations,
  Provider Sessions, Operator Settings, Factory Visualization, and System
  Initialization services. `pkg/wire` composes these services and
  `pkg/initializer` owns application lifecycle.
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
- `docs/architecture/packaged-structure.md` for the current Go package families,
  service package convention, dependency direction, and composition boundaries.

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
- `packages/` contains publishable API, model-provider, and packaged-factory
  sources and generated distribution artifacts. Authored packaged factories
  live under `packages/packaged-factories/factories/`; generated package output
  lives under `packages/packaged-factories/generated/`.
- `pkg/` has six approved top-level package families: `initializer`, `platform`,
  `root`, `services`, `transports`, and `wire`.
- `pkg/root/` exposes the caller-facing `BuildProcess` construction boundary.
  `pkg/wire/` owns the canonical application dependency graph, and
  `pkg/initializer/` activates and unwinds already-constructed lifecycle roles.
- `pkg/transports/` owns protocol composition for CLI, HTTP, MCP, generated
  transport contracts and clients, and representation mapping. Service-specific
  adapters may live under the owning service's `transports/` directory.
- `pkg/platform/` contains policy-free cross-cutting implementations such as
  filesystem, process, logging, metrics, replay storage, clocks, PTY, and
  runtime-artifact support.
- `pkg/services/` contains the product-domain service families. Service roots
  expose public contracts and operations; private implementation belongs under
  the owning service's `internal/` tree, focused construction providers under
  `wire/`, and service-owned protocol adapters under `transports/`.
- `pkg/services/factory_definitions/` owns Factory definition loading,
  persistence, validation, compilation, authored layout, packaged distribution,
  catalog behavior, and invocation policy.
- `pkg/services/factory_runtime/` owns event-first orchestration, scheduling,
  dispatch, runtime projections, Petri execution, JavaScript workflows, and
  checkpoint recovery.
- `pkg/services/factory_sessions/` owns live and durable Factory Session state,
  runtime opening, lifecycle gateways, response streams, invocation, controls,
  and persisted execution behavior.
- `pkg/services/recordings/` owns the canonical Factory Event ledger, recording
  lifecycle, replay, artifacts, and read-model projections.
- `pkg/services/work/` owns Work and Work Request contracts, admission, content,
  materialization, staging, lineage, read behavior, and pure invocation return
  policy.
- `pkg/services/workers/` owns worker and workstation execution, runners,
  prompt/output shaping, worktrees, mock-worker behavior, and worker capability
  policy.
- `pkg/services/providers/` owns provider identity, catalog, lifecycle, ACP
  integration, and provider execution. Workers consume Providers through its
  public contracts; provider implementations do not belong to Workers.
- `pkg/services/models/` owns the managed local-model catalog, assets, runtime
  readiness and lifecycle, host supervision, source resolution, cache, pull,
  and local inference support.
- `pkg/services/automations/` owns cron, filesystem watcher, script poller,
  hosted-source, reconciliation, and invocation-schedule behavior.
- `pkg/services/provider_sessions/` owns provider-session discovery and
  transcript/session inspection.
- `pkg/services/operator_settings/`, `pkg/services/factory_visualization/`, and
  `pkg/services/system_initialization/` own settings resolution, runtime
  presentation/projection, and system bootstrap/rollback respectively.
- `pkg/services/edges/` aggregates the exact replaceable external-effect ports
  accepted by `root.BuildProcess`; it is not a product service locator.
- `pkg/services/factory_runtime/internal/orchestrators/petri/` contains internal
  Petri-net primitives. External packages consume Factory Runtime root
  contracts instead.
- `pkg/services/factory_runtime/internal/services/orchestration/javascript/` contains
  JavaScript workflow runtime, preview, source lookup, storage, and validation
  implementations. Public orchestration contracts are exposed at
  `pkg/services/factory_runtime`.
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
- Run `make generate-api` for the bundled OpenAPI and direct Go/dashboard
  clients. Use `make interfaces-all` when a change also affects generated
  contract schemas or publishable UI package clients; the scoped
  `interfaces-go` and `interfaces-ui` targets are available when only one
  consumer family changed.
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
- `make build-all` regenerates public interfaces and builds the publishable UI
  packages, dashboard, and Go CLI from canonical sources.
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
