import { buildFactoryGraphTopologyFromDefinition } from "../draft/factory-graph-draft-graph";
import type { FactoryGraphNodeKind } from "../draft/factory-graph-draft-types";
import { maintainerRuntimeShapedFactory } from "../fixtures/maintainer-runtime-shaped-factory.fixture";
import {
  SYSTEM_TIME_WORK_TYPE_ID,
  systemTimeGraphNodeId,
} from "../operations/factory-graph-customer-display";
import {
  FACTORY_GRAPH_TOGGLEABLE_NODE_KINDS,
  projectFactoryGraphByHiddenNodeClasses,
} from "../work-state/factory-graph-node-class-visibility";

const runtimeTopology = buildFactoryGraphTopologyFromDefinition(
  maintainerRuntimeShapedFactory,
);

function nodeIds(topology: { nodes: { id: string }[] }) {
  return topology.nodes.map((node) => node.id);
}

function edgeIds(topology: { edges: { id: string }[] }) {
  return topology.edges.map((edge) => edge.id);
}

function nodesByKind(kind: FactoryGraphNodeKind) {
  return runtimeTopology.nodes.filter((node) => node.kind === kind);
}

function hiddenSet(...kinds: FactoryGraphNodeKind[]) {
  return new Set(kinds);
}

describe("projectFactoryGraphByHiddenNodeClasses", () => {
  it("keeps customer-display topology when no classes are hidden", () => {
    const projected = projectFactoryGraphByHiddenNodeClasses(
      runtimeTopology,
      new Set(),
    );

    expect(nodeIds(projected)).not.toEqual(
      expect.arrayContaining([
        systemTimeGraphNodeId("work-type", SYSTEM_TIME_WORK_TYPE_ID),
      ]),
    );
    expect(nodeIds(projected)).toEqual(
      expect.arrayContaining([
        "work-type:task",
        "work-state:task:init",
        "workstation:process",
        "worker:processor",
        "resource:executor-slot",
      ]),
    );
  });

  it.each(FACTORY_GRAPH_TOGGLEABLE_NODE_KINDS)(
    "hides %s nodes and incident edges when that class is hidden",
    (kind) => {
      const projected = projectFactoryGraphByHiddenNodeClasses(
        runtimeTopology,
        hiddenSet(kind),
      );

      expect(projected.nodes.every((node) => node.kind !== kind)).toBe(true);
      const hiddenNodeIds = new Set(nodesByKind(kind).map((node) => node.id));
      expect(
        projected.edges.every(
          (edge) =>
            !hiddenNodeIds.has(edge.sourceId) &&
            !hiddenNodeIds.has(edge.targetId),
        ),
      ).toBe(true);
      expect(projected.nodes.length).toBeLessThan(runtimeTopology.nodes.length);
    },
  );

  it("hides multiple node classes together", () => {
    const projected = projectFactoryGraphByHiddenNodeClasses(
      runtimeTopology,
      hiddenSet("work-state", "resource", "worker"),
    );

    expect(nodeIds(projected)).not.toEqual(
      expect.arrayContaining([
        "work-state:task:init",
        "work-state:task:done",
        "resource:executor-slot",
        "worker:processor",
        "worker:workspace-setup",
      ]),
    );
    expect(nodeIds(projected)).toEqual(
      expect.arrayContaining(["work-type:task", "workstation:process"]),
    );
    expect(
      edgeIds(projected).every(
        (edgeId) =>
          !edgeId.includes("work-state:") &&
          !edgeId.startsWith("worker-") &&
          !edgeId.startsWith("workstation-resource:"),
      ),
    ).toBe(true);
  });

  it("drops layout positions for hidden nodes when layout input is provided", () => {
    const layoutPositionsByNodeId = new Map(
      runtimeTopology.nodes.map((node, index) => [
        node.id,
        { x: index * 10, y: index * 5 },
      ]),
    );

    const projected = projectFactoryGraphByHiddenNodeClasses(
      { layoutPositionsByNodeId, topology: runtimeTopology },
      hiddenSet("workstation"),
    );

    expect(projected.layoutPositionsByNodeId?.has("workstation:process")).toBe(
      false,
    );
    expect(projected.layoutPositionsByNodeId?.has("work-type:task")).toBe(true);
    expect(projected.layoutPositionsByNodeId?.size).toBe(
      projected.nodes.length,
    );
  });
});
