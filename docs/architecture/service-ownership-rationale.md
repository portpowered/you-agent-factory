# Service Ownership Rationale

This document records the durable design intent behind the Packaged Service
Structure owner tree: why each committed service owns what it owns, which
large responsibility clusters deliberately stay plain modules, which public
surfaces map to which durable owner, and which constructors, datastores,
lifecycle roles, and protocol adapters belong to which destination.

It is prose, not a ratchet. Nothing here is counted, diffed, or gated by a
lint target, and adding a package or a service does not require editing it to
keep a checker green. Content that *must not regress* lives under
[`docs/internal/baselines/`](../internal/baselines/README.md) instead — today
that is the unfinished-package-move ledger and the ownership-inventory freeze.

For the live package layout and the rules that are enforced mechanically, see
[Backend Package Structure](packaged-structure.md). For public resource
vocabulary, see [Data Model](data-model.md).

## Destination Vocabulary

Every package destination resolves to one of three buckets. Product owners are
the directories under `pkg/services`; non-service families are the remaining
top-level `pkg` families; the architecture exception is the single broad
external-effect package that is not a product service.

**Product owners**

- `factory_definitions`
- `factory_sessions`
- `factory_runtime`
- `work`
- `workers`
- `providers`
- `provider_sessions`
- `models`
- `automations`
- `recordings`
- `factory_visualization`
- `costs`
- `operator_settings`
- `system_initialization`
- `chat_sessions`
- `events`
- `worker_sessions`
- `webhooks`

**Non-service families**

- `initializer`
- `root`
- `wire`
- `platform`
- `transports`

**Architecture exceptions**

- `edges` — Process Edges (pkg/services/edges) is the sole broad external-effect architecture exception for the Packaged Service Structure program.

## Owner Rationale Cards

Each committed service — 18 top-level owners and 52 nested
subservices — records the same six aspects: what it has authority over, what
state it stores, what lifecycle it runs, who consumes it, where its transaction
boundary sits, and how failures are surfaced and recovered.

### `automations`

Top-level owner · `pkg/services/automations`

| Aspect | Rationale |
| --- | --- |
| Authority | Desired/observed trigger instances, cursors/checkpoints, and Work Request generation from clocks or external observations. |
| State store | Automation definitions, instance status, and cursor/checkpoint store owned by Automations. |
| Lifecycle | Reconciles desired specs into long-running trigger instances with start/stop/join/restart policy. |
| Consumers | Factory Runtime (via Work commands), Work Admission, Operator Settings, transports commanding reconcile/list/control. |
| Transaction boundary | Trigger instance and cursor mutations stay inside Automations; Work admission is a cross-owner command. |
| Failure recovery | Missed triggers, duplicate emissions, cursor recovery, cancellation, and downstream Work rejection are typed Automations facts. |

#### `automations/cron`

Nested subservice of `automations` · `pkg/services/automations/internal/services/cron`

| Aspect | Rationale |
| --- | --- |
| Authority | Deterministic schedule ticks that command Work admission. |
| State store | Schedule/jitter/expiry configuration and tick cursors under Automations store. |
| Lifecycle | Produces ticks under Reconciliation supervision; no independent daemon outside Automations. |
| Consumers | Automations Reconciliation and Work Admission via Automations root. |
| Transaction boundary | Shares Automations instance/cursor transaction; does not own Work content. |
| Failure recovery | Invalid schedules and missed ticks surface as Automations status/recovery facts. |

#### `automations/filesystem_watchers`

Nested subservice of `automations` · `pkg/services/automations/internal/services/filesystem_watchers`

| Aspect | Rationale |
| --- | --- |
| Authority | Watch configured paths, preseed, debounce/coalesce, persist cursor, and command Work. |
| State store | Watcher cursors and configured path subscriptions in Automations store. |
| Lifecycle | Long-running watch instances supervised by Reconciliation. |
| Consumers | Automations root consumers and Work Admission; moves filesystem ingest off Factory Runtime. |
| Transaction boundary | Cursor and subscription mutations are Automations-local; Work creation is a command. |
| Failure recovery | Watcher restart and cursor recovery remain Automations failure/recovery policy. |

#### `automations/hosted_sources`

Nested subservice of `automations` · `pkg/services/automations/internal/services/hosted_sources`

| Aspect | Rationale |
| --- | --- |
| Authority | Observe hosted systems, poll/restart/checkpoint, normalize observations, and command Work. |
| State store | Hosted-source cursors/checkpoints and secrets resolution handles under Automations. |
| Lifecycle | Polling/restart lifecycle owned by Automations, not Workers Hosted Runner. |
| Consumers | Automations root and Work Admission; Workers retains only hosted Work execution. |
| Transaction boundary | Observation/cursor state is Automations-local; Work Requests cross the Work boundary. |
| Failure recovery | Poll/restart and secret-resolution failures classify as Automations source faults. |

#### `automations/reconciliation`

Nested subservice of `automations` · `pkg/services/automations/internal/services/reconciliation`

| Aspect | Rationale |
| --- | --- |
| Authority | Apply desired automation specs, construct instances, track status, and supervise lifecycle. |
| State store | Desired/observed automation instance registry and status in Automations store. |
| Lifecycle | Owns start/stop/join/restart orchestration for trigger subservices. |
| Consumers | Automations root command surface and trigger subservices. |
| Transaction boundary | Desired-spec application and instance status mutations are one Automations aggregate. |
| Failure recovery | Restart policy and reconcile conflicts remain Automations recovery behavior. |

#### `automations/script_pollers`

Nested subservice of `automations` · `pkg/services/automations/internal/services/script_pollers`

| Aspect | Rationale |
| --- | --- |
| Authority | Supervise polling commands/sources that emit canonical Work Requests. |
| State store | Poller cursors/checkpoints under Automations store. |
| Lifecycle | Timeout/restart supervision under Reconciliation. |
| Consumers | Automations root and Work Admission. |
| Transaction boundary | Cursor and poller status are Automations-local; emitted Work crosses owners by command. |
| Failure recovery | Timeout and parse failures remain Automations poller recovery facts. |

### `chat_sessions`

Top-level owner · `pkg/services/chat_sessions`

| Aspect | Rationale |
| --- | --- |
| Authority | Owns Chat Session, target-episode, turn, attachment, control-intent, and Factory-target catalog authority, including validation, lifecycle transitions, idempotency, and the live session service implementation. |
| State store | Process-local Chat Session state and its sequenced delivery metadata; Events owns the source-native stream records and Recordings owns the canonical durable Factory Event ledger. |
| Lifecycle | Wire constructs the service once; session, turn, attachment, control, Factory-target binding, and response-bridge operations run through the Chat Sessions lifecycle. |
| Consumers | ACP transport, Factory Sessions target execution, Events sequencing, Worker Sessions response bridging, and Chat Sessions-owned transport adapters. |
| Transaction boundary | Chat Session version guards serialize session-local mutations; Factory-target execution and event sequencing remain explicit cross-owner commands through published capabilities. |
| Failure recovery | Typed validation, not-found, busy, conflict, transition, attachment-position, and retention-gap failures preserve the session boundary; retries converge through request identities and idempotent operations. |

### `costs`

Top-level owner · `pkg/services/costs`

| Aspect | Rationale |
| --- | --- |
| Authority | Exact monetary valuation, pricing coverage, unpriced usage classification, and deterministic rollups over canonical runtime usage. |
| State store | No durable state; each report uses detached Operator Settings and Factory Visualization results in local accumulators. |
| Lifecycle | Stateless request operation constructed once by Wire; no autonomous lifecycle or hidden global price cache. |
| Consumers | HTTP/CLI transports and the process root capability. |
| Transaction boundary | One report reads the Operator Settings price table and the Factory Visualization metrics query, then owns all valuation and rollup decisions locally. |
| Failure recovery | Missing identities, prices, and class-specific rates remain visible as UNPRICED line items; dependency and malformed-input failures are typed Costs outcomes. |

### `events`

Top-level owner · `pkg/services/events`

| Aspect | Rationale |
| --- | --- |
| Authority | Owns the process-local source-native event stream: event identity, envelopes, append and source attachment, retained reads, cursors, subscriptions, retention gaps, and backpressure outcomes. |
| State store | In-memory topic stores and subscription state; Recordings remains the canonical durable Factory Event ledger and replay source. |
| Lifecycle | Wire constructs the stream service once; append, attach, retained-read, and subscription lifecycles are active for the process lifetime and close with the application. |
| Consumers | Chat Sessions sequencing, Worker Sessions publication, Factory Sessions response consumers, ACP transport composition, and Recordings event capture. |
| Transaction boundary | Topic append and subscription state are Events-local; Chat Sessions owns session sequencing and Recordings owns durable capture, so cross-owner handoffs use explicit event contracts. |
| Failure recovery | Invalid identities, duplicate appends, source mismatches, retention gaps, and closed subscriptions return typed stream facts without moving durable history into Events. |

### `factory_definitions`

Top-level owner · `pkg/services/factory_definitions`

| Aspect | Rationale |
| --- | --- |
| Authority | Authored Factory aggregate catalog: validation, persistence, versioning, activation, and packaged installation. |
| State store | Named Factory roots, authored layouts, .current-factory pointer, snapshots, and packaged assets. |
| Lifecycle | No autonomous background daemon; activation is request-driven through Factory Sessions. |
| Consumers | Factory Sessions, System Bootstrap, Automations (trigger specs), transports, Wire construction. |
| Transaction boundary | Catalog/authoring/compilation/validation/snapshot/distribution share one Definition aggregate transaction. |
| Failure recovery | Malformed layouts, activation conflicts, and persistence failures remain Definition-owned typed facts. |

#### `factory_definitions/authoring_layout`

Nested subservice of `factory_definitions` · `pkg/services/factory_definitions/internal/services/authoring_layout`

| Aspect | Rationale |
| --- | --- |
| Authority | Parse/render split authored layouts and atomically create/replace/flatten/expand one Factory aggregate. |
| State store | Authored layout files under Definition-owned filesystem datastore. |
| Lifecycle | Request-driven authoring operations; no independent daemon. |
| Consumers | Factory Definitions root and Definition-owned transports. |
| Transaction boundary | Layout writes participate in the Definition aggregate transaction. |
| Failure recovery | Parse/render and layout conflicts are Definition authoring failures. |

#### `factory_definitions/catalog`

Nested subservice of `factory_definitions` · `pkg/services/factory_definitions/internal/services/catalog`

| Aspect | Rationale |
| --- | --- |
| Authority | List/delete/resolve named/project/global Factories and atomically manage the current pointer. |
| State store | Named Factory catalog and .current-factory pointer. |
| Lifecycle | Catalog mutations are command-driven; no background reconciler. |
| Consumers | Factory Definitions root, System Bootstrap, Factory Sessions activation. |
| Transaction boundary | Catalog and current-pointer updates are atomic within Definitions. |
| Failure recovery | Missing/conflicting names and pointer races are Definition catalog failures. |

#### `factory_definitions/compilation`

Nested subservice of `factory_definitions` · `pkg/services/factory_definitions/internal/services/compilation`

| Aspect | Rationale |
| --- | --- |
| Authority | Convert authored/canonical source into one normalized LoadedFactorySource without executing it. |
| State store | Compiled effective definition artifacts derived from authored store. |
| Lifecycle | Compile on command; Runtime executes, Definitions does not. |
| Consumers | Factory Definitions root and Runtime semantic-validation callers via root. |
| Transaction boundary | Compilation outputs are Definition-owned; Runtime does not mutate authored store. |
| Failure recovery | Compile/reference-resolution failures stay on the Definition boundary. |

#### `factory_definitions/distribution`

Nested subservice of `factory_definitions` · `pkg/services/factory_definitions/internal/services/distribution`

| Aspect | Rationale |
| --- | --- |
| Authority | Built-in package catalog, packaged installation, and scaffold creation producing Factory aggregates. |
| State store | Packaged Factory assets and installation outputs in Definition store. |
| Lifecycle | Install/scaffold commands; no marketplace lifecycle in this program. |
| Consumers | System Bootstrap, CLI/HTTP Definition adapters, Factory Definitions root. |
| Transaction boundary | Installation writes the same Definition aggregate transactionally. |
| Failure recovery | Install/scaffold failures roll back or report Definition distribution faults. |

#### `factory_definitions/invocation_policy`

Nested subservice of `factory_definitions` · `pkg/services/factory_definitions/internal/services/invocation_policy`

| Aspect | Rationale |
| --- | --- |
| Authority | Definition-owned interpretation of authored Factory behavior at invocation time: decision envelopes, interpolation, output shaping, work-type selection, quorum lineage, work propagation, workstation execution limits, and TTS observability identity. |
| State store | Stateless invocation-time policy derivations over authored worker, workstation, and packaged configuration. |
| Lifecycle | Policy interpretation runs on command during worker and workstation invocation; no independent daemon. |
| Consumers | Factory Definitions root, Factory Sessions invocation, and Workers dispatch consumers via Definitions root. |
| Transaction boundary | Invocation policy does not mutate peer stores; interpretation outputs are Definition-owned values consumed at invocation. |
| Failure recovery | Malformed authored policy and unsupported invocation contracts surface as Definition-owned rejection facts. |

#### `factory_definitions/snapshots_portability`

Nested subservice of `factory_definitions` · `pkg/services/factory_definitions/internal/services/snapshots_portability`

| Aspect | Rationale |
| --- | --- |
| Authority | Capture detached snapshots and prepare/materialize bundled assets for export/import/replay use. |
| State store | Editable/version snapshots and bundled portable assets. |
| Lifecycle | Snapshot/export/import are request-driven operations. |
| Consumers | Recordings replay consumers via Definition compile/import root operations; Definition transports. |
| Transaction boundary | Snapshot materialization stays inside Definitions; Recordings owns replay plans. |
| Failure recovery | Portability validation and materialization failures are Definition-owned. |

#### `factory_definitions/validation`

Nested subservice of `factory_definitions` · `pkg/services/factory_definitions/internal/services/validation`

| Aspect | Rationale |
| --- | --- |
| Authority | Structural, topology, pre-persist, tool, and workstation compatibility validation policy. |
| State store | Validation policy and diagnostics over Definition aggregate; may query Runtime root for orchestration semantics. |
| Lifecycle | Validation runs on command before persist/activation. |
| Consumers | Factory Definitions root and authoring/activation callers. |
| Transaction boundary | Validation does not mutate peer stores; persist remains Definition transaction. |
| Failure recovery | Validation diagnostics are Definition rejection facts, not Runtime failures. |

### `factory_runtime`

Top-level owner · `pkg/services/factory_runtime`

| Aspect | Rationale |
| --- | --- |
| Authority | Per-session orchestration, execution state, instance mechanics, dispatch planning, and checkpoints. |
| State store | Runtime checkpoints, submission/result/dispatch buffers, and live instance state. |
| Lifecycle | Owns run handle pause/resume/stop/replacement/recovery for one leased execution instance. |
| Consumers | Factory Sessions (lease/start), Workers (dispatch), Work, Recordings (event candidates), Visualization (sanitized observations). |
| Transaction boundary | Runtime instance and checkpoint mutations are Runtime-local; peer Work/Workers/Recordings interactions are commands/events. |
| Failure recovery | Crash recovery uses Runtime checkpoints plus Recordings history; orchestration failures normalize at Runtime root. |

#### `factory_runtime/checkpoint_recovery`

Nested subservice of `factory_runtime` · `pkg/services/factory_runtime/internal/services/checkpoint_recovery`

| Aspect | Rationale |
| --- | --- |
| Authority | Capture/load versioned execution checkpoints and restore orchestration state idempotently. |
| State store | Runtime-owned checkpoint store coordinated with Recordings history ownership. |
| Lifecycle | Recovery commands after durability decision; not an independent deployable yet. |
| Consumers | Factory Runtime root and Instance Host. |
| Transaction boundary | Checkpoint write/load is Runtime-local; canonical history remains Recordings. |
| Failure recovery | Incompatible checkpoints and restore conflicts are Runtime recovery failures. |

#### `factory_runtime/dispatch_planning`

Nested subservice of `factory_runtime` · `pkg/services/factory_runtime/internal/services/dispatch_planning`

| Aspect | Rationale |
| --- | --- |
| Authority | Convert enabled transitions into stable dispatch intents and accept correlated results. |
| State store | In-flight dispatch intent/outbox buffers owned by Runtime. |
| Lifecycle | Publishes intents after Workers command/result contract lock. |
| Consumers | Workers execution and Factory Runtime Instance Host. |
| Transaction boundary | Intent lifecycle is Runtime-local; Workers owns execution attempts. |
| Failure recovery | Duplicate/correlated-result conflicts recover inside Runtime outbox policy. |

#### `factory_runtime/instance_host`

Nested subservice of `factory_runtime` · `pkg/services/factory_runtime/internal/services/instance_host`

| Aspect | Rationale |
| --- | --- |
| Authority | Construct inert instances from lease/start data and manage run handle lifecycle. |
| State store | Live instance handles and runtime-owned artifact closure metadata. |
| Lifecycle | Start/stop/pause/resume/replace/recover for one Runtime instance. |
| Consumers | Factory Sessions placement/lease commands and Orchestration. |
| Transaction boundary | Instance handle mutations stay in Runtime; Session desired lifecycle stays in Sessions. |
| Failure recovery | Replacement and recovery failures remain Runtime instance faults. |

#### `factory_runtime/orchestration`

Nested subservice of `factory_runtime` · `pkg/services/factory_runtime/internal/services/orchestration`

| Aspect | Rationale |
| --- | --- |
| Authority | Select orchestration kind, compile/validate/preview/execute/resume strategy semantics, encode checkpoints. |
| State store | Compiled orchestration strategy state for the active instance. |
| Lifecycle | Execute/resume under Instance Host; Petri/JavaScript are internal variants. |
| Consumers | Factory Runtime root, Definition semantic validation callers via Runtime root. |
| Transaction boundary | Strategy execution shares Runtime instance transaction; authored Factory config remains Definitions. |
| Failure recovery | Strategy validation and execution faults normalize through Runtime orchestration. |

### `factory_sessions`

Top-level owner · `pkg/services/factory_sessions`

| Aspect | Rationale |
| --- | --- |
| Authority | Session identity, desired lifecycle, placement, recovery, discovery, invocation coordination, and session response streams. |
| State store | Live session registry, session control store, and session-owned persistence coordination (canonical history delegated to Recordings). |
| Lifecycle | Owns desired lifecycle reconcile, live selection/activation, and durable start/resume/control. |
| Consumers | Transports, Factory Runtime, Work, Workers, Provider Sessions, Models, Visualization, Recordings. |
| Transaction boundary | Session aggregate mutations are Sessions-local; peer services are commanded through roots. |
| Failure recovery | Placement/activation conflicts, durable control failures, and timeout/cancellation are Sessions recovery facts. |

#### `factory_sessions/durable_execution`

Nested subservice of `factory_sessions` · `pkg/services/factory_sessions/internal/services/durable_execution`

| Aspect | Rationale |
| --- | --- |
| Authority | Durable start/resume/control/inspection and session-owned persistence coordination. |
| State store | Session durable execution identity/persistence handles; delegates canonical history/artifacts to Recordings. |
| Lifecycle | Durable control lifecycle for a session. |
| Consumers | Factory Sessions root, Live Runtime, Recordings root. |
| Transaction boundary | Session persistence coordination is Sessions-local; ledger append is Recordings command. |
| Failure recovery | Durable control and resume conflicts recover in Durable Execution. |

#### `factory_sessions/identity`

Nested subservice of `factory_sessions` · `pkg/services/factory_sessions/internal/services/identity`

| Aspect | Rationale |
| --- | --- |
| Authority | Normalize/discover targets, derive logical keys, resolve folders, registry-aware identity lookup. |
| State store | Logical session keys and folder/identity resolution metadata. |
| Lifecycle | Identity resolution is request-driven under Sessions. |
| Consumers | Factory Sessions root and nested Live Runtime/Durable Execution. |
| Transaction boundary | Identity derivation shares Sessions aggregate authority. |
| Failure recovery | Ambiguous/missing identity targets are Sessions identity failures. |

#### `factory_sessions/invocation`

Nested subservice of `factory_sessions` · `pkg/services/factory_sessions/internal/services/invocation`

| Aspect | Rationale |
| --- | --- |
| Authority | Resolve invocation input, command Work, observe completion, enforce timeout/cancellation, record telemetry. |
| State store | Per-session invocation observation state. |
| Lifecycle | Timeout/cancellation lifecycle for one session invocation. |
| Consumers | Work root, Factory Sessions root, Recordings telemetry consumers. |
| Transaction boundary | Invocation coordination is Sessions-local; Work admission/state are Work commands. |
| Failure recovery | Timeouts and cancellations are Sessions invocation recovery facts. |

#### `factory_sessions/live_runtime`

Nested subservice of `factory_sessions` · `pkg/services/factory_sessions/internal/services/live_runtime`

| Aspect | Rationale |
| --- | --- |
| Authority | Open/list/get/snapshot/pause/resume/close; sole owner of live registry, selection, activation lock, cleanup. |
| State store | Live session registry and mutable live-runtime aggregate. |
| Lifecycle | Live activation lock and cleanup lifecycle. |
| Consumers | Factory Sessions root, Runtime Opening, Durable Execution, Response Stream. |
| Transaction boundary | Live registry mutations are exclusive to Live Runtime inside Sessions. |
| Failure recovery | Activation races and cleanup failures recover under Live Runtime. |

#### `factory_sessions/response_stream`

Nested subservice of `factory_sessions` · `pkg/services/factory_sessions/internal/services/response_stream`

| Aspect | Rationale |
| --- | --- |
| Authority | Allocate session event stores, publish/complete, filter subscriptions, enforce retention/backpressure, track cursors. |
| State store | Ephemeral session response-event stores and cursors. |
| Lifecycle | Publish/complete and subscription lifecycle for a session. |
| Consumers | Factory Sessions root, transports/Visualization presentation consumers. |
| Transaction boundary | Response-store mutations are Sessions-local; canonical Factory events belong to Recordings. |
| Failure recovery | Backpressure/drop and retention faults are Response Stream recovery policy. |

#### `factory_sessions/runtime_opening`

Nested subservice of `factory_sessions` · `pkg/services/factory_sessions/internal/services/runtime_opening`

| Aspect | Rationale |
| --- | --- |
| Authority | Coordinate peer root services and return a lifecycle plan without importing peer implementations. |
| State store | No independent datastore; produces lifecycle plan over already-constructed roles. |
| Lifecycle | Opening/binding lifecycle after root binding contract stabilizes. |
| Consumers | Factory Sessions root and peer service roots (Definitions, Runtime, Workers, Models, Recordings). |
| Transaction boundary | Does not own peer transactions; only plans Sessions-owned activation steps. |
| Failure recovery | Binding/plan failures surface as Sessions opening faults without peer downcasts. |

### `factory_visualization`

Top-level owner · `pkg/services/factory_visualization`

| Aspect | Rationale |
| --- | --- |
| Authority | Request-activated visualization lifecycle, live projection, response/event presentation, queues, backpressure, final-write coordination. |
| State store | Visualization cursors, output queues, and presentation state for an activated request. |
| Lifecycle | Inert by default; Start/Stop/Wait/cleanup only when request parameters activate it. |
| Consumers | Transports (parameters/codecs/sinks), Recordings query/subscription, sanitized Runtime observations. |
| Transaction boundary | Activation and presentation mutations are Visualization-local; Recordings/Runtime remain peer roots. |
| Failure recovery | Backpressure drops, activation failures, and drain errors are Visualization recovery facts. |

#### `factory_visualization/activation_lifecycle`

Nested subservice of `factory_visualization` · `pkg/services/factory_visualization/internal/services/activation_lifecycle`

| Aspect | Rationale |
| --- | --- |
| Authority | Interpret approved request options, bind session/runtime, own Start/Stop/Wait/cleanup. |
| State store | Activation binding state for the request. |
| Lifecycle | Request-driven visualization lifecycle. |
| Consumers | Factory Visualization root and transport adapters. |
| Transaction boundary | Activation state is Visualization-local. |
| Failure recovery | Failed binds and unclean shutdowns are activation recovery faults. |

#### `factory_visualization/live_view_projection`

Nested subservice of `factory_visualization` · `pkg/services/factory_visualization/internal/services/live_view_projection`

| Aspect | Rationale |
| --- | --- |
| Authority | Retain cursor/events, subscribe retained-then-live, consume Runtime observations and Recordings queries, emit View. |
| State store | Live view cursor/event retention owned by Visualization. |
| Lifecycle | Subscription lifecycle under activation. |
| Consumers | Recordings projection/query root and Runtime observation contract. |
| Transaction boundary | View projection state is Visualization-local; canonical projections stay in Recordings. |
| Failure recovery | Reconnect/gap handling follows Visualization cursor policy. |

#### `factory_visualization/response_event_presentation`

Nested subservice of `factory_visualization` · `pkg/services/factory_visualization/internal/services/response_event_presentation`

| Aspect | Rationale |
| --- | --- |
| Authority | Best-effort/lossless queues, bounded drop/backpressure, exclusive final writes, drain. |
| State store | Output queues and presentation buffers. |
| Lifecycle | Queue drain lifecycle tied to activation. |
| Consumers | Transport sinks/codecs and Visualization root. |
| Transaction boundary | Queue mutations are Visualization-local. |
| Failure recovery | Drop/backpressure and final-write conflicts are presentation recovery facts. |

### `models`

Top-level owner · `pkg/services/models`

| Aspect | Rationale |
| --- | --- |
| Authority | Opaque runtime scopes, model catalog, assets, supervised hosting/readiness, nested leases, and inference. |
| State store | Model catalog/readiness store, asset cache layout, supervised runtime slots and leases. |
| Lifecycle | Process-scoped service is inert until commanded; hosts manage unload/eviction/shutdown. |
| Consumers | Workers, Factory Sessions, Operator Settings, Models HTTP/CLI adapters. |
| Transaction boundary | Catalog/assets/host/lease/inference share Models process scope; peers never see host internals. |
| Failure recovery | Pull/host/lease/inference failures normalize at Models root with typed readiness/availability facts. |

#### `models/assets`

Nested subservice of `models` · `pkg/services/models/internal/services/assets`

| Aspect | Rationale |
| --- | --- |
| Authority | Resolve source/cache layout, pull, verify, classify, remove, and report immutable asset readiness. |
| State store | Asset cache and verification metadata. |
| Lifecycle | Pull/remove operations; no independent public lifecycle. |
| Consumers | Models Catalog/Inference via Models root. |
| Transaction boundary | Asset mutations are Models-local. |
| Failure recovery | Pull/verify failures classify as Models asset readiness faults. |

#### `models/catalog`

Nested subservice of `models` · `pkg/services/models/internal/services/catalog`

| Aspect | Rationale |
| --- | --- |
| Authority | Canonical model identity, configured operation/binding discovery, source metadata, list/get, readiness projection. |
| State store | Model catalog and configured binding metadata. |
| Lifecycle | Catalog queries/commands; readiness projection without hosting. |
| Consumers | Models root consumers including Workers and direct `you models` flows. |
| Transaction boundary | Catalog identity is Models-local; Providers owns external provider IDs. |
| Failure recovery | Unknown model/operation lookups are Catalog rejection facts. |

#### `models/inference`

Nested subservice of `models` · `pkg/services/models/internal/services/inference`

| Aspect | Rationale |
| --- | --- |
| Authority | Resolve scoped model operation, acquire lease, load/reuse handle, invoke, release, normalize artifacts/failures. |
| State store | Invocation-time handle/lease usage over Runtime Host slots. |
| Lifecycle | Acquire/release around one inference operation. |
| Consumers | Workers and Models transports via Models root. |
| Transaction boundary | Inference does not own catalog/host stores; it uses Host leases transactionally for one call. |
| Failure recovery | Invocation and artifact-export failures normalize through Inference. |

#### `models/runtime_host`

Nested subservice of `models` · `pkg/services/models/internal/services/runtime_host`

| Aspect | Rationale |
| --- | --- |
| Authority | Own supervised runtime slots, health/readiness, handle reuse, idle unload, resource-pressure eviction, shutdown. |
| State store | Supervised process/runtime slot registry. |
| Lifecycle | Host supervision lifecycle including idle unload and shutdown. |
| Consumers | Models Inference and Leases nested subservice. |
| Transaction boundary | Slot mutations are exclusive to Runtime Host (+ nested Leases). |
| Failure recovery | Eviction and supervision faults recover inside Runtime Host. |

##### `models/runtime_host/leases`

Nested subservice of `models/runtime_host` · `pkg/services/models/internal/services/runtime_host/internal/services/leases`

| Aspect | Rationale |
| --- | --- |
| Authority | Capacity reservation, holder identity, acquisition/release/expiry, contention over Host-owned slots. |
| State store | Lease records over Runtime Host slots. |
| Lifecycle | Lease acquire/release/expiry lifecycle nested under Host. |
| Consumers | Models Inference via Runtime Host. |
| Transaction boundary | Leases share Host atomic capacity invariant; not a sibling top-level cut. |
| Failure recovery | Contention/expiry failures are lease recovery facts under Host. |

#### `models/runtime_scopes`

Nested subservice of `models` · `pkg/services/models/internal/services/runtime_scopes`

| Aspect | Rationale |
| --- | --- |
| Authority | Open/close opaque scopes for detached Factory-session model configuration without constructing another Service. |
| State store | Scope binding registry rejecting stale/foreign references. |
| Lifecycle | Open/close scope lifecycle; does not construct hosts/pullers. |
| Consumers | Factory Sessions and Models root callers. |
| Transaction boundary | Scope registry is Models-local and process-scoped. |
| Failure recovery | Stale/foreign scope references are typed scope faults. |

### `operator_settings`

Top-level owner · `pkg/services/operator_settings`

| Aspect | Rationale |
| --- | --- |
| Authority | Operator-owned configuration, backend identity, defaults, provider/worker presets, precedence, semantic updates, atomic persistence. |
| State store | Authoritative operator settings document store. |
| Lifecycle | Load/update/persist on command; no autonomous reconciler. |
| Consumers | Factory Sessions, Workers, Models, System Bootstrap, CLI, Wire. |
| Transaction boundary | Document and resolution share one atomic operator document aggregate. |
| Failure recovery | Malformed/unsupported/conflict/persistence failures are Operator Settings typed facts. |

#### `operator_settings/document`

Nested subservice of `operator_settings` · `pkg/services/operator_settings/internal/services/document`

| Aspect | Rationale |
| --- | --- |
| Authority | Load, decode, normalize, validate, identity/version, inventory, semantic update, and atomically persist the operator document. |
| State store | Operator settings document files/store. |
| Lifecycle | Atomic persist lifecycle for document updates. |
| Consumers | Operator Settings Resolution and root consumers. |
| Transaction boundary | Document writes are the Operator Settings transaction. |
| Failure recovery | Decode/validation/persist failures stay on Document. |

#### `operator_settings/resolution`

Nested subservice of `operator_settings` · `pkg/services/operator_settings/internal/services/resolution`

| Aspect | Rationale |
| --- | --- |
| Authority | Apply backend scope, environment, defaults, presets, and invocation overrides into an immutable effective selection. |
| State store | No independent store; reads Document and environment/effect ports. |
| Lifecycle | Pure resolution on command. |
| Consumers | Workers/Models/Sessions via Operator Settings root; may query Providers for canonical IDs. |
| Transaction boundary | Resolution does not mutate Document; callers persist via Document commands. |
| Failure recovery | Conflict/unsupported override failures are Resolution facts. |

### `provider_sessions`

Top-level owner · `pkg/services/provider_sessions`

| Aspect | Rationale |
| --- | --- |
| Authority | Secure resolution and read-only inspection of provider-owned persisted session artifacts, transcripts, reasoning, tools, parse errors, and usage. |
| State store | Resolved provider-session and transcript projections; no provider execution store. |
| Lifecycle | Read-only resolve/inspect lifecycle; no execution daemon. |
| Consumers | Factory Sessions, Workers (carry SessionRef), HTTP Provider Sessions adapters. |
| Transaction boundary | Readers share Provider Sessions root transaction for one SessionRef resolve; Providers owns SessionRef minting. |
| Failure recovery | Secure path/parse failures and unknown SessionRef errors are Provider Sessions facts. |

#### `provider_sessions/codex_reader`

Nested subservice of `provider_sessions` · `pkg/services/provider_sessions/internal/services/codex_reader`

| Aspect | Rationale |
| --- | --- |
| Authority | Secure JSONL rollout discovery, timestamped layouts, symlink containment, native parsing, normalized detail projection. |
| State store | Codex transcript discovery/projection state for a resolve. |
| Lifecycle | Read-only resolve path for Codex SessionRefs. |
| Consumers | Provider Sessions root registry. |
| Transaction boundary | Reader-local parse projection; no cross-owner writes. |
| Failure recovery | Symlink/path and parse errors normalize through Codex Reader. |

#### `provider_sessions/cursor_reader`

Nested subservice of `provider_sessions` · `pkg/services/provider_sessions/internal/services/cursor_reader`

| Aspect | Rationale |
| --- | --- |
| Authority | Cursor directory/store resolution, SQLite access, protobuf/blob decoding, transcript reconstruction, usage projection. |
| State store | Cursor store resolution/projection state for a resolve. |
| Lifecycle | Read-only resolve path for Cursor SessionRefs. |
| Consumers | Provider Sessions root registry. |
| Transaction boundary | Reader-local decode projection; no provider execution. |
| Failure recovery | Store/decode failures normalize through Cursor Reader. |

### `providers`

Top-level owner · `pkg/services/providers`

| Aspect | Rationale |
| --- | --- |
| Authority | Provider protocol, catalog/enumeration, identity and selection, availability/capabilities, SessionRef identity, native adapters, and one normalized execution attempt. |
| State store | Provider catalog descriptors and availability/capability snapshots; execution is attempt-scoped. |
| Lifecycle | Catalog refresh and one-shot Execute attempts; no Work retry policy. |
| Consumers | Workers (selection/retry), Operator Settings (validation), transports (`you providers list`), Provider Sessions (SessionRef). |
| Transaction boundary | Provider protocol, selection, session identity, adapter choice, and one Execute attempt are Providers-local; Workers owns dispatch retry/scheduling. |
| Failure recovery | Availability/prerequisite and native execution failures classify at Providers; Workers decides retry. |

#### `providers/catalog`

Nested subservice of `providers` · `pkg/services/providers/internal/services/catalog`

| Aspect | Rationale |
| --- | --- |
| Authority | Canonical provider IDs/aliases, deterministic list/get, metadata, availability/prerequisite probes, capability snapshots. |
| State store | Provider catalog and probe snapshots. |
| Lifecycle | List/get/refresh on command. |
| Consumers | Providers root, Operator Settings validation, Workers selection data. |
| Transaction boundary | Catalog mutations/snapshots are Providers-local. |
| Failure recovery | Probe and unknown-ID failures are Catalog facts. |

#### `providers/execution`

Nested subservice of `providers` · `pkg/services/providers/internal/services/execution`

| Aspect | Rationale |
| --- | --- |
| Authority | Own provider protocol selection, native adapter choice, stream decode, progress, diagnostics, optional SessionRef, and one normalized provider attempt. |
| State store | Attempt-scoped execution state; no durable Work store. |
| Lifecycle | One Execute attempt lifecycle. |
| Consumers | Workers via Providers root; Provider Sessions resolves emitted SessionRef. |
| Transaction boundary | Provider selection, session identity, adapter execution, and the attempt boundary are Providers-local; Workers owns when to invoke and any multi-attempt policy. |
| Failure recovery | Native adapter and failure-normalization faults are Execution facts. |

### `recordings`

Top-level owner · `pkg/services/recordings`

| Aspect | Rationale |
| --- | --- |
| Authority | Canonical event acceptance, ordering, replay, recording lifecycle, artifacts/export, subscriptions, and query projections. |
| State store | Canonical session event store, artifact store, and projection store. |
| Lifecycle | Recorder capture/flush lifecycle plus replay/delivery; projections reduce canonical facts. |
| Consumers | Factory Sessions, Factory Runtime, Workers, Visualization, HTTP/CLI dashboard/history consumers. |
| Transaction boundary | Ledger append/projection rebuild share Recordings canonical identity; peers emit facts, they do not write the ledger. |
| Failure recovery | Gap/cursor/retention, replay divergence, and recorder crash/finalization are Recordings recovery concerns. |

#### `recordings/artifacts_export`

Nested subservice of `recordings` · `pkg/services/recordings/internal/services/artifacts_export`

| Aspect | Rationale |
| --- | --- |
| Authority | Persist replay artifacts and build/validate/decode/atomically export privacy-bounded portable recordings. |
| State store | Artifact store and portable export packages. |
| Lifecycle | Export/materialize on command; atomic write policy. |
| Consumers | Recordings Replay and public export consumers. |
| Transaction boundary | Artifact writes are Recordings-local and privacy-bounded. |
| Failure recovery | Export validation and atomic-write failures are Artifacts/Export faults. |

#### `recordings/canonical_ledger`

Nested subservice of `recordings` · `pkg/services/recordings/internal/services/canonical_ledger`

| Aspect | Rationale |
| --- | --- |
| Authority | Validate, sequence, idempotently append, retain, and subscribe to canonical Factory events. |
| State store | Canonical session event store with cursor/gap/retention policy. |
| Lifecycle | Append/subscribe lifecycle for canonical facts. |
| Consumers | Runtime/Workers/Sessions emitters and Projection/Replay consumers via Recordings root. |
| Transaction boundary | Ledger append is the Recordings source-of-truth transaction. |
| Failure recovery | Idempotency, gap, and retention conflicts recover in Canonical Ledger. |

#### `recordings/projection_query`

Nested subservice of `recordings` · `pkg/services/recordings/internal/services/projection_query`

| Aspect | Rationale |
| --- | --- |
| Authority | Deterministically reduce canonical events into world-state and customer/dashboard views; validate reconnect projections. |
| State store | Projection store derived from canonical ledger. |
| Lifecycle | Projection rebuild/query lifecycle private to Recordings. |
| Consumers | Visualization, Sessions, dashboard transports via Recordings root. |
| Transaction boundary | Projection rebuild is Recordings-local; not a separate deployable in this program. |
| Failure recovery | Reconnect/projection divergence faults are Projection/Query recovery facts. |

#### `recordings/recording_lifecycle`

Nested subservice of `recordings` · `pkg/services/recordings/internal/services/recording_lifecycle`

| Aspect | Rationale |
| --- | --- |
| Authority | Select session recording target, capture events, periodic/final flush, error accumulation, crash/finalization. |
| State store | Recorder target and flush state for a session recording. |
| Lifecycle | Capture/flush/finalization lifecycle. |
| Consumers | Factory Sessions/Runtime emitters via Recordings root. |
| Transaction boundary | Recorder state is Recordings-local around canonical append. |
| Failure recovery | Crash/final flush failures recover in Recording Lifecycle. |

#### `recordings/replay`

Nested subservice of `recordings` · `pkg/services/recordings/internal/services/replay`

| Aspect | Rationale |
| --- | --- |
| Authority | Load/validate/hydrate historical recordings, deterministic clock, replay plans/hooks, delivery timing, divergence detection. |
| State store | Historical recording loads and replay plan handles. |
| Lifecycle | Replay delivery lifecycle with deterministic timing. |
| Consumers | Dashboard/CLI replay consumers and Artifacts/Export. |
| Transaction boundary | Replay plans are Recordings-owned; peers do not import replay implementations. |
| Failure recovery | Divergence and hydrate failures are Replay recovery facts. |

### `system_initialization`

Top-level owner · `pkg/services/system_initialization`

| Aspect | Rationale |
| --- | --- |
| Authority | Customer-invoked initialization workflow coordinating Operator Settings and Factory Definition without owning their stores. |
| State store | No independent datastore; observes initialization outcome only. |
| Lifecycle | Single Initialize command with idempotent/partial-failure reporting. |
| Consumers | CLI/HTTP `you init` adapters; commands Operator Settings and Factory Definitions roots. |
| Transaction boundary | Cross-owner ordering only; Settings and Definitions retain their own transactions/rollback. |
| Failure recovery | Partial failure/rollback reporting is Bootstrap-owned without absorbing peer stores. |

### `webhooks`

Top-level owner · `pkg/services/webhooks`

| Aspect | Rationale |
| --- | --- |
| Authority | Factory-configured outbound webhook subscriptions, canonical event filtering, signed delivery attempts, bounded retry/backoff, and session-owned dead-letter diagnostics; Recordings remains the event authority. |
| State store | Process-local endpoint delivery state and session-owned dead-letter JSONL; no canonical Factory state. |
| Lifecycle | Wire constructs the service once; Factory Sessions start and stop session-scoped endpoint subscribers and delivery workers. |
| Consumers | Factory Definitions configuration, Recordings canonical events, platform HTTP/clock/logging effects, and external webhook receivers. |
| Transaction boundary | Webhook delivery, retry, and dead-letter state remain Webhooks-local; canonical events are read-only input and terminal records use an injected runtime-storage effect. |
| Failure recovery | Secret, receiver, retry, cancellation, and dead-letter failures are structured Webhooks diagnostics and never mutate or block Factory Session or Work state. |

### `work`

Top-level owner · `pkg/services/work`

| Aspect | Rationale |
| --- | --- |
| Authority | Work Request admission, secure content staging/materialization, invocation/return policy, lineage, state commands, and detached queries. |
| State store | Work and dispatch store plus staged content references. |
| Lifecycle | Admission and state commands; no scheduling daemon (Automations owns triggers). |
| Consumers | Automations, Factory Runtime, Factory Sessions, Workers, Recordings, Definitions, transports. |
| Transaction boundary | Admission/content/state share Work identity and mutation invariants. |
| Failure recovery | Acceptance/rejection, staging/materialization, and state-command failures are Work facts. |

#### `work/admission`

Nested subservice of `work` · `pkg/services/work/internal/services/admission`

| Aspect | Rationale |
| --- | --- |
| Authority | Normalize, validate, deduplicate, authorize, and accept Work Requests; emit acceptance/rejection facts. |
| State store | Accepted Work Request records in Work store. |
| Lifecycle | Admission is command-driven. |
| Consumers | Automations and public protocols via Work root; Runtime consumes admitted Work. |
| Transaction boundary | Acceptance is the Work intake transaction. |
| Failure recovery | Validation/authorization rejections are Admission facts. |

#### `work/content_materialization`

Nested subservice of `work` · `pkg/services/work/internal/services/content_materialization`

| Aspect | Rationale |
| --- | --- |
| Authority | Resolve staged/local/remote/data-URL content with cache, SSRF, size, cleanup, and cancellation policy. |
| State store | Materialization cache and resolved content handles. |
| Lifecycle | Resolve/cleanup lifecycle for content fetches. |
| Consumers | Workers and Work root consumers needing immutable content. |
| Transaction boundary | Materialization policy is Work-local; Workers do not import Work modules. |
| Failure recovery | SSRF/size/cancellation failures are Materialization facts. |

#### `work/content_staging`

Nested subservice of `work` · `pkg/services/work/internal/services/content_staging`

| Aspect | Rationale |
| --- | --- |
| Authority | Securely persist submitted content, issue/verify opaque references, enforce expiry and owned-path policy. |
| State store | Staged content reference store and owned paths. |
| Lifecycle | Stage/expire/cleanup lifecycle. |
| Consumers | Work Admission and Materialization via Work root. |
| Transaction boundary | Staging writes share Work content transaction. |
| Failure recovery | Expiry and path-policy violations are Staging facts. |

#### `work/state_access`

Nested subservice of `work` · `pkg/services/work/internal/services/state_access`

| Aspect | Rationale |
| --- | --- |
| Authority | Adapt Factory Sessions root for submit/move/read snapshot operations and detached list/get/move results. |
| State store | Detached Work query/projection results over Work store; never exposes Runtime/Petri state. |
| Lifecycle | Read/move commands without peer store access. |
| Consumers | Transports and peer services via Work root; may query Sessions/Recordings roots. |
| Transaction boundary | State Access does not write Sessions/Recordings stores. |
| Failure recovery | Not-found and move conflicts are State Access facts. |

### `worker_sessions`

Top-level owner · `pkg/services/worker_sessions`

| Aspect | Rationale |
| --- | --- |
| Authority | Owns the live Worker Session lifecycle for one Factory dispatch: session identity, start/control, retained observation, terminal outcome, publication, and dispatch association. |
| State store | Process-local Worker Session records and retained observations backed by the Events stream; durable Factory history remains owned by Recordings. |
| Lifecycle | Wire constructs the service once; start, control, publish, read, stream, and terminalization operations share the Worker Session lifecycle and close with the process. |
| Consumers | Workers dispatch, Factory Sessions, Events, Chat Sessions response bridging, and CLI/HTTP Worker Session transports. |
| Transaction boundary | Worker Session lifecycle and publication state are session-local; Workers decides when a request-scoped attempt runs while Worker Sessions records and exposes its observable session. |
| Failure recovery | Invalid lifecycle transitions, stale controls, missing sessions, publication conflicts, retention gaps, and terminalization failures remain typed Worker Session facts. |

### `workers`

Top-level owner · `pkg/services/workers`

| Aspect | Rationale |
| --- | --- |
| Authority | Request-scoped runner selection/assembly, workstation behavior, capacity, dispatch execution policy, retry, and Work results; Providers owns provider protocol and one native attempt. |
| State store | Worker registry/execution store and immutable runtime assembly artifacts. |
| Lifecycle | Dispatch execution and runner lifecycles; Providers owns native provider attempts. |
| Consumers | Factory Runtime dispatch, Factory Sessions, Models, Providers, Work, Recordings. |
| Transaction boundary | Execution/retry policy is Workers-local; Providers Execute is one attempt command. |
| Failure recovery | Dispatch/retry/cancellation failures normalize as Workers execution facts. |

#### `workers/runners`

Nested subservice of `workers` · `pkg/services/workers/internal/services/runners`

| Aspect | Rationale |
| --- | --- |
| Authority | Common private Runner contract/registry implemented by agent/script/inference/hosted runners. |
| State store | Runner registry identities/capabilities; execution state lives in runner implementations. |
| Lifecycle | Registry selection for one dispatch; runners own attempt execution. |
| Consumers | Runtime Assembly and Workstation Execution. |
| Transaction boundary | Registry is Workers-private; provider adapters live under Providers. |
| Failure recovery | Unknown runner/capability mismatches are registry rejection facts. |

##### `workers/runners/agent`

Nested subservice of `workers/runners` · `pkg/services/workers/internal/services/runners/internal/services/agent`

| Aspect | Rationale |
| --- | --- |
| Authority | Implement Runner for agent harness/tools, retries, stop tokens, decision envelopes, terminal results. |
| State store | Agent attempt execution state under Workers. |
| Lifecycle | Agent runner attempt lifecycle. |
| Consumers | Workers Runner registry; may command Providers/Models through Workers policy. |
| Transaction boundary | Agent attempt is Workers-local; Providers owns native provider attempt details. |
| Failure recovery | Harness/retry/stop-token failures are Agent Runner facts. |

##### `workers/runners/hosted`

Nested subservice of `workers/runners` · `pkg/services/workers/internal/services/runners/internal/services/hosted`

| Aspect | Rationale |
| --- | --- |
| Authority | Implement Runner for hosted execution request/result, remote lifecycle observation, cancellation, normalized outcome. |
| State store | Hosted execution attempt state under Workers. |
| Lifecycle | Hosted runner attempt lifecycle (not hosted polling). |
| Consumers | Workers Runner registry; Automations Hosted Sources remain separate. |
| Transaction boundary | Hosted Work execution is Workers-local; source polling is Automations. |
| Failure recovery | Remote lifecycle/cancellation failures are Hosted Runner facts. |

##### `workers/runners/inference`

Nested subservice of `workers/runners` · `pkg/services/workers/internal/services/runners/internal/services/inference`

| Aspect | Rationale |
| --- | --- |
| Authority | Implement a request-scoped Runner for model binding/readiness, provider invocation through Providers, and output mapping; it does not own provider protocol, selection, session, adapter, or native execution behavior. |
| State store | Inference runner attempt state under Workers. |
| Lifecycle | Request-scoped worker attempt lifecycle coordinating Models and one Providers attempt. |
| Consumers | Workers Runner registry, Models Inference, and Providers root. |
| Transaction boundary | Workers decides when and which worker attempt runs; Providers owns provider protocol, selection, session, adapter, and one native attempt while Models owns local model readiness/inference. |
| Failure recovery | Binding/readiness/output-mapping failures are Inference Runner facts. |

##### `workers/runners/script`

Nested subservice of `workers/runners` · `pkg/services/workers/internal/services/runners/internal/services/script`

| Aspect | Rationale |
| --- | --- |
| Authority | Implement Runner for script command policy, process execution, progress/events, timeout, and result. |
| State store | Script attempt execution state under Workers. |
| Lifecycle | Script runner attempt lifecycle. |
| Consumers | Workers Runner registry. |
| Transaction boundary | Script attempt is Workers-local. |
| Failure recovery | Timeout/process failures are Script Runner facts. |

#### `workers/workstations`

Nested subservice of `workers` · `pkg/services/workers/internal/services/workstations`

| Aspect | Rationale |
| --- | --- |
| Authority | Context/environment, interpolation, behavior routing, dispatch, and final output. |
| State store | Workstation execution context for a dispatch. |
| Lifecycle | Dispatch execution path lifecycle under Workers. |
| Consumers | Runner registry and Factory Runtime dispatch consumers via Workers root. |
| Transaction boundary | Workstation execution shares Workers dispatch transaction. |
| Failure recovery | Routing/dispatch failures are Workstation Execution facts. |

## Responsibility Clusters

These are large responsibility clusters that stay under a committed owner
without becoming nested services of their own.

| Owner | Cluster | Name | Note |
| --- | --- | --- | --- |
| `automations` | `time_work_retry_policy` | Time-work parsing and retry classification | Plain internal modules for scheduling/value policy; not nested services. |
| `factory_definitions` | `policy_modules` | Decision-envelope and compatibility policy modules | Stateless Definition policy derivations remain parent-internal modules. |
| `factory_runtime` | `engine_tick_subsystems` | Engine tick loop and ordered subsystems | Plain private implementation sharing engine state; not independently addressable services. |
| `factory_runtime` | `orchestration_variants` | Petri and JavaScript orchestration variants | Internal Orchestration variants sharing Runtime lifecycle/checkpoints; not top-level peers. |
| `factory_sessions` | `request_validation_modules` | Request preparation and validation modules | Stateless normalization/validation remain plain modules under Sessions. |
| `factory_visualization` | `sink_codec_adapters` | Sink/codec/source adapters | Protocol mechanics stay private modules or Visualization-owned transport adapters. |
| `models` | `effect_adapters` | Source/cache/process/HTTP effect adapters | Construction-time effect ports without product authority or root interfaces. |
| `operator_settings` | `identity_input_modules` | Identity inventory and input-index parsing | Pure document discovery/parsing modules without independent authority. |
| `provider_sessions` | `reader_support_modules` | Path/blob/token/transcript support modules | Format implementation details beneath owning readers. |
| `providers` | `native_adapters` | Native provider adapters | Codex/Claude/Cursor/OpenCode/Gemini/Kiro/Pi/Agy variants remain Execution-private modules. |
| `recordings` | `projection_policy` | Projection policy helpers | Pure projection helpers stay under Recordings; Projection is not a top-level peer in this program. |
| `system_initialization` | `legacy_migration` | Legacy Factory migration module | Deletion-only private module, not a nested subservice; removed when input inventory reaches zero. |
| `work` | `invocation_return_policy` | Invocation input and return-policy modules | Deterministic parent-internal transformations exposed only through the Work root. |
| `work` | `lineage_graph_modules` | Lineage and dependency-graph modules | Deterministic projections over Work contracts used by State Access; no separate store. |
| `workers` | `prompting_worktree` | Prompting and worktree preparation | Request-scoped Workers modules without independent lifecycle. |
| `workers` | `runner_registry` | Runner contract and registry mechanics | Private Workers registry backing runner subservices; not a separate product owner. |

## Public Surfaces And Their Durable Owners

Behavior tests and public CLI, HTTP, MCP, replay, and visualization surfaces
each map to exactly one durable service owner.

| Surface | Kind | Path | Durable owner | Note |
| --- | --- | --- | --- | --- |
| `behavior_test:workers-hosted-poller` | behavior_test | `pkg/services/workers/services/hosted_logic` | `automations` | Hosted poller/source behavior coverage under Workers hosted_logic moves with Automation Hosted Sources; Workers retains hosted runner execution tests only. |
| `cli:docs-workers-provider-topics` | cli | `docs/reference/workers.md` | `providers` | Provider enumeration/modelProvider CLI docs burn down to Providers Catalog; Workers docs retain selection/retry/runner policy only. |
| `cli:you-providers-list` | cli | `pkg/transports/cli` | `providers` | you providers list and provider-facing CLI adapters consume Providers Catalog as the durable owner. |
| `http:openapi-worker-provider-fields` | http | `api/components` | `providers` | OpenAPI provider identity/capability fields stay transport-shaped but durable domain owner is Providers. |
| `mcp:factory-session-tools` | mcp | `pkg/transports/mcp` | `factory_sessions` | MCP Factory Session tools adapt Factory Sessions; they are not Workers-owned provider surfaces. |
| `replay:recordings-replay` | replay | `pkg/services/recordings/wire` | `recordings` | Replay and artifact reconstruction remain Recordings-owned public surfaces through recordings/wire after DEL-REC removed the transitional replay/ shim. |
| `visualization:factory-visualization` | visualization | `pkg/services/factory_visualization` | `factory_visualization` | Dashboard/live-view visualization remains Factory Visualization-owned. |

## Owned Roles

Every constructor, datastore, lifecycle role, and protocol adapter belongs to
exactly one committed destination.

### Constructors

| Role | Name | Destination | Note |
| --- | --- | --- | --- |
| `constructor:automations` | Automations service constructor | `automations` | Wire constructs Automations at the Automations root. |
| `constructor:factory_definitions` | Factory Definitions service constructor | `factory_definitions` | Wire constructs Factory Definitions at the Definitions root. |
| `constructor:factory_runtime` | Factory Runtime service constructor | `factory_runtime` | Wire constructs Factory Runtime at the Runtime root. |
| `constructor:factory_sessions` | Factory Sessions service constructor | `factory_sessions` | Wire constructs Factory Sessions at the Sessions root. |
| `constructor:factory_visualization` | Factory Visualization service constructor | `factory_visualization` | Wire constructs Factory Visualization at the Visualization root. |
| `constructor:models` | Models service constructor | `models` | Wire constructs Models via models/wire only. |
| `constructor:operator_settings` | Operator Settings service constructor | `operator_settings` | Wire constructs Operator Settings at the Operator Settings root. |
| `constructor:provider_sessions` | Provider Sessions service constructor | `provider_sessions` | Wire constructs Provider Sessions at the Provider Sessions root. |
| `constructor:providers` | Providers service constructor | `providers` | Providers root constructor is the durable destination for Workers provider extraction. |
| `constructor:recordings` | Recordings service constructor | `recordings` | Wire constructs Recordings at the Recordings root. |
| `constructor:system_initialization` | System Bootstrap service constructor | `system_initialization` | Wire constructs System Bootstrap at the System Bootstrap root. |
| `constructor:work` | Work service constructor | `work` | Wire constructs Work at the Work root. |
| `constructor:workers` | Workers service constructor | `workers` | Wire constructs Workers at the Workers root for selection/retry/runners only. |

### Datastores

| Role | Name | Destination | Note |
| --- | --- | --- | --- |
| `datastore:automationStore` | automationStore | `automations` | structures.md Automation datastore. |
| `datastore:definitionStore` | definitionStore | `factory_definitions` | structures.md Factory Definition datastore. |
| `datastore:ledgerStore` | ledgerStore | `recordings` | structures.md Session Ledger datastore. |
| `datastore:modelStore` | modelStore | `models` | structures.md Model Runtime datastore. |
| `datastore:providerSessionStore` | providerSessionStore | `provider_sessions` | structures.md Provider Session datastore. |
| `datastore:runtimeStore` | runtimeStore | `factory_runtime` | structures.md Factory Runtime datastore. |
| `datastore:sessionStore` | sessionStore | `factory_sessions` | structures.md Factory Session datastore. |
| `datastore:workStore` | workStore | `work` | structures.md Work datastore. |
| `datastore:workerStore` | workerStore | `workers` | structures.md Worker Execution datastore. |

### Lifecycle roles

| Role | Name | Destination | Note |
| --- | --- | --- | --- |
| `lifecycle_role:automations-hosted-sources` | Automation Hosted Sources lifecycle | `automations` | Hosted polling/source reconciliation lifecycle is owned by Automations. |
| `lifecycle_role:factory-session` | Factory Session lifecycle | `factory_sessions` | Desired/observed session lifecycle stays Factory Sessions-owned. |
| `lifecycle_role:initializer` | Initializer process lifecycle | `initializer` | Process start/stop/join remains Initializer-owned. |
| `lifecycle_role:provider-session` | Provider Session lifecycle | `provider_sessions` | Provider session transcript/lifecycle stays Provider Sessions-owned. |
| `lifecycle_role:providers-execution` | Providers Execution attempt lifecycle | `providers` | Normalized provider inference attempt lifecycle is owned by Providers Execution. |
| `lifecycle_role:recordings` | Recordings lifecycle | `recordings` | Ledger/recording lifecycle stays Recordings-owned. |
| `lifecycle_role:workers-runners` | Workers runner attempt lifecycle | `workers` | Work selection/retry and runner attempts remain Workers-owned. |

### Protocol adapters

| Role | Name | Destination | Note |
| --- | --- | --- | --- |
| `protocol_adapter:cli` | CLI transport adapter | `transports` | CLI protocol adapter stays under transports; domain owners remain separate. |
| `protocol_adapter:http` | HTTP/REST/SSE transport adapter | `transports` | HTTP protocol adapter stays under transports. |
| `protocol_adapter:mcp` | MCP transport adapter | `transports` | MCP protocol adapter stays under transports. |
| `protocol_adapter:visualization-sink` | Factory Visualization presentation adapter | `factory_visualization` | Visualization presentation adapters remain Factory Visualization-owned. |

## Deferred Architecture Debt

Work the Packaged Service Structure program deliberately left outside its
migration packets. These are recorded decisions, not tracked ratchets.

| ID | Package | Deferred work |
| --- | --- | --- |
| FND-06 | `pkg/services/edges` | Narrow Edges implementation imports and the broad external-effect surface; deferred to FND-06. This packet only records the architecture exception. |
