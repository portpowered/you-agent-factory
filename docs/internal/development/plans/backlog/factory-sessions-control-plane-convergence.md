# Factory Sessions Control-Plane Convergence Plan

## Status

Proposed.

## Problem statement

The process currently constructs and exposes several overlapping ways to open,
start, invoke, inspect, and control a Factory Session. `Factory Sessions` also
hosts compatibility entrypoints for canonical Recording reads and retains
secondary runtime/application-opening graphs that inject execution dependencies
after `root.BuildProcess` has already constructed the process.

As a result, the same customer operation can reach different constructors,
service facets, and state paths. The distinction between live and durable
execution has leaked into method families even though both represent the same
public `Factory Session` resource.

## Customer ask

Construct every process-scoped service once through `root.BuildProcess`. Treat
the resulting process as a collection of service roots invoked by HTTP, CLI,
ACP, MCP, and other transports. Remove secondary runtime opening, application
opening, execution opening, and on-demand application construction.

Keep `Factory Sessions` as the public control plane for starting, invoking,
inspecting, and controlling Factory Sessions. Do not expose Factory Runtime's
data-plane operations directly to transports or customers. Move canonical
Recording event and artifact reads to `Recordings`, collapse redundant live and
durable Session entrypoints, and explicitly settle dispatch ownership without
creating a second dispatch store.

## Intended outcome

A process has one constructed instance of each product service. Starting a
Factory Session creates session-scoped state inside those already-constructed
services; it does not construct another application or service graph.

Customers and transports observe one Factory Sessions vocabulary regardless of
persistence or hosting mode:

- `Start` creates a Factory Session;
- `Invoke` submits an invocation to an existing Factory Session;
- `Get` and `List` return unified session projections;
- `Control` evaluates session and dispatch control intents;
- `ReadResult` selects complete or partial session results; and
- `SubscribeResponses` exposes only the ephemeral response stream owned by
  Factory Sessions.

Factory Runtime remains an internal data-plane dependency of Factory Sessions.
Recordings owns canonical Factory Event, replay, and artifact read contracts.
Dispatch remains a customer-visible child of a Factory Session: Factory
Sessions owns its customer-facing query and control semantics, Recordings owns
the canonical dispatch facts, and Factory Runtime privately performs execution
mechanics.

## Original documents and normative inputs

- `C:\Users\andre\work\portos\infinite-you\.claude\worktrees\test-cleanup\docs\internal\standards\code\planning-standards.md`
- `C:\Users\andre\work\portos\infinite-you\.claude\worktrees\test-cleanup\docs\internal\standards\code\code-review-standards.md`
- `C:\Users\andre\work\portos\infinite-you\.claude\worktrees\test-cleanup\docs\internal\standards\code\general-backend-standards.md`
- `C:\Users\andre\work\portos\infinite-you\.claude\worktrees\test-cleanup\docs\architecture\architecture.md`
- `C:\Users\andre\work\portos\infinite-you\.claude\worktrees\test-cleanup\docs\architecture\structures.md`
- `C:\Users\andre\work\portos\infinite-you\.claude\worktrees\test-cleanup\docs\architecture\data-model.md`
- `C:\Users\andre\work\portos\infinite-you\.claude\worktrees\test-cleanup\docs\architecture\packaged-structure.md`
- `C:\Users\andre\work\portos\infinite-you\.claude\worktrees\test-cleanup\docs\architecture\service-ownership-rationale.md`
- `C:\Users\andre\work\portos\infinite-you\.claude\worktrees\test-cleanup\docs\internal\development\plans\archive\08-20\packaged-service-structure\package-service-factory-sessions.md`

The archived Factory Sessions plan is background evidence, not the target
authority for this plan. In particular, this plan supersedes its proposal that
transports call Factory Runtime observation directly.

## Architectural decisions

### AD-1: `root.BuildProcess` is the only application construction boundary

`root.BuildProcess` and canonical `pkg/wire` providers construct all
process-scoped services and inject their complete dependencies. A transport can
select and call a service from the built process, but it cannot open another
application, runtime service graph, or execution service.

Per-session records, runtime instances, Recording scopes, subscriptions,
Worker Sessions, leases, and attempts are domain resources created by methods
on already-constructed services. They are not newly injected services.

### AD-2: Factory Sessions is the public control plane

Factory Sessions accepts customer-level commands and queries. Its private
implementation may coordinate Factory Definitions, Work, Factory Runtime,
Recordings, Events, Worker Sessions, and other injected services. It returns
Factory Sessions-owned values and opaque identities, never runtime engines,
bindings, executors, gateways, service objects, or internal Petri values.

Factory Runtime remains process-scoped but private to application services. No
HTTP, CLI, ACP, or MCP endpoint receives Factory Runtime merely to implement a
Factory Session operation.

### AD-3: Recordings owns canonical history and artifact reads

Recordings owns durable Factory Event reads, reconnect cursors, replay,
canonical artifacts, and artifact retrieval. Transports serving those
operations receive `recordings.Service` directly from the process. Factory
Sessions may return links to those resources but does not implement a second
ledger or artifact repository.

Factory Sessions continues to own ephemeral `FactoryResponseEvent` retention
and subscription because those events represent the immediate session response
channel rather than canonical replay history.

### AD-4: Dispatch uses a hybrid ownership boundary

A Dispatch is customer-visible in the context of a Factory Session, so the
public session control plane may retain:

- list and get dispatches for a Factory Session;
- approve a waiting dispatch or session policy boundary;
- retry a failed dispatch; and
- interrupt an active dispatch.

This does not authorize Factory Sessions to own a second dispatch state store.
The responsibilities are:

| Responsibility | Owner |
| --- | --- |
| Customer request validation, session scoping, action availability, and response vocabulary | Factory Sessions |
| Canonical dispatch facts, transitions, history, and replay projection | Recordings |
| Scheduling, retry creation, interruption mechanics, and active execution state | Factory Runtime, privately invoked by Factory Sessions |
| Worker-attempt interruption and lifecycle | Worker Sessions, coordinated by the private data plane |

Before the legacy dispatch methods are migrated, story FSCP-01 must prove
whether their current results are historical projections, live runtime reads,
or a merge of both. Story FSCP-06 then establishes one projection rule. The
default target is a Recordings-backed canonical projection with narrowly
identified transient fields supplied through the private data plane only when
they cannot yet be represented by canonical events.

### AD-5: narrow interfaces are views, not additional services

A consumer may declare the smallest interface it needs, and the single
Factory Sessions root may satisfy that interface structurally. Such an
interface does not have a constructor, registry entry, opener, mutable binding,
or independent lifecycle.

`DurableExecutionService`, `TargetExecutionService`, `LiveControlService`, and
other mode-specific owner-published facets are not retained as separately
constructible services.

## Ownership matrix

| Operation or data | Public owner | Internal collaboration |
| --- | --- | --- |
| Start, invoke, recover, get, list, session lifecycle, session result | Factory Sessions | Definitions, Work, Runtime, Recordings, Events |
| Ephemeral response events | Factory Sessions | Events where needed for process-local delivery |
| Named Factory catalog selection and immutable definition resolution | Factory Definitions | Sessions receives resolved identity/value |
| Apply a resolved Factory version to a session | Factory Sessions | Private Factory Runtime activation |
| Canonical Factory Events, reconnect, replay | Recordings | None through Sessions |
| Canonical artifacts and artifact retrieval | Recordings | Sessions may expose references/links |
| Customer-facing dispatch query and control | Factory Sessions | Recordings facts; private Runtime mechanics |
| Work admission, materialization, movement, and Work reads | Work | Work commands the appropriate data plane without a Sessions runtime back-query |
| Runtime observation used to build a session projection | Factory Sessions | Private Factory Runtime query |
| Worker Session observation and transcripts | Worker Sessions | Session projections use explicit IDs, not returned observation services |
| Resource and workstation controls scoped to a session | Factory Sessions | Private Runtime mechanics and canonical Recording events |

## Current-state evidence

The following snapshot was measured on August 24, 2026. Counts are the number
of Go files under `pkg`, `cmd`, and `tests` containing the symbol; they indicate
migration breadth, not behavioral test coverage.

| Legacy surface | Files |
| --- | ---: |
| `StartAsync` | 78 |
| `StartSync` | 62 |
| `OpenFactorySession` | 63 |
| `OpenFactorySessionFromFolder` | 11 |
| `GetSession` | 112 |
| `GetFactorySession` | 63 |
| `ListSessions` | 76 |
| `ListFactorySessions` | 58 |
| `DurableExecutionService` | 15 |
| `TargetExecutionService` | 15 |
| `runtimeopening` | 42 |
| `applicationopening` | 5 |
| `executionopening` | 12 |

Additional structural evidence:

- `factory_sessions/internal/sessionservice/execution.go` forwards nineteen
  methods to a separately attached durable execution service.
- `pkg/wire` still builds runtime-opening requests, application openers,
  execution-opening factories, runtime assembly callbacks, and HTTP service
  tables after process construction.
- Factory Sessions publishes both live and durable method families for start,
  get, list, control, result, and event behavior.
- `TargetExecutionService` combines session control with canonical Factory
  Event subscription and is consumed by Chat Sessions and ACP.
- Factory Sessions forwards named Factory activation even though Factory
  Definitions already publishes that catalog operation.

Existing tests cover many individual paths, including lifecycle controls,
durable recovery, replay, response-stream isolation, and root composition, but
there is no current behavior matrix proving equivalence across every legacy
entrypoint. Coverage is therefore insufficient to delete the old paths before
FSCP-01 lands.

## Goals

- Make `root.BuildProcess` the only application/service construction boundary.
- Construct every product service once per process with complete dependencies.
- Preserve Factory Runtime as a private data plane behind Factory Sessions.
- Publish one mode-neutral Factory Sessions operation vocabulary.
- Preserve current REST, CLI, ACP, and MCP customer behavior during migration.
- Make Recordings the direct authority for canonical event and artifact reads.
- Define dispatch ownership without duplicating canonical state.
- Remove service setters and late runtime dependency injection.
- Delete every secondary opener and compatibility service after caller
  migration.
- Leave `main` releasable after every story.

## Non-goals

- Exposing Factory Runtime operations or types to public transports.
- Changing scheduling, retry, cancellation, or Worker execution policy.
- Redesigning the canonical Factory Event schema or Recording storage format.
- Renaming public REST routes or CLI commands solely to mirror Go method names.
- Moving dispatch history wholesale to Recordings transports before the
  customer-facing dispatch boundary is characterized.
- Combining unrelated Factory Definitions, Work, Workers, or UI cleanup with
  this migration.
- Removing useful consumer-declared narrow interfaces that are structurally
  satisfied by the one injected root and have no independent lifecycle.

## Target Factory Sessions contract

The exact Go names may be refined during FSCP-02, but the final peer-facing
shape must express these operations once:

```go
type Service interface {
	Start(context.Context, StartRequest) (StartResult, error)
	Invoke(context.Context, InvokeRequest) (InvokeResult, error)
	Get(context.Context, GetRequest) (Session, error)
	List(context.Context, ListRequest) (ListResult, error)
	Control(context.Context, ControlRequest) (ControlResult, error)
	ReadResult(context.Context, ResultRequest) (Result, error)
	ListDispatches(context.Context, DispatchListRequest) (DispatchListResult, error)
	GetDispatch(context.Context, DispatchGetRequest) (Dispatch, error)
	SubscribeResponses(context.Context, ResponseSubscriptionRequest) (ResponseSubscription, error)
}
```

Contract constraints:

- `Start` creates a session and returns its stable identity. It does not encode
  synchronous versus asynchronous transport waiting in the method name.
- `Invoke` is separate from `Start`. A one-command CLI flow may call both.
- Wait budgets and partial-result selection are request values, not alternate
  services or constructors.
- `Control` uses a typed action and action-specific payload. Session actions
  include pause, resume, cancel, terminate, close, approve, and recover;
  dispatch actions include retry and interrupt.
- `Get` and `List` return one projection for active, durable, interrupted, and
  terminal sessions. Persistence and hosting are fields, not APIs.
- Dispatch results are Factory Sessions-owned customer values projected from
  canonical Recording facts.
- No request or result aliases Factory Runtime implementation types.
- Canonical event and artifact operations are absent from this contract.
- Named Factory catalog activation is absent. A session-level activation, if
  retained, accepts a resolved immutable definition/version reference and is
  not named `ActivateNamedFactory`.

## Migration rules

- The replacement is additive until all callers have migrated. During the
  overlap, the mode-neutral Factory Sessions methods are canonical and the old
  paths accept no new callers.
- Temporary adapters may translate old calls to the canonical root, but may not
  construct services, retain separate state, or translate canonical calls back
  into the retiring implementation.
- Every migration story must preserve public protocol behavior unless its
  acceptance criteria explicitly names an intended behavior change.
- A story that discovers an uncharacterized behavior stops at evidence and
  updates this plan; it does not silently choose a new semantic.
- Generated files are changed only through their canonical authored inputs.
- Shared contract changes are owned by the story that introduces the canonical
  operation. Caller stories consume that contract rather than redefining it.

## Work stories

The plan expects ten independently mergeable stories. FSCP-01 establishes
coverage, FSCP-02 and FSCP-03 introduce the canonical path, FSCP-04 through
FSCP-08 migrate behavior in bounded slices, FSCP-09 deletes the retired graph,
and FSCP-10 installs regression guards and aligns architecture documentation.

### FSCP-01 — Characterize behavior across legacy entrypoints

As a maintainer, I need an executable behavior matrix for the existing session
entrypoints so that convergence does not silently change session identity,
lifecycle, persistence, result, stream, or dispatch behavior.

#### Acceptance criteria

- A checked-in matrix maps each live/durable/open/start entrypoint to the
  customer behavior it currently provides and the focused test proving it.
- Tests prove equivalent session identity and initial status for supported
  start/open paths using the same normalized input.
- Tests cover successful and failing start, invoke, pause, resume, cancel,
  terminate, recover, close, complete result, partial result, and timeout
  behavior where those paths currently support it.
- Tests prove response-event ordering and concurrent session isolation.
- Tests prove canonical event reconnect and artifact reads independently of
  ephemeral response streaming.
- Dispatch tests identify, field by field, whether active and terminal reads
  currently come from Runtime state, durable execution state, or Recording
  projections.
- No production constructor or entrypoint is removed in this story.
- Focused Factory Sessions, Recordings, HTTP, CLI, and ACP tests pass.

### FSCP-02 — Introduce the mode-neutral Factory Sessions operations

As a transport author, I need one Factory Sessions vocabulary so that the same
resource is not addressed through live, durable, target, and execution service
families.

#### Acceptance criteria

- `factorysessions.Service` publishes one additive `Start`, `Invoke`, `Get`,
  `List`, `Control`, `ReadResult`, dispatch query, and response-subscription
  vocabulary consistent with the target constraints.
- Canonical operations return the same observable results and typed failures
  as the characterized legacy paths for equivalent inputs.
- Request and result contracts use Factory Sessions vocabulary and opaque IDs;
  public signatures contain no Factory Runtime engines, services, bindings,
  sidecars, Petri state, or Runtime request/result aliases.
- Live and durable mode are represented as values in requests and projections.
- Existing callers continue to compile through temporary one-way adapters.
- New tests target the canonical operations rather than topology alone.

### FSCP-03 — Construct and inject the process service graph once

As a process host, I need `root.BuildProcess` to return fully constructed
service roots so that starting a session cannot create or mutate a second
dependency graph.

#### Acceptance criteria

- A process built by `root.BuildProcess` contains the fully constructed Factory
  Sessions, Factory Definitions, Factory Runtime, Recordings, Events, Work,
  Workers, Worker Sessions, Providers, Models, and other required roots.
- Factory Sessions receives its private data-plane and peer dependencies during
  canonical wire construction; no required dependency is supplied by a setter
  after construction.
- Calling `Start` twice creates isolated session-scoped state while retaining
  the same process-scoped service instances.
- A failed session start unwinds only resources created for that session and
  leaves the process roots usable for a subsequent start.
- Concurrent starts do not exchange session, recording, work, dispatch, or
  response-event identity.
- Process lifecycle activation and unwind remain owned by `pkg/initializer`.
- The old opening path remains temporarily callable only as a one-way adapter
  to the process-scoped roots; it no longer builds an independent graph.

### FSCP-04 — Migrate start and invocation callers

As a CLI, HTTP, ACP, or MCP caller, I need start and invocation to use the same
Factory Sessions root so that synchronous waiting and target selection do not
select alternate constructors.

#### Acceptance criteria

- HTTP, CLI, ACP, MCP, Chat Sessions, automations, and internal invocation
  callers use canonical `Start` and `Invoke` operations.
- A synchronous CLI/API flow is implemented by canonical operations plus a
  bounded wait/result policy, not `StartSync` construction.
- Existing public requests, responses, exit behavior, status codes, timeout
  behavior, and cancellation-on-timeout behavior remain compatible.
- `StartAsync`, `StartSync`, `OpenFactorySession`, and
  `OpenFactorySessionFromFolder` have no production callers outside their
  temporary compatibility adapters.
- `InvocationService` and `TargetExecutionService` are no longer injected as
  separately selected authorities.
- Focused transport-equivalence and concurrent-start tests pass.

### FSCP-05 — Migrate unified session reads, lifecycle, and results

As a customer, I need one session inventory and lifecycle vocabulary so that a
session does not change APIs when persistence or hosting changes.

#### Acceptance criteria

- All transports call canonical `Get`, `List`, `Control`, and `ReadResult` for
  session-level behavior.
- Active, durable, interrupted, recovering, and terminal sessions appear in one
  projection with explicit mode/status fields.
- Pause, resume, cancel, terminate, close, approve, and recover return stable,
  idempotent outcomes for repeated request identities.
- Complete, unavailable, partial, and failed results preserve characterized
  behavior.
- The transport layer no longer merges independent live and durable
  inventories or selects a service by session mode.
- The old `GetSession`/`GetFactorySession`, `ListSessions`/
  `ListFactorySessions`, live lifecycle, and live result method families have
  no production callers outside compatibility adapters.

### FSCP-06 — Establish one session-scoped dispatch projection and control path

As a customer inspecting a Factory Session, I need dispatch status and controls
to agree with replay while Factory Runtime remains private.

#### Acceptance criteria

- Factory Sessions list/get dispatch operations return a documented projection
  whose canonical fields are derived from Recordings-owned dispatch facts.
- Active and terminal dispatch reads agree with replay for every field that is
  declared canonical.
- Any transient field not yet derivable from Recordings is explicitly named,
  tested, and obtained through a private Factory Sessions-to-Runtime query; no
  transport receives Runtime to read it.
- Approve, retry, and interrupt enter through canonical Factory Sessions
  `Control`, validate session/dispatch association, and delegate mechanics to
  the private data plane.
- A retry produces a new attempt/dispatch identity according to existing
  policy, and replay reconstructs the same relationship.
- An interrupt reaches the associated Worker Session without exposing a Worker
  Session service object through Factory Sessions.
- Unknown, terminal, stale, and duplicate dispatch controls return stable typed
  outcomes and do not append contradictory canonical events.
- The old durable dispatch methods have no production callers outside their
  one-way adapter.

### FSCP-07 — Route canonical events and artifacts to Recordings

As a customer inspecting history, I need canonical events and artifacts to come
from the Recording authority regardless of how the session was hosted.

#### Acceptance criteria

- Recordings publishes the required root contracts for canonical event reads,
  reconnect/subscription, artifact listing, and artifact retrieval.
- HTTP, CLI, MCP, replay, and inspection callers receive `recordings.Service`
  directly from the built process for those operations.
- Existing public URLs, cursor behavior, ordering, retention-gap behavior,
  redaction, visibility, and artifact retrieval semantics remain compatible.
- Factory Sessions retains only ephemeral response-event subscription and
  session-level links/references to canonical resources.
- `ReadEvents`, `ListArtifacts`, `GetArtifact`, durable Factory Event streams,
  and probes have no production implementation in Factory Sessions.
- Replay and concurrent-session isolation tests prove the canonical and
  ephemeral streams cannot be confused.

### FSCP-08 — Remove adjacent-service gateways and caller back-queries

As a service maintainer, I need each peer to use its injected root and explicit
values so that Factory Sessions does not act as a locator for Definitions,
Work, Worker Sessions, or Runtime objects.

#### Acceptance criteria

- Named Factory catalog activation is called on Factory Definitions directly.
- Applying a resolved definition to a session, when requested, enters through a
  distinctly named Factory Sessions operation carrying immutable resolved
  identity/value data.
- `DefinitionActivationGateway` and provider/downcast paths have no production
  callers.
- Work owns Work submission and movement without retrieving a Runtime object or
  engine snapshot from Factory Sessions.
- Worker Sessions observation is requested from the injected Worker Sessions
  root using explicit Factory Session/Worker Session identities.
- Factory Sessions exposes no `CurrentRuntime`, `ResolveWorkRuntime`,
  `WithRuntimeRead`, engine snapshot, returned observation service, or peer
  service gateway.
- Resource/workstation Session controls continue to enter through Factory
  Sessions and reach the private data plane without public Runtime exposure.

### FSCP-09 — Delete secondary openers, setters, and compatibility services

As a maintainer, I need the retired paths removed after migration so that new
callers cannot reintroduce multiple construction and execution authorities.

#### Acceptance criteria

- Production code contains no `internal/runtimeopening`,
  `internal/applicationopening`, `internal/executionopening`, or
  `internal/ondemandtarget` package.
- `DurableExecutionService`, `TargetExecutionService`, mode-specific live
  service facets, opening request/result bundles, application service tables,
  and separately constructible execution services are deleted.
- Runtime-time setters including `SetWorkerInvoker`, `SetWorkerExecution`,
  `SetDirectWorkerExecution`, `SetWorkerProgressPublisher`, and
  `SetWorkerAttemptStarter` are deleted from production code.
- `pkg/wire` contains no runtime/application/execution opener providers,
  factories, mutable service tables, or runtime assembly callbacks.
- No method on a product service returns another product service or a
  session-specific service façade.
- All characterized public behaviors continue to pass through the canonical
  process roots.
- Package-boundary, dependency-direction, short Go, functional session,
  replay, HTTP, CLI, ACP, and MCP tests pass.

### FSCP-10 — Guard the construction and ownership boundaries

As a future contributor, I need executable boundary checks and current
architecture documentation so that alternate openers and mode-specific service
families do not return.

#### Acceptance criteria

- A focused structural check fails when production code introduces a secondary
  application/runtime/execution opener, a post-construction service setter, or
  an owner-published live/durable service family.
- The check allows domain resource creation inside an injected service and
  consumer-declared narrow interfaces that have no constructor or lifecycle.
- Behavioral tests, rather than source inventory alone, remain the primary
  evidence for start, lifecycle, dispatch, event, artifact, and replay behavior.
- Architecture, packaged-structure, data-model, and service-ownership docs
  describe Factory Sessions as public control plane, Factory Runtime as private
  data plane, Recordings as canonical history/artifact authority, and the
  hybrid dispatch boundary.
- Stale documentation that instructs callers to open a runtime/application or
  call Factory Runtime from a transport is removed or explicitly archived.

## Package and contract changes

### `pkg/root`, `pkg/wire`, and `pkg/initializer`

- Keep `root.BuildProcess` as the caller-facing construction boundary.
- Make the canonical wire graph provide each fully constructed service root.
- Pass complete peer and edge dependencies into service constructors.
- Retain process activation/unwind in `pkg/initializer`.
- Remove opener factories, service tables, and late-binding callbacks after
  migration.

### `pkg/services/factory_sessions`

- Introduce the mode-neutral operations and Sessions-owned customer values.
- Keep Runtime coordination private.
- Keep ephemeral response events.
- Keep session-scoped dispatch query/control semantics backed by canonical
  Recording facts.
- Remove canonical event/artifact operations, named catalog activation,
  returned peer gateways, runtime objects, mode-specific interfaces, and
  opener contracts.

### `pkg/services/recordings`

- Publish canonical event read/reconnect and artifact query/retrieval contracts
  required by current transports.
- Provide dispatch projection queries consumed privately by Factory Sessions.
- Keep Recording/replay state authoritative; do not depend on Factory Sessions
  for canonical facts.

### `pkg/services/factory_runtime`

- Remain a process-scoped private data plane.
- Accept identity-addressed commands/queries from application services without
  returning a bound service.
- Create and release per-session execution state internally.
- Do not become a direct Factory Session transport dependency.

### Other service and transport packages

- Factory Definitions retains named catalog activation and resolves immutable
  definition/version values.
- Work and Worker Sessions consume their own injected roots and explicit IDs.
- Chat Sessions and ACP replace `TargetExecutionService` with narrow views of
  the canonical injected roots.
- HTTP/CLI/MCP adapters preserve public behavior while changing their internal
  service dependency.

## API and generated-contract policy

This convergence does not inherently require public REST route or payload
changes. Existing public protocol behavior should remain stable while Go
service contracts converge.

If implementation discovers that a public schema must change, that change must
be added to this plan as a separately reviewable behavior story. It must be
authored under `api/openapi-main.yaml` or `api/components/`, regenerated with
the appropriate `make generate-api` or `make interfaces-all` target, and
verified with `make api-smoke`. Generated files must not be edited manually.

## Verification strategy

Each story runs the narrowest affected package and transport tests. Before the
deletion story merges, the lane must also run:

- `go test ./pkg/root ./pkg/wire ./pkg/initializer/...`
- `go test ./pkg/services/factory_sessions/...`
- `go test ./pkg/services/factory_runtime/...`
- `go test ./pkg/services/recordings/...`
- focused Work, Worker Sessions, Chat Sessions, Events, HTTP, CLI, ACP, and MCP
  tests affected by caller migration;
- functional Factory Session root-composition, concurrent isolation, recovery,
  response-event, canonical replay, dispatch, and artifact tests;
- `make test`;
- `make lint`; and
- the relevant package/dependency boundary checks.

When public OpenAPI artifacts change, also run `make generate-api`, confirm the
generated diff comes only from authored sources, and run `make api-smoke`.

Test evidence must distinguish a gate that actually measured the target
property from one that exited early. CI evidence belongs in a PR comment, not a
commit.

## Delivery criterion

Implementation-stage delivery criterion: The implementation stage marks this
criterion satisfied and stops after its final head is pushed, the PR is open,
CI has started, and all blocking review feedback is addressed. It does not poll
or re-check CI after this finish line. The review stage owns driving CI to
terminal-and-passing, resolving merge conflicts, and merging the PR; merge
remains the lane-wide delivery boundary. CI-run evidence goes in a PR comment
and never in a commit.

The implementation/review cycle continues through merge for each independently
mergeable story. No story may depend on a later story to restore releasability,
and the lane is not complete until FSCP-09 removes the retired construction and
service paths and FSCP-10's guards and documentation are merged.

## Implementation task-packet requirement

Before a story is submitted to an implementation worker, convert that story
into the structure required by
`docs/internal/standards/templates/task-templates.md`: one-line problem
statement, customer ask, expected solution, this plan's absolute path as the
original document, explicit package/file changes, induced contracts, service
changes, API changes, and focused tests. Keep each packet bounded to one story;
do not submit this entire ten-story plan as one task.
