import { describe, expect, it } from "vitest";

import { createEmptyFactoryGraphDraft } from "./factory-graph-draft-types";
import {
  type FactoryGraphLargeEditorFixture,
  factoryGraphLargeEditorFixtures,
} from "./factory-graph-large-editor-fixtures";
import {
  applyPendingFactoryLayout,
  hasFactoryLayoutChanges,
  moveFactoryLayoutNode,
  moveFactoryLayoutNodesByDelta,
} from "./factory-graph-layout-operations";
import {
  FACTORY_GRAPH_LAYOUT_PERFORMANCE_BUDGETS,
  measureMedianOperationMs,
} from "./factory-graph-layout-performance-budgets";
import { projectFactoryGraphWithCanonicalLayout } from "./factory-graph-layout-projection";
import { applyFactoryGraphPendingEdits } from "./factory-graph-operations";

const MULTI_NODE_DRAG_SELECTION_SIZE = 20;

async function projectFixtureWithTiming(
  fixture: FactoryGraphLargeEditorFixture,
) {
  const startedAt = performance.now();
  const projected = await projectFactoryGraphWithCanonicalLayout({
    canonicalLayout: fixture.layout,
    topology: fixture.topology,
  });
  return {
    durationMs: performance.now() - startedAt,
    projected,
  };
}

async function expectFixtureWithinBudget(
  fixture: FactoryGraphLargeEditorFixture,
  options?: { projectionIterations?: number; projectionWarmup?: number },
) {
  const budget = FACTORY_GRAPH_LAYOUT_PERFORMANCE_BUDGETS[fixture.fixtureKey];
  const projectionIterations = options?.projectionIterations ?? 3;
  const projectionWarmup = options?.projectionWarmup ?? 1;

  for (let index = 0; index < projectionWarmup; index += 1) {
    await projectFixtureWithTiming(fixture);
  }

  const projectionSamples: number[] = [];
  let projected = null as Awaited<
    ReturnType<typeof projectFactoryGraphWithCanonicalLayout>
  > | null;
  for (let index = 0; index < projectionIterations; index += 1) {
    const timedProjection = await projectFixtureWithTiming(fixture);
    projectionSamples.push(timedProjection.durationMs);
    projected = timedProjection.projected;
  }

  projectionSamples.sort((left, right) => left - right);
  const projectionMedianMs =
    projectionSamples[Math.floor(projectionSamples.length / 2)] ?? 0;
  expect(projectionMedianMs).toBeLessThanOrEqual(budget.initialProjectionMs);
  expect(projected).not.toBeNull();
  if (!projected) {
    return;
  }
  const primaryNodeId = fixture.topology.nodes[0]?.id;
  expect(primaryNodeId).toBeDefined();
  if (!primaryNodeId) {
    return;
  }

  const dragSingleMedianMs = await measureMedianOperationMs(() => {
    moveFactoryLayoutNode(fixture.layout, primaryNodeId, { x: 12, y: 24 });
  });
  expect(dragSingleMedianMs).toBeLessThanOrEqual(budget.dragSingleNodeMs);

  const selectedNodeIds = fixture.topology.nodes
    .slice(0, MULTI_NODE_DRAG_SELECTION_SIZE)
    .map((node) => node.id);
  const dragMultiMedianMs = await measureMedianOperationMs(() => {
    moveFactoryLayoutNodesByDelta(
      fixture.layout,
      selectedNodeIds,
      { x: 8, y: 8 },
      projected.layoutPositionsByNodeId,
    );
  });
  expect(dragMultiMedianMs).toBeLessThanOrEqual(budget.dragMultiNodeMs);

  const pendingLayout = moveFactoryLayoutNode(fixture.layout, primaryNodeId, {
    x: 144,
    y: 288,
  });
  const saveMedianMs = await measureMedianOperationMs(() => {
    hasFactoryLayoutChanges(fixture.layout, pendingLayout);
    applyPendingFactoryLayout(fixture.factoryDefinition, pendingLayout);
    applyFactoryGraphPendingEdits({
      baseFactoryDefinition: fixture.factoryDefinition,
      draft: createEmptyFactoryGraphDraft(),
      pendingLayout,
    });
  });
  expect(saveMedianMs).toBeLessThanOrEqual(budget.saveLayoutRecomputationMs);
}

describe("factory graph layout performance budgets", () => {
  it("keeps the 100 node fixture within the documented canonical budgets", async () => {
    await expectFixtureWithinBudget(factoryGraphLargeEditorFixtures.hundred, {
      projectionIterations: 1,
      projectionWarmup: 0,
    });
  }, 30_000);

  it("keeps the 500 node fixture within the documented canonical budgets", async () => {
    await expectFixtureWithinBudget(
      factoryGraphLargeEditorFixtures.fiveHundred,
      {
        projectionIterations: 1,
        projectionWarmup: 0,
      },
    );
  }, 60_000);

  it("uses the stress 1000 node fixture to detect severe projection regressions", async () => {
    await expectFixtureWithinBudget(
      factoryGraphLargeEditorFixtures.stressThousand,
      {
        projectionIterations: 1,
        projectionWarmup: 0,
      },
    );
  }, 180_000);
});
