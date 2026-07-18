# `@you-agent-factory/factory-replay`

Framework-independent replay utilities for UI-local Factory consumers. The
package deterministically orders and accepts Factory events, reconstructs
reducer-owned state at a logical tick, advances immutable checkpoints, and
projects customer Work into exclusive progress categories.

The package consumes Factory contracts from `@you-agent-factory/client` as
types only. Its runtime has no React, Zustand, dashboard, network, event-stream,
or browser-storage dependency.

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

Use `projectFactoryWorkProgressAtTick` when canonical event history is the
source for Work progress, or `projectFactoryWorkProgress` when the consumer
already has explicit selected-tick evidence.

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

## Distribution verification

Run `bun run verify` from this directory to typecheck and test the package,
produce clean ESM runtime and declaration output, validate the runtime boundary,
inspect the exact registry tarball inventory, and install both the packed client
and replay packages in a clean temporary consumer.
