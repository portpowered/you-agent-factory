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

## Scope boundary

This model is **integration metadata only**. Publishing or updating the
inventory does not authorize package moves, public contract changes, or
transport cutovers. Later PSS fan-in packets must still land through the named
serial lanes after any live portfolio holds clear.
