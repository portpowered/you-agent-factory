# Shared-surface ownership model

This document is the Packaged Service Structure **integration metadata only**
scheduling contract for shared OpenAPI/HTTP, CLI, and MCP composition surfaces.

It does **not** authorize package moves, public contract changes, or transport
cutovers by itself. Live inventory data lives in
[`shared-surface-ownership.json`](./shared-surface-ownership.json) and is
validated against
[`shared-surface-ownership.schema.json`](./shared-surface-ownership.schema.json).

## Record shape

Every shared-surface record includes:

| Field | Meaning |
| --- | --- |
| `surfaceId` | Stable surface ID |
| `protocolFamily` | Customer-facing family: `openapi-http`, `cli`, or `mcp` |
| `exclusiveChangedPathSummary` | Exclusive changed-path responsibility summary |
| `serialIntegratorLaneId` | Exactly one of `PSS-I02`, `PSS-I03`, or `PSS-I04` |
| `activeHolder` | Active write-lease holder, if any |
| `ownerRequestQueue` | Ordered accepted owner-request queue |
| `holdConditionRefs` | Optional references to portfolio hold IDs |

## Serial integrator rules

- Each surface has **exactly one serial integrator** lane.
- Concurrent write leases on the same shared surface are forbidden
  (`concurrentWriteLeasesAllowed: false`).
- Dual integrator declarations are invalid.

## Owner-request queue behavior

- Accepted owner requests wait in **deterministic-stable** order.
- Only the **head of queue** may become the active integrator
  (`activationPolicy: head-of-queue-only`).
- Owner-local adapter work outside the exclusive shared paths remains
  **unblocked**.

## PSS-I02 OpenAPI / HTTP shared surfaces

`openapi-http` surfaces serialize exclusively under **PSS-I02**. Required shared
surfaces cover:

- authored OpenAPI entrypoint and fragments (`api/openapi-main.yaml`,
  `api/components/**`)
- bundled OpenAPI output (`api/openapi.yaml`)
- generated Go server and client artifacts
- generated TypeScript OpenAPI client when regenerated with the same contract
  lane
- top-level HTTP server/route composition

Accepted `HTTP-*` / service-owned adapter cutovers queue on those surfaces;
this inventory does not perform any cutover by itself.

Owner-local **service-owned** HTTP adapters under service transport paths remain
**concurrent-safe** relative to PSS-I02 as long as they do not edit the exclusive
shared composition paths above.

OpenAPI and generated output changes occur only for **approved public contract**
changes, not package motion.

## PSS-I03 CLI shared surfaces

`cli` surfaces serialize exclusively under **PSS-I03**. Required shared surfaces
cover:

- CLI root assembly (`pkg/transports/cli` root composition)
- shared Cobra/manifest generation authority (`contracts/cli/**`,
  `climanifest` / `climanifestcobra` / `climanifestgen`, generated CLI family
  artifacts)
- global help and shell-completion composition paths that serialize with root
  ownership

Accepted `CLI-*` / service-owned adapter cutovers queue on those surfaces; this
inventory does not perform any CLI cutover by itself.

Owner-local **service-owned** CLI adapters remain explicitly allowed to proceed
on **disjoint** paths while PSS-I03 serializes only shared root/manifest
composition.

This inventory **does not transfer** or redefine the live **Schema CLI**
manifest/generation ownership. It only names the PSS serial lane that must later
integrate through that ownership after portfolio holds clear.

## PSS-I04 MCP shared surfaces

`mcp` surfaces serialize exclusively under **PSS-I04**. Required shared surfaces
cover:

- top-level MCP server composition (`pkg/transports/mcp/server`,
  `pkg/transports/mcp/stdio` SDK registration and stdio serve wiring)
- shared tool registry / discovery catalog composition (`contracts/mcp/**`,
  `pkg/transports/mcp/discoverygen`, `pkg/transports/mcp/generated`)

Accepted `MCP-*` / service-owned adapter cutovers queue on those surfaces; this
inventory does not perform any MCP cutover by itself.

Owner-local **service-owned** MCP adapters remain **concurrent-safe** relative
to PSS-I04 as long as they do not edit the exclusive shared registry/server
composition paths above.

Top-level MCP owns **server and registration only**. Tools continue to depend on
**injected root contracts** rather than service implementations.

## Live portfolio hold conditions

Required **portfolio holds** record current live contention so PSS shared-lane
work waits correctly instead of bypassing owners:

| Hold ID | External owner | Blocks | Release condition |
| --- | --- | --- | --- |
| `hold.schema-cli.pr-1262.cli-manifest-generation` | Schema CLI `docs`/`models`/`mcp` (**PR #1262**) live CLI-manifest/generation ownership | Conflicting **PSS-I03** shared root/manifest/generation cutovers | Accepted clear or merge of Schema CLI PR #1262 ownership |
| `hold.standardized-providers.conductor` | **Standardized Providers** conductor live provider composition/invocation ownership | Conflicting provider-config/composition cutovers, including **PSS-I02** OpenAPI cutovers that would race provider-config or CLI-manifest generation while the conductor is live | Accepted clear or merge of the Standardized Providers conductor ownership |

Holds are **non-bypassable** (`bypassable: false`). Each hold is actionable: it
names the blocked PSS lane or surface class, the external owner, and the release
condition. Owner-local PSS packets whose exclusive paths do **not** overlap the
held surfaces remain allowed to proceed
(`ownerLocalNonOverlappingAllowed: true`).

This inventory **does not seize** CLI-manifest or provider-conductor ownership
and does not open competing provider catalog/execution abstractions. It only
records hold conditions that block conflicting cutovers until those owners
clear.

## Scope boundary

This model is **integration metadata only**. Publishing or updating the
inventory does not authorize package moves, public contract changes, or
transport cutovers. Later PSS fan-in packets must still land through the named
serial lanes after any live portfolio holds clear.

## Complete-inventory validation

Use `ValidateCompleteDocument` in `internal/sharedsurfaceownership` for the
canonical inventory. It fail-closes when a required OpenAPI/HTTP, CLI, or MCP
surface family member is missing, when a surface lacks a serial integrator,
when a surface is mapped to more than one of PSS-I02/I03/I04, when the
owner-request queue model is missing, or when a required portfolio hold is
absent. Diagnostics name the affected surface or hold ID and the violated rule.

Validation is **read-only**: it does not mutate authored OpenAPI, CLI
manifests, MCP registries, or generated artifacts, and it does not require a
regeneration step.
