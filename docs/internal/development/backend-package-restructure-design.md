# Backend Package Restructure Design

---
author: codex agent
last modified: 2026, may, 25
doc-id: AGF-DEV-003
---

This document defines the canonical target package tree for the backend
restructure under `pkg/`. Later migration stories should move code toward this
tree without intentionally changing runtime behavior.

## Goals

- shrink large package boundaries, especially `pkg/workers` and the
  `pkg/factory` root package
- make dependency direction obvious before code motion begins
- keep stable backend-facing seams explicit while pushing implementation details
  into narrower packages
- give reviewers one package-tree contract to compare every migration against

## Non-goals

- no intentional runtime, API, replay, or generated-contract behavior changes
- no opportunistic package renames outside the approved tree below
- no broad refactors that are not needed to move code into the named
  destination packages

## Target `pkg/` Tree

```text
pkg/
├── api/                    # HTTP transport, generated server contracts, API fixtures
├── apisurface/             # Stable API/service boundary contracts and API-owned error/result types
│   └── optional/           # Optional-field helpers for generated API payloads
├── cli/                    # CLI entrypoints, flows, and render helpers
├── config/                 # Config parsing, boundary normalization, runtime projections
├── factory/                # Stable factory-facing boundary package only
│   ├── context/            # Shared factory execution context seam
│   ├── engine/             # Petri-net execution engine internals
│   ├── events/             # Canonical event history, generated event helpers, subscriptions
│   ├── projections/        # Read models and derived runtime views
│   ├── requests/           # Work-request normalization, relation indexing, trace propagation
│   ├── runtime/            # Runtime loop, dispatch wiring, runtime-owned buffers
│   ├── scheduler/          # Transition selection and queueing behavior
│   ├── state/              # Net topology and validation primitives
│   ├── subsystems/         # Dispatcher, transitioner, circuit-breaker, termination internals
│   ├── throttle/           # Throttle windows and pause semantics
│   ├── token_transformer/  # Runtime token mutation helpers
│   └── workstationconfig/  # Runtime workstation lookup and config adaptation
├── generatedclient/        # Generated API client bindings
├── interfaces/            # Shared backend contracts and leaf runtime types
├── logging/               # Logging abstractions
├── petri/                 # Petri net primitives
├── replay/                # Replay artifact IO and reconstruction
├── service/               # Runtime orchestration and service-layer coordination
│   └── ingest/            # External input ingestion edges such as file watching
├── testutil/              # Shared backend test harnesses and fixtures
├── timework/              # Time-triggered work helpers
├── workcontent/           # Stable generated-contract <-> runtime content translation seam
└── workers/               # Stable worker contracts and narrow worker subpackages
    ├── executor/         # Agent, script, noop, and workstation execution orchestration
    ├── process/          # Subprocess request shaping, env merge, timeout and exit handling
    ├── prompting/        # Prompt contracts, rendering, template resolution
    └── provider/         # Provider CLI execution, diagnostics, retry/error normalization
```

## Package Decisions

### `pkg/workers`

Target shape:

- `pkg/workers` remains the stable home for shared worker-facing contracts that
  are consumed across the backend, such as executor registration seams, worker
  config-facing types, and result metadata that other packages must import.
- `pkg/workers/process` owns subprocess execution details now mixed into
  `command.go`, `command_env.go`, and the platform-specific process files.
- `pkg/workers/prompting` owns prompt contracts, prompt rendering, project
  prompt loading, and template field resolution now mixed into `prompt*.go`
  and `template_fields.go`.
- `pkg/workers/provider` owns provider execution, provider session extraction,
  normalized provider errors, diagnostics, and corpus-backed normalization.
- `pkg/workers/executor` owns agent, script, noop, workstation, and pool-level
  orchestration that composes prompting, provider, and process behavior.

Dependency direction:

- `process`, `prompting`, and `provider` are leafward helper packages.
- `executor` may depend on `process`, `prompting`, `provider`, `interfaces`,
  `logging`, `config`, and `factory/context`.
- the stable `workers` root may re-export or host shared contracts, but it
  should not become the implementation home for process, prompt, provider, or
  executor behavior once the migration is complete.

### `pkg/factory`

Target shape:

- `pkg/factory` becomes a narrow boundary package for stable factory-facing
  contracts, options, clocks, and small cross-runtime types that need to remain
  importable by higher layers.
- `pkg/factory/requests` owns the canonical work-request normalization path,
  request JSON helpers, relation indexing, request ID shaping, and trace
  propagation currently mixed into `work_request*.go` and `submit_trace.go`.
- `pkg/factory/events` owns canonical event history, generated event helpers,
  replay-facing event emission helpers, subscriptions, and snapshot-friendly
  event buffering now mixed into `event_history*.go`.
- `pkg/factory/runtime` remains the owner of runtime loop assembly and should
  depend on the stable `factory` root plus `events`, `requests`, `engine`,
  `scheduler`, `subsystems`, and `workers`.

Dependency direction:

- `factory/engine`, `factory/scheduler`, `factory/state`, and
  `factory/subsystems` stay implementation-oriented packages below the stable
  `factory` and `service` layers.
- `factory/events` and `factory/requests` may depend on `interfaces`,
  `workcontent`, `factory/state`, `factory/projections`, and other lower-level
  factory helpers as needed, but higher orchestration layers should consume
  their exported seams instead of duplicating that logic.
- `pkg/factory` must not regain ownership of large request-normalization or
  event-history implementations after those moves land.

### `pkg/listeners`

Target: move to `pkg/service/ingest`.

Rationale:

- the current file watcher is an external ingestion edge owned by service-layer
  runtime assembly, not a stable domain concept on its own
- `pkg/service` already constructs and owns the watcher lifecycle
- moving it under `service/ingest` keeps transport and filesystem concerns out
  of the top-level `pkg/` surface

### `pkg/buffers`

Target: move to `pkg/factory/runtime`.

Rationale:

- the generic typed buffer is currently used only by the factory engine/runtime
  dispatch path as the runtime result channel
- it is not a demonstrated cross-domain backend boundary yet
- keeping the implementation under `factory/runtime` matches the actual owner
  of buffer capacity, drop behavior, and runtime lifecycle decisions

### `pkg/workcontent`

Target: remain top-level.

Rationale:

- it is the canonical translation seam between generated API contracts and the
  backend-owned `interfaces.WorkContentPart` model
- it is consumed by `api`, `config`, `factory`, `replay`, and `service`
  without belonging to one of those packages semantically
- keeping it top-level preserves a stable, narrow boundary around shared work
  content conversion behavior

### `pkg/apisurface`

Target: remain top-level.

Rationale:

- it is the service-owned contract consumed by API handlers, CLI runtime entry
  points, and service-layer implementations
- the package defines boundary-only errors and result types that should not be
  buried inside `service` or `api`
- keeping it top-level makes the runtime API boundary explicit and helps avoid
  transport packages importing deep service implementation details

## Stable Boundaries vs. Implementation Packages

Stable backend-facing boundaries:

- `pkg/apisurface`
- `pkg/config`
- `pkg/factory`
- `pkg/factory/context`
- `pkg/interfaces`
- `pkg/service`
- `pkg/workcontent`
- selected shared contracts in `pkg/workers`

Implementation-oriented packages:

- `pkg/factory/engine`
- `pkg/factory/events`
- `pkg/factory/projections`
- `pkg/factory/requests`
- `pkg/factory/runtime`
- `pkg/factory/scheduler`
- `pkg/factory/state`
- `pkg/factory/subsystems`
- `pkg/factory/throttle`
- `pkg/factory/token_transformer`
- `pkg/factory/workstationconfig`
- `pkg/service/ingest`
- `pkg/workers/executor`
- `pkg/workers/process`
- `pkg/workers/prompting`
- `pkg/workers/provider`

Rule of thumb:

- if a package must be imported by multiple top-level backend surfaces to talk
  about a shared contract, it should stay stable and narrow
- if a package mainly hides runtime mechanics, subprocess wiring, filesystem
  work, or behavior composition, it should live below a stable boundary

## Dependency Rules

1. Leaf packages such as `interfaces`, `logging`, `petri`, and `workcontent`
   must stay free of higher-layer runtime imports.
2. `pkg/workers/process`, `prompting`, and `provider` must not import
   `pkg/factory/runtime`, `pkg/service`, or `pkg/api`.
3. `pkg/workers/executor` composes lower worker packages but should not become
   a second service layer.
4. `pkg/factory/requests` and `pkg/factory/events` are the only approved homes
   for canonical request-normalization and event-history behavior.
5. `pkg/service/ingest` may depend on stable factory and interface boundaries,
   but factory internals must not depend back on service ingestion packages.
6. `pkg/api` and `pkg/cli` should depend on stable seams such as `apisurface`,
   `service`, `config`, `factory`, and `workcontent`, not on worker or factory
   implementation packages unless an existing stable boundary truly does not
   exist yet.

## Migration Order

1. Publish this target tree and use it as the approval contract.
2. Move worker subprocess plumbing into `pkg/workers/process`.
3. Move prompt rendering into `pkg/workers/prompting`.
4. Move provider execution and normalization into `pkg/workers/provider`.
5. Move executor orchestration into `pkg/workers/executor`.
6. Move factory request normalization into `pkg/factory/requests`.
7. Move canonical event history into `pkg/factory/events`.
8. Re-home helper-style packages according to the decisions above.
9. Close the migration with explicit verification notes and deferred-cleanup
   inventory if any compatibility seams remain temporarily.

## Verification Expectations Per Phase

Every package-motion phase should include:

- repo-wide import updates in the same change that introduces the new package
- behavior-focused tests at the existing observable seam for the moved logic
- compile or type validation plus affected test execution
- a progress note that maps the moved files to the target package named here

