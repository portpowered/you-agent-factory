# BTRC P1 owner-contract register

This register records the dependency classification for the first two BTRC P1
cutovers. It is intentionally limited to the Factory Sessions runtime-opening
boundary changed by stories `btrc-p1-root-injection-001` and
`btrc-p1-root-injection-002`; later P1 stories extend the register when they
move additional operation contracts or private observability state.

## Build-time owner ports

Canonical `pkg/wire` constructs these contracts once and passes each one as a
separate argument to the Factory Sessions runtime-opening factory. The
runtime-opening factory stores the exact members it consumes; it does not
retain the aggregate `edges.Edges` value or select another service graph.

| Owner | Contract | Contents |
| --- | --- | --- |
| Provider Sessions | `ProviderSessionsPorts` | Provider Sessions root |
| Factory Runtime | `FactoryRuntimePorts` | workflow definitions, preview, runtime executors, provider invocation, mock runner, assembler, clock resolver, session logger factory, selected clock, provider override, submission recorder, and dispatch recorder |
| Factory Definitions | `FactoryDefinitionsPorts` | validator, named paths, loading, replay decoding, and snapshot capabilities |
| Factory Sessions | `FactorySessionsPorts` | Sessions root, execution/scaffold/validation capabilities, runtime identity, home, provider identity, invocation metrics recorder, and runtime-host observer |
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

`factorysessions.RuntimeOpeningRequest` remains the value-only request for the
runtime-opening operation. It contains factory, runtime, session, worker,
recording, model-cache, and resolved operator selections. Runtime operation
adapters may still carry observation callbacks as compatibility fallbacks, but
the fixed observer and metrics ports selected by Wire take precedence. No
operation callback can re-read `edges.Edges`, adapt a runner, or replace a fixed
owner effect.

## Private owner state

The runtime-opening `Factory` retains the selected owner ports in private
fields. Opened runtime products, cleanup scopes, recording bindings, and model
runtime scopes are operation-scoped state and are not exposed through the
construction contract. Hosted clock/client/secret effects and visualization
sink/root-observer effects are captured by their canonical Wire providers or
application adapter once; dynamic log and metric destinations remain deferred
to the observability-owner story.

## Construction evidence

`pkg/services/factory_sessions/internal/runtimeopening/factories_test.go`
supplies distinct fakes for every owner-port contract, asserts exact identity
retention, and verifies construction does not invoke collaborator functions.
`pkg/wire/runtime_inputs_test.go` verifies exact edge projection and one-time
default runner selection. The root functional support test
`tests/functional/internal/support/public_process_observation_test.go` enters
through `root.BuildProcess` and `Process.Execute` and observes the injected
provider override through public Work. `pkg/wire/wire_gen.go` is regenerated
from canonical `pkg/wire/wire.go`; no second injector or compatibility factory
is introduced.
