# Managed Model Runtime

This note documents the source abstraction boundary for managed model runtimes in
infinite-you. It is intentionally sparse and complements the public OpenAPI and
reference docs rather than replacing them.

## Customer Contract

Customers interact with **managed runtimes** through one provider-agnostic
lifecycle vocabulary:

- **identity** — stable runtime name shared by list, inspect, pull or install,
  factory dependency validation, and invocation surfaces
- **readinessState** — whether invocation can proceed now (`READY`, `MISSING`,
  `LOADING`, `FAILED`, `UNSUPPORTED`)
- **lifecycleState** — install, cache, and load progression (`NOT_APPLICABLE`,
  `NOT_INSTALLED`, `INSTALLING`, `INSTALLED`, `LOADING`, `LOADED`)
- **locality** — `LOCAL` or `CLOUD`
- **supportedOperations** — provider-agnostic operation contract
- **managedRuntimePull.pullOutcome** — source-agnostic pull or install outcomes

Legacy discovery fields such as `status`, `loadState`, and `outcome` remain as
compatibility projections of the managed-runtime contract.

## Source Abstraction Boundary

Managed runtimes sit above backend asset sources. The public contract does not
require customers to know whether assets resolve from one upstream repository,
a managed mirror, or another configured backend.

Responsibilities split as follows:

| Layer | Responsibility |
| --- | --- |
| OpenAPI and CLI surfaces | Publish managed-runtime vocabulary only |
| Catalog and readiness projection | Classify configured workers and resources into managed lifecycle states |
| Source resolver | Select a backend source for a managed runtime identity |
| Asset puller or installer | Materialize required assets into the managed cache |
| Local runtime manager | Load installed assets into invocation-ready handles |

Optional `sourceDiagnostics` may expose resolver-classified details for
operators. Those details are advanced diagnostics, not primary customer setup
language.

## Multiple Backend Sources

One managed runtime identity may be satisfied by more than one configured
backend source. Source selection is an implementation concern of the resolver
layer. Public field names and primary wording stay stable regardless of whether
the active source is an upstream repository, a regional mirror, or another
provider integration.

## Factory Dependency Contract

Packaged factories and authored factories declare managed runtime dependencies
through the same `MODEL` resource shape in `factory.json`:

- `resources[].type: MODEL` carries the stable managed runtime identity in
  `model`, plus `backend` and `loadPolicy`.
- LOCAL `INFERENCE_WORKER` entries reference that resource through
  `workers[].resources[]` and keep `model` aligned with the managed runtime
  identity. Legacy `MODEL_WORKER` inference workers remain accepted during the
  migration window.
- Validation rejects unsupported identities, invalid backend or load-policy
  combinations, and LOCAL workers without a matching `MODEL` resource.
- Readiness for a factory dependency uses the same `managedRuntime` projection
  as `/models` discovery and inspect.

## Invocation Consumption

Invocation surfaces consume **readinessState** and **lifecycleState** from the
managed runtime contract instead of re-deriving source-specific cache or
repository conditions.

When a required runtime is `MISSING`, `LOADING`, `FAILED`, or `UNSUPPORTED`,
invocation returns actionable outcomes derived from the managed contract through
`apisurface.InvocationErrorFromManagedRuntime`. When a runtime is `READY`,
packaged and authored factories invoke through the same managed runtime layer.

Production composition constructs the root `models.Service` through
`pkg/services/models/wire` and injects that service directly into Factory Sessions.
Opening a Factory Session passes Models-owned runtime data to `Service.ForRuntime`;
it does not receive or invoke a separate Models runtime opener or construct the
Models dependency itself. The runtime view is bound from model-scoped dependencies:
the dynamic active-runtime configuration reader,
the process model host, asset puller, logger, clock, pull-metrics recorder,
direct-invocation executor builder, and factory runner identity. Stable
collaborators are supplied as direct values; only runtime configuration remains
a callback because activating another factory changes it after construction.
The model service does not receive `FactoryService`, `runtimehost.Host`, or an
adapter around either coordinator.

## Pull or Install Lifecycle

`POST /models/{model_name}/pull` and `you models pull` use one canonical customer
action. Pull results classify managed outcomes (`ALREADY_READY`,
`INSTALLED_SUCCESSFULLY`, `ALREADY_PRESENT`, `STILL_LOADING`, `TIMED_OUT`,
`SOURCE_FETCH_FAILED`, `UNSUPPORTED_RUNTIME`) and expose readiness through
`managedRuntimePull`.

Source selection runs through the resolver layer before asset materialization.
Post-pull cache inspection classifies readiness and lifecycle without contacting
upstream sources again. Pull lifecycle transitions are logged and emitted as
`managed_runtime.pull.*` counters when a metrics recorder is configured at the
service boundary. Model host load/lease/unload/crash activity additionally emits
`model_host.*` diagnostics from `pkg/services/models/internal/host`; pull telemetry remains solely at
the canonical model-service boundary. See `docs/architecture/model-host.md`.
These operations do not currently emit canonical
`FactoryEvent` records; invocation and factory session surfaces remain the
event-first runtime contract.
