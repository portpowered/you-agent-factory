# BTRC P1 owner-contract register

This register records the dependency classification for the first BTRC P1
cutover. It is intentionally limited to the Factory Sessions runtime-opening
boundary changed by story `btrc-p1-root-injection-001`; later P1 stories extend
the register when they move external effects or operation contracts.

## Build-time owner ports

Canonical `pkg/wire` constructs these contracts once and passes each one as a
separate argument to the Factory Sessions runtime-opening factory. The
runtime-opening factory stores the exact members it consumes; it does not
retain the aggregate `edges.Edges` value or select another service graph.

| Owner | Contract | Contents |
| --- | --- | --- |
| Provider Sessions | `ProviderSessionsPorts` | Provider Sessions root |
| Factory Runtime | `FactoryRuntimePorts` | workflow definitions, preview, runtime executors, provider invocation, mock runner, assembler, clock resolver, session logger factory |
| Factory Definitions | `FactoryDefinitionsPorts` | validator, named paths, loading, replay decoding, and snapshot capabilities |
| Factory Sessions | `FactorySessionsPorts` | Sessions root, execution/scaffold/validation capabilities, runtime identity, home, and provider identity |
| Work | `WorkPorts` | Work factory and content materializer |
| Automations | `AutomationsPorts` | automation and hosted-source factories |
| Models | `ModelsPorts` | Models root |
| Recordings | `RecordingsPorts` | projection, lifecycle, ledger, recorder, replay, and input capabilities |
| Workers | `WorkersPorts` | execution/runtime/hooks and command/provider adapters |
| Operator Settings | `OperatorSettingsPorts` | backend-scope capability |

These are owner-local construction ports, not operation requests. A missing
owner contract or member fails construction before any collaborator is called.

## Operation values

`factorysessions.RuntimeOpeningRequest` remains the value-only request for the
runtime-opening operation. It contains factory, runtime, session, worker,
recording, model-cache, and resolved operator selections. The P1 operation
contract stories will remove remaining operation-time collaborators from the
opening adapters; this story does not claim those later removals.

## Private owner state

The runtime-opening `Factory` retains the selected owner ports in private
fields. Opened runtime products, cleanup scopes, recording bindings, and model
runtime scopes are operation-scoped state and are not exposed through the
construction contract. Dynamic log and metric destinations remain deferred to
the observability-owner story.

## Construction evidence

`pkg/services/factory_sessions/internal/runtimeopening/factories_test.go`
supplies distinct fakes for every owner-port contract, asserts exact identity
retention, and verifies construction does not invoke collaborator functions.
`pkg/wire/wire_gen.go` is regenerated from canonical `pkg/wire/wire.go`; no
second injector or compatibility factory is introduced.
