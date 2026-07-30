# Backend Package Structure

This document describes the current Go package layout and the durable ownership
rules behind it. It is a living architecture map, not a migration ledger.
Temporary exceptions, historical package paths, and planned moves belong in
development plans or mechanical baselines rather than here.

The normative engineering rules remain under
[`docs/internal/standards/`](../internal/standards/STANDARDS.md). In particular,
use the
[`general backend standard`](../internal/standards/code/general-backend-standards.md)
for dependency direction, code shape, testing, and package-size requirements.

## Top-Level Package Families

The backend has six direct package families:

```text
pkg/
  initializer/
  platform/
  root/
  services/
  transports/
  wire/
```

Their responsibilities are:

| Family | Responsibility |
| --- | --- |
| `pkg/root` | Caller-facing process construction. `root.BuildProcess` is the normal boundary used by the CLI and functional tests. |
| `pkg/wire` | Canonical dependency graph and production provider selection. Construction is inert; lifecycle activation happens later. |
| `pkg/initializer` | Application and process lifecycle. It starts, stops, cancels, joins, and unwinds roles that were already constructed. |
| `pkg/services` | Product-domain contracts, operations, and implementations grouped by durable owner. |
| `pkg/transports` | CLI, HTTP, MCP, generated transport contracts and clients, and boundary mapping/composition. |
| `pkg/platform` | Policy-free implementations for cross-cutting effects and infrastructure. |

Do not recreate retired domain roots beside these families. New product behavior
belongs to the narrow service owner; reusable technical behavior belongs to a
focused Platform package; protocol behavior belongs at a transport boundary.

## Construction and Lifecycle

The normal application flow is:

```text
cmd/factory
  -> pkg/root.BuildProcess
  -> pkg/wire
  -> constructed service and transport roles
  -> pkg/initializer
  -> selected CLI, HTTP, MCP, or background lifecycle
```

`cmd/factory` supplies process input and translates the terminal result.
`pkg/root` presents the reusable construction boundary. `pkg/wire` selects and
connects concrete providers once. `pkg/initializer` owns activation and
shutdown; it does not serve as a second dependency-injection graph.

Factory Session runtime opening is a domain operation over the injected Factory
Sessions and Factory Runtime capabilities. It can create session-owned runtime
state, but it is not another application construction pass.

Functional application tests follow the same path through `root.BuildProcess`
and `Process.Execute`. Replaceable process, filesystem, provider, clock, and
other external effects are supplied through the exact ports aggregated by
`pkg/services/edges`.

## Service Package Convention

Each direct child of `pkg/services` is a durable product owner. The common
package shape is:

```text
pkg/services/<owner>/
  *.go          public contracts, values, and owner-level operations
  internal/     private implementation
  wire/         focused construction providers used by the application graph
  transports/   service-owned protocol adapters when needed
```

This is an ownership convention, not a demand that every service root have the
same number of interfaces, files, or subservices.

- Root files expose the contracts and owner-level operations that peer services
  and transports are allowed to consume.
- Implementation state, IO, orchestration details, and private collaborators
  belong under the owner's `internal/` tree.
- A service's `wire/` package exposes focused construction providers; it does
  not own application lifecycle or customer policy.
- A service's `transports/` package adapts its public contracts to a protocol.
  Top-level transports still own server, command-tree, and MCP composition.
- Nested collaborators under `internal/services/` are private implementation
  details, not public peer-service APIs.
- Tests and fixtures stay with the smallest owner. Package-specific test
  directories do not establish a reusable production package convention.

Cross-service code should depend on the peer service root, not its private
implementation. When a change crosses several services, place the policy with
the service that owns the durable state or decision and keep other services and
transports as callers or adapters.

## Current Service Owners

| Service | Durable responsibility |
| --- | --- |
| `factory_definitions` | Authored Factory loading, validation, compilation, persistence, catalogs, packaged distribution, and invocation policy derived from definitions. |
| `factory_runtime` | Event-first orchestration, scheduling, dispatch, Petri execution, JavaScript workflows, runtime projections, and checkpoint recovery. |
| `factory_sessions` | Live and durable Factory Session state, runtime opening, invocation, response streams, lifecycle gateways, controls, and persisted execution behavior. |
| `recordings` | Canonical Factory Event ledger, recording lifecycle, replay, artifacts, and historical/read-model projections. |
| `work` | Work and Work Request admission, content, staging, materialization, lineage, reads, and pure invocation return policy. |
| `workers` | Worker and workstation execution, runner policy, prompts and output shaping, worktrees, mock workers, and worker capability policy. |
| `providers` | Provider identity, catalog, lifecycle, configuration, ACP integration, and provider execution. |
| `models` | Managed local-model catalog, assets, runtime readiness and lifecycle, host supervision, source resolution, cache, pull, and local inference support. |
| `automations` | Cron, filesystem watcher, script poller, hosted-source, reconciliation, and invocation scheduling behavior. |
| `provider_sessions` | Provider-session discovery and provider transcript/session inspection. |
| `operator_settings` | Operator configuration documents, defaults, input inventory, and effective settings resolution. |
| `factory_visualization` | Factory runtime presentation, live-view projections, and response-event presentation. |
| `system_initialization` | System bootstrap and rollback operations. |
| `edges` | Aggregation of replaceable external-effect ports accepted at the root construction boundary. It is not a general service locator. |

Providers and Workers are separate owners. Providers owns provider protocols,
catalogs, lifecycle, and execution. Workers owns worker/workstation behavior and
consumes Providers through public contracts. Likewise, Recordings owns the
canonical ledger and replay even when Factory Runtime or Factory Sessions emits
or consumes those records.

## Platform Packages

`pkg/platform` contains technical implementations that can be reused without
choosing product policy. Current areas include browser launching, clocks,
content staging, filesystem and directory replacement, generated-artifact
support, HTTP server mechanics, logging, metrics, portable files, process and
PTY execution, randomness, replay storage, standard streams, and runtime
artifacts.

Platform packages may implement effect interfaces owned by a service. They
should not decide which Factory, Work, worker, provider, model, schedule, or
session behavior the product selects.

## Transport Boundaries

`pkg/transports` currently contains:

- `cli` for command-tree composition, flags, presentation, and CLI protocol
  mechanics;
- `http` for generated server/client contracts, route composition, and shared
  HTTP boundary behavior;
- `mcp` for MCP tool and server composition; and
- `mapping` for representation conversion at public boundaries.

Transport packages translate and invoke service capabilities. They do not own
canonical domain state or silently reimplement service policy. When an adapter
is specific to one service, prefer the owning service's `transports/<protocol>`
package and keep the top-level protocol package focused on composition.

## Publishable Packages and Generated Artifacts

Publishable sources live outside the Go `pkg/` tree:

```text
packages/
  api/
  model-providers/
  packaged-factories/

ui/packages/
  client/
  components/
  factory-emulator/
  factory-replay/
  factory-visualizers/
```

Authored packaged Factory definitions live under
`packages/packaged-factories/factories/`. Their generated catalog and expanded
artifacts live under `packages/packaged-factories/generated/` and must not be
hand-edited.

OpenAPI is authored in `api/openapi-main.yaml` and `api/components/`.
`api/openapi.yaml`, generated Go HTTP contracts, generated schema packages, and
generated UI clients are derived outputs. Use `make generate-api` for the
direct OpenAPI/Go/dashboard path or `make interfaces-all` when publishable
contract and UI-package interfaces must also be refreshed.

## Tests and Mechanical Checks

New cross-boundary functional scenarios live under
`tests/functional/<domain>/<subsection>/`. `tests/functional_test/` contains
legacy fixture and compatibility coverage and is not the destination for new
scenarios. Shared functional support belongs under
`tests/functional/internal/support`; repository-wide Go test utilities belong
under `internal/testutil`.

The package layout is guarded mechanically:

- `make pkg-file-count` checks the per-package Go file budget.
- `make pkg-boundary` checks dependency and ownership boundaries.
- `make pkg-structure` checks repository-specific package and functional-test
  shape.
- `make package-target-manifest-check` validates package target inventory.
- `make lint` runs these checks with the rest of the lint suite.

Mechanical baselines may record temporary, deletion-only debt. Those baselines
describe current exceptions; they do not redefine durable ownership or belong
in this architecture map.
