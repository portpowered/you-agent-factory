# Transport and mapping convergence audit

The service packages have started to acquire service-local `transports/cli`,
`transports/http`, and `transports/mcp` packages, but the application still has
two competing transport architectures:

1. service-local adapters that are intended to call one service root; and
2. legacy top-level CLI, HTTP, and mapping packages that compose services,
   recreate service interfaces, perform domain decisions, and sometimes build
   runtime or application scopes.

The target is to make every protocol path have this shape:

```text
protocol input
  -> top-level protocol registrar/forwarder
  -> one owning service transport adapter receiving the raw protocol request
  -> one owning service root operation
  -> protocol result/error mapping
```

The top-level CLI, HTTP, and MCP packages retain only command-node creation,
route/tool registration, aggregate protocol dispatch, and direct forwarding of
raw protocol values. They do not decode an operation into domain values,
resolve inputs, map results, render service output, or classify service errors.

A service-local transport receives the direct protocol objects: Cobra command
and arguments for CLI, `http.ResponseWriter`/`*http.Request` plus generated path
or query parameters for HTTP, and raw tool arguments for MCP. It owns all
operation-specific decoding, input-source resolution, representation
validation, request/result mapping, rendering, streaming, and typed-error
translation before and after calling its one service root.

This plan is aligned with:

- `docs/internal/standards/code/general-backend-standards.md`
- `docs/internal/standards/code/planning-standards.md`
- `docs/architecture/packaged-structure.md`
- `docs/architecture/data-model.md`
- `docs/temp/projects/packaged-service-structure/package-service-factory-definitions.md`
- `docs/temp/projects/packaged-service-structure/package-service-factory-runtime.md`
- `package-service-factory-sessions.md`
- `package-service-workers.md`

## 1. Audit conclusion

One new product-domain service is required: **Documentation**. The packaged
topic catalog, aliases, ordering, descriptions, embedded document lookup, and
unsupported-topic behavior are customer-visible capabilities independent of
Cobra. They currently live in `pkg/transports/cli/docs`, but their correct home
is `pkg/services/documentation` with a CLI adapter under
`pkg/services/documentation/transports/cli`.

No other new product-domain service is required by the audited logic.

The apparent missing domains are already owned:

| Misplaced concern | Correct owner |
| --- | --- |
| Factory authored decoding, compatibility rejection, validation, persistence, catalogs, Current Factory, invocation signature/default Work type | Factory Definitions |
| Session start, invocation, unified live/persisted listing, lifecycle, result selection, ephemeral response events | Factory Sessions |
| Runtime observation, control, dispatch planning, checkpoint behavior, workflow preview | Factory Runtime |
| Work admission, content staging/materialization, request preparation, default application, reads, moves, lineage | Work |
| Canonical Factory Events, reconnect validation, replay, dispatch history, artifacts, historical projections | Recordings |
| Worker prompt contracts, runner diagnostics, worker execution vocabulary | Workers |
| Model catalog, runtime scope, asset lifecycle, readiness, direct model invocation | Models |
| Provider catalog, ACP execution/configuration application, provider execution | Providers |
| Operator configuration documents, ACP persistence, defaults and effective settings | Operator Settings |
| Provider transcript/session inspection | Provider Sessions |
| Dashboard and terminal visualization projections, event/result presentation | Factory Visualization |
| Process bootstrap and rollback | System Initialization |
| Packaged reference topic catalog and document retrieval | Documentation (new) |
| Application run/server/MCP activation and unwind | `pkg/initializer` |
| Command-tree construction, route/tool registration, aggregate forwarding, and generated contracts | top-level `pkg/transports` |
| Reusable policy-free Cobra/input, HTTP client/response, SSE/NDJSON, terminal, and stdio mechanics used by owner adapters | focused `pkg/platform` packages |
| CLI/OpenAPI/MCP generation, parity checks, snapshots, and baselines | build/test tooling, not a runtime service |

The audit requires these new package locations and one new durable authority:

- `pkg/services/documentation` with `wire` and `transports/cli`;
- `pkg/initializer/transports/cli` for application `run`, `server`, and MCP
  lifecycle command adaptation;
- `pkg/services/workers/transports/http` for Worker-owned OpenAPI diagnostic
  and prompt-contract mapping used by outward representations;
- focused `pkg/platform` packages for policy-free CLI/HTTP/SSE/terminal/stdio
  mechanics used by multiple owner adapters;
- service-local mapping files or subpackages under each owning service's
  transport.

Do not create an `application`, `transport_services`, `api_facade`,
`presentation_services`, `command_surface`, or `system_status` product service.
Those names would hide the same cross-owner composition instead of assigning
each decision. CLI manifests and OpenAPI/MCP catalogs are protocol contracts,
not a cross-protocol product authority; unlike Documentation, they remain
transport/build artifacts.

## 2. Two transport levels with different rules

### The only legitimate top-level transport behavior

- Build the Cobra root and command nodes from the authored/generated CLI
  contract.
- Register generated HTTP routes and non-OpenAPI protocol routes.
- Register MCP tools from the generated discovery catalog.
- Attach already-constructed service transport handlers to those nodes,
  routes, and tools.
- Forward the untouched operation-level protocol objects to the selected
  service transport.
- Handle only protocol-shell failures that occur before an owner is selected,
  such as an unknown route, unknown command, malformed generated path
  parameter, or MCP tool name absent from the generated registry.

The forwarding signatures are intentionally raw:

```go
// CLI
type Handler func(*cobra.Command, []string) error

// HTTP: generated signatures are forwarded unchanged.
func (a *Aggregate) SubmitWorkBySessionId(
    w http.ResponseWriter,
    r *http.Request,
    sessionID generated.SessionID,
) {
    a.work.SubmitWorkBySessionId(w, r, sessionID)
}

// MCP
type ToolHandler func(context.Context, json.RawMessage) (json.RawMessage, error)
```

Top-level transports do not turn those objects into resolved-input records,
service requests, response DTOs, or presentation output.

### Legitimate service-local transport behavior

- Read direct Cobra arguments/flags/inherited streams, HTTP
  request/path/query/header/body values, or raw MCP tool arguments.
- Apply protocol-specific input precedence and representation validation.
- Enforce representation syntax: required JSON member, enum spelling, one JSON
  object, path/body identifier equality, base64 validity, content type, query
  syntax, or CLI flag exclusivity.
- Map protocol values to one owner root request and call that root.
- Map owner results and typed errors to CLI output/exit errors, HTTP
  status/body/headers, or MCP results.
- Render owner results and frame/flush owner streams as SSE or NDJSON.
- Propagate protocol cancellation and protocol-specific stream deadlines.
- Use focused policy-free Platform helpers for repetitive mechanics.

### Logic that must leave every transport

- Select a Factory, Work type, worker, workstation, provider, model, runtime,
  or session mode.
- Merge live and persisted resources or infer canonical lifecycle state.
- Apply defaults derived from Factory definitions or operator configuration.
- Validate semantic topology, Work rules, prompt contracts, runtime policy, or
  execution compatibility.
- Stage/materialize Work content as part of a transport-owned workflow.
- Decide retryability, result precedence, replay validity, checkpoint
  recovery, or terminal success from Petri tokens/dispatch history.
- Load or persist product files directly when the owner service already owns
  that filesystem behavior.
- Construct a service, runtime scope, worker pool, model scope, HTTP service
  table, or initializer graph.
- Type-assert for optional product capabilities.
- Define service-shaped interfaces over generated API values.
- Join two or more service roots to manufacture an operation.

Syntax and semantics must remain distinct. For example, HTTP may reject a
missing `workTypeName` field shape, but only Work and Factory Definitions may
decide which Work type is effective or whether it exists.

## 3. Current broken dependency contracts

### T1 — `pkg/transports/mapping` is an application layer

`mapping/contract.go` publishes `RuntimeAPI`, `LiveSessionAPI`, `WorkAPI`,
`InvocationAPI`, and four durable session facet interfaces. The interfaces use
generated OpenAPI values and duplicate service roots.

`mapping/composition` then constructs these façades, binds live runtime roles,
type-asserts `factoryruntime.APIFactory`, joins Work with Sessions event access,
and returns an `HTTPBinding` service table.

Required change: delete every operational interface and constructor from
`pkg/transports/mapping`. Pure representation functions move to the owning
service transport. `pkg/wire` injects roots directly into their adapters.

### T2 — HTTP binding performs secondary injection

`pkg/transports/http/application.Handler.Bind` receives
`factorysessions.RuntimeHTTPServices`, creates Models and Sessions adapters at
runtime, builds another mapped service graph, and supplies the Sessions HTTP
adapter with a large dependency bag.

Required change: `pkg/wire` constructs every inert owner adapter once from its
owner root. `pkg/initializer` activates the already-built HTTP role. No
`RuntimeHTTPServices`, `HTTPBinder`, `HTTPBinding`, `BindDurableExecution`, or
runtime adapter construction remains.

### T3 — the Sessions HTTP adapter is a multi-service application façade

`factory_sessions/transports/http.Adapter` currently has fields for Sessions,
Runtime, Factory status, Work, Work reads, invocation, Factory Definitions,
Factory validation, workflow preview, four durable facets, two list readers,
Worker prompts, invocation Work-type policy, Work staging, and request
preparation.

That is the clearest boundary failure in the HTTP surface. The adapter owns
Factory, Work, Runtime, Recordings, and Workers routes in addition to Sessions.

Required change: leave only `factorysessions.Service`, a logger/protocol
writer, and protocol-only settings in this adapter.

### T4 — CLI root is an application operation container

`pkg/transports/cli.CommandOperations` and `CommandFactory` aggregate domain
operations, service fragments, effects, filesystem/home/environment readers,
runtime builders, initializers, HTTP clients, and presentation behavior.
`root.go` and `root_work.go` then resolve Factory selection, operator defaults,
invocation input, Current Factory behavior, runtime opening, and command
execution.

Required change: the CLI root owns only the command tree, global protocol
flags, dispatch to already-built command-family adapters, and Cobra/process
execution. Each owner adapter translates its own typed errors before returning;
the top-level executor does not classify product failures. Each family comes
from its owner transport. Application role commands come from Initializer's
CLI adapter.

### T5 — service-local transports still discover peers

Several service transports import and call peer domains:

- Factory Sessions HTTP imports Definitions, Runtime, Recordings, Work, and
  Workers.
- Models HTTP imports Work and Workers; Models CLI imports Sessions, Operator
  Settings, Work, and Workers.
- Factory Runtime CLI imports Definitions, dashboard transport mechanics, and
  HTTP server mechanics.
- Factory Visualization CLI imports Definitions, Sessions, Work, Workers, and
  the old dashboard package.
- Work HTTP imports Factory Runtime for move error/state behavior.
- Recordings HTTP imports Factory Definitions for event contracts/projection.
- Factory Definitions CLI/MCP imports process initialization and performs
  product filesystem resolution at the protocol edge.

Required change: a service transport imports its owning root, generated
protocol contracts, and protocol mechanics. Peer-service coordination happens
inside the owning service implementation through directly injected peer roots.

### T6 — identical Work behavior exists in two HTTP adapters

Structured Work item validation, staging request decoding, staged item
conversion, Work Request mapping, legacy state normalization, and submission
response mapping exist in both:

- `factory_sessions/transports/http/handlers_work_*.go`; and
- `work/transports/http/admission_mapping.go` plus related Work handlers.

Required change: Work HTTP is the only implementation. Sessions must not
retain forwarding handlers or copies after generated route composition points
at the Work adapter.

### T7 — transport computes default Work policy

Sessions HTTP loads the Current Factory, maps OpenAPI back into a Factory
definition, calls `InvocationWorkTypeService.DefaultWorkType`, then gives that
value to Work request preparation.

Required change: expose one Work admission operation accepting the public Work
request and session identity. Work obtains required definition-derived policy
through a narrow Factory Definitions peer injected into Work, or receives the
resolved admission policy from Sessions as part of an owner operation. HTTP
does not perform the join.

### T8 — CLI infers invocation completion from engine internals

`pkg/transports/cli/run/run_clean_invocation.go` searches Petri snapshots,
terminal tokens, and dispatch history to decide success and select output.
`run/run.go` also counts token states.

Required change: Factory Sessions `Invoke`/`ReadResult` returns the canonical
terminal invocation result; Factory Runtime returns customer-level
observation counts. CLI only renders those results. Petri token/place/state
types disappear from CLI imports.

### T9 — Models transport invokes Workers and opens Sessions scopes

Models HTTP prepares content through Work and invokes a `workers.ModelInvoker`.
Models CLI adapts `factorysessions.ModelsCLIPresentationCollaborator`, opens a
Sessions-owned Models scope, and carries Factory/Operator Settings bootstrap
values.

Required change: `models.Service.InvokeModel` is the single invocation
authority. Models internally consumes Work content preparation and any provider
execution dependency. Runtime scope is a Models-owned operation built once;
Factory Sessions is not a Models CLI bridge.

### T10 — ACP CLI combines persistence, live configuration, and catalog policy

`pkg/transports/cli/acp.Service` loads Operator Settings, configures Providers
through a type assertion, lists providers, identifies custom entries, filters
ACP identities by suffix, and persists add/delete changes.

Required change:

- Operator Settings owns add/delete/config document operations.
- Providers owns applying effective ACP configuration and returns descriptors
  containing source and integration kind.
- Initializer applies effective settings to Providers once.
- Providers CLI renders `providers.ListProviders`; it does not join the two
  roots or infer ACP identity from a `-acp` suffix.

The customer command may retain the `you workers acp` spelling for
compatibility while its handler is supplied by Providers/Operator Settings.

### T11 — CLI init constructs a service in the transport

`pkg/transports/cli/initsetup.NewConfigurer` calls
`operator_settings/wire.NewServiceFromConfigDocument` during command execution.

Required change: Wire constructs `operator_settings.Service` once. The CLI
adapter calls a root configuration operation. Packaged ACP defaults and line
reader effects are injected into the root or passed as request values, not
used to build a second service.

### T12 — Factory replacement advances version in CLI

`pkg/transports/cli/factory/replace_current.go` builds the save request and
advances a Hybrid Logical Timestamp before persistence.

Required change: Factory Definitions owns optimistic version progression and
stale-version enforcement. CLI sends the caller's observed version and replace
intent, then renders the returned definition.

### T13 — representation compatibility policy is scattered

Legacy field rejection lives under
`mapping/factorydefinition/retiredboundary`; Factory config conversion spans
multi-thousand-line `mapping/factoryconfig` files; Work legacy state
normalization exists in two handlers; global config defaults are assigned in
`mapping/globalconfig`.

Required change:

- Protocol-only aliases remain in the relevant owner transport and have an
  explicit removal contract.
- Authored Factory and operator configuration compatibility, defaults, and
  persistence format rules move to their owning service codec/validation
  implementation.
- Generated OpenAPI conversion remains in the owner HTTP transport.

### T14 — event and artifact ownership is split

Sessions HTTP and `mapping/factorysession` expose canonical event replay,
dispatch queries, and artifact queries while Recordings already has HTTP
adapters for events and artifacts. `mapping/factoryeventprojection` and
`http/workstationprojection` also map Recordings views outside its transport.

Required change: Recordings owns all canonical event, dispatch-history,
artifact, workstation-history, and reconnect projection routes/mappers.
Factory Sessions retains only ephemeral `FactoryResponseEvent` streaming and
session-level links.

### T15 — status and dashboard projections are composed at transport edges

`mapping/composition/factory_status.go` chooses unscoped Runtime observation or
session-proxied observation. `pkg/transports/cli/dashboard` derives queue,
active execution, failure, provider-session, and fallback Work views from
Recordings/Runtime/Work/Workers values.

Required change:

- Factory Runtime owns status observation by runtime/session binding.
- Factory Visualization owns dashboard view projection and presentation.
- CLI/HTTP render an already projected status/view.

### T16 — transport error classification depends on strings and internal errors

Examples include `configFailurePhase`, Work error prefix/substring checks,
factory persistence message rewriting, ACP suffix classification, and transport
knowledge of Runtime move errors.

Required change: owners return typed errors with stable category, target, and
retryability. Transports switch only on those public owner errors. Human
wording remains in the transport where it is presentation-specific.

### T17 — prompt-contract authoring joins Definitions and Workers in Sessions HTTP

The Current Factory workstation prompt-contract routes load a Factory through
the Definitions façade, search for a workstation in the generated API shape,
derive bundled document paths, and invoke Worker prompt validation from the
Sessions handler.

Required change: these are Factory authoring operations, not Session
lifecycle operations. Factory Definitions exposes workstation prompt-contract
and validation operations over its canonical definition values. The private
implementation may use Workers-owned pure prompt-template validation; the HTTP
adapter calls Definitions once. Keep actual invocation-time prompt rendering in
Workers.

### T18 — packaged Factory catalog bypasses Factory Definitions

The top-level `handlers_models.go` also serves `ListPackagedFactories` by
reading the internal packaged Factory catalog directly. The route is colocated
with Models only because of server history.

Required change: Factory Definitions HTTP owns the packaged Factory route and
calls `factorydefinitions.Service.ListBuiltInPackagedFactories`. Top-level HTTP
must not import `internal/packagedfactorycatalog`.

### T19 — packaged Documentation is modeled as a CLI utility

`pkg/transports/cli/docs` owns the stable topic vocabulary, aliases, display
order, descriptions, embedded filesystem, topic lookup, unsupported-topic
error, quick-start content, and index generation. `commandregistry` then owns
the operation and output behavior. This is a complete customer capability
split across two transport packages without a service root.

Required change: add `documentation.Service` with `List` and `Get` operations.
The service owns topic identity, aliases, ordering, metadata, document lookup,
and typed not-found failures. Its CLI transport receives the raw Cobra command
and arguments, calls the root, and renders index or Markdown output. The
canonical Markdown remains authored under `docs/reference`; the embedded
filesystem is injected by `documentation/wire`.

### T20 — top-level protocol packages transform requests before forwarding

The CLI root and `climanifestcobra` currently resolve inputs, combine defaults,
construct command-family configs, and select handler variants before calling a
service adapter. The HTTP server embeds the Sessions adapter and directly owns
Models, Provider Sessions, packaged Factory, and dashboard handlers. MCP server
accepts a Factory Sessions `ToolOperation` rather than an owner-neutral set of
raw handlers.

Required change: top-level CLI passes `*cobra.Command` and `[]string` directly
to an owner handler; service CLI adapters read their own flags/arguments and
streams. Top-level HTTP forwards each generated method signature unchanged to
the owner HTTP adapter. Top-level MCP registers generated definitions and
forwards raw JSON arguments to the handler registered for that tool. No
operation-specific request or response transformation remains at the top
level.

## 4. Top-level CLI package audit

Every current production package is classified below. “Retain” means retain as
protocol mechanics, normally under an `internal` subtree. “Move” means the
command behavior goes to the named owner transport. Compatibility wrappers are
temporary and must be deleted in the same migration story.

| Current CLI package | Business logic found | Final disposition |
| --- | --- | --- |
| `cli` root files | Cross-domain operation bag; Factory/operator/default/input/runtime selection; command execution | Reduce to node attachment and raw-handler forwarding; owner adapters and Initializer receive direct Cobra objects |
| `acp` | Settings persistence + Provider configuration/catalog join + ACP classification | Split between Operator Settings and Providers CLI/root operations |
| `baseline` | CLI snapshot/test baseline mechanics | Move to CLI contract test/tool support; never imported by runtime command code |
| `batchload` | Batch Work input loading | Move to Work CLI decode; semantic batch validation in Work |
| `clicontract` | Generated CLI contract verification | Keep only as transport build/test tooling |
| `clidiag` | Diagnostic formatting | Move reusable mechanics to `pkg/platform/terminal`; each owner CLI chooses messages |
| `clihttp` | Generic HTTP request/response mechanics | Move policy-free client execution to `pkg/platform/httpclient`; owner CLI adapters own endpoint mapping |
| `cliinputs` | Cobra input inventory plus runtime argument/flag extraction | Keep inventory under transport test/tooling; move reusable extraction to `pkg/platform/cliinput`; owner adapter resolves operation inputs |
| `climanifest` | CLI manifest model, precedence, validation; currently imports Work | Keep authored/generated contract model under top-level CLI contract tooling; remove Work and runtime-operation dependencies |
| `climanifestcobra` | Cobra projection, generic parsing, family construction, relationship rules | Split: build-only node/flag projection stays top-level; runtime extraction/resolution moves to owner adapters using Platform helpers |
| `climanifestgen` | CLI generated artifact toolchain | Retain as build/tooling package |
| `cliserver` | Server URI/legacy port normalization | Move to `pkg/platform/httpclient` or owner CLI adapters; compatibility policy stays with the affected command owner |
| `cobracompletion` | Factory catalog/signature lookups mixed with Cobra completion | Definitions CLI owns Factory completion handlers receiving raw Cobra completion args; shell-script mechanics move to Platform CLI |
| `commandidentity` | Cobra identity inventory | Keep as CLI contract test/tooling |
| `commandregistry` | Handler registries containing Factory, Work, Models, Sessions, init, run behavior | Delete aggregate; each owner publishes its command-family adapter |
| `completionprojection` | Factory/Work-aware completion policy | Move owner projections to Definitions/Work CLI; generic choice matching to `pkg/platform/cliinput` |
| `config` | Factory layout flatten/expand and error phase inference | Move to Factory Definitions CLI; owner returns typed failure category |
| `custom-factory` | Retired/fixture command tree remnants | Remove if no generated/build consumer; never a second Factory domain |
| `dashboard` | Cross-domain runtime/history projection and fallback inference | Move projection/rendering to Factory Visualization CLI |
| `default` | Default run configuration and server alias | Initializer CLI owns default application intent; top-level only attaches the alias node |
| `docs` | Topic registry, aliases, embedded lookup, index and Markdown output | New Documentation service plus `documentation/transports/cli` |
| `factory` | Named Factory CRUD, validate, activate/replace, version policy, local/HTTP modes | Move to Factory Definitions CLI; version/default policy to root |
| `factoryload` | Factory loading error classification | Move to Definitions CLI; use typed Definitions errors |
| `factoryrun` | Empty/retired package | Delete |
| `generated` | Generated CLI manifests and IDs | Retain generated output |
| `initsetup` | Settings configuration plus secondary service construction | Replace with Operator Settings CLI calling one injected root |
| `mcp` | Maps CLI values to initializer MCP intent | Move to Initializer CLI adapter; top-level CLI only registers command |
| `observation` | CLI tree/parse snapshots for process-edge tests | Move to process/CLI observation tooling; not part of runtime owner dispatch |
| `resolvedinput` | Generic precedence result model | Move reusable value mechanics to `pkg/platform/cliinput`; each owner CLI performs its resolution |
| `run` | Runtime/session/application opening, Factory selection, recording path, result inference, dashboard lifecycle | Split: Initializer CLI owns role lifecycle; Sessions owns invoke/result; Definitions selection; Recordings target; Visualization rendering |
| `runconfig` | Dependency/configuration bag containing services/effects/presentation | Delete; use immutable CLI values plus directly injected adapters |
| `sessionpath` | URL path escaping | Move to `pkg/platform/httpclient` |
| `submit` | Work decode, batch parsing, HTTP invocation, output | Move to Work CLI |
| `terminalpolicy` | stdout/stderr/quiet/logger policy | Move policy-free stream helpers to `pkg/platform/terminal`; operation output policy belongs to each owner CLI |
| `timedisplay` | time formatting | Move to the owner presentation adapter, primarily Visualization CLI |
| `work` | Thin wrappers over Work CLI | Delete wrappers after root command uses Work CLI directly |
| `workflow` | workflow preview/validation request mapping | Move to Factory Runtime CLI |

The generic CLI framework is not a service. Only the build-time/node projection
portion remains at the top-level transport. Reusable runtime mechanics live in
Platform and are called from owner CLI adapters. Its intended dependencies are
Cobra, generated CLI contract values, and protocol primitives; it must not
import a product service.

## 5. Top-level HTTP and MCP audit

### HTTP

| Current package | Audit | Final disposition |
| --- | --- | --- |
| `http` | Router/UI server plus duplicate Models/Provider Sessions handlers and direct packaged Factory catalog access | Keep generated route registration, unknown-route shell errors, and raw forwarding only; Visualization HTTP serves UI assets and owner handlers own all API responses |
| `http/application` | Runtime service-table binding and adapter construction | Delete; composition belongs to Wire |
| `http/apitypes` | Generated-support scalar codecs | Retain only protocol-scalar helpers; owner DTO mapping stays owner-local |
| `http/client` | Generated client | Retain generated |
| `http/generated` | Generated server contract | Retain generated |
| `http/workstationprojection` | Recordings historical projection mapping | Move to Recordings HTTP |
| `http/contracttests`, `http/servertests`, `http/testdata` | Public protocol verification and fixtures | Retain and retarget to composed owner adapters |

`pkg/transports/http.Server` should receive one generated aggregate forwarder
already composed by Wire. The aggregate holds owner HTTP handlers, not service
roots. Every generated method forwards the original writer, request, and
generated parameters unchanged. Dashboard UI requests forward to Factory
Visualization HTTP. The server must not decode, map, render, or construct any
owner operation.

### MCP

| Current package | Audit | Final disposition |
| --- | --- | --- |
| `mcp/server` | Generic MCP JSON/tool dispatch and framing, currently coupled to Sessions `ToolOperation` | Retain only owner-neutral registration and raw dispatch |
| `mcp/generated` | Generated discovery contract | Retain generated |
| `mcp/discoverygen` | Tool catalog generation/verification | Retain tooling |
| `mcp/stdio` | Builds runtime/session MCP roles and opens server | Move tool composition to Wire and stdio lifecycle to Initializer; generic server receives raw handlers |

Service MCP packages receive raw JSON arguments and may decode schemas, map
arguments/results/errors, and call their one owner root. Top-level MCP owns
only generated catalog registration and dispatch to the already-selected raw
handler. Factory Definitions MCP must not initialize an application or own
product filesystem resolution; Models MCP must not create model runtime scopes;
Sessions MCP must not inventory other service capabilities.

## 6. `pkg/transports/mapping` package-by-package disposition

The intended final state is that `pkg/transports/mapping` disappears. Shared
HTTP scalar helpers may remain under `pkg/transports/http/apitypes`; every
resource mapper otherwise lives beside its owner adapter.

| Current mapping package | Current responsibility | Destination |
| --- | --- | --- |
| root `contract.go`, `runtime_api.go`, `session_api.go` | Alternate service interfaces and runtime façades | Delete after roots/adapters converge |
| root `surface.go`, `workflow.go`, `session_cursor.go` | Factory Event/session result/reconnect representation | Recordings HTTP or Sessions HTTP according to canonical vs ephemeral ownership |
| root `factory_preview.go` | Workflow preview request/result mapping | Factory Runtime HTTP |
| root `factory_validation.go` | Validation mapping plus human rendering and taxonomy inference | Definitions HTTP mapping; human renderer to Definitions CLI; semantic taxonomy in Definitions |
| root `factory_status.go` | Runtime status mapping | Factory Runtime HTTP |
| `composition` | Cross-service API construction and runtime binding | Delete; Wire composes adapters |
| `factoryconfig` | OpenAPI/Factory conversion mixed with authored compatibility and layout rules | Generated conversion to Definitions HTTP; authored codec/compatibility to Definitions internal services |
| `factoryconfig/authored` | Authored YAML/agent path decoding and Worker vocabulary | Definitions internal authored codec; consume canonical Definitions values |
| `factoryconfig/diagnostics` | Definitions diagnostics | Definitions transport/internal validation |
| `factorydefinition` | Operational Definitions façade and OpenAPI mapping | Operation calls to Definitions HTTP adapter; pure maps beside it |
| `factorydefinition/retiredboundary` | Retired authored-field rejection | Definitions codec/validation with explicit compatibility removal tests |
| `factoryeventprojection` | Recordings world-state projection mapping | Recordings HTTP |
| `factorysession` | Live/durable/invocation service façades plus projections | Delete façades; Sessions HTTP mapping for sessions/response events; Recordings HTTP for canonical history |
| `factorysnapshot` | Factory snapshot OpenAPI mapping | Definitions HTTP |
| `globalconfig` | Operator config codec plus default assignment | Operator Settings codec; protocol mapping in Settings HTTP if API-exposed |
| `optional` | Generated pointer/value helpers | `http/apitypes` or small local helpers |
| `validationentry` | Calls Definitions validation and persistence mapping | Definitions root operations and Definitions HTTP mapping |
| `workcontent` | Work content/OpenAPI conversion | Work HTTP, exposed as a pure owner mapper only where nested API resources require it |
| `workerdiagnostics` | Worker diagnostics/OpenAPI conversion | Workers HTTP mapping |
| `workerinference` | Model/worker operation-binding mapping | Models HTTP for model contracts; Workers HTTP for worker contracts |
| `taxonomyvalidationtests` and mapping/OpenAPI test packages | Boundary parity evidence | Move with the owning Definitions/owner mapper tests and preserve generated parity cases |

Mapping functions must never accept a service interface or `context.Context`.
Those two traits distinguish an adapter/operation from a pure mapper and should
be enforced mechanically.

## 7. Service-by-service source-to-destination map

This is the reverse inventory: for each service owner, it identifies all
current top-level transport families that belong with it. Entries describe
ownership, not permission for the service transport to call peer services.

| Service owner | Current top-level sources to absorb | Existing owner transports | Required final correction |
| --- | --- | --- | --- |
| Automations | No central operation package identified; only aggregate HTTP/MCP registration | `automations/transports/http` | Raw handler receives protocol objects and calls Automations only; keep reconciliation/schedule policy in root |
| Factory Definitions | `cli/factory`, `cli/factoryload`, `cli/config`, Factory-specific `cobracompletion`/`completionprojection`, `mapping/factoryconfig`, `mapping/factorydefinition`, `mapping/factorysnapshot`, `mapping/validationentry`, Factory validation rendering, top-level packaged Factory HTTP route | CLI/HTTP/MCP | Own all Factory CRUD/catalog/authoring/validation/packaging/completion adapters; remove process initialization and OS policy from transports |
| Factory Runtime | `cli/workflow`, Runtime parts of `cli/run`, `mapping/factory_preview`, `mapping/factory_status`, top-level status composition | CLI/HTTP/MCP | Own raw status/control/workflow-preview adapters; no Definitions, server, dashboard, or Sessions proxy dependencies |
| Factory Sessions | Session command registry/root glue, `mapping/factorysession` session and response-event portions, invocation portions of `cli/run`, central MCP Sessions operation | CLI/HTTP/MCP | Receive raw protocol requests and call converged Sessions root; remove all adjacent owner routes and live/durable merge logic |
| Recordings | Recording/replay portions of `cli/run`, `mapping/surface`, canonical portions of `mapping/workflow` and `mapping/factorysession`, `mapping/factoryeventprojection`, `http/workstationprojection` | CLI/HTTP/MCP | Own event/reconnect/dispatch/artifact/history adapters and detached projection mapping |
| Work | `cli/submit`, `cli/batchload`, thin `cli/work`, Work-specific completion projection, `mapping/workcontent`, Work handlers duplicated under Sessions HTTP | CLI/HTTP/MCP | Own all Work protocol decode/render and call one Work admission/read/control root operation |
| Workers | `mapping/workerdiagnostics`, Worker-owned portions of `mapping/workerinference`, prompt contract/validation HTTP mapping currently under Sessions | none complete for HTTP; root execution exists | Add Workers HTTP representation adapters/mappers; actual provider integration catalog commands do not belong here |
| Providers | `cli/acp` live configuration/catalog portions, current `you workers list` and `workers acp` handlers, provider-related generated mapping | CLI/HTTP/MCP | Own provider catalog/ACP application and raw command adapters; descriptors carry source/kind |
| Models | Models command construction in CLI root/registry, Models portions of `mapping/workerinference`, duplicate top-level Models HTTP methods | CLI/HTTP/MCP | Call Models root directly; remove Sessions/Work/Workers/Settings composition from adapters |
| Provider Sessions | Duplicate top-level provider-session HTTP handler/mapping | HTTP | Keep owner HTTP handler and delete top-level duplicate; add other protocols only when exposed |
| Operator Settings | `cli/initsetup`, Settings half of `cli/acp`, `mapping/globalconfig`, operator-default resolution in CLI root | CLI/HTTP/MCP | Own raw settings commands/config codecs; remove transport-time wire construction |
| Factory Visualization | `cli/dashboard`, event/result human rendering and fallback presentation in `cli/run`, dashboard UI static serving in top-level HTTP | CLI/HTTP/MCP | Own detached presentation, terminal rendering, event redaction, and dashboard asset HTTP handling |
| System Initialization | CLI root pre-run initialization decision and system bootstrap presentation | CLI | Owner CLI receives raw command context for bootstrap policy; application role lifecycle remains Initializer |
| Documentation (new) | `cli/docs`, docs handler in `commandregistry`, docs-topic baseline metadata; embedded source currently exposed by `docs/reference` | none | Add root `List`/`Get`, `wire`, and CLI adapter; top-level CLI only attaches `docs` nodes and forwards Cobra objects |

No separate CLI/MCP Automations package should be created until a real customer
operation exists.

### Non-service destinations

| Capability | Destination | Reason it is not a new service |
| --- | --- | --- |
| Application `run`, `server`, and `mcp serve` activation/unwind | `pkg/initializer/transports/cli` | Lifecycle over already-built roles is Initializer authority |
| CLI manifest model, node/flag projection, generated IDs | `pkg/transports/cli` contract/build packages | Defines the CLI protocol itself |
| OpenAPI generated server/client and scalar support | `pkg/transports/http` | Defines the HTTP protocol itself |
| MCP generated discovery and generic registration | `pkg/transports/mcp` | Defines the MCP protocol itself |
| CLI input extraction, terminal streams, generic HTTP client, SSE/NDJSON and stdio helpers | focused `pkg/platform` packages | Policy-free mechanics called by owner adapters |
| CLI tree observations, parity checks, generators and baselines | build/test tooling | Verification and generation, not customer runtime authority |
| Binary version string | existing build-info/version package | Immutable build metadata, not a stateful product operation |
| Replaceable effect port aggregation | `pkg/services/edges` | BuildProcess construction boundary, not a product resource and therefore no protocol adapter |

## 8. Required root contract additions or corrections

Except for Documentation, these are not new services. They are the minimum
owner operations needed so a transport does not compensate for a shallow or
incomplete root.

Documentation is the one exception: it is a newly identified service because
the current CLI package already contains protocol-independent catalog and
retrieval behavior.

### Documentation

```go
type Service interface {
	List(context.Context, ListRequest) (ListResult, error)
	Get(context.Context, GetRequest) (Document, error)
}

type Topic struct {
	Name        string
	Description string
	Aliases     []string
	Order       int
}

type Document struct {
	Topic    Topic
	Markdown string
}
```

- `List` returns canonical topics in service-owned display order.
- `Get` accepts canonical names or aliases and returns the canonical identity.
- Unsupported names return a typed Documentation error containing safe
  canonical choices.
- The root owns no Cobra, output writer, binary name, or terminal formatting.
- Documentation CLI renders the quick-start/index text because command spelling
  and binary name are CLI representation concerns.
- `documentation/wire` injects the packaged `fs.FS`; service code does not
  import the CLI package.

### Factory Definitions

- A single decode/compile/validate operation for authored Factory input.
- Session-aware Current Factory read/save operations coordinated with Sessions
  through a narrow injected peer, not an HTTP façade.
- Definition-owned optimistic replacement/version progression.
- Invocation signature and default Work-type policy returned as detached
  values.
- Prompt-template context inputs derived from the selected definition without
  returning OpenAPI values.
- Workstation prompt-contract and authoring validation operations; Definitions
  may use a Workers-owned pure validator internally, while invocation-time
  rendering remains Workers-owned.

### Factory Sessions

- The converged `Start`, `Invoke`, `Activate`, `Get`, `List`, `Control`,
  `ReadResult`, `PrepareSync`, and response-subscription contract from
  `package-service-factory-sessions.md`.
- `List` returns one unified projection; no transport merging.
- `Invoke`/`ReadResult` returns canonical primary output and terminal status;
  no CLI Petri/dispatch fallback logic.

### Factory Runtime

- Observe by explicit runtime/session binding and return public counts/status.
- Workflow preview/validation as owner operations.
- Work move execution remains a Runtime implementation dependency behind Work,
  not an HTTP-visible Runtime move interface.

### Work

- One admission operation that accepts session identity and caller Work input,
  applies content preparation, definition-derived defaults, semantic
  validation, materialization, and submission.
- Work-owned typed errors for not found, invalid target state, active dispatch,
  terminated runtime, duplicate request, and validation targets.
- A detached read/move result sufficient for every CLI/HTTP/MCP renderer.

### Recordings

- Canonical event subscription/read with reconnect validation.
- Dispatch and artifact list/get operations.
- Detached workstation/world/dashboard historical projections.
- No requirement for a transport to import Factory Definitions event types.

### Models

- Direct `InvokeModel` through the Models root.
- Models-owned open/close scope only if a public request truly needs a scoped
  runtime; ordinary CLI/HTTP invoke should hide it.
- Content preparation and provider/worker execution dependencies injected into
  Models once.

### Providers and Operator Settings

- Providers accepts one effective ACP configuration operation and reports
  source/kind in provider descriptors.
- Operator Settings exposes configure/add/delete/ensure-default operations on
  its one root.
- Initializer applies effective settings before commands that consume the live
  provider catalog.

### Factory Visualization

- One detached dashboard/view projection that accepts owner-level observation
  input internally and returns presentation-ready values.
- Event redaction and customer-facing event filtering are owner presentation
  policy, shared by CLI/HTTP/MCP where equivalent.

## 9. Final dependency rules

For `pkg/services/<owner>/transports/<protocol>`:

```text
allowed:
  owner root
  generated protocol contract
  top-level generated protocol primitives
  policy-free Platform mechanics
  standard library

prohibited:
  another product service root
  owner internal packages
  another transport's operational adapter
  pkg/wire or an owner wire package
  pkg/initializer, except initializer's own transport
```

For `pkg/transports/<protocol>`:

```text
allowed:
  generated protocol contracts
  command/route/tool definition and registration
  owner-handler interfaces expressed in raw protocol objects
  aggregate forwarding to already-constructed owner adapters

prohibited:
  product service roots
  product policy
  operation input resolution or DTO mapping
  service result/error rendering or classification
  runtime/session factories
  dependency bags or service tables
```

For owner-local mapping code:

```text
allowed:
  pure request/result conversion

prohibited:
  context.Context
  service interfaces
  filesystem/network/process calls
  clocks or mutable state
  lifecycle, selection, validation policy, retries, or logging
```

The only construction direction is:

```text
pkg/wire
  -> constructs service roots once
  -> constructs each owner transport once
  -> composes raw-forwarding protocol aggregate handlers/registries once
  -> gives inert roles to pkg/initializer
```

## 10. Target package tree

```text
pkg/
  initializer/
    transports/
      cli/
        application.go       # raw Cobra handler for run/server/MCP lifecycle
        decode.go
        presentation.go
        errors.go

  platform/
    cliinput/                 # policy-free Cobra value and stdin mechanics
    terminal/                 # TTY, width, color, pager, and stream mechanics
    httpclient/               # policy-free request, auth, SSE, and retry mechanics
    stdio/                    # policy-free stdio lifecycle

  services/
    <owner>/
      *.go                    # one owner root and detached values
      internal/
      wire/
      transports/
        cli/
          handler.go          # receives *cobra.Command and []string unchanged
          decode.go           # owner command precedence and semantic conversion
          presentation.go
          errors.go
        http/
          handlers.go         # generated signature; receives writer/request/params
          decode.go           # owner request representation -> owner request value
          mapping.go          # owner values -> generated response representation
          errors.go
          streaming.go        # only where the owner exposes a stream
        mcp/
          handler.go          # receives raw JSON arguments
          definitions.go      # owner tool names/descriptions/schema contributions
          schemas.go
          mapping.go
          errors.go

    documentation/
      contracts.go            # Topic, Document, List/Get requests and typed errors
      service.go
      internal/
        catalog/
      wire/
      transports/
        cli/
          handler.go          # receives raw Cobra command/arguments
          presentation.go     # quick start, index, and Markdown output

    workers/
      transports/
        http/
          diagnostics_mapping.go
          prompt_contract_mapping.go

  transports/
    cli/
      root.go                 # create root node and attach owner nodes
      register.go             # register generated/declared command nodes
      forwarders.go           # pass *cobra.Command and []string unchanged
      execute.go              # invoke Cobra; no product error interpretation
      contract/               # command declarations and build-only node metadata
      generated/
    http/
      server.go               # create router and register generated routes
      forwarders.go           # pass writer/request/path params unchanged
      responses.go            # protocol-shell failures only (404/405/malformed path)
      apitypes/
      client/
      generated/
      contracttests/
      servertests/
    mcp/
      server/
        register.go           # register definitions and forward raw JSON
      generated/
      discoverygen/

  wire/
    cli_transports.go
    http_transports.go
    mcp_transports.go
    transport_aggregate.go

internal/
  transporttooling/
    cliobservation/           # command-tree and parse snapshots
    clicontractgen/           # manifest/baseline/generation checks
```

There is no final `pkg/transports/mapping`, `pkg/transports/http/application`,
`pkg/transports/cli/commandregistry`, `pkg/transports/cli/runconfig`, or
`pkg/transports/cli/docs`. Top-level protocol packages contain no owner DTO
conversion, input-precedence resolution, result rendering, service-aware
completion, or product error interpretation.

## 11. Directional file-family moves

```text
pkg/transports/cli/factory*              -> factory_definitions/transports/cli
pkg/transports/cli/config                -> factory_definitions/transports/cli
pkg/transports/cli/submit                -> work/transports/cli
pkg/transports/cli/work wrappers         -> delete; use work/transports/cli
pkg/transports/cli/workflow              -> factory_runtime/transports/cli
pkg/transports/cli/dashboard             -> factory_visualization/transports/cli
pkg/transports/cli/acp                   -> operator_settings + providers transports
pkg/transports/cli/initsetup             -> operator_settings/transports/cli
pkg/transports/cli/run and mcp lifecycle -> initializer/transports/cli
pkg/transports/cli/docs                  -> documentation root + transports/cli
commandregistry docs handlers            -> documentation/transports/cli
docs/reference/embed.go                  -> documentation/wire packaged source input

pkg/transports/cli/cliinputs and resolvedinput
                                          -> platform/cliinput + owner CLI adapters
pkg/transports/cli/clihttp, cliserver, sessionpath
                                          -> platform/httpclient + owner CLI adapters
pkg/transports/cli/clidiag and terminalpolicy
                                          -> platform/terminal + owner presentation
pkg/transports/cli/cobracompletion       -> owner completion providers + platform shell mechanics
pkg/transports/cli/observation, baseline, commandidentity
                                          -> internal CLI contract/test tooling
pkg/transports/cli/climanifestcobra      -> build-only top-level node registration;
                                             runtime decode/dispatch moves to owners

pkg/transports/http/application          -> delete; construct in pkg/wire
pkg/transports/http/handlers_models.go   -> delete; models/transports/http owns
pkg/transports/http/handlers_provider_session.go
                                          -> delete; provider_sessions/transports/http owns
pkg/transports/http/workstationprojection
                                          -> recordings/transports/http
pkg/transports/http dashboard/static route
                                          -> factory_visualization/transports/http
all top-level HTTP request/response mapping
                                          -> corresponding owner transports/http

pkg/transports/mcp/server owner dispatch -> owner-neutral raw handler registry
pkg/transports/mcp/stdio service wiring  -> pkg/wire + platform/stdio lifecycle

pkg/transports/mapping/factoryconfig*    -> factory_definitions internal + transports/http
pkg/transports/mapping/factorydefinition -> factory_definitions/transports/http
pkg/transports/mapping/factorysession    -> sessions/transports/http + recordings/transports/http
pkg/transports/mapping/factorysnapshot   -> factory_definitions/transports/http
pkg/transports/mapping/factoryeventprojection
                                          -> recordings/transports/http
pkg/transports/mapping/globalconfig      -> operator_settings internal/transports
pkg/transports/mapping/validationentry   -> factory_definitions root/transports/http
pkg/transports/mapping/workcontent       -> work/transports/http
pkg/transports/mapping/workerdiagnostics -> workers/transports/http
pkg/transports/mapping/workerinference   -> models or workers transports/http by resource
pkg/transports/mapping/composition       -> delete
```

The `docs/reference/*.md` authoring location does not move. What moves is the
runtime catalog, alias/order policy, lookup behavior, typed failures, and CLI
presentation. Documentation wire receives the embedded filesystem as a
construction input; neither the service root nor its adapter imports a CLI
package to discover documents.

## 12. Implementation stories

### TC-01 — Enforce the transport boundary before moving behavior

As a maintainer, I can detect new cross-domain transport policy so the
decomposition does not regress while migration proceeds.

Acceptance criteria:

- A boundary check inventories service transports importing peer service roots,
  top-level transports importing product roots, mapping functions accepting
  service/context values, and production transports importing `wire`.
- The check also rejects top-level handlers that decode owner request bodies,
  resolve owner input precedence, map owner DTOs, render owner results, or
  interpret owner errors.
- CLI aggregate handlers must forward the original `*cobra.Command` and
  `[]string`; generated HTTP aggregate methods must forward the original
  `http.ResponseWriter`, `*http.Request`, and generated path/query parameters;
  MCP dispatch must forward raw JSON arguments.
- Existing violations are captured in a deletion-only baseline.
- New violations or increases fail `make pkg-boundary`.
- Existing CLI/API behavior is unchanged.

Verification: boundary checker unit tests, `make pkg-boundary`, `make lint`.

### TC-00 — Packaged reference docs use a Documentation service

As a caller, packaged documentation has one catalog and retrieval authority,
while the top-level CLI merely registers and forwards the `docs` command.

Acceptance criteria:

- `pkg/services/documentation` exposes detached Topic/Document values, `List`
  and `Get`, stable ordering/aliases, and typed unknown-topic errors.
- Documentation wire receives the packaged `fs.FS`; canonical Markdown remains
  authored under `docs/reference`.
- `documentation/transports/cli` receives the original Cobra command and
  arguments, selects the requested topic, calls the root, and renders quick
  start, index, and topic Markdown.
- `pkg/transports/cli/docs` and command-registry docs behavior are deleted.
- `AGENTS.md`, packaged-structure ownership, package-target manifests, and
  boundary checks recognize Documentation as a service owner.
- Existing `you docs`, aliases, ordering, output, and unsupported-topic behavior
  remain covered.

Verification: Documentation root tests, CLI adapter tests, and
`make docs-reference-smoke`.

### TC-02 — Work commands and routes use one Work-owned operation

As a caller, submit/upsert/stage/list/show/move behavior is identical across
CLI, HTTP, and MCP and is decided once by Work.

Acceptance criteria:

- Work root performs preparation, definition-derived defaults, semantic
  validation, staging/materialization, and submission.
- `work/transports/http` owns every Work route.
- Sessions Work handler files and central submit/work implementations are
  deleted, not forwarded.
- Legacy state/body compatibility and error/status/exit parity remain covered.
- Work transport imports no Factory Runtime root.

Verification: Work unit/integration tests, HTTP contract tests, focused CLI
functional tests, `make api-smoke` if the contract changes.

### TC-03 — Factory commands and routes use Factory Definitions directly

As a caller, Factory list/get/create/replace/update/delete/validate/preview
selection behaves consistently without transport-owned version or validation
policy.

Acceptance criteria:

- Factory Definitions owns authored codec compatibility, validation,
  persistence, version progression, invocation signature/default policy, and
  Current Factory operations.
- Definitions CLI/HTTP/MCP own their command/routes/tools.
- Sessions Factory handlers and central Factory/config façades are deleted.
- Generated OpenAPI mapping is pure and local to Definitions HTTP.

Verification: Definitions package tests, authored fixture parity tests, CLI
functional tests, HTTP contract tests, generation check.

### TC-04 — Canonical history uses Recordings transports

As a caller, event replay, reconnect, dispatch history, artifacts, and
historical workstation views have one canonical authority.

Acceptance criteria:

- Recordings root and transports own canonical event, dispatch, artifact,
  workstation, and world-state reads.
- Sessions retains only ephemeral response-event subscription and
  session-level links.
- Reconnect validation and stream-expiry outcomes remain identical for HTTP,
  CLI, and MCP consumers.
- `factoryeventprojection` and `workstationprojection` central packages are
  deleted.

Verification: replay/projection tests, SSE reconnect contract tests, artifact
tests, race tests for streaming paths.

### TC-05 — Factory Sessions transports call the converged Sessions root

As a caller, live and persisted sessions behave as one resource across every
transport.

Acceptance criteria:

- Sessions HTTP/CLI/MCP receive only `factorysessions.Service` and protocol
  mechanics.
- List/get/control/start/invoke/result use the converged root contract.
- HTTP no longer merges live/persisted rows or selects lifecycle facets.
- The Sessions adapter has no dependency bag, peer service imports, or
  type-asserted optional capabilities.
- Public live/durable compatibility behavior remains covered until the API
  contract intentionally converges.

Verification: Sessions root tests, CLI/HTTP/MCP parity tests, lifecycle and
response-event streaming tests.

### TC-06 — Runtime status and visualization are owner projections

As a caller, status and dashboard output are based on stable owner projections,
not transport inference from Petri or ledger internals.

Acceptance criteria:

- Runtime returns public observation/status values by explicit binding.
- Visualization returns a detached dashboard/presentation view.
- CLI imports no Petri marking/token/place types and has no fallback result
  selection from dispatch history.
- Existing human/JSON output remains stable or an intentional contract update
  is documented and generated.

Verification: Runtime observation tests, Visualization projection/render tests,
CLI golden/functional tests.

### TC-07 — Models transports invoke Models only

As a caller, model list/get/pull/invoke behavior does not depend on a Sessions
presentation bridge or Workers invoker at the transport edge.

Acceptance criteria:

- Models root owns direct invocation and hides runtime scope lifecycle.
- Models CLI/HTTP/MCP import only Models plus protocol mechanics.
- `ModelsCLIPresentationCollaborator` and transport composition bridge are
  deleted.
- Content, bindings, readiness, pull, and typed failure parity remain covered.

Verification: Models unit/integration tests, CLI/HTTP/MCP contract tests,
inference failure tests.

### TC-08 — Provider and Settings commands stop joining services in CLI

As an operator, ACP add/delete/list and provider catalog behavior remains
consistent while persistence and live configuration have clear owners.

Acceptance criteria:

- Operator Settings root owns configuration document mutations.
- Providers root owns effective ACP application and descriptors report source
  and kind without suffix inference.
- Initializer applies effective settings once.
- CLI invokes one owner operation per command and performs no type assertion or
  cross-root join.
- Transport-time `settingswire.New...` construction is deleted.

Verification: Settings and Providers package tests, ACP CLI functional tests,
daemon concurrency/reuse/shutdown regression tests.

### TC-09 — Application commands are Initializer adapters

As a caller, `run`, `server`, and `mcp serve` activate and unwind the same
preconstructed application roles without a CLI-owned runtime graph.

Acceptance criteria:

- Initializer CLI maps resolved command values to application intents.
- `runconfig` contains no services/effects and is deleted or reduced to a pure
  request value owned by Initializer CLI.
- CLI does not build Factory Session runtime requests, sidecars, Models scopes,
  HTTP service tables, or recording services.
- Startup, cancellation, dashboard opening, recording reporting, and exit
  behavior retain functional coverage.

Verification: Initializer tests, root-process functional tests through
`root.BuildProcess`, shutdown/race tests.

### TC-10 — Wire composes protocol adapters once

As a maintainer, every protocol adapter is constructed once and lifecycle only
activates inert roles.

Acceptance criteria:

- Wire constructs each owner CLI/HTTP/MCP adapter directly from its owner root.
- Wire builds the aggregate generated HTTP handler, CLI family registry, and
  MCP tool registry once.
- Aggregate handlers retain no mapping callbacks: they forward raw protocol
  objects to the registered owner handler unchanged.
- `http/application`, `HTTPBinder`, `RuntimeHTTPServices`, secondary Bind
  methods, service tables, and adapter dependency bags are deleted.
- Initializer does not construct product services or adapters.

Verification: Wire composition tests, boundary tests, functional application
smoke tests.

### TC-11 — Retire the central mapping application layer

As a maintainer, representation code is discoverable beside its owner and
cannot become another service graph.

Acceptance criteria:

- Every pure mapper is moved to its owner transport.
- Every operational façade is replaced by a root call and deleted.
- `pkg/transports/mapping` no longer exists.
- Mapping tests move with owners and preserve OpenAPI round-trip/parity cases.

Verification: owner mapper unit tests, generated contract checks,
`make pkg-boundary`, `make pkg-structure`, `make pkg-file-count`.

### TC-12 — MCP composition follows the same ownership rules

As an MCP client, tools retain their schemas and results while each tool calls
one owner root and stdio lifecycle remains generic.

Acceptance criteria:

- Service MCP registries receive one owner root.
- Top-level MCP server receives a precomposed tool registry.
- MCP stdio does not construct Sessions/Runtime roles.
- Discovery generation and checked-in generated catalogs remain in sync.

Verification: MCP schema/discovery checks, service tool tests, stdio functional
tests.

### TC-13 — Seal parity, documentation, and delivery

As a customer, CLI, HTTP, MCP, dashboard, replay, and lifecycle behavior remain
coherent after transport convergence.

Acceptance criteria:

- CLI/HTTP equivalence tests cover shared operations and typed errors.
- OpenAPI and generated Go/TypeScript artifacts are synchronized for any
  intentional contract change.
- Architecture docs and package-target manifests describe the final paths.
- `make verify-pr`, relevant functional/race tests, and lint/package checks pass.
- Required CI reaches terminal green, blocking review feedback and conflicts
  are resolved, and the PR is actually merged. A pushed branch or open PR is
  not completion.

## 13. Recommended execution order

The first implementation slice should be TC-01 and TC-00, followed by TC-02.

Documentation is the cleanest proof of the corrected boundary. It is small,
self-contained, currently has real catalog and lookup policy stranded under
CLI, and requires a raw Cobra handoff all the way to a service-owned adapter.
It proves the node-builder/registrar/forwarder split without first entangling
Sessions or Runtime. Work is the next proving lane because the repository
already contains a Work root and Work-owned CLI/HTTP/MCP packages while the
Sessions adapter contains an obvious duplicate implementation. Together they
establish both a new-service extraction and an existing-service convergence.

Then execute:

1. TC-02 Work.
2. TC-03 Factory Definitions.
3. TC-04 Recordings canonical reads.
4. Factory Sessions root stories FSE-01 through FSE-04 from
   `package-service-factory-sessions.md`.
5. TC-05 Sessions transports.
6. TC-06 Runtime/Visualization and TC-07 Models.
7. TC-08 Settings/Providers.
8. TC-09 and TC-10 application lifecycle/composition.
9. TC-11 and TC-12 final central mapping/MCP removal.
10. TC-13 parity and delivery.

Do not delete `ExecutionService`, `RuntimeHTTPServices`, or central mapped
facets before the owner operations they currently compensate for exist. Do not
allow compatibility forwarding layers to survive the vertical story that
replaces them; otherwise the repository will retain both architectures.

## 14. Completion definition

Transport convergence is complete only when:

- top-level protocol packages only construct nodes/routes/tools, register owner
  handlers, and forward the original protocol request objects unchanged;
- top-level protocol packages do not import product service roots and perform no
  owner decoding, DTO mapping, input resolution, result rendering, completion
  policy, or product error interpretation;
- each service transport imports only its owner root and policy-free platform
  mechanics, receives the direct Cobra/HTTP/MCP request, and owns all
  representation transformation for that owner;
- mapping code is pure and owner-local;
- all customer operations cross one owner root before side effects;
- packaged topic catalog and retrieval behavior is owned by Documentation;
- generic CLI input, terminal, HTTP-client, SSE, and stdio mechanics live in
  focused Platform packages, while manifest generation and observation live in
  tooling rather than production dispatch;
- Wire is the only construction graph;
- Initializer is the only application lifecycle authority;
- no transport merges domain state, applies domain defaults, or infers
  terminal outcomes;
- `pkg/transports/mapping`, `pkg/transports/http/application`, transport service
  bags, and transport-time service constructors are gone;
- CLI, HTTP, MCP, replay, streaming, and generated-contract parity is proven;
- required CI and review are resolved and the implementation is merged.
