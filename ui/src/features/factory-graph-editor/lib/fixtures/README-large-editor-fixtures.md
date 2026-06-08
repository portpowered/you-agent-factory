# Large factory graph editor fixtures

Repeatable large-graph fixtures for canonical layout editing performance checks.

## Fixture catalog

- `factoryGraphLargeEditorFixtures.hundred`: baseline editor graph with at least 100 topology nodes.
- `factoryGraphLargeEditorFixtures.fiveHundred`: representative editor graph with at least 500 topology nodes and documented performance budgets.
- `factoryGraphLargeEditorFixtures.stressThousand`: stress graph with at least 1000 topology nodes and looser projection budgets for severe regression detection.

Each fixture includes shared `layout.nodes` metadata for a subset of workstation nodes plus a saved viewport. Topology is built from unique work types and workstations so graph node counts scale predictably.

## Performance budgets

Budgets live in `factory-graph-layout-performance-budgets.ts` and are enforced by `factory-graph-layout-performance.test.ts`.

For the 500 node fixture:

- initial projection: 35000 ms via `projectFactoryGraphWithCanonicalLayout`
- drag response: 5 ms single-node move, 50 ms 20-node delta move
- waypoint edit and undo/redo: 5 ms add/move/remove cycle, 10 ms command apply plus inverse
- save-related layout recomputation: 50 ms median for dirty checks plus pending layout application

The 1000 node stress fixture uses a 90000 ms projection budget plus looser drag thresholds so severe regressions are caught without blocking ordinary CI on the same thresholds as the 500 node case.

Browser verification for the 500 node fixture is covered by the `FiveHundredNodeCanonicalProjection` Storybook story. That story uses synchronous grid auto-layout plus canonical `layout.nodes` resolution, `onlyRenderVisibleElements={false}`, and a fixed `defaultViewport` (avoiding imperative `fitView`, which hangs the Vitest browser runner on 500-node graphs). ELK-backed projection budgets remain enforced in `factory-graph-layout-performance.test.ts`.
