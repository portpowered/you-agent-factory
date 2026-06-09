# Dynamic Workflows v0 — Program Plan

Operator-facing planning record for the Dynamic Workflows v0 program. This
document tracks batch completion, cross-surface contract posture, and the
recommended next batch for maintainers scheduling factory work.

**Last updated:** 2026-06-09 (Batch 001 retro — completion checklist)

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
  fixtures in `pkg/api/testdata/durable-session-contract-fixtures.json`.

Batch 001 intentionally landed **contracts and projections**, not full transport
wiring or real runtime execution. Handlers may still return `501 NotImplemented`
or empty persisted listings until later batches wire transports and stores.

#### Batch 002 — fake-session skeleton (planned)

Batch 002 targets **deterministic fake-session skeleton work**: API/CLI/MCP/UI
surfaces that call the shared `factorysessionexecution.Service` (including the
injectable fake implementation) so reviewers can exercise durable session
start, status, result, dispatch, artifact, event, and lifecycle flows without
a JavaScript VM or durable persistence backend.

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
| Generated contracts and fixture hooks | ✅ | `pkg/api/generated/` and UI generated types refreshed; contract tests can bind to OpenAPI examples. |
| Durable route handlers | 🔌 | `StartDurableFactorySessionAsync`, `StartDurableFactorySessionSync`, durable result/dispatch/artifact reads, and lifecycle controls in `pkg/api/handlers_factory.go` return **`501 NotImplemented`**. |
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
| `POST /workflow-previews` handler | ✅ | `PreviewWorkflow` in `pkg/api/handlers_factory.go` runs the shared preview contract (validation + policy projection) without starting a session. |
| Durable start-time validation wiring | 🔌 | Validation/policy contracts exist; **`POST /factory-sessions/async|sync`** handlers still return **`501`** so start-time enforcement awaits Batch 002 service injection. |

### PR #776 — shared execution service, fake service, mappers, and fixtures

**Merged:** 2026-06-09 · [contract-service-seams](https://github.com/portpowered/you-agent-factory/pull/776)

| Item | Status | Completion notes |
|------|--------|------------------|
| `factorysessionexecution.Service` interface | ✅ | Start (async/sync), read/status, result/dispatch/artifact projection, lifecycle controls, listing scopes, and idempotency are defined in `pkg/factorysessionexecution/`. |
| Deterministic fake service | ✅ | Injectable fake implements the same contract with stable scenario ids, JavaScript orchestrator projections, dispatch lists, result states, artifact refs, and event sequences (`fake_service.go`, `fake_fixture.go`). |
| `apisurface` mappers | ✅ | `pkg/apisurface/factorysession/` round-trips OpenAPI execution requests/responses, session records, results, dispatches, artifacts, and lifecycle payloads. |
| Durable session contract fixtures | ✅ | `pkg/api/testdata/durable-session-contract-fixtures.json` covers Petri and JavaScript scenarios (running, partial, final, failed-with-partial, canceled, timed-out, interrupted, multi-dispatch). |
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

Cross-surface **schema or projection conflicts** (if any) are inventoried separately in the gap inventory section added by the next retro story.

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

### Decision context for the next batch

Maintainers use this plan to decide whether **Batch 002 fake-session skeleton**
work can proceed or whether a **contract-repair batch** must close blocking
gaps across OpenAPI, `factorysessionexecution.Service`, apisurface mappers,
`FactoryEvent` payloads, and durable session fixtures first.

Subsequent sections of this document (added in follow-on retro stories) will
record:

- A cross-surface conflict/gap inventory with blocking vs non-blocking
  classification.
- An explicit go/no-go recommendation for Batch 002.

The **Batch 001 completion checklist** above is the authoritative record of
what merged in PRs #767–#776. Treat Batch 001 as **contract-complete at the
schema and service-interface layer** and Batch 002 as **blocked only by
documented contract conflicts** in the gap inventory, not by the transport stubs
listed in the checklist.

### Related references

- `docs/reference/orchestrators.md` — canonical nouns and dynamic-workflow aliases
- `docs/reference/sessions.md` — live session discovery and CLI routing
- `pkg/factorysessionexecution/` — shared durable session execution service
- `pkg/apisurface/factorysession/` — OpenAPI ↔ service mappers
- `pkg/api/testdata/durable-session-contract-fixtures.json` — contract fixtures
