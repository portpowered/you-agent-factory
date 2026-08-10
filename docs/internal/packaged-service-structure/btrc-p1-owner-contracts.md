# BTRC P1 owner-contract register

This register records the dependency classification for the first five BTRC P1
cutovers. It is intentionally limited to the Factory Sessions runtime-opening
boundary changed by stories `btrc-p1-root-injection-001` and
`btrc-p1-root-injection-002`, plus the shared Factory Sessions root cutover in
`btrc-p1-root-injection-003`, and the value-only opening cutover in
`btrc-p1-root-injection-004`, and the private observability-scope ownership
cutover in `btrc-p1-root-injection-005`; later P1 stories extend the register
when they move additional operation contracts or private owner state.

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
`pkg/services/factory_sessions/internal/runtimeopening/root_reuse_test.go`
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
is introduced.
