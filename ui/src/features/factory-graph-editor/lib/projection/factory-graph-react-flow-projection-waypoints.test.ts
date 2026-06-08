import { describe, expect, it } from "vitest";

import { baseFactoryDefinition } from "../draft/factory-graph-draft.test-helpers";
import { buildFactoryGraphTopologyFromDefinition } from "../draft/factory-graph-draft-graph";
import { createEmptyFactoryGraphDraft } from "../draft/factory-graph-draft-types";
import { applyFactoryGraphPendingEdits } from "../operations/factory-graph-operations";
import {
  addFactoryLayoutEdgeWaypoint,
  moveFactoryLayoutEdgeWaypoint,
  removeFactoryLayoutEdgeWaypoint,
  setFactoryLayoutEdgeWaypoints,
} from "../layout/factory-graph-layout-edge-waypoints";
import { createDefaultFactoryLayout } from "../layout/factory-graph-layout-operations";
import {
  decorateProjectedEdgesWithWaypoints,
  factoryGraphReactFlowEdgeIdentity,
} from "./factory-graph-react-flow-edge-waypoint-projection";
import { projectFactoryGraphToReactFlow } from "./factory-graph-react-flow-projection";

const EDGE_ID =
  "workstation-output:workstation:draft->work-state:story:done";

function projectedEdgeIdentities(
  edges: ReturnType<typeof projectFactoryGraphToReactFlow>["edges"],
) {
  return new Map(
    edges.map((edge) => [edge.id, factoryGraphReactFlowEdgeIdentity(edge)]),
  );
}

describe("factory graph React Flow projection waypoint semantics", () => {
  it("keeps canonical edge identity when decorating projected edges with authored waypoints", () => {
    const topology = buildFactoryGraphTopologyFromDefinition(
      baseFactoryDefinition,
    );
    const baseline = projectFactoryGraphToReactFlow({ topology });
    const layout = setFactoryLayoutEdgeWaypoints(
      createDefaultFactoryLayout(),
      EDGE_ID,
      [
        { x: 120, y: 80 },
        { x: 180, y: 140 },
      ],
    );

    const decorated = decorateProjectedEdgesWithWaypoints({
      edges: baseline.edges,
      editorMode: true,
      layout,
      selectedWaypointEdgeId: EDGE_ID,
    });

    const baselineIdentities = projectedEdgeIdentities(baseline.edges);
    for (const edge of decorated) {
      expect(factoryGraphReactFlowEdgeIdentity(edge)).toEqual(
        baselineIdentities.get(edge.id),
      );
    }

    const decoratedTarget = decorated.find((edge) => edge.id === EDGE_ID);
    expect(decoratedTarget?.data?.waypoints).toEqual([
      { x: 120, y: 80 },
      { x: 180, y: 140 },
    ]);
    expect(
      decorated
        .filter((edge) => edge.id !== EDGE_ID)
        .every((edge) => edge.data?.waypoints === undefined),
    ).toBe(true);
  });

  it("keeps generated-route and authored-route edges on the same canonical identity", () => {
    const topology = buildFactoryGraphTopologyFromDefinition(
      baseFactoryDefinition,
    );
    const generated = projectFactoryGraphToReactFlow({ topology });
    const authored = decorateProjectedEdgesWithWaypoints({
      edges: generated.edges,
      editorMode: true,
      layout: setFactoryLayoutEdgeWaypoints(
        createDefaultFactoryLayout(),
        EDGE_ID,
        [{ x: 90, y: 120 }],
      ),
    });

    expect(projectedEdgeIdentities(authored)).toEqual(
      projectedEdgeIdentities(generated.edges),
    );
  });

  it("keeps projected handles compatible with rendered connection anchors after waypoint layout", () => {
    const topology = buildFactoryGraphTopologyFromDefinition(
      baseFactoryDefinition,
    );
    const projection = projectFactoryGraphToReactFlow({
      filterEdgesToRenderedHandles: true,
      topology,
    });
    const nodesById = new Map(
      projection.nodes.map((node) => [node.id, node]),
    );
    const decorated = decorateProjectedEdgesWithWaypoints({
      edges: projection.edges,
      editorMode: true,
      layout: setFactoryLayoutEdgeWaypoints(
        createDefaultFactoryLayout(),
        EDGE_ID,
        [{ x: 90, y: 120 }],
      ),
    });

    for (const edge of decorated) {
      if (!edge.sourceHandle || !edge.targetHandle) {
        continue;
      }

      const sourceAnchorIds = new Set(
        nodesById
          .get(edge.source)
          ?.data.connectionAnchors.map((anchor) => anchor.id) ?? [],
      );
      const targetAnchorIds = new Set(
        nodesById
          .get(edge.target)
          ?.data.connectionAnchors.map((anchor) => anchor.id) ?? [],
      );

      expect(sourceAnchorIds.has(edge.sourceHandle)).toBe(true);
      expect(targetAnchorIds.has(edge.targetHandle)).toBe(true);
    }
  });

  it("leaves graph topology unchanged through add, move, and remove waypoint layout operations", () => {
    const topology = buildFactoryGraphTopologyFromDefinition(
      baseFactoryDefinition,
    );
    const originalTopology = structuredClone(topology);
    const baseline = projectFactoryGraphToReactFlow({ topology });

    let layout = addFactoryLayoutEdgeWaypoint(
      createDefaultFactoryLayout(),
      EDGE_ID,
      { x: 10, y: 20 },
    );
    layout = addFactoryLayoutEdgeWaypoint(layout, EDGE_ID, { x: 30, y: 40 });
    layout = moveFactoryLayoutEdgeWaypoint(layout, EDGE_ID, 1, {
      x: 50,
      y: 60,
    });
    layout = removeFactoryLayoutEdgeWaypoint(layout, EDGE_ID, 0);

    decorateProjectedEdgesWithWaypoints({
      edges: baseline.edges,
      editorMode: true,
      layout,
    });

    expect(topology).toEqual(originalTopology);
    expect(topology.edges.map((edge) => edge.id)).toEqual(
      originalTopology.edges.map((edge) => edge.id),
    );
    for (const edge of topology.edges) {
      const original = originalTopology.edges.find(
        (candidate) => candidate.id === edge.id,
      );
      expect(edge).toEqual(original);
    }
  });

  it("leaves saved topology output unchanged after layout-only waypoint edits", () => {
    const topology = buildFactoryGraphTopologyFromDefinition(
      baseFactoryDefinition,
    );
    const pendingLayout = setFactoryLayoutEdgeWaypoints(
      createDefaultFactoryLayout(),
      EDGE_ID,
      [{ x: 200, y: 300 }],
    );
    const saveInput = applyFactoryGraphPendingEdits({
      baseFactoryDefinition,
      draft: createEmptyFactoryGraphDraft(),
      pendingLayout,
    });

    expect(saveInput.ok).toBe(true);
    if (!saveInput.ok) {
      return;
    }

    const savedTopology = buildFactoryGraphTopologyFromDefinition(
      saveInput.value,
    );

    expect(savedTopology.edges.map((edge) => edge.id)).toEqual(
      topology.edges.map((edge) => edge.id),
    );
    for (const edge of savedTopology.edges) {
      const original = topology.edges.find((candidate) => candidate.id === edge.id);
      expect(edge).toMatchObject({
        id: original?.id,
        kind: original?.kind,
        sourceId: original?.sourceId,
        targetId: original?.targetId,
      });
      expect(edge).not.toHaveProperty("waypoints");
    }
    expect(saveInput.value.layout?.edges).toEqual([
      {
        id: EDGE_ID,
        waypoints: [{ x: 200, y: 300 }],
      },
    ]);
    for (const [index, workstation] of (
      baseFactoryDefinition.workstations ?? []
    ).entries()) {
      expect(saveInput.value.workstations?.[index]).toMatchObject(workstation);
    }
  });
});
