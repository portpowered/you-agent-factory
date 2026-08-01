# Model Host Data Plane

This note documents the process-wide model host boundary that owns local managed
runtime assets, supervised server processes, readiness, leases, and
provider-neutral failure classes. It complements
`docs/architecture/managed-model-runtime.md`, which describes the customer-facing
managed-runtime vocabulary.

## Ownership Boundary

The model host is owned at **process/service** scope, not by individual factory
sessions.

| Responsibility | Owner |
| --- | --- |
| Local model asset cache inspection | `pkg/services/models/internal/services/runtime_host` via the Models Assets service |
| Pull or install materialization | `pkg/services/models/internal/services/assets` delegating to `pkg/services/models/internal/local` pull/cache |
| Supervised local server lifecycle (`LLAMACPP`) | `pkg/services/models/internal/services/runtime_host` runtime slots |
| Readiness and failure-class projection | `pkg/services/models/internal/services/runtime_host` |
| Lease issuance, release, and capacity | `pkg/services/models/internal/services/runtime_host/internal/services/leases` |
| Idle unload and resource-pressure eviction | `pkg/services/models/internal/services/runtime_host` |
| Managed-runtime API/CLI vocabulary | root `models.Service` + `pkg/transports/mapping` |
| Factory session runtime state | per-session runtime only |

Factory sessions and workers **borrow** local model capacity through host leases.
They do not own subprocesses, asset caches, or unload policy.

Direct invocation and local inference/agent worker execution route through
the private runtime-host execution adapter when the process-wide host is configured.
Supervised leases pass `ServingEndpoint` metadata from
`lease.Endpoint` into runtime execution so inference uses the host-owned server
boundary instead of bypassing it with a separate local runtime load path.
`CatalogHost.InspectReadiness` preserves installed asset `READY`/`INSTALLED`
projection until a supervised runtime slot exists; live slot state overlays
loading, ready, and failed outcomes. The canonical model service consumes those
neutral snapshots and applies invocation readiness gating and public failure
classification.

## Service Wiring

Wire constructs one process-wide host during runtime dependency assembly:

- providers: `ProvideLocalModels` and `ProvideModelService` in
  `pkg/wire/providers`
- host implementation contract: `pkg/services/models/internal/services/runtime_host.Service`
  with legacy local-runtime compatibility retained under `pkg/services/models/internal/legacyhost`
- session access: the model service and Worker Execution service receive the
  same injected local-model domain

Managed-runtime list, inspect, pull, and invocation-readiness surfaces are owned
by `pkg/services/models/internal/service`. Transports receive the root service interface directly, while the
lower-level model host reports readiness snapshots and process-neutral failure
classes.

## Supervised Local Runtimes

`LLAMACPP` backends require installed cache assets plus a healthy supervised
server process before readiness becomes `READY`. Worker configs must include
`--health-endpoint <url>`. `CatalogHost.AcquireLease` waits on configured health
checks before issuing leases with `endpoint` metadata.

## Leases and Capacity

Lease capacity comes from the scoped `MODEL` resource `capacity`. The host shares
one supervised runtime across concurrent leases when capacity allows.
`AcquireLease` rejects overcommit with `ErrCapacityExhausted`. `ReleaseLease`
schedules idle unload via `Options.IdleUnloadAfter` when no leases remain.
`Options.MaxLoadedRuntimes` evicts idle resident slots before loading another
supervised runtime.

## Operator Diagnostics

Model host activity emits structured logs and optional counters without recording
prompts, request payload content, or generated model output.

Diagnostic fields include:

- `managed_runtime_identity`
- `backend`
- `readiness_state` / `lifecycle_state`
- `failure_class`
- `lease_id` (lease paths)
- `unload_reason` (`explicit`, `idle`, `pressure_eviction`)

Counter names use the `model_host.*` prefix:

- `model_host.load.success` / `model_host.load.failure`
- `model_host.readiness.timeout`
- `model_host.process.crash`
- `model_host.lease.acquire` / `model_host.lease.release` / `model_host.lease.exhausted`
- `model_host.unload`

When configured, counters route through the explicitly injected metrics edge
composed by `pkg/wire`.
Managed-runtime pull logging and `managed_runtime.pull.*` counters are owned by
`pkg/services/models/internal/service/pull.go`; model-host pull execution does not emit a second
telemetry series.

## Failure Classes

Provider-neutral failure classes drive caller outcomes:

- `missing_assets`
- `loading_timeout`
- `process_crash`
- `unsupported_runtime`
- `cancelled`
- `capacity_exhausted`

These map to managed-runtime `readinessState` values through
`pkg/services/models/internal/legacyhost/contract.go`, the private runtime-host service,
and `ManagedRuntimeFromSnapshot`.
