# Factory emulator

`@you-agent-factory/factory-emulator` is a transport-neutral, non-React package
for deterministic Factory emulator contracts. Scenario parsing validates both
the package-local schema and references to a caller-supplied UI client
`FactoryDefinition`.

```ts
import factory from "./factory.json" with { type: "json" };
import scenario from "./scenario.json" with { type: "json" };
import { parseFactoryEmulatorScenario } from "@you-agent-factory/factory-emulator";

const parsed = parseFactoryEmulatorScenario(scenario, factory);
```

The parser returns a detached scenario value. `safeParseFactoryEmulatorScenario`
returns all structure and semantic diagnostics without partially accepting an
invalid scenario.

Factories can be preflighted against the deterministic v1 execution subset
before any event history is emitted:

```ts
import {
  inspectFactoryEmulatorCompatibility,
  writeFactoryEventsIfCompatible,
} from "@you-agent-factory/factory-emulator";

const compatibility = inspectFactoryEmulatorCompatibility(factory);
if (!compatibility.supported) {
  console.error(compatibility.diagnostics);
}

// Incompatible Factories return every diagnostic without calling sink.write.
await writeFactoryEventsIfCompatible(factory, eventBatch, sink);
```

The inspector treats an omitted orchestrator as the documented Petri default,
does not mutate or retain the Factory, and reports stable codes with paths into
the caller-supplied UI client `FactoryDefinition`.

## Caller-owned event history

`MemoryFactoryEventSink` retains ordered Factory Events up to a required
caller-selected bound. Each non-empty batch is atomic: history changes only
after the asynchronous write resolves, and rejection or capacity overflow
preserves the prior snapshot. Concurrent calls are serialized in call order.

```ts
import { MemoryFactoryEventSink } from "@you-agent-factory/factory-emulator";

const sink = new MemoryFactoryEventSink({ maxEvents: 1_000 });
await sink.write(eventBatch);
const detachedHistory = sink.snapshot();
await sink.close();
```

Input batches and returned snapshots are mutation-isolated. `close()` is
idempotent, waits for writes accepted before close, and rejects later writes
with a `FactoryEventSinkError` whose code is `closed`. Empty batches and
all-or-nothing capacity failures use the `empty_batch` and `capacity_exceeded`
codes. The optional `beforeWrite` hook provides caller-controlled backpressure
and failure injection without transferring ownership of retained history.

`RecordingFactoryEventSink` applies the same asynchronous, bounded, atomic
lifecycle to a canonical UI-client `FactoryRecording`. It rejects duplicate
event IDs, non-canonical ordering, mixed Factory or session identities, and any
batch that would make the full recording invalid, without changing the prior
snapshot.

```ts
import { RecordingFactoryEventSink } from "@you-agent-factory/factory-emulator";

const recordingSink = new RecordingFactoryEventSink({
  maxEvents: 1_000,
  recording: {
    schemaVersion: "factory-recording/v1",
    id: "customer-support-example",
    title: "Customer support example",
    factory,
  },
});
await recordingSink.write(eventBatch);
const detachedRecording = recordingSink.snapshot();
```

## Package verification

The package is built and verified independently from the dashboard. Run
`bun run verify` from this directory to check the generated schema module,
types, formatting and lint rules, focused tests, compiled output, dependency
boundary, packed inventory, and a clean installed consumer. `bun run generate`
refreshes the committed runtime schema module after editing
`schema/scenario.schema.json`.

`@you-agent-factory/client` is an explicit peer contract because the emulator
uses its canonical Factory, Factory Event, and recording APIs without bundling
or duplicating them. Consumers install both packages at the same version.
