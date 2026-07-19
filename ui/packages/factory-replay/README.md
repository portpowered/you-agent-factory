# `@you-agent-factory/factory-replay`

Framework-independent replay utilities for UI-local Factory consumers. The
package deterministically orders and accepts Factory events, reconstructs
reducer-owned state at a logical tick, advances immutable checkpoints, and
projects customer Work into exclusive progress categories.

The package consumes Factory contracts from `@you-agent-factory/client` as
types only. Its runtime has no React, React Flow, Zustand, dashboard, network,
event-stream, or browser-storage dependency.

## Usage

Import the public entry point and provide a reducer for the state your consumer
owns:

```ts
import {
  projectFactoryStateAtTick,
  type FactoryReplayReducer,
} from "@you-agent-factory/factory-replay";

const result = projectFactoryStateAtTick({ events, reducer, tick: 12 });
```

Hosts that retain only a selected-tick domain checkpoint can use
`projectFactoryWorldAtTick` and `advanceFactoryReplay`. Those APIs keep domain
state cloning, tick assignment, and world projection explicit while retaining
only accepted event IDs in the checkpoint rather than replay history.

## High-volume replay evidence

`ui/src/features/timeline/state/timeline/performance/replay-retained-memory.test.ts`
replays a deterministic 10,000-event recording through `advanceFactoryReplay`.
It budgets the UTF-8 serialized retained event history plus checkpoint at
2,000,000 bytes and verifies that three identical runs retain exactly the same
amount and produce the same final hosted projection. The harness deliberately
uses no wall-clock or process-heap assertions, which are not deterministic
across CI runtimes.

Use `projectFactoryWorkProgressAtTick` when canonical event history is the
source for Work progress, or `projectFactoryWorkProgress` when the consumer
already has explicit selected-tick evidence.
Progress classification follows failed, completed, active, queued, then
unclassified precedence. Active Dispatch evidence is ended by responses,
interruptions, and terminal reconciliation events, using the same lifecycle
rules as Dispatch overlays.

Use `projectFactoryTopologyAtTick` to reconstruct the last canonical Factory
topology effective at a selected tick. Use `projectFactoryTopology` when the
consumer already has the selected-tick Factory definition. Both return stable,
sorted public node and connection identities without mutating caller data.
`FACTORY_TOPOLOGY_RELATIONSHIPS` is the shared semantic node-kind and handle
vocabulary for renderers. Invalid relationship endpoints return a fail-closed
result with structured issues and no partial graph.

Use `projectFactoryLoadAtTick` to reconstruct distinct customer Work counts and
resource occupancy from canonical events, or `projectFactoryLoad` with explicit
selected-tick evidence. Count and occupancy entries reference the same stable
Work State and resource node IDs as the topology projection. Missing evidence
is reported as unavailable rather than fabricated as zero; dangling,
contradictory, invalid-capacity, and over-capacity evidence remains visible
through deterministic structured issues.

Use `projectFactoryActivityAtTick` to reconstruct active Dispatch overlays
after all canonically ordered events at a selected tick, or
`projectFactoryActivity` with explicit selected-tick evidence. Overlays use
stable Dispatch and Work projection IDs, reference known worker, workstation,
resource, and topology-connection IDs, and expose unavailable Work, resource,
or route evidence without inventing endpoints. Completion, interruption, and
terminal reconciliation remove activity and release occupancy.

## Distribution verification

Run `bun run verify` from this directory to typecheck and test the package,
produce clean ESM runtime and declaration output, validate the runtime boundary,
inspect the exact registry tarball inventory, and install both the packed client
and replay packages in a clean temporary consumer. The installed consumer
compiles against the public topology, semantic handle, issue, overlay, load,
and progress contracts, then verifies that repeated projections remain pure
and reproducible across caller-owned presentation changes.
