# BTRC P1 owner-contract register

This register records the dependency classification for all six BTRC P1
cutovers. It is intentionally limited to the Factory Sessions runtime-opening
boundary changed by stories `btrc-p1-root-injection-001` and
`btrc-p1-root-injection-002`, plus the shared Factory Sessions root cutover in
`btrc-p1-root-injection-003`, and the value-only opening cutover in
`btrc-p1-root-injection-004`, and the private observability-scope ownership
cutover in `btrc-p1-root-injection-005`, plus the end-to-end process proof in
`btrc-p1-root-injection-006`.

## P1 deletion register

The following rows record only seams replaced by this packet. A retained
compatibility method is called out explicitly rather than being reported as a
deletion. No compatibility factory, second injector, or runtime composition
path was added.

| Row | P1 status | Evidence |
| --- | --- | --- |
| Aggregate `RuntimeOpeningDependencies` constructor seam | Removed in story 001 | Factory Sessions runtime-opening owner contracts and construction tests use separate ports. |
| Aggregate runtime-opening external-effects bag | Removed in story 002 | Canonical Wire projects fixed owner ports; runtime opening accepts values and opaque IDs. |
| Active Factory Sessions child-assembly/`ForRuntime` construction path | Removed from the active path in story 003 | Runtime opening retains the canonical root capability; the inert compatibility method remains for unmigrated callers and is not claimed deleted. |
| Behavioral collaborators carried by operation requests | Removed in story 004 | Direct JavaScript and MCP stdio requests are value-only; presentation values are boundary-local. |
| Per-operation logger and metrics sink factories | Removed in story 005 | Runtime Log/Metrics Owners open private close-once scopes from destination values and IDs. |
| Remaining service-specific legacy seams assigned to P2-P6 | Deferred | Not changed or claimed complete by P1. |

## Build-time owner ports

Canonical `pkg/wire` constructs these contracts once and passes each one as a
separate argument to the Factory Sessions runtime-opening factory. The
runtime-opening factory stores the exact members it consumes; it does not
retain the aggregate `edges.Edges` value or select another service graph.

| Owner | Contract | Contents |
| --- | --- | --- |
| Provider Sessions | `ProviderSessionsPorts` | Provider Sessions root |
| Factory Runtime | `FactoryRuntimePorts` plus `RuntimeLogOwner` and `RuntimeMetricsOwner` | base logger, workflow definitions, preview, runtime executors, provider invocation, mock runner, assembler, clock resolver, session logger factory, selected clock, provider override, submission recorder, dispatch recorder, and process-scoped observability owners |
| Factory Definitions | `FactoryDefinitionsPorts` | validator, named paths, loading, replay decoding, and snapshot capabilities |
| Factory Sessions | `FactorySessionsPorts` | Sessions root, its directly retained runtime assembly, execution/scaffold/validation capabilities, runtime identity, home, provider identity, invocation metrics recorder, and runtime-host observer |
| Work | `WorkPorts` | Work factory and content materializer |
| Automations | `AutomationsPorts` | automation and hosted-source factories |
| Models | `ModelsPorts` | Models root |
| Recordings | `RecordingsPorts` | projection, lifecycle, ledger, recorder, replay, and input capabilities |
| Workers | `WorkersPorts` | execution/runtime/hooks, command/provider adapters, and distinct process-selected provider and script command runners |
| Operator Settings | `OperatorSettingsPorts` | backend-scope capability |

These are owner-local construction ports, not operation requests. A missing
required owner contract or member fails construction before any collaborator is
called. Optional edge replacements remain nil when absent; the canonical Wire
provider selects a platform default once for required command runners and the
runtime clock.

## Operation values

`factorysessions.RuntimeOpeningRequest`, `factorysessions.ApplicationOpeningRequest`,
and `factorysessions.InvocationTarget` are value-only selections for runtime
opening and one-shot invocation. The invocation and execution opening
interfaces accept only the request and context; their logger and metrics
behavior comes from the retained Factory Runtime and Factory Sessions owner
ports. The application-opening resolver and CLI runner adapter likewise no
longer accept a per-call logger.

Presentation boundaries remain explicit and separate from operation values. The
application-opening service receives host-readiness, lifecycle-completion,
historical-replay, and hosted-service callbacks through its explicit
`ApplicationOpeningPresentation` argument rather than its durable request.
Durable `StartRequest` is value-only; the live JavaScript owner exposes event
observation through a private `StartSyncWithEventConsumer` capability so
invocation presentation does not become request state, while ordinary durable
callers use `StartSync`.

The direct JavaScript and MCP stdio openings follow the same rule:
`DirectJavaScriptRunRequest` and `StdioOpeningRequest` contain only selection
values, while `DirectJavaScriptRunPresentation` and
`StdioOpeningPresentation` carry protocol output streams and optional host
readiness at the transport adapter boundary. Host adapters receive the
presentation value directly and cannot rebind the durable request.

## Private owner state

The runtime-opening `Factory` retains the selected owner ports in private
fields. Opened runtime products, cleanup scopes, recording bindings, and model
runtime scopes are operation-scoped state and are not exposed through the
construction contract. Hosted clock/client/secret effects and visualization
sink/root-observer effects are captured by their canonical Wire providers or
application adapter once. `RuntimeLogOwner` and `RuntimeMetricsOwner` retain
the base logger, artifact clock, collision ID generator, and path reserver at
process construction; their `Open` methods accept only destination values and
opaque session/runtime identities. Each returned log or metrics sink is
wrapped by the Factory Runtime owner in a close-once operation scope. The
shared owners are never closed by a session, so closing or failing one scope
cannot close another scope or the process root. The base logger is also
retained by the Factory Runtime owner, so application and invocation calls
cannot rebind it.

The Factory Sessions root is constructed once with the selected process clock.
Canonical Wire narrows that same root to its owner-private runtime assembly
once, and runtime opening consumes the retained capability directly. The
compatibility `ForRuntime` method is not part of the runtime-opening path and
does not construct a child service. Session gateways, runtime handles, model
scopes, and cleanup stacks remain private to each opened operation.

## Construction evidence

`pkg/services/factory_sessions/internal/runtimeopening/factories_test.go`
supplies distinct fakes for every owner-port contract, asserts exact identity
retention, and verifies construction does not invoke collaborator functions.
`pkg/services/factory_sessions/internal/applicationopening/service_test.go`
proves the value-only resolver/opening handoff and lifecycle cleanup behavior.
`pkg/services/factory_sessions/internal/runtimeopening/factories_test.go`
opens two failing runtimes concurrently and proves both use the same injected
Factory Sessions runtime root while each private Models scope is closed once.
`pkg/wire/runtime_inputs_test.go` verifies exact edge projection and one-time
default runner selection, inert observability-owner construction,
collision-free scope paths, scope isolation, and unwritable-destination errors.
The Factory Runtime scope-opening tests verify value forwarding and close-once
cleanup. The root functional support test
`tests/functional/internal/support/public_process_observation_test.go` enters
through `root.BuildProcess` and `Process.Execute` and observes the injected
provider override through public Work. `pkg/wire/wire_gen.go` is regenerated
from canonical `pkg/wire/wire.go`; no second injector or compatibility factory
is introduced. The end-to-end root-composition test
`tests/functional/sessions/root_composition/process_reuse_inert_test.go`
constructs once, observes no injected edge or observability activity, then
executes the same process twice through separate public server lifetimes and
asserts distinct session/runtime identities, isolated success/failure results,
canonical dispatch ordering, response streams, and exact runner cardinality.
