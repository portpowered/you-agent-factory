import { describe, expect, it } from "vitest";

import { createEmptyFactoryGraphDraft } from "../../draft/factory-graph-draft-types";
import {
  type FactoryGraphLargeEditorFixture,
  factoryGraphLargeEditorFixtures,
} from "../../fixtures/factory-graph-large-editor-fixtures";
import { applyFactoryGraphPendingEdits } from "../../operations/factory-graph-operations";
import {
  addFactoryLayoutEdgeWaypoint,
  moveFactoryLayoutEdgeWaypoint,
  removeFactoryLayoutEdgeWaypoint,
} from "../factory-graph-layout-edge-waypoints";
import {
  applyPendingFactoryLayout,
  hasFactoryLayoutChanges,
  moveFactoryLayoutNode,
  moveFactoryLayoutNodesByDelta,
} from "../factory-graph-layout-operations";
import { projectFactoryGraphWithCanonicalLayout } from "../factory-graph-layout-projection";
import {
  applyFactoryLayoutCommand,
  createUpdateFactoryLayoutEdgeWaypointsCommand,
  invertFactoryLayoutCommand,
} from "../history/factory-graph-layout-commands";
import {
  FACTORY_GRAPH_LAYOUT_PERFORMANCE_BUDGETS,
  measureMedianOperationMs,
} from "./factory-graph-layout-performance-budgets";

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
  const saveMedianMs = await measureMedianOperationMs(
    () => {
      hasFactoryLayoutChanges(fixture.layout, pendingLayout);
      applyPendingFactoryLayout(fixture.factoryDefinition, pendingLayout);
      applyFactoryGraphPendingEdits({
        baseFactoryDefinition: fixture.factoryDefinition,
        draft: createEmptyFactoryGraphDraft(),
        pendingLayout,
      });
    },
    { warmup: 5, iterations: 9 },
  );
  expect(saveMedianMs).toBeLessThanOrEqual(budget.saveLayoutRecomputationMs);

  const sampleEdgeId = fixture.topology.edges[0]?.id;
  expect(sampleEdgeId).toBeDefined();
  if (!sampleEdgeId) {
    return;
  }

  const waypointEditMedianMs = await measureMedianOperationMs(() => {
    let layout = addFactoryLayoutEdgeWaypoint(fixture.layout, sampleEdgeId, {
      x: 12,
      y: 24,
    });
    layout = moveFactoryLayoutEdgeWaypoint(layout, sampleEdgeId, 0, {
      x: 36,
      y: 48,
    });
    removeFactoryLayoutEdgeWaypoint(layout, sampleEdgeId, 0);
  });
  expect(waypointEditMedianMs).toBeLessThanOrEqual(budget.waypointEditMs);

  const waypointHistoryMedianMs = await measureMedianOperationMs(() => {
    const layout = addFactoryLayoutEdgeWaypoint(fixture.layout, sampleEdgeId, {
      x: 80,
      y: 90,
    });
    const command = createUpdateFactoryLayoutEdgeWaypointsCommand({
      edgeId: sampleEdgeId,
      layout: fixture.layout,
      to: [{ x: 80, y: 90 }],
    });
    if (!command) {
      throw new Error("Expected waypoint command to be created.");
    }
    const nextLayout = applyFactoryLayoutCommand(layout, command);
    applyFactoryLayoutCommand(nextLayout, invertFactoryLayoutCommand(command));
  });
  expect(waypointHistoryMedianMs).toBeLessThanOrEqual(budget.waypointHistoryMs);

  const waypointSaveMedianMs = await measureMedianOperationMs(
    () => {
      const pendingWaypointLayout = addFactoryLayoutEdgeWaypoint(
        fixture.layout,
        sampleEdgeId,
        { x: 144, y: 288 },
      );
      hasFactoryLayoutChanges(fixture.layout, pendingWaypointLayout);
      applyPendingFactoryLayout(
        fixture.factoryDefinition,
        pendingWaypointLayout,
      );
      applyFactoryGraphPendingEdits({
        baseFactoryDefinition: fixture.factoryDefinition,
        draft: createEmptyFactoryGraphDraft(),
        pendingLayout: pendingWaypointLayout,
      });
    },
    { warmup: 5, iterations: 9 },
  );
  expect(waypointSaveMedianMs).toBeLessThanOrEqual(
    budget.saveLayoutRecomputationMs,
  );
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
