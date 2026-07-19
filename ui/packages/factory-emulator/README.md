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

## Long-lived session lifecycle

Create a framework-independent session from a compatible Factory, a parsed
scenario, and a caller-owned event sink. Construction revalidates the inputs
and optional safety limits before any event is written.

```ts
import {
  createFactoryEmulatorSession,
  MemoryFactoryEventSink,
} from "@you-agent-factory/factory-emulator";

const sink = new MemoryFactoryEventSink({ maxEvents: 10_000 });
const session = createFactoryEmulatorSession({
  factory,
  scenario: parsed,
  sink,
});

const before = session.status();
const started = await session.start();
await session.submit({
  name: "follow-up",
  workType: "ticket",
  state: "ready",
  input: "Customer reply",
});
const dispatched = await session.advanceToNext();
const advanced = await session.advanceBy(250);
const current = session.state();
const closed = await session.close();
```

`start()` writes one atomic canonical bootstrap batch in `RUN_REQUEST`,
`INITIAL_STRUCTURE_REQUEST`, `SESSION_STARTED` order. The started state becomes
visible only after the sink accepts the complete batch. `state()` and
`status()` return detached, structured-cloneable snapshots; the kernel does not
retain playback or event history. An idle session remains open for later Work.

`submit()` accepts one validated scenario Work value or a non-empty batch and
normalizes it into one canonical `WORK_REQUEST`. The complete request is
validated before its atomic sink write, so an invalid item cannot partially
create Work. Submissions remain available while other Work is active and after
the session returns to idle.

Virtual time advances only when the host calls `advanceBy(durationMs)` or
`advanceToNext()`. Ready Work starts in one deterministic scheduler batch;
`advanceToNext()` then jumps to the earliest due instant and completes every
dispatch due there in stable Work order. `advanceBy()` processes every due
outcome through its requested instant. Event timestamps are always the scenario
`startAt` plus committed virtual elapsed time; these commands use no wall-clock
or browser timers. Receipts, state, status, and validation errors are detached
structured-cloneable values.

Every state-changing command retains its complete detached candidate state and
canonical event batch until the caller-owned sink accepts the batch. While a
write is pending, other state-changing commands fail with
`FactoryEmulatorPendingCommandError`; read-only snapshots remain available.
After rejection, status exposes the structured sink error and pending phase.
Retry the same command with the same arguments to write the byte-identical
batch before execution continues, or call `reset()` to explicitly discard the
rejected transaction and restore the pre-start state.

`close()` writes one canonical `SESSION_COMPLETED` terminal batch, then awaits
the sink's optional `close()` boundary before exposing the terminal closed
state. A terminal write or sink-close rejection remains retryable. Sink-close
retries never duplicate an already accepted terminal event, and successful
close rejects all later state-changing commands.

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
