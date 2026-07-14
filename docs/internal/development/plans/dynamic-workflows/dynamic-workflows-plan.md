# Dynamic Workflows v0 — Program Plan

Operator-facing planning record for the Dynamic Workflows v0 program. This
document tracks batch completion, cross-surface contract posture, and the
recommended next batch for maintainers scheduling factory work.

**Last updated:** 2026-06-11 UTC (residual active-surface import gate verified — Batch 002 fake-session skeleton may proceed)

## Program overview

### Intent

Dynamic Workflows v0 introduces **JavaScript orchestrator-backed factories**
alongside existing Petri factories without splitting the runtime model. A
**dynamic workflow** is shorthand for a factory whose authored orchestrator
kind is `JAVASCRIPT`. Customers author workflow source, request policy, and
structured args; the platform runs that definition as a durable
`FactorySession` with shared dispatches, artifacts, results, and event-stream
semantics.

The v0 program goal is to land a **contract kernel** first—OpenAPI shapes,
service seams, event payloads, validation/policy stubs, and representative
fixtures—so API, CLI, MCP, dashboard, and later runtime/persistence lanes can
share one vocabulary before real JavaScript execution or durable stores exist.

### Canonical nouns and boundaries

| Noun | Role in v0 |
|------|------------|
| `Factory` | Authored orchestration definition (Petri graph or JavaScript workflow source). |
| `FactoryOrchestrator` | Authored identity (`PETRI` or `JAVASCRIPT`) on a factory. |
| `FactorySession` | **Canonical runtime object** for every live or durable orchestration on a host. |
| `Dispatch` | Shared child-execution record for Petri transitions or JavaScript tasks. |
| `FactoryArtifact` | Session-owned output (results, findings, logs, checkpoint refs). |
| `FactoryEvent` | Canonical stream for lifecycle, phase, checkpoint, dispatch, artifact, and replay facts. |

**Do not introduce `DynamicWorkflowRun` as a separate canonical runtime noun.**
A dynamic workflow run is a `FactorySession` with
`runtime.orchestratorKind = JAVASCRIPT`. Reference docs (`docs/reference/orchestrators.md`)
and generated contracts must preserve this boundary.

Petri compatibility remains: factories without an authored orchestrator block
default to `orchestrator.kind = PETRI`. JavaScript sessions project phase,
checkpoint refs, child dispatch counts, and result refs under
`runtime.javascript` without exposing raw VM checkpoint bodies.

### Batch posture

#### Batch 001 — contract kernel (merged)

Batch 001 established the shared contract surfaces that downstream batches must
not reinterpret:

- **Data model and orchestrator projections** (PR #767): `FactoryOrchestrator`,
  kind-specific `FactorySession` runtime projections, shared dispatch/artifact
  semantics, Petri defaulting.
- **OpenAPI durable session routes and schemas** (PR #771): async/sync start,
  get/list scopes, results, dispatches, artifacts, lifecycle controls, source
  resolution, `requestId` idempotency, generated types and contract fixtures.
- **FactoryEvent session lifecycle** (PR #772): `SESSION_STARTED`,
  `SESSION_RESULT_UPDATED`, `SESSION_COMPLETED`, orchestrator phase/checkpoint
  events, dispatch queue/interruption/reconciliation, artifact creation,
  reconnect and replay expectations.
- **Workflow source validation and policy stub** (PR #773): JavaScript/TypeScript
  loader expectations, source lookup order, read-only effective policy defaults,
  artifact URI conventions, structured JSON result constraints.
- **Shared execution service seams** (PR #776): `factorysessionexecution.Service`,
  deterministic fake service, apisurface mappers, durable session contract
  fixtures in `pkg/transports/http/testdata/durable-session-contract-fixtures.json`.

Batch 001 intentionally landed **contracts and projections**, not full transport
wiring or real runtime execution. Handlers may still return `501 NotImplemented`
or empty persisted listings until later batches wire transports and stores.

#### Obsolete Batch 001 preview guidance (stale — requires contract repair)

Batch 001 also merged **transitional** workflow-preview surfaces that are **not**
the intended final public contract or JavaScript orchestration ownership. Treat
any forward-looking guidance that still points at these terms as stale Batch 001
output that must be repaired before fake-session skeleton work proceeds:

| Stale Batch 001 surface | Why it is obsolete | Corrected target |
|-------------------------|--------------------|------------------|
| `POST /workflow-previews` route and `PreviewWorkflow` handler | Standalone workflow-preview product route, not Factory or Factory Session preview semantics | **Factory preview** or **Factory Session preview** surfaces under `pkg/orchestrators/javascript/preview` |
| Removed root `pkg/workflow*` packages (for example `pkg/workflowsource/`, `pkg/workflowpreview/`) | Transitional JavaScript orchestration helpers outside orchestrator ownership | `pkg/orchestrators/javascript` subpackages: `source`, `validation`, `store`, `policy`, `preview`, `result` |
| `pkg/orchestrators/javascript/validator` | Wrong package name from early planning | `pkg/orchestrators/javascript/validation` |

Historical mentions of these surfaces in the Batch 001 checklist below are
**retrospective completion notes only**. They record what merged in PRs #767–#776;
they do **not** authorize building the next batch on the standalone
workflow-preview route, root `pkg/workflow*` ownership, or `validator` package
names.

**Contract repair kernel (merged 2026-06-11 UTC):** Branch
`dynamic-workflows-contract-repair-kernel-resubmit` closed blocking inventory
items B1–B12 at the contract-kernel layer. Evidence: canonical `POST
/factories/preview` (`pkg/transports/mapping/factory_preview.go`,
`pkg/orchestrators/javascript/preview`), orchestrator-owned JavaScript helpers
under `pkg/orchestrators/javascript/{source,validation,policy,preview,result,store}`,
durable event/result enum alignment (`SessionCompletedEventPayload.finalStatus`,
`FactoryEventSessionResultStatus`), fake canonical events and reconnect cursors
(`pkg/factorysessionexecution/canonical_events.go`), durable read/control field
repair (`budgets`/`usage`/`providerSessionRefs`, `ControlErrorToAPI` status),
result mode shaping (`result_projection.go`), fixture `events[]` plus
`javascript-awaiting-approval`, and focused contract/apisurface/fake-service
tests listed in the gap inventory below.

**Active-surface repair (verified 2026-06-11 UTC):** Branch
`dynamic-workflows-contract-repair-active-surface-followup` was the **mandatory
gate before Batch 002 fake-session skeleton transport wiring**. Kernel repair
closed schema/projection drift, but planner verification still found active
standalone `POST /workflow-previews` / `WorkflowPreview` surfaces and root
`pkg/workflow*` ownership outside the orchestrator boundary. **Do not schedule
Batch 002 skeleton handler/CLI/MCP/UI wiring until this gate passes.**

Follow-up stories closed the active-surface gate:

| Story | Outcome |
|-------|---------|
| Expose preview through Factory semantics | Canonical `POST /factories/preview` with `FactoryPreviewRequest` / `FactoryPreviewResult`; `POST /workflow-previews` retained only as a deprecated compatibility alias with successor headers. |
| Move JavaScript orchestration ownership | `pkg/orchestrators/javascript/{source,validation,policy,preview,result,store}` owns behavior; root `pkg/workflow*` packages are thin `compat.go` aliases only. |
| Align API, apisurface, CLI, and MCP consumers | Handlers, mappers, CLI preview, and MCP validation exercise the Factory preview seam; compatibility alias coverage is explicit and non-primary. |
| Update UI generated types and preview adapters | `ui/src/api/factory-preview/` is canonical; `ui/src/api/workflow-preview/` and `ui/src/features/workflow-preview/` are compatibility wrappers. |
| Synchronize contracts and record blocker evidence | `make generate-api` synchronized; scoped `rg` verification and focused tests recorded below. |

Scoped active-surface verification (2026-06-11 UTC):

```bash
rg -n "workflow-previews|WorkflowPreview|pkg/workflow" \
  api/openapi-main.yaml api/components pkg/api pkg/apisurface pkg/cli pkg/mcp \
  pkg/factorysessionexecution pkg/factory/sessions pkg/orchestrators/javascript ui/src
```

Remaining hits are **only** generated compatibility aliases
(`pkg/transports/http/generated/server.gen.go`, `ui/src/api/generated/openapi.ts`),
deprecated OpenAPI compatibility routes/schemas (`api/openapi-main.yaml`,
`WorkflowPreviewRequest.yaml` / `WorkflowPreviewResult.yaml`),
obsolete compatibility handlers/tests (`pkg/transports/http/handlers_factory.go`,
`pkg/transports/http/contracttests/openapi_contract_factory_validation_test.go`,
`pkg/transports/http/servertests/server_factory_preview_test.go`), and explicit
compatibility UI wrappers (`ui/src/api/workflow-preview/`,
`ui/src/features/workflow-preview/`). No scoped path imports root `pkg/workflow*`
as a final owner.

Focused verification:

```bash
make generate-api
go test ./pkg/api/contracttests ./pkg/api/servertests ./pkg/apisurface \
  ./pkg/apisurface/factorysession ./pkg/factorysessionexecution ./pkg/factory/sessions \
  ./pkg/mcp/workflow ./pkg/cli/workflow
npm --prefix ui run typecheck
```

**Residual active-surface import repair (verified 2026-06-11 UTC):** Branch
`dynamic-workflows-contract-repair-residual-active-surface-imports` closed the
follow-up gate that scoped verification still found active CLI, MCP, API test,
and CLI source-normalization code importing deprecated root `pkg/workflow*`
compatibility shims instead of orchestrator-owned JavaScript packages.

| Story | Outcome |
|-------|---------|
| Canonical Factory preview remains primary | Server tests prove `POST /workflow-previews` returns the same preview body as `POST /factories/preview` with Deprecation and Link successor headers. |
| CLI preview and source normalization use orchestrator packages | CLI-package tests prove preview JSON matches `apisurface.BuildFactoryPreview` and normalize success/not-found diagnostics. |
| MCP workflow preview tests target orchestrator ownership | MCP tests import orchestrator `preview` and `source` directly and prove not-found diagnostics through `ValidateTool`. |
| API and apisurface tests prove compatibility without active shim imports | Contract/server tests assert canonical vs deprecated preview routes; apisurface tests exercise orchestrator source/preview directly. |
| Residual import gate blocks Batch 002 until clean | Scoped `rg` verification is clean (no output); behavioral contract/server/CLI/MCP/apisurface tests prove canonical preview and orchestrator ownership without root shim imports. |

Scoped residual import verification (2026-06-11 UTC):

```bash
rg -n "github.com/portpowered/infinite-you/pkg/workflow(preview|source|validation|policy|result)" \
  pkg/api pkg/apisurface pkg/cli pkg/mcp pkg/factorysessionexecution pkg/factory/sessions \
  --glob '!**/generated/**'
rg -n "/workflow-previews|WorkflowPreview" \
  api/openapi-main.yaml api/components pkg/transports/http pkg/transports/mapping pkg/transports/cli pkg/transports/mcp ui/src \
  --glob '!**/generated/**'
find pkg/orchestrators/javascript -maxdepth 2 -type f | sort
go test ./pkg/transports/http/contracttests ./pkg/transports/http/servertests ./pkg/transports/mapping \
  ./pkg/transports/mcp/workflow ./pkg/transports/cli/workflow ./pkg/transports/cli/workflowsource
npm --prefix ui run typecheck
```

The first `rg` command reports no active imports. Remaining workflow-preview
hits are generated aliases, deprecated OpenAPI compatibility routes/schemas,
compatibility handlers/tests, and explicit compatibility UI wrappers only.

#### Batch 002 — fake-session skeleton (scheduled)

Batch 002 targets **deterministic fake-session skeleton work**: API/CLI/MCP/UI
surfaces that call the shared `factorysessionexecution.Service` (including the
injectable fake implementation) so reviewers can exercise durable session
start, status, result, dispatch, artifact, event, and lifecycle flows without
a JavaScript VM or durable persistence backend.

**Go:** active-surface repair verified 2026-06-11 UTC. Schedule Batch 002
skeleton wiring against the repaired contract kernel and orchestrator-owned
preview/source seams. Remaining gaps are stubbed transport (501 handlers, empty
persisted listing) and wire-up routing for durable `GET /factory-sessions/{id}`
— not schema blockers.

Skeleton work should:

- Reuse Batch 001 OpenAPI shapes and service/domain projections verbatim.
- Prove round-trip through apisurface mappers and contract fixtures.
- Emit or consume `FactoryEvent` payloads consistent with durable read models.
- Treat transport stubs from Batch 001 as wiring targets, not permission to
  redefine schemas.

#### Later lanes (post–Batch 002)

After the fake-session skeleton proves cross-surface contract alignment, later
batches are expected to land in roughly this order. Exact batch IDs may shift;
dependencies are architectural, not calendar promises.

| Lane | Purpose |
|------|---------|
| **Runtime** | Real JavaScript workflow execution, orchestrator phase transitions, checkpoint refs, child dispatch execution. |
| **Persistence** | Durable session stores, persisted/all listing with real rows, reconnect/replay against stored events and state. |
| **Dispatch bridge** | Provider-session integration for queued/running dispatches, interruption, and reconciliation. |
| **Policy** | Enforcement beyond read-only MVP defaults; capability denial before runtime; effective-policy stability across surfaces. |
| **Release host** | Packaging, deployment, and host integration for beta factories running dynamic workflows. |
| **Beta readiness** | End-to-end operator flows, observability, failure handling, and customer-facing docs for a bounded beta. |

Each lane must keep `FactorySession` canonical and extend—not fork—the Batch
001 contract kernel.

## Batch 001 completion checklist

Merged PR range: [#767](https://github.com/portpowered/you-agent-factory/pull/767) →
[#771](https://github.com/portpowered/you-agent-factory/pull/771) →
[#772](https://github.com/portpowered/you-agent-factory/pull/772) →
[#773](https://github.com/portpowered/you-agent-factory/pull/773) →
[#776](https://github.com/portpowered/you-agent-factory/pull/776) (2026-06-08 through
2026-06-09 UTC).

Status key:

| Symbol | Meaning |
|--------|---------|
| ✅ | Contract surface landed (schemas, types, service seams, tests, or docs) |
| 🔌 | Transport/handler wiring still stubbed (`501 NotImplemented` or empty durable rows) |

### PR #767 — data model and orchestrator projections

**Merged:** 2026-06-08 · [contract-data-model-orchestrators](https://github.com/portpowered/you-agent-factory/pull/767)

| Item | Status | Completion notes |
|------|--------|------------------|
| `FactoryOrchestrator` with `PETRI` / `JAVASCRIPT` kinds on authored factories | ✅ | Factory create/read/validate accept orchestrator blocks; existing Petri configs default to `PETRI` without edits. |
| Kind-specific `FactorySession` runtime projections | ✅ | Petri sessions expose marking/enabled transitions; JavaScript sessions expose phase, phases, checkpoint refs/summaries, args digest, child dispatch counts under `runtime.javascript`. |
| Shared dispatch and artifact contracts | ✅ | One dispatch/artifact model across orchestrators; Petri transition fields and JavaScript task fields stay in kind-specific projections. |
| Raw JavaScript checkpoints behind refs | ✅ | Public projections expose checkpoint id/label/summary/ref metadata only; raw VM bodies live in internal artifacts. |
| `FactoryEvent` orchestrator terminology alignment | ✅ | Lifecycle, phase, checkpoint, dispatch, and artifact events use canonical payloads without Petri-only leakage into JavaScript progress. |
| No `DynamicWorkflowRun` canonical noun | ✅ | API/generated models and reference docs treat dynamic workflows as `JAVASCRIPT` orchestrator-backed `FactorySession` executions. |
| Durable execution transport | 🔌 | Batch 001 scoped the **data model**; durable start/read/control handlers were not wired in this PR. |

### PR #771 — OpenAPI durable session routes and schemas

**Merged:** 2026-06-08 · [contract-openapi-session-execution](https://github.com/portpowered/you-agent-factory/pull/771)

| Item | Status | Completion notes |
|------|--------|------------------|
| `POST /factory-sessions/async` and `POST /factory-sessions/sync` request/response schemas | ✅ | `FactorySessionExecutionRequest`, async/sync responses, sync timeout outcomes, and `requestId` idempotency semantics are defined in `api/openapi.yaml` with generated Go/TypeScript types. |
| Session get/list with `scope=live\|persisted\|all` | ✅ | `ListFactorySessionsResponse` distinguishes live summaries from durable summaries; scope parameter and de-duplication rules are documented. |
| Result, dispatch, artifact read routes | ✅ | OpenAPI defines `GET /factory-sessions/{session_id}/result(s)`, dispatch list/get, and artifact list/get shapes including status enums and link refs. |
| Lifecycle control routes (approve, pause, resume, cancel, terminate, retry-dispatch) | ✅ | Request/response schemas and error shapes are specified; control semantics reference canonical session/dispatch projections. |
| Workflow source resolution contract | ✅ | `WORKFLOW_FILE` / `WORKFLOW_NAME` lookup order (project → user → package → built-in → explicit factory) is documented on durable start routes. |
| Generated contracts and fixture hooks | ✅ | `pkg/transports/http/generated/` and UI generated types refreshed; contract tests can bind to OpenAPI examples. |
| Durable route handlers | 🔌 | `StartDurableFactorySessionAsync`, `StartDurableFactorySessionSync`, durable result/dispatch/artifact reads, and lifecycle controls in `pkg/transports/http/handlers_factory.go` return **`501 NotImplemented`**. |
| Persisted listing rows | 🔌 | `ListFactorySessions` with `scope=persisted` or `scope=all` returns **`durableSessions: []`** (empty) until a persistence backend is wired. |
| Live-session compatibility routes | ✅ | `POST /factory-sessions` (open), `GET /factory-sessions/{id}`, live result/partial-result reads, and live `scope=live` listing remain wired through `sessionRuntime` for existing Petri workspace flows. |

### PR #772 — FactoryEvent session lifecycle additions

**Merged:** 2026-06-09 · [contract-event-stream-session-lifecycle](https://github.com/portpowered/you-agent-factory/pull/772)

| Item | Status | Completion notes |
|------|--------|------------------|
| Session lifecycle events | ✅ | `SESSION_STARTED`, `SESSION_RESULT_UPDATED`, `SESSION_COMPLETED` bracket one durable execution with replay-safe payloads. |
| Orchestrator progress events | ✅ | `ORCHESTRATOR_PHASE_CHANGED` and `ORCHESTRATOR_CHECKPOINT_WRITTEN` expose phase/checkpoint refs without raw checkpoint bodies. |
| Dispatch and artifact observability events | ✅ | `DISPATCH_QUEUED`, `DISPATCH_INTERRUPTED`, `DISPATCH_RECONCILED`, and `ARTIFACT_CREATED` cover child-work and output facts before/after worker execution. |
| Canonical `FactoryEvent.context` envelope | ✅ | Shared identity fields (session id, orchestrator kind, phase, checkpoint, dispatch, sequence, request id) avoid duplicating context inside payloads. |
| Event replay and stream reconnect spec | ✅ | After-event-id/sequence reconnect, idempotent replay, and dispatch/provider reconciliation expectations are specified and covered by focused tests. |
| Durable event route transport for new session-scoped reads | 🔌 | Global and session-scoped event streaming for **live** sessions exists; wiring durable session event reads to the fake/real execution service is a Batch 002 skeleton target, not a Batch 001 schema gap. |

### PR #773 — workflow source validation and policy stub behavior

**Merged:** 2026-06-09 · [contract-validation-policy-stub](https://github.com/portpowered/you-agent-factory/pull/773)

| Item | Status | Completion notes |
|------|--------|------------------|
| Workflow source validation before session creation | ✅ | Inline and file-backed JavaScript validation rejects syntax errors, invalid meta/schemas, unsupported globals/primitives, and forbidden host access with path-aware diagnostics. |
| JavaScript and TypeScript loader expectations | ✅ | `.js` is the MVP executable format; `.ts` follows bounded transpile/placeholder behavior with structured unsupported-loader diagnostics. |
| Ordered source lookup contract | ✅ | `FACTORY_ID`, `FACTORY_INLINE`, `WORKFLOW_FILE`, `WORKFLOW_NAME`, and `INLINE_WORKFLOW` kinds share one resolution order across API normalization surfaces. |
| Read-only effective policy defaults | ✅ | Default mode `READ_ONLY`, bounded child limits, stable policy hashes, and fail-closed denied-capability diagnostics before runtime side effects. |
| Structured JSON result and artifact URI rules | ✅ | JSON-compatible primary results via `WorkContent`; large/binary outputs use artifact refs or `you-artifact://sessions/{session_id}/artifacts/{artifact_id}` URIs. |
| `POST /workflow-previews` handler (**stale Batch 001**) | ✅ (compatibility only) | **Closed by active-surface repair:** `PreviewWorkflow` remains a deprecated alias of canonical `POST /factories/preview` in `pkg/transports/http/handlers_factory.go`. Primary preview semantics live under `pkg/orchestrators/javascript/preview` and `pkg/transports/mapping/factory_preview.go`. |
| Durable start-time validation wiring | 🔌 | Validation/policy contracts exist; **`POST /factory-sessions/async|sync`** handlers still return **`501`** so start-time enforcement awaits Batch 002 service injection. |

### PR #776 — shared execution service, fake service, mappers, and fixtures

**Merged:** 2026-06-09 · [contract-service-seams](https://github.com/portpowered/you-agent-factory/pull/776)

| Item | Status | Completion notes |
|------|--------|------------------|
| `factorysessionexecution.Service` interface | ✅ | Start (async/sync), read/status, result/dispatch/artifact projection, lifecycle controls, listing scopes, and idempotency are defined in `pkg/factorysessionexecution/`. |
| Deterministic fake service | ✅ | Injectable fake implements the same contract with stable scenario ids, JavaScript orchestrator projections, dispatch lists, result states, artifact refs, and event sequences (`fake_service.go`, `fake_fixture.go`). |
| `apisurface` mappers | ✅ | `pkg/transports/mapping/factorysession/` round-trips OpenAPI execution requests/responses, session records, results, dispatches, artifacts, and lifecycle payloads. |
| Durable session contract fixtures | ✅ | `pkg/transports/http/testdata/durable-session-contract-fixtures.json` covers Petri and JavaScript scenarios (running, partial, final, failed-with-partial, canceled, timed-out, interrupted, multi-dispatch). |
| Projection/event consistency checks | ✅ | Service tests assert result status aligns with latest `SESSION_RESULT_UPDATED` events where fixtures include an event stream. |
| API handler injection of fake/real service | 🔌 | Handlers do not yet delegate durable routes to `factorysessionexecution.Service`; Batch 002 skeleton work wires transport → service → mappers. |
| Persisted listing backend | 🔌 | Service defines `scope=persisted|all` semantics; API listing returns empty durable rows until persistence lane or fake listing injection lands. |

### Batch 001 transport stub summary (not contract blockers by default)

The following gaps are **implementation wiring**, not missing OpenAPI/service definitions:

| Surface | Stubbed behavior | Owning follow-up |
|---------|------------------|------------------|
| Durable start (`async` / `sync`) | `501 NotImplemented` | Batch 002 — inject `factorysessionexecution.Service` (fake first) |
| Durable result index (`/results`) | `501 NotImplemented` | Batch 002 |
| Durable dispatch list/get | `501 NotImplemented` | Batch 002 |
| Durable artifact list/get | `501 NotImplemented` | Batch 002 |
| Lifecycle controls (approve, pause, resume, cancel, terminate, retry-dispatch) | `501 NotImplemented` | Batch 002 |
| `scope=persisted` / `scope=all` durable rows | Empty `durableSessions` arrays | Batch 002 fake listing, later persistence lane for real rows |

Cross-surface **schema or projection conflicts** are inventoried in the gap inventory section below. Transport stubs from the checklist are **not** repeated here unless they mask a contract mismatch.

## Cross-surface contract gap inventory

Classification key:

| Class | Meaning |
|-------|---------|
| **Blocking** | Schema, enum, mapper, or projection mismatch that would mislead Batch 002 skeleton consumers or break round-trip contract tests once handlers wire up |
| **Non-blocking** | Documented drift or incomplete coverage that can ship with Batch 002 skeleton work if tracked explicitly |
| **Stubbed transport** | Handler or backend wiring gap already accounted for in the Batch 001 checklist; not a contract conflict |

Evidence sources: `api/openapi.yaml`, `pkg/factorysessionexecution/`, `pkg/transports/mapping/factorysession/`, `pkg/transports/http/testdata/durable-session-contract-fixtures.json`, `pkg/transports/http/contracttests/`, and `pkg/transports/http/handlers_factory.go`.

### OpenAPI (`api/openapi.yaml`)

| Gap | Class | Notes |
|-----|-------|-------|
| Durable route handlers return `501 NotImplemented` | Stubbed transport | Start async/sync, durable `/results`, dispatch/artifact reads, lifecycle controls — see checklist 🔌 rows |
| `scope=persisted` / `scope=all` returns empty `durableSessions` | Stubbed transport | Listing contract is defined; rows await service injection or persistence lane |
| `SessionCompletedEventPayload.finalStatus` references live `FactorySessionStatus` (`ACTIVE`/`IDLE`/`FINISHED`) instead of durable `FactorySessionDurableLifecycleStatus` (`SUCCEEDED`/`FAILED`/…) | ✅ **Closed** (B1) | OpenAPI refs `FactorySessionDurableLifecycleStatus`; fake/projection emit `finalStatus`; `openapi_contract_events_test.go`, `projectiontests/session_lifecycle_projection_test.go` |
| `FactoryEventSessionResultStatus` omits `NOT_READY` and `UNAVAILABLE` present on `FactorySessionResultStatus` | ✅ **Closed** (B2) | Enum extended with `x-enum-varnames`; contract and projection tests round-trip `NOT_READY`/`UNAVAILABLE` |
| `ErrControlRequestIDConflict` in service has no matching OpenAPI `ErrorResponse.code` | Non-blocking (B7 downgraded) | Control `requestId` reuse with different tuples maps to lifecycle `CONFLICT` outcome on `FactorySessionLifecycleControlResponse` (with required `status`), not HTTP `ErrorResponse` — see `fake_service.go`, `ControlErrorToAPI`, `lifecycle_test.go` |
| `FactorySessionExecutionLinks` lacks `dispatches`/`artifacts` while `FactorySessionLifecycleControlLinks` includes them | Non-blocking | Start/get polling links vs post-control inspection links are intentionally asymmetric today |
| List endpoint exposes only `scope`; no query filters for status, orchestrator, recoverable, stale lease, or time ranges | Non-blocking | Service `ListSessionsRequest.Filters` is richer than current OpenAPI list params |
| `FactorySessionDurableReadModel` requires `budgets`/`usage` but service/mapper types lack those fields | ✅ **Closed** (B11) | `SessionBudgets`/`SessionUsage` on service types; `SessionReadResponseToAPI` always emits `usage.resources`; `factory_session_mapper_test.go` |
| `GET /factory-sessions/{session_id}` documents oneOf live `FactorySession` \| durable `FactorySessionDurableReadModel` | Stubbed transport (wire-up) | Schema and mappers ready; handler routing is Batch 002 `API-2` target — not a contract blocker |

**Aligned:** Durable start routes (async/sync), source kinds and resolution order, `requestId` idempotency with `EXECUTION_REQUEST_ID_CONFLICT`, result/dispatch/artifact read schemas, lifecycle control request/response shapes, listing scope enum, event reconnect params (`after_event_id`, `after_sequence`), and FactoryEvent payload vocabulary in schema oneOf.

### `factorysessionexecution.Service` (`pkg/factorysessionexecution/`)

| Gap | Class | Notes |
|-----|-------|-------|
| No production `Service` implementation; only `FakeService` | Stubbed transport | Expected Batch 001 state; handlers do not inject service yet |
| Workflow source resolution not in start path | ✅ **Closed** (B8) | `ResolveStartSource` in `start_source.go` uses `pkg/orchestrators/javascript/source`; `start_source_test.go` proves resolution order. Fake fixtures pre-seed resolved source until Batch 002 handler injection calls start path. |
| `SessionReadResult` missing `budgets`, `usage` | ✅ **Closed** (B11) | `SessionBudgets`/`SessionUsage` on `SessionReadResult`; fixture and mapper round-trips |
| `DispatchSummary` missing `providerSessionRefs` | ✅ **Closed** (B12) | Service projection and `dispatchSummaryToAPI` map refs; fixture `javascript-running-n-dispatch` includes `providerSessionRefs` |
| `ReadEvents` validates reconnect cursor but returns full event list | ✅ **Closed** (B9) | `FilterEventsAfterReconnect` in `canonical_events.go`; `canonical_events_test.go`, fake-service event tests |
| `GetResult` ignores `mode`/`includeArtifacts` shaping | ✅ **Closed** (B10) | `ProjectResultRead` in `result_projection.go`; `result_projection_test.go` |
| `deriveProjectionEvents` emits non-canonical events | ✅ **Closed** (B3) | `BuildCanonicalSessionEvents` emits full envelope with `finalStatus`; `deriveProjectionEvents` delegates |
| `InspectionLinks` includes `dispatches`/`artifacts` but OpenAPI `FactorySessionExecutionLinks` does not | Non-blocking | Service is richer than start-response link schema |
| `RETRY_DISPATCH` evaluation accepts on active sessions but fake only mutates on `FAILED` | Non-blocking | Service rule vs fake behavior inconsistency |

**Aligned:** Full `Service` interface (start async/sync, get, controls, result, dispatch/artifact reads, events, listing), durable lifecycle model (12 statuses, control kinds/outcomes), start and control idempotency helpers, listing scope (`live`/`persisted`/`all`) with filters and dedup, projection consistency validators, and deterministic `FakeService` with fixture-backed scenarios.

### apisurface mappers (`pkg/transports/mapping/factorysession/`)

| Gap | Class | Notes |
|-----|-------|-------|
| Handlers do not call mappers (501 stubs) | Stubbed transport | Mappers are proven via unit/fake-consumer tests only |
| `ControlErrorToAPI` omits required `status` on `FactorySessionLifecycleControlResponse` | ✅ **Closed** (B6) | `ControlErrorToAPI` populates `status`; `factory_session_mapper_test.go` |
| `SessionReadResponseToAPI` does not map `budgets`/`usage` | ✅ **Closed** (B11) | `sessionBudgetsToAPI` / `sessionUsageToAPI` in `helpers.go` |
| `dispatchSummaryToAPI` omits `providerSessionRefs` | ✅ **Closed** (B12) | `providerSessionRefsToAPI` in `helpers.go`; mapper fixture tests |
| `executionLinksToAPI` drops `dispatches`/`artifacts` from service `InspectionLinks` | Non-blocking | Matches narrower OpenAPI `FactorySessionExecutionLinks` |
| `ListSessionsRequestFromAPI` maps only `scope` | Non-blocking | OpenAPI has no filter params yet; service filters unused at API boundary |
| `EventReadResponseToAPI` silently skips unmarshal failures | Non-blocking | Tolerates invalid fake events today; will hide real envelope gaps |
| `SyncStartResponseToAPI` silently drops unmarshalable embedded `Result` | Non-blocking | Edge-case loss on sync start mapping |

**Aligned:** Start request/response mapping, session/result/dispatch/artifact projections, lifecycle control mapping, listing mapping, and bidirectional fixture round-trips in `factory_session_*_test.go` and `factory_session_fake_consumer_test.go`.

### FactoryEvent payloads

| Gap | Class | Notes |
|-----|-------|-------|
| `SessionCompletedEventPayload.finalStatus` enum mismatch (live vs durable) | ✅ **Closed** (B1) | See OpenAPI row |
| `FactoryEventSessionResultStatus` ⊂ `FactorySessionResultStatus` | ✅ **Closed** (B2) | Event payloads include `NOT_READY`/`UNAVAILABLE` |
| Fake/synthetic events are not valid `FactoryEvent` documents | ✅ **Closed** (B3/B4) | `BuildCanonicalSessionEvents`; fixture `events[]` validate in `generated_contract_durable_session_test.go` |
| Reconnect filtering not implemented at service layer | ✅ **Closed** (B9) | `ReadEvents` applies `FilterEventsAfterReconnect` |
| Dual phase event models (`ORCHESTRATOR_PHASE_CHANGED` vs `JAVASCRIPT_PHASE_CHANGE`) | Non-blocking | Both in schema oneOf; no durable-client preference guidance yet |
| No typed durable event payload structs in `factorysessionexecution` | Non-blocking | Only string kind list + minimal JSON snippets in `projection_consistency.go` |

**Aligned:** Canonical `FactoryEvent` envelope (`schemaVersion`, `id`, `type`, `context`, `payload`), `FactoryEventContext` reconnect fields, durable session event type vocabulary in OpenAPI, `SessionProjectionEventKinds` list, and live SSE route contract on session events.

### Contract fixtures (`pkg/transports/http/testdata/durable-session-contract-fixtures.json`)

| Gap | Class | Notes |
|-----|-------|-------|
| No `events[]` arrays with canonical `FactoryEvent` envelopes | ✅ **Closed** (B4) | Running, terminal, and `javascript-awaiting-approval` scenarios include canonical `events[]`; `assertDurableSessionScenarioEventFixtures` |
| No `AWAITING_APPROVAL` session scenario | ✅ **Closed** (B5) | Fixture `javascript-awaiting-approval` (`dur-sess-js-awaiting-001`) with `canApprove` actions |
| Interrupted/recoverable scenario only in fake builtin, not JSON catalog | Non-blocking | `BuiltinInterruptedRecoverableScenario` in `fake_fixture.go`; not in `scenarios[]` |
| No `STILL_RUNNING` sync outcome fixture | Non-blocking | Only `COMPLETED` and `TIMED_OUT` sync outcomes represented |
| Lifecycle control fixtures incomplete | Non-blocking | Only PAUSE, CANCEL, RETRY_DISPATCH samples; missing APPROVE, RESUME, `INVALID_STATE`, `CONFLICT`, control idempotency replay |
| No reconnect/event-cursor fixture | Non-blocking | `after_event_id`/`after_sequence` not fixture-tested |
| `providerSessionRefs` absent from dispatch fixtures | ✅ **Closed** (B12) | `javascript-running-n-dispatch` dispatch rows include `providerSessionRefs` |
| List filters not fixture-tested | Non-blocking | Service tests cover filters; fixtures only exercise `scope` |

**Aligned:** 11-scenario matrix (adds `javascript-awaiting-approval`; Petri + JavaScript; running, paused, failed-with-partial, timed-out, canceled, succeeded, unsupported-runner, missing-source), per-scenario `executionRequest`/`session`/`listSummary`/`dispatches`/`result`/`events[]` coverage where applicable, `idempotentReplay` block, `listResponse` with `scope: all`, lifecycle control samples on three scenarios, host-path omission guard, and OpenAPI validate/round-trip in `generated_contract_durable_session_test.go`.

### Cross-cutting summary

| Mismatch | Class | Primary surfaces |
|----------|-------|------------------|
| HTTP handlers 501 / empty persisted list | Stubbed transport | `handlers_factory.go` ↔ OpenAPI ↔ service |
| No `factorysessionexecution.Service` wired into API server | Stubbed transport | `handlers_factory.go` |
| Workflow source resolution documented but not in execution service start path | ✅ **Closed** (B8) | `start_source.go` ↔ `pkg/orchestrators/javascript/source` |
| `GET /factory-sessions/{id}` union vs handler returns live shape only | Stubbed transport (wire-up) | Batch 002 `API-2` handler routing |
| `SessionCompleted` + fake events use wrong status model / invalid envelope | ✅ **Closed** (B1/B3) | OpenAPI events ↔ fake ↔ apisurface |
| Result status enums differ (event vs REST) | ✅ **Closed** (B2) | OpenAPI events ↔ `FactorySessionResult` |
| `budgets`/`usage` on durable read model — no service/mapper fields | ✅ **Closed** (B11) | OpenAPI ↔ service ↔ apisurface |
| `providerSessionRefs` on dispatches — no service/mapper/fixture | ✅ **Closed** (B12) | OpenAPI ↔ service ↔ apisurface ↔ fixtures |
| `ControlErrorToAPI` missing required `status` | ✅ **Closed** (B6) | OpenAPI ↔ apisurface |
| Control idempotency conflict has no distinct OpenAPI error code | Non-blocking (B7 downgraded) | Lifecycle `CONFLICT` outcome on control response |
| Event reconnect params validated but not enforced | ✅ **Closed** (B9) | OpenAPI ↔ `ReadEvents` |
| `GetResult` ignores `mode`/`includeArtifacts` | ✅ **Closed** (B10) | OpenAPI ↔ service |
| Execution links vs lifecycle control links shape drift | Non-blocking | OpenAPI ↔ service ↔ apisurface |
| List filter model in service without OpenAPI params | Non-blocking | OpenAPI ↔ service |
| Fixture gaps (STILL_RUNNING, interrupted in JSON, expanded lifecycle matrix) | Non-blocking | Remaining fixture catalog expansion during Batch 002 |

Stubbed transport gaps are **expected Batch 002 wiring targets**, not permission to redefine schemas. Blocking items B1–B12 are closed or downgraded as of 2026-06-11 UTC contract-repair merge.

## Batch 002 go/no-go recommendation

**Recommendation: Go** — schedule Batch 002 fake-session skeleton work as the
immediate next batch. Contract-repair kernel merged 2026-06-11 UTC on branch
`dynamic-workflows-contract-repair-kernel-resubmit`; active-surface repair
verified 2026-06-11 UTC on branch
`dynamic-workflows-contract-repair-active-surface-followup`.

### Evidence

The Batch 001 completion checklist shows contract surfaces landed (✅) while durable
transport remains stubbed (🔌). Contract repair closed blocking mismatches B1–B6,
B8–B12 at the schema/projection layer and downgraded B7 (control `requestId`
conflict uses lifecycle `CONFLICT` outcome, not HTTP `ErrorResponse.code`).
Active-surface repair then closed the remaining planner gate: no scoped path treats
standalone `POST /workflow-previews` / `WorkflowPreview` or root `pkg/workflow*`
packages as the primary preview or JavaScript orchestration owner.

Focused verification passes:

- `make generate-api` (generated artifacts synchronized)
- `go test ./pkg/api/contracttests ./pkg/api/servertests ./pkg/apisurface ./pkg/apisurface/factorysession ./pkg/factorysessionexecution ./pkg/factory/sessions ./pkg/mcp/workflow ./pkg/cli/workflow`
- `go test ./pkg/factory/projections/projectiontests ./pkg/factory/events ./pkg/factory/validation`
- `npm --prefix ui run typecheck`
- Scoped `rg` verification over `api/openapi-main.yaml`, `api/components`, `pkg/api`,
  `pkg/apisurface`, `pkg/cli`, `pkg/mcp`, `pkg/factorysessionexecution`,
  `pkg/factory/sessions`, `pkg/orchestrators/javascript`, and `ui/src` reports only
  generated compatibility aliases, deprecated OpenAPI compatibility routes/schemas,
  and explicit obsolete/compatibility tests or UI wrappers.

Batch 002 skeleton consumers can now prove round-trip through apisurface mappers and
contract fixtures, emit/consume canonical `FactoryEvent` payloads, and wire handlers
to `factorysessionexecution.Service` without redefining schemas or depending on
obsolete workflow-preview ownership.

### Blocking findings (contract-repair closure — 2026-06-11 UTC)

| # | Finding | Status | Evidence |
|---|---------|--------|----------|
| B1 | `SessionCompletedEventPayload.finalStatus` durable lifecycle enum | ✅ Closed | OpenAPI + `event_history_session_lifecycle.go`, projection tests |
| B2 | `FactoryEventSessionResultStatus` includes `NOT_READY` / `UNAVAILABLE` | ✅ Closed | OpenAPI `x-enum-varnames`, contract tests |
| B3 | Fake emits canonical `FactoryEvent` envelopes | ✅ Closed | `canonical_events.go`, `canonical_events_test.go` |
| B4 | Contract fixtures include canonical `events[]` | ✅ Closed | `durable-session-contract-fixtures.json`, `generated_contract_durable_session_test.go` |
| B5 | `AWAITING_APPROVAL` fixture scenario | ✅ Closed | `javascript-awaiting-approval` scenario |
| B6 | `ControlErrorToAPI` includes required `status` | ✅ Closed | `factory_session_mapper.go`, mapper tests |
| B7 | Control `requestId` conflict OpenAPI `ErrorResponse.code` | ⬇️ Downgraded | Lifecycle `CONFLICT` outcome on control response; distinct HTTP error code deferred |
| B8 | Workflow source resolution under orchestrator ownership | ✅ Closed | `start_source.go`, `pkg/orchestrators/javascript/source`, `start_source_test.go` |
| B9 | `ReadEvents` reconnect cursor filtering | ✅ Closed | `FilterEventsAfterReconnect`, fake-service tests |
| B10 | `GetResult` honors `mode` / `includeArtifacts` | ✅ Closed | `result_projection.go`, `result_projection_test.go` |
| B11 | `budgets` / `usage` on durable read model | ✅ Closed | Service types + `SessionReadResponseToAPI`, fixture round-trips |
| B12 | `providerSessionRefs` on dispatches | ✅ Closed | Service + mapper + fixture `javascript-running-n-dispatch` |

Durable `GET /factory-sessions/{id}` union routing remains a **stubbed transport**
Batch 002 `API-2` target — schema and mappers are ready.

### Non-blocking follow-ups (safe to carry into Batch 002 after repair)

| Finding | Class | Carry-forward rationale |
|---------|-------|-------------------------|
| Durable handlers return `501` / empty persisted listings | Stubbed transport | Expected Batch 001 state; Batch 002 wiring target after contracts stabilize |
| `FactorySessionExecutionLinks` lacks `dispatches`/`artifacts` vs lifecycle control links | Non-blocking | Asymmetric link shapes are documented; skeleton can start with narrower start links |
| Service list filters richer than OpenAPI list params | Non-blocking | Filters work in service tests; API query params can land later |
| `RETRY_DISPATCH` fake vs service rule drift on active sessions | Non-blocking | Fake behavior can align during skeleton without schema change |
| Dual phase event models (`ORCHESTRATOR_PHASE_CHANGED` vs `JAVASCRIPT_PHASE_CHANGE`) | Non-blocking | Both in schema oneOf; client guidance can follow skeleton |
| Interrupted/recoverable, `STILL_RUNNING` sync, incomplete lifecycle control fixture matrix, reconnect cursor fixtures, list-filter fixtures | Non-blocking | Expand fixture catalog during or after skeleton; not schema blockers |
| `EventReadResponseToAPI` / `SyncStartResponseToAPI` silent drop on unmarshal failure | Non-blocking | Tighten once canonical events land (B3/B4) |

### Smallest contract-repair batch (before Batch 002 skeleton)

Schedule one focused batch — **not** a broad refactor — with this minimum scope:

1. **Preview and JavaScript orchestrator ownership repair:** align public preview
   behavior with **Factory preview** or **Factory Session preview** semantics;
   retire or remap the standalone `POST /workflow-previews` surface; move
   validation, source loading, storage, policy, preview preparation, and result
   validation under `pkg/orchestrators/javascript` subpackages instead of root
   `pkg/workflow*` transitional packages.
2. **OpenAPI repairs:** align `SessionCompletedEventPayload.finalStatus` and
   `FactoryEventSessionResultStatus` with durable REST enums; add distinct
   `FACTORY_SESSION_CONTROL_REQUEST_ID_CONFLICT` (or equivalent) for control
   `requestId` reuse with different tuples.
3. **Fake + fixtures:** rewrite `deriveProjectionEvents` to emit canonical envelopes;
   add `events[]` to fixture scenarios; add `AWAITING_APPROVAL` scenario.
4. **Service seams:** call workflow source resolution from start path; enforce
   `ReadEvents` cursor filtering; honor `GetResult` `mode`/`includeArtifacts`; add
   `budgets`/`usage` and `providerSessionRefs` to projections.
5. **Mappers:** map new fields; fix `ControlErrorToAPI` required `status`; regenerate
   contracts if OpenAPI changes.
6. **Verification:** extend contract/fake-consumer tests to prove event and durable-read
   round-trips for at least running, `AWAITING_APPROVAL`, and terminal scenarios.

Estimated posture: **one vertically sliced contract batch** touching the five surfaces
already named in this plan — smaller than Batch 002 skeleton (which also spans
CLI/MCP/UI) but mandatory so skeleton work does not encode broken contracts.

### Re-entry criteria for Batch 002 (Go)

**Met 2026-06-11 UTC** after contract-repair kernel merge and active-surface repair
verification:

- Blocking table B1–B12 rows are closed or explicitly downgraded (B7).
- Active-surface repair gate passed: canonical Factory preview semantics, orchestrator-owned
  JavaScript helpers, aligned API/CLI/MCP/UI consumers, synchronized generated contracts, and
  scoped `rg` verification with only compatibility/obsolete hits.
- `durable-session-contract-fixtures.json` includes canonical `events[]` for representative
  scenarios and an `AWAITING_APPROVAL` row.
- Fake-service + mapper round-trip tests pass for durable session get, result, dispatch,
  artifact, event, and lifecycle control paths without schema workarounds.
- Transport stubs (501 handlers, empty persisted listing, durable GET routing) remain the
  **only** deliberate gaps — wired in Batch 002 skeleton against the repaired contract kernel.

### Product behavior goals (in scope for the program)

- Customers can define JavaScript workflow factories and start them as durable
  `FactorySession` executions with explicit orchestrator identity.
- Session status, partial/final results, dispatches, artifacts, and lifecycle
  controls are observable through API, CLI, MCP, dashboard, and events with
  matching semantics.
- Workflow sources resolve through a shared lookup order; validation and
  read-only policy defaults reject unsafe or malformed input before execution.
- Event streams bracket one session execution and support reconnect/replay
  without duplicating applied facts.
- Contract fixtures and fake-service projections give reviewers deterministic
  scenarios (running, terminal, partial, failed, canceled, interrupted) before
  real runtime exists.

### Out of scope for current batches (explicit non-goals)

The following are **not** goals of Batch 001 retro documentation or of Batch
002 skeleton work unless a later batch explicitly schedules them:

- **Real JavaScript execution** in a production VM/sandbox (runtime lane).
- **Durable persistence stores** and real persisted-session listing backends
  (persistence lane).
- **MCP tool wiring** beyond contract-level planning shared with API/CLI.
- **CLI command implementation** and **dashboard UI screens** beyond skeleton
  flows needed to exercise the service contract.
- **Provider dispatch bridges** and live external tool sessions (dispatch-bridge
  lane).
- **Broad refactors** of unrelated packages, route registration cleanup, or
  Petri-runtime rewrites not required for dynamic-workflow contracts.
- Introducing **`DynamicWorkflowRun`** or parallel session nouns in OpenAPI,
  events, or operator docs.

Stubbed transport behavior (for example `501 NotImplemented` handlers or empty
persisted rows) is an **implementation gap**, not a license to change Batch
001 schema or service semantics. Later stories in this retro document separate
transport stubs from true cross-surface contract conflicts.

### Granular parallel execution plan

The original Batch 002 shape is too wide for maximum throughput: API handler
wiring, MCP tools, CLI commands, website components, host integration files,
and contract repair do not all have the same dependencies. The code comparison
on 2026-06-09 shows:

- `factorysessionexecution.Service`, deterministic fake scenarios, OpenAPI
  durable route shapes, generated Go/TypeScript types, and mapper packages exist.
- Durable API handlers in `pkg/transports/http/handlers_factory.go` still return `501` for
  async/sync start, results, dispatches, artifacts, lifecycle controls, and
  durable events; `scope=persisted|all` returns empty durable rows.
- `pkg/transports/mcp/workflow/` exposes preview/start-validation through the canonical Factory
  preview seam; session/dispatch/artifact MCP tools depend on deferred Batch 002
  skeleton wiring.
- `ui/src/api/factory-sessions/api.ts` and the current factory-session detail
  panel are live-session oriented; generated durable types are present for
  fixture-backed adapters and components before HTTP wiring exists.
- `pkg/transports/http/testdata/durable-session-contract-fixtures.json` now includes canonical
  `events[]` arrays and an `AWAITING_APPROVAL` scenario from contract-repair kernel
  work; remaining event/control tranches are Batch 002 skeleton wiring targets.

Use the following graph to schedule smaller work items. `CR-*` closes contract
drift; `API-*`, `MCP-*`, `UI-*`, and `CLI-*` can advance as soon as their
specific inputs exist rather than waiting for a monolithic fake-session batch.

```mermaid
flowchart TB
  K[Batch 001 contract kernel complete]

  subgraph CR[Contract repair and fake fidelity]
    CR1[CR-1 event enums and terminal status alignment]
    CR2[CR-2 canonical FactoryEvent envelopes in fake service]
    CR3[CR-3 fixture events plus AWAITING_APPROVAL scenario]
    CR4[CR-4 service behavior: source resolution, result mode, event cursors]
    CR5[CR-5 mapper field repair: control status, provider session refs, read-model fields]
    CRQ[CR-Q contract round-trip gate]
  end

  subgraph API[API fake transport]
    API0[API-0 inject factorysessionexecution.Service into server]
    API1[API-1 async and sync start handlers with idempotency errors]
    API2[API-2 get/list durable sessions with live and durable routing]
    API3[API-3 result, dispatch, and artifact read handlers]
    API4[API-4 lifecycle control handlers]
    API5[API-5 durable session event read or stream handler]
    APIQ[API-Q handler contract and fixture tests]
  end

  subgraph MCP[MCP and host integration]
    MCP0[MCP-0 canonical session/dispatch/artifact tool schemas from generated contract]
    MCP1[MCP-1 mock MCP client tests for discovery and validation]
    MCP2[MCP-2 session start/list/status/result tools against HTTP client]
    MCP3[MCP-3 dispatch/artifact/event/control tools]
    MCP4[MCP-4 host integration directories and docs]
    MCPQ[MCP-Q real API smoke against fake service]
  end

  subgraph UI[Website tranches]
    UI0[UI-0 durable type guards, API adapter tests, fixture loaders]
    UI1[UI-1 session list accepts live plus durable summaries]
    UI2[UI-2 durable detail shell: status, source, policy, lifecycle]
    UI3[UI-3 result, dispatch, artifact panels from fixtures]
    UI4[UI-4 event timeline and lifecycle controls]
    UIQ[UI-Q integration refresh against wired fake API]
  end

  subgraph CLI[CLI tranches]
    CLI0[CLI-0 command contract and output shapes]
    CLI1[CLI-1 session run/list/show/result against durable endpoints]
    CLI2[CLI-2 dispatch/artifact/event/control commands]
    CLIQ[CLI-Q CLI smoke against fake API]
  end

  subgraph RT[Runtime and later verticals]
    RT0[Runtime shell and host API stubs]
    RT1[In-memory session runner]
    RT2[Single-agent fixture parity pass]
    F0[Fanout phases]
    D0[Dispatch bridge]
    P0[Persistence inspection]
    R0[Resume checkpoints]
    H0[Policy and failure hygiene]
    REL[Release hosts]
    BETA[Customer beta readiness]
  end

  K --> CR1
  K --> CR4
  K --> CR5
  CR1 --> CR2
  CR2 --> CR3
  CR3 --> CRQ
  CR4 --> CRQ
  CR5 --> CRQ

  K --> API0
  CRQ --> API1
  API0 --> API1
  API0 --> API2
  CR5 --> API2
  API1 --> API3
  CR4 --> API3
  CR5 --> API3
  API2 --> API4
  CR3 --> API5
  API2 --> API5
  API3 --> APIQ
  API4 --> APIQ
  API5 --> APIQ

  K --> MCP0
  K --> MCP1
  K --> MCP4
  API1 --> MCP2
  API2 --> MCP2
  API3 --> MCP3
  API4 --> MCP3
  API5 --> MCP3
  MCP2 --> MCPQ
  MCP3 --> MCPQ

  K --> UI0
  K --> UI1
  K --> UI2
  CR3 --> UI3
  CR5 --> UI3
  CR3 --> UI4
  API2 --> UIQ
  API3 --> UIQ
  API4 --> UIQ
  API5 --> UIQ
  UI1 --> UIQ
  UI3 --> UIQ
  UI4 --> UIQ

  K --> CLI0
  API1 --> CLI1
  API2 --> CLI1
  API3 --> CLI2
  API4 --> CLI2
  API5 --> CLI2
  CLI1 --> CLIQ
  CLI2 --> CLIQ

  APIQ --> RT0
  MCPQ --> RT2
  UIQ --> RT2
  CLIQ --> RT2
  RT0 --> RT1
  RT1 --> RT2
  RT2 --> F0
  F0 --> D0
  F0 --> P0
  D0 --> R0
  P0 --> R0
  R0 --> H0
  MCP4 --> REL
  H0 --> REL
  REL --> BETA
```

#### Suggested micro-batches

| ID | Smallest independently testable behavior | Can start when | Primary evidence |
|----|------------------------------------------|----------------|------------------|
| CR-1 | Event terminal/result enums match durable REST status vocabulary. | Batch 001 complete | OpenAPI contract tests and generated type diff. |
| CR-2 | Fake service emits canonical `FactoryEvent` envelopes with context and payload identity boundaries. | CR-1 | Fake-service event tests and `EventReadResponseToAPI` round-trip. |
| CR-3 | Durable fixture catalog includes `events[]` and `AWAITING_APPROVAL`. | CR-2 | Fixture validation for running, approval, terminal, and failed-with-partial scenarios. |
| CR-4 | Fake/service reads honor source resolution, result mode/includeArtifacts, and event reconnect cursors. | Batch 001 complete | Service unit tests over `pkg/orchestrators/javascript/source` (replacing the removed transitional root `pkg/workflowsource/` **stale Batch 001**), result projection, and cursor filtering. |
| CR-5 | Mappers preserve required lifecycle, provider-correlation, and read-model fields. | Batch 001 complete | Mapper round-trip tests against fixtures. |
| API-0 | Server can receive an injectable durable execution service without changing route contracts. | Batch 001 complete | Server construction tests still pass with nil/live defaults and fake injection. |
| API-1 | Async/sync start routes return fake sessions and idempotency conflicts. | CR-Q and API-0 | Handler tests for accepted, completed, timed-out, and conflict outcomes. |
| API-2 | `GET` and list route durable rows coexist with live-session compatibility. | CR-5 and API-0 | Handler tests for `scope=live|persisted|all` and durable id lookup. |
| API-3 | Result, dispatch, and artifact reads map through the service. | API-1 plus CR-4/CR-5 | Handler tests for not-ready, partial, final, failed, and missing resources. |
| API-4 | Lifecycle controls map approve/pause/resume/cancel/terminate/retry outcomes. | API-2 plus CR-5 | Handler tests for accepted, no-op, invalid-state, terminal, replay, and conflict. |
| API-5 | Durable event reads honor reconnect cursors and canonical envelopes. | CR-3 plus API-2 | Event route tests for initial history and after-event/sequence replay. |
| MCP-0 | Canonical MCP tool schemas exist for session, dispatch, and artifact nouns. | Batch 001 complete | Schema/discovery tests using generated OpenAPI types; no live server required. |
| MCP-1 | Mock client validates preview/start error shape and tool discovery. | Contract repair complete | Mock MCP tests over repaired Factory/Factory Session preview surfaces and new session tool catalog. |
| MCP-2 | MCP start/list/status/result tools call the current HTTP durable endpoints. | API-1 and API-2 | Mock HTTP tests first; real fake-server smoke after API wiring. |
| MCP-3 | MCP dispatch/artifact/event/control tools match API behavior. | API-3, API-4, API-5 | Mock and fake-server parity tests. |
| MCP-4 | Host integration directories and packaging manifests exist. | Batch 001 complete | File inventory tests and release-archive inclusion checks; tool implementation can follow. |
| UI-0 | Website durable-session adapters and guards accept generated durable shapes. | Batch 001 complete | Type-level/API tests with durable fixtures; no server required. |
| UI-1 | Session list can render live and durable summaries together. | Batch 001 complete | Component tests using `scope=all` fixture responses. |
| UI-2 | Durable detail shell renders status, source, policy, lifecycle, and actions. | Batch 001 complete | Component tests from `FactorySessionDurableReadModel` fixtures. |
| UI-3 | Result, dispatch, and artifact panels render fake durable data. | CR-3 and CR-5 | Component and adapter tests over repaired fixtures. |
| UI-4 | Event timeline and lifecycle controls render canonical events/outcomes. | CR-3 | Component/hook tests with canonical event arrays and control responses. |
| CLI-0 | Durable command names, JSON output shape, and help text stay on factory/session nouns. | Batch 001 complete | CLI command tests without HTTP execution. |
| CLI-1 | Durable run/list/show/result commands call fake API endpoints. | API-1 and API-2 | CLI HTTP tests for start/list/show/result. |
| CLI-2 | Dispatch/artifact/event/control commands call fake API endpoints. | API-3, API-4, API-5 | CLI HTTP tests for each inspection/control path. |
| PAR-Q | Cross-surface parity test proves one fake session is explainable everywhere. | API-Q, MCP-Q, UI-Q, CLI-Q | API, CLI, MCP, and UI all show same session id, status, result, dispatches, artifacts, and events. |

#### Tranche cadence

Each tranche should converge on one fake scenario before adding a broader
scenario matrix. This keeps website and MCP work unblocked while still forcing
realignment after API/service implementation catches up.

```mermaid
sequenceDiagram
  participant Contract as Contract/Fixtures
  participant API as API Fake Transport
  participant MCP as MCP Tools
  participant UI as Website
  participant CLI as CLI
  participant Gate as Parity Gate

  Contract->>MCP: generated schemas and fixture examples
  Contract->>UI: durable generated types and fixture examples
  Contract->>CLI: command/output contract examples
  Contract->>API: repaired mapper/service expectations
  API->>MCP: fake HTTP start/list/status/result paths
  API->>UI: fake HTTP list/detail/result paths
  API->>CLI: fake HTTP command paths
  MCP->>Gate: tool discovery and fake-session smoke
  UI->>Gate: fixture component tests, then fake-server integration
  CLI->>Gate: command HTTP smoke
  API->>Gate: handler contract tests
  Gate-->>Contract: contract drift or missing scenario feedback
```

#### Scheduling guidance

- **Immediate next phase:** schedule **API-0 through API-5** and parallel
  CLI/MCP/UI skeleton tranches. Contract repair (CR-1 through CR-5) merged
  2026-06-11 UTC; **CR-Q** gate satisfied.
- **Active work:** fake-session skeleton wiring (`API-*`, `CLI-*`, `MCP-*`,
  `UI-*`) against the repaired contract kernel.
- Treat **CR-Q** as the gate for all handler and cross-surface parity claims.
- Split API work by route family. `API-1` and `API-2` unblock MCP/CLI start and
  list/status work; `API-3` unblocks result/dispatch/artifact panels and tools;
  `API-4` and `API-5` can follow independently.
- Put a parity gate after every tranche: first running/succeeded, then
  not-ready/partial, then failed-with-partial, then awaiting approval and
  lifecycle controls, then interrupted/reconnect.
- Do not start real runtime, persistence, dispatch bridge, or resume work until
  at least one fake session vertical passes `PAR-Q`; otherwise later lanes will
  debug transport drift and runtime behavior at the same time.

### Decision context for the next batch

Maintainers use this plan to decide which **micro-batches** can proceed in
parallel and which require a contract-repair gate before handler or parity
claims. The old Batch 002 label remains useful as a milestone name, but the
actual queue should be scheduled as the granular graph above.

**Current decision (2026-06-11 UTC):** **Go** for Batch 002 fake-session
skeleton. Contract-repair kernel (`dynamic-workflows-contract-repair-kernel-resubmit`)
closed B1–B12 (B7 downgraded). Schedule **API-0 through API-5** and parallel
CLI/MCP/UI skeleton tranches per the granular graph above. **CR-Q** gate is
satisfied; transport stubs from the checklist are the remaining wiring targets.

The **Batch 001 completion checklist** remains the authoritative record of what
merged in PRs #767–#776. Batch 001 plus contract-repair is **contract-complete**
at the interface-definition and fake-projection layer; handler parity claims
require Batch 002 skeleton wiring.

### Related references

- `docs/reference/orchestrators.md` — canonical nouns and dynamic-workflow aliases
- `docs/reference/sessions.md` — live session discovery and CLI routing
- `pkg/factorysessionexecution/` — shared durable session execution service
- `pkg/transports/mapping/factorysession/` — OpenAPI ↔ service mappers
- `pkg/transports/http/testdata/durable-session-contract-fixtures.json` — contract fixtures
