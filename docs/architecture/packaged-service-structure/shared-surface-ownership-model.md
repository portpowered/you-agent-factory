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

## Scope boundary

This model is **integration metadata only**. Publishing or updating the
inventory does not authorize package moves, public contract changes, or
transport cutovers. Later PSS fan-in packets must still land through the named
serial lanes after any live portfolio holds clear.
