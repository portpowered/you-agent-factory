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

await session.submit({
  works: [
    { name: "reply", workType: "ticket", state: "ready" },
    { name: "approval", workType: "ticket", state: "ready" },
  ],
  relations: [
    {
      type: "DEPENDS_ON",
      sourceWorkName: "reply",
      targetWorkName: "approval",
      // Omit only when the target Work Type declares the default "complete".
      requiredState: "classified",
    },
  ],
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

`submit()` accepts one validated scenario Work value, a relationship-free Work
array, or a `{ works, relations }` batch. `DEPENDS_ON` is the only supported
emulator relationship; both endpoints must be names from that batch. The
complete request is validated before its atomic sink write, so an invalid item
or graph cannot partially create Work. Submissions remain available while
other Work is active and after the session returns to idle.

Accepted relationship batches emit one canonical `WORK_REQUEST` carrying the
resolved relations, followed by one `RELATIONSHIP_CHANGE_REQUEST` per relation
in declared order. The complete event sequence is one sink batch, and state and
identity counters commit only after that batch succeeds. Use
`replayFactoryEmulatorSubmissions(events)` to reconstruct detached Work identity,
initial state, payload, and outbound relationships from canonical submission
events.

Virtual time advances only when the host calls `advanceBy(durationMs)` or
`advanceToNext()`. Ready Work starts in one deterministic scheduler batch;
`advanceToNext()` then jumps to the earliest due instant and completes every
dispatch due there in stable Work order. `advanceBy()` processes every due
outcome through its requested instant. Event timestamps are always the scenario
`startAt` plus committed virtual elapsed time; these commands use no wall-clock
or browser timers. Receipts, state, status, and validation errors are detached
structured-cloneable values.

Each scheduler batch considers at most 50 eligible bindings in deterministic
Work-in-queue order. Multi-input workstations require a stable binding for every
input, and selected candidates claim all consumable Work and declared top-level
resource capacity atomically. Independent candidates can start together when
capacity permits; active dispatches retain their claims until completion, and
released capacity is available to waiting Work in the following scheduler
batch. Work with `DEPENDS_ON` relations remains ready but cannot enter a
scheduler candidate until every target Work currently occupies its normalized
required state. Dependency eligibility is recalculated after each accepted
completion batch, so newly unblocked Work can dispatch in the same `advanceBy()`
command. Worker-only resource metadata does not change mocked execution
capacity.

Accepted, continued, rejected, and failed outcomes preserve Work lineage while
routing to explicit destinations in declared order. Accepted fan-out supports
multiple outputs and both `OUTPUT_AS_PAYLOAD` and `PRESERVE_INPUT` propagation.
Missing routes use the supported Factory defaults: failures enter the input Work
Type's failed state, STANDARD rejections use the effective failure route, and
REPEATER rejections return to their inputs.

Supported workerless `LOGICAL_MOVE` workstations do not need scenario rules.
Their `VISIT_COUNT` guards read the inclusive transition counts carried by the
first authored input lineage, and an eligible move routes synchronously at the
current virtual instant without creating active worker-dispatch state. The
resulting canonical `DISPATCH_RESPONSE` preserves lineage, propagation mode,
and declared output order; session Work snapshots expose the carried `visits`
map for deterministic inspection. Zero-duration logical cycles share the
session's configured cycle and cooperative-yield boundaries.

Canonical identities and event ordering are derived from the Factory identity,
the complete validated scenario (including its seed), normalized command
inputs, command order, and virtual elapsed time. Object key insertion order,
wall-clock time, and host playback speed do not participate. `reset()` restores
the pre-start counters, rule cursors, identities, initial submissions, and
virtual-time origin while retaining the caller-owned sink, so the host can
clear or replace its own recording destination and reproduce the same supported
history byte for byte.

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

## Execution safety and cooperative scheduling

The session enforces deterministic safety budgets before a calculated batch is
sent to the sink. Defaults allow 1,000 completed dispatches, 10,000 canonical
events, one virtual hour, 1,000 consecutive zero-duration scheduler batches,
100 synchronous scheduler batches, and 1,000 Work items in one synchronous
batch. Overrides must be positive safe integers and cannot exceed the exported
`FACTORY_EMULATOR_LIMIT_HARD_CAPS`.

Crossing an event, completed-dispatch, or virtual-time budget throws
`FactoryEmulatorExecutionPausedError` and exposes a detached
`budget-exceeded` diagnostic through `status().error`. The diagnostic identifies
the limit, configured and observed values, and virtual-time context. The
calculated over-limit batch is not written or committed, and the kernel does
not fabricate failed Work. A consecutive zero-time scheduler chain uses the
distinct `zero-duration-cycle` diagnostic. Initial and runtime Work sets larger
than `maxSynchronousWorkItems` fail atomically with
`bounded-work-exceeded` before partial Work or events become visible.

Hosts can provide `yieldControl` to choose their own cooperative task boundary.
Long advancement commands await it after every `maxSynchronousBatches`
accepted scheduler batches and then resume the same serialized deterministic
command. The kernel itself does not create timers or Web Workers.

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
