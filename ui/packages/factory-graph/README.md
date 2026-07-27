# `@you-agent-factory/factory-graph`

The canonical, lossless input boundary for Factory graph renderers. A graph
source always contains the complete Factory definition, selected-tick runtime
projection, and optional saved visualization layout. Hosts retain event
streaming, persistence, and editor-session ownership.

The package also owns the reusable semantic node presentations (documents,
workers, work types, resources, work states, constraints, icons, handles, and
node chrome). Hosts supply selection callbacks and keep routing decisions
outside the package.

```ts
import { createFactoryGraphSource } from "@you-agent-factory/factory-graph";

const source = createFactoryGraphSource({
  factory,
  runtime: { topology, activity, load },
  selectedTick,
  layout,
});
```
